# Implementation Notes

本 spec の per-task ループ実装中に得られた知見を、`### Task <id>` 見出し単位で
追記する。先行 task の見出しは改変・削除・並び替えしないこと（前方伝播の規律）。

## Implementation Notes

### Task 1

- 採用方針: 5 レイヤ縦断（`model.ItemSearchHit` → `PostgresItemRepo.SearchByUserAndKeyword` →
  `itemsearch.ItemSearchSummary` → `itemSearchHitResponse` (handler) → `ItemSearchServiceAdapter`）
  すべてに `HatebuFetchedAt *time.Time` を追加し、各レイヤで純粋な pass-through とした。
- 重要な判断:
  - JSON タグは `"hatebu_fetched_at,omitempty"` を採用し、Go 側 nil → JSON から省略する流儀を
    既存 `favicon_url` に揃えた（design.md 行 410 / 414 の方針）。フロント側は次の task 2 以降で
    TypeScript 型を `string | null` で受け、`undefined ?? null` の正規化で `-` 表示判定を統一する。
  - リポジトリ層は `sql.NullTime` で Scan し、`Valid` → `&Time` を保持する既存パターン
    （`FindByID` / `ListNeedingHatebuFetch` 等）に揃えた。SQL の `SELECT` 列追加のみで
    `WHERE` / `ORDER BY` / `LIMIT` / `JOIN` は不変（NFR 3.1 を担保）。
  - 既存テスト `TestSearch_HitsConvertedToSummaries` を拡張し、その上で `HatebuFetchedAt`
    特化テスト `TestSearch_HatebuFetchedAt_NilAndNonNilPassthrough` を別途追加した。
    nil/非 nil の両条件を 1 テスト 1 検証に分けつつ、既存 reflect.DeepEqual の全フィールド
    マッピング検証も `HatebuFetchedAt` を含む新 struct を期待値とした形で更新済み。
  - repo DB 結合テストは既存 `setupItemSearchTestDB` の Skip ガード（DB 接続不可で `t.Skip`）
    を踏襲。CI（DB 起動なし）では Skip され、ローカル DB ありの場合のみ NULL/値あり両分岐を
    実行する。
- 残存課題: なし（task 2 以降の TypeScript 型 / UI 配線は後続 task で対応）。

### Task 2

- 採用方針: `web/src/types/item.ts` の `ItemSearchHit` interface に
  `hatebu_fetched_at: string | null` を必須フィールドとして追加し、既存 `ItemSummary` の
  doc コメント表記（`未取得時はnull`）と整合させた。doc コメントには検索 API が Go 側
  `omitempty` で未取得時にキーを省略する点と、呼び出し側で `undefined ?? null` 正規化する
  方針（design.md Notes for Developers）を併記した。
- 重要な判断:
  - 既存 doc コメントの「省略: `hatebu_fetched_at`（検索 API レスポンスに含まれないため）」は
    削除し、追加フィールドの仕様で置き換えた（design.md の差分指示と整合）。
  - 型を `string | null` の必須プロパティ（optional `?` ではなく）で追加したことで、既存テスト
    fixture（`search-results.test.tsx` / `use-item-search.test.tsx`）の mock object に
    `hatebu_fetched_at: null` 補填が必要になった。CI を壊さないために本 task 内で最小限の
    fixture 修正を加えた。`hatebu_fetched_at` を null にした選択は「既存テスト = 未取得記事の
    分岐挙動」を改変しないことを優先したため。値ありシナリオのテスト追加は Task 6
    （SearchResults 配線時）でカバーされる予定。
  - 既存 `ItemSearchResponse` / 他フィールドの構造・順序・型は不変。新フィールドの配置は
    `hatebu_count` の直後とし、関連フィールドをグルーピングした（design.md の Notes for
    Developers では位置指定なし）。
- 残存課題:
  - tsc baseline で既存の TS エラー（`feed-list.test.tsx` の `onOpenSettings` / `viewMode`、
    `starred-item-list.test.tsx` の `expandedItemId`、`starred-nav-item.test.tsx` の
    `selectedView` / `selectedFeedId`、`rewrites.test.ts` の `NODE_ENV`）が残存している。
    Task 2 と無関係な既存問題のため本 task ではノータッチ。Task 5 / 7 で `ItemList` /
    `ItemDetail` を改修する際にも触れない予定。確認事項として PR レビュワーに共有する必要あり。
  - Task 6（SearchResults 配線）で `hatebu_fetched_at` 値ありケースのテストを追加する想定。
