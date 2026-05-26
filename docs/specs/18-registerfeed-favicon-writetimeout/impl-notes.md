# 実装ノート: Issue #18 RegisterFeed の同期 favicon 取得が WriteTimeout を超過するリスクを解消する

## 実装方針（確定済み Option A: favicon 取得の非同期化）

`FeedService.RegisterFeed` は購読作成完了後、favicon 取得の完了を待たずに `(feed, sub, nil)` を
即時返すように変更した。favicon 取得は購読作成後に独立した goroutine で非同期実行する。

- favicon 取得はリクエストスコープの `ctx` から `context.WithoutCancel(ctx)` で切り離した
  **独立 context** で実行する。これによりリクエスト ctx のキャンセル/タイムアウトに
  引きずられず、レスポンス送出後のリクエストスコープ完了で取得が中断されない（要件 3.3）。
- goroutine リーク防止のため、独立 context に `context.WithTimeout` で
  **上限時間 `backgroundFaviconTimeout = 30 * time.Second`** を付与し、完了/打ち切り時に
  `defer cancel()` でリソースを解放する（要件 4.1, 4.2, 4.3）。
- favicon 取得失敗・未検出・タイムアウト時は現行どおり null 保持（DB 更新せず、`slog` ログ
  出力のみ）。挙動を弱めていない（要件 2.3, 2.4, 2.5 / NFR 2）。
- WriteTimeout の延長（Option B）は採用していない。`internal/app/app.go` の HTTP サーバー
  設定（WriteTimeout 15s）は変更していない。

## 主要な変更点

### `internal/feed/service.go`

- `backgroundFaviconTimeout` 定数（30 秒）を追加（要件 4.1）。
- `FeedService` に `faviconWG sync.WaitGroup` フィールドを追加。バックグラウンド goroutine の
  完了を追跡する。本番フローでは `Wait` を呼ばないため本番挙動には影響せず、テストから
  非同期完了を決定論的に待つための補助。
- `RegisterFeed` の手順 5 を同期呼び出しから `startFaviconFetch`（非同期起動）に変更。
  シグネチャ・戻り値 `(*model.Feed, *model.Subscription, error)` は不変（後方互換 / 要件 5）。
- `startFaviconFetch(ctx, feedID, siteURL)` を新設。`context.WithoutCancel` で独立 context を
  生成し、`faviconWG.Add(1)` → goroutine 内で `context.WithTimeout` を付与して
  `fetchAndSaveFavicon` を呼ぶ。
- `fetchAndSaveFavicon` の引数を `*model.Feed` から `feedID, siteURL string` に変更。
  **返却済みの `feed` ポインタへの書き戻し（`feed.FaviconData = ...` / `feed.FaviconMime = ...`）を
  廃止**し、取得結果は DB の `UpdateFavicon` にのみ反映する。これにより返却済みポインタへの
  並行書き込み（データ競合）を回避（`go test -race` clean）。
- `faviconTargetURL(feed)` ヘルパーを追加（SiteURL 空時は FeedURL フォールバック。従来ロジックを抽出）。
- `waitFaviconFetch()` をテスト容易性のための補助メソッドとして追加（本番未使用）。

### `internal/feed/service_test.go`

- `mockFeedRepo` に `mu sync.Mutex` と `getFaviconCall()` ヘルパーを追加し、
  `UpdateFavicon` の `faviconCall` 記録をロックで保護（バックグラウンド goroutine からの
  書き込みとテストの読み出しの競合回避）。`UpdateFavicon` 内の返却済み feed ポインタへの
  書き戻しは本番に合わせて廃止。
- 既存 `TestFeedService_RegisterFeed_FaviconSavedOnSuccess` / `_FaviconFetchFailure` /
  `_Integration_WithHTTPServer` は非同期化に伴い `svc.waitFaviconFetch()` で完了を待ってから
  assert するよう調整（assert 内容自体は維持・強化。緩和していない）。

### `internal/feed/service_async_test.go`（新規）

新規 AC 向けテストを集約。`controllableFaviconFetcher`（block チャネル / context 観測付き）と
`deadlineObservingFetcher` を用意。

## テスト観点と AC トレーサビリティ

