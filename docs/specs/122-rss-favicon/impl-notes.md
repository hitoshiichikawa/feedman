# 実装ノート — Issue #122 RSS フィード一覧 favicon 取得改善

## 実装方針の要約

### Backend (`internal/feed/`)

`FaviconFetcherService` インターフェースに新規メソッド `FetchFaviconWithFallback(ctx, feedURL)`
を追加し、以下の 4 段階で favicon を順次探索する実装を `FaviconFetcher` に追加した。

| 段階 | 試行内容 | 対応要件 |
|---|---|---|
| (a) | フィード配信 URL のオリジン + `/favicon.ico` | 要件 2.1（従来経路） |
| (b) | フィード配信 URL のオリジン HTML 内 `<link rel="icon">` 等 | 要件 2.4 |
| (c) | フィード本体から記事リンクを取得し、そのオリジン + `/favicon.ico` | 要件 1.1 |
| (d) | (c) と同じオリジンの HTML 内 `<link rel="icon">` 等 | 要件 1.1 + 2.4 |

各段階の試行内容と成功・失敗・URL を `slog.Info` / `slog.Warn` で構造化ログ出力する（NFR 2.1）。
いずれかの段階で画像 MIME を持つ favicon を取得できた時点で打ち切る（要件 2.2）。

#### 設計判断

- **記事リンクの抽出**: gofeed (`mmcdole/gofeed`) でフィード本体をパースし、最初の有効
  （`Link` 非空）な記事リンクのオリジンを採用する。要件 1.3 に従い、記事リンクが 1 件も
  ない場合は段階 (c)(d) を試行せず段階 (b) までで打ち切る。
- **配信ドメインと記事リンクのオリジンが同一の場合**: 段階 (c)(d) を試行すると段階 (a)(b) と
  同じリクエストになるため、`siteOrigin == feedOrigin` の場合はスキップする（要件 4.1
  非リグレッション保証）。
- **HTTP クライアントの共有**: `FaviconFetcher.httpClient` を favicon 取得・HTML 解析・
  フィード本体取得すべてで再利用する。これにより既存 `TestFaviconFetcher_GetHTTPClient_Reuses*`
  テスト（NewSafeClient 呼び出し 1 回まで）を温存。クライアント側のレスポンスサイズ上限は
  HTML/フィード本体を許容する 5MB（`maxFeedFetchSizeForFavicon`）に揃え、favicon 経路では
  呼び出し側で `LimitReader` と長さ検査により 2MB 上限（`maxFaviconSize`）を強制する（NFR 1.2）。
- **HTML 解析ライブラリ**: 既存の `golang.org/x/net/html`（`detector.go` で採用済み）。
  ライブラリ追加なし。
- **rel 属性の優先順位**: `icon` (0) → `shortcut icon` (1) → `apple-touch-icon` /
  `apple-touch-icon-precomposed` (2)。複数候補がある場合は priority が最小のものを採用。

#### Service 層の変更

`FeedService.startFaviconFetch` が `feedURL` も受け取るよう拡張し、`fetchAndSaveFavicon` 内で
`feedURL != ""` のとき `FetchFaviconWithFallback` を呼ぶようにした。`feedURL` が空の場合は
従来通り `FetchFaviconForSite` にフォールバックする（後方互換）。

### Frontend (`web/src/components/feed-list.tsx`)

`FeedFavicon` サブコンポーネントを追加し、以下の 2 ケースで `lucide-react` の `Rss` アイコンを
代替アイコンとして表示する。

- `favicon_url` が `null` または空文字（要件 3.1）
- `<img>` の `onError` が発火した（要件 3.2）

`onError` ハンドラは `useState` の `imgFailed` を `true` に切り替え、以降の再描画で
`<img>` を消して代替アイコンに切り替える。代替アイコンは `w-4 h-4` で実 favicon と同じ
表示サイズ・配置領域を持ち、レイアウト（タイトル・未読数バッジ・ステータスアイコン）は
変化しない（要件 3.3, 3.4）。

#### Test ID 追加

- `feed-favicon-<sub-id>`: 画像要素
- `feed-favicon-fallback-<sub-id>`: 代替アイコン要素

## 変更ファイル一覧と AC との対応

