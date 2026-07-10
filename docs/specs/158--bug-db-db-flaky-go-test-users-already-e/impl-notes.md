# Implementation Notes - Issue #158

## 概要

`go test ./...` を Backend Tests job 上で実行すると、`internal/database` と
`internal/repository` が同一テスト用 DB（`feedman_test`）を共有したまま **パッケージ間並列**
で実行され、`migrate` の冪等性検証中に他パッケージのテストが同じ DB に DDL/DML を流し込み
`pq: relation "users" already exists` 等の flaky 失敗を発生させていた。

本 PR は方針「**案 C + 案 A**」のうち、**案 A（`go test -p 1` でパッケージ間並列を 1 に
抑制）** を導入する（案 C の postgres services + `TEST_DATABASE_URL` は既に main 反映済み）。

## 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `.github/workflows/ci.yml` | Backend Tests job「Run tests」ステップを `go test ./...` → `go test -p 1 ./...` に変更し、意図コメント（Issue #158 / DB 共有レース回避）を追記 |
| `README.md` | 「バックエンドのテスト」節に `-p 1` 直列化コマンドと、ローカル PostgreSQL 起動例 + `TEST_DATABASE_URL` 設定例を追記 |
| `docs/specs/158--bug-db-db-flaky-go-test-users-already-e/impl-notes.md` | 本ファイル（新規） |

`internal/*` の本体・テスト・マイグレーションは **一切変更していない**（Out of Scope 順守）。

## 設計判断

### なぜ案 C + 案 A か（最小コストの根本修正）

- **案 A 単独**: `-p 1` で直列化するだけでは、CI 環境で PostgreSQL が無いと依然 skip され
  回帰を検知できない。**案 C（postgres service + `TEST_DATABASE_URL`）** が前提として必要
- **案 C 単独**: PostgreSQL を CI に立てても、パッケージ間並列で同じ DB を踏み合うため
  `pq: relation "users" already exists` 等のレースが残る
- **案 C + 案 A**: 既に main に入っている案 C の前提を活かしたうえで、`-p 1` を 1 行追加する
  だけで根本回避できる。schema-per-package / database-per-package の根本再設計は Out of
  Scope（spec に明記）

### `-p 1` の選択理由（vs `t.Parallel()` 削除 / `-parallel 1`）

- `go test -p N` は **パッケージ間** の並列度を制御するフラグであり、`-p 1` で「同一 binary
  で複数パッケージを順次実行」ではなく「パッケージごとに別 binary を順次 spawn」となる
- `-parallel N` はパッケージ **内部** の `t.Parallel()` 並列度を制御するフラグで、本件の
  原因（パッケージ間レース）には作用しない。Req 3.5（パッケージ内部 `t.Parallel()` は維持）
  との両立のため `-parallel` ではなく `-p` を選択
- 既存テストの `t.Parallel()` 呼び出しは触らずに保留できるため、Req 3.1〜3.5（後方互換）を
  自然に満たす

### 既存テストの直列化前の DB ライフサイクル

`internal/database/migrate_test.go` と `internal/repository/*_test.go` はいずれも
`testDatabaseURL`（既定 `postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable`）
の同一 DB を使用しており、各テストが `DROP TABLE` 系の後片付けを行う前に他パッケージのテスト
が `CREATE TABLE` を始めるとレースする。`-p 1` で「`internal/database` 完了 → `internal/repository`
開始」の時間軸を保証することで、Req 1.3（DB セットアップ・マイグレーション・後片付けが
時間的に重ならない）を満たす。

## 検証結果

| 検証項目 | 結果 |
|---|---|
| YAML 構文確認（`python3 -c "import yaml; yaml.safe_load(...)"`） | OK |
| `go vet ./...` | clean（warning なし） |
| `gofmt -l .` | 既存 pre-existing 未整形ファイル群のみ（本 PR で導入された差分は無し / 本 PR は Go ソースを変更していない） |
| `go test -p 1 ./...` | 全パッケージ ok（ローカルに DB が無いため `internal/database` / `internal/repository` は `t.Skipf` で skip 想定。CI では service container 経由で実走） |
| 既存テスト関数名・env 名・接続先意味の変更 | なし（Req 3.1〜3.4 順守） |

## Requirements トレーサビリティ

| Req ID | 実現箇所 |
|---|---|
| 1.1 (`-p 1` 設定で `go test` をパッケージ間並列化しない) | `.github/workflows/ci.yml` の Run tests ステップ |
| 1.2 (10 回連続実行で `pq: relation "users" already exists` 失敗 0%) | `-p 1` 直列化で DB 共有レースを排除（CI 実走で検証される最終確認項目） |
| 1.3 (両パッケージのセットアップ・後片付けが時間的に重ならない) | `-p 1` がパッケージ別 binary を順次 spawn することで保証 |
| 1.4 (既存 service container + `TEST_DATABASE_URL` を変更せず利用) | `ci.yml` の services / env ブロックは無変更 |
| 2.1 (ローカルも CI と同等コマンドで直列化される) | `README.md` 「バックエンドのテスト」節を `go test -p 1 ./...` に統一 |
| 2.2 (test documentation に直列化コマンド例を掲載) | `README.md` 「バックエンドのテスト」節に追記 |
| 2.3 (DB 未到達なら `t.Skipf`、CI／ローカルで失敗扱いにしない) | 既存テスト挙動を変更していないため自動的に成立 |
| 3.1〜3.5 (後方互換) | テスト関数名・env 名・本体ロジック・マイグレーション SQL・パッケージ内 `t.Parallel()` のいずれも未変更 |
| NFR 1.1 / 1.2 (flaky 失敗率 0%, retry 追加なし) | `-p 1` のみで達成、`continue-on-error` 等の誤魔化しなし |
| NFR 2.1 (実行時間 50% 超劣化させない) | パッケージ間並列度を 1 に下げるため数十秒オーダーの増加は予想されるが、ローカル実走（cached ヒット込）で大幅劣化は観測されず、CI 本番計測で最終確認 |
| NFR 3.1 (失敗箇所が不可視化されない) | `-p 1` は出力フォーマットを変えず、パッケージ単位の `ok` / `FAIL` 表示は維持される |

## 確認事項（レビュワー向け）

- NFR 2.1（実行時間 50% 超劣化禁止）は本 PR の commit を main へ merge 後の CI 実走で
  実測値を確認する想定。ローカルでは fully-cached のため有意な比較が取れない
- 既存 spec の Out of Scope に従い、`schema-per-package` / `database-per-package` の根本
  再設計は今回着手していない。仮に将来パッケージ追加で `-p 1` の実行時間影響が許容できなく
  なった場合は別 spec で再設計する
- `gofmt -l .` で未整形ファイルが出るが、これは本 PR 範囲外の既存差分。本 PR は Go ソースに
  触れていないため fix もしていない

## STATUS

STATUS: complete
