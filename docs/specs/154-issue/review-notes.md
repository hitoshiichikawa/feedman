# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-30T14:50:00Z -->

## Reviewed Scope

- Branch: claude/issue-154-impl-issue
- HEAD commit: 109ca62d3f0c899a43605639672612aef7a21be2
- Compared to: develop..HEAD
- Commits: 22 件（feat 4 / refactor 1 / test 1 / docs 16）
- Files changed: 25 ファイル（+1857 / -505）

## Verified Requirements

### Requirement 1: 一覧行右端メタ表示

- 1.1 — `web/src/components/item-meta-actions.tsx` (ItemMetaActions) を ItemRow / SearchResultRow のタイトル行右端に配置 (item-list.tsx L397-403, search-results.tsx L347-353)
- 1.2 — item-meta-actions.tsx L65-72（Bookmark + 数値）が L77-97 の Star Button より JSX 上左側に配置（hatebu → star 順）
- 1.3 — item-meta-actions.tsx L44 `hatebuFetchedAt === null ? "-" : String(hatebuCount)` / item-meta-actions.test.tsx の境界テスト (`0` + null → `-`)
- 1.4 — 同上分岐の else 側 / item-meta-actions.test.tsx で `0` + 取得済み → `0` 表示確認
- 1.5 — item-meta-actions.tsx L92-94 `fill-yellow-400 text-yellow-400` 切替 / item-meta-actions.test.tsx の状態テスト
- 1.6 — item-meta-actions.tsx L94 `text-muted-foreground` アウトライン + Button は常時 enabled
- 1.7 — item-list.tsx L381-392 / search-results.tsx L325-340 で既存の `<time>` / `(推定)` / `<summary>` 構造が完全維持

### Requirement 2: 一覧上スター⭐️トグル操作

- 2.1 — item-meta-actions.tsx L54-57 `onToggleStar(itemId, !isStarred)` → useToggleStar.mutate へ
- 2.2 — use-item-state.ts L79-122 `onMutate` 内で同期的に `setQueryData` 実行（楽観更新）
- 2.3 — item-meta-actions.tsx L55 `e.stopPropagation()` / item-list.test.tsx / search-results.test.tsx に伝播抑止テスト
- 2.4 — use-item-state.ts L124-136 `onError` で `previousItems` / `previousSearch` を復元 / use-item-state.test.tsx でロールバックテスト
- 2.5 — use-item-state.ts L137-143 `onSettled` で `["items"]` / `["item-search"]` / `["cross-feed-items"]` を invalidate

### Requirement 3: 詳細ヘッダーからのメタ撤去

- 3.1 — item-detail.tsx L85-125 の新ヘッダー構造に `hatebu-count` が存在しない / item-detail.test.tsx に non-existence 検証 (`#154` describe)
- 3.2 — 同上、`star-toggle` / `item-detail-meta-group` も存在しない
- 3.3 — item-detail.tsx タイトル/著者/元記事リンク/本文 すべて維持 (L88-145) / item-detail.test.tsx の維持確認テスト
- 3.4 — item-detail.tsx L47-53 `useEffect` で `onMarkAsRead` 自動既読化を維持

### Requirement 4: 3 一覧への適用統一

- 4.1 — item-list.tsx L161-166 で `ItemRow` に `onToggleStar={handleToggleStar}` を渡す（通常一覧）
- 4.2 — starred-item-list.tsx L133-136 で同様（スター横断一覧）
- 4.3 — search-results.tsx L149-155 で `SearchResultRow` に渡す（検索結果一覧）
- 4.4 — search-results.tsx L294-305 の `SearchResultFavicon` + `feed_title` バッジ / L325-340 の `<time>` 維持
- 4.5 — starred-item-list.tsx の `starred-item-feed-title-${id}` 行表示は不変

### Requirement 5: 検索 API への hatebu_fetched_at 追加

- 5.1 — internal/handler/item_search_handler.go L72-75 `HatebuFetchedAt *time.Time` json タグ `"hatebu_fetched_at,omitempty"` / item_search_handler_test.go に取得済み記事のレスポンス JSON 検証 / model/item.go / itemsearch/service.go / repository/postgres_item_repo.go の 5 レイヤ縦断 pass-through
- 5.2 — 既存フィールド（FaviconURL, FeedTitle 等）は不変。item_search_handler_test.go の既存検証も pass
- 5.3 — search-results.tsx L351 `hit.hatebu_fetched_at ?? null` で正規化 → ItemMetaActions が `-` 判定 / search-results.test.tsx で検証
- 5.4 — 同上分岐の else 側で数値表示 / search-results.test.tsx の境界テスト (0 件・取得済み多数の 2 ケース)

