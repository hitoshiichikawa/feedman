# Implementation Plan

- [x] 1. `FeedSearchBar` の外部 searchQuery 同期 useEffect を追加する
  - `web/src/components/feed-search-bar.tsx` に `useEffect` を追加し、
    `state.isSearching && state.searchScope === 'feed' && state.searchFeedId === selectedFeedId`
    のとき `state.searchQuery` を `localQuery` に setLocalQuery で反映する（design.md
    「`feed-search-bar.tsx`（修正）」節の疑似コード参照）
  - 既存の `useState(initialLocalQuery)`（L42）、`handleSubmit`、`handleClear`、描画 JSX は
    変更しない（既存テストの後方互換を担保）
  - 早期 return（`selectedFeedId === null` で `null` を返す、L44-46）が `useEffect` 呼び出し
    順序に影響しないよう、`useEffect` は `useState` の直後・early return の前に置く
  - `web/src/components/feed-search-bar.test.tsx` にテストを追加:
    - 初期 mount 時に `state.isSearching=true && scope='feed' && searchFeedId='feed-X'` で
      `input.value` が `state.searchQuery` を反映する（既存のクリアボタンテストでカバー済みの
      ケースを別ケースとして明示化）
    - mount 後に外部 dispatch `SET_SEARCH_QUERY` を実行 → `input.value` が新キーワードに同期
      （新規ケース、Req 1.2 の一般化）
  - _Requirements: 1.2, 1.3, NFR 2.1_

- [x] 2. `FeedPaneHeader` コンポーネントを新規作成する
- [x] 2.1 `web/src/components/feed-pane-header.tsx` を新規作成する
  - design.md の `FeedPaneHeaderProps` 型に従ったコンポーネントを実装する
    （`mode: 'normal' | 'search-feed'`, `feedId: string`, `filter?: ItemFilter`,
    `onFilterChange?: (filter: ItemFilter) => void`）
  - `mode === 'normal'` のとき: 既存 `item-list.tsx` L143-169 のフィードヘッダ DOM 構造
    （`flex flex-shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-2`）を
    そのまま移植し、内部に `<Tabs value={filter} onValueChange={onFilterChange}>` +
    `<FeedSearchBar />` + `<ManualRefreshButton />` を配置する。`ManualRefreshBanner` も
    ヘッダ直下に描画する
  - `mode === 'search-feed'` のとき: 同じ外側コンテナ div を用い、内部は `<FeedSearchBar />`
    のみを描画する（暫定方針、design.md 確認事項 1, 2 を参照）。FilterTabs / ManualRefresh は
    描画しない
  - **`<FeedSearchBar />` は両モードで同一 React tree 位置に置く**こと（mode 切替で unmount
    されないよう、条件分岐は親コンテナの className または `aria-hidden` 等で行わず、純粋に
    兄弟要素の有無で切替える）
  - `ManualRefreshButton` を再利用するため、`item-list.tsx` 側で `ManualRefreshButton` を
    `export` 化する（または `feed-pane-header.tsx` 側に切り出して両者から利用する形にする）
  - `useFeeds` / `useManualRefresh` の wiring は `FeedPaneHeader` 内部で完結する
    （`item-list.tsx` から移譲。`feedId` 経由で `subscriptionId` を解決する）
  - _Requirements: 1.1, 2.1, 2.3, 3.4_
- [x] 2.2 `web/src/components/feed-pane-header.test.tsx` を新規作成する (P)
  - `mode="normal"` で FilterTabs / FeedSearchBar / ManualRefreshButton が描画されること
  - `mode="normal"` で `onFilterChange` がタブ切替で呼ばれること
  - `mode="search-feed"` で FeedSearchBar のみ描画され、FilterTabs / ManualRefreshButton が
    描画されないこと
  - mode が `'normal'` → `'search-feed'` に切り替わるとき、`FeedSearchBar` の input 要素が
    DOM 上で **同一インスタンス**（mount 維持）として保持されること（Req 1.1 の「画面上に
    表示し続ける」の構造的担保。テスト手段としては再 render 前後で同じ getByTestId が同一
    要素を返すか、もしくは input の dataset を追跡する）
  - _Requirements: 1.1, 2.3, 3.4_
  - _Boundary: web/components/feed-pane-header.tsx_
  - _Depends: 2.1_

- [x] 3. `ItemList` から責務を縮退してフィードヘッダ要素を撤去する
  - `web/src/components/item-list.tsx` の以下を撤去する:
    - フィードヘッダ DOM（L143-169 の `<div className="flex flex-shrink-0 ...">` 配下）
    - `FeedSearchBar` import / 配置
    - `Tabs` / `TabsList` / `TabsTrigger` import / 配置
    - `useState<ItemFilter>("all")`（L37）および filter リセットの `useEffect`（L84-86）
    - `useFeeds` / `subscriptionId` 解決ロジック（L62-66）
    - `useManualRefresh` / `ManualRefreshButton` 配置 / `ManualRefreshBanner` 配置（L67, 161-167, 172）
  - `ItemListProps` に `filter: ItemFilter` を追加し、内部 useState の代わりに props 経由で
    受け取るように修正する
  - `ManualRefreshButton` は `FeedPaneHeader` から利用するため、`item-list.tsx` で
    **export 化**するか、または `manual-refresh-button.tsx` として切り出す。設計簡素化のため
    本タスクでは `item-list.tsx` 内で `export function ManualRefreshButton` 化する案を採用
  - `ItemRow` / `ItemDetailArea` の export は維持する（他コンポーネントが import している）
  - 本タスクは記事リスト本体（取得 / ローディング/エラー/空状態 / 無限スクロール / 詳細展開）の
    挙動を変更しない（Req 3.3 / NFR 1.1 の非回帰担保）
  - `web/src/components/item-list.test.tsx` を以下に従って更新する:
    - フィルタタブ / FeedSearchBar / ManualRefreshButton の存在確認テストは `FeedPaneHeader`
      側のテスト（Task 2.2）に移管したため、`ItemList` 側からは撤去する
    - `ItemList` を直接 render する形のテストでは `filter` props を明示渡しする
    - 記事リスト描画・ローディング/エラー/空状態・無限スクロール sentinel のテストは保持する
  - _Requirements: 3.3, 3.4, NFR 1.1_

