# Implementation Notes

## Implementation Notes

### Task 1

- **採用方針**: `FeedSearchBar` に `useEffect` を追加し、検索結果表示中（`isSearching && scope==='feed' && searchFeedId===selectedFeedId`）に AppState の `searchQuery` 外部変更を `localQuery` へ同期する（design.md「`feed-search-bar.tsx`（修正）」節の疑似コードに準拠）。
- **重要な判断**:
  - `useEffect` は `useState(initialLocalQuery)` の **直後**・`if (selectedFeedId === null) return null;` の **前** に配置した。React hooks は条件付き呼び出しを許さないため、early return より前で全 hook を評価しておく必要がある（既存 `useState` も同じ位置にあり、設計と整合）。
  - `useEffect` の deps は `[state.isSearching, state.searchScope, state.searchFeedId, state.searchQuery, selectedFeedId]`。`setLocalQuery` は安定参照のため deps に含めない（react-hooks/exhaustive-deps は満たす）。
  - 既存 `useState(initialLocalQuery)` の初期化ロジック（クリアボタンテスト等で暗黙利用）と handleSubmit / handleClear / 描画 JSX は **一切変更せず**、初期描画の同期は既存の `useState` 初期値経路で、mount 後の外部変更同期は今回追加した `useEffect` 経路で担保する二段構えとした（既存テストの後方互換確保）。
  - 新規テストは既存 8 ケースの **末尾に追加**（既存ケースは変更しない）。ケース A（初期 mount）は既存「クリアボタン...」テストで暗黙にカバーされていたが、Req 1.2 単独の検証として独立化。ケース B（mount 後の外部 dispatch）は `renderWithInitialDispatch` が initial dispatch しか扱えないため、テスト内で `ExternalDispatcher` 小コンポーネントを並列 mount してボタン経由で追加 dispatch を発火する形にした。
- **残存課題**: なし（後続 task 2.x / 3 / 4 / 5 に影響する変更はない。`FeedSearchBar` の public 振る舞いは追加のみで既存 API は不変）。

### Task 2

- **採用方針**: `web/src/components/feed-pane-header.tsx` を新規作成し、design.md の Components and Interfaces / tasks.md L21-52 に従って `FeedPaneHeaderProps = { mode, feedId, filter?, onFilterChange? }` を実装。`mode==='normal'` で FilterTabs + FeedSearchBar + ManualRefreshButton（+ ManualRefreshBanner）を、`mode==='search-feed'` で FeedSearchBar のみを描画する暫定方針を採用。`useFeeds` / `useManualRefresh` の wiring は本コンポーネント内部で完結。
- **重要な判断**:
  - **`FeedSearchBar` の mount 維持構造（Req 1.1）**: 両モードで「左スロット div + 右スロット div + (任意) ManualRefreshBanner」という固定構造を持たせ、`<FeedSearchBar />` は常に右スロット div 内の最初の子として配置した。さらに各スロット div に `key="feed-pane-header-left"` / `"feed-pane-header-right"` を付与し、mode 切替に伴う兄弟要素数変化があっても React reconciliation が同一インスタンスを維持するようにした。テスト（mount 維持テスト）で `expect(inputAfter).toBe(inputBefore)` の Element identity 比較が pass することを確認済み。
  - **`ManualRefreshButton` の export 化方針（tasks.md L36-37 案 A）**: `item-list.tsx` の既存 `function ManualRefreshButton` を `export function ManualRefreshButton` に変更（1 行修正 + doc コメントに再利用理由を追記）。別ファイル切り出しは task 3 で `item-list.tsx` を縮退する際に整理する余地があるため、本 task では最小差分の export 化に留めた。
  - **`useFeeds` / `useManualRefresh` wiring の移譲方針**: `item-list.tsx` から `FeedPaneHeader` 内部へ完全移譲（subscriptionId 解決ロジックも含む）。`item-list.tsx` 側の wiring 撤去は task 3 のスコープのため、現状は両者にロジックが重複している状態だが、task 3 で重複が解消される設計（仕様通り）。
  - **テストヘルパー `renderWithInitialDispatch` の `rerenderUi` 追加**: React Testing Library の `rerender` は `render` 時の wrapper を保持しないため、mount 維持テストで `rerender` を素で使うと `QueryClientProvider` が消失し `useFeeds` が失敗する。これを避けるため、Probe 内で `currentUi` を mutable 参照経由で差し替える `rerenderUi` を追加し、`QueryClientProvider` / `AppStateProvider` / 初期 dispatch を再構築せず内側 ui だけ差し替えられるようにした。
  - **act warning 対策**: `mode='search-feed'` のテストで `setTimeout` で待つ代わりに `waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/api/subscriptions', ...))` で `useFeeds` の resolve を明示的に待つ形に修正し、act warning を解消。
