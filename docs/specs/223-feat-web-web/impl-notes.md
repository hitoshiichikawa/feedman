# 実装ノート（#223）

Web にパスキー導線（新規作成 / ログイン）を追加し、いずれの導線でも auth_code → Cookie
session の合流経路を通じて既存の 2 ペイン UI 認証状態に到達させる spec の実装ノートを
task 単位で記録する。前方伝播（先行 task の learning を後続 task で温存）を規律とする。

## Implementation Notes

### Task 1

- **採用方針**: `SessionExchangeService` は `TokenService.ExchangeAuthCode` の前半 3 段
  （`HashNativeSecret` → `AuthCodeConsumer.FindByHash` → `VerifyPKCES256Verifier` →
  `AuthCodeConsumer.MarkUsed`）を再利用し、以降のみ session 発行（`generateSessionID`
  → `SessionCreator.Create`）に差し替える。
- **重要な判断**:
  - `AuthCodeConsumer` は既存 `internal/auth/token_service.go` の interface をそのまま
    流用（新規 interface を作らない / CLAUDE.md §4 コピペ禁止）。`SessionCreator` のみ
    新規に `Create` 1 メソッドの狭い interface として追加した（interface segregation /
    CLAUDE.md §5）。`repository.SessionRepository` が構造的に充足する。
  - session ID 生成は `internal/auth/service.go` の既存 `generateSessionID`（32 バイト
    crypto random / hex）を直接呼び、Google OAuth callback と同じ 32 バイト crypto random
    /hex 方針を維持する。ロジックの複製は行わない。
  - now は `func() time.Time` フィールドで抽象化し `NewSessionExchangeService` で
    `time.Now` を注入。テストは同 package から `svc.now` を上書きして時刻を固定する
    （既存 `TokenService` と同じ idiom）。
  - 拒否は `ErrInvalidGrant` に uniform 化、infra error は `fmt.Errorf("session
    exchange: ...: %w", err)` で wrap して handler が 500 に振り分けられる形にする。
    平文 auth_code / code_verifier / session_id はログ・エラーに出さず、追跡ログは
    `code_hash[:8]` / `session_id_hash[:8]` のみ出力する（既存 `TokenService.ExchangeAuthCode`
    と同方針 / NFR 1.1）。
- **残存課題**: task 2 で `NewSessionExchangeService(authCodeRepo, sessionRepo,
  sessionTTL)` を `internal/app/app.go` で wiring し、`NativeAuthHandler` に注入する
  必要がある。`sessionTTL` は既存 `SessionMaxAge`（秒 int）を `time.Duration` 化して
  渡す（`time.Duration(cfg.SessionMaxAge) * time.Second`）。handler 層で
  `errors.Is(err, auth.ErrInvalidGrant)` により 400 INVALID_GRANT へ、その他 error は
  500 INTERNAL_ERROR へ振り分ける（design.md §Error Handling / task 2 詳細）。

### Task 2

- **採用方針**: `NativeAuthHandler.Session` を既存 `Token` / `Refresh` / `Revoke` と
  同 idiom（`dec.DisallowUnknownFields()` + `invalidRequestError` /
  `invalidGrantError` / `WriteInternalServerError` 再利用）で実装し、成功時のみ
  既存 `sessionCookieName` 定数と OAuth Callback と同一属性で Set-Cookie する。
- **重要な判断**:
  - **Cookie 属性の特定と再利用**: 既存 Google OAuth callback の Cookie 発行箇所は
    `grep 'http.SetCookie'` および `sessionCookieName` の検索で `internal/handler/auth_handler.go`
    の `AuthHandler.Callback` 手順 5（line 193-202）と特定。既存定数
    `sessionCookieName` = `"session_id"`、`http.SameSiteLaxMode` をそのまま参照し、
    Domain / Secure / MaxAge は `AuthHandler` と同じ `cfg.CookieDomain` /
    `cfg.CookieSecure` / `cfg.SessionMaxAge` を `WithSessionExchange` 経由で注入する。
    Path=`"/"` / HttpOnly=`true` は既存 Cookie と同じくハードコード。実装差分は
    `TestNativeAuthHandler_Session_Success` で Domain / MaxAge / HttpOnly / Secure /
    SameSite の全 5 属性を assert して回帰防止済み。
  - **handler が service を受ける interface**: design.md では `sessionExchange
    *auth.SessionExchangeService`（具体型）を想定していたが、既存 `TokenExchangeService`
    と同 idiom で `SessionExchanger` narrow interface（`ExchangeAuthCodeForSession` 1
    メソッドのみ）を handler package に追加し、`*auth.SessionExchangeService` を構造的に
    充足させた（interface segregation / CLAUDE.md §5 / testability 優先）。design.md の
    「Cookie 属性の完全一致」「fail-closed の連動」「NFR 2.1 の既存挙動不変」の 3 契約は
    interface 化によって毀損されない（handler の依存が抽象化されるだけで挙動は不変）。
  - **既存 constructor の後方互換**: `NewNativeAuthHandler` は functional option
    （`NativeAuthHandlerOption` + `WithSessionExchange`）で拡張。既存呼び出し
    `NewNativeAuthHandler(svc)` は variadic のため無変更で compile 通過（NFR 2.1）。
    既存 `native_auth_handler_test.go` / `router_test.go` / `integration_test.go` /
    `native_auth_e2e_db_test.go` / `passkey_e2e_db_test.go` の 15 callsites を編集
    せずに済み、既存 Token / Refresh / Revoke tests が完全不変を維持することを
    `go test ./...` all-green で確認済み。この functional option 採用は既存
    `item.WithMetrics` / `fetchpkg.WithMetrics` と同 idiom で codebase 一貫性も維持。
  - **fail-closed の連動**: 本 route の登録は `deps.NativeAuthHandler != nil` gate に
    含めることで、既存 3 route（token/refresh/revoke）と連動して
    `NATIVE_AUTH_JWT_SECRET` 未設定時に 404 で縮退する（NFR 2.2）。
    `TestNewRouter_NativeAuthSession_NotRegisteredWhenHandlerNil` で確認。
