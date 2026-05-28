# Implementation Notes

本ノートは Issue #117（フィード横断スター記事一覧）の per-task 実装ループにおける
各 task の learning を記録する。

## Implementation Notes

### Task 1

Repository 層 `ListStarredByUser` メソッドの追加と DB 結合テスト。

#### 採用方針

- `repository.StarredItemRow` 構造体（`model.ItemWithState` を embed + `FeedTitle string`）を
  `interfaces.go` 内に新設。design.md の選択肢のうち「`repository` パッケージ内に新設」を採用。
  理由: `model` パッケージは純粋ドメインモデルに留め、JOIN 由来の派生カラム（feed_title）は
  repository 層のクエリ結果として扱う方が責務分離が綺麗になるため。
- SQL は `items i INNER JOIN item_states s ON i.id = s.item_id INNER JOIN feeds f ON i.feed_id = f.id`
  の 3 段 INNER JOIN を採用。`WHERE s.user_id = $1 AND s.is_starred = true` で部分インデックス
  `idx_item_states_user_starred` を駆動。
- `SELECT` に `true AS is_starred` を明示し、INNER JOIN の結果が常に true であることを SQL
  レベルで宣言する（design.md §Repository SQL 設計方針と同じ）。

#### 重要な判断

- **mockItemRepo へのスタブ追加**: `internal/item/upsert_test.go` の `mockItemRepo` に
  `ListStarredByUser` のスタブを追加（戻り値 `(nil, nil)`）。Service 層テストでは未使用だが、
  `var _ ItemRepository = (*PostgresItemRepo)(nil)` と同じ interface 充足のため
  サービス層テストの mock も interface を満たす必要があるため。Task 2 で実体実装が入る。
- **テスト fixture の helper 関数**: `insertTestItem` / `insertTestItemState` を新設。既存
  `postgres_feed_repo_test.go` の `insertTestUser` / `insertTestFeedWithTitle` /
  `setupListDueTestDB` は package-private で同 package 内から再利用可能だったため、それらを
  再利用しつつ items / item_states 専用 helper のみ追加した。
- **テストケース構成**: 設計指示の (a)〜(g) を以下にマップ:
  - (a) (c) (d) (g): 「自ユーザーの複数フィードのスター記事が降順で返り feed_title が付与される」
    1 ケースに統合（同一 fixture で 4 観点を同時に検証可能なため）
  - (b): 「他ユーザーのスター記事は返らない」（NFR 2.1）
  - (b) 拡張: 「スター解除済み記事は返らない」（is_starred=false の状態行が混入しないこと）
  - (e): 「cursor 指定時に当該時刻より前の記事のみ返る」
  - (f): 「スター記事が 0 件のとき空スライスが返る」
  - 追加: 「limit が指定件数で SQL レベルに反映される」（境界系の補強）

#### EXPLAIN ANALYZE 結果（NFR 1.1 / 1.2 検証）

5000 記事 / 100 ユーザー / 50 フィード / user1 が 100 件スター / 他ユーザー 99 名で合計
396 件スターの状態で `ANALYZE` 済みの状態において、cursor なし先頭ページ取得の実プランは
以下:

```
Limit  (cost=56.79..56.80 rows=5 width=1180) (actual time=0.267..0.271 rows=50 loops=1)
   InitPlan 1 (returns $0)
     ->  Seq Scan on users  (cost=0.00..3.25 rows=1 width=16) (actual time=0.005..0.009 rows=1 loops=1)
           Filter: ((email)::text = 'seed-1@example.com'::text)
   ->  Sort  (cost=53.54..53.55 rows=5 width=1180) (actual time=0.266..0.268 rows=50 loops=1)
         Sort Key: i.published_at DESC
         ->  Nested Loop  (cost=4.61..53.48 rows=5 width=1180) (actual time=0.041..0.228 rows=100 loops=1)
               ->  Nested Loop  (cost=4.47..52.68 rows=5 width=1172) (actual time=0.036..0.168 rows=100 loops=1)
                     ->  Bitmap Heap Scan on item_states s  (cost=4.19..11.18 rows=5 width=17) (actual time=0.026..0.033 rows=100 loops=1)
                           Recheck Cond: ((user_id = $0) AND is_starred)
                           ->  Bitmap Index Scan on idx_item_states_user_starred  (cost=0.00..4.18 rows=5 width=0) (actual time=0.022..0.022 rows=100 loops=1)
                                 Index Cond: (user_id = $0)
                     ->  Index Scan using items_pkey on items i  (cost=0.28..8.30 rows=1 width=1171) (actual time=0.001..0.001 rows=1 loops=100)
                           Index Cond: (id = s.item_id)
               ->  Index Scan using feeds_pkey on feeds f  (cost=0.14..0.16 rows=1 width=23) (actual time=0.000..0.000 rows=1 loops=100)
                     Index Cond: (id = i.feed_id)
 Planning Time: 2.160 ms
 Execution Time: 1.352 ms
```

