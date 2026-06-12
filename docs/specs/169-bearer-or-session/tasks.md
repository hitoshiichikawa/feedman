# Implementation Plan

- [x] 1. auth: JWTVerifier を追加
  - `internal/auth/jwt_verifier.go` を新規作成: `NewJWTVerifier(secret []byte)` /
    `VerifyAccessToken(tokenString string) (string, error)`（design.md 検証規則表どおり:
    `jwt.WithValidMethods` で HS256 限定・`jwt.WithExpirationRequired` で exp 必須・
    `token_use=="access"` 厳密一致・`sub` 非空。now はテスト注入可能な非公開 field で
    `jwt.WithTimeFunc` に渡す。jti / kid は参照しない）
  - `internal/auth/jwt_verifier_test.go` を新規作成（design.md Testing Strategy auth 1〜6:
    #166 JWTIssuer と固定 secret / 固定 now を共有し、有効 / 期限切れ / 署名不正 / 用途不一致 /
    alg 偽装 / sub 空・不正文字列を table-driven で検証）
  - _Requirements: 2.1, 2.2, 2.3, 4.1, 4.5, NFR 1.2, NFR 3.1_

- [x] 2. middleware: BearerOrSession 複合認証を追加
  - `internal/middleware/bearer_or_session.go` を新規作成: `JWTVerifier` 最小 IF +
    `NewBearerOrSessionMiddleware(jwtVerifier, sessionFinder)`（design.md 判定フロー 0〜3:
    verifier nil なら `NewSessionMiddleware(sessionFinder)` をそのまま返す縮退 / Bearer scheme
    （`strings.EqualFold` で大文字小文字不区別）検出 → 検証成功で `ContextWithUserID` 注入 /
    失敗・token 部空は `http.Error(w, "unauthorized", http.StatusUnauthorized)` で既存と同形の
    401 即応答（Cookie fallback なし）/ Bearer 無し・他 scheme は SessionMiddleware へ委譲）。
    既存 `session.go` は変更しない
  - ログは `slog.Warn` に検証エラー理由のみ渡す（token 文字列・claims 値を渡さない）
  - `internal/middleware/bearer_or_session_test.go` を新規作成（design.md Testing Strategy
    middleware 1〜6: stub verifier + stub SessionFinder で分岐・401 同形・委譲・nil 縮退を検証）
  - _Requirements: 1.1, 1.3, 1.4, 2.4, 2.5, 2.6, 2.7, 3.1, 3.2, 4.2, 4.3, NFR 1.1_
  - _Depends: 1_

- [ ] 3. router / app: 認証必須グループの差し替えと wiring
  - `internal/handler/router.go`: `RouterDeps.JWTVerifier middleware.JWTVerifier`（任意・nil 可）を
    追加し、認証必須グループの `middleware.NewSessionMiddleware(deps.SessionFinder)` 行を
    `middleware.NewBearerOrSessionMiddleware(deps.JWTVerifier, deps.SessionFinder)` に 1 行
    差し替える（RateLimit / MaxBodyBytes / Logging の順序・位置・適用範囲は不変）
  - `internal/app/app.go`: #166 の `cfg.NativeAuthJWTSecret != ""` 分岐内に
    `deps.JWTVerifier = auth.NewJWTVerifier([]byte(cfg.NativeAuthJWTSecret))` を追記し、
    未設定時の warn 文言を Bearer 認証無効も含む内容に更新する（warn は 1 回のまま・起動は成功）
  - `internal/handler/router_test.go` に JWTVerifier 注入時の Bearer 到達（stub verifier・
    Cookie 無し）/ nil 時の従来挙動（Cookie で従来どおり・未認証 401 同形）のケースを追加
  - _Requirements: 1.2, 3.3, 3.4, 4.2, 4.4, NFR 2.1, NFR 2.2_
  - _Depends: 2_

- [ ] 4. 統合確認: 発行 ↔ 検証の通しと既存回帰
  - `internal/handler/router_test.go` に、#166 `auth.JWTIssuer` で発行した token を
    `auth.JWTVerifier` 注入済み `NewRouter` へ Bearer 提示し、Cookie 無しで既存 API ルートの
    認証が成立する通しケース（同一 secret・固定 now）を追加
  - 期限切れ token + 有効 Cookie 併送で 401 になる（fallback しない）ケースを追加
  - 既存の Cookie 系テスト（`internal/middleware/session_test.go` /
    `internal/middleware/router_integration_test.go` 等）が無変更で green であることを確認
  - _Requirements: 1.1, 1.2, 2.2, 2.4, NFR 2.1, NFR 3.1_
  - _Depends: 3_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。DB 結合テストは `TEST_DATABASE_URL` 未接続時に `t.Skip` され、
unit テスト + 静的解析のみで green 判定される（CI でも同条件）。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
