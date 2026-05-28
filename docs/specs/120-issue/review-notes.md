# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-120-impl-issue
- HEAD commit: 37e511fe2f5af3211acb6005ffe01ced13e43d97
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `web/src/components/header-search-bar.tsx` を新規実装し、`AppShell` ヘッダー領域に
  常設配置（`web/src/components/app-shell.tsx:48` で `<HeaderSearchBar />` を render）。
  `header-search-bar.test.tsx` / `app-shell.test.tsx` で常設レンダリングを検証
- 1.2 — `web/src/components/feed-search-bar.tsx` を新規実装し、`ItemList` のフィルタタブと
  同列の上部領域（`item-list.tsx:146`）に配置。`feed-search-bar.test.tsx` /
  `item-list.test.tsx` で表示条件を検証
- 1.3 — `HeaderSearchBar.handleSubmit` で `SET_SEARCH_QUERY({scope:'global'})` を dispatch
  （`header-search-bar.tsx:33-42`）。`header-search-bar.test.tsx` で submit 時の dispatch を確認
- 1.4 — `FeedSearchBar.handleSubmit` で `SET_SEARCH_QUERY({scope:'feed', feedId})` を dispatch
  （`feed-search-bar.tsx:55-65`）。`feed-search-bar.test.tsx` で確認。`/api/items/search?feed_id=...`
  への伝搬は `use-item-search.ts:45-47` と `use-item-search.test.tsx` で検証
- 1.5 — 両検索バーで `trimmed.length === 0` で early-return（`header-search-bar.tsx:34-37` /
  `feed-search-bar.tsx:55-58`）。Service 層は `strings.TrimSpace(rawQuery)` 空時に空結果を返す
  （`internal/itemsearch/service.go:124-127`）。`header-search-bar.test.tsx` /
  `feed-search-bar.test.tsx` / `internal/itemsearch/service_test.go` で確認
- 1.6 — クリアボタン押下で `CLEAR_SEARCH` を dispatch（両バー共通）。reducer は
  `searchQuery=''` / `isSearching=false` / `searchScope='global'` / `searchFeedId=null` に戻す
  （`web/src/contexts/app-state.tsx:122-128`）。`app-state.test.tsx` で `selectedFeedId` /
  `filter` 保持を確認
- 2.1 — `SearchByUserAndKeyword` の SQL が `JOIN subscriptions s ... AND s.user_id = $1` を
  必須化、`feedID==nil` で `$3::uuid IS NULL` ガードにより横断検索
  （`internal/repository/postgres_item_repo.go` の検索 SQL）。
  `internal/repository/postgres_item_repo_search_test.go` で確認
- 2.2 — `feedID!=nil` で `i.feed_id = $3` フィルタが付与。`service_test.go` でフィード内検索
  ケース、`postgres_item_repo_search_test.go` で DB レベル検証
- 2.3 — SQL の WHERE 句に `(i.title ILIKE $2 OR i.content ILIKE $2)`。
  `postgres_item_repo_search_test.go` のタイトル一致 / 本文一致 / 両方一致 ケースで確認
