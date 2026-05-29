# Implementation Notes

## Implementation Notes

### Task 1

- 採用方針: design.md「AppState（Modified）」節および tasks.md task 1 の指示通り、`AppAction` ユニオン型に `ClearSelectedFeedAction = { type: "CLEAR_SELECTED_FEED" }` を追加し、`appReducer` に `SELECT_FEED` と同等の副作用パターン（`selectedFeedId` のみ null、`expandedItemId=null` / `filter="all"` / 検索状態リセット）を持つ case を実装した。
- 重要な判断:
  - `selectedView` は `"feed"` に倒した（`SELECT_FEED` と同じ）。理由: 「選択フィードを失った直後にユーザーが見るのは未選択 feed 表示」というシナリオが自然で、`selectedView` を変えないと `"starred"` 状態のまま選択 ID が null になる不整合が起こり得るため（design.md「Components and Interfaces > AppState」の `Postconditions` と整合）。
  - 検索状態（`searchQuery` / `isSearching` / `searchScope` / `searchFeedId`）も同時にリセットした。理由: 既存 `SELECT_FEED` が検索状態をリセットしている事実（reducer 既存実装）に合わせ「選択フィードが消える＝検索コンテキストも消える」のが自然なため。tasks.md 本文には明示されていないが、design.md の「`SELECT_FEED` と同じ副作用パターンを踏襲」という記述に従った解釈。
  - 既存 19 テストには一切手を加えず、新規 5 テストを追加することで挙動変更がないことを担保した（NFR 1.1）。
- 残存課題: なし。task 2 以降で `AppShell` から `dispatch({ type: "CLEAR_SELECTED_FEED" })` を呼ぶ wiring が予定されている（task 5）。本 task の実装はその下流に必要な reducer 機能を提供する。

### Task 2

- 採用方針: design.md「Components and Interfaces > FeedList（Modified）」節および tasks.md task 2 の指示通り、`FeedListProps` に `onOpenSettings: (subscription: Subscription) => void` を追加し、行コンテナを既存 `<button>` から `<div role="button" tabIndex={0}>` に変更してネスト button 問題を回避した。ギアボタン（`<button data-testid="feed-settings-button-<id>">`）を行末尾に配置し、Tailwind の `group-hover` / `group-focus-within` / `focus-visible:opacity-100` で表示制御。
- 重要な判断:
  - 行コンテナを `<div>` 化したが、既存テスト（`fireEvent.click(screen.getByText("Tech Blog"))` 等）は `<div>` 上でも click が発火するため互換性を維持できた（NFR 1.1）。`data-testid="feed-item-<id>"` / `data-selected` 属性も維持。
  - キーボード起動は `onKeyDown(Enter | Space)` で `e.preventDefault()` してから `onSelectFeed` を呼び、`<button>` のネイティブ activate 挙動を再現（AC 1.5 / WAI-ARIA Practice 準拠）。
  - ギアボタンの `onClick` は `e.stopPropagation()` で行 click ハンドラへの伝搬を止め、加えて `onKeyDown(Enter | Space)` でも `stopPropagation` を行い、ネイティブ button click と親 div の onKeyDown 二重発火を防いだ（AC 1.4 拡張）。
  - app-shell.tsx の `<FeedList>` 利用箇所には no-op `onOpenSettings={() => {}}` を渡し、task 5 で正式 wiring が入るまで TypeScript エラーのみを解消（NFR 1.1: 挙動は完全に不変）。インライン JSX に渡す関数リテラルだが props 数が 4 件で軽量なため、再レンダ過敏なコンポーネントではない判断で `useCallback` 化はしていない。
  - 既存 16 件のテストには既存テストの観点（render 後の DOM 表示）を一切壊さずに `onOpenSettings={() => {}}` props のみを追加し、新規 12 件のテストを追加（合計 28 件 pass）。
  - Tailwind class の検証テスト（`expect(className).toContain("opacity-0")` 等）を入れたのは、`group-hover` 等の CSS 擬似クラスは jsdom が評価しないため、`opacity-0` の解除を runtime で検証できないことの代替として class 文字列レベルで担保するため。
- 残存課題: なし。task 5 で AppShell の `handleOpenSettings` 実装と wiring を行い、no-op を置き換える予定。

## 受入基準カバレッジ（task 1 分のみ）

| Requirement | テスト |
|---|---|
| 4.2（部分）: 解除されたフィードが選択中だった場合に右ペインをクリアするために必要な reducer 機能 | `app-state.test.tsx` の `CLEAR_SELECTED_FEED アクションで selectedFeedId が null になり、expandedItemId と filter がリセットされること` / `CLEAR_SELECTED_FEED アクションで検索状態（searchQuery / isSearching / searchScope / searchFeedId）もリセットされること` / `CLEAR_SELECTED_FEED アクションは初期状態に対しても安全に動作すること（冪等性）` |
| NFR 1.1: 既存 action 挙動の非変更 | `app-state.test.tsx` の `CLEAR_SELECTED_FEED アクション導入後も既存 SELECT_FEED の挙動が変わらないこと（NFR 1.1 回帰）` / `... 既存 EXPAND_ITEM のトグル挙動 ...` / `... 既存 SET_FILTER の挙動 ...` および既存 17 テストが全て green |

## verify 実行結果（task 1 分のみ）

- `web/src/contexts/app-state.test.tsx`: 22 件 pass（既存 17 件 + 新規 5 件）
- `npm test`（web 全体）: 322 件 pass / 34 ファイル全て green（既存テストの破壊なし、NFR 1.1 担保）
- 実行ノードは Node 24.11.1 を利用（`web/package.json` の依存 `whatwg-url@16.0.1` が Node `^20.19.0 || ^22.12.0 || >=24.0.0` を要求するため。`PATH` 上の Node 22.11.0 はバージョン不整合で vitest が起動しない既存環境問題があり、Node 24.x で代替実行した）。

## 確認事項

- なし（task 1 単体では requirements.md / design.md / tasks.md と矛盾なく実装可能だった）
