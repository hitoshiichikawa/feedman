# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-117-impl-issue
- HEAD commit: 58e25fa4cea3406d72b479b6cc9a209fb8054edf
- Compared to: develop..HEAD

## Verified Requirements

### Requirement 1（左ペイン「お気に入り」ナビゲーション項目）

- 1.1 — `web/src/components/starred-nav-item.tsx` を `app-shell.tsx` の左ペイン先頭に `px-2 pt-2` で常時 1 件配置。`web/src/components/app-shell.test.tsx`「左ペイン先頭に StarredNavItem ...」と `web/src/components/starred-nav-item.test.tsx`「「お気に入り」テキストと Star アイコン...」でカバー
- 1.2 — `starred-nav-item.tsx` がベース／hover／active クラス `bg-accent text-accent-foreground font-medium` を `feed-list.tsx` と完全一致で適用。`starred-nav-item.test.tsx`「クリックすると ... 'starred' に遷移」「初期状態でアクティブクラスが付与されない」で表示規約をカバー
- 1.3 — `SELECT_STARRED` reducer 遷移により `selectedView` が `"starred"` に切替、`app-shell.tsx` が `state.selectedView === "starred"` で右ペインを `StarredItemList` に切替える。`app-shell.test.tsx`「「お気に入り」項目クリック → 右ペインが StarredItemList に切替 ...」でカバー
- 1.4 — `app-state.tsx` の `SELECT_FEED` reducer が `selectedView="feed"` も併せて設定。`app-state.test.tsx`「SELECT_STARRED の後に SELECT_FEED を dispatch すると selectedView が 'feed' に戻ること」と `app-shell.test.tsx` のフローテストでカバー
- 1.5 — `SELECT_STARRED` で `selectedFeedId=null` に遷移。既存 `feed-list.tsx` は `selectedFeedId === null` のときどのフィード行もアクティブにしない既存挙動を活用（構造的担保）

### Requirement 2（右ペイン表示）

- 2.1 — `starred-item-list.tsx` ヘッダに `<h2 data-testid="starred-item-list-title">お気に入り</h2>`。`starred-item-list.test.tsx`「ヘッダにコンテキストタイトル「お気に入り」を表示すること」でカバー
- 2.2 — `ListStarredByUser` SQL の `ORDER BY i.published_at DESC` + `useStarredItems` フックで実現。`postgres_item_repo_starred_test.go`「自ユーザーの複数フィードのスター記事が降順で返り feed_title が付与される」で順序を検証
- 2.3 — `ItemRow` を `item-list.tsx` から `export function` 化（既存呼び出し側に影響なし）し `starred-item-list.tsx` で再利用。`starred-item-list.test.tsx`「複数フィードのスター記事と各行の feed_title を併記して表示する」でカバー
- 2.4 — 各 `ItemRow` の直後に `feed_title` を `text-muted-foreground text-xs truncate` で 1 行併記。同テストで feed_title 表示文字列とクラスを検証
- 2.5 — Intersection Observer + sentinel + `useStarredItems().fetchNextPage`。`starred-item-list.test.tsx`「sentinel が visible になったとき次ページを fetch し cursor を付与すること」でカバー
- 2.6 — `allItems.length === 0 && !isError` で `data-testid="starred-item-list-empty"`「記事がありません」描画。`starred-item-list.test.tsx`「記事 0 件のときに空状態 ...」でカバー（エラー状態との同時非表示も検証）
- 2.7 — `isError` で `data-testid="starred-item-list-error"`「記事の読み込みに失敗しました」（`text-destructive` 色）。`starred-item-list.test.tsx`「API 取得に失敗したときエラー状態 ...」でカバー（空状態との同時非表示も検証）
- 2.8 — クリックで `EXPAND_ITEM` dispatch + `ItemDetailArea` を行直下に展開。`starred-item-list.test.tsx`「記事行クリックで EXPAND_ITEM が dispatch されて expandedItemId が更新される」でカバー

### Requirement 3（スター解除と一覧反映）

