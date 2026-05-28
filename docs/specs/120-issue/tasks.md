# Implementation Plan

- [x] 1. pg_trgm 拡張と GIN インデックスを追加するマイグレーションを作成する
  - `internal/database/migrations/20260528120000_add_item_search_indexes.up.sql` を新規作成し、
    `CREATE EXTENSION IF NOT EXISTS pg_trgm;` を先頭に追加する
  - 同 up.sql に `CREATE INDEX idx_items_title_trgm ON items USING GIN (title gin_trgm_ops);` を追加する
  - 同 up.sql に `CREATE INDEX idx_items_content_trgm ON items USING GIN (content gin_trgm_ops) WHERE content IS NOT NULL;` を追加する
  - 対応する `20260528120000_add_item_search_indexes.down.sql` で `DROP INDEX IF EXISTS` を 2 本記述する
    （`pg_trgm` 拡張は他用途で使われる可能性があるため `DROP EXTENSION` は行わない）
  - _Requirements: NFR 1.2_

- [x] 2. 検索ドメイン用のモデルとエラーコードを追加する
- [x] 2.1 `internal/model/item.go` に `ItemSearchHit` 構造体を追加する
  - design.md の Data Models 節で定義した `ItemSummary` 相当のフィールド + `FeedTitle string` +
    `FaviconData []byte` + `FaviconMime string` を持たせる
  - 既存 `ItemWithState` パターンと整合する命名・並び順にする
  - _Requirements: 4.2_
- [x] 2.2 `internal/model/errors.go` に検索向けのエラーコードと生成関数を追加する
  - `ErrCodeInvalidSearchQuery = "INVALID_SEARCH_QUERY"` および
    `ErrCodeFeedNotSubscribed = "FEED_NOT_SUBSCRIBED"` を定数に追加
  - `NewInvalidSearchQueryError(reason string) *APIError`（Category: "validation"）と
    `NewFeedNotSubscribedError(feedID string) *APIError`（Category: "authorization"）を追加
  - `internal/handler/feed_handler.go` の `mapAPIErrorToHTTPStatus` に
    `ErrCodeInvalidSearchQuery → http.StatusBadRequest` および
    `ErrCodeFeedNotSubscribed → http.StatusForbidden` のエントリを追加
  - _Requirements: 3.3, 3.5, 4.5_

- [x] 3. Repository 層で検索 SQL を実装する
- [x] 3.1 `internal/repository/interfaces.go` に `ItemSearchRepository` インターフェースを追加する
  - design.md の `SearchByUserAndKeyword(ctx, userID, pattern string, feedID *string, cursorID string, cursorPublishedAt time.Time, limit int)` シグネチャに従う
  - `PostgresItemRepo` が当該インターフェースを満たすことを compile-time check に追加する
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.6, 3.1, 3.2, 3.4, 4.1, 4.2_
- [x] 3.2 `internal/repository/postgres_item_repo.go` に `SearchByUserAndKeyword` を実装する
  - design.md の参照 SQL（`items JOIN subscriptions ... JOIN feeds ... LEFT JOIN item_states`、
    `ILIKE $2`、`($3::uuid IS NULL OR i.feed_id = $3)` の任意フィルタ、
    `(published_at, id) < (cursor)` のタプル比較ページング、`ORDER BY published_at DESC, id DESC`）
    を実装する
  - `feedID == nil` の場合は SQL の `$3` パラメータに `nil`（database/sql の NULL 渡し）を
    与える。`feedID != nil` の場合は `*feedID` を渡す
  - `cursorPublishedAt` がゼロ値の場合は cursor 条件を WHERE から外す（既存 `ListByFeed` の慣習に準拠）
  - 既存 `scanItem` / `itemSelectColumns` のパターンを参考にしつつ、本 SQL は SELECT 列が異なるため
    専用の scanner を用意する
  - 実装は SELECT 専用であり、items / item_states に UPDATE / INSERT を行わない（Req 5.3 の不変条件）
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.6, 3.1, 3.2, 3.4, 4.1, 4.2, 5.3_
- [x] 3.3 Repository 層の DB 結合テスト `internal/repository/postgres_item_repo_search_test.go` を追加する
  - 既存 `postgres_subscription_repo_db_test.go` の Skip ガードパターン
    （`TEST_DATABASE_URL` 未設定時は `t.Skip`）を踏襲する
  - ケース（横断検索）: タイトル一致 / 本文一致 / 両方一致 / どちらも不一致 / 大文字小文字差 /
    他ユーザーの購読記事は返らない / 解除済みフィードの記事は返らない / 同 published_at の id 安定ソート
  - ケース（フィード内検索）: `feedID != nil` で当該フィードの記事のみ返ること、同ユーザー購読中の
    他フィードの記事が混入しないこと（Req 2.2）
  - 検索実行前後で対象記事の `item_states.is_read` / `is_starred` が変化しないことを検証する（Req 5.3）
  - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.6, 3.1, 3.2, 3.4, 4.1, 5.3_

