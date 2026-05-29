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

## 受入基準カバレッジ

| Requirement | Test |
|---|---|
| Req 1.2 (初期描画) | `feed-search-bar.test.tsx` 新規ケース「初期 mount 時に検索結果表示中... `state.searchQuery` を反映すること」/ 既存「クリアボタン...」ケースでも暗黙的にカバー |
| Req 1.2 (mount 後の外部 dispatch 同期 / 一般化) | `feed-search-bar.test.tsx` 新規ケース「mount 後に外部から SET_SEARCH_QUERY を dispatch すると input の value が新キーワードに同期されること」 |
| Req 1.3 (入力編集追随) | 既存「キーワード入力 + Enter で SET_SEARCH_QUERY...」「入力に前後空白がある場合は trim された値で dispatch」で onChange の追随を検証済み |
| NFR 2.1 (即応性) | React の同期 state update に依存。Req 1.3 の既存 user.type ベーステストで間接的に担保 |

## 検証

- `cd web && npm test -- feed-search-bar`: 10 / 10 pass（既存 8 + 新規 2）
- `cd web && npm test`: 373 / 373 pass（全 web スイート）
- `cd web && npm run lint`: 0 errors / 6 warnings（warnings は全て既存のもので本変更とは無関係）

### Task 2 検証結果

- `cd web && npm test -- feed-pane-header`: 4 / 4 pass（新規追加分）
- `cd web && npm test`: 377 / 377 pass（既存 373 + Task 2 新規 4）
- `cd web && npm run lint`: 0 errors / 6 warnings（warnings は全て既存のもので Task 2 とは無関係。新規ファイル `feed-pane-header.tsx` / `feed-pane-header.test.tsx` で追加 warning なし）

## 確認事項

- なし。design.md の疑似コードどおりに実装し、既存テストの破壊なし。

STATUS: complete
