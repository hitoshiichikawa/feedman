# 実装ノート: Issue #28 FeedDetector / FaviconFetcher の HTTP クライアント再利用

## 実装サマリ

`FeedDetector` / `FaviconFetcher` はリクエストごとに `getHTTPClient()` 内で新しい
`*http.Client` を生成していたため、クライアント単位で独立したコネクションプールが割り当てられ、
TCP/TLS コネクションが再利用されていなかった。本変更では各構造体に再利用用の HTTP クライアントを
保持するフィールド `httpClient *http.Client` を追加し、コンストラクタ
（`NewFeedDetector` / `NewFaviconFetcher`）で **一度だけ** 生成して格納する形に変更した。
`getHTTPClient()` は生成済みインスタンスをそのまま返すだけになり、リクエスト間でコネクション
プールが共有される。

### 変更内容

- `internal/feed/detector.go`
  - `FeedDetector` に `httpClient *http.Client` フィールドを追加
  - タイムアウト・最大サイズをマジックナンバーから定数化（`detectorTimeout = 10 * time.Second` /
    `detectorMaxResponseSize = 5 * 1024 * 1024`。**値は従来と完全に同一**で、`DetectFeedURL` 内の
    インライン `maxBodySize` も同一定数を参照するよう統一）
  - `newDetectorHTTPClient(ssrfGuard)` ヘルパを追加し、コンストラクタから呼んでクライアントを 1 回生成
  - `getHTTPClient()` は `d.httpClient` を返すだけに変更（生成ロジックを排除）
- `internal/feed/favicon.go`
  - `FaviconFetcher` に `httpClient *http.Client` フィールドを追加
  - `newFaviconHTTPClient(ssrfGuard)` ヘルパを追加し、コンストラクタから呼んでクライアントを 1 回生成
    （タイムアウト `faviconTimeout` / 最大サイズ `maxFaviconSize` の **既存定数をそのまま使用**）
  - `getHTTPClient()` は `f.httpClient` を返すだけに変更

公開シグネチャ（`NewFeedDetector(SSRFValidator)` / `NewFaviconFetcher(SSRFValidator)`、
各公開メソッドの引数・戻り値型、`SSRFValidator` インターフェース）はいずれも変更していない（NFR 1）。

### 各 AC への対応

| AC | 対応内容 |
|---|---|
| 1.1 / 1.2 / 1.3 | コンストラクタで 1 回生成したクライアントを再利用。リクエスト都度の生成を排除しコネクションプールを共有 |
| 2.1 / 2.2 | 検出ロジック自体は不変。同一インスタンスから複数回検出しても検出結果・未検出エラーが従来と同一 |
| 3.1 / 3.2 / 3.3 | favicon 側も同様にコンストラクタで 1 回生成・再利用 |
| 4.1 / 4.2 | favicon 取得ロジックは不変。成功時データ・MIME、失敗時 nil・空 MIME・エラーなしが従来と同一 |
| 5.1〜5.5 | `ValidateURL` による事前検証はリクエスト都度実行され（再利用クライアントとは独立）、SSRF ガード経由クライアント（`NewSafeClient`）も DNS 解決後の IP 検証を含めて防御ロジックを温存。本変更ではガードの防御ロジック自体には一切触れていない |
| 6.1 / 6.2 | タイムアウト・サイズ上限の値は定数化のみで数値は不変（detector: 10s / 5MB、favicon: 5s / 2MB） |

## 並行安全性の担保方法（NFR 2.1）

クライアントを **コンストラクタで即時 1 回生成してフィールドに格納** する方式を採用した。
生成後 `httpClient` フィールドは書き換えられず read-only な参照となるため、複数 goroutine が
同一インスタンスの `getHTTPClient()` / `DetectFeedURL` / `FetchFavicon` を同時に呼んでも、
フィールドへの並行書き込みが発生せずデータ競合は起こらない（`sync.Once` による遅延初期化も
選択肢だったが、コンストラクタで `ssrfGuard` を受け取り即時生成する方が単純で並行安全なため
こちらを採用）。

