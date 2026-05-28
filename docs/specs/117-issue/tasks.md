# Implementation Plan

- [x] 1. Repository 層: `ListStarredByUser` メソッドの追加と DB 結合テスト
  - `internal/repository/interfaces.go` の `ItemRepository` インターフェースに `ListStarredByUser(ctx, userID, cursor, limit)` メソッドを追加する
  - `internal/repository/postgres_item_repo.go` に同メソッドの実装を追加する。SQL は `items i INNER JOIN item_states s ON i.id = s.item_id INNER JOIN feeds f ON i.feed_id = f.id WHERE s.user_id = $1 AND s.is_starred = true [AND i.published_at < $cursor] ORDER BY i.published_at DESC LIMIT $limit` とし、`f.title AS feed_title` を SELECT に含める
  - 戻り値の型は `[]repository.StarredItemRow`（または `[]model.ItemWithStateAndFeed`）として feed_title を保持する構造体を新設する
  - 既存 `idx_item_states_user_starred (user_id, is_starred) WHERE is_starred = true` 部分インデックスを活用できる SQL 形状であることを `EXPLAIN ANALYZE` で確認し、impl-notes.md に貼り付ける（NFR 1.1 / 1.2）
  - コンパイル時 interface 適合チェック（`var _ ItemRepository = (*PostgresItemRepo)(nil)`）を維持する
  - `internal/repository/postgres_item_repo_starred_test.go` を新規作成し、setupListDueTestDB と同じパターンで以下を検証する: (a) 自ユーザーのスター記事のみ返る、(b) 他ユーザーのスター記事が混入しない、(c) `published_at` 降順、(d) 複数フィードにまたがる、(e) cursor 境界、(f) スター 0 件で空スライス、(g) `feed_title` が正しく付与される
  - _Requirements: 2.2, 2.4, 4.1, 4.2, 4.9, 4.10, NFR 1.1, NFR 1.2, NFR 2.1_
  - _Boundary: ItemRepository, PostgresItemRepo_

- [x] 2. Service 層: `ListStarredItems` メソッドの追加と単体テスト
  - `internal/item/service.go` の `ItemService` に `ListStarredItems(ctx, userID, cursorStr, limit)` メソッドを追加する
  - 既存 `ListItems` のカーソルパース・`limit+1` 取得・`has_more` 判定・`next_cursor` 算出・サマリー変換のロジックを**ヘルパー関数として抽出**して再利用する（`parseItemCursor` / `buildItemListResult` 等）。既存 `ListItems` の挙動は変えないこと
  - 横断スター用の `StarredItemSummary`（既存 `ItemSummary` に `FeedTitle string` を追加）を新設し、サービス層レスポンス `StarredItemListResult` を返す
  - 不正カーソル時は `model.NewInvalidFilterError("無効なカーソル値: " + cursorStr)` を返す
  - `internal/item/service_test.go` に以下を追加: (a) 空カーソルで先頭ページ、(b) 不正カーソルで `INVALID_FILTER` エラー、(c) `limit+1` 取得による `has_more=true` 判定、(d) `has_more=false` 時に `NextCursor` が空、(e) `RFC3339Nano` フォーマットの next_cursor
  - _Requirements: 2.2, 4.2, 4.3, 4.4, 4.5, 4.7, 4.8, NFR 3.1_
  - _Boundary: ItemService_
  - _Depends: 1_

- [x] 3. Handler 層: `ListStarredItems` ハンドラ + アダプタ + ルート登録
  - `internal/handler/item_handler.go` の `ItemServiceInterface` に `ListStarredItems(ctx, userID, cursorStr, limit) (*starredItemListResult, error)` を追加する
  - `starredItemSummaryResponse` 型（`itemSummaryResponse` の全フィールド + `FeedTitle string \`json:"feed_title"\``）と `starredItemListResult` 型を新設する
  - `(h *ItemHandler).ListStarredItems` を実装する。`UserIDFromContext` 失敗で 401、`cursor` / `limit` クエリパラメータをパース、`handleServiceError` で `model.APIError` を HTTP status にマップ
  - `internal/handler/service_adapter.go` の `ItemServiceAdapterFromDomain` に `ListStarredItems` を追加し、ドメイン層 `StarredItemListResult` を handler の `starredItemListResult` に変換する
  - `internal/handler/router.go` の `/api/feeds` 配下に `r.Get("/starred/items", itemHandler.ListStarredItems)` を `/{id}` ルートと**同居**して登録する。chi v5 のトライ木は静的セグメントを優先するため `/api/feeds/{id}/items` と衝突しないことをコメントで明記する
  - `internal/handler/item_handler_test.go` に以下を追加: (a) 401（未認証）、(b) 200 正常応答と JSON 形状検証、(c) `?cursor=` パラメータの service 層への伝搬、(d) 不正 cursor で 400、(e) スター 0 件で `items=[]` / `has_more=false`
  - _Requirements: 4.1, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8, 4.10_
  - _Boundary: ItemHandler, ItemServiceAdapterFromDomain, Router_
  - _Depends: 2_