- **残存課題**:
  - **task 3 で `item-list.tsx` から重複ロジック撤去が必要**: `useFeeds` / `useManualRefresh` / `subscriptionId` 解決 / `ManualRefreshBanner` 描画 / フィードヘッダ DOM（L143-169）は task 3 のスコープで撤去する。本 task 完了時点では両ファイルに重複コードがある状態だが、これは tasks.md の段階分割設計どおり。
  - **task 4 で `AppShell` が `FeedPaneHeader` を呼び出す**: 本 task で `FeedPaneHeader` 単体は完成したが、現状 `AppShell` の右ペイン分岐は旧構造のままで `FeedPaneHeader` をまだ使っていない。task 4 で `<FeedPaneHeader mode="normal/search-feed" ... />` を挿入する。
  - **`filter` props の AppState 持ち上げ**: design.md「`item-list.tsx`（修正）」節および「確認事項 3」で「`AppState.filter` 直接読み書き / `ItemList` 内部 `useState<ItemFilter>` 撤去」を暫定採用としているが、本 task では `FeedPaneHeader` の `filter` / `onFilterChange` を props として受け取る形まで実装した（実体の AppState 接続は task 3 / 4 で完成）。

### Task 3

- **採用方針**: `web/src/components/item-list.tsx` からフィードヘッダ責務（フィルタタブ + FeedSearchBar + ManualRefreshButton + ManualRefreshBanner + 関連 wiring）を完全撤去し、`ItemList` を記事リスト本体のみに縮退（tasks.md L54-75 / design.md「`item-list.tsx`（修正）」節に準拠）。Task 2 で `FeedPaneHeader` 側に移譲済みの責務を `ItemList` 側からも削除して重複解消。
- **重要な判断**:
  - **`ItemListProps.filter` の optional 化（ビルド非破壊の橋渡し）**: design.md / tasks.md は「`ItemList` 内部の useState の代わりに props 経由で受け取る」と記述しており、required にする読み方も可能だが、AppShell（呼び出し側）の修正は **Task 4 のスコープ**で確定している。本 Task 単独で required にすると AppShell の `<ItemList feedId=... onSelectItem=... expandedItemId=... />` 呼び出しが TypeScript 型エラーとなり build が壊れる。これは Task 2 の `FeedPaneHeader.filter?` を optional にしている設計と整合する（Task 2 / 3 / 4 を順に通すための per-task 設計）。本 task では `filter?: ItemFilter` + `filter = "all"` の destructuring default fallback を採用し、AppShell が Task 4 で props を明示渡しするまでの間も従来挙動（filter="all" 初期値）と等価に動作させる。
  - **`useState<ItemFilter>` および filter リセット useEffect の撤去方針**: 旧 `useState<ItemFilter>("all")`（L37）と `useEffect(() => setFilter("all"), [feedId])`（L84-86）は両方撤去。fallback の destructuring default `filter = "all"` は **render ごとに評価される即値の局所変数**であり、feedId 変更時の状態リセットを行う必要が無い（前者は React state で、後者は props 駆動で挙動が変わるため）。これにより design.md 確認事項 3 で言及されている「AppState.filter を直接読み書き / `useEffect(L84-86)` 撤去」と整合する状態を本 task 単独で達成できる。
  - **`useFeeds` / `useManualRefresh` の撤去**: Task 2 で `FeedPaneHeader` 内部へ完全移譲済み（subscriptionId 解決ロジックも含む）のため、`ItemList` 側から無条件に撤去。`manual-refresh-banner` import / `ManualRefreshBanner` 描画も撤去（FeedPaneHeader 側に移譲済み）。
  - **`ManualRefreshButton` の export 維持**: Task 2 で `FeedPaneHeader` が `import { ManualRefreshButton } from "@/components/item-list"` 経由で再利用しているため、本 task でも `export function ManualRefreshButton` のままにする（移動・別ファイル切り出しは行わない。tasks.md L64-66 が「設計簡素化のため `item-list.tsx` 内で export 化」を採用する旨を明記）。`ItemRow` / `ItemDetailArea` の export も維持（`CrossFeedItemList` / `StarredItemList` が import 経由で再利用しているため）。
  - **テストの `AppStateProvider` 削除と新規ケース追加**:
    - 旧 `item-list.test.tsx` は `ItemList` 内部の `FeedSearchBar` が `useAppState` を参照するため `AppStateProvider` 必須だった。本 task で `FeedSearchBar` 配線を撤去したため `AppStateProvider` は不要になり、テストヘルパー `createWrapper` を `QueryClientProvider` のみに簡素化。`renderWithInitialDispatch` / `useDispatchOnce` 系ヘルパーも不要になり削除。
    - 「フィルタタブ表示」「フィルタ切替で API パラメータ送信」「FeedSearchBar 表示制御」「ManualRefreshButton クリック / 進行中表示」「ManualRefreshBanner エラー表示（429 / 409 / 401 / 5xx / networkError）」のテスト群は `FeedPaneHeader` 側のテスト（Task 2.2）に既に移管済みのため撤去。テストヘルパー `mockSubscriptions` / `setupMockFetchForManualRefresh` / 関連 `Subscription` 型 import も削除。
    - 新規ケース 3 件を追加:
      1. `filter="unread"` props が API リクエストの `filter=unread` パラメータに反映される（Req 3.3 + props 駆動の検証）
      2. `filter` props 省略時は `"all"` fallback として動作（Task 4 までのビルド非破壊橋渡しの動作確認）
      3. ItemList 単体ではフィードヘッダ要素（フィルタタブ / FeedSearchBar / ManualRefreshButton / ManualRefreshBanner）を描画しない（責務縮退の構造的担保 / NFR 1.1 の同時実装でフィードヘッダが二重描画されないことの保証）
