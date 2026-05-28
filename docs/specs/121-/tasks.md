# Implementation Plan

- [ ] 1. DB マイグレーション追加（`user_cross_feed_views` 表）
  - `internal/database/migrations/20260528120000_add_user_cross_feed_views.up.sql` を新規作成し、`user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE` / `last_seen_at TIMESTAMPTZ NOT NULL` / `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()` を含む CREATE TABLE を記述
  - 対となる `.down.sql` に `DROP TABLE IF EXISTS user_cross_feed_views;` を記述
  - `internal/model/crossfeed.go` を新規作成し `UserCrossFeedView struct { UserID string; LastSeenAt time.Time; UpdatedAt time.Time }` を定義
  - _Requirements: 4.1, 4.5_

- [ ] 2. UserCrossFeedViewRepository の interface 追加と Postgres 実装 (P)
  - `internal/repository/interfaces.go` に `UserCrossFeedViewRepository` interface（`Get(ctx, userID) (*model.UserCrossFeedView, error)` / `Upsert(ctx, userID, lastSeenAt) error`）を追加
  - `internal/repository/postgres_user_cross_feed_view_repo.go` に `PostgresUserCrossFeedViewRepo` 構造体と `NewPostgresUserCrossFeedViewRepo(db)` / `Get` / `Upsert`（`INSERT ... ON CONFLICT (user_id) DO UPDATE SET last_seen_at=EXCLUDED.last_seen_at, updated_at=now()`）を実装
  - compile-time interface check（`var _ UserCrossFeedViewRepository = (*PostgresUserCrossFeedViewRepo)(nil)`）を追加
  - `internal/repository/postgres_user_cross_feed_view_repo_test.go` に integration test を追加（Upsert→Get→再 Upsert→Get で last_seen_at が更新されること）
  - _Requirements: 4.1, 4.3, 4.5_
  - _Boundary: UserCrossFeedViewRepository_
  - _Depends: 1_

- [ ] 3. ItemRepository.ListNewAcrossFeeds の追加（横断 JOIN クエリ実装） (P)
  - `internal/repository/interfaces.go` の `ItemRepository` interface に `ListNewAcrossFeeds(ctx, userID, sinceTime, cursorPublishedAt, cursorItemID, limit) ([]CrossFeedItem, error)` を追加し、`CrossFeedItem struct { model.ItemWithState; FeedTitle string; FaviconData []byte; FaviconMime string }` を新設
  - `internal/repository/postgres_item_repo.go` に `ListNewAcrossFeeds` を実装。`items i JOIN subscriptions s ON s.feed_id=i.feed_id AND s.user_id=$1 JOIN feeds f ON f.id=i.feed_id LEFT JOIN item_states st ON st.item_id=i.id AND st.user_id=$1 WHERE i.published_at > $2 [AND (i.published_at, i.id) < ($3, $4)] ORDER BY i.published_at DESC, i.id DESC LIMIT $N` のクエリを構築（cursor 有無で分岐）
  - `internal/repository/postgres_item_repo_test.go` に integration test 3 件以上を追加: (a) 2 フィード購読 + 6 記事で `sinceTime` 以後の記事が `published_at DESC, id DESC` で取得される、(b) 同一 published_at で id 降順タイブレーク、(c) cursor 指定時に複合キー境界で次ページが正しく返る
  - 既存 `ListByFeed` / `Create` / `Update` 等のメソッド signature を変更しないこと（NFR 1.2 / Req 5.1）
  - _Requirements: 2.1, 2.2, 2.3, 4.2, 5.1, NFR 1.1, NFR 1.2_
  - _Boundary: ItemRepository_
  - _Depends: 1_

