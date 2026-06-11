# Review Notes (#166)

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-11T22:17:36Z -->

## Reviewed Scope

- Branch: `claude/issue-166-impl-auth-token-exchange`
- HEAD commit: `1915d28f1c6a7e0ca065fc832a6cf5426ead1a00`
- Compared to: `origin/develop..HEAD`
- Feature Flag Protocol: **opt-out**（CLAUDE.md 宣言値）→ flag 観点は適用せず、通常 3 カテゴリ判定のみ。

### 判定サマリ

- requirements.md の全 numeric ID（Req 1.1〜3.5 / NFR 1.1〜3.1）について、対応する実装またはテストがすべて存在することを確認した。
- tasks.md の `_Boundary:_` は task 5（`(P)` なし）以外には明示が無く、変更ファイルはすべて design.md の File Structure Plan 内に閉じる。boundary 逸脱なし。
- `go build ./... && go vet ./... && go test ./...` を実行し、全 22 パッケージ green を再確認した。
- impl-notes.md が明示した実装判断 2 点（`DisallowUnknownFields` / 部分失敗の 500 分類）は、いずれも design.md（Out of Scope / 部分失敗の扱い）と整合しており、reject 対象外。

### 注記: 差分 stat に映る `docs/specs/167-*` / `168-*` の削除

`git diff --stat origin/develop..HEAD` に `docs/specs/167-auth-refresh-rotation/` と `docs/specs/168-reuse-detection-revoke/` の削除（合計 750 行）が表示されるが、これは本 impl ブランチの merge-base（`9c94cc6`、#166 design merge 後）が古く、develop に後から merge された #167 / #168 spec PR（`b869f23` / `e2a0e80`）の差分が「未存在」として映っているだけ。`git log 9c94cc6..HEAD -- docs/specs/167-*/ docs/specs/168-*/` は空で、本ブランチ自身は該当 spec を変更していないため、boundary 逸脱としては扱わない。

## Verified Requirements

### Requirement 1: Token 交換の成功パス

- **1.1** — `handler.TestNativeAuthHandler_Token_Success`（200 + 4 フィールド検証）/ `handler.TestNewRouter_NativeAuthToken_RegisteredWhenHandlerInjected` / `handler.TestIntegration_NativeAuthFlow_CallbackThenExchangeSucceeds`（callback → exchange 通し）
- **1.2** — `auth.TestExchangeAuthCode_Success`（MarkUsed が stored.ID で 1 回呼ばれる）/ `handler.TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400`（同一 code 再交換が 400）
- **1.3** — `auth.TestExchangeAuthCode_Success`（TokenHash = `HashNativeSecret(plain)` / FamilyID = family.ID / UserID 整合 / ExpiresAt = `now + 30d`）
- **1.4** — `auth.TestIssueAccessToken_SuccessfulIssuance`（HS256 / sub = userID / `exp - iat == 900` / 自己署名検証）/ `auth.TestExchangeAuthCode_Success`（ExpiresIn=900）/ `auth.TestIssueAccessToken_WrongSecretFailsVerification`（自己完結性: 鍵秘匿前提で検証可能）
- **1.5** — `handler.TestNewRouter_NativeAuthToken_DoesNotRequireSession`（Cookie 無しで 200）/ `internal/handler/router.go` の認証不要グループ内登録
- **1.6** — `handler.TestNativeAuthHandler_Token_Success`（`access_token` / `refresh_token` / `token_type` / `expires_in` の snake_case 4 フィールド）

### Requirement 2: 交換の拒否パス

- **2.1** — `auth.TestExchangeAuthCode_VerifierMismatch`（MarkUsed 呼ばれず）/ `auth.TestVerifyPKCES256Verifier`（末尾 1 文字改変ケース等）
- **2.2** — `auth.TestExchangeAuthCode_AuthCodeNotFound`（`FindByHash` が nil → `ErrInvalidGrant`）/ `handler.TestIntegration_NativeAuthFlow_UnknownAuthCodeReturns400`
- **2.3** — `auth.TestExchangeAuthCode_MarkUsedNotUsable`（`repository.ErrAuthCodeNotUsable` → `ErrInvalidGrant`）/ `handler.TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400`
- **2.4** — `auth.TestVerifyPKCES256Verifier`（42 / 129 文字 / `+` / `/` / 空白 / `=`）/ `auth.TestExchangeAuthCode_VerifierMalformed`
- **2.5** — `handler.TestNativeAuthHandler_Token_InvalidJSON`（不正 JSON → 400 INVALID_REQUEST）/ `handler.TestNativeAuthHandler_Token_MissingFields`（auth_code 欠落 / code_verifier 欠落 / 両欠落 / 空文字 2 種の 5 サブケース）/ `handler.TestNativeAuthHandler_Token_UnknownFieldRejected`（DisallowUnknownFields による strict 拒否）
- **2.6** — `handler.TestNativeAuthHandler_Token_InvalidGrant`（sentinel と wrap の両ケースで 400 INVALID_GRANT を返し、message に `verifier` / `expired` / `used` を含まない）/ `handler.TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400`（message に `used` / `expired` を含めない）
- **2.7** — `auth.TestExchangeAuthCode_AuthCodeNotFound` / `_VerifierMismatch` / `_VerifierMalformed` / `_MarkUsedNotUsable` のいずれも `CreateFamily` / `CreateToken` 呼び出しが 0 回（拒否時 refresh 永続化なし）

### Requirement 3: 署名鍵管理と安全な縮退

