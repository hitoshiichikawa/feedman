# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-38-impl--auth-google-health-ip
- HEAD commit: 0989d1aab72ed674c9f3ff4de2a7da1c1106577a
- Compared to: develop..HEAD

備考: 本 spec ディレクトリには `tasks.md` / `design.md` が存在しない（design-less impl）。
そのため `_Boundary:_` アノテーションが無く、boundary 逸脱の判定は requirements.md と
既存コードの突き合わせで行った。Feature Flag Protocol は CLAUDE.md で `opt-out` 宣言のため
flag 観点の確認は行わない（既存挙動保持）。

## Verified Requirements

- 1.1 — `internal/middleware/ip_ratelimit.go` `Middleware()` + `internal/handler/router.go` の `r.With(unauthIPMW).Get("/auth/google/login", ...)`。テスト `TestIPRateLimiter_Returns429WhenLimitExceeded_PerEndpoint/auth_google_login`、`TestNewRouter_UnauthIPRateLimit_429OnExcess/login`
- 1.2 — 同上（callback ルート）。テスト `..._PerEndpoint/auth_google_callback`、`TestNewRouter_UnauthIPRateLimit_429OnExcess/callback`
- 1.3 — 同上（`r.With(unauthIPMW).Get("/health", healthHandler)`）。テスト `..._PerEndpoint/health`、`TestNewRouter_UnauthIPRateLimit_429OnExcess/health`
- 1.4 — `Middleware()` の `limiter.Allow()` 成功時に `next.ServeHTTP`。テスト `TestIPRateLimiter_AllowsRequestsWithinLimit`（burst 内 5 件すべて 200 + handler 5 回呼出）
- 1.5 — `getOrCreateLimiter(key)` が IP をキーに独立バケットを生成。テスト `TestIPRateLimiter_IsolatesRateLimitsPerIP`、`TestNewRouter_UnauthIPRateLimit_IsolatesPerIP`
- 1.6 — 既存 `writeRateLimitResponse`（`internal/middleware/ratelimit.go:248`）を再利用し `Retry-After` ヘッダーを付与。テスト `TestIPRateLimiter_Returns429WithRetryAfterHeader`（数値かつ >=1 を検証）
- 2.1 — `internal/config/config.go` `getEnvInt("RATE_LIMIT_UNAUTH_IP", 30)`。テスト `TestLoad_DefaultValues`（RateLimitUnauthIP=30）、`TestDefaultIPRateLimiterConfig`
- 2.2 — 同上（env 指定時は指定値）。テスト `TestLoad_CustomValues`（RATE_LIMIT_UNAUTH_IP=15）
- 2.3 — 既存 `getEnvInt` がパース失敗時に Warn ログ + 既定フォールバック（`config.go:192-200` で確認）。テスト `TestLoad_InvalidRateLimitUnauthIP_FallsBackToDefault`（"not-a-number" → 30、エラーなし）
- 3.1 — `internal/middleware/clientip.go` `clientIPFromRemoteAddr` が `r.RemoteAddr`（接続元アドレス）のみから IP を判定。テスト `TestIPRateLimiter_IgnoresXForwardedFor`（RemoteAddr ベース動作）
- 3.2 — `clientIPFromRemoteAddr` は X-Forwarded-For を一切参照しない（コードで確認）。テスト `TestIPRateLimiter_IgnoresXForwardedFor`（XFF を別 IP に偽装しても RemoteAddr 同一なら 429）
- 3.3 — IP 判定不能時は固定キー `unknownIPKey` で制限し無制限通過を許さない。テスト `TestIPRateLimiter_IndeterminateIP_NotUnlimited`（"not-an-address" の 2 回目が 429）
- 4.1 — 既存 `RateLimiter` 型は無改変。新規 `IPRateLimiter` を別型として追加。既存 `internal/middleware/ratelimit_test.go` 全通過（`go test ./internal/middleware/...` ok）
- 4.2 — `/auth/google/login`・`/auth/google/callback`・`/health` の 3 ルートにのみ `r.With(unauthIPMW)` を適用し、`/api/*` は従来の userID 単位 RateLimiter のまま。テスト `TestNewRouter_UnauthIPRateLimit_NotAppliedToLogoutAndMe`
- 4.3 — IP 制限は IP キー、userID 制限は既存実装のまま分離。既存 ratelimit テスト + handler テスト全通過
- NFR 1.1 — `slog.Warn("rate limit exceeded", slog.String("limit_type", "unauth_ip"))` で拒否事象を運用ログに記録（`ip_ratelimit.go` で確認）
- NFR 1.2 — 拒否ログ属性は `limit_type` のみで、セッション情報・トークンを含めない（コードで確認）
- NFR 2.1 — `app.go` で既定 30 を用い設定追加なしに wiring。テスト `TestLoad_DefaultValues`
- NFR 2.2 — `UnauthIPRateLimiter` が nil の場合は素通し（`unauthIPMW = func(next) { return next }`）。`r.With(...)` 個別適用で既存ミドルウェア順序・ルーティング不変。テスト `TestNewRouter_UnauthIPRateLimit_NilLimiter_NoRestriction`
- NFR 3.1 — `cleanupLoop`/`cleanup`（TTL = CleanupInterval×2）で期限切れ IP エントリを解放。`shutdownCoordinator` で `Stop()` を `sync.Once` 保護下に呼出。テスト `TestIPRateLimiter_CleanupRemovesExpiredEntries`、`TestShutdownCoordinator_StopsIPRateLimiterCleanupGoroutine`、`TestShutdownCoordinator_StopsBothLimiters`

## Findings

なし

## 補足（判定理由に含めない参考事項）

- impl-notes.md (b) の対応表は実装・テスト・コードと一致しており、既存テストでの偶然カバー分も
  含めて AC 紐付けが明示されている（missing test 判定の根拠を満たす）。
- `gofmt -l` が報告する未整形ファイルは impl-notes.md (補足) のとおり本 Issue 着手前から存在する
  baseline であり、本変更追加分は整形済み。スタイル観点のため判定理由にはしない。

## Summary

requirements.md の全 numeric ID（1.1〜1.6 / 2.1〜2.3 / 3.1〜3.3 / 4.1〜4.3 / NFR 1.1〜3.1）が
実装と対応テストでカバーされ、新規挙動には対応テストが追加されている。既存 `RateLimiter` は
無改変で userID 単位の後方互換が維持され、対象 4 パッケージのテストはすべて green。AC 未カバー /
missing test / boundary 逸脱のいずれも検出されなかった。

RESULT: approve
