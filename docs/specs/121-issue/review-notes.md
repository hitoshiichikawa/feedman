# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-121-impl-issue
- HEAD commit: 82623e876f30ca79cf40d5f258899ea16d5fd1e8
- Compared to: develop..HEAD（HEAD 全体レビュー）

差分は 33 ファイル / +4321 / -81 行（Go バックエンド 4 ドメイン + Frontend 9 ファイル）。
9 タスクの commit 系列（feat → docs(impl-notes) → docs(tasks)）がきれいに分割されており、
task 1〜9 すべての checkbox が `- [x]` 済み、`impl-notes.md` の Task 1〜9 セクションも完備。
CLAUDE.md `## Feature Flag Protocol` 節は `**採否**: opt-out` のため、flag 観点の細目は
適用せず通常 3 カテゴリで判定。

## Verified Requirements

### Requirement 1（横断一覧エントリの左ペイン配置）

- 1.1 — 「すべての新着記事」仮想エントリを先頭常設: `web/src/components/feed-list.tsx`
  で `feeds.map` の **直前**に `<button data-testid="all-new-items-entry">` を常時 render
  （購読 0 件メッセージは feeds.map の後ろに配置）。テスト
  `feed-list.test.tsx`「購読 0 件でも『すべての新着記事』仮想エントリが描画される」で検証
- 1.2 — 選択時に右ペインを横断一覧へ切替: `app-shell.tsx` で
  `state.viewMode === "cross-feed"` 分岐により `<CrossFeedItemList />` を render。
  `onSelectAllNewItems={() => dispatch({ type: "SELECT_ALL_NEW_ITEMS" })}` を FeedList へ
  配線。テスト `app-shell.test.tsx`「『すべての新着記事』エントリ click で
  viewMode='cross-feed' に切替わり…」で検証
- 1.3 — 横断選択中に個別フィード選択で個別表示に戻る: reducer `SELECT_FEED` が
  `viewMode: "feed"` に倒し、AppShell が分岐で既存 `<ItemList />` を描画。テスト
  `app-state.test.tsx`「SELECT_FEED アクションで viewMode が 'feed' に遷移」+
  `app-shell.test.tsx` の横断→個別→横断遷移テストで検証
- 1.4 — 選択中の視覚的強調: `feed-list.tsx` で `isAllNewItemsSelected` のとき
  `bg-accent text-accent-foreground font-medium` を適用し `aria-current="page"` /
  `data-selected="true"`。さらに viewMode==='cross-feed' のときは個別フィード行を選択中
  扱いしない排他処理（`viewMode !== "cross-feed" && feed.feed_id === selectedFeedId`）。
  feed-list.test.tsx で 2 件検証
- 1.5 — レイアウト整合: `<FeedFavicon feedId="__all__" faviconURL={null} feedTitle="..." />`
  で代替アイコン描画、既存個別フィード行と同じ button 構造／className 規約。テスト
  「既存個別フィード行の並び順とスタイルが本機能導入で変化しない」で検証

### Requirement 2（横断新着記事のマージ表示）

- 2.1 — 全購読フィード対象: `postgres_item_repo.go` `ListNewAcrossFeeds` の SQL
  `JOIN subscriptions s ON s.feed_id = i.feed_id AND s.user_id = $1` で当該ユーザーの
  購読フィードのみを取得。integration test
  `postgres_item_repo_cross_feed_test.go`「他ユーザーが購読していないフィードの記事は
  混入しない」で検証
- 2.2 — published_at 降順マージ: SQL `ORDER BY i.published_at DESC, i.id DESC`。
  「2 フィード購読 + 6 記事で sinceTime 以後のみ published_at 降順で返る」で検証
- 2.3 — 同一日時の決定論順序: `(published_at, id)` 複合キーによる id 降順 tiebreak。
  「同一 published_at で id 降順タイブレークが効く」で検証
- 2.4 — 既存個別フィードと同等情報表示: `CrossFeedItemList` が既存 `ItemRow` /
  `ItemDetailArea` を再利用し、`toItemSummary` で `CrossFeedItem` → `ItemSummary` に射影
- 2.5 — 既読化／スター付与の同一操作性: `useMarkAsRead` / `useToggleStar` を流用、
  `useItemDetail` / `ItemDetailArea` も再利用。`use-item-state.ts` の onSuccess /
  onSettled に `['cross-feed-items']` invalidation を追加（同期）