- 3.1 — 既存 `useToggleStar` mutation を `starred-item-list.tsx` から `ItemDetailArea` 経由で呼び出す（既存挙動を変更していない）
- 3.2 — `useStarredItems` の queryKey が `["items", "starred"]`。既存 `useToggleStar.onSettled` の `invalidateQueries({ queryKey: ["items"] })` が prefix match で横断キャッシュも自動 invalidate（`use-starred-items.ts` のコメントと design.md で明示。構造的担保）
- 3.3 — TanStack Query の `refetchOnMount` 既定挙動と上記 prefix invalidate により、他フィードへ切り替え後に「お気に入り」を再選択した際は最新一覧を再取得（構造的担保）
- 3.4 — `["items"]` prefix invalidate により、単一フィード画面でのスター解除も横断キャッシュを invalidate（構造的担保）

### Requirement 4（API）

- 4.1 — `ListStarredByUser` SQL の `WHERE s.user_id = $1 AND s.is_starred = true`。`postgres_item_repo_starred_test.go`「他ユーザーのスター記事は返らない」、`integration_test.go`「TestIntegration_ListStarredItems_OnlyOwnStarredItems」「TestIntegration_ListStarredItems_NoOtherUsersItemsLeaked」でカバー
- 4.2 — `ORDER BY i.published_at DESC`。repository / integration テストで降順を検証
- 4.3 — `starredItemListResult`（items / next_cursor / has_more）が既存 `itemListResult` と同形。`item_handler_test.go`「TestItemHandler_ListStarredItems_Success」「TestItemHandler_ListStarredItems_EmptyResult」でカバー
- 4.4 — `parseItemCursor("")` がゼロ値を返し SQL の cursor 条件が省略される。`service_test.go`「TestItemService_ListStarredItems_EmptyCursor」、`postgres_item_repo_starred_test.go`「cursor 指定時に当該時刻より前の記事のみ返る」内の cursor=zero 検証でカバー
- 4.5 — `parseItemCursor` の RFC3339Nano → RFC3339 フォールバックパース。`service_test.go`「TestItemService_ListStarredItems_HasMoreTrue」「TestItemService_ListStarredItems_NextCursorRFC3339NanoFormat」、`integration_test.go`「TestIntegration_ListStarredItems_CursorPropagation」でカバー
- 4.6 — `UserIDFromContext` 失敗で 401。`item_handler_test.go`「TestItemHandler_ListStarredItems_NoUserID_ReturnsUnauthorized」、`integration_test.go`「TestIntegration_ListStarredItems_Unauthorized_Returns401」と `TestIntegration_ProtectedEndpoints_RequireAuth` への `/api/feeds/starred/items` 追加でカバー
- 4.7 — `result.Items == nil` 時に handler 層で `[]starredItemSummaryResponse{}` に正規化。`item_handler_test.go`「TestItemHandler_ListStarredItems_EmptyResult」「TestItemHandler_ListStarredItems_EmptyResult_NilItems」、`integration_test.go`「TestIntegration_ListStarredItems_EmptyResult」でカバー
- 4.8 — `parseItemCursor` 失敗で `model.NewInvalidFilterError` → `handleServiceError` で 400。`service_test.go`「TestItemService_ListStarredItems_InvalidCursor」、`item_handler_test.go`「TestItemHandler_ListStarredItems_InvalidCursor_ReturnsBadRequest」、`integration_test.go`「TestIntegration_ListStarredItems_InvalidCursor_Returns400」でカバー
- 4.9 — SQL `s.user_id = $1` strict invariant。repository / integration の cross-user 検証でカバー
- 4.10 — `model.ItemWithState` embed により `FeedID` を含み、追加で `f.title AS feed_title` を SELECT。repository / handler / integration テストで `feed_id` / `feed_title` の存在を検証

### Requirement 5（既存挙動への非干渉）

- 5.1 — `ListByFeed` SQL / `ListItems` handler の signature・実装は無変更。`ListItems` service は `parseItemCursor` / `buildItemListResult` ヘルパー抽出を経由するよう書き換えられたが、ヘルパーは既存ロジックを意味的に等価のまま切り出したもので、対応するサービス単体テスト群（`TestItemService_ListItems_*`）が green を維持する diff になっている。さらに `integration_test.go`「TestIntegration_ListItems_ByFeedID_StillWorksAfterStarredRouteAdded」で `/api/feeds/{id}/items` のディスパッチが既存挙動のまま動作することを担保
- 5.2 — `PUT /api/items/{id}/state` 関連コードは無変更。既存 `TestIntegration_ItemStateManagement` が引き続き green であることで担保（tasks.md 注記どおり追加テスト不要）
- 5.3 — `feed-list.tsx` は無変更、`item-list.tsx` は `ItemRow` / `ItemDetailArea` への `export` 追加のみで実装無変更、フィルタタブ実装も無変更。`app-shell.test.tsx` のフローテスト「フィード行クリック → 右ペインが ItemList に戻る」「フィルタタブ "全て" が表示される」でカバー