- [x] 4. ドメインサービス `internal/itemsearch.SearchService` を実装する
  - `internal/itemsearch/service.go` を新規作成し、design.md の `SearchService.Search(ctx, userID, rawQuery string, feedID *string, cursorStr string, limit int)` シグネチャ・正規化
    ロジック（前後空白 trim、空クエリ判定、LIKE メタ文字エスケープ）・`feedID != nil` 時の購読確認
    （既存 `SubscriptionRepository.Exists` 相当の経路を再利用、未購読なら `NewFeedNotSubscribedError`
    を返す）・`limit+1` 取得→HasMore 判定・NextCursor 生成（`<RFC3339Nano>|<uuid>` 形式）を実装する
  - cursor 形式不正時は `model.NewInvalidSearchQueryError` を返す
  - `SearchService` は `ItemSearchRepository` に加えて購読確認に必要な repository（`SubscriptionRepository`
    の Exists 相当メソッド、未存在なら追加）を依存に取る
  - `internal/itemsearch/service_test.go` を新規作成し、テーブル駆動で正規化ロジック・空クエリ・
    cursor 不正・`feedID` 未購読 → 403・`feedID` 購読あり → repository 呼び出し・limit 上限を検証する。
    repository はモックを差し替える
  - _Requirements: 1.5, 2.4, 2.6, 3.5, 4.1, 4.4_

- [x] 5. Handler `/api/items/search` を実装しルーターに登録する
- [x] 5.1 `internal/handler/item_search_handler.go` を新規作成する
  - `ItemSearchServiceInterface` を定義し、`Search(ctx, userID, rawQuery string, feedID *string, cursorStr string, limit int) (*itemSearchResponse, error)` を持たせる
  - HTTP ハンドラは既存 `feed_handler.go` / `item_handler.go` の `UserIDFromContext` パターンを
    踏襲し、401/400/403/500 を `handleServiceError` で返す
  - クエリパラメータ `feed_id` を受け取り、UUID パース成功なら `*string` として service に渡す。
    パース失敗は 400 INVALID_SEARCH_QUERY
  - レスポンス型 `itemSearchResponse` を `itemSearchHitResponse` 配列で持ち、design.md の JSON
    フィールド名（`feed_title` / `favicon_url`）に従う。`favicon_url` は `*string` で
    `omitempty` を付ける（subscription_handler.go と同じ流儀）
  - NFR 3.1 の構造化ログ（`slog.Info("item search request", user_id, search_type, scope, query_len, feed_id)`）を発行する
  - _Requirements: 1.3, 1.4, 3.3, 3.5, 4.4, 4.5, NFR 3.1_
- [x] 5.2 `internal/handler/router.go` に `/api/items/search` ルートを登録する
  - `RouterDeps` に `ItemSearchService ItemSearchServiceInterface` を追加する
  - **`/api/items/{id}` よりも前**にルートを登録する（chi の static 優先順位を担保）
  - _Requirements: 1.3, 1.4, 3.3_
