# Requirements Document

## Introduction

`go test ./...` を CI（GitHub Actions Backend Tests job）またはローカル PostgreSQL 接続環境で
実行すると、`internal/database` と `internal/repository` の DB 結合テストが **同一テスト用 DB を
共有したまま並列実行**される結果、`migrate` の冪等性検証中に他パッケージのテストが同じ DB へ
DDL/DML を流し込み、`pq: relation "users" already exists` や行数不一致などのレースで間欠的に
失敗（flaky）する。CI は既に `postgres` service と `TEST_DATABASE_URL` を設定済み（PR 起点で
案C が反映済み）であり、本 spec が解決すべき残課題は **テスト実行の並列度を絞ってパッケージ間
レースを排除する**こと（修正方針: 案 C + 案 A）。本変更は CLAUDE.md「テスト規約」の flaky
quarantine 禁止方針に基づき、原因を特定したうえで根本修正として扱う。

## Requirements

### Requirement 1: CI における DB 結合テストの直列化

**Objective:** As a CI 運用者, I want Backend Tests job のテスト実行が同一 PostgreSQL を共有しても
レースを起こさないよう直列化されている, so that DB 結合テストが flaky に失敗せず main の green
状態を信頼できる

#### Acceptance Criteria

1. The Backend Tests job shall `go test` をパッケージ間で並列化しない設定で実行する
2. When Backend Tests job が同一 commit に対して連続 10 回実行された場合, the Backend Tests job
   shall `pq: relation "users" already exists` を含むスキーマ衝突エラーで失敗しない
3. When `internal/database` と `internal/repository` の両パッケージが同一 job 内で実行される場合,
   the Backend Tests job shall 両パッケージの DB セットアップ・マイグレーション・後片付けが
   時間的に重ならない順序で実行する
4. The Backend Tests job shall 既存の `postgres` service container と `TEST_DATABASE_URL`
   環境変数（案C 由来の設定）を変更せずそのまま利用する

### Requirement 2: ローカル開発環境での再現性

**Objective:** As a 開発者, I want ローカルで `go test ./...` 相当のコマンドを叩いた際にも CI と
同じ直列化方針が適用される, so that 「CI では落ちないがローカルで落ちる（またはその逆）」と
いう非対称な flaky を発生させない

#### Acceptance Criteria

1. When 開発者がリポジトリルートで CI と同等のテスト実行コマンドを実行する場合, the test runner
   shall CI と同じ並列度抑制ポリシーで全パッケージのテストを実行する
2. The test documentation（`README.md` または `docs/` 配下の該当箇所）shall ローカルでの DB 結合
   テスト実行手順として直列化を含むコマンド例を提示する
3. If `TEST_DATABASE_URL` が未設定かつ default 接続先（`postgres://feedman:feedman@localhost:5432/feedman_test`）
   へも到達できない場合, the DB-backed tests shall 従来通り `t.Skipf` で skip し、CI／ローカル
   いずれでも失敗扱いにしない

### Requirement 3: 既存テスト資産との後方互換

**Objective:** As a 既存テストの保守者, I want 直列化対応が既存テストの命名・接続情報・skip 条件を
壊さない形で導入される, so that 既存の DB 結合テスト群を書き換える追加コストを発生させない

#### Acceptance Criteria

1. The change shall 既存のテスト関数名（`TestXxx` 形式）を 1 件も改名しない
2. The change shall 既存の環境変数名（`TEST_DATABASE_URL` 等）と default 接続先の意味を変更しない
3. The change shall アプリケーション本体（`api` / `worker` / `internal/*` の非 `_test.go` ファイル）
   の挙動を変更しない
4. The change shall 既存のマイグレーション SQL ファイルおよびマイグレーション適用ロジックを
   変更しない
5. Where 既存テストが `t.Parallel()` を内部で使用している場合, the change shall そのパッケージ
   **内部**の並列実行までは禁止しない（パッケージ **間** の並列のみ抑制する）

## Non-Functional Requirements

### NFR 1: 信頼性（flaky 率）

1. When Backend Tests job が同一 commit に対して連続 10 回実行された場合, the Backend Tests job
   shall `pq: relation "users" already exists` または同等のスキーマ衝突に起因する失敗率を 0% に
   する
2. The Backend Tests job shall flaky 起因の retry を CI ワークフロー側に追加せずに上記成功率を
   達成する（quarantine / `continue-on-error` / 自動 retry での誤魔化しを行わない）

### NFR 2: CI 実行時間

1. When 直列化が適用された Backend Tests job を main 直近 5 回の実行で測定した場合, the Backend
   Tests job shall 並列実行時の実行時間と比較して 50% を超える劣化を起こさない（数値目標: ローカル
   実測で並列時 ~30 秒の場合、直列化後も 45 秒以内を目安とする）

### NFR 3: 観測性

1. If テストが失敗した場合, the test runner shall どのパッケージ・どのテスト関数で失敗したかを
   従来と同等の粒度で stdout に出力する（並列度を絞ったことで失敗箇所が不可視化されない）

## Out of Scope

- DB 結合テスト自体のアサーション内容・カバレッジ拡張
- `api` / `worker` / `internal/*` のアプリケーション本体ロジックの変更
- マイグレーション SQL（`db/migrations/*.sql` 等）の内容変更
- パッケージごとに独立した DB / スキーマを払い出すアーキテクチャ変更（schema-per-package /
  database-per-package 等の根本再設計）。今回は最小コストの直列化で flaky を排除する
- `go-vet` / `govulncheck` / `frontend` 等、Backend Tests 以外の CI job への変更
- ローカル `docker-compose` の構成変更
- CLAUDE.md 言語方針の変更や `.claude/rules/*` の改訂

## Open Questions

- なし（修正方針は Issue コメントで「案 C（CI に postgres service と `TEST_DATABASE_URL` 追加。
  既に反映済み）+ 案 A（`go test` の並列度抑制）」に確定済み。`-p 1` の具体的位置やコマンド表現は
  Architect/Developer の領分とする）