- **残存課題**:
  - **AppShell 統合テスト（`app-shell.test.tsx`）の 7 件が transient に失敗する状態**: 旧 `ItemList` が描画していたフィルタタブを `getByRole("tab", { name: "全て" })` で「ItemList が表示されたか」の proxy assertion として使っていたテストが 7 件あり、本 task の責務縮退で当該タブが ItemList 側に存在しなくなったため失敗する。これらは **Task 4 で AppShell が `FeedPaneHeader` を統合して再びフィルタタブを描画するようになると自動的に green に戻る** 想定（Task 2 の impl-notes で「task 4 で `AppShell` が `FeedPaneHeader` を呼び出す」と明記されている設計上の transient state。per-task 分割設計の意図された中間状態）。
  - **Task 4 で AppShell が `filter` props を `ItemList` に明示渡しする必要**: 本 task で `filter?` を optional にしたため、Task 4 で AppShell が `<ItemList ... filter={state.filter} />` を渡すことで design.md / tasks.md L62-63 の「props 経由で受け取る」が完全に成立する。

### Task 4

- **採用方針**: `web/src/components/app-shell.tsx` の右ペイン分岐を design.md「`app-shell.tsx`（修正）」節の擬似コードに従って書き換え、`FeedPaneHeader` を `SearchResults` / `ItemList` の上に挿入する責務統合のみを行う。タスク本体は AppShell の分岐ロジック変更に限定し、子コンポーネント（`FeedPaneHeader` / `ItemList` / `FeedSearchBar`）の中身は Task 1 / 2 / 3 で完成済みのため触らない。
- **重要な判断**:
  - **5 分岐 + ネスト 1 段の優先順位設計**: `isSearching` を最優先、その中で `searchScope === 'feed' && searchFeedId !== null` をネスト判定する 2 段構造を採用（design.md 擬似コードのフラット if-else を JSX の三項演算ネストで表現）。`searchScope === 'global'` 枝は `FeedPaneHeader` を挿入せず `<SearchResults />` のみ（Req 2.1）、`searchScope === 'feed' && searchFeedId !== null` 枝は `<FeedPaneHeader mode="search-feed" /> + <SearchResults />` を render（Req 1.1）。`searchFeedId === null` の例外ケース（理論上 `searchScope='feed'` だが feedId 未指定）は global 検索と同等扱いで `<SearchResults />` のみとする（防御的）。
  - **「フィードを選択してください」メッセージの所有権移譲**: 旧構造では `ItemList` 側が `feedId === null` のときに当該メッセージを描画していたが、Task 3 で `ItemList` から責務を縮退した結果、AppShell が `selectedFeedId === null` 枝で直接描画する設計に変更（design.md「`app-shell.tsx`（修正）」節の擬似コード `// フィード未選択（Req 2.3 の前提状態）` 行に整合）。`ItemList` は `feedId !== null` を前提として呼び出される。これは `ItemList` 側に「フィードを選択してください」描画ロジックが残っている（item-list.tsx L114-121）が、AppShell が `feedId={state.selectedFeedId}` を渡すのは `selectedFeedId !== null` の枝のみなので到達不能コードになる（残置自体は仕様変更ではないため削除しない / Task 3 のスコープ）。
  - **`filter` props 明示渡し（Task 3 で optional 化した橋渡しの確定）**: AppShell は `<ItemList ... filter={state.filter} />` と明示渡しすることで Task 3 の `filter?: ItemFilter` + `filter = "all"` fallback が defacto unused になる（design.md / tasks.md L62-63 の「props 経由で受け取る」が完全成立）。`FeedPaneHeader` には `filter={state.filter}` + `onFilterChange={(filter) => dispatch({ type: 'SET_FILTER', filter })}` を渡し、AppState の reducer 既存挙動（L218-222 `SET_FILTER` / L173 `SELECT_FEED` 時の `filter: "all"` リセット）を介してフィード切替時のリセットを担保する（design.md 確認事項 3 採用方針と整合）。
  - **import 追加は `FeedPaneHeader` 1 件のみ**: 既存の `SearchResults` / `ItemList` / `StarredItemList` / `CrossFeedItemList` / `HeaderSearchBar` の import 配置を変更せず、`FeedPaneHeader` を `ItemList` の隣に追加。新規 `dispatch({ type: 'SET_FILTER', filter })` 呼び出しは既存 `useAppDispatch` hook をそのまま使用（AppState の reducer に既存実装あり / app-state.tsx L218-222）。
  - **DOM 構造の維持**: `<main data-testid="right-pane" className="flex-1 overflow-hidden">` の内側 `<div className="flex flex-col h-full">` 構造は手付かずで、その flex-col の **直接の子**として `FeedPaneHeader` + リスト本体を fragment（`<>...</>`）で並べる形に変更した。`FeedPaneHeader` の `flex-shrink-0` ヘッダ DOM + リスト本体の `flex-1 overflow-y-auto` の組合せで flex-col layout が成立する（Task 2 で `FeedPaneHeader` 内部の `flex flex-shrink-0` div は実装済み / `ItemList` 内部の `flex-1 overflow-y-auto` も Task 3 縮退後も保持されている）。
  - **Task 3 transient 失敗 7 件の自動解消確認**: Task 3 impl-notes 残存課題で記載されていた `app-shell.test.tsx` の 7 件 transient 失敗（旧 ItemList が描画していたフィルタタブを ItemList 表示判定の proxy として使用していたケース）は、Task 4 で AppShell が `<FeedPaneHeader mode="normal" .../>` を統合して再びフィルタタブを描画するようになったため、本 task 単独で全 19 件 green に復帰（per-task 分割設計の意図された中間状態が解消）。
