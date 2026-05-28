# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-26-impl-
- HEAD commit: 30cb7410ed2f19749a46cef03ff91d9081c99a05
- Compared to: develop..HEAD

本 Issue は design / tasks フェーズを経ない design-less impl（`docs/specs/26-/` に `design.md` /
`tasks.md` は存在せず `requirements.md` / `impl-notes.md` のみ）。`_Boundary:_` アノテーションが
存在しないため、boundary 逸脱判定は変更スコープ（handler / middleware 内のリファクタ）と
requirements の Out of Scope 境界の突き合わせで実施した。Feature Flag Protocol は CLAUDE.md
宣言が `opt-out` のため flag 観点は適用しない。

## Verified Requirements

- 1.1 — 重複型 `apiErrorResponse` を削除し、フォーマット定義は `internal/middleware/error_response.go` の `ErrorResponseBody` に単一化（feed_handler.go の型削除を diff で確認）
- 1.2 — `writeAPIErrorResponse` 関数を削除し書き込み処理を `middleware.WriteErrorResponse` に単一化（feed_handler.go の関数削除を diff で確認）
- 1.3 — `grep 'writeAPIErrorResponse|apiErrorResponse'` を `internal/` 配下で実行し 0 件（等価な重複定義の残存なし）
- 1.4 — feed/item/subscription/user 全 4 handler の全エラー応答呼び出しが `middleware.WriteErrorResponse` に置換済み（4 ファイルの diff で確認）
- 1.5 — `req.URL == ""` / `req.FeedURL == ""` の URL 空チェック分岐も `middleware.WriteErrorResponse(... model.NewInvalidURLError(...))` 経由に置換（feed_handler.go diff）
- 2.1 — 集約先 `ErrorResponseBody` のフィールド構成・JSON タグ（code/message/category/action）が旧 `apiErrorResponse` と完全一致（error_response.go:12-17 と削除前型を突合）/ `TestFeedHandler_ErrorResponse_ExactJSONBody`・`TestFeedHandler_ErrorResponse_ContainsAllFields`
- 2.2 — `middleware.WriteErrorResponse` は引数 statusCode を `WriteHeader(statusCode)` にそのまま渡す（旧実装と同一）。各 handler の既存ステータス検証テスト群が PASS
- 2.3 — `WriteErrorResponse` が `Content-Type: application/json` を設定（error_response.go:22）/ `TestFeedHandler_ErrorResponse_ExactJSONBody`・`TestFeedHandler_handleServiceError_InternalError_ExactJSONBody`
- 2.4 — 401/UNAUTHORIZED の固定 JSON ボディ一致テスト `TestFeedHandler_ErrorResponse_ExactJSONBody`（新規追加）
- 2.5 — 400/INVALID_REQUEST: 既存 `TestFeedHandler_RegisterFeed_InvalidJSON_ReturnsBadRequest` ほか InvalidJSON テスト群が PASS
- 2.6 — 404/FEED_NOT_FOUND: `GetFeed` の `feed == nil` 分岐を `middleware.WriteErrorResponse` に置換、既存 NotFound テスト群が PASS
- 2.7 — 500/INTERNAL_ERROR・category=system: `handleServiceError` の非 APIError 分岐を置換、固定 JSON ボディ一致テスト `TestFeedHandler_handleServiceError_InternalError_ExactJSONBody`（新規追加）
- 2.8 — コード→ステータス対応: `mapAPIErrorToHTTPStatus` は無変更（diff に現れず）、SubscriptionLimit/ItemNotFound 等の既存テストが PASS
- 3.1 — `go build ./...` 実行し PASS（コンパイルエラーなし）
- 3.2 — `go test ./internal/handler/... ./internal/middleware/...` 実行し両パッケージ `ok`
- 3.3 — 集約後実装に対する固定 JSON ボディ一致・ステータス・Content-Type・フィールド存在の検証ケースを保持（新規 2 件 + 既存 `ContainsAllFields`）
- NFR 1.1 — `internal/middleware/error_response.go` は handler を import せず（model のみ import）、import 循環は発生しない。`go build` PASS で確認
- NFR 1.2 — 固定 JSON バイト列一致テスト + 全既存テスト PASS により差分等価を担保
- NFR 2.1 — `middleware.WriteErrorResponse` への集約完了により 1 箇所修正で全エンドポイントへ反映可能

## Findings

なし

## Summary

handler 側の重複エラーレスポンス定義（型・関数）を削除し middleware の既存 exported 実装へ
集約済み。集約先は旧実装とフィールド・JSON タグ・Content-Type・WriteHeader が完全等価で、
差分等価担保の固定 JSON ボディ一致テストが新規 2 件追加され、`go build` / `go test`（handler /
middleware）が green。全 numeric AC をカバーし、3 カテゴリいずれの逸脱もない。

RESULT: approve