- [ ] 4. crossfeed.Service の実装（ドメインロジック）
  - `internal/crossfeed/service.go` を新規作成し `Service struct { itemRepo repository.ItemRepository; userCrossFeedViewRepo repository.UserCrossFeedViewRepository }` と `NewService` を定義
  - `ListNewItems(ctx, userID, cursorStr, limit) (*NewItemsResult, error)` を実装: (1) `userCrossFeedViewRepo.Get` で lastSeen 取得、(2) nil なら `sinceTime = now - 24h` fallback、非 nil なら `sinceTime = lastSeen`、(3) `cursorStr` を `strings.LastIndex(s, ":")` で複合分解、(4) `itemRepo.ListNewAcrossFeeds` を `limit+1` で呼び HasMore 判定、(5) NextCursor を `<RFC3339Nano>:<itemID>` 形式で組み立て、(6) `CrossFeedItemSummary` 配列を返す。`FeedFaviconURL` は `favicon_data` 非空なら `data:<mime>;base64,<encoded>` 形式の data URL を構築（`internal/subscription/service.go` の方式と整合）
  - `TouchLastSeen(ctx, userID) error` を実装: `userCrossFeedViewRepo.Upsert(ctx, userID, time.Now())` を呼ぶ
  - cursorStr 不正形式時は `model.NewInvalidFilterError("invalid cursor: ..." )` を返す（既存エラーコードを再利用）
  - `internal/crossfeed/service_test.go` を新規作成し unit test 4 件以上: (a) lastSeen ありで sinceTime=lastSeen、(b) lastSeen なしで fallback 24h、(c) limit+1 取得で HasMore=true / NextCursor 生成、(d) TouchLastSeen が Upsert を now() で呼ぶ。Repository は mock 実装で代替
  - _Requirements: 2.1, 2.2, 2.3, 4.1, 4.2, 4.3, 4.4, 4.5_
  - _Boundary: crossfeed.Service_
  - _Depends: 2, 3_

- [ ] 5. CrossFeedHandler と ルート登録、DI 配線 (P)
  - `internal/handler/crossfeed_handler.go` を新規作成: `CrossFeedServiceInterface`（`ListNewItems` / `TouchLastSeen`）と `CrossFeedHandler` 構造体、`ListItems`（GET /api/items/cross-feed）/ `TouchLastSeen`（PUT /api/users/me/cross-feed-last-seen）ハンドラを実装。クエリパラメータ `cursor`（省略可）/ `limit`（省略時 50 / 上限 200 にクランプ）をパース。レスポンス DTO `crossFeedItemResponse`（`id, feed_id, feed_title, feed_favicon_url, title, link, summary, published_at, is_date_estimated, is_read, is_starred, hatebu_count`）と `crossFeedListResponse`（`items, next_cursor, has_more, since_time`）を定義。エラーは既存 `handleServiceError` 再利用
  - `internal/handler/service_adapter.go` に `CrossFeedServiceAdapter`（domain `crossfeed.NewItemsResult` → handler DTO 変換）と compile-time interface check を追加
  - `internal/handler/router.go` の `RouterDeps` に `CrossFeedService CrossFeedServiceInterface` を追加し、認証必須グループ内に `r.Route("/api/items/cross-feed", ...)` と `r.Put("/api/users/me/cross-feed-last-seen", ...)` を登録（既存 `r.Route("/api/users", ...)` の Withdraw と同居）
  - `internal/handler/crossfeed_handler_test.go` に handler test 3 件以上: (a) 認証なしで 401、(b) 認証ありで items 配列が返る、(c) PUT 成功で 204
  - `internal/app/app.go` の `runServe` で `userCrossFeedViewRepo := repository.NewPostgresUserCrossFeedViewRepo(db)` / `crossFeedService := crossfeed.NewService(itemRepo, userCrossFeedViewRepo)` / `deps.CrossFeedService = handler.NewCrossFeedServiceAdapter(crossFeedService)` を追加
  - _Requirements: 1.2, 2.1, 4.3, NFR 1.3, NFR 2.1_
  - _Boundary: CrossFeedHandler, CrossFeedServiceAdapter, RouterDeps_
  - _Depends: 4_

