# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-2-impl-db
- HEAD commit: a3c9a0365e4bcda24e3567d2aa46b25a1198a76d
- Compared to: develop..HEAD

差分は 3 ファイル（`internal/database/db.go` / `internal/database/db_test.go` /
`docs/specs/2-db/impl-notes.md`）に閉じている。`tasks.md` / `design.md` は本 spec ディレクトリに
存在しない（Architect 未経由の Issue）ため、`_Boundary:_` アノテーション照合は適用対象外。
CLAUDE.md の `## Feature Flag Protocol` 採否は **opt-out** のため flag 観点の確認は行わない。

## Verified Requirements

- 1.1 — `db.SetMaxOpenConns(maxOpenConns)`（db.go:41）/ `TestOpen_SetsMaxOpenConns`（実物 `db.Stats().MaxOpenConnections == 25` を検証）
- 1.2 — `maxOpenConns = 25`（db.go:19、無制限 0 以外の正の有限値）/ `TestPoolConstants_AreFinitePositive`（`maxOpenConns > 0`）
- 1.3 — `2 * maxOpenConns = 50 <= 100` / `TestPoolConstants_TwoProcessSumWithinMaxConnections`
- 2.1 — `db.SetMaxIdleConns(maxIdleConns)`（db.go:42）/ `TestPoolConstants_AreFinitePositive`（`maxIdleConns > 0`）
- 2.2 — `maxIdleConns = 10 <= maxOpenConns(25)` / `TestPoolConstants_AreFinitePositive`（`maxIdleConns <= maxOpenConns`）
- 3.1 — `db.SetConnMaxLifetime(connMaxLifetime)`（db.go:43）/ `TestPoolConstants_AreFinitePositive`（`connMaxLifetime > 0`）
- 3.2 — `connMaxLifetime = 5 * time.Minute`（db.go:28、正の有限の時間値）/ `TestPoolConstants_AreFinitePositive`
- 4.1 — 接続プール設定値はパッケージレベルの名前付き定数として定義（db.go:15-29、マジックナンバー直書きなし）
- 4.2 — `Open(databaseURL string) (*sql.DB, error)` のシグネチャ無変更（diff 上で引数・返り値型の変更なし）
- 4.3 — 呼び出し側 `internal/app/app.go:90`（runServe）/ `:215`（runWorker）は無変更で `database.Open(cfg.DatabaseURL)` を呼ぶ（差分は database パッケージに閉じ、呼び出し側コードに変更なし）
- NFR 1.1 — 2 プロセス合算 ≤ 100 を `TestPoolConstants_TwoProcessSumWithinMaxConnections` で担保
- NFR 2.1 — 既存テスト（`TestOpen_ReturnsDBForAnyURL` / `TestOpen_WithValidURL_ReturnsDB`）を無変更で維持し、`Open` → `Ping` → ワイヤリングの観測挙動を保持。`go test ./internal/database/...` 通過を確認

## Findings

なし

## Summary

全 numeric AC（1.1〜4.3）および NFR 1.1 / 2.1 が実装とテストでカバーされている。`Open` シグネチャ
無変更で後方互換を保ち、設定値は名前付き定数化済み。`go test ./internal/database/...` も通過。

RESULT: approve
