# Implementation Plan

- [x] 1. 検索 API レスポンスへの `hatebu_fetched_at` 追加（Go 5 レイヤ縦断）
  - `internal/model/item.go` の `ItemSearchHit` に `HatebuFetchedAt *time.Time` を追加する
  - `internal/repository/postgres_item_repo.go` の `SearchByUserAndKeyword` の SELECT 句に `i.hatebu_fetched_at` を追加し、`sql.NullTime` で Scan して `hit.HatebuFetchedAt` に代入する（NULL → nil / 値あり → ポインタ）
  - `internal/itemsearch/service.go` の `ItemSearchSummary` に `HatebuFetchedAt *time.Time` を追加し、`Search` メソッドの summary 組み立てで repo 結果から pass-through する
  - `internal/handler/item_search_handler.go` の `itemSearchHitResponse` に `HatebuFetchedAt *time.Time` json タグ `"hatebu_fetched_at,omitempty"` を追加する
  - `internal/handler/service_adapter.go` の `ItemSearchServiceAdapter.Search` のレスポンス組み立てで `HatebuFetchedAt` をコピーする
  - 既存テスト `internal/handler/item_search_handler_test.go` / `internal/itemsearch/service_test.go` / `internal/repository/postgres_item_repo_search_test.go` を改修し、(a) 取得済み記事のレスポンス JSON に `hatebu_fetched_at` が含まれる (b) 未取得記事では omit される (c) repo / service / handler の各層で pass-through が成立する を検証する
  - WHERE 句 / ORDER BY / LIMIT / 既存フィールド構造は不変であることを担保する（NFR 3.1）
  - _Requirements: 5.1, 5.2, 5.3, 5.4_
  - _Boundary: itemSearchHitResponse, ItemSearchServiceAdapter, ItemSearchSummary, ItemSearchHit, SearchByUserAndKeyword_

- [x] 2. TypeScript 型 `ItemSearchHit` への `hatebu_fetched_at` 追加 (P)
  - `web/src/types/item.ts` の `ItemSearchHit` interface に `hatebu_fetched_at: string | null` を追加する
  - 既存 doc コメント「省略: `hatebu_fetched_at`」の記述を削除し、新フィールドの意味（未取得時は null、取得済みなら RFC3339 文字列）を追記する
  - 既存 `ItemSearchResponse` / 他フィールドの構造は不変であることを担保する
  - _Requirements: 5.1_
  - _Boundary: types/item.ts_
  - _Depends: 1_

- [x] 3. 共通コンポーネント `ItemMetaActions` の新規作成 (P)
  - `web/src/components/item-meta-actions.tsx` を新規作成する
  - Props: `{ itemId: string; isStarred: boolean; hatebuCount: number; hatebuFetchedAt: string | null; onToggleStar: (itemId: string, nextStarred: boolean) => void }`
  - はてブ数表示: `hatebuFetchedAt === null ? "-" : String(hatebuCount)` を `<span data-testid={"item-hatebu-count-" + itemId}>` で表示（`Bookmark` アイコン + 数値）
  - スター⭐️トグル: `Button variant="ghost" size="icon-sm"` + lucide `Star`、`is_starred=true` で `fill-yellow-400 text-yellow-400`、`false` でアウトライン
  - `aria-label`: `isStarred ? "スターを解除する" : "スターを付ける"`、`aria-pressed={isStarred}`、`data-testid={"item-star-toggle-" + itemId}`
  - `onClick`: `e.stopPropagation()` を呼んだ後 `onToggleStar(itemId, !isStarred)` を発火する
  - 32px ヒット領域は `size="icon-sm"`（既存 `ItemDetail` 実装と同等）で確保する
  - `web/src/components/item-meta-actions.test.tsx` を新規作成し、(a) hatebu 表示分岐（null → `-` / 値あり → 数値）(b) star icon の塗り分け (c) クリック時の stopPropagation 呼出と onToggleStar 発火 (d) aria-label / aria-pressed の状態整合 (e) 0 件と「未取得」の区別表示 を検証する
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.1, 2.3, NFR 1.1, NFR 1.2, NFR 1.3, NFR 1.4, NFR 2.1_
  - _Boundary: ItemMetaActions_

- [x] 4. `useToggleStar` の `["item-search"]` キャッシュ拡張
  - `web/src/hooks/use-item-state.ts` の `useToggleStar` を改修する
  - `onMutate`: 既存の `["items"]` 処理に加えて `queryClient.cancelQueries({ queryKey: ["item-search"] })` と `getQueriesData<InfiniteData<ItemSearchResponse>>({ queryKey: ["item-search"] })` のスナップショットを取得し、`setQueryData` で `pages[].items[].is_starred` を反転更新する
  - `onError`: 既存の `previousData` 復元に加えて `["item-search"]` の `previousData` も復元する
  - `onSettled`: 既存の `invalidateQueries({ queryKey: ["items"] })` / `["cross-feed-items"]` に加えて `invalidateQueries({ queryKey: ["item-search"] })` を追加する
  - 既存の `["cross-feed-items"]` invalidate / `["items", "starred"]` の前置キー共有による自動カバーは不変であることを担保する
  - `web/src/hooks/use-item-state.test.ts`（存在しない場合は新規作成）に、(a) `["item-search"]` への楽観反映 (b) `onError` 発火時の `["item-search"]` ロールバック (c) `["items"]` 系の既存挙動非回帰 のテストを追加する
  - _Requirements: 2.1, 2.2, 2.4, 2.5, NFR 2.2, NFR 2.3_
  - _Boundary: useToggleStar_

