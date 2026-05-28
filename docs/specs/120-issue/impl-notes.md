# Implementation Notes

本ファイルは Issue #120「記事の検索機能」の per-task 実装での learning と残存課題を
記録する。各 task ごとに `### Task <id>` 見出しで learning を追記し、先行 task の
learning は改変しない。

## Implementation Notes

### Task 1

- 採用方針: design.md の Physical Data Model / Migration Strategy 節の指定どおり、`pg_trgm`
  拡張の `CREATE EXTENSION IF NOT EXISTS` と `items` テーブル `title` / `content` への
  GIN（`gin_trgm_ops`）インデックスを追加する SQL マイグレーション 2 ファイル
  （`20260528120000_add_item_search_indexes.up.sql` / `.down.sql`）を新規追加した。
- 重要な判断:
  - **down.sql で `DROP EXTENSION pg_trgm` を行わない**。design.md「Physical Data Model」の
    「拡張は他用途で使用されうるため DROP しない方針」決定（tasks.md Task 1 の指示と整合）に
    従い、down はインデックス 2 本の `DROP INDEX IF EXISTS` のみとする。`pg_trgm` は今後他の
    検索機能や類似度比較でも利用され得るため、down で巻き戻すと他機能の indexes が壊れる
    リスクを避ける。
  - **`content` GIN は `WHERE content IS NOT NULL` の partial index**。`items.content` は
    `TEXT NULL` 許容（`initial_schema.up.sql:67`）であり、NULL 行を index から除外することで
    index サイズと更新コストを抑える。tasks.md Task 1 の SQL 文と完全一致。
  - **down の DROP 順は up の CREATE 逆順**（先に `idx_items_content_trgm` → 後に
    `idx_items_title_trgm`）。両 index は独立で依存関係はないが、慣習として逆順を採用。
  - 既存 migration（`20260527080000_add_item_states_timestamps.*.sql`）と同じく、ファイル先頭に
    日本語 1 行の説明コメント、各 SQL 文を改行で区切るスタイルに揃えた。
- 残存課題（次 task に影響する事項）:
  - 本番運用時の `CREATE INDEX CONCURRENTLY` 化（design.md「Migration Strategy」/
    `golang-migrate` の transaction wrap 制約）は別 Issue 扱い。現スケールでは通常
    `CREATE INDEX` で許容範囲と design 判断済み。Task 3.3 以降の DB 結合テストでは
    `TEST_DATABASE_URL` 経由で実 PostgreSQL に migration が適用される前提で本 index が
    検索 SQL から利用される。
  - 本 task は SQL のみで Go コード変更なし。検索本体（service / repository / handler / web）は
    Task 2〜8 で順次実装される。
