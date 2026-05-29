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

### Task 3

- 採用方針: `ItemRepository` interface に `ListNewAcrossFeeds(ctx, userID, sinceTime, cursorPublishedAt, cursorItemID, limit) ([]CrossFeedItem, error)` を追加し、`postgres_item_repo.go` で `items × subscriptions × feeds × item_states (LEFT)` を 1 クエリで JOIN する実装を追加。cursor 有無で SQL を分岐し、cursor あり時のみ `(i.published_at, i.id) < ($3, $4::uuid)` のタプル比較を WHERE に含める（plain text の文字列比較を避けるため `::uuid` キャストを付与し、`SearchByUserAndKeyword` の既存パターンと整合）。新規 row 型 `CrossFeedItem` は `model.ItemWithState` を embed し `FeedTitle` / `FaviconData` / `FaviconMime` を併記する形で `interfaces.go` に追加した。
- 重要な判断: (1) cursor の有効性判定は `!cursorPublishedAt.IsZero() && cursorItemID != ""` の AND 条件とし、片方のみゼロ値で渡された場合は cursor なし扱い（先頭から取得）に倒すことで呼び出し側のバリデーション漏れを安全側に倒した。(2) integration test は `setupListDueTestDB` の既存ヘルパを流用するが、ヘルパ `insertCrossFeedTestItem` / `insertCrossFeedTestItemState` / `updateCrossFeedFeedFavicon` / `sortDescending` は同パッケージ内の他テスト群（`insertTestItem` / `insertStarredTestItem` 等）との命名衝突を避けるため、テストの目的を表す接頭辞 `CrossFeed` を付与した独立命名で定義した（task 2 で確立済の命名分離パターンに準拠）。テストは独立ファイル `postgres_item_repo_cross_feed_test.go` に分離し、既存 `postgres_item_repo_test.go` には触れていない。(3) `ItemRepository` interface への新メソッド追加に伴い `internal/item/service_test.go` の `mockItemRepoForService` と `internal/item/upsert_test.go` の `mockItemRepo` の interface 適合が失われたため、両モックに `ListNewAcrossFeeds` の最小スタブ（常に nil を返す）を追加した。これは task 3 の boundary（ItemRepository）の interface 拡張に追随する不可避な修正であり、`item` ドメインの挙動は変更していない。
- 残存課題: なし。task 1, 2 で言及されていた `postgres_item_repo_starred_test.go` での `insertTestItem` 重複事象は本 task 着手時点では **解消済み**（`grep -rn "func insertTestItem"` で `postgres_item_repo_search_test.go` の 1 箇所のみヒット）であり、`go vet ./...` / `go build ./...` / `go test -count=1 ./internal/repository/... ./internal/item/...` がいずれもクリーンに通る状態に復旧している。DB 結合テスト本体は `TEST_DATABASE_URL` が未設定の環境（CI / 本作業環境）ではスキップされるため、`go test -short` ではテスト本体が起動しない点は既存パターンと同じ。

## 確認事項

- task 1 で migration timestamp が design.md 指定（`20260528120000`）と既存 migration `20260528120000_add_item_search_indexes` で衝突したため、`20260528130000` に micro-adjust した。後続 task の本文中に同 timestamp を参照する箇所は無いため影響なし（design.md は書き換えていない）。
- `go vet ./...` 全体実行時に `internal/repository/postgres_item_repo_starred_test.go` で `insertTestItem redeclared in this block` の既存事象が出力されたが、本 task で触れていないファイル群であり、本 task の変更とは無関係（変更パッケージ `internal/model` / `internal/database` 単体での vet は pass）。後続 task または別 Issue での対処を要する可能性がある旨を Reviewer / PM に共有する。