- [ ] 4. `AppShell` の右ペイン分岐に `FeedPaneHeader` 挿入ロジックを統合する
  - `web/src/components/app-shell.tsx` の右ペイン分岐（L147-159 付近）を design.md
    「`app-shell.tsx`（修正）」節の擬似コードに従って書き換える:
    - `isSearching && searchScope === 'feed' && searchFeedId !== null` の枝で
      `<FeedPaneHeader mode="search-feed" feedId={state.searchFeedId} />` を `<SearchResults />`
      の上に挿入する（Req 1.1）
    - `isSearching && searchScope === 'global'` の枝は従来どおり `<SearchResults />` のみ
      （Req 2.1）
    - `selectedView === 'starred'` の枝は従来どおり `<StarredItemList />`（変更なし）
    - `viewMode === 'cross-feed'` の枝は従来どおり `<CrossFeedItemList />`（変更なし）
    - `selectedFeedId !== null` の枝で `<FeedPaneHeader mode="normal" feedId={state.selectedFeedId}
      filter={state.filter} onFilterChange={(filter) => dispatch({ type: 'SET_FILTER', filter })} />` +
      `<ItemList feedId={state.selectedFeedId} filter={state.filter} ... />` を render（Req 3.4）
    - `selectedFeedId === null` の枝で「フィードを選択してください」メッセージを表示（Req 2.3 の
      前提状態を含む）
  - 既存ヘッダーの `<HeaderSearchBar />` 配置（L94-95）は変更しない（Req 3.1）
  - 既存の検索対象範囲・ユーザー隔離・記事操作整合（バックエンド `/api/items/search` 経路、
    `useItemSearch` の `feed_id` 任意付与）は本タスクで変更しない（Req 3.2 / 3.3 の非回帰担保）
  - 右ペインの `<main>` の DOM 構造（`flex-1 overflow-hidden` → 内部 `flex flex-col h-full`）は
    維持し、`FeedPaneHeader` をその flex-col の 1 番目に置く（NFR 1.2 の検索結果画面表示領域
    の同一性担保: `SearchResults` 本体の DOM 構造は変更されない）
  - _Requirements: 1.1, 2.1, 2.2, 2.3, 3.1, 3.2, 3.3, 3.4, NFR 1.2_
  - _Depends: 1, 2.1, 3_

- [ ] 5. `AppShell` 統合テストでフィード内検索の連続操作を検証する
  - `web/src/components/app-shell.test.tsx` に統合テストを追加する。fetch モックは既存パターン
    を踏襲し、`/api/feeds`, `/api/items/search`, `/api/items` のレスポンスを差し替える:
    1. フィード選択 → キーワード入力 → Enter で `SearchResults` が表示され、同時に
       `feed-search-bar` testid が画面上に存在する（Req 1.1）
    2. 検索結果表示中に `feed-search-input` の value が現在の検索キーワードを反映している
       （Req 1.2）
    3. 検索結果表示中に input を編集 → Enter で `useItemSearch` の queryKey が更新され、
       fetch が新しいキーワードで呼ばれる（Req 1.3, 1.4）
    4. 検索結果表示中に input を空にして Enter → 検索結果表示が維持される（Req 1.5）
    5. 検索結果表示中にクリアボタン押下 → 通常一覧（`ItemList`）に戻り、選択フィードと
       filter は保持される（Req 1.6, NFR 1.1）
    6. 横断検索結果表示中（`HeaderSearchBar` から submit）に `feed-search-bar` testid が画面上に
       存在しないこと（Req 2.1）
    7. フィード内検索結果表示中に左ペインで別フィードを選択 → 検索が解除され、選択先フィードの
       通常一覧（`feed-search-bar` testid + `ItemList`）が表示される（Req 2.2）
    8. 初期状態（フィード未選択）で `feed-search-bar` testid が存在しないこと（Req 2.3）
    9. 検索結果表示中の `SearchResults` の DOM 構造（カード構造・空状態・ローディング・エラー
       表示）が本変更導入前と同一であることをスナップショット相当の構造比較で確認する（NFR 1.2）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.1, 2.2, 2.3, 3.1, 3.4, NFR 1.1, NFR 1.2_
  - _Depends: 4_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。Web フロント変更のみのため、web 配下の test / lint / build を連結する。

<!-- stage-a-verify -->
```sh
cd web && npm test && npm run lint && npm run build
```