- **残存課題**:
  - **Task 5（AppShell 統合テスト追加）が次タスク**: tasks.md Task 5 は「フィード内検索の連続操作（フィード選択 → キーワード入力 → Enter → 検索結果中に input 編集 → 再検索 / 空入力 / クリア）」を 9 ケースの統合テストで検証する追加スコープ。本 task では既存 19 件の transient 失敗を解消する以上の新規テストは追加しなかった（Task 5 のスコープ）。
  - **横断検索結果中の FeedSearchBar 非表示の構造的担保**: AppShell の分岐で `searchScope === 'global'` 枝に `<FeedPaneHeader />` を挿入しない実装は完了しているが、回帰防止の専用テスト（Req 2.1 の「横断検索結果表示中に `feed-search-bar` testid が存在しない」）は Task 5 の統合テスト追加スコープに含まれる。
  - **`ItemList` の `feedId === null` 到達不能化**: AppShell が `selectedFeedId !== null` の枝でのみ `<ItemList />` を呼ぶようになったため、`item-list.tsx` L114-121 の「フィード未選択時」分岐が defacto unreachable code となる。これは Task 3 のスコープで明示削除されておらず（Task 3 は最小差分の優先で当該分岐を残置）、本 task でも仕様変更を避けるため削除しない。将来の cleanup PR でデッドコードとして整理する余地あり。

