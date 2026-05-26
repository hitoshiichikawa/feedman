# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T22:54:00Z -->

## Reviewed Scope

- Branch: claude/issue-106-impl-fix-web-itemdetail
- HEAD commit: 876a7cb70a21dfedc5c4b58048756dc873169e1b
- Compared to: develop..HEAD

本 Issue は design-less impl（`design.md` / `tasks.md` は存在しない）。`_Boundary:_`
アノテーションは存在しないため、boundary 判定は requirements.md の Out of Scope と
変更ファイル範囲（`web/` フロントエンドの wiring）に照らして実施した。

変更ファイル:
- `web/src/hooks/use-items.ts`（`useItemDetail` 追加）
- `web/src/hooks/use-items.test.tsx`（テスト追加）
- `web/src/components/item-list.tsx`（`ItemDetail` 配線 / `ItemDetailArea` 追加）
- `web/src/components/item-list.test.tsx`（テスト追加）
- `docs/specs/106-fix-web-itemdetail/{requirements,impl-notes}.md`

## Verified Requirements

- 1.1 — item-list.test「選択中の記事行の直下に記事詳細エリアを展開表示すること」（`row.nextElementSibling` に `item-content` が含まれ button 内にネストしないことを検証）／item-list.tsx の `<div>` ラップ + `ItemRow` 直後の兄弟描画
- 1.2 — 同上テスト（本文「これは記事の本文です」表示を検証）／`ItemDetail` の `item-content` 描画
- 1.3 — item-list.test「展開中の記事詳細にスター切替・はてブ数・元記事リンクが表示されること」（`star-toggle` / `hatebu-count` / `original-link` を検証）
- 1.4 — item-list.tsx の `isExpanded` による `bg-accent` ハイライト（既存実装の流用）＋既存 item-list.test のクリックテスト
- 1.5 — item-list.test「いずれの記事も選択されていない場合は記事詳細エリアを表示しないこと」（`item-content`/`item-detail-loading` 非表示・詳細 fetch なしを検証）／`isExpanded && <ItemDetailArea>` のガード
- 2.1 — item-list.test「未読記事の詳細を展開すると…」「選択中の記事行の直下に…」（`/api/items/item-1` 取得を検証）＋use-items.test「itemIdが指定された場合に記事詳細を取得できること」
- 2.2 — item-list.test「記事詳細の取得が完了していない間はローディング表示を提示すること」（遅延レスポンスで検証）／`ItemDetailArea` の `isLoading` 分岐
- 2.3 — item-list.test「記事詳細の取得に失敗した場合はエラー表示を提示すること」＋use-items.test「APIエラー時はエラー状態になること」／`ItemDetailArea` の `isError` 分岐
- 2.4 — item-list.test「選択中の記事行の直下に…展開表示」（ローディング消失 → 本文表示を検証）
- 3.1 — item-list.test「未読記事の詳細を展開すると既読化リクエストを送信すること」（`PUT /api/items/item-1/state` `{is_read:true}` を検証）
- 3.2 — item-list.test「既読記事の詳細を展開しても既読化リクエストを送信しないこと」
- 3.3 — `ItemDetail` の `useEffect([item.id])` 既存挙動。item-detail.test.tsx（13 件 pass）で担保
- 4.1 — item-list.test「詳細のスター切替ボタン押下でスター反転の更新リクエストを送信すること」（`{is_starred:true}` を検証）
- 4.2 — `useToggleStar` の `onMutate` 楽観的更新（`setQueryData` で一覧キャッシュの `is_starred` を即時更新）。本 PR で新規追加した挙動ではなく既存フックの責務。use-item-state.test.tsx（3 件 pass）で API 配線を担保
- 4.3 — `useToggleStar` の `onError` ロールバック（context から元データ復元）。既存フックの責務（本 PR の差分外）
- 5.1 — item-list.test「展開中の記事を閉じる（expandedItemId=null）と詳細エリアが消えること」
- 5.2 — 同上＋`isExpanded=false` で `bg-accent` 非付与（既存実装）
- 5.3 — item-list.test「別の記事を選択すると直前の詳細を閉じて新たな記事詳細を展開すること」
- 5.4 — 同上（`getAllByTestId("item-content")` が 1 件であることを検証）
- NFR 1.1 — 既存 item-list.test 9 件が全 pass（一覧表示・無限スクロール sentinel・フィルタ切替・推定日付）。実装は既存ロジックに非侵襲（行ラップと詳細描画の追加のみ）
- NFR 1.2 — `ItemDetail` の `sanitizeContentHtml`（本 PR で不変）。item-detail.test.tsx で担保
- NFR 2.1 — item-list.test「取得が完了していない間はローディング表示」（取得完了を待たず同期的に展開枠を描画）／`ItemDetailArea` の同期描画

## Findings

なし

## Summary

requirements.md の全 AC（1.1〜5.4 + NFR）について、本 PR の差分または既存テスト・既存
実装のいずれかで観測可能なカバレッジを確認した。本 Issue で新規に配線した挙動（詳細展開・
ローディング/エラー表示・既読化・スター押下・トグル開閉）には対応テストが追加されており、
4.2/4.3/3.3/NFR 1.2 は本 PR で再利用した既存フック・既存 ItemDetail の責務として既存近傍
テストで担保されている。boundary は `web/` フロントエンドの wiring に閉じており Out of Scope
（バックエンド・一覧 API・デザイン刷新等）への逸脱はない。関連スイート（item-list.test 19 /
use-items.test 8 / use-item-state.test 3 / item-detail.test 13）を再実行し全 43 件 pass を確認した。

RESULT: approve