- [x] 4. Handler 層: 結合テスト（既存挙動の非干渉確認込み）
  - `internal/handler/integration_test.go` に新規シナリオを追加する: (a) 認証クッキー付きで `GET /api/feeds/starred/items` を呼び、自ユーザーのスター記事のみが含まれる、(b) 他ユーザーが事前にスターした記事を一切返さない（NFR 2.1）、(c) スター 0 件ユーザーで `200 { items: [], has_more: false }`、(d) 不正 cursor で 400、(e) 未認証で 401
  - 既存 `GET /api/feeds/{id}/items` を `/starred` 追加後も同一フィクスチャで呼び出し、応答が変化しないこと（要件 5.1 / 5.3）を確認する回帰テストを 1 件追加する
  - 既存スター更新エンドポイント `PUT /api/items/{id}/state` の挙動が変化しないこと（要件 5.2）を、既存統合テスト群がそのまま green であることで担保する（追加テスト不要）
  - chi のルーティング優先順位を信頼するのではなく、実 router 経由で `/api/feeds/starred/items` が `ListStarredItems` ハンドラに到達することを統合的に検証する
  - _Requirements: 4.6, 4.7, 4.9, 5.1, 5.2, 5.3, NFR 2.1_
  - _Boundary: ItemHandler, Router, Integration_
  - _Depends: 3_

- [ ] 5. Web: 型定義と `useStarredItems` フック
  - `web/src/types/item.ts` に `StarredItemSummary extends ItemSummary { feed_title: string }` と `StarredItemListResponse` を追加する
  - `web/src/hooks/use-starred-items.ts` を新規作成し、`useInfiniteQuery` で `GET /api/feeds/starred/items?limit=50[&cursor=...]` を呼び出す。queryKey は `["items", "starred"]`（前置キー `"items"` を共有して既存 `useToggleStar` の invalidate に乗る）
  - `web/src/hooks/use-starred-items.test.tsx` を新規作成し、(a) 初回リクエストの URL クエリ検証、(b) `next_cursor` を pageParam として送る、(c) `has_more=false` のとき `getNextPageParam` が `undefined` を返す、を検証する
  - _Requirements: 2.2, 2.5, 3.2, 3.3, 3.4, 4.5, NFR 3.1_
  - _Boundary: useStarredItems_
  - _Depends: 3_

- [ ] 6. Web: AppState 拡張と StarredNavItem コンポーネント (P)
  - `web/src/contexts/app-state.tsx` の `AppState` に `selectedView: "feed" | "starred"` を追加する（初期値 `"feed"`）
  - 新規アクション `SELECT_STARRED` を追加。reducer は `selectedView="starred"`, `selectedFeedId=null`, `expandedItemId=null`, `filter="all"` に遷移する。既存 `SELECT_FEED` も `selectedView="feed"` を設定するよう修正する
  - `web/src/contexts/app-state.test.tsx` に reducer 遷移テストを追加する: (a) 初期 view が `"feed"`、(b) `SELECT_STARRED` で `selectedView` が遷移し他フィールドがリセット、(c) `SELECT_FEED` で `selectedView` が `"feed"` に戻る
  - `web/src/components/starred-nav-item.tsx` を新規作成。`<button>` で「お気に入り」テキスト + `Star` アイコンを表示。クリックで `SELECT_STARRED` を dispatch。`selectedView === "starred"` のとき既存 `feed-list.tsx` と同じアクティブクラス（`bg-accent text-accent-foreground font-medium`）を適用
  - `web/src/components/starred-nav-item.test.tsx` を新規作成し、表示・クリック・アクティブクラス切替を検証する
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_
  - _Boundary: AppState, StarredNavItem_

- [ ] 7. Web: StarredItemList コンポーネントと AppShell 統合 (P)
  - `web/src/components/item-list.tsx` の `ItemRow` コンポーネントを export して再利用可能にする（既存挙動を変えない非破壊的変更）。`ItemDetailArea` も同様に export 候補
  - `web/src/components/starred-item-list.tsx` を新規作成し、`useStarredItems()` で取得した記事を `ItemRow` で表示する。ヘッダに「お気に入り」タイトル（要件 2.1）、各行に `feed_title` を併記（要件 2.4、タイトル直下に薄い文字色で 1 行）、Intersection Observer による無限スクロール（要件 2.5）、空状態「記事がありません」（要件 2.6）、エラー状態「記事の読み込みに失敗しました」（要件 2.7）、`useAppState().expandedItemId` 連携の排他展開（要件 2.8）を実装する
  - `web/src/components/app-shell.tsx` を修正し、左ペイン先頭に `StarredNavItem` を配置、右ペインを `state.selectedView` で `ItemList` と `StarredItemList` に切替える
  - 既存単一フィード動線（フィード行クリック・フィルタタブ・記事詳細展開）が本変更後も変化しないこと（要件 5.3）を、既存 `web/src/components/item-list.test.tsx` / `web/src/components/app-shell.test.tsx` の既存テストがそのまま green であることで担保する
  - `web/src/components/app-shell.test.tsx` に「お気に入り」項目クリック → 右ペイン切替 → フィード行クリックで戻る、のフローテストを追加する
  - `web/src/components/starred-item-list.test.tsx` を新規作成し、空状態・エラー状態・複数行表示と `feed_title` 併記・Intersection Observer による次ページ取得・記事行クリックで `EXPAND_ITEM` dispatch を検証する
  - _Requirements: 1.3, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 3.1, 3.2, 5.3_
  - _Boundary: StarredItemList, AppShell, ItemRow（export）_
  - _Depends: 5, 6_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを構造化ブロックで宣言する。Go バックエンドのテスト・静的解析、および Web フロントエンドのテスト・lint・ビルドを `.github/workflows/ci.yml` のジョブ構成に揃える。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./... && cd web && npm test && npm run lint && npm run build
```