cursor 指定時（`i.published_at < now() - interval '30 minutes'`）のプラン:

```
Limit  (cost=56.83..56.84 rows=5 width=1180) (actual time=0.230..0.233 rows=50 loops=1)
   ...
   ->  Bitmap Index Scan on idx_item_states_user_starred  (cost=0.00..4.18 rows=5 width=0) (actual time=0.018..0.018 rows=100 loops=1)
   ...
   ->  Index Scan using items_pkey on items i  (cost=0.28..8.31 rows=1 width=1171) (actual time=0.001..0.001 rows=1 loops=100)
         Index Cond: (id = s.item_id)
         Filter: (published_at < (now() - '00:30:00'::interval))
 Planning Time: 0.900 ms
 Execution Time: 0.290 ms
```

確認できた事項:

- **NFR 1.2（部分インデックスを破壊しない）**: `idx_item_states_user_starred` が
  `Bitmap Index Scan` で実際に選択されている。Recheck Cond / Index Cond が想定通り。
- **NFR 1.1（単一フィード API と同等の応答時間）**: 1.352 ms / 0.290 ms はいずれも単一フィード
  クエリの一般的な実行時間（< 数 ms）と同水準であり、有意な悪化はない。
- **N+1 不在**: `feed_title` 取得のための JOIN は `feeds_pkey` を用いた 1 段の Index Scan で、
  ループ内追加クエリは発生していない（design.md §Performance & Scalability 通り）。
- 注意: cursor 適用条件 `published_at < ...` は `Index Scan using items_pkey` の Filter として
  適用されている。今回の fixture では `item_states` 経由で対象が 100 件まで絞り込まれた
  状態で items_pkey にぶつけているため、Filter 適用後の効率は十分。本番でスター件数が
  数万件規模になる場合は `items(id) + published_at` の複合インデックス追加判断が将来的に
  必要になる可能性があるが、本 spec の Non-Goals「追加インデックスの導入判断は design の
  領分」「DB スキーマ追加・新規マイグレーション無し」と整合するため、本タスクでは扱わない。

#### 残存課題

- なし。Task 2（Service 層）は本タスクの `StarredItemRow` を受け取って `StarredItemListResult`
  を組み立てる予定。`StarredItemRow` の構造体定義は安定しており、Task 2 で追加変更しない想定。
- 派生タスク候補: 本番運用でスター件数が極端に多くなった場合の SQL チューニング判断
  （`items` への複合インデックス追加 or マテビュー化）は将来課題として残置可。本 Issue の
  Non-Goals 範囲内。

## 確認事項

なし（design.md / requirements.md と本実装に矛盾は確認されていない）。

## Requirements Coverage (Task 1 範囲)

| Requirement | テスト |
|---|---|
| 2.2（公開日時降順で全フィード横断スター記事を表示） | `TestPostgresItemRepo_ListStarredByUser/自ユーザーの複数フィードのスター記事が降順で返り_feed_title_が付与される` |
| 2.4（フィードタイトルの併記） | 同上（`FeedTitle` フィールドが "Feed A" / "Feed B" として返ることを検証） |
| 4.1（リクエストユーザー自身のスター記事のみ） | 同上 + `他ユーザーのスター記事は返らない` |
| 4.2（published_at 降順） | 同上 |
| 4.9（別ユーザーのスター記事を一切含めない） | `他ユーザーのスター記事は返らない` |
| 4.10（feed_id を含む） | `StarredItemRow` が `model.ItemWithState` を embed しているため `FeedID` も含まれる（実装上の自明な担保） |
| NFR 1.1（応答時間） | EXPLAIN ANALYZE 結果（1.352 ms / 0.290 ms、上記）で検証 |
| NFR 1.2（部分インデックスを利用可能な状態） | EXPLAIN ANALYZE で `Bitmap Index Scan on idx_item_states_user_starred` が選択されることを確認 |
| NFR 2.1（クロスユーザー漏洩なし） | `他ユーザーのスター記事は返らない` |
| 追加: cursor 境界 | `cursor 指定時に当該時刻より前の記事のみ返る` |
| 追加: 空状態 | `スター記事が 0 件のとき空スライスが返る` |
| 追加: limit 境界 | `limit が指定件数で SQL レベルに反映される` |

## テスト実行コマンド

```sh
# テスト用 PostgreSQL に接続可能な環境で
TEST_DATABASE_URL="postgres://<user>:<pass>@<host>:5432/feedman_test?sslmode=disable" \
  go test ./internal/repository/... -run TestPostgresItemRepo_ListStarredByUser

# 接続できない場合は t.Skip で自動スキップ（既存テストと同じパターン）
go test ./internal/repository/... -run TestPostgresItemRepo_ListStarredByUser
```