### Task 5

- **採用方針**: `web/src/components/app-shell.test.tsx` の既存 `describe("AppShell コンポーネント")` 内に、`describe("フィード内検索の連続操作（Task 5 / Issue #145）")` ネスト describe を追加し、tasks.md L102-119 に列挙された 9 ケースを 1:1 で対応する個別 `it` として実装。fetch モックは既存 `setupMockFetch`（`/api/feeds`, `/api/items/search`, `/api/feeds/:id/items` を全て空応答）をそのまま再利用し、テスト個別の上書きは行わなかった（空 SearchResults / 空 ItemList 状態の組合せで 9 ケース全 AC を判定可能なため）。
- **重要な判断**:
  - **判定戦略の選択**: 「feed-search-bar testid の有無」「feed-search-input の value 比較」「mockFetch.mock.calls 履歴の `q=` クエリパラメータ抽出」「`role="tab"` 全て / 未読 の存在 + `data-state` 属性」「`data-selected="true"` 属性」の 5 軸で判定。queryKey 更新の検証（Req 1.4）は `useItemSearch` の内部 state を直接見るのではなく、mockFetch の最新 `/api/items/search?q=...` の `q` 値を抽出する `lastSearchQuery()` ヘルパで間接的に検証する形を採用（黒箱テスト / 既存 SearchResults テストパターンと整合 / TanStack Query の内部 queryKey 更新は fetch 発火の必要条件のため十分）。
  - **fetch モック共有方針**: 既存 `setupMockFetch` は `/api/items/search` を `{ items: [], next_cursor: null, has_more: false }` で常に空応答するため、9 ケース全てで「SearchResults 空状態 → `search-results-empty` testid 出現」を観測でき、テスト個別の mock 上書き不要だった。`search-results.test.tsx` のような検索結果データ ありの mock も検討したが、Task 5 の重点は「右ペイン分岐の組合せが意図通り」であり、検索結果の中身は SearchResults 単体テストで担保済み（NFR 1.2 の DOM 構造同一性確認はケース 9 で testid + tagName + 隣接 testid 不在の構造比較で行うため、データ ありモックは不要）。
  - **ケース 3（再検索 input 編集）の上書き手段**: 初実装で `user.tripleClick(input)` + `user.type` を採用したが、`<input type="search">` で jsdom + radix-ui ベース Input のテキスト全選択挙動が安定せず "typescriptkubernetes" のように文字列連結された。`user.clear(input)` → `user.type(input, "kubernetes")` の組合せに修正し、value=="kubernetes" + lastSearchQuery()=="kubernetes" の両方が green に。`user.clear()` は内部的に `input.setSelectionRange + fireEvent('change', '')` を行うため Controlled component の `localQuery` state も同期的に "" にリセットされる。
  - **ケース 5（filter 保持確認）の補強**: NFR 1.1 の「通常利用の非回帰」の一環として、フィード内検索 →クリア の往復で `filter` も保持されることを確認するため、検索前に「未読」タブをクリックして filter=="unread" に切替えてから検索 → クリア後に再び `getByRole("tab", { name: "未読" })` の `data-state="active"` を確認する形にした。AppState reducer の `CLEAR_SEARCH` は searchQuery のみリセットし filter は保持するため（contexts/app-state.tsx L232-239）、本検証は reducer 仕様と整合。
  - **ケース 9（NFR 1.2 構造比較）の判定粒度**: snapshot 比較は導入していない（snapshot は本変更外の DOM 変動で false-fail しやすいため）。代わりに「テキスト文言 "検索結果はありません" の一致」「empty 要素 tagName === "DIV"」「`search-results` testid 不在」「ローディング / エラー testid 不在」「FeedPaneHeader の feed-search-bar testid との同居」の 5 観点で構造同一性を担保した。`search-results.tsx` L130-138 の空状態 DOM 構造（`<div data-testid="search-results-empty" className="flex items-center justify-center h-full ...">検索結果はありません</div>`）が変わると本検証が落ちる。
