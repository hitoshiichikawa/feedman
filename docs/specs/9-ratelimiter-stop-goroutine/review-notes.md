# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-9-impl-ratelimiter-stop-goroutine
- HEAD commit: 58344173ebec7599525fb1cb01cbb40418168359
- Compared to: develop..HEAD

本 Issue は design-less impl（`design.md` / `tasks.md` なし）であり、`_Boundary:_`
アノテーションによる境界制約は存在しない。Feature Flag Protocol は CLAUDE.md で
`opt-out` 宣言のため、flag 観点の確認は行わず通常の 3 カテゴリ判定のみを適用した。

## Verified Requirements

- 1.1 — `internal/app/app.go` の `runServe` シャットダウン経路で `newShutdownCoordinator(server, rateLimiter)` → `coordinator.shutdown(ctx)` を経由し `internal/app/shutdown.go` の `stopRateLimiter()` が `rateLimiter.Stop()` を呼ぶ。`TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine` で実行確認
- 1.2 — `RateLimiter.Stop()`（`internal/middleware/ratelimit.go`）本体は無変更でクリーンアップ goroutine は稼働中継続。既存 `internal/middleware/ratelimit_test.go` が無変更で pass
- 1.3 — `internal/app/shutdown_test.go: TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine` が `runtime.NumGoroutine()` の起動前水準への収束（`waitGoroutineCount`）でリーク非発生を観察検証
- 1.4 — `shutdown.go` の `stopOnce sync.Once` で `Stop()` を高々 1 回に保証。`TestShutdownCoordinator_DoubleInvocationDoesNotPanic` で二重 close panic 非発生を確認
- 2.1 — `internal/app/shutdown_test.go: TestShutdownCoordinator_DoesNotPanic` で shutdown 実行時の panic 非発生を検証
- 2.2 — `TestShutdownCoordinator_DoubleInvocationDoesNotPanic` で重複起動シナリオ（shutdown 2 回）の panic 非発生を検証
- 2.3 — `func (rl *RateLimiter) Stop()`（引数・戻り値）は `internal/middleware/ratelimit.go` で無変更を確認（diff に同ファイルの変更なし）
- 2.4 — `Stop()` 本体無変更。panic 防止は app 層の `sync.Once` に閉じており、既存ミドルウェア・既存テストの挙動は不変（middleware テスト無変更 pass）
- 3.1 — `Stop()` 呼び出しは shutdown 経路（`stop` シグナル受信後）のみで、通常稼働中は呼ばれない構造
- 3.2 — `shutdown.go` の `shutdown` メソッドが `server.Shutdown(ctx)`（drain 待機）完了後に `stopRateLimiter()` を呼ぶ順序を固定
- 4.1 — `runServe`（RateLimiter 構築モード）が coordinator 経由で停止
- 4.2 — `runWorker` は RateLimiter を構築・参照せず（diff・grep で確認）、coordinator も呼ばないため停止処理が走らない
- NFR 1.1 — goroutine 数比較（`runtime.NumGoroutine()` + `waitGoroutineCount` ポーリング）による観察可能な検証手段を提供
- NFR 1.2 — `TestShutdownCoordinator_DoubleInvocationDoesNotPanic` が二重起動での panic 非発生を検証
- NFR 2.1 / 2.2 — レート制限応答挙動は不変。既存 `internal/middleware` / `internal/handler` テストが無変更で pass

## Findings

なし

## Summary

全 AC（1.1〜4.2）および NFR が、新規 `internal/app/shutdown.go` の実装と
`internal/app/shutdown_test.go` の 3 テストケースで観察可能にカバーされている。
`RateLimiter.Stop()` 本体は無変更でシグネチャ・既存挙動を保持し、panic 防止は app 層の
`sync.Once` に閉じている。`go test -race -count=3 -run TestShutdownCoordinator ./internal/app/...`
および middleware テストの green を再実行で確認した。AC 未カバー / missing test /
boundary 逸脱のいずれも検出されなかった。

RESULT: approve