- [x] 5. `ItemList` / `StarredItemList` への `ItemMetaActions` 配線
  - `web/src/components/item-list.tsx` の `ItemRow` プロパティに `onToggleStar: (itemId: string, nextStarred: boolean) => void` を追加する
  - `ItemRow` 内のタイトル行で、既存の読み取り専用 `Star` アイコン（`data-testid={"star-" + id}`）を削除し、日時表示の右隣に `<ItemMetaActions ... />` を配置する（`hatebuCount={item.hatebu_count}` / `hatebuFetchedAt={item.hatebu_fetched_at}` / `isStarred={item.is_starred}` / `onToggleStar={onToggleStar}` を渡す）
  - `ItemList` 本体で `<ItemRow ... onToggleStar={handleToggleStar} />` を渡す
  - `web/src/components/starred-item-list.tsx` で `<ItemRow ... onToggleStar={handleToggleStar} />` を渡す
  - 既存の `data-testid="item-title-row-${id}"` / 公開日時 / (推定) バッジ / 概要 / 既読薄表示は不変であることを担保する
  - 既存テスト `web/src/components/item-list.test.tsx` / `starred-item-list.test.tsx` を改修し、(a) 一覧行右端に `item-hatebu-count-${id}` と `item-star-toggle-${id}` が出現 (b) スター⭐️トグルクリックで mutation が呼ばれ、`expandedItemId` が変化しない（伝播抑止）(c) `is_starred=true/false` の見た目分岐 (d) 既存無限スクロール / 既読薄表示 / フィルタ / 空状態テストの非回帰 を検証する
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 2.3, 2.5, 4.1, 4.2, 4.5, 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, NFR 3.2_
  - _Boundary: ItemList, StarredItemList, ItemRow_
  - _Depends: 3, 4_

- [x] 6. `SearchResults` への `ItemMetaActions` 配線
  - `web/src/components/search-results.tsx` の `SearchResultRowProps` に `onToggleStar: (itemId: string, nextStarred: boolean) => void` を追加する
  - `SearchResultRow` 内のタイトル行で、既存の読み取り専用 `Star` アイコン（`data-testid={"search-result-star-" + id}`）を削除し、日時表示の右隣に `<ItemMetaActions ... />` を配置する（`hatebuCount={hit.hatebu_count}` / `hatebuFetchedAt={hit.hatebu_fetched_at}` / `isStarred={hit.is_starred}` / `onToggleStar={onToggleStar}` を渡す）
  - `SearchResults` 本体で `<SearchResultRow ... onToggleStar={handleToggleStar} />` を渡す
  - 既存のフィード badge（横断検索のみ favicon + feed_title）/ 日時 `<time>` / (推定) / 既読薄表示 / 概要 / `search-results-loading` / `search-results-error` / `search-results-empty` 出し分けは不変であることを担保する
  - `web/src/components/search-results.test.tsx`（存在しない場合は新規作成）に、(a) 検索結果行で `hatebu_fetched_at === null` のとき `-` を表示 (b) `hatebu_fetched_at` が文字列のとき `hatebu_count` の数値を表示 (c) スター⭐️トグルクリックで `useToggleStar.mutate` が発火し `expandedItemId` が変化しない (d) フィード badge / loading / error / empty の非回帰 を検証する
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 2.3, 2.5, 4.3, 4.4, 5.3, 5.4, 6.1, 6.2, 6.4, 6.5, 6.7, NFR 3.2_
  - _Boundary: SearchResults, SearchResultRow_
  - _Depends: 2, 3, 4_

- [x] 7. `ItemDetail` ヘッダーからのメタ撤去
  - `web/src/components/item-detail.tsx` のタイトル右側 `<div data-testid="item-detail-meta-group">` 全体（`hatebu-count` 表示 + `star-toggle` Button）を削除する
  - タイトル `<h3>` / タイトルリンク / `item-detail-title-row` / 著者表示 / 元記事リンク / 本文サニタイズ + 折りたたみ / 「続きを読む」トグル / 展開時の自動既読化 `useEffect` は不変であることを担保する
  - `ItemDetailProps` の `onToggleStar` prop は型互換維持のため残置するが、本体内で使用しないことを TSDoc コメントで明示する（cleanup は別 Issue）
  - 使われなくなった import（`Star`, `Bookmark`, `Button` のうち本コンポーネントの他用途で使われていないもの）を削除する
  - 既存テスト `web/src/components/item-detail.test.tsx` を改修し、(a) ヘッダー領域に `hatebu-count` / `star-toggle` testid が **存在しない** (b) `item-detail-meta-group` が存在しない (c) タイトル / 著者 / 元記事リンク / 本文表示 / 折りたたみ / 自動既読化テストが全 pass する を検証する
  - _Requirements: 3.1, 3.2, 3.3, 3.4, NFR 3.3_
  - _Boundary: ItemDetail_
  - _Depends: 5, 6_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の構造化ブロックで宣言する。Go 側（`internal/...`）と Web 側（`web/`）の両方を確認する。

<!-- stage-a-verify -->
```sh
cd web && npm test -- --run && npm run lint && cd .. && go test ./... && go vet ./...
```
