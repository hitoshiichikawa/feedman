# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-12T09:30:00Z -->

## Reviewed Scope

- Branch: claude/issue-169-impl-bearer-or-session
- HEAD commit: 7d5bb6a
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `internal/middleware/bearer_or_session.go` 判定フロー 3c で `VerifyAccessToken` 成功時に既存 `ContextWithUserID` で同一 context key へ userID 注入。`middleware.TestBearerOrSession_ValidBearer_InjectsUserID` / `handler.TestNewRouter_BearerAuth_IssuerVerifierRoundTrip` で担保
- 1.2 — `internal/handler/router.go` の差し替えは認証 middleware 1 行のみ・下流ハンドラ無変更。`handler.TestNewRouter_BearerAuth_ReachesAPIWithoutCookie` で `/api/subscriptions` 到達確認
- 1.3 — `bearer_or_session.go` の Bearer 成功パスは `sessionFinder` を呼ばない構造。`TestBearerOrSession_ValidBearer_InjectsUserID` で `finder.findCalls == 0` を assert
- 1.4 — 判定フロー上 Bearer 検出後は成功 or 401 の二択（Cookie 評価へ降りない実装）。裏面検証は `TestBearerOrSession_InvalidBearerWithValidCookie_Returns401`（fallback しないことで「無効 Bearer なら Cookie 側ユーザーに解決しない」= 「有効 Bearer なら Bearer 側ユーザーで処理」の対偶）
- 2.1 — `jwt_verifier.go` の `jwt.Parse` 署名検証失敗で error 返却。`auth.TestVerifyAccessToken_WrongSecret`
- 2.2 — `jwt.WithExpirationRequired` + leeway 0。`auth.TestVerifyAccessToken_Expired`（exp 1 秒超過 / 丁度の境界）/ `handler.TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401`
- 2.3 — `token_use == "access"` 厳密一致のみ受理。`auth.TestVerifyAccessToken_TokenUseMismatch`（refresh / 欠落 / 空文字の 3 ケース）
- 2.4 — `bearer_or_session.go` 判定フロー 3b: 検証失敗時 `http.Error` 即応答。`TestBearerOrSession_InvalidBearerWithValidCookie_Returns401`（sessionFinder 0 回）/ `TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401`
- 2.5 — 判定フロー 3a: token 部 trim 後空で 401。`TestBearerOrSession_SchemeAndTokenEdgeCases`（`Bearer` 単独 / `Bearer ` 空白のみ）/ `auth.TestVerifyAccessToken_MalformedTokens`（非 JWT 文字列 / 空文字）
- 2.6 — `http.Error(w, "unauthorized", http.StatusUnauthorized)` で `session.go` の 3 箇所と同一文。`TestBearerOrSession_401SameShapeAsSessionMiddleware` で実応答（status / body / Content-Type）を直接比較
- 2.7 — 全拒否パスが同一固定応答（Req 2.6 と同根拠）。理由は `slog.Warn` のみで応答に出さない
- 3.1 — 判定フロー 1: Authorization 無しで `delegate.ServeHTTP`。`TestBearerOrSession_NoAuthorizationHeader_DelegatesToSession`（有効 Cookie 200 / verifier 0 回）
- 3.2 — 判定フロー 2: `strings.EqualFold(scheme, "Bearer")` 偽で委譲。`TestBearerOrSession_SchemeAndTokenEdgeCases` Basic ケース
- 3.3 — 委譲先は `NewSessionMiddleware(sessionFinder)` の戻り値そのもの。`TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior`（有効 Cookie 200）
- 3.4 — 委譲先が既存 SessionMiddleware の戻り値そのもののため未認証応答が同一。`TestBearerOrSession_NoAuthorizationHeader_DelegatesToSession`（Cookie 無し 401）
- 4.1 — `app.go` `cfg.NativeAuthJWTSecret != ""` 分岐内で `auth.NewJWTVerifier([]byte(cfg.NativeAuthJWTSecret))` を生成（issuer と同 env 値共用）。`auth.TestVerifyAccessToken_IssuerVerifierRoundTrip` / `handler.TestNewRouter_BearerAuth_IssuerVerifierRoundTrip`
- 4.2 — `bearer_or_session.go` 判定フロー 0: `jwtVerifier == nil` で `NewSessionMiddleware(sessionFinder)` をそのまま返す。`TestBearerOrSession_NilVerifier_FallsBackToSessionMiddleware` / `TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior`
- 4.3 — 判定フロー 0 では Authorization ヘッダを一切読まない（return された SessionMiddleware は Authorization を参照しない）。`TestBearerOrSession_NilVerifier_FallsBackToSessionMiddleware`「Bearer 付き + 有効 Cookie のとき Cookie で認証成立」サブテスト
- 4.4 — `app.go` 未設定分岐は warn を出すのみで起動継続（verifier を生成しないだけ）。既存 `config.TestLoad_NativeAuthJWT` で未設定起動が担保される旨が impl-notes に明記
- 4.5 — `JWTVerifier.now func() time.Time` 非公開 field + `newFixedVerifier` ヘルパー（同パッケージから注入）。全 verifier 単体テストで使用
- NFR 1.1 — `jwt_verifier.go` の error 文字列は jwt ライブラリの error のみ（token 値を含まない）。`auth.TestVerifyAccessToken_ErrorDoesNotContainToken` で実検証。middleware 側 `slog.Warn` も `err.Error()` のみ渡す
- NFR 1.2 — `JWTVerifier` は secret と now のみで検証（DB・外部呼び出しなし）。Bearer 成功パスで `sessionFinder` 0 回（`TestBearerOrSession_ValidBearer_InjectsUserID`）
- NFR 2.1 — `session.go` 無変更（diff stat 確認）。差し替えは認証 middleware 1 箇所のみ・RateLimit / MaxBodyBytes / Logging の順序・位置・適用範囲は不変（router.go diff で確認）
- NFR 2.2 — `deps.JWTVerifier` が nil なら `NewBearerOrSessionMiddleware` が SessionMiddleware を返すため構成が従来と文字どおり同一。`TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior`
- NFR 3.1 — auth 8 テスト / middleware 6 テスト / router 4 テストすべて in-process（DB / 外部ネットワーク依存なし）。`go test ./internal/auth/ ./internal/middleware/ ./internal/handler/` で全 pass を確認