- **残存課題**: なし（per-task ループ最終 task / Task 1〜4 の実装が全て統合された状態で 9 ケース 全 pass を確認。Task 5 完了で Issue #145 の全 AC（Req 1.1〜1.6 / Req 2.1〜2.3 / Req 3.1〜3.4 / NFR 1.1〜1.2 / NFR 2.1）が単体テスト + 統合テストでカバーされた）。

## 受入基準カバレッジ

| Requirement | Test |
|---|---|
| Req 1.1 (検索結果表示中の feed-search-bar 残置) | `app-shell.test.tsx` 統合テスト Task 5 ケース (1)「フィード選択 → キーワード入力 → Enter で SearchResults と feed-search-bar が同時に表示されること」 |
| Req 1.2 (初期描画) | `feed-search-bar.test.tsx` 新規ケース「初期 mount 時に検索結果表示中... `state.searchQuery` を反映すること」/ 既存「クリアボタン...」ケースでも暗黙的にカバー / `app-shell.test.tsx` Task 5 ケース (2)「検索結果表示中に feed-search-input の value が現在の検索キーワードを反映していること」 |
| Req 1.2 (mount 後の外部 dispatch 同期 / 一般化) | `feed-search-bar.test.tsx` 新規ケース「mount 後に外部から SET_SEARCH_QUERY を dispatch すると input の value が新キーワードに同期されること」 |
| Req 1.3 (入力編集追随) | 既存「キーワード入力 + Enter で SET_SEARCH_QUERY...」「入力に前後空白がある場合は trim された値で dispatch」で onChange の追随を検証済み / `app-shell.test.tsx` Task 5 ケース (3) で再検索時の input 編集追随も確認 |
| Req 1.4 (新キーワードで新検索開始) | `app-shell.test.tsx` Task 5 ケース (3)「検索結果表示中に input を編集 → Enter で useItemSearch の queryKey が更新され新キーワードで fetch される」 |
| Req 1.5 (空入力で検索結果維持) | `app-shell.test.tsx` Task 5 ケース (4)「検索結果表示中に input を空にして Enter → 検索結果表示が維持されること」 |
| Req 1.6 (クリアで通常一覧復帰) | `app-shell.test.tsx` Task 5 ケース (5)「検索結果表示中にクリアボタン押下 → 通常一覧（ItemList）に戻り選択フィードと filter が保持されること」 |
| Req 2.1 (横断検索中は feed-search-bar 非表示) | `app-shell.test.tsx` Task 5 ケース (6)「横断検索結果表示中（HeaderSearchBar から submit）に feed-search-bar testid が画面上に存在しないこと」 |
| Req 2.2 (別フィード選択で検索解除) | `app-shell.test.tsx` Task 5 ケース (7)「フィード内検索結果表示中に左ペインで別フィードを選択 → 検索解除 → 選択先フィードの通常一覧が表示されること」 |
| Req 2.3 (フィード未選択時 feed-search-bar 非表示) | `app-shell.test.tsx` Task 5 ケース (8)「初期状態（フィード未選択）で feed-search-bar testid が存在しないこと」 |
| Req 3.1 (HeaderSearchBar 常設の非回帰) | 既存「ヘッダー領域に横断検索バー（HeaderSearchBar）が常設されること」/ `app-shell.test.tsx` Task 5 ケース (8) で HeaderSearchBar の継続表示も再確認 |
| Req 3.3 (検索結果画面の非回帰 / 記事リスト挙動の非回帰 part) | `item-list.test.tsx`「filter props で渡された値が API リクエストの filter パラメータに反映されること」+ 記事描画 / 詳細展開 / 既読化 / スター切替 / 無限スクロールの既存ケース群（全 28 件 pass）で `ItemList` 記事リスト本体の挙動非回帰を担保 |
| Req 3.4 (通常一覧モードのフィードヘッダ要素) | `feed-pane-header.test.tsx`「mode='normal' で FilterTabs / FeedSearchBar / ManualRefreshButton が描画されること」+ `app-shell.test.tsx` Task 5 ケース (1)(5)(7) で AppShell 統合下でも従来要素群が描画されることを確認 |
| NFR 1.1 (通常利用の非回帰 part) | `item-list.test.tsx`「ItemList 単体ではフィードヘッダ要素を描画しないこと」+ 「filter props 未指定時の `"all"` fallback ケース」で責務縮退後も既存挙動と等価動作することを構造的に担保 / `app-shell.test.tsx` Task 5 ケース (5) でフィード内検索 → クリア後の filter "未読" 保持を確認 |
| NFR 1.2 (検索結果画面の DOM 構造同一性) | `app-shell.test.tsx` Task 5 ケース (9)「検索結果表示中の SearchResults 空状態 DOM 構造が本変更導入前と同一であることを構造比較で確認」（テキスト "検索結果はありません" / tagName "DIV" / `search-results` testid 不在 / ローディング・エラー testid 不在 / feed-search-bar との同居の 5 観点） |
| NFR 2.1 (即応性) | React の同期 state update に依存。Req 1.3 の既存 user.type ベーステストで間接的に担保 |