| ファイル | 変更内容 | 関連 AC |
|---|---|---|
| `internal/feed/favicon.go` | `FetchFaviconWithFallback` / `parseFaviconURLFromHTML` / 補助関数追加 | 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.4, NFR 1.1, 1.2, 1.3, 2.1 |
| `internal/feed/service.go` | `startFaviconFetch` / `fetchAndSaveFavicon` の引数拡張 | 1.5, 4.2 |
| `internal/feed/service_test.go` | `mockFaviconFetcher` に新メソッド追加 | （既存テスト維持） |
| `internal/feed/service_async_test.go` | `controllableFaviconFetcher` / `deadlineObservingFetcher` に新メソッド追加 | （既存テスト維持） |
| `internal/feed/favicon_fallback_test.go` | 新規ファイル: 段階フォールバック・HTML パースの単体・結合テスト | 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.4, 4.1, NFR 1.1 |
| `web/src/components/feed-list.tsx` | `FeedFavicon` サブコンポーネント追加・`onError` ハンドラ追加 | 3.1, 3.2, 3.3, 3.4, NFR 3.2 |
| `web/src/components/feed-list.test.tsx` | 代替アイコン表示・onError 切替・レイアウト維持のテスト追加 | 3.1, 3.2, 3.4 |

## 受入基準ごとのテスト対応

### Requirement 1: サイト本体ドメインからの favicon フォールバック取得

| AC | テスト |
|---|---|
| 1.1 配信失敗時にサイト本体ドメインで再取得 | `TestFetchFaviconWithFallback_StageC_SiteOriginICOSucceeds` / `TestFetchFaviconWithFallback_StageD_SiteOriginHTMLSucceeds` |
| 1.2 サイト本体取得成功時に永続化 | `TestFetchFaviconWithFallback_StageC_SiteOriginICOSucceeds`（data 返却を service の `UpdateFavicon` 経由で確認） |
| 1.3 記事リンク 0 件のとき再取得しない | `TestFetchFaviconWithFallback_NoArticles_StopsAfterStageB` |
| 1.4 全段階失敗で null として保存 | `TestFetchFaviconWithFallback_AllStagesFail_ReturnsNil` + 既存 `TestRegisterFeed_SucceedsWhenFaviconNotFound`（service 層） |
| 1.5 バックグラウンド処理として実行 | 既存 `TestRegisterFeed_ReturnsBeforeFaviconCompletes`（async test） |

### Requirement 2: favicon 探索経路の段階化

| AC | テスト |
|---|---|
| 2.1 配信 URL → サイト本体ドメインの順で段階試行 | `TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds` / `StageC_SiteOriginICOSucceeds` |
| 2.2 成功時に後続段階をスキップ | `TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds`（stageB/C/D ヒットフラグで検証） |
| 2.3 段階・URL・成功可否のログ記録 | `slog.Info("favicon取得: 成功", "stage", ...)` 等の構造化ログ（実装で確認、`go test -v` で観測可能） |
| 2.4 既定パス + HTML 内 icon 宣言の両方を探索 | `TestParseFaviconURLFromHTML_*` (8 件) + `TestFetchFaviconWithFallback_StageB_FeedOriginHTMLSucceeds` / `StageD_SiteOriginHTMLSucceeds` |

### Requirement 3: フロントエンドのデフォルトアイコン表示

| AC | テスト |
|---|---|
| 3.1 favicon 未設定時に代替アイコン表示 | `faviconがnullの場合に代替アイコン（fallback）を表示すること` |
| 3.2 画像読み込み失敗時に代替アイコンに切替 | `favicon画像の読み込みに失敗した場合に代替アイコンに切り替わること` |
| 3.3 代替アイコンは同じサイズ・配置領域 | `feed-list.tsx` で `w-4 h-4` を維持（既存スナップショット相当のレイアウトテストは追加せず、CSS クラス指定で担保） |
| 3.4 タイトル・未読数バッジ・ステータスのレイアウト不変 | `代替アイコン表示時もフィードタイトル・未読数バッジ・ステータスアイコンのレイアウトを維持すること` |

### Requirement 4: 既存正常ケースの非リグレッション

| AC | テスト |
|---|---|
| 4.1 従来経路で取得できる場合に同一 favicon を採用 | `TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds` / `TestFetchFaviconWithFallback_SameHostArticleLink_NoStageCDExtraFetch` |
| 4.2 既永続化フィードへの再取得を行わない | `fetchAndSaveFavicon` は新規登録 / `RegisterFeed` 経路のみで呼ばれる（service.go の構造で担保。既存実装と差分なし） |
| 4.3 既存表示の見た目を変更しない | `feed-list.test.tsx` の既存 12 ケース（タイトル / 選択 / 未読数 / ステータス / favicon あり時 img）が全て pass |