`*http.Client` 自体は標準ライブラリの仕様として複数 goroutine からの並行利用が安全である。

`go test -race ./internal/feed/...` で 10 goroutine からの同時アクセステスト
（`TestFeedDetector_Concurrent_NoDataRace` / `TestFaviconFetcher_Concurrent_NoDataRace`）を
含めて green を確認済み（production code に race なし）。

> 補足: 並行テスト追加に伴い、テスト用 `mockSSRFGuard` の呼び出し回数カウンタを `atomic.Int64`
> 化した（mock 自身が race を起こさないようにするためで、production code とは無関係）。

## 追加・変更したテストと検証結果

### detector_test.go（追加）

- `TestFeedDetector_GetHTTPClient_ReusesSameInstanceWithGuard`（AC 1.1, 1.2, 5.1）: SSRF ガード有効時、
  同一インスタンスの `getHTTPClient()` が同一ポインタを返し `NewSafeClient` 呼び出しが 1 回以下
- `TestFeedDetector_GetHTTPClient_ReusesSameInstanceWithoutGuard`（AC 1.1, 1.2）: ガード無効(nil)でも同一クライアント
- `TestFeedDetector_DetectFeedURL_NoAdditionalClientPerRequest`（AC 1.2, 2.1, 3）: 3 回検出で結果一致 + クライアント追加生成なし
- `TestFeedDetector_DetectFeedURL_NotDetectedResultStable`（AC 2.2）: 未検出エラーの安定性（異常系）
- `TestFeedDetector_SSRFBlocked_StableAfterReuse`（AC 5.1, 5.3）: 再利用後も SSRF ブロック維持 + ValidateURL が各回呼ばれる（異常系）
- `TestFeedDetector_GetHTTPClient_TimeoutPreserved`（AC 6.1）: タイムアウト 10 秒の維持（境界値）
- `TestFeedDetector_Concurrent_NoDataRace`（NFR 2.1）: 10 goroutine 同時実行で race なし

### favicon_test.go（追加）

- `TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithGuard`（AC 3.1, 3.2, 5.2）
- `TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithoutGuard`（AC 3.1, 3.2）
- `TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest`（AC 3.2, 4.1, 3）: 3 回取得で結果一致 + 追加生成なし
- `TestFaviconFetcher_FetchFavicon_FailureResultStable`（AC 4.2）: 取得失敗(404)時の nil・空 MIME・エラーなしの安定性（異常系）
- `TestFaviconFetcher_SSRFBlocked_StableAfterReuse`（AC 5.2, 5.4）: 再利用後も SSRF ブロック維持（異常系）
- `TestFaviconFetcher_GetHTTPClient_TimeoutPreserved`（AC 6.2）: タイムアウト 5 秒の維持（境界値）
- `TestFaviconFetcher_Concurrent_NoDataRace`（NFR 2.1）: 10 goroutine 同時実行で race なし

### 既存テストの扱い

- 既存テストは一切弱体化・コメントアウトしていない。`mockSSRFGuard` にカウンタフィールドと
  read メソッドを追加したのみ（既存の `NewSafeClient` / `ValidateURL` の戻り値・挙動は不変）
- `DetectFeedURL` 内のインライン `maxBodySize` 定数を `detectorMaxResponseSize` 参照に統一したが
  値は `5 * 1024 * 1024` で同一（AC 6.1 維持）

### 検証結果

- `gofmt -l`（変更 4 ファイル）: 差分なし（clean）
- `go vet ./internal/feed/...`: 警告なし
- `go test -race ./internal/feed/...`: **PASS**（feed パッケージ 73 テスト全 pass / 0 fail）
- `go build ./...`: 成功
- `go test ./...`: 全パッケージ **PASS**（fail なし）

## 受入基準とテストの対応（全 numeric ID）

