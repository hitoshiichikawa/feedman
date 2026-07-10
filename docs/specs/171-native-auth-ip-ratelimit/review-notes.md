# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-12T09:20:00Z -->

## Reviewed Scope

- Branch: claude/issue-171-impl-native-auth-ip-ratelimit
- HEAD commit: 38b03e47712b9e633dbbd638acf1b1741a7b08a2
- Compared to: develop..HEAD
- 変更ファイル（実装範囲）: `internal/handler/router.go` / `internal/handler/router_test.go`
  （design.md File Structure Plan と完全一致）
- 変更ファイル（spec / 進捗）: `docs/specs/171-native-auth-ip-ratelimit/impl-notes.md` /
  `docs/specs/171-native-auth-ip-ratelimit/tasks.md`（task 1, 2 の `- [x]` 進捗 mark）

## Verified Requirements

- 1.1 — `TestNewRouter_NativeAuthIPRateLimit_429OnExcess` の `token` サブケース。
  burst=1 注入で 1 回目 200 → 2 回目 429、`callCount` が 1 のままで service 到達なしを確認
  （router.go で `r.With(unauthIPMW, MaxBodyBytes).Post("/api/auth/token", ...)` の最外側 unauthIPMW 適用）
- 1.2 — 同テストの `refresh` サブケース。`rotateCalls` が 1 のままで 2 回目 429
- 1.3 — 同テストの `revoke` サブケース。`revokeCalls` が 1 のままで 2 回目 429
- 1.4 — 同テスト各サブケースの 1 回目応答（200/204 + service 到達 1 回）+
  `_Degradations` の "UnauthIPRateLimiter nil で 5 連続通過" サブケース
- 1.5 — `_429OnExcess` で `Retry-After` ヘッダー非空を検証 + `_SameShapeAsExistingRoutes` で
  /health 429 の Retry-After と同形式を確認
- 1.6 — `TestNewRouter_NativeAuthIPRateLimit_IndependentPerIP`。IP A 枯渇後に
  IP B(`198.51.100.20`) からの要求が 200 で通過し、`callCount == 2`（IP 別バケット）
- 2.1 — 既存未認証 3 ルート（`/health` / `/auth/google/login` / `/auth/google/callback`）の
  ルート登録に diff ゼロ、既存 `router_unauth_ratelimit_test.go` 系も無変更 green
  （`go test ./internal/handler/ ./internal/middleware/` 全 pass）
- 2.2 — 認証必須グループ（Session → RateLimit(General) → Logging）に diff ゼロ。
  middleware 種類・順序は無変更
- 2.3 — `TestNewRouter_NativeAuthIPRateLimit_SameShapeAsExistingRoutes`。
  /health 429 と /api/auth/token 429 を status / Content-Type / body / Retry-After で
  直接比較し同一形式を確認
- 2.4 — `_Degradations` の "NativeAuthHandler nil のとき 3 ルートは 404 のまま"
  サブケース（limiter ありでも 3 ルートとも 404 = 本変更が no-op）
- NFR 1.1 — 既存 `IPRateLimiter` の `slog.Warn("rate limit exceeded", limit_type=unauth_ip)`
  をそのまま共用。テスト実行ログにも `WARN rate limit exceeded limit_type=unauth_ip` が
  3 ルートで出力されることを確認
- NFR 1.2 — 同上（#38 設計のまま、limit 種別のみで PII なし）
- NFR 2.1 — config / app.go / `RouterDeps` に diff なし。env 未指定で既定 30 req/min/IP が
  そのまま native auth 3 ルートにも適用される（既存 wiring に同乗）
- NFR 2.2 — 既存ルートのチェーンに変更なし、native auth ルートへの route 単位 `With` 追加のみ
  （design.md の文言と一致）
- NFR 3.1 — 追加テスト 4 本すべて httptest + モック service（`alwaysSucceedExchangeService`）+
  小 burst の `IPRateLimiter` 注入で in-process 完結（外部ネットワーク・DB 依存なし）

## Findings

なし（approve のため Findings は記載なし。確認した観点は「Verified Requirements」を参照）。

## Summary

design.md の Components and Interfaces（router 3 ルートへの `unauthIPMW` 追加）と File
Structure Plan（router.go + router_test.go のみ変更）に厳密に従った最小実装。
requirements.md の全 numeric ID（Req 1.1〜1.6, 2.1〜2.4, NFR 1.1, 1.2, 2.1, 2.2, 3.1）が
追加テスト 4 本（`_429OnExcess` table-driven / `_IndependentPerIP` /
`_SameShapeAsExistingRoutes` / `_Degradations`）と既存テスト無変更 green で担保されている。
boundary 逸脱なし、`go test ./internal/handler/ ./internal/middleware/` 全 pass。

RESULT: approve
