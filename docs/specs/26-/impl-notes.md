# 実装ノート: Issue #26 エラーレスポンス関数の二重定義を統一する

## 概要

`api` のエラーレスポンス処理が `internal/middleware` と `internal/handler` の 2 箇所に
重複定義されていた技術債を、middleware 側の既存実装へ集約して解消した。ユーザー可視の
振る舞い（JSON 構造・HTTP ステータス・Content-Type）は完全に同一を維持する差分等価の
リファクタである。

## 集約先の判断理由

- 統一フォーマット型 `ErrorResponseBody` と書き込み関数 `WriteErrorResponse` /
  `WriteInternalServerError` は既に `internal/middleware/error_response.go` に存在し、
  exported かつ専用テスト（`internal/middleware/error_response_test.go`）も整備済みだった。
- handler 側 `apiErrorResponse` 型・`writeAPIErrorResponse` 関数は middleware 側と
  フィールド構成・JSON タグ（`code`/`message`/`category`/`action`）・処理内容が完全に等価で、
  unexported のため再利用性が低かった。
- handler パッケージは既に `internal/middleware` を import 済み。middleware は handler を
  import していないため、middleware へ集約しても **import 循環は発生しない**（NFR 1.1 充足）。

以上より「handler 側の重複定義を削除し、middleware 側の既存 exported 実装へ集約」する方針を
採った。

## 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `internal/handler/feed_handler.go` | 重複型 `apiErrorResponse` と関数 `writeAPIErrorResponse` を削除。ハンドラ本体・`handleServiceError` の全呼び出しを `middleware.WriteErrorResponse` に置換 |
| `internal/handler/item_handler.go` | `writeAPIErrorResponse` 呼び出し（6 箇所）を `middleware.WriteErrorResponse` に置換 |
| `internal/handler/subscription_handler.go` | `writeAPIErrorResponse` 呼び出し（5 箇所）を `middleware.WriteErrorResponse` に置換 |
| `internal/handler/user_handler.go` | `writeAPIErrorResponse` 呼び出し（1 箇所）を `middleware.WriteErrorResponse` に置換 |
| `internal/handler/feed_handler_test.go` | 固定 JSON ボディ完全一致の回帰テストを 2 件追加（401 / 500） |

- 集約後、handler パッケージ内に等価な重複定義は **1 つも残っていない**（`grep` で確認済み /
  Req 1.3 充足）。
- handler 側で重複削除後も `encoding/json` import は `RegisterFeed` 等の
  `json.NewDecoder`/`json.NewEncoder` で引き続き使用されており、未使用 import は発生しない
  （`go build ./...` で検証済み）。`user_handler.go` は元々 `json` を import しておらず変更不要。

## 後方互換の担保方法（Req 2 / NFR 1.2）

- `middleware.WriteErrorResponse(w, statusCode, *model.APIError)` は旧 `writeAPIErrorResponse` と
  シグネチャが同一で、フィールド構成・JSON タグ・`Content-Type: application/json` 設定・
  `WriteHeader(statusCode)` の処理が完全に等価。引数（statusCode と `*model.APIError`）を
  そのまま渡すため出力 JSON バイト列は不変。
- `handleServiceError` の 500 系は `middleware.WriteInternalServerError` を使わず、従来と同じ
  `model.APIError{Code: "INTERNAL_ERROR", Category: "system", ...}` を `WriteErrorResponse` へ
  渡す形を維持した。これにより従来と完全に同一の JSON ボディ・メッセージ文言を保証する
  （`WriteInternalServerError` も同一ボディを生成するが、明示的に同一引数を保つ方が差分等価の
  検証が容易なため）。
- 新規回帰テスト `TestFeedHandler_ErrorResponse_ExactJSONBody` /
  `TestFeedHandler_handleServiceError_InternalError_ExactJSONBody` で、フィールド順序を含む
  固定 JSON バイト列（末尾改行込み）・HTTP ステータス・Content-Type が既知の固定値と
  完全一致することを検証。リファクタ前のコミットでこのテストが green であることを確認してから
  本体を変更しているため、置換前後でレスポンスが不変であることを担保している。

## ビルド・テスト結果

- `go build ./...`: **PASS**（コンパイルエラーなし / Req 3.1 充足）
- `go test ./...`: **全パッケージ PASS**（`internal/middleware`・`internal/handler` 含む全 19
  パッケージで `ok` / Req 3.2 充足）
- `go vet ./internal/handler/... ./internal/middleware/...`: **PASS**

### gofmt について（参考）

`gofmt -l` では `internal/handler/integration_test.go` /
`internal/handler/router_full_test.go` / `internal/middleware/ratelimit_test.go` /
`internal/handler/feed_handler_test.go` の既存箇所が報告されるが、これらは
**いずれも本件の変更前から存在する既存の gofmt 非準拠**（`git stash` 前後で同一を確認）であり、
本リファクタで新たに導入したものではない。本件で追加したテストコード（`feed_handler_test.go`
への追加分）は gofmt 準拠（`gofmt -d` の差分に追加分は現れない）。スコープ外（既存コードの
盲目的整形は禁止規約）のため既存非準拠箇所には手を入れていない。