### Requirement 6: 既存挙動の非回帰

- 6.1 — item-list.tsx L357 `item.is_read && "opacity-60"` / search-results.tsx L289 維持
- 6.2 — item-list.tsx L92-113 / starred-item-list.tsx / search-results.tsx L92-100 IntersectionObserver 維持
- 6.3 — item-list.tsx の filter props（外部から受領）不変
- 6.4 — item-list.tsx L371 / search-results.tsx L316 `<a onClick={(e) => e.stopPropagation()}>` 維持
- 6.5 — item-list.tsx L383-391 / search-results.tsx L331-339 `(推定)` バッジ維持
- 6.6 — StarredItemList のヘッダ「お気に入り」テキスト不変
- 6.7 — search-results.tsx の loading/error/empty 出し分け不変（L102-138）

### Non-Functional Requirements

- NFR 1.1 — item-meta-actions.tsx L46, 82 `aria-label={starLabel}` 動的切替
- NFR 1.2 — item-meta-actions.tsx L83 `aria-pressed={isStarred}` / item-meta-actions.test.tsx の aria 状態テスト
- NFR 1.3 — item-meta-actions.tsx L80 `size="icon-sm"` (= size-8 = 32px) / item-meta-actions.test.tsx で `data-size="icon-sm"` と `size-8` class を 2 重に確認
- NFR 1.4 — native `<button>` のため Tab/Enter/Space 標準挙動 (button.test.tsx で別途担保)
- NFR 2.1 — item-meta-actions.tsx L55 `e.stopPropagation()` / 各一覧 test で `expandedItemId` 不変を検証
- NFR 2.2 — use-item-state.ts L79-122 `onMutate` 同期実行（setQueryData は同期）
- NFR 2.3 — use-item-state.ts L124-136 `onError` 同期実行（setQueryData は同期）
- NFR 3.1 — `itemSummaryResponse` / `starredItemSummaryResponse`（service_adapter.go L129-141, L161-176）は不変、`itemSearchHitResponse` も既存フィールド保持
- NFR 3.2 — 旧 `star-${id}` / `search-result-star-${id}` を撤去し、`item-star-toggle-${id}` で後継 (item-meta-actions.tsx L81)
- NFR 3.3 — 詳細側 `star-toggle` / `hatebu-count` 撤去 + 一覧側は `item-` prefix で衝突回避

## Test Execution Results

- `go test ./internal/handler/... ./internal/itemsearch/...` — pass（cached）
- `go vet ./...` — pass（出力なし）
- `cd web && npm test -- --run` — **40 files / 389 tests passed**（impl-notes Task 7 の記載と一致）

## Boundary Check

- Task 5 の `_Boundary:_` は `ItemList, StarredItemList, ItemRow` だが `cross-feed-item-list.tsx` も 1 行 (`onToggleStar={handleToggleStar}`) 追加されている。ItemRow の `onToggleStar` を required prop にしたため必然の波及で、impl-notes Task 5「`cross-feed-item-list.tsx` への波及」節で明示済み。pass-through のみで挙動変更なし
- Task 7 の `_Boundary:_` は `ItemDetail` のみだが `item-list.test.tsx` の 2 件のテストが更新されている。これは ItemDetail 内の撤去 testid (`star-toggle` / `hatebu-count`) を参照していた既存テストを意味論に追従させた test fixup で、impl-notes Task 7「`item-list.test.tsx` への波及」節で明示済み。実装コードは不変
- 上記 2 件はいずれも spec の必然的副作用として impl-notes に判断理由が記録されており、AC を満たすために必要かつ最小限の変更。boundary 逸脱には該当しない

## Findings

なし

## Summary

Issue #154 の全 7 task が完遂しており、6 つの Requirement と 3 つの NFR すべてに対応する
実装とテストが揃っている。Go 側 5 レイヤ縦断 (`HatebuFetchedAt` の model/repo/service/
handler/adapter pass-through) は test-driven で担保、Web 側は新規 `ItemMetaActions` 共通化
+ `useToggleStar` の `["item-search"]` キャッシュ拡張 + ItemDetail ヘッダーからのメタ撤去が
一貫した設計で実装されている。Task 5/7 の boundary 軽微逸脱（`cross-feed-item-list.tsx`
1 行追加 / `item-list.test.tsx` 2 件 fixup）は impl-notes に判断理由が記録されており、
ItemRow required prop 化に伴う必然の pass-through / 撤去 testid 参照の test fixup で
あって挙動変更を伴わないため許容範囲。`<button>` ネスト警告は既存設計由来で別 Issue 化
方針が記録済み（lint / 既存設計の問題であり 3 カテゴリのいずれにも該当しない）。

RESULT: approve
