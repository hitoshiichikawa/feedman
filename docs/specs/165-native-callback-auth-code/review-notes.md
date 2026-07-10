# Review Notes (#165)

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-12T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-165-impl-native-callback-auth-code
- HEAD commit: e5f02c5f90d6a5caac770a6b856cd5a5f7e3a97c
- Compared to: origin/develop..HEAD
- Feature Flag Protocol: opt-out（3 カテゴリ判定のみ）

## 判定サマリ

requirements.md の全 numeric ID（Req 1.1〜1.6 / 2.1〜2.5 / 3.1〜3.4 / 4.1〜4.2 / NFR 1.1〜1.3 / NFR 2.1 /
NFR 3.1）に対応する実装と新規テストがそろっており、変更ファイルは design.md の File Structure
Plan の枠内（`internal/auth/{pkce,native,service}.go`, `internal/handler/auth_handler.go`,
`internal/handler/{auth_handler_test,integration_test}.go`, `internal/app/app.go`,
`docs/specs/165-native-callback-auth-code/tasks.md`）に収まっている。`go build ./...` /
`go vet ./...` / `go test ./internal/auth/ ./internal/handler/ ./internal/app/` いずれも green。

## Verified Requirements

- 1.1 — `AuthHandler.Login` の native 分岐で `oauth_state` + `oauth_native_challenge` Cookie を設定し
  Google へ 307 redirect（`auth_handler.go` Login）。`TestAuthHandler_Login_Native_SetsChallengeCookieAndRedirects` で検証
- 1.2, 1.3, 1.4 — `auth.ValidatePKCES256`（method=S256 厳密一致 / 43 文字 base64url 検証）と
  Login の 400 固定メッセージ応答。`TestValidatePKCES256` の table-driven ケース、および
  `TestAuthHandler_Login_Native_InvalidPKCE_Rejects` で検証
- 1.5 — `flow=native` 指定なしの Web flow は既存パスをそのまま通過。既存
  `TestAuthHandler_Login_RedirectsToGoogleWithState` 等が無修正で green
- 1.6 — `flow=native` 不在かつ残存 `oauth_native_challenge` cookie がある場合のみ
  `clearNativeChallengeCookie` を発行。`TestAuthHandler_Login_Web_ClearsStaleNativeCookie` で検証
- 2.1 — `AuthHandler.handleNativeCallback` が 303 `feedman://auth/callback?auth_code=...` を返却。
  `TestAuthHandler_Callback_Native_RedirectsToAppSchemeWithoutSession` および統合テスト
  `TestIntegration_NativeAuthFlow_LoginCallbackReturnsAuthCode` で検証
- 2.2 — `auth.HandleNativeCallback` が `NativeAuthCodeTTL = 60s` で `model.AuthCode`
  (CodeHash / UserID / PKCEChallenge / ExpiresAt / Used=false) を `AuthCodeCreator.Create` に渡す。
  `TestHandleNativeCallback_ExistingUser_IssuesHashedAuthCode` で各フィールドを検証
- 2.3 — native 分岐は `service.HandleCallback` / `createSession` 経路を通らず session_id Cookie も
  発行しない。`TestHandleNativeCallback_DoesNotCreateSession` および handler 側の
  session_id Cookie 不在検証で確認
- 2.4 — `resolveUserFromOAuth` を `HandleCallback` / `HandleNativeCallback` の双方から呼び出して
  既存ユーザー / 新規ユーザーを Web flow と同一規則で扱う。
  `TestHandleNativeCallback_ExistingUser_...` / `TestHandleNativeCallback_NewUser_CreatesUserAndIssuesCode` で検証
- 2.5 — `HandleNativeCallback` は平文 code を戻り値のみで返し、保存は SHA-256 hash、ログは
  `auth_code_hash` の先頭 8 文字のみ。失敗パスでも平文を含まないことを
  `TestHandleNativeCallback_StoreFails_ReturnsErrorWithoutPlaintext` で確認
- 3.1 — state 検証（既存定数時間比較）は handler の native 分岐より前に位置し、失敗時に
  `handleNativeCallback` を呼ばない。`TestAuthHandler_Callback_Native_InvalidState_RejectsWithoutIssuingCode` で検証
- 3.2 — native cookie 不在時は既存 Web flow に fallthrough（`auth_handler.go` Callback の
  `if nativeCookie, ... err == nil` 分岐の外側）。既存 Web 統合テストが無修正で green
- 3.3 — `handleNativeCallback` 冒頭で `clearNativeChallengeCookie` を成否非依存に発行し
  単回破棄を保証。`TestAuthHandler_Callback_Native_RedirectsToAppSchemeWithoutSession` の
  削除 Set-Cookie 検証で確認
- 3.4 — service / 形式不正のいずれもクライアントには固定メッセージ（500 `authentication failed` /
  400 `invalid pkce parameters`）。`TestAuthHandler_Callback_Native_ServiceError_Returns500` /
  `TestAuthHandler_Callback_Native_TamperedChallenge_Rejects` で検証
- 4.1, 4.2 — Web flow のコード経路（`HandleCallback` / `createSession` / cookie 設定 /
  BaseURL リダイレクト）は無変更。既存 `TestIntegration_AuthFlow_LoginCallbackMeLogout` /
  `TestAuthHandler_Callback_NewUser_CreatesSessionAndRedirects` 等が無修正 pass
- NFR 1.1 — `generateAuthCode` が `crypto/rand` 32 byte（256bit）を `base64.RawURLEncoding` で
  URL-safe 文字列化（`native.go`）
- NFR 1.2 — `ValidatePKCES256` の `method != "S256"` early return（plain / 小文字 s256 / 欠落を拒否）
- NFR 1.3 — Login / Callback の 4xx / 5xx 応答はすべて固定文字列（`invalid pkce parameters`,
  `invalid state parameter`, `missing authorization code`, `authentication failed`）で
  クライアント入力・内部詳細を反射しない
- NFR 2.1 — Web flow 既存テストが無修正で green。`TestAuthHandler_Login_Web_NoNativeCookie_HeadersUnchanged` で
  残存 native cookie が無いリクエストの応答ヘッダ完全不変を検証
- NFR 3.1 — OAuth provider は既存 `mockOAuthProvider`、auth_code repo は `mockAuthCodeCreator`
  でモック化し、すべての検証が外部ネットワーク非依存

## Findings

なし

## 検証ログ

```
$ go build ./...
（成功）

$ go vet ./...
（成功）

$ go test ./internal/auth/ ./internal/handler/ ./internal/app/
ok  	github.com/hitoshi/feedman/internal/auth	(cached)
ok  	github.com/hitoshi/feedman/internal/handler	(cached)
ok  	github.com/hitoshi/feedman/internal/app	(cached)
```

DB 結合テストは `TEST_DATABASE_URL` 未設定環境で skip される既存慣習どおり（impl-notes.md 記載と一致）。

## Summary

全 AC（Req 1〜4 / NFR 1〜3）に対応する実装とテストが揃い、変更は File Structure Plan の枠内に
完全に収まっている。`go build` / `go vet` / `go test ./internal/auth/ ./internal/handler/ ./internal/app/`
すべて green。boundary 逸脱・AC 未カバー・missing test のいずれにも該当しない。

RESULT: approve