- [ ] 6. Frontend: FeedFavicon コンポーネント抽出と AppStateContext 拡張 (P)
  - `web/src/components/feed-favicon.tsx` を新規作成し、既存 `web/src/components/feed-list.tsx` 内 private function `FeedFavicon` を **挙動不変** で切り出す（props 型 `{ feedId: string; faviconURL: string | null; feedTitle: string }` も同じ）。エクスポートは named export
  - `web/src/components/feed-favicon.test.tsx` を新規作成し既存挙動を保護: (a) faviconURL あり時に `<img>` 描画、(b) faviconURL null 時に `Rss` 代替アイコン描画、(c) `<img>` の `onError` で `Rss` に切替
  - `web/src/components/feed-list.tsx` を修正して新規 `feed-favicon.tsx` から import するように変更（内部 private function 定義は削除）
  - `web/src/contexts/app-state.tsx` を拡張: `AppState` に `viewMode: 'none' | 'feed' | 'cross-feed'` を追加（初期値 `'none'`）。新 action `SELECT_ALL_NEW_ITEMS`（viewMode='cross-feed', selectedFeedId=null, expandedItemId=null, filter='all'）を追加し、既存 `SELECT_FEED` は viewMode='feed' + selectedFeedId 設定に拡張
  - `web/src/contexts/app-state.test.tsx` を更新し reducer test を追加: `SELECT_ALL_NEW_ITEMS` で正しい state 遷移、`SELECT_FEED` で viewMode='feed' になること
  - _Requirements: 1.2, 1.3, 3.2, 3.3, 5.1, 5.2_
  - _Boundary: FeedFavicon, AppStateContext_

- [ ] 7. Frontend: FeedList に「すべての新着記事」エントリ追加と FeedList テスト更新
  - `web/src/components/feed-list.tsx` の `FeedList` props に `viewMode: ViewMode` と `onSelectAllNewItems: () => void` を追加（後方互換のため省略可とせず必須化）。`feeds.map` の **直前** に `<AllNewItemsEntry>` 相当の button を常設描画（購読 0 件でも表示。Req 1.1）。選択中時は既存個別フィードと同じ `bg-accent text-accent-foreground font-medium` を適用、`data-testid="all-new-items-entry"` / `aria-current="page"`（選択中時）を付与。favicon 領域は `<FeedFavicon feedId="__all__" faviconURL={null} feedTitle="すべての新着記事" />` で代替アイコン表示
  - 既存購読 0 件時の「フィードが登録されていません」メッセージは購読配列が 0 件かつ仮想エントリ表示後の領域に表示するよう調整（仮想エントリ自体は常時表示、Req 1.1）
  - `web/src/components/feed-list.test.tsx` にテストを追加: (a) 購読 0 件でも仮想エントリが描画、(b) 仮想エントリ click で `onSelectAllNewItems` が呼ばれる、(c) viewMode='cross-feed' のとき仮想エントリが `data-selected="true"`、(d) 既存個別フィード行の並び順・スタイルが維持されていること
  - _Requirements: 1.1, 1.4, 1.5, 5.2, NFR 2.1, NFR 3.1_
  - _Boundary: FeedList_
  - _Depends: 6_