### Non-Functional Requirements

- NFR 1.1 — `impl-notes.md` Task 1 に `EXPLAIN ANALYZE` 結果（5000 記事 / 100 ユーザー fixture で cursor なし 1.352 ms / cursor あり 0.290 ms）を貼付。単一フィード API と同水準を確認
- NFR 1.2 — `EXPLAIN ANALYZE` で `Bitmap Index Scan on idx_item_states_user_starred` が選択されることを確認。マイグレーション追加なし（既存部分インデックスを破壊しない）
- NFR 2.1 — SQL `s.user_id = $1` + `TestIntegration_ListStarredItems_NoOtherUsersItemsLeaked`（他ユーザー専用記事が一切混入しないことを strict 検証）でカバー
- NFR 3.1 — `starredItemListResult` が `itemListResult` と同形、`starredItemSummaryResponse` が `itemSummaryResponse` を struct embed して `feed_title` のみ追加、JSON 上 items が常に `[]` で null にならないことを `TestItemHandler_ListStarredItems_EmptyResult_NilItems` で担保。既存 `ItemSummary` / `itemSummaryResponse` 型は完全に無変更（前者は新規 `StarredItemSummary` が `extends` で拡張）

## Boundary 検証

- Task 1（`_Boundary: ItemRepository, PostgresItemRepo_`）: `internal/repository/interfaces.go` / `postgres_item_repo.go` / 新規 `postgres_item_repo_starred_test.go` のみ → OK
- Task 2（`_Boundary: ItemService_`）: `internal/item/service.go` / `service_test.go` / `upsert_test.go`（既存 mock の interface 充足のためのスタブ追加） → OK
- Task 3（`_Boundary: ItemHandler, ItemServiceAdapterFromDomain, Router_`）: `internal/handler/item_handler.go` / `service_adapter.go` / `router.go` / `item_handler_test.go` のみ → OK
- Task 4（`_Boundary: ItemHandler, Router, Integration_`）: `internal/handler/integration_test.go` のみ → OK
- Task 5（`_Boundary: useStarredItems_`）: `web/src/hooks/use-starred-items.ts` / `use-starred-items.test.tsx` / `web/src/types/item.ts`（`StarredItemSummary` / `StarredItemListResponse` の追加のみで既存型無変更） → OK
- Task 6（`_Boundary: AppState, StarredNavItem_`）: `web/src/contexts/app-state.tsx` / `app-state.test.tsx` / `starred-nav-item.tsx` / `starred-nav-item.test.tsx` のみ → OK
- Task 7（`_Boundary: StarredItemList, AppShell, ItemRow（export）_`）: `starred-item-list.tsx` / `starred-item-list.test.tsx` / `app-shell.tsx` / `app-shell.test.tsx` / `item-list.tsx`（`ItemRow` と `ItemDetailArea` を export 化するのみで実装変更なし。tasks.md 詳細項目で「`ItemDetailArea` も同様に export 候補」と明示的に許可されている範囲） → OK

境界外への変更は検出されなかった。

## Findings

なし

## Summary

Issue #117 の全 numeric AC（Req 1.1〜1.5 / 2.1〜2.8 / 3.1〜3.4 / 4.1〜4.10 / 5.1〜5.3 / NFR 1.1, 1.2, 2.1, 3.1）に対応する実装とテストが揃っており、tasks.md の各 `_Boundary:_` 範囲を逸脱する変更も検出されなかった。既存 `ItemSummary` / `itemSummaryResponse` / `ListByFeed` / `ListItems` の signature・スキーマは完全に保たれており、ヘルパー抽出（`parseItemCursor` / `toItemSummary` / `buildItemListResult`）も `ListItems` の挙動を意味的に変えない安全なリファクタである。

RESULT: approve
