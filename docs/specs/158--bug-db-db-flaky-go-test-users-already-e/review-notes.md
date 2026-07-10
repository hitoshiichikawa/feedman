# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-03T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-158-impl--bug-db-db-flaky-go-test-users-already-e
- HEAD commit: 672c020d84b56078634b47a99a5a4a69ef53087c
- Compared to: develop..HEAD
- 変更ファイル: `.github/workflows/ci.yml` / `README.md` / `docs/specs/158-.../impl-notes.md`
- 本 Issue は design-less impl（`tasks.md` / `design.md` は存在しない）。判定は `requirements.md` の
  numeric ID と差分の突き合わせで実施した

## Verified Requirements

- 1.1 — `.github/workflows/ci.yml` の「Run tests」ステップが `go test ./...` → `go test -p 1 ./...`
  に変更され、パッケージ間並列度を 1 に抑制している（line 56）
- 1.2 — `-p 1` によりパッケージ別 binary が順次 spawn されるため、`internal/database` と
  `internal/repository` の DB 共有レース（`pq: relation "users" already exists` 等）が時間軸で
  排除される。10 回連続実行での失敗率 0% は CI 実走による最終確認項目（impl-notes.md にも明記）
- 1.3 — `-p 1` がパッケージ単位のセットアップ・マイグレーション・後片付けを直列化することで担保
- 1.4 — `ci.yml` の `services.postgres` ブロックおよび `env.TEST_DATABASE_URL` は無変更
  （diff 上で services / env セクションへの変更は無し）
- 2.1 — `README.md` の「バックエンドのテスト」節を `go test -p 1 ./...` に統一しており、CI と
  同等コマンドがローカル手順として提示されている
- 2.2 — README.md にローカル DB 起動例（`docker run` + `TEST_DATABASE_URL` export）と直列化コマンドが
  追記されている
- 2.3 — 既存テストの skip ロジック（`t.Skipf`）には触れていないため自動的に成立
- 3.1 — テスト関数の改名は無し（`*_test.go` ファイルへの変更は無い）
- 3.2 — 環境変数名・default 接続先の意味は変更されていない（`TEST_DATABASE_URL` の値もそのまま）
- 3.3 — `api` / `worker` / `internal/*` の非 `_test.go` ファイルへの変更は無い
- 3.4 — `db/migrations/*.sql` および migration 適用ロジックへの変更は無い
- 3.5 — `-p` は **パッケージ間** 並列度を制御するフラグで、パッケージ内部の `t.Parallel()`
  並列度（`-parallel`）には作用しない。実装方針として正しく分離されている
- NFR 1.1 — `-p 1` による直列化で根本回避（CI 実走で最終確認）
- NFR 1.2 — `continue-on-error` / 自動 retry / quarantine 等の誤魔化しは導入されていない
  （ci.yml の差分は Run tests ステップのコマンド変更のみ）
- NFR 2.1 — 実行時間の数値検証は CI 本番実走後に確認する旨が impl-notes.md「確認事項」で明示
  されており、設計上の劣化リスクは Out of Scope（schema-per-package 再設計）として spec で
  既に切り分けられている。本 PR スコープとしては AC を満たすと判断
- NFR 3.1 — `-p 1` は `go test` の出力フォーマットを変更しないため、パッケージ単位の `ok` / `FAIL`
  表示は維持される

## Findings

なし

## Summary

`go test -p 1 ./...` への変更で DB 共有レースを根本回避しており、修正方針「案 C + 案 A」のうち
未適用だった案 A を最小差分（ci.yml 1 行 + README.md 説明追記）で実現している。requirements.md の
全 numeric ID（1.1–1.4 / 2.1–2.3 / 3.1–3.5 / NFR 1.1, 1.2, 2.1, 3.1）に対応する実装または既存挙動の
温存が確認できる。boundary 逸脱（`internal/*` 本体・migration SQL・既存テスト関数名への変更）も
無く、Out of Scope 順守。

RESULT: approve