| Requirement / AC | テスト |
|---|---|
| 1.1, 1.2, 1.3（favicon 完了を待たず即時返却・応答時間に加算しない） | `TestRegisterFeed_ReturnsBeforeFaviconCompletes`（favicon を block したまま RegisterFeed が 2 秒以内に返り、応答時間が 1 秒未満であることを検証） |
| 1.4 / NFR 1.1（WriteTimeout 15s 未満で返す） | `TestRegisterFeed_ReturnsBeforeFaviconCompletes`（即時返却を担保。15s の WriteTimeout に対し応答は favicon 取得時間と無関係） |
| 2.1, 2.4（取得失敗でも登録成功・null 保持） | `TestRegisterFeed_SucceedsWhenFaviconFetchErrors` / 既存 `TestFeedService_RegisterFeed_FaviconFetchFailure` |
| 2.2（タイムアウト時も登録成功） | `TestRegisterFeed_ReturnsBeforeFaviconCompletes`（取得が長時間でも登録は成功）+ `TestStartFaviconFetch_AppliesTimeoutDeadline`（タイムアウト機構の存在）|
| 2.3（未検出時 null 保持） | `TestRegisterFeed_SucceedsWhenFaviconNotFound`（data==nil 境界値） |
| 2.5 / NFR 2.1（失敗/未検出/タイムアウトのログ記録） | `fetchAndSaveFavicon` 内の `slog.Warn`/`slog.Info`（feedID/siteURL 付き）。テスト実行ログで出力を確認 |
| 3.1, 3.2（成功時に保存・後続で参照可能） | `TestRegisterFeed_FaviconSavedAsynchronously` / 既存 `TestFeedService_RegisterFeed_FaviconSavedOnSuccess` |
| 3.3（リクエストスコープ完了で中断しない） | `TestRegisterFeed_FaviconContinuesAfterRequestCtxCanceled`（リクエスト ctx キャンセル後も独立 context で取得完了・保存。呼び出し時 ctx が未キャンセルであることも検証） |
| 4.1（上限時間 30 秒以内） | `TestBackgroundFaviconTimeout_IsBounded`（定数 ≤ 30s）/ `TestStartFaviconFetch_AppliesTimeoutDeadline`（context にデッドライン設定） |
| 4.2（上限到達で打ち切り） | `TestStartFaviconFetch_AppliesTimeoutDeadline`（WithTimeout context を fetcher に渡している＝上限到達で ctx がキャンセルされる機構を担保） |
| 4.3（完了/打ち切りでリソース解放） | `defer cancel()` による解放。`go test -race` 全 green（goroutine リーク・競合なし） |
| 5.1, 5.3（レスポンス形式・既存エラー応答維持） | 既存 handler テスト（`internal/handler/feed_handler_test.go`）全 green。`RegisterFeed` シグネチャ不変 |
| 5.2（201 Created 維持） | 既存 `TestFeedHandler_RegisterFeed_Success` 等（ハンドラー層未変更で維持） |
| NFR 3.1（レスポンス形式 feed/subscription 同一） | `RegisterFeed` 戻り値・handler 層未変更。既存テストで担保 |

## 品質ゲート結果

- `go build ./...`: OK
- `go vet ./...`: OK
- `gofmt -l`（変更ファイル）: 差分なし（clean）
- `go test ./... -race`: 全パッケージ PASS

## 確認事項（レビュワー判断ポイント）

- **データ競合回避の設計**: 返却済み `feed` ポインタへの favicon 書き戻しを廃止した。
  これにより登録レスポンスには favicon バイナリが含まれない（従来も登録レスポンスは
  feed/subscription のみで、favicon バイナリは別経路の想定 / 要件 NFR 3.1 と整合）。
  favicon は DB の `UpdateFavicon` 経由でのみ反映され、後続のフィード取得操作（`GetFeed` 等）で
  参照可能（要件 3.2）。
- **テスト補助メソッド `waitFaviconFetch()` / `faviconWG`**: 本番フローでは `Wait` を呼ばず
  挙動に影響しない。非同期完了を決定論的に検証するための最小限の機構であり、本番挙動を
  弱めるものではない（テスト規約「実装ではなくテスト側を弱める」には該当しない）。
- **`mockFeedRepo.UpdateFavicon` の書き戻し廃止**: 本番 `fetchAndSaveFavicon` が返却済み
  ポインタへ書き戻さなくなったため、モックも同様に共有ポインタへの書き込みを廃止し
  `faviconCall` 記録のみとした（`-race` clean のため）。既存テストの assert は維持。
- requirements.md との矛盾は検出されなかった。

STATUS: complete