- 2.4 — `escapeLikePattern` で `\` / `%` / `_` をエスケープ後 `%pattern%` 形式に組み立て
  （`internal/itemsearch/service.go:266-272`）。`service_test.go` のエスケープテーブル
  テストで確認
- 2.5 — SQL の WHERE で自然除外。`postgres_item_repo_search_test.go` の「どちらも不一致」
  ケースで確認
- 2.6 — `ILIKE` 採用により case-insensitive。`postgres_item_repo_search_test.go` の
  大文字小文字差ケースで確認
- 3.1 — `JOIN subscriptions s ... AND s.user_id = $1` が常に必須
  （`postgres_item_repo.go` 検索 SQL）。
- 3.2 — 同 JOIN により非購読フィードの記事は結果集合から自然除外。
  `postgres_item_repo_search_test.go` の他ユーザー隔離ケースで確認
- 3.3 — `ItemSearchHandler.Search` で `middleware.UserIDFromContext` 失敗時 401 UNAUTHORIZED
  （`internal/handler/item_search_handler.go:112-119`）。`item_search_handler_test.go` の
  認証なしケースで確認
- 3.4 — JOIN で `subscriptions` に行が無い解除済みフィードは結果から除外。
  `postgres_item_repo_search_test.go` で確認
- 3.5 — Service 層が `subRepo.FindByUserAndFeed(...)==nil` で `model.NewFeedNotSubscribedError`
  を返し、handler は 403 にマップ（`feed_handler.go` の `mapAPIErrorToHTTPStatus` に
  新規エントリ追加）。`internal/itemsearch/service_test.go` / `item_search_handler_test.go` の
  未購読 feed_id ケースで確認
- 4.1 — `ORDER BY i.published_at DESC NULLS LAST, i.id DESC`。
  `postgres_item_repo_search_test.go` の安定ソート / ページネーションテストで確認
- 4.2 — レスポンスに `feed_title` / `favicon_url` を含め、`SearchResults` の `SearchResultRow`
  が `showFeedBadge={state.searchScope === 'global'}` で出し分け
  （`web/src/components/search-results.tsx`）。`search-results.test.tsx` で
  global / feed 両スコープのバッジ表示を確認
- 4.3 — `allHits.length === 0` で `search-results-empty` 表示
  （`search-results.tsx:138-148`）。`search-results.test.tsx` で確認
- 4.4 — `isLoading` で `search-results-loading` を即時表示（NFR 1.1 と整合）。
  `search-results.test.tsx` で確認
- 4.5 — `isError` で `search-results-error` 表示。`search-results.test.tsx` で確認
- 4.6 — `useItemDetail` / `ItemDetail` を再利用し本文展開を提供
  （`search-results.tsx` の `SearchResultDetailArea`）。`search-results.test.tsx` で
  展開挙動を確認
- 4.7 — `app-shell.tsx:85-95` で右ペインを `state.isSearching ? <SearchResults /> : <ItemList ... />`
  に切替。`app-shell.test.tsx` の統合テスト「検索モード時に SearchResults がレンダされる」
  で確認
- 5.1 — `useMarkAsRead` を `SearchResults` から再利用（`search-results.tsx:56-64`）
- 5.2 — `useToggleStar` を同様に再利用
- 5.3 — 検索 SQL は SELECT 専用で `items` / `item_states` に UPDATE / INSERT なし。
  `postgres_item_repo_search_test.go` のスナップショット不変テスト
  （`item_states.is_read` / `is_starred` / `updated_at` 等の検索前後比較）で確認
- NFR 1.1 — TanStack Query の `isLoading` が即時 true
  （`use-item-search.ts` + `search-results.tsx:109-119`）。`search-results.test.tsx` で確認
- NFR 1.2 — `pg_trgm` 拡張と GIN インデックス 2 本（title / content）を追加
  （migration `20260528120000_add_item_search_indexes.up.sql`）。
  具体的な 200ms 目標の数値検証は Task 9（deferrable `- [ ]*`）に分離
- NFR 2.1 — 既存 `ItemList` 経路（`state.isSearching === false`）は変更なし。
  `app-shell.test.tsx` の通常モード描画テストで確認
- NFR 2.2 — reducer の `CLEAR_SEARCH` は `selectedFeedId` / `filter` を保持
  （`app-state.tsx:122-128`）。`app-state.test.tsx` で確認
- NFR 2.3 — `FeedSearchBar` が `selectedFeedId === null` で `null` を返す
  （`feed-search-bar.tsx:46-48`）。`feed-search-bar.test.tsx` / `item-list.test.tsx` で確認
- NFR 3.1 — `ItemSearchHandler.Search` 入口で `slog.Info("item search request", user_id,
  search_type, scope, query_len, feed_id)` を発行
  （`item_search_handler.go:156-162`）。`search_type` は `global` / `feed` の 2 値で観測可能

## Findings

なし

## Summary

Issue #120 の全 numeric AC（Req 1.1-1.6, 2.1-2.6, 3.1-3.5, 4.1-4.7, 5.1-5.3, NFR 1.1, 1.2,
2.1-2.3, 3.1）が実装またはテストで観測可能にカバーされており、tasks.md の `_Boundary:_` /
`_Requirements:_` 制約も逸脱なし。Feature Flag Protocol は opt-out のため flag 観点の確認は
対象外（CLAUDE.md `## Feature Flag Protocol` 採否 = opt-out）。Developer の impl-notes
（Task 1-8）に Go 全 21 パッケージ green / Web 30 ファイル 276 テスト全 pass / ESLint
errors 0 / go vet clean の記録あり。

> 補足（reject 対象外、informational）: develop には merge 済みの #117（StarredItemList /
> StarredNavItem 等）が、本ブランチの base commit（7f555a3）には含まれていないため、
> `git diff develop..HEAD` 上では `StarredItemList` 系コード・`SELECT_STARRED` action・
> `ItemDetailArea` / `ItemRow` の export 等が「削除」されているように見えるが、これらは
> 本ブランチが追加 / 変更していない差分であり、tasks.md の boundary 逸脱には該当しない。
> develop への merge 前に rebase が必要だが、これは Reviewer の 3 カテゴリ判定の対象外。

RESULT: approve
