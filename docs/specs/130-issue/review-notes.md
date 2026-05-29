# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T03:58:24Z -->

## Reviewed Scope

- Branch: claude/issue-130-impl-issue
- HEAD commit: 116e4a7d7ba4bfce7984b4a2239a35f8b41f785a
- Compared to: merge-base(develop, HEAD)=45263c10..HEAD（develop tip は本ブランチ分岐後に複数 PR が merge されており、本フィーチャー固有の差分のみ評価するため merge-base を採用。`develop..HEAD` 単純比較ではブランチ分岐後に develop に入った crossfeed / favicon 等の無関係な削除が含まれ純粋なレビュー対象を歪めるため）
- 実装対象差分（実装 / テスト / spec のみ抽出）:
  - web/src/contexts/app-state.tsx / app-state.test.tsx
  - web/src/components/feed-list.tsx / feed-list.test.tsx
  - web/src/components/subscription-settings.tsx / subscription-settings.test.tsx
  - web/src/components/subscription-settings-dialog.tsx (new) / subscription-settings-dialog.test.tsx (new)
  - web/src/components/app-shell.tsx / app-shell.test.tsx
  - docs/specs/130-issue/tasks.md / impl-notes.md

## Verified Requirements

- 1.1 — feed-list.test.tsx「ギアボタンのクラスに opacity-0 / group-hover:opacity-100 / group-focus-within:opacity-100 / focus-visible:opacity-100 が含まれること」で初期非表示クラスを検証
- 1.2 — 同上テストで group-hover:opacity-100 等の表示クラス + 行コンテナへの group クラス付与を検証（CSS 擬似クラスを jsdom が評価しない制約に対し、class 文字列レベルで担保）
- 1.3 — feed-list.test.tsx「ギアボタンクリックで onOpenSettings に対象 subscription が渡されること」+ app-shell.test.tsx (a) でクリック→「フィードの設定」ダイアログ表示まで end-to-end 検証
- 1.4 — feed-list.test.tsx「ギアボタンクリックで onSelectFeed が呼ばれないこと（stopPropagation 検証）」/「keyDown(Enter/Space) が行の onKeyDown に伝搬しないこと」で検証
- 1.5 — feed-list.test.tsx「Tab でフォーカス可能 / focus() で activeElement になる」/「Enter で onOpenSettings 発火」/「行コンテナで Enter/Space → onSelectFeed」の 3 テストで担保
- 2.1 — app-shell.test.tsx (a) でダイアログ内に `unsubscribe-button` / `fetch-interval-select` が render される（既存 state 反映）ことを検証。SubscriptionSettings 本体は NFR 1.2 で未改変温存
- 2.2 / 2.3 / 2.4 — SubscriptionSettings 既存実装を温存（diff は onUnsubscribed シグネチャ拡張のみ）。subscription-settings.test.tsx 既存 9 件 + 新規 1 件で担保
- 2.5 — subscription-settings-dialog.test.tsx「Esc キーで onOpenChange(false) が呼ばれること」+ app-shell.test.tsx (b)「Esc で閉じる」で end-to-end 検証
- 3.1 / 3.2 / 3.3 / 3.4 / 3.5 — SubscriptionSettings の AlertDialog 既存実装を完全温存（diff は onUnsubscribed の引数追加のみ）。subscription-settings.test.tsx 既存テストが全件温存
- 4.1 — app-shell.test.tsx (c)「Tech Blog が一覧から消える」/ (d)「News Feed が消え Tech Blog が残る」で [feeds] invalidate→refetch 動作を検証
- 4.2 — app-shell.test.tsx (c)「右ペインに『フィードを選択してください』が表示され、全てタブが消える」+ app-state.test.tsx「CLEAR_SELECTED_FEED で selectedFeedId が null」で reducer〜UI を end-to-end 検証
- 4.3 — app-shell.test.tsx (d)「Tech Blog 選択中に News Feed を解除 → 『全て』タブが残る /『フィードを選択してください』が出ない」で検証
- 4.4 — subscription-settings.test.tsx 新規「onUnsubscribed が feed_id 引数で呼ばれる」+ subscription-settings-dialog.test.tsx「onUnsubscribed → onOpenChange(false) も呼ばれる」+ app-shell.test.tsx (c)(d) 統合
- 4.5 — feed が一覧から消える時点で未読バッジ・ステータスアイコンも render されない（feed-list.tsx の構造的保証）。app-shell.test.tsx (c)(d) で feed 行自体の消滅を確認
- 5.1 — app-shell.test.tsx (e)「500 失敗時に News Feed が一覧に残存」で検証
- 5.2 — app-shell.test.tsx (e)「フィードの設定ダイアログが残存して再試行可能」で検証
- 5.3 — app-shell.test.tsx (e)「feed-item-sub-1 の data-selected=true が維持され、『フィードを選択してください』が表示されない」で検証 + AppShell handleUnsubscribed の構造的保証（mutation onSuccess 内でのみ発火）が JSDoc で明示
- NFR 1.1 — app-state.test.tsx の SELECT_FEED / EXPAND_ITEM / SET_FILTER 回帰テスト 3 件追加 + 既存 feed-list.test.tsx / app-shell.test.tsx の既存ケースが全件温存され pass（impl-notes 報告: web 全体 322 件 / 34 ファイル green）
- NFR 1.2 — SubscriptionSettings の既存 9 件テストが温存され pass（diff は onUnsubscribed シグネチャ拡張のみで挙動変更なし）
- NFR 2.1 — feed-list.test.tsx「aria-label が『<feed_title> の設定』」+「Tab でフォーカス可能」で検証
- NFR 2.2 — subscription-settings-dialog.test.tsx「Esc で onOpenChange(false)」で radix-ui 既定挙動を実環境で確認
- NFR 3.1 — SubscriptionSettings の `unsubscribe.isPending` による button disabled 既存挙動を温存（diff なし、NFR 1.2 で担保）

## Boundary 検証

- 変更ファイルは tasks.md の `_Boundary:_` で許可された範囲（AppState / FeedList / SubscriptionSettings / SubscriptionSettingsDialog / AppShell）のみに収まっている
- バックエンド（Go: api / worker / internal/*）への変更は皆無で、design.md「Impact: バックエンドへの変更は一切ない」と整合
- 想定外コンポーネント（hooks / 他 web コンポーネント）への変更もなし
- Feature Flag Protocol: CLAUDE.md の `**採否**: opt-out` のため flag 観点の細目は適用外（通常レビューのみ実施）

## Findings

なし

## Summary

requirements.md の全 numeric AC（1.1〜5.3）および NFR（1.1 / 1.2 / 2.1 / 2.2 / 3.1）について、対応する実装またはテストが diff 内に確認できる。tasks.md の `_Boundary:_` 制約にも適合しており、reducer / FeedList / SubscriptionSettings / Dialog ラッパ / AppShell の統合が design.md の Architecture / Requirements Traceability と整合した形で実装されている。テストは単体（reducer・FeedList・Dialog・SubscriptionSettings）+ 統合（AppShell の (a)〜(e) 5 シナリオ）の両層でカバーされており、AC 4.2 / 4.3 / 5.3 の核心分岐も end-to-end に検証されている。impl-notes の自己報告（web 全体 322 件 pass / 34 ファイル green）とも整合し、reject 対象となる AC 未カバー / missing test / boundary 逸脱は検出されなかった。

RESULT: approve
