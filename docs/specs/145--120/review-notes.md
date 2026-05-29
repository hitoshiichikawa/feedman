# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-145-impl--120
- HEAD commit: 6a1a584e5f9727ac9b55f1cbc6516874c06db640
- Compared to: develop..HEAD（merge-base 75c9014 起点。`git diff --stat develop..HEAD` 上に
  `internal/feed/favicon*.go` / `docs/specs/148-*` の削除が含まれるが、これらは PR #150
  （Issue #148）が develop に merge された後の base 側追加であり本ブランチ側での意図的削除
  ではない。本 PR で実際に変更/追加されたファイルは `web/src/components/*` と
  `docs/specs/145--120/*` の 10 ファイル）

## Verified Requirements

- 1.1 — `app-shell.tsx` L156-164 で `isSearching && searchScope==='feed' && searchFeedId!==null`
  枝に `<FeedPaneHeader mode="search-feed">` を `<SearchResults />` の上に挿入。
  `app-shell.test.tsx` ケース (1)「フィード選択→Enter で SearchResults と feed-search-bar が同時表示」
  / `feed-pane-header.test.tsx`「mode 切替で feed-search-input が同一 DOM 要素として保持」で構造担保
- 1.2 — `feed-search-bar.tsx` L48-62 の `useEffect` で `state.searchQuery` の外部変更を
  `localQuery` に sync + L36-42 の初期化ロジック。`feed-search-bar.test.tsx` の
  「初期 mount 時に input.value が state.searchQuery を反映」/「外部 dispatch で input.value 同期」
  および `app-shell.test.tsx` ケース (2) で検証
- 1.3 — `feed-search-bar.tsx` L105 の `onChange` 既存挙動 + `app-shell.test.tsx` ケース (3)
  「input を編集 → value が "kubernetes" に追随」
- 1.4 — `feed-search-bar.tsx` L68-81 `handleSubmit` で `SET_SEARCH_QUERY` dispatch。
  `feed-search-bar.test.tsx`「Enter で SET_SEARCH_QUERY dispatch」+ `app-shell.test.tsx`
  ケース (3)「Enter 後に `/api/items/search?q=kubernetes` が呼ばれる（`lastSearchQuery()=="kubernetes"`）」
- 1.5 — `feed-search-bar.tsx` L70-73 の `trimmed.length===0` ガード。
  `feed-search-bar.test.tsx`「空入力 Enter で dispatch されない」/「空白のみ Enter で dispatch されない」+
  `app-shell.test.tsx` ケース (4)「input 空にして Enter 後も search-results-empty が維持」
- 1.6 — `feed-search-bar.tsx` L83-87 `handleClear` で `CLEAR_SEARCH` dispatch。
  `feed-search-bar.test.tsx`「クリアボタン押下で CLEAR_SEARCH」+ `app-shell.test.tsx`
  ケース (5)「クリアで通常一覧（フィルタタブ）に戻り `feed-item-sub-1` の `data-selected="true"` 保持」
- 2.1 — `app-shell.tsx` L165-167 で `searchScope==='global'` 枝は `<SearchResults />` のみ。
  `app-shell.test.tsx` ケース (6)「HeaderSearchBar から submit 後に `feed-search-bar` testid が存在しない」
- 2.2 — 既存 AppState reducer の `SELECT_FEED` が検索状態をリセット。`app-shell.test.tsx`
  ケース (7)「別フィード選択で search-results-empty が消え、選択先 feed-item-sub-2 が選択中」
- 2.3 — `feed-search-bar.tsx` L64-66 の早期 return + `app-shell.tsx` L189-193 の
  「フィードを選択してください」枝。`app-shell.test.tsx` ケース (8)「フィード未選択時に
  feed-search-bar testid が存在しない」+ `feed-search-bar.test.tsx`「フィード未選択時は描画しない」
- 3.1 — `app-shell.tsx` L96 `<HeaderSearchBar />` 配置を維持。`app-shell.test.tsx`
  ケース (8) 末で `header-search-bar` testid 継続表示を確認
- 3.2 — バックエンド API / `useItemSearch` の `feed_id` 任意付与ロジックは変更なし
  （diff に backend ファイル変更なし。`web/src/components/*` のみ）
- 3.3 — `search-results.tsx` 変更なし（diff に含まれない）+ `item-list.test.tsx` の記事描画/
  ローディング/エラー/空状態/詳細展開/無限スクロール/タイトルリンクテスト群（28 件）保持で
  記事リスト本体の挙動非回帰を担保
- 3.4 — `feed-pane-header.tsx` `mode==='normal'` で FilterTabs + FeedSearchBar +
  ManualRefreshButton の 3 要素を描画。`feed-pane-header.test.tsx`「mode='normal' で
  3 要素が描画」+「タブ切替で onFilterChange 呼出」
- NFR 1.1 — `item-list.test.tsx`「ItemList 単体ではフィードヘッダ要素を描画しない」+
  「filter props 未指定時の 'all' fallback ケース」+ `app-shell.test.tsx` ケース (5)
  でフィード内検索 → クリア後の filter "未読" 保持を確認
- NFR 1.2 — `search-results.tsx` 無変更により表示領域・カード構造・空状態・ローディング/
  エラー出し分けは自動的に同一性維持。`app-shell.test.tsx` ケース (9) で 5 観点
  （文言 "検索結果はありません" / tagName "DIV" / `search-results` testid 不在 / ローディング
  / エラー testid 不在 / `feed-search-bar` との同居）構造比較
- NFR 2.1 — React の同期 state update に依存（`feed-search-bar.tsx` の controlled component
  `value={localQuery}` + `onChange` 即時 `setLocalQuery`）。`feed-search-bar.test.tsx` の
  `user.type` ベース既存テスト群で間接的に担保

## Findings

なし

## Summary

Issue #145 の全 AC（Req 1.1-1.6 / 2.1-2.3 / 3.1-3.4 / NFR 1.1-1.2 / NFR 2.1）が
impl コード（`feed-search-bar.tsx` / `feed-pane-header.tsx`（新規）/ `app-shell.tsx` /
`item-list.tsx`）と単体・統合テスト（`feed-search-bar.test.tsx` +2 ケース /
`feed-pane-header.test.tsx` 新規 4 ケース / `app-shell.test.tsx` 新規 9 ケース）で
カバーされている。Developer 報告では `cd web && npm test` 374/374 pass、`npm run lint` 0 errors、
`npm run build` 成功。`_Boundary:_` 違反は検出されず（変更は `web/src/components/*` の 5 ファイル
+ spec ファイルのみで backend `internal/*` への変更は無し）。CLAUDE.md の Feature Flag Protocol は
`**採否**: opt-out` のため flag 観点の追加判定は適用外。

RESULT: approve