- **3.1** — `config.TestLoad_NativeAuthJWT`「設定されているとき値を採用する」サブテスト（env 経由）/ `internal/config/config.go` `cfg.NativeAuthJWTSecret = os.Getenv("NATIVE_AUTH_JWT_SECRET")`
- **3.2** — `config.TestLoad_NativeAuthJWT`「未設定のとき空文字を保持し起動を継続する」/ `handler.TestNewRouter_NativeAuthToken_NotRegisteredWhenHandlerNil`（nil → 404）/ `internal/app/app.go` の `if cfg.NativeAuthJWTSecret != "" { ... }` 分岐
- **3.3** — `internal/app/app.go` の `slog.Warn("NATIVE_AUTH_JWT_SECRET is not set; POST /api/auth/token is disabled")`（起動経路で 1 回出力）。impl-notes.md は assert を入れていない旨を明示しており、要件「起動時に記録する」自体は実装で観察可能。
- **3.4** — `auth.TestIssueAccessToken_SuccessfulIssuance` の `newFixedIssuer`（固定 secret + 固定 `now` 注入）/ `_KidPropagation` の table-driven 3 ケース
- **3.5** — `auth.TestIssueAccessToken_SuccessfulIssuance`（header.kid 検証）/ `_KidPropagation`（v1 / v2 / 2026-06）/ `internal/auth/jwt_issuer.go` の `token.Header["kid"] = i.kid`

### Non-Functional Requirements

- **NFR 1.1** — `internal/auth/token_service.go` `generateRefreshToken`（`crypto/rand` 32 byte → `base64.RawURLEncoding`）/ `auth.TestExchangeAuthCode_Success`（TokenHash 長 64 文字 = SHA-256 hex）
- **NFR 1.2** — `auth.TestExchangeAuthCode_Success`（`storedToken.TokenHash != pair.RefreshToken`）/ `_DoesNotLeakPlainSecretsInError` / `internal/auth/token_service.go` の `slog.Info` は `refresh_token_hash[:8]` のみ
- **NFR 1.3** — `auth.TestExchangeAuthCode_DoesNotLeakPlainSecretsInError`（`secretCode` / `secretVerifier` 両方が `err.Error()` に含まれない）/ handler 側 slog.Info は固定メッセージ
- **NFR 1.4** — `internal/auth/pkce.go` `subtle.ConstantTimeCompare` / `auth.TestVerifyPKCES256Verifier` の網羅ケース
- **NFR 1.5** — `handler.TestNativeAuthHandler_Token_InternalError`（`"db connection refused"` がレスポンス message に含まれない）/ `handler.TestNativeAuthHandler_Token_InvalidGrant`（理由差別化なし）
- **NFR 2.1** — `go test ./...` 全 22 パッケージ green（既存 `createIntegrationRouter` は無変更）
- **NFR 2.2** — `config.TestLoad_NativeAuthJWT`（未設定起動成功）/ `handler.TestNewRouter_NativeAuthToken_NotRegisteredWhenHandlerNil`
- **NFR 3.1** — TokenService の 8 テストはすべて mock 駆動（DB / net 不要）/ handler の 7 テストも同様

## Findings

なし。

## 検証ログ

```
$ go build ./...
(出力なし、exit 0)

$ go vet ./...
(出力なし、exit 0)

$ go test ./...
ok  	github.com/hitoshi/feedman	(cached)
ok  	github.com/hitoshi/feedman/cmd/feedman	0.671s
ok  	github.com/hitoshi/feedman/internal/app	1.126s
ok  	github.com/hitoshi/feedman/internal/auth	(cached)
ok  	github.com/hitoshi/feedman/internal/config	1.562s
ok  	github.com/hitoshi/feedman/internal/crossfeed	(cached)
ok  	github.com/hitoshi/feedman/internal/database	(cached)
ok  	github.com/hitoshi/feedman/internal/feed	(cached)
ok  	github.com/hitoshi/feedman/internal/handler	(cached)
ok  	github.com/hitoshi/feedman/internal/hatebu	(cached)
ok  	github.com/hitoshi/feedman/internal/item	(cached)
ok  	github.com/hitoshi/feedman/internal/itemsearch	(cached)
ok  	github.com/hitoshi/feedman/internal/logger	(cached)
ok  	github.com/hitoshi/feedman/internal/metrics	(cached)
ok  	github.com/hitoshi/feedman/internal/middleware	(cached)
ok  	github.com/hitoshi/feedman/internal/model	(cached)
ok  	github.com/hitoshi/feedman/internal/repository	(cached)
ok  	github.com/hitoshi/feedman/internal/security	(cached)
ok  	github.com/hitoshi/feedman/internal/subscription	(cached)
ok  	github.com/hitoshi/feedman/internal/user	(cached)
ok  	github.com/hitoshi/feedman/internal/worker/cleanup	(cached)
ok  	github.com/hitoshi/feedman/internal/worker/fetch	(cached)
?   	github.com/hitoshi/feedman/web/node_modules/flatted/golang/pkg/flatted	[no test files]
```

DB 結合テスト（`internal/repository/*_db_test.go`）は `TEST_DATABASE_URL` 未接続時に `t.Skip` する既存設計のとおり SKIP（本 spec 追加分も同じ）。

## Summary

全 numeric AC（Req 1.1〜3.5 / NFR 1.1〜3.1）に対応する実装またはテストが揃い、design.md / tasks.md の File Structure Plan / `_Boundary:_` に沿った変更のみで構成されている。`go build` / `go vet` / `go test ./...` も全 22 パッケージ green。implの 2 つの判断（`DisallowUnknownFields` / 部分失敗 500 分類）も design 整合的。reject 理由なし。

RESULT: approve