### Requirement 3（発信元フィードのバッジ表示）

- 3.1 — フィード名表示: `cross-feed-item-list.tsx` の各記事行で
  `<span className="truncate">{item.feed_title}</span>`。テスト「各記事行に feed_title と
  FeedFavicon が描画される」で `getByText("Feed A")` / `getByText("Feed B")` を assert
- 3.2 — favicon バッジ表示: `<FeedFavicon feedId={item.feed_id}
  faviconURL={item.feed_favicon_url} feedTitle={item.feed_title} />` を badge に併記。
  test で `feed-favicon-feed-a` の存在を assert
- 3.3 — favicon 未設定／読込失敗時の代替アイコン: `feed-favicon.tsx` の
  `showImage = hasURL && !imgFailed` ロジックで Rss icon fallback。
  `feed-favicon.test.tsx` の 4 ケース（URL あり / null / 空文字 / onError 発火）で検証
- 3.4 — レイアウト視認性: badge は `flex items-center gap-1.5 px-4 pb-2 -mt-1 text-xs
  text-muted-foreground` で記事 row 直下に配置、title/published_at の視認性を阻害しない

### Requirement 4（新着判定基準と前回時刻の永続化）

- 4.1 — ユーザー単位で 1 件記録・更新: `user_cross_feed_views` テーブル
  （`user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE`）+
  `UserCrossFeedViewRepository.Upsert(... ON CONFLICT (user_id) DO UPDATE ...)` で
  1 行/ユーザーを保証。integration test `postgres_user_cross_feed_view_repo_test.go`
  （Upsert→Get→再 Upsert）で last_seen_at 更新を検証
- 4.2 — sinceTime より後の記事を新着として抽出: SQL `WHERE i.published_at > $2`、
  service `resolveSinceTime` で 3 段優先順位（client override → stored lastSeen → 24h
  fallback）。`crossfeed/service_test.go` の 3 ケースで検証
- 4.3 — セッション初回表示完了時にサーバ側を更新、以降は更新しない:
  `cross-feed-item-list.tsx` の `hasInitializedSessionRef` + `crossFeedSessionSince === null`
  条件で 1 回だけ `touchMutateRef.current()` を呼ぶ。テスト「初回データ受信後に touch
  mutation が 1 回だけ呼ばれる」「crossFeedSessionSince が既に非 null のとき touch
  mutation も上書きも発火しない」+ `app-shell.test.tsx` 横断→個別→横断遷移後も PUT は
  1 回のみで検証
- 4.4 — 初回 fallback: `defaultFallbackWindow = 24 * time.Hour`、`resolveSinceTime` で
  `s.now().Add(-defaultFallbackWindow)`。テスト「overrideSince=nil + lastSeen なしで
  fallback 24h を採用する」で検証
- 4.5 — 再ログイン後も保持: `user_cross_feed_views` の DB 永続化（session lifecycle と
  独立）。integration test で Upsert→Get の永続化を検証
- 4.6 — 新着 0 件時の空状態表示: `cross-feed-item-list.tsx`「allItems.length === 0」
  → `data-testid="cross-feed-item-list-empty"` + 「新着記事はありません」。テストで検証
- 4.7 — 同一セッション内 baseline 固定: `AppStateContext.crossFeedSessionSince` を
  `SET_CROSS_FEED_SESSION_SINCE` で固定し、`SELECT_FEED` / `SELECT_ALL_NEW_ITEMS` 双方で
  保持。`useCrossFeedItems` が URL に `&since=<encoded>` を付与、handler が `since` query
  parameter を `overrideSince` として service に渡し、service は overrideSince 非 nil 時
  `user_cross_feed_views` を参照しない。reducer / hook / handler / app-shell の各層で
  テスト整備

### Requirement 5（個別フィード閲覧機能への非干渉）

- 5.1 — 個別フィード閲覧の挙動不変: AppShell 分岐で既存 ItemList をそのまま render
  （`viewMode === "feed"` または `none` で既存パス）。Backend は `ListByFeed` 等の既存
  メソッド signature を変更せず、新規 `ListNewAcrossFeeds` のみ追加。テスト「viewMode='none'
  のとき既存『フィードを選択してください』表示が維持」で検証
