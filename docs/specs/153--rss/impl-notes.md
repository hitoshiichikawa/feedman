# 実装ノート: Issue #153「登録できないRSSがある」

## 概要

Feed Detector がフィード URL の HTTP レスポンスのステータスコードを判定せず、
非 2xx 応答（ボット保護による 429 やサーバーエラー）でも HTML として
パースしていたため「フィード未検出」として誤って失敗していた問題を修正した。

本実装では `internal/feed/detector.go` の `DetectFeedURL` に HTTP ステータスコード
判定を追加し、非 2xx（< 200 || >= 300）の場合は新しいエラー `FEED_HTTP_ERROR`
を返すようにした。これにより UI 側がユーザーに「サイト側のブロック / URL 間違い
/ 一時的にサーバが応答しない」可能性を示唆できるようになる。

## 設計判断

### 1. エラー粒度（Open Question への対応）

要件 Open Questions では「4xx と 5xx で別エラーコードを持つか、まとめて 1 つの
カテゴリにするか」が論点として挙げられていた。本実装では **1 つのエラーコード
`FEED_HTTP_ERROR` + `Details["status_code"]` でステータスコードを付帯情報として
表現する** 方針を採った。

理由:

- Req 2.4 が「ステータスコードを付記する」ことを要求しており、`Details` の
  メカニズムで満たせる（既存 `FEED_COOLDOWN` の `retry_after_seconds` と同パターン）
- 4xx / 5xx の区別はユーザー向けメッセージ上では同一の対処（別 URL を試す /
  時間を置いて再試行）に集約できるため、コード分割の実益が低い
- 将来 403 / 404 / 429 などをカテゴリ細分化する必要が生じても、`Details` を拡張
  すれば対応可能（後方互換性を保ちながら拡張できる）

### 2. HTTP ステータスコードマッピング（handler 層）

`mapAPIErrorToHTTPStatus` で `FEED_HTTP_ERROR` を **`502 Bad Gateway`** にマップした
（既存 `FETCH_FAILED` と同じステータス）。理由:

- 「上流サイトが応答を返したがエラー応答だった」状況は、API として見れば
  上流ゲートウェイのエラーを通知する 502 が semantically 適切
- `FETCH_FAILED`（レスポンス取得前の失敗）と HTTP ステータスは同じだが、
  エラーコード / メッセージ / Details は別系統で UI が区別表示できる
- 422 Unprocessable Entity（`FEED_NOT_DETECTED` のマッピング）と区別することで、
  「ユーザー入力の URL は正常だが上流側で問題」という意味を表現できる

### 3. 3xx 最終応答の扱い

Go の `http.Client` は既定でリダイレクトを最大 10 回追跡するため、最終応答に
3xx が残るのは Location ヘッダ欠落等の限定ケースであることを確認した。
本実装では Req 1.4 に従い、`resp.StatusCode < 200 || resp.StatusCode >= 300` で
判定する 1 つの分岐に統合し、3xx も同様に `FEED_HTTP_ERROR` として扱う。
テスト `TestDetectFeedURL_HTTPError_3xxFinalResponse` で Location なしの 302 を
返すケースを検証している。

### 4. ボディ読み込みのスキップ

非 2xx 応答時は `io.ReadAll` を呼ばずに即座にエラーを返す実装にした。

- Req 4.4「4xx 応答内のリンクは信頼しない」の意図に沿っている
- 不要な I/O / メモリ確保を回避（NFR 1.2 とは別軸だが運用負荷を下げる）
- テスト `TestDetectFeedURL_HTTPError_4xx_DoesNotParseHTMLFeedLink` で
  4xx + HTML 内 RSS リンクのケースを検証

### 5. ログ出力（NFR 2.1）

`slog.Warn` で `url` と `status` を出力。既存 `internal/feed/favicon.go` の
`FetchFavicon` での非 2xx ログ出力（"favicon取得: HTTPステータス異常"）と同じ
パターンを採用した。これにより運用者が頻発するブロッキング先を grep で特定できる。

## 確認事項