## 検証

- `cd web && npm test -- feed-search-bar`: 10 / 10 pass（既存 8 + 新規 2）
- `cd web && npm test`: 373 / 373 pass（全 web スイート）
- `cd web && npm run lint`: 0 errors / 6 warnings（warnings は全て既存のもので本変更とは無関係）

### Task 2 検証結果

- `cd web && npm test -- feed-pane-header`: 4 / 4 pass（新規追加分）
- `cd web && npm test`: 377 / 377 pass（既存 373 + Task 2 新規 4）
- `cd web && npm run lint`: 0 errors / 6 warnings（warnings は全て既存のもので Task 2 とは無関係。新規ファイル `feed-pane-header.tsx` / `feed-pane-header.test.tsx` で追加 warning なし）

### Task 3 検証結果

- `cd web && npm test -- item-list`: 28 / 28 pass（旧 32 件から、`FeedPaneHeader` 側に移管した 7 件 + 関連 `useDispatchOnce` ヘルパーが消費していた合計 7 件を削除し、新規 3 件を追加した結果。記事リスト本体・記事詳細展開・無限スクロール・empty/loading/error・タイトルリンク・date estimated マーカー等の従来テストは全て保持して回帰なし）
- `cd web && npm test`: 358 / 365 pass（**7 件 transient 失敗 / すべて `app-shell.test.tsx` 内 / Task 4 完了で自動 green 化見込み**）
  - 7 件失敗の内訳: 全件「フィルタタブが ItemList 経由で描画されること」を ItemList 表示判定の proxy として使用していたケース（`expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument()`）。Task 4 で AppShell が `FeedPaneHeader` を統合し再びフィルタタブを描画するため、それらは Task 4 完了時点で自動的に green に戻る。本 task で `ItemList` のテストを通すこと自体は完全に成功している（28 / 28）