- 5.2 — 並び順・スタイル・未読数バッジ・ステータスアイコン不変: `feed-list.tsx` の
  feeds.map 内コードは既存ロジックを保持し、最上部に仮想エントリを追加するのみ。テスト
  「既存個別フィード行の並び順とスタイル不変」で検証
- 5.3 — 既読化／スター操作の同期: `use-item-state.ts` の `useMarkAsRead` (onSuccess) と
  `useToggleStar` (onSettled) に `queryClient.invalidateQueries({ queryKey:
  ['cross-feed-items'] })` を追加
- 5.4 — 横断→個別→横断戻り時の最新状態反映: `useCrossFeedItems` の `staleTime: 0` で
  毎マウント時に refetch、`since` 固定で同一 baseline を維持。`app-shell.test.tsx` の遷移
  テストで検証

### Non-Functional Requirements

- NFR 1.1 — 1 秒以内の初期表示: `ListNewAcrossFeeds` を 1 クエリ JOIN（N+1 回避）で実装、
  既存 `idx_subscriptions_user_id` / `idx_items_feed_published_at` を活用、limit 既定 50
- NFR 1.2 — 個別フィード閲覧速度の非劣化: 既存 `ListByFeed` 等は変更されておらず、
  ItemRepository への新メソッド追加のみ
- NFR 1.3 — 上限 or 段階的読み込み: handler で `limit > 200` を 200 にクランプ、
  cursor-based ページング（`limit+1` 取得で HasMore 判定）+ Frontend `useInfiniteQuery` +
  IntersectionObserver
- NFR 2.1 — 個別フィード選択・登録・スクロール不変: 既存 FeedList の feeds.map ループは
  維持、最上部追加のみ
- NFR 2.2 — 既読化・スター付与の同一 API/契約: PUT `/api/items/:id/state` を既存のまま
  使用、新 API なし
- NFR 3.1 — キーボード操作: 仮想エントリは `<button>` 要素として実装、ネイティブ Tab/Enter で
  個別フィード行（同じく button）と同等。テストで aria-current='page' 属性を検証
- NFR 3.2 — テキストコントラスト: 既存 `text-muted-foreground` 規約を踏襲

## Boundary Check（tasks.md `_Boundary:_` 整合）

- Task 2: `UserCrossFeedViewRepository` 新規追加のみ — 適合
- Task 3: `ItemRepository` に `ListNewAcrossFeeds` 追加 / `CrossFeedItem` 型追加のみ。
  既存メソッド signature 不変 — 適合
- Task 4: `crossfeed.Service` 新規 package — 適合
- Task 5: `CrossFeedHandler` / `CrossFeedServiceAdapter` / `RouterDeps`（`CrossFeedService`
  フィールド追加、`app.go` に DI 配線追加。既存 handler 群への破壊的変更なし） — 適合
- Task 6: `FeedFavicon` 切り出しは挙動不変、`AppStateContext` 拡張は `viewMode` /
  `crossFeedSessionSince` の追加と新規 action 追加に留まる — 適合
- Task 7: `FeedList` に仮想エントリ追加のみ、feeds.map 内既存挙動は維持 — 適合
- Task 8: `useCrossFeedItems` / `useTouchCrossFeedLastSeen` 新規。`useMarkAsRead` /
  `useToggleStar` への変更は invalidate 追加 1 行ずつで楽観的更新ロジック不変 — 適合
- Task 9: `CrossFeedItemList` 新規 + `AppShell` 分岐追加。既存 starred / ItemList パス
  温存 — 適合

### 既存 item ドメインの mock 修正（boundary 拡張に追随する不可避な修正）

`internal/item/service_test.go` / `internal/item/upsert_test.go` の `mockItemRepoForService`
/ `mockItemRepo` に `ListNewAcrossFeeds` の no-op スタブを追加（+14 / +13 行）。これは
ItemRepository interface への新メソッド追加に追随する Go の interface 適合のための必要
修正であり、`item` ドメインの挙動には影響しない（impl-notes Task 3 の重要な判断 (3) で
明示）。boundary 逸脱には当たらない。

## Findings

なし

## Summary

Task 1〜9 すべて完了し、Requirement 1〜5 / NFR 1〜3 の全 numeric ID について、対応する実装
（diff 内）とテストの追加が確認できた。`_Boundary:_` 違反なし。

RESULT: approve