### 解決済み

- **3xx 最終応答の発生頻度**: Go `http.Client` の既定挙動を確認した上で、
  Location ヘッダ欠落等の限定ケースとして同一分岐で扱うと決定（Req 1.4）
- **HTTP ステータスコードマッピング**: handler 層 `mapAPIErrorToHTTPStatus` で
  `FEED_HTTP_ERROR` を `StatusBadGateway` (502) にマップ済み（既存 `FETCH_FAILED`
  と同じ）

### 未解決（PR レビュワー / 運用判断に委ねる）

- **UI 側のメッセージ表示**: Req 2.1 / 2.2 は「Feed Registration UI shall ...」と
  記述されているが、本 PR は backend のエラー仕様までを対象とし、Next.js 側の
  メッセージ表示は別途確認が必要。UI 側は既存の APIError 表示機構（Code /
  Message / Action / Details）を経由して新エラーコードを受け取れる
- **Worker 側のフィード取得**: Out of Scope に明記されている通り、定期フェッチ処理
  （`internal/worker/fetch/`）の同様の HTTP エラー扱い改善は本 Issue 対象外。
  必要なら別 Issue で扱う

## テスト追加内容

### `internal/feed/detector_test.go`

| テスト名 | 観点 | 対応 AC |
|---|---|---|
| `TestDetectFeedURL_HTTPError_4xx` | 404 / 429 / 403 で `FEED_HTTP_ERROR` 返却、`Details["status_code"]` に int でステータス載せる | Req 1.3, 1.5, 2.4 |
| `TestDetectFeedURL_HTTPError_5xx` | 500 / 503 で `FEED_HTTP_ERROR` 返却、Category=feed、Action 非空 | Req 1.3, 1.5, 2.1, 2.4 |
| `TestDetectFeedURL_HTTPError_4xx_DoesNotParseHTMLFeedLink` | 4xx 応答ボディに HTML feed link があっても HTML パースせず HTTP エラー返却 | Req 4.4 |
| `TestDetectFeedURL_HTTPError_3xxFinalResponse` | Location なし 302 を 3xx 最終応答として HTTP エラー処理 | Req 1.4 |
| `TestDetectFeedURL_2xxStillWorks` | 200 応答の従来の検出フローが継続動作 | Req 3.1, NFR 1.1 |

### `internal/model/errors_test.go`

| テスト名 | 観点 | 対応 AC |
|---|---|---|
| `TestNewFeedHTTPError` | 4 種のステータスコード（429/404/503/500）で Code / Category / Details["status_code"] / Action 構造を検証 | Req 1.5, 2.1, 2.3, 2.4 |

### 既存テストの回帰確認

- `TestDetectFeedURL_DirectRSSFeed` / `TestDetectFeedURL_DirectAtomFeed` /
  `TestDetectFeedURL_HTMLWithFeedLink` 等の 200 応答系既存テストは無修正で
  全件 pass（Req 3.1, 3.2, NFR 1.1 を担保）

## 受入基準達成確認

