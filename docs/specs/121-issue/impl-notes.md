# Implementation Notes — Issue #121 フィード横断新着一覧機能

## Implementation Notes

### Task 1

- 採用方針: `user_cross_feed_views` テーブル（UUID PK + last_seen_at + updated_at）と対応する Go ドメインモデル `UserCrossFeedView` を新規追加。
- 重要な判断: design.md / tasks.md が指定する migration timestamp `20260528120000` が既存 `20260528120000_add_item_search_indexes` と完全衝突するため、同日 1 時間後ろ倒しの `20260528130000` に micro-adjust した（採番のみの差異で意味的変更なし）。
- 残存課題: なし。

### Task 2

- 採用方針: `UserCrossFeedViewRepository` interface を `internal/repository/interfaces.go` に追加し、`internal/repository/postgres_user_cross_feed_view_repo.go` に Postgres 実装（`Get` / `Upsert`）を新設。Upsert は `INSERT ... ON CONFLICT (user_id) DO UPDATE SET last_seen_at=EXCLUDED.last_seen_at, updated_at=now()` で冪等性を確保し、`updated_at` は DB 側 `now()` で統一して時刻ドリフトを避ける。
- 重要な判断: integration test の DB セットアップは既存 `setupItemSearchTestDB` / `setupListDueTestDB` と同パターン（`TEST_DATABASE_URL` 環境変数 fallback + Skip ガード + 全テーブル DROP→`database.RunMigrations`）を採用し、ヘルパ名は他テスト群と衝突しないよう `setupCrossFeedViewTestDB` / `insertTestUserForCrossFeedView` / `crossFeedViewTestDatabaseURL` と命名で分離した（既存 `insertTestItem` 重複事象と同種の衝突を予防）。`cleanupSQL` は `user_cross_feed_views` を先頭に追加（CASCADE で消えるが明示的に列挙）。
- 残存課題: なし。Task 1 で確認済の `postgres_item_repo_starred_test.go` の `insertTestItem` 重複事象は task 2 着手時点でも継続しており（origin/claude/issue-121-impl-issue を merge した後も解消されない既存事象。merge 元の develop 経由で持ち込まれた事象）、本 task の変更とは無関係。`internal/repository` パッケージ全体での `go vet` / `go test` は当該既存事象でビルド失敗するが、本 task の新規ファイル単体は `go build ./internal/repository/...` で正常コンパイルすることを確認済。

## 確認事項

- task 1 で migration timestamp が design.md 指定（`20260528120000`）と既存 migration `20260528120000_add_item_search_indexes` で衝突したため、`20260528130000` に micro-adjust した。後続 task の本文中に同 timestamp を参照する箇所は無いため影響なし（design.md は書き換えていない）。
- `go vet ./...` 全体実行時に `internal/repository/postgres_item_repo_starred_test.go` で `insertTestItem redeclared in this block` の既存事象が出力されたが、本 task で触れていないファイル群であり、本 task の変更とは無関係（変更パッケージ `internal/model` / `internal/database` 単体での vet は pass）。後続 task または別 Issue での対処を要する可能性がある旨を Reviewer / PM に共有する。
