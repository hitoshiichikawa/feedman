# Implementation Plan

- [x] 1. config: JWT 署名鍵の環境変数を追加
  - `internal/config/config.go` に `NativeAuthJWTSecret`（env `NATIVE_AUTH_JWT_SECRET`、任意）と
    `NativeAuthJWTKid`（env `NATIVE_AUTH_JWT_KID`、既定 `"v1"`）を追加。未設定でも起動を
    失敗させない（既存 required 項目の検証ロジックに含めない）
  - `.env.sample` に両変数とコメント（未設定なら native token 交換が無効になる旨）を追記
  - 既存 config テストの慣習に合わせ、設定あり / なし / kid 既定値の unit test を追加
  - _Requirements: 3.1, 3.2, NFR 2.2_

- [ ] 2. auth: PKCE verifier 検証を追加
  - `internal/auth/pkce.go` に `VerifyPKCES256Verifier(verifier, storedChallenge string) bool` を
    追加（形式 `^[A-Za-z0-9._~-]{43,128}$` → S256 導出 → `subtle.ConstantTimeCompare`）
  - `internal/auth/pkce_test.go` に RFC 7636 Appendix B の test vector を含む table-driven
    ケース（一致 / 不一致 / 42・129 文字 / 不正文字）を追加
  - _Requirements: 2.1, 2.4, NFR 1.4_

- [ ] 3. auth: JWTIssuer を追加
  - `go.mod` に `github.com/golang-jwt/jwt/v5` を追加
  - `internal/auth/jwt_issuer.go` を新規作成: `AccessTokenTTL = 15 * time.Minute` /
    `NewJWTIssuer(secret []byte, kid string)` / `IssueAccessToken(userID string) (string, error)`
    （claims: sub / exp / iat / jti / token_use="access"、header kid、now はテスト注入可能な
    非公開 field）
  - `internal/auth/jwt_issuer_test.go` を新規作成（design.md Testing Strategy 2: parse して
    claims / kid / 期限を検証、空 userID は error）
  - _Requirements: 1.4, 3.4, 3.5_

- [ ] 4. auth: TokenService.ExchangeAuthCode を追加
  - `internal/auth/token_service.go` を新規作成: `ErrInvalidGrant` sentinel /
    `AuthCodeConsumer`・`RefreshTokenStore` 最小 interface / `RefreshTokenTTL = 30 * 24 * time.Hour` /
    `TokenPair` / `NewTokenService` / `ExchangeAuthCode`（design.md の交換フロー手順 2〜6。
    拒否はすべて `ErrInvalidGrant` に正規化し、拒否時は refresh を永続化しない）
  - refresh token は crypto/rand 32 byte → base64.RawURLEncoding、保存は
    `HashNativeSecret`（#165）の hash のみ。ログに平文を出さない
  - `internal/auth/token_service_test.go` を新規作成（design.md Testing Strategy 3 の 6 ケース）
  - _Requirements: 1.2, 1.3, 2.1, 2.2, 2.3, 2.6, 2.7, NFR 1.1, NFR 1.2, NFR 1.3, NFR 3.1_
  - _Depends: 2, 3_

- [ ] 5. handler: NativeAuthHandler と router 登録
  - `internal/handler/native_auth_handler.go` を新規作成: `TokenExchangeService` 最小 IF /
    `NewNativeAuthHandler` / `Token`（JSON decode → 必須フィールド検査 → service 呼び出し →
    200 / 400 INVALID_REQUEST / 400 INVALID_GRANT / 500 を `middleware.WriteErrorResponse` で応答。
    レスポンスフィールドは access_token / refresh_token / token_type / expires_in）
  - `internal/handler/router.go`: `RouterDeps.NativeAuthHandler`（任意）を追加し、認証不要
    グループに `r.With(middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).Post("/api/auth/token", ...)` を
    nil ガード付きで登録（nil なら未登録 = 404 の fail-closed）
  - `internal/handler/native_auth_handler_test.go` を新規作成（design.md Testing Strategy 4）、
    `internal/handler/router_test.go` に注入時 / nil 時のルーティングケースを追加
  - _Requirements: 1.1, 1.5, 1.6, 2.5, 2.6, 3.2, NFR 1.5, NFR 2.1_
  - _Depends: 4_

- [ ] 6. wiring と統合テスト
  - `internal/app/app.go`: `repository.NewPostgresRefreshTokenRepo(db)` を wiring し、
    `cfg.NativeAuthJWTSecret != ""` のときのみ issuer / TokenService / NativeAuthHandler を
    生成して deps に注入。未設定時は `slog.Warn` を 1 回出力（design.md app.go 節どおり）
  - `internal/handler/integration_test.go` に native login → callback → token 交換成功 →
    同一 code 再交換 400 の通しケースを追加
  - 既存ルートのルーティング・既存統合テストが無変更で green であることを確認
  - _Requirements: 1.1, 1.2, 3.2, 3.3, NFR 2.1, NFR 2.2, NFR 3.1_
  - _Depends: 5_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。DB 結合テストは `TEST_DATABASE_URL` 未接続時に `t.Skip` され、
unit テスト + 静的解析のみで green 判定される（CI でも同条件）。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