## Findings

なし（approve）。

確認した観点:

- **AC カバレッジ**: requirements.md の全 numeric ID（Req 1.1-1.4 / 2.1-2.7 / 3.1-3.4 / 4.1-4.5 / NFR 1.1-1.2 / 2.1-2.2 / 3.1）について実装または対応テストの紐付けを確認した（上記 Verified Requirements 参照）
- **missing test**: 各 AC に対応するテストが新規追加されている（`internal/auth/jwt_verifier_test.go` 8 テスト、`internal/middleware/bearer_or_session_test.go` 6 テスト、`internal/handler/router_test.go` 4 ケース追加）。Req 2.6（401 同形）は実 `SessionMiddleware` の応答と直接比較する手堅い検証になっている
- **boundary 逸脱**: design.md File Structure Plan のとおり `internal/auth/jwt_verifier.go(_test.go)` / `internal/middleware/bearer_or_session.go(_test.go)` の新規 4 ファイル + `internal/handler/router.go` / `internal/handler/router_test.go` / `internal/app/app.go` の変更 3 ファイル。`session.go` は無変更、新規 migration / config / 依存・`.env.sample` 変更なしを diff stat で確認
- **CLAUDE.md テスト規約整合**: AAA 構造で書かれ、命名が `Test<対象>_<条件>_<期待結果>` 形式（Go の `t.Run` サブテストも含む）、table-driven の使用、stub と実物（issuer ↔ verifier 通し）の使い分けが規約に整合
- **CLAUDE.md 禁止事項整合**: テストの assert 緩めは無し。`session.go` は無変更。secret はコミットされていない（テスト用固定値のみ）
- **Feature Flag Protocol**: CLAUDE.md 採否 `opt-out` のため flag 観点の確認は適用外
- **検証実行**: `go build ./...` 成功 / `go test ./internal/auth/ ./internal/middleware/ ./internal/handler/` 全 pass を実行確認

## Summary

design.md / tasks.md の指針どおりに 4 タスクが実装され、全 numeric ID（機能要件 19 件 + NFR 6 件）に紐づく実装またはテストが揃っている。境界逸脱なし、`session.go` 無変更、新規依存なし、401 同形性は実 SessionMiddleware との直接比較で担保。

RESULT: approve
