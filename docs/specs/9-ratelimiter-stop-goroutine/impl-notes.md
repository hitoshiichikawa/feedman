# 実装ノート: RateLimiter.Stop() をシャットダウン時に呼び出して goroutine リークを防止

Issue #9 / design-less impl（design.md / tasks.md なし）。

## 採用した panic 防止方針とトレードオフ

要件には 2 つの制約の両立が求められた:

- **AC 2.1 / 2.2 / NFR 1.2**: シャットダウン経路（および重複起動シナリオ）で panic を発生させない
- **AC 2.3 / 2.4**: `RateLimiter.Stop()` の公開シグネチャ・既存挙動を変更しない

現状の `RateLimiter.Stop()` は `close(rl.stopCh)` を呼ぶため、二重呼び出しすると
`close of closed channel` panic を起こす。既存テスト（`internal/middleware/ratelimit_test.go`）は
`defer rl.Stop()` で 1 回だけ Stop を呼ぶ前提であり、`Stop()` 本体を変えると既存挙動が変わる。

### 採用方針: app 層で「1 回だけ呼ぶ」を保証（`Stop()` 本体は無変更）

`internal/app/shutdown.go` に `shutdownCoordinator` 構造体を新設し、シャットダウン手続き
（`server.Shutdown` → `RateLimiter.Stop`）を束ねた。`RateLimiter.Stop()` の呼び出しは
`sync.Once` で保護し、**シャットダウン経路から高々 1 回だけ**実行されることを保証する。

- `RateLimiter.Stop()` 本体は **一切変更していない**（AC 2.3 / 2.4 を満たす）。
  既存ミドルウェア・既存テストの挙動は不変。
- panic 防止は app 層（呼び出し側）の `sync.Once` で実現。これにより、シグナルハンドラの
  重複・複数経路からの呼び出しなど「シャットダウン経路が重複起動され得る状況」（AC 2.2）でも
  二重 close panic が発生しない（NFR 1.2 を満たす）。

タスク指示の第一候補（「`Stop()` を厳密に 1 回だけ呼ぶ」）に沿いつつ、シグナル重複等の
万一の重複起動に対する堅牢性を `sync.Once` で確保した。`Stop()` 自体を `sync.Once` で
idempotent 化する案は採らなかった（AC 2.4「既存挙動を変更しない」をより厳密に守るため、
変更は app 層に閉じた）。

### 停止タイミング（AC 3.1 / 3.2）

`shutdownCoordinator.shutdown()` は `server.Shutdown(ctx)`（稼働中 HTTP リクエストの drain 待機）
の **完了後**に `RateLimiter.Stop()` を呼ぶ順序とした。これにより、稼働中リクエストの処理を
不当に阻害せず、バックグラウンドのクリーンアップ goroutine だけを終了する。通常稼働中
（`<-stop` 受信前）には Stop を呼ばない（AC 3.1）。

### 適用範囲（AC 4.1 / 4.2）

- `runServe`（API サーバーモード）のみ RateLimiter を構築・停止する。
- `runWorker` は RateLimiter を構築しないため変更していない（AC 4.2）。
  `newShutdownCoordinator` は `rateLimiter == nil` を許容し、その場合 Stop を呼ばない設計だが、
  worker からは coordinator 自体を呼び出していない。

## 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `internal/app/app.go` | `runServe` で `RateLimiter` をインライン構築から変数 `rateLimiter` に切り出し。シャットダウンシーケンスを `shutdownCoordinator` 経由に変更（`server.Shutdown` 直呼びを置換） |
| `internal/app/shutdown.go` | 新規。`shutdownCoordinator` 構造体と `newShutdownCoordinator` / `shutdown` / `stopRateLimiter` メソッドを定義 |
| `internal/app/shutdown_test.go` | 新規。goroutine リーク検証・panic 非発生・二重起動 idempotency のテスト |

`RateLimiter.Stop()`（`internal/middleware/ratelimit.go`）は無変更。

## テスト観点と AC 対応

