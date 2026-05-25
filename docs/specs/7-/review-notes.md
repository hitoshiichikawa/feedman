# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-7-impl-
- HEAD commit: 74ce5951a0c1f14c8a079d22eb0fed50cb64cc5b
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — ロギングをチェーンに登録: `internal/handler/router.go` の `r.Group` 2 箇所で `r.Use(logging)`。テスト `TestNewRouter_Logging_EmitsSingleAccessLogPerEndpoint`
- 1.2 — 任意エンドポイントでログ 1 件: 同テストが `len(access) != 1` を assert（二重ログ無し）
- 1.3 — `/health` でログ 1 件: 同テストのサブテスト "health endpoint"
- 1.4 — `/auth/*` でログ 1 件: 同テストのサブテスト "auth route"（`/auth/me`）
- 1.5 — `/api/*` でログ 1 件: 同テストのサブテスト "api route"（`/api/subscriptions`、有効セッション）
- 2.1 — method/path/status/duration_ms を含む: `TestNewRouter_Logging_IncludesRequestFields`
- 2.2 — 認証済みで user_id を含む: `TestNewRouter_Logging_AuthenticatedRequest_IncludesUserID`（`user_id == "user-test-1"`）
- 2.3 — 未認証で user_id 空/非出力: `TestNewRouter_Logging_UnauthenticatedRequest_OmitsUserID` + `TestNewRouter_Logging_ProtectedRoute_NoSession_NoUserIDLog`
- 2.4 — 5xx 時にログ status が実 status と一致: `TestNewRouter_Logging_5xxStatusMatchesResponse`（503）
- 3.1 — 既存ミドルウェア相対順序の保持: 最上位 Recovery→SecurityHeaders→CORS 不変、`/api/*` の Session→RateLimit は既存のまま、Logging を末尾に追加。既存テスト群 green
- 3.2 — セッション無しの 401 保持: 既存 `TestNewRouter_ProtectedRoute_NoSession_Returns401` green
- 3.3 — セッション有りの成功応答保持: 既存 `TestNewRouter_ProtectedRoute_WithSession_GET_Succeeds` green
- 3.4 — レート制限挙動の維持: `internal/middleware` の既存 RateLimiter テスト群 green
- 3.5 — 全エンドポイントのルーティング保持: 既存 `TestNewRouter_*Routes_AllEndpoints` 全件 green
- NFR 1.1 — 同期ログ 1 件・追加 I/O なし: `NewLoggingMiddleware` のロジック不変（スコープ外）、ログ count == 1 で担保
- NFR 2.1 — 変更前に成功したリクエストの status/body 不変: 既存 `router_full_test.go` / `integration_test.go` green
- NFR 3.1 — アプリ標準ログ出力と同一経路: `internal/app/app.go` で `Logger: slog.Default()` 注入、`TestNewRouter_Logging_NilLogger_FallsBackToDefault` でフォールバック検証

## Findings

なし

## Summary

全 numeric ID（Req 1.1〜1.5 / 2.1〜2.4 / 3.1〜3.5 / NFR 1.1, 2.1, 3.1）に対応する実装とテストを確認した。変更は `internal/handler/router.go` / `internal/app/app.go` / 新規 `internal/handler/router_logging_test.go` に限定され、`NewLoggingMiddleware` 本体は不変（Out of Scope 遵守）で boundary 逸脱なし。対象パッケージの `go test` は green。`/api/*` 未認証（401）時に Session で中断されログが出ない挙動は requirements.md の Open Questions を人間判断で確定した design 上のトレードオフであり impl-notes に明記済みで、AC 未カバー・missing test には当たらない（informational）。Feature Flag Protocol は opt-out のため flag 観点は適用せず。

RESULT: approve