## 受入基準とテストの対応

| Req ID | 内容 | 担保テスト / 検証 |
|---|---|---|
| 1.1 / 1.2 | フォーマット定義・書き込み処理の単一化 | `internal/middleware/error_response.go` への集約（`go build` PASS） |
| 1.3 | 等価な重複定義を残さない | `grep 'writeAPIErrorResponse\|apiErrorResponse'` で 0 件を確認 |
| 1.4 / 1.5 | 各エンドポイント・URL 空チェック等が単一実装を経由 | 全 4 handler の呼び出し置換 + 既存テスト群（`TestFeedHandler_RegisterFeed_EmptyURL_ReturnsBadRequest` 等） |
| 2.1 | 同一 JSON フィールド・タグ名 | `TestFeedHandler_ErrorResponse_ExactJSONBody`, `TestFeedHandler_ErrorResponse_ContainsAllFields` |
| 2.2 | 同一 HTTP ステータスコード | 各 handler の `*_NoUserID_ReturnsUnauthorized` / `*_ReturnsBadRequest` / `*_ReturnsNotFound` / `*_ReturnsInternalServerError` 等の既存テスト群 |
| 2.3 | Content-Type=application/json | `TestFeedHandler_ErrorResponse_ExactJSONBody`, `TestFeedHandler_handleServiceError_InternalError_ExactJSONBody` |
| 2.4 | 401 / UNAUTHORIZED | `TestFeedHandler_ErrorResponse_ExactJSONBody`（固定ボディ一致）+ 全 handler の `*_NoUserID_ReturnsUnauthorized` |
| 2.5 | 400 / INVALID_REQUEST | `TestFeedHandler_RegisterFeed_InvalidJSON_ReturnsBadRequest`, `TestFeedHandler_UpdateFeedURL_InvalidJSON_ReturnsBadRequest`, item/subscription の InvalidJSON テスト |
| 2.6 | 404 / FEED_NOT_FOUND | `TestFeedHandler_GetFeed_NotFound`, `TestFeedHandler_GetFeed_OtherUsersFeed_ReturnsNotFound`, `TestFeedHandler_UpdateFeedURL_FeedNotFound_ReturnsNotFound` |
| 2.7 | 500 / INTERNAL_ERROR・category=system | `TestFeedHandler_handleServiceError_InternalError_ExactJSONBody`（固定ボディ一致）, `TestFeedHandler_RegisterFeed_InternalError_ReturnsInternalServerError` |
| 2.8 | コード→ステータス対応（INVALID_URL→400 / SSRF_BLOCKED→403 / SUBSCRIPTION_LIMIT→409 / ITEM_NOT_FOUND→404 等） | `TestFeedHandler_RegisterFeed_SubscriptionLimit_ReturnsConflict`, `TestFeedHandler_RegisterFeed_FeedNotDetected_ReturnsUnprocessableEntity`, item/subscription 各テスト（`mapAPIErrorToHTTPStatus` は無変更で対応維持） |
| 3.1 | ビルド通過 | `go build ./...` PASS |
| 3.2 | 既存テスト通過 | `go test ./...` 全 PASS |
| 3.3 | 書き込み処理・ステータス・フィールド存在の検証ケース保持 | 既存テスト群 + 追加した固定 JSON 一致テスト 2 件 |
| NFR 1.1 | import 循環を新たに発生させない | middleware は handler を import しない（`go build` PASS で循環なしを確認） |
| NFR 1.2 | 外部観測可能な振る舞いを差分等価に保つ | 固定 JSON バイト列一致テスト + 全既存テスト PASS |
| NFR 2.1 | 1 箇所修正で全エンドポイントへ反映できる単一定義 | `middleware.WriteErrorResponse` への集約完了 |

## 実装上の判断

- `handleServiceError` の 500 系で `middleware.WriteInternalServerError(w)` を使う選択肢も
  あったが、従来コードと同一の `model.APIError` リテラルを `WriteErrorResponse` に渡す形を
  維持した（両者は同一ボディを生成するが、差分等価の明示性を優先）。

## Feature Flag Protocol

本リポジトリ `CLAUDE.md` の `## Feature Flag Protocol` 節の `**採否**:` は `opt-out` のため、
flag 裏実装の追加フローは適用していない（通常の単一実装パスで実装）。

## 確認事項

- なし。要件・現状調査の通りに集約でき、後方互換も担保できた。`mapAPIErrorToHTTPStatus`
  （コード→ステータス対応規則）は Out of Scope のため一切変更していない。

STATUS: complete