| AC / NFR | 検証テスト | 観点 |
|---|---|---|
| AC 1.1 / 1.3 (シャットダウン時に Stop 呼び出し / goroutine 残存させない) | `TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine` | `NewRateLimiter` で goroutine 増加を確認 → `shutdown` 後に `runtime.NumGoroutine()` が起動前水準に収束（goroutine リークなし） |
| AC 1.2 / 1.4 (稼働中は継続 / 停止は 1 回) | 既存 `internal/middleware/ratelimit_test.go`（稼働中はクリーンアップ goroutine 継続で 429 応答等が機能）+ `TestShutdownCoordinator_DoubleInvocationDoesNotPanic`（1 回しか Stop が呼ばれないこと=二重 close panic なしで担保） | |
| AC 2.1 (panic を発生させず正常終了) | `TestShutdownCoordinator_DoesNotPanic` | `shutdown` 呼び出しで panic しないこと |
| AC 2.2 / NFR 1.2 (重複起動でも panic しない / 二重起動の panic 非発生検証) | `TestShutdownCoordinator_DoubleInvocationDoesNotPanic` | `shutdown` を 2 回呼んでも panic せず goroutine も収束（`sync.Once` による idempotency） |
| AC 2.3 / 2.4 (Stop シグネチャ・既存挙動不変) | 既存 `internal/middleware/ratelimit_test.go` 全件が無変更で pass | `Stop()` 本体を変更していないため既存テストがそのまま green |
| AC 3.1 / 3.2 (停止タイミング) | `TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine`（`server.Shutdown` 後に Stop を呼ぶ順序を coordinator が保証） | コード上で `server.Shutdown` → `stopRateLimiter` の順序を固定 |
| AC 4.1 (構築モードは停止する) | `runServe` が coordinator 経由で停止（実コード）+ 上記 goroutine リークテスト | |
| AC 4.2 (非構築モードは停止しない) | `runWorker` 無変更（RateLimiter を構築しない）。`internal/app/run_test.go` 既存テストが pass | |
| NFR 1.1 (goroutine 停止を観察可能に検証) | `TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine` の `runtime.NumGoroutine()` 比較（`go.uber.org/goleak` 等の新規依存は追加せず標準 `testing` + `runtime` で実装） | |
| NFR 2.1 / 2.2 (応答挙動不変 / 既存テスト無変更で通過) | 既存 `internal/middleware/ratelimit_test.go`・`internal/handler/*_test.go` が無変更で pass | |

goroutine 数の比較は非同期停止を考慮し、`waitGoroutineCount`（最大 2 秒のポーリングで
収束待ち）で flaky を回避。`-race -count=5` でも安定して pass することを確認済み。

## 検証コマンド結果

- `gofmt -l`（変更ファイル）: 差分なし
- `go vet ./internal/app/... ./internal/middleware/...`: pass
- `go build ./...`: pass
- `go test ./...`: 全パッケージ pass（`internal/repository` の DB 結合テストもローカル PostgreSQL で pass）
- `go test -race -count=5 ./internal/app/...`: pass（goroutine リークテストの安定性確認）

> 環境注記: 本環境のデフォルト Go は 1.22.2 だが go.mod は `go 1.25` を要求するため、
> `GOTOOLCHAIN=go1.25.0` を明示してツールチェインを取得・実行した。CI（`go test ./...` /
> `npm test`）は go.mod 準拠で 1.25 系を使う想定。

## 確認事項

- 新規 `shutdownCoordinator` は `runServe` のシャットダウン経路をテスト可能な単位に切り出す
  ためのもの。`runServe` 全体は DB 接続を要しテストしにくいため、停止ロジックのみを
  coordinator として分離した（過度な抽象化を避け、責務はシャットダウン手続きの調整に限定）。
- NFR 1.2 の「二重起動で panic しない」検証は app 層（`shutdownCoordinator`）で担保している。
  `RateLimiter.Stop()` 単体は依然として二重呼び出しで panic する（既存挙動の維持＝AC 2.4）。
  仕様の Open Questions でも「停止処理側を idempotent 化するか / 1 回だけ呼ぶかは実装判断に委ねる」
  とされており、本実装は後者（呼び出し側で 1 回保証 + 重複起動への堅牢性）を採用した。

STATUS: complete
