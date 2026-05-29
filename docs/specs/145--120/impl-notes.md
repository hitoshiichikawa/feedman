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

## 確認事項

- なし。design.md の疑似コードどおりに実装し、既存テストの破壊なし。

STATUS: complete