| AC ID | 担保テスト |
|---|---|
| 1.1 | `TestFeedDetector_GetHTTPClient_ReusesSameInstanceWithGuard` / `..._WithoutGuard` |
| 1.2 | `TestFeedDetector_GetHTTPClient_ReusesSameInstanceWithGuard` / `TestFeedDetector_DetectFeedURL_NoAdditionalClientPerRequest` |
| 1.3 | `TestFeedDetector_DetectFeedURL_NoAdditionalClientPerRequest`（クライアント再利用＝プール共有を呼び出し回数で担保） |
| 2.1 | `TestFeedDetector_DetectFeedURL_NoAdditionalClientPerRequest`（複数回検出で結果一致）+ 既存 `TestDetectFeedURL_*` |
| 2.2 | `TestFeedDetector_DetectFeedURL_NotDetectedResultStable` + 既存 `TestDetectFeedURL_HTMLNoFeedLink` |
| 3.1 | `TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithGuard` / `..._WithoutGuard` |
| 3.2 | `TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithGuard` / `TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest` |
| 3.3 | `TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest` |
| 4.1 | `TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest`（複数回取得で結果一致）+ 既存 `TestFaviconFetcher_FetchFavicon_Success` |
| 4.2 | `TestFaviconFetcher_FetchFavicon_FailureResultStable` + 既存 `TestFaviconFetcher_FetchFavicon_404` |
| 5.1 | `TestFeedDetector_SSRFBlocked_StableAfterReuse`（ガード有効 + 再利用） |
| 5.2 | `TestFaviconFetcher_SSRFBlocked_StableAfterReuse` |
| 5.3 | `TestFeedDetector_SSRFBlocked_StableAfterReuse` + 既存 `TestDetectFeedURL_SSRFBlocked` |
| 5.4 | `TestFaviconFetcher_SSRFBlocked_StableAfterReuse` + 既存 `TestFaviconFetcher_FetchFavicon_SSRFBlocked` |
| 5.5 | `TestFeedDetector_SSRFBlocked_StableAfterReuse` / `TestFaviconFetcher_SSRFBlocked_StableAfterReuse`（ValidateURL がリクエスト都度呼ばれること、および `NewSafeClient` 経由クライアント再利用で防御ロジックが温存されることを担保。DNS リバインディング対策を含む DNS 解決後の IP 検証はガード実装（`internal/security`、本 Issue 対象外）が担い、本変更はそれを呼び出すパスを変えていない） |
| 6.1 | `TestFeedDetector_GetHTTPClient_TimeoutPreserved` |
| 6.2 | `TestFaviconFetcher_GetHTTPClient_TimeoutPreserved` |
| NFR 1.1 / 1.2 | 公開シグネチャ無変更（`go build ./...` / 既存全テスト pass で担保。`var _ FaviconFetcherService = (*FaviconFetcher)(nil)` のコンパイル時チェックも維持） |
| NFR 2.1 | `TestFeedDetector_Concurrent_NoDataRace` / `TestFaviconFetcher_Concurrent_NoDataRace`（`-race` 実行） |

## 確認事項

- AC 5.5 の「DNS 解決後の IP 検証（DNS リバインディング対策を含む SSRF 検証）」は、SSRF ガード
  （`NewSafeClient` が返すクライアントのトランスポート / dialer）が担う実装で、`internal/security`
  配下にあり本 Issue のスコープ外（Out of Scope の「SSRF ガードの防御ロジック自体の変更」）。
  本変更ではガード経由クライアントを再利用するだけで、その内部のフック（DNS 解決後検証）は
  一切変更していないため、再利用後も同一に適用される。`mockSSRFGuard` は単純な
  `&http.Client{Timeout: ...}` を返すため DNS リバインディング検証の実挙動はモックでは再現
  できないが、これは本 Issue 導入前のテストでも同じであり退行ではない（実防御の検証は
  `internal/security` 側テストの責務）。
- 上記以外に requirements.md と実装の矛盾は検出されなかった。requirements.md は書き換えていない。

STATUS: complete