- [x] 5.3 `internal/handler/service_adapter.go` に `ItemSearchServiceAdapter` を追加する
  - `itemsearch.SearchService` の戻り値（`ItemSearchSummary[]` + favicon の `[]byte`/mime）を
    `itemSearchResponse` に変換する。`FaviconData` が空でなければ
    `data:<mime>;base64,...` 形式の data URL に整形（`subscription.Service.ListSubscriptions` と
    同じパターンを再利用）
  - compile-time check `var _ ItemSearchServiceInterface = (*ItemSearchServiceAdapter)(nil)` を追加
  - _Requirements: 4.2_
- [x] 5.4 `internal/handler/item_search_handler_test.go` を新規作成する
  - テーブル駆動で 200 成功（横断 / フィード内） / 401（withUserID なし） / 400（cursor 不正・
    feed_id UUID パース失敗） / 403（未購読 feed_id） / 空クエリ → 200 OK 空配列を検証する。
    既存 `item_handler_test.go` の `mockItemService` パターンを踏襲し、mock service を差し替える
  - _Requirements: 1.3, 1.4, 1.5, 3.3, 3.5_
- [x] 5.5 `internal/app/app.go` に `itemsearch.SearchService` の wiring を追加する
  - `itemsearch.NewSearchService(itemRepo, subRepo)` を生成し、`handler.NewItemSearchServiceAdapter` で
    アダプタを構築、`RouterDeps.ItemSearchService` にセットする
  - _Requirements: 1.3, 1.4_

- [x] 6. Web フロントエンドの状態管理に検索モード（横断 / フィード内）を追加する
  - `web/src/contexts/app-state.tsx` の `AppState` に `searchQuery: string`, `isSearching: boolean`,
    `searchScope: 'global' | 'feed'`, `searchFeedId: string | null` を追加する
  - アクション型 `SET_SEARCH_QUERY`（`query` / `scope` / 任意の `feedId` を受け取る）と
    `CLEAR_SEARCH` を追加し、reducer で design.md の規約どおりに処理する（`SELECT_FEED` 時に
    `searchQuery=''` / `isSearching=false` / `searchScope='global'` / `searchFeedId=null` に
    もリセット、`CLEAR_SEARCH` で `selectedFeedId` と `filter` は **保持**）
  - `web/src/contexts/app-state.test.tsx` にケースを追加（`SET_SEARCH_QUERY` で scope='global' /
    scope='feed' / `CLEAR_SEARCH` / `SELECT_FEED` の検索状態リセット）
  - _Requirements: 1.5, 1.6, NFR 2.1, NFR 2.2_

- [ ] 7. Web フロントエンドの検索 UI とフェッチ層を実装する
- [x] 7.1 `web/src/types/item.ts` に検索用の型を追加する (P)
  - `SearchScope = 'global' | 'feed'`
  - `ItemSearchHit`（`ItemSummary` 相当 + `feed_title: string` + `favicon_url: string | null`）
  - `ItemSearchResponse`（`items: ItemSearchHit[]`, `next_cursor: string | null`, `has_more: boolean`）
  - _Requirements: 4.2_
  - _Boundary: web/types/item.ts_
- [x] 7.2 `web/src/hooks/use-item-search.ts` を新規作成する (P)
  - design.md の `useItemSearch(query, scope, feedId)` シグネチャに従い `useInfiniteQuery` で
    `/api/items/search?q=...&limit=50&cursor=...` を呼ぶ（`scope === 'feed'` のとき `&feed_id=...` を付与）
  - `enabled: query.trim().length > 0 && !(scope === 'feed' && !feedId)` で空クエリ / 不正組合せを無効化する
  - `web/src/hooks/use-item-search.test.tsx` を追加（既存 `use-items.test.tsx` パターンを踏襲。
    横断 / フィード内の両ケースをカバー）
  - _Requirements: 1.3, 1.4, 1.5, 4.1, 4.4, 4.5_
  - _Boundary: useItemSearch_
  - _Depends: 7.1_