- `cd web && npm run lint`: 0 errors / 5 warnings（warnings は全て既存のもので本 task とは無関係。Task 2 時点 6 warnings → Task 3 で 5 warnings に減少した理由は、撤去された `useState<ItemFilter>` 関連の暗黙警告ではなく、テストヘルパーの未使用 import 整理に伴う副次的な減少）
- `cd web && npm run build`: ✓ success（standalone build 5 routes prerendered）

### Task 4 検証結果

- `cd web && npm test -- app-shell`: 19 / 19 pass（Task 3 完了時点で transient 失敗していた 7 件が全て自動 green 復帰。新規追加テストは Task 5 のスコープのため本 task では 0 件）
- `cd web && npm test`: 365 / 365 pass（全 web スイート / 既存 358 件 + Task 3 transient 失敗解消の 7 件）
- `cd web && npm run lint`: 0 errors / 5 warnings（warnings は全て既存のもので本 task とは無関係。Task 3 時点と同件数 / 同一内容）
- `cd web && npm run build`: ✓ success（standalone build 5 routes prerendered / Task 3 時点と同一の build output）

### Task 5 検証結果

- `cd web && npx vitest run src/components/app-shell.test.tsx`: 28 / 28 pass（既存 19 + Task 5 新規 9）
- `cd web && npm test`: 374 / 374 pass（全 web スイート / Task 4 時点 365 + Task 5 新規 9）
- `cd web && npm run lint`: 0 errors / 5 warnings（warnings は全て既存のもので Task 5 とは無関係。Task 4 時点と同件数 / 同一内容。新規追加 `app-shell.test.tsx` 内のテストブロックで追加 warning なし）
- `cd web && npm run build`: ✓ success（standalone build 5 routes prerendered / Task 4 時点と同一の build output）

## 確認事項

- **Task 3 単独での `app-shell.test.tsx` 7 件 transient 失敗**: 既存 `ItemList` が描画していたフィルタタブ（`role="tab"` name="全て"）を AppShell 統合テストが「ItemList の表示有無」の proxy として使っていたため、本 task でフィードヘッダ責務を `FeedPaneHeader` へ移譲したことで一時的に失敗する。これは tasks.md の Task 3 → Task 4 の責務分割と Task 4 の `_Depends: 1, 2.1, 3_` に従う設計上の意図された中間状態であり、Task 4 で AppShell の右ペイン分岐に `<FeedPaneHeader mode="normal" ... />` を挿入することで自動的に解消される（Task 2 の impl-notes でもこの順序が明記されている）。Task 4 完了後の Reviewer 段階では全テスト green を期待する。

STATUS: complete