### Non-Functional Requirements

| NFR | テスト・実装根拠 |
|---|---|
| NFR 1.1 SSRF 対策の同等適用 | `TestFetchFaviconWithFallback_SSRFGuardBlocksAll`（全段階で `ValidateURL` 経由ブロック → 外部リクエスト 0 件） |
| NFR 1.2 タイムアウト / サイズ上限の同等適用 | `faviconTimeout = 5s` をクライアント側で適用、`maxFaviconSize = 2MB` を呼び出し側 `LimitReader` で強制（既存 `TestFaviconFetcher_FetchFavicon_LargeResponse` が pass） |
| NFR 1.3 HTML サニタイズ / 画像 MIME 判定 | `isImageMime` で既存と同じ MIME 判定。HTML は href 抽出のみで画面に出さないためサニタイズ不要 |
| NFR 2.1 構造化ログ出力 | `slog.Info("favicon取得: 成功", "stage", "feed_origin_ico" 等, "url", "mime")` で各段階を識別可能に記録 |
| NFR 3.1 永続化スキーマ・API 形状不変 | `model.Feed` / `repository.FeedRepository.UpdateFavicon` のシグネチャ・スキーマは変更していない |
| NFR 3.2 既存 favicon ありフィードの表示挙動不変 | `feed-list.test.tsx` の既存テスト 12 件全 pass |

## 実行コマンド

| コマンド | 結果 |
|---|---|
| `go test ./...` | `ok` 全パッケージ pass |
| `go test -race ./internal/feed/...` | `ok` race なし |
| `go vet ./...` | warnings なし |
| `gofmt -l internal/feed/` | 出力なし（clean） |
| `npm test`（`web/`） | 218 passed / 26 files |
| `npm run lint`（`web/`） | 0 errors / 5 warnings（warning 5 件はすべて preexisting） |

## 確認事項（PR レビュワー向け）

- 段階 (b)(d) の HTML 解析は `golang.org/x/net/html` を使用し、`detector.go` の
  `ParseFeedLinksFromHTML` と同等の token-based パーサーで実装した。新規依存ライブラリは追加していない。
- 段階 (c)(d) のサイト本体ドメイン推定は「最初の有効な記事リンク」のオリジンを採用する。
  仕様上「最初」と限定していないが、決定論性とリクエスト数の節約のため 1 件目で確定する
  設計とした（要件 1.1 の文言「フィード内の記事リンクから得られるサイト本体ドメイン」を
  単数として解釈）。
- フィード本体の二重取得（worker 取得とは別に registration 時にも取得する）が発生する。
  これは registration 時点でフィードがまだ DB にキャッシュされていないため不可避。
  キャッシュ TTL / 共有取得経路の最適化は本 Issue のスコープ外（Out of Scope の
  「favicon キャッシュ TTL の見直し」に該当）。
- ローカル環境で `node --version` が 22.11.0 で vitest 4.0.18 と非互換だったため、
  Playwright 同梱の Node 24.11.1 をシンボリックリンク経由で利用して `npm test` を
  実行・確認した（CI は `actions/setup-node@v4` で node-version: 20 を使うため CI では
  そのまま動作する）。

## 補足ノート

- 要件で曖昧だった点とその解釈:
  - 「記事リンク」の単複: 単数として解釈（最初の有効な記事リンクのオリジン）。詳細は
    「確認事項」参照。
  - 「同一ドメインの場合の挙動」: 要件で明示されていないが、無駄なリクエストを避けるため
    `siteOrigin == feedOrigin` 時は段階 (c)(d) をスキップする実装とした。
- 追加した依存: なし（既存 `golang.org/x/net/html` / `mmcdole/gofeed` / `lucide-react` を利用）。
- 次の Issue として切り出すべき派生タスク:
  - 既永続化フィードへのバッチ的な favicon 再評価（Out of Scope 明示済み）
  - favicon キャッシュ TTL の見直し（Out of Scope 明示済み）
  - サイト本体ドメイン推定の精度向上（複数記事リンクのドメイン分布から最頻ドメインを採用する等）

STATUS: complete