- [x] 7.3 `web/src/components/header-search-bar.tsx` と `header-search-bar.test.tsx` を新規作成する (P)
  - design.md の `HeaderSearchBar` 疑似シグネチャに従い、入力欄 / Enter ハンドラ / クリアボタンを実装する
  - 空入力 Enter で `SET_SEARCH_QUERY` を dispatch しないこと（Req 1.5）を確認するテストを含む
  - 入力消去ボタンで `CLEAR_SEARCH` が発行されること（Req 1.6）を確認するテストを含む
  - submit 時は `scope: 'global'` で dispatch
  - _Requirements: 1.1, 1.3, 1.5, 1.6_
  - _Boundary: HeaderSearchBar_
  - _Depends: 6_
- [x] 7.4 `web/src/components/feed-search-bar.tsx` と `feed-search-bar.test.tsx` を新規作成する (P)
  - design.md の `FeedSearchBar` 疑似シグネチャに従い、`selectedFeedId === null` のとき `null` を返す
    （NFR 2.3）
  - 入力欄 / Enter ハンドラ / クリアボタンを実装し、submit 時は `scope: 'feed'`,
    `feedId: state.selectedFeedId` で dispatch
  - フィード選択時 render / フィード未選択時非 render / 空入力 Enter dispatch なし / クリアボタン
    `CLEAR_SEARCH` のテストを含む
  - _Requirements: 1.2, 1.4, 1.5, 1.6, NFR 2.3_
  - _Boundary: FeedSearchBar_
  - _Depends: 6_
- [x] 7.5 `web/src/components/search-results.tsx` と `search-results.test.tsx` を新規作成する (P)
  - `useItemSearch(state.searchQuery, state.searchScope, state.searchFeedId)` の結果に対し、
    ローディング / エラー / 空状態 / 結果リストの 4 状態を出し分ける（TanStack Query の `isLoading`
    即時 true により NFR 1.1 の「1 秒以内のローディング表示」を満たす）
  - 結果カードは既存 `ItemList` の `ItemRow` パターンを参考にしつつ、`searchScope === 'global'`
    のときのみ `feed_title` と `favicon_url` バッジを併記する（favicon は `<img src={favicon_url} />`、
    欠落時は代替アイコン）。`searchScope === 'feed'` ではバッジを省略する
  - 既存 `useItemDetail` / `useMarkAsRead` / `useToggleStar` を再利用して本文展開・既読化・スターを提供する
    （Req 4.6, 5.1, 5.2）
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 5.1, 5.2, NFR 1.1_
  - _Boundary: SearchResults_
  - _Depends: 7.2_

- [ ] 8. `AppShell` / `ItemList` に検索バーと SearchResults の出し分けを統合する
  - `web/src/components/app-shell.tsx` のヘッダー領域に `HeaderSearchBar` を配置する
  - 右ペインを `state.isSearching ? <SearchResults /> : <ItemList .../>` で切替える
  - `web/src/components/item-list.tsx` の既存フィルタ群（「すべて／未読」等）と同列の上部領域に
    `<FeedSearchBar />` を追加する（フィード未選択時は `FeedSearchBar` 内部で `null` を返すため
    レイアウト差分は出ない）
  - `web/src/components/app-shell.test.tsx` に統合テストを追加（検索モード切替で `ItemList` ではなく
    `SearchResults` がレンダされること、`CLEAR_SEARCH` で `ItemList` に戻ること）
  - `web/src/components/item-list.test.tsx` を更新（フィード選択時に `FeedSearchBar` が表示され、
    フィード未選択時には表示されないこと）
  - _Requirements: 1.1, 1.2, 1.6, 4.1, 4.7, NFR 2.1, NFR 2.2, NFR 2.3_

- [ ]* 9. NFR 1.2 のパフォーマンス確認スクリプトを `docs/specs/120-/` に追加する
  - 10,000 件規模の items テストデータ生成スクリプトと `EXPLAIN ANALYZE` の出力サンプルを残す
  - pg_trgm GIN がヒットすること、検索が 200ms 以内であることをサンプルログとして記録する
  - 数値目標が運用者と確定したら正規化テストに昇格する
  - _Requirements: NFR 1.2_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の構造化
ブロックで宣言する。リポジトリ慣習に従い、Go 側のテスト・vet と Web 側のテスト・lint を連結する。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./... && cd web && npm test && npm run lint
```