| AC ID | 担保するテスト |
|---|---|
| Req 1.1 (HTTP レスポンス受信時にステータスコードを判定対象に) | `TestDetectFeedURL_HTTPError_4xx/5xx` （ステータスコード分岐を経由して FEED_HTTP_ERROR を返す）|
| Req 1.2 (2xx は既存フロー継続) | `TestDetectFeedURL_2xxStillWorks` + 既存 `TestDetectFeedURL_DirectRSSFeed` 等 |
| Req 1.3 (4xx/5xx は HTML パースせず固有エラー) | `TestDetectFeedURL_HTTPError_4xx` / `TestDetectFeedURL_HTTPError_5xx` / `TestDetectFeedURL_HTTPError_4xx_DoesNotParseHTMLFeedLink` |
| Req 1.4 (3xx 最終応答も HTTP エラー扱い) | `TestDetectFeedURL_HTTPError_3xxFinalResponse` |
| Req 1.5 (HTTP エラーと FEED_NOT_DETECTED を別コード) | `TestDetectFeedURL_HTTPError_4xx/5xx` （Code != FEED_NOT_DETECTED を assert）+ `TestNewFeedHTTPError` |
| Req 2.1 (原因示唆 + 対処) | `TestNewFeedHTTPError`（Action 非空）+ Message テンプレ（「ブロック / URL 間違い / 一時的にサーバが応答しない」を含む） |
| Req 2.2 (2xx 未検出は既存メッセージ継続) | 既存 `TestDetectFeedURL_HTMLNoFeedLink` |
| Req 2.3 (内部詳細を露出しない) | `NewFeedHTTPError` は固定テンプレ Message でレスポンス body を含まない |
| Req 2.4 (ステータスコード付記) | `TestDetectFeedURL_HTTPError_4xx` / `TestNewFeedHTTPError`（`Details["status_code"]` int 検証 + Message に "HTTP" 含む） |
| Req 3.1 (2xx + フィード Content-Type の従来結果) | `TestDetectFeedURL_2xxStillWorks` |
| Req 3.2 (2xx + HTML + フィードリンクの従来優先順位) | 既存 `TestDetectFeedURL_HTMLWithFeedLink` / `TestDetectFeedURL_HTMLWithMultipleFeedLinks_PrioritySelection` |
| Req 3.3 (ネットワーク失敗は既存 FETCH_FAILED) | 既存 `client.Do(req)` の err 分岐を変更せず維持（コード review で確認可） |
| Req 3.4 (新たな外部依存追加なし) | `go.mod` 変更なし |
| Req 4.1 (200 + 空ボディ → FEED_NOT_DETECTED) | 既存 `IsDirectFeed` / `ParseFeedLinksFromHTML` の挙動を維持 |
| Req 4.2 (200 + Content-Type 欠落 + XML 有効 → 検出成功) | 既存 `TestIsDirectFeed_XMLContentTypeWithRSSBody` 等 |
| Req 4.3 (200 + 非 HTML/XML → FEED_NOT_DETECTED) | 既存挙動を維持（detector.go の `!strings.Contains(strings.ToLower(mediaType), "html")` 分岐は HTTP 判定後に位置） |
| Req 4.4 (4xx + HTML link でも HTTP エラー) | `TestDetectFeedURL_HTTPError_4xx_DoesNotParseHTMLFeedLink` |
| NFR 1.1 (2xx の挙動同一) | `TestDetectFeedURL_2xxStillWorks` + 既存テスト全件 pass |
| NFR 1.2 (5MB 上限維持) | `io.LimitReader(resp.Body, detectorMaxResponseSize)` を変更せず |
| NFR 1.3 (10 秒タイムアウト維持) | `detectorTimeout` を変更せず（`TestFeedDetector_GetHTTPClient_TimeoutPreserved` が引き続き pass）|
| NFR 2.1 (URL + status をログ出力) | `slog.Warn("フィード検出: HTTPステータス異常", "url", inputURL, "status", resp.StatusCode)` を追加 |
| NFR 3.1 (ボット保護回避コードを追加しない) | User-Agent / Cookie 永続化 / JS 評価などは一切追加せず |

## 動作確認方法

```bash
# 単体テスト
cd /home/hitoshi/.issue-watcher/worktrees/hitoshiichikawa-feedman/slot-1
go test ./internal/feed/... ./internal/model/...

# 全パッケージテスト
go test ./internal/...

# 静的解析
go vet ./...
gofmt -l internal/feed/ internal/model/ internal/handler/
```

実機検証手順（手動）:

1. API サーバを起動し、フィード登録エンドポイントに `https://www.wtwco.com/ja-jp/search/getsearchrssfeed?type=Insight`
   を POST する
2. レスポンスとして `502 Bad Gateway` + `{"code":"FEED_HTTP_ERROR","message":"...HTTP 429...","details":{"status_code":429}}`
   が返ることを確認
3. 通常のフィード URL（例: `https://blog.golang.org/feed.atom`）が引き続き登録できることを回帰確認

STATUS: complete