- **残存課題**: なし。task 3（`GET /api/passkey/capability` handler）は Boundary
  `PasskeyHandler, Router` で独立に着手可能。

## AC トレース

Task 1 で担保した AC は以下:

- **3.1（新規作成後のセッション合流）** / **4.2（ログイン成功後のセッション合流）**:
  `ExchangeAuthCodeForSession` の 5 段（find → verify → markUsed → generate → create）
  および正常系テスト `TestExchangeAuthCodeForSession_Success` で担保。拒否時の
  session 非永続化は `TestExchangeAuthCodeForSession_AuthCodeNotFound` /
  `_VerifierMismatch` / `_MarkUsedNotUsable` で担保。
- **NFR 1.1（機密情報の非漏出）**:
  `TestExchangeAuthCodeForSession_DoesNotLeakPlainSecretsInError` が ErrInvalidGrant /
  infra wrap の両経路で error メッセージに平文 authCode / codeVerifier が含まれない
  ことを検証。実装側は追跡ログを `code_hash[:8]` / `session_id_hash[:8]` に留めており、
  平文値を slog にも出さない（既存 `TokenService` と同方針）。

Task 2 で担保した AC は以下:

- **3.1（新規作成後のセッション合流）** / **4.2（ログイン成功後のセッション合流）**:
  `NativeAuthHandler.Session` の 204 応答 + Set-Cookie 発行を
  `TestNativeAuthHandler_Session_Success` および
  `TestNewRouter_NativeAuthSession_RegisteredWhenHandlerInjected` で担保。
  Cookie 属性は既存 Google OAuth Callback（`AuthHandler.Callback`）と同一
  （Name=`session_id` / Path=`/` / Domain=`CookieDomain` / MaxAge=`SessionMaxAge` /
  HttpOnly / Secure / SameSite=Lax）で、Web は追加操作なしに Cookie セッション認証状態に
  到達する。
- **3.4（合流失敗時にセッションに到達させない）** / **4.7（サーバエラー時に認証状態に
  到達させない）**: 400 INVALID_GRANT を `TestNativeAuthHandler_Session_InvalidGrant`、
  500 INTERNAL_ERROR を `TestNativeAuthHandler_Session_InternalError` で担保。両者とも
  Set-Cookie 未発行を assert し、handler 経路で session 発行が起きないことを検証。
- **NFR 1.1（機密情報の非漏出）**: 応答 message に内部詳細
  （`db connection refused` / `verifier` / `expired` / `mismatch` 等の拒否理由語彙）が
  反射されないことを `TestNativeAuthHandler_Session_InvalidGrant` /
  `_InternalError` の両テストで assert。handler は slog に平文 authCode /
  codeVerifier / sessionID を出さない実装（追跡は service 層の hash log に一元化）。
- **NFR 2.1（既存挙動不変）**: 既存 `Token` / `Refresh` / `Revoke` methods は完全不変
  （signature も含めて未編集）で、`go test ./...` all-green を確認。
  `NewNativeAuthHandler` の signature 拡張は variadic option 採用により既存 15
  callsites の書き換えを不要にした。route 登録は既存 3 route と同じ
  `NativeAuthHandler != nil` gate と `unauthIPMW` + `MaxBodyBytes` 順序で行い、
  route 順序も末尾追加のみ。
- **NFR 2.2（縮退時の既存挙動維持）**: `NATIVE_AUTH_JWT_SECRET` 未設定 →
  `NativeAuthHandler = nil` → `/api/auth/session` route 未登録 → 404 を
  `TestNewRouter_NativeAuthSession_NotRegisteredWhenHandlerNil` で担保。
  `TestNewRouter_NativeAuthSessionIPRateLimit_429OnExcess` および
  `_SameShapeAsExistingRoutes` で `unauthIPMW` の閾値超過時の 429 応答が既存
  `/health` 429 応答と body / header レベルで同一形式であることを assert。

## 確認事項

- Task 1 時点: 現時点でなし。design.md § SessionExchangeService の Contracts と実装は一致。
- Task 2 追記: design.md §NativeAuthHandler.Session の struct field 例示
  （`sessionExchange *auth.SessionExchangeService` の具体型）に対し、実装は
  `SessionExchanger` narrow interface を追加してそこに `*auth.SessionExchangeService`
  を構造的に充足させる形にした（interface segregation / CLAUDE.md §5 / 既存
  `TokenExchangeService` と同 idiom / testability 向上）。API 契約
  （POST /api/auth/session / Cookie 属性 / status code / error code）および
  fail-closed の連動は不変。人間 Reviewer による design.md との整合性確認を推奨。