- [ ] 8. Frontend: useCrossFeedItems / useTouchCrossFeedLastSeen フックと型定義 (P)
  - `web/src/types/crossfeed.ts` を新規作成し `CrossFeedItem`（既存 `ItemSummary` フィールド + `feed_id, feed_title, feed_favicon_url`）と `CrossFeedListResponse`（`items, next_cursor, has_more, since_time`）を定義
  - `web/src/hooks/use-cross-feed-items.ts` を新規作成: `useCrossFeedItems()` で `useInfiniteQuery({ queryKey: ['cross-feed-items'], queryFn: ({ pageParam }) => apiClient.get<CrossFeedListResponse>('/api/items/cross-feed?cursor=...&limit=50'), getNextPageParam: (last) => last.has_more ? last.next_cursor : undefined, initialPageParam: null, staleTime: 0 })` を実装。`useTouchCrossFeedLastSeen()` で `useMutation({ mutationFn: () => apiClient.put('/api/users/me/cross-feed-last-seen') })` を実装
  - `web/src/hooks/use-cross-feed-items.test.tsx` に hook test 2 件以上: (a) infinite query が `/api/items/cross-feed` を呼ぶ、(b) `useTouchCrossFeedLastSeen` の mutate が PUT を送る（モック確認）
  - `web/src/hooks/use-item-state.ts` の `useMarkAsRead` / `useToggleStar` を **修正** し、`onSuccess` / `onSettled` で既存 `queryClient.invalidateQueries({ queryKey: ['items'] })` に加え `queryClient.invalidateQueries({ queryKey: ['cross-feed-items'] })` を追加（Req 5.3 同期）
  - _Requirements: 2.5, 5.3, 5.4, NFR 1.3, NFR 2.2_
  - _Boundary: useCrossFeedItems, useTouchCrossFeedLastSeen, useMarkAsRead, useToggleStar_
  - _Depends: 5_

- [ ] 9. Frontend: CrossFeedItemList コンポーネントと AppShell 切替配線
  - `web/src/components/cross-feed-item-list.tsx` を新規作成: `useCrossFeedItems()` から無限スクロール（IntersectionObserver、既存 `item-list.tsx` と同 pattern）でデータ取得。記事行は既存 `ItemList` の `ItemRow` 相当（タイトル / 公開日時 / summary / 既読・スター）に **フィード badge**（`<FeedFavicon faviconURL={item.feed_favicon_url} ...>` + `<span>{item.feed_title}</span>`）を **左 16px favicon + フィード名 small font** で併記。展開／既読化／スターは `AppStateContext` の `expandedItemId` と既存 `useMarkAsRead` / `useToggleStar` を流用。空状態（first page items が 0 件）時に「新着記事はありません」を表示（Req 4.6）。マウント時に `useEffect` で初回データ受信完了後 1 回だけ `useTouchCrossFeedLastSeen().mutate()` を呼ぶ（Req 4.3）。フィルタ tabs は表示しない（Non-Goals）
  - `web/src/components/cross-feed-item-list.test.tsx` に component test 4 件以上: (a) 0 件返却時に空状態メッセージ、(b) 初回データ受信後に touch mutation が 1 回だけ呼ばれる、(c) 各記事行に feed_title と FeedFavicon が描画される、(d) 既読・スター操作が既存 `useMarkAsRead` / `useToggleStar` に伝播する
  - `web/src/components/app-shell.tsx` を修正: `useAppState()` から `viewMode` を取得し、viewMode='cross-feed' のとき `<CrossFeedItemList />`、それ以外（viewMode='feed' or 'none'）は既存 `<ItemList feedId={selectedFeedId} ... />` を描画。`FeedList` に `viewMode` と `onSelectAllNewItems={() => dispatch({ type: 'SELECT_ALL_NEW_ITEMS' })}` を渡す。`handleSelectFeed` は既存どおり `dispatch({ type: 'SELECT_FEED', feedId })`
  - `web/src/components/app-shell.test.tsx` を更新: (a) viewMode='cross-feed' で CrossFeedItemList が描画、viewMode='feed' で ItemList が描画されること、(b) viewMode='none' のとき既存「フィードを選択してください」表示が維持されること（Req 5.1 非リグレッション）
  - _Requirements: 1.2, 1.3, 2.4, 2.5, 3.1, 3.2, 3.3, 3.4, 4.3, 4.6, 5.1, 5.4, NFR 3.2_
  - _Boundary: CrossFeedItemList, AppShell_
  - _Depends: 7, 8_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の構造化ブロックで宣言する。Go バックエンド（`go test ./...` / `go vet ./...`）と Frontend（`npm test` / `npm run lint`）の組み合わせは `.github/workflows/ci.yml` の backend / go-vet / frontend / frontend-lint ジョブと整合する。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./... && (cd web && npm test) && (cd web && npm run lint)
```
