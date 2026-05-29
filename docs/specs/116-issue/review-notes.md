# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-28T06:18:12Z -->

## Reviewed Scope

- Branch: claude/issue-116-impl-
- HEAD commit: 51cae7ace9db1239404eb82fef054abc1e29cc41
- Compared to: develop..HEAD
- 変更ファイル: `web/src/components/item-detail.tsx`（実装）、
  `web/src/components/item-detail.test.tsx`（テスト）、
  `docs/specs/116-/requirements.md` / `docs/specs/116-/impl-notes.md`（仕様文書）
- 備考: 本 spec は design-less impl（`tasks.md` / `design.md` 不在）。boundary は
  requirements.md Introduction の「ItemDetail コンポーネントに閉じた UI 配置・装飾の見直し」
  を実質的な境界として扱った。Feature Flag Protocol は採否 = `opt-out` のため flag 観点の
  追加判定は行わない。

## Verified Requirements

- 1.1 — `item-detail.tsx` line 88-95: `<div data-testid="item-detail-title-row">` 配下に
  タイトル `<h3>` とメタグループを配置。テスト "タイトル・はてブ数・スター切替が
  同一のヘッダー行に配置されること" で担保
- 1.2 — `item-detail.tsx` line 89: `flex items-start gap-3` で左右配置、h3 は `flex-1 min-w-0`
  / meta-group は `flex-shrink-0`
- 1.3 — `item-detail.tsx` line 89: `flex items-start` で同一行整列がデフォルト
- 1.4 — `item-detail.tsx` line 94 (`flex-1 min-w-0`) + line 109 (`flex-shrink-0`) で
  折り返し時もメタ情報側がタイトルと重ならない
- 1.5 — `item-detail.tsx` line 107-110: `<div data-testid="item-detail-meta-group"
  className="flex flex-shrink-0 items-center gap-1">` がはてブ数とスターをラップ。
  テスト "はてブ数とスター切替が同一のメタ情報グループに隣接して配置されること" で担保
- 2.1 — `item-detail.tsx` line 136-146: Button 子要素を `Star` アイコンのみに変更。
  旧文言「スター」「スター解除」が削除されている。テスト "スター切替コントロールに
  テキストラベルが表示されないこと（未スター時 / スター付き時）" で担保
- 2.2 — `item-detail.tsx` line 141-145 (`text-muted-foreground` 分岐)。
  テスト "未スター時はアイコンが塗りなし..." で担保
- 2.3 — `item-detail.tsx` line 141-145 (`fill-yellow-400 text-yellow-400` 分岐)。
  テスト "スター付き時はアイコンが塗りあり..." で担保
- 2.4 / 2.5 — `item-detail.tsx` line 137: `onClick={() => onToggleStar(item.id, !item.is_starred)}`
  既存挙動維持。既存テスト "スター切替ボタンをクリックすると onToggleStar が呼ばれること"
  / "スター付き記事のスターボタンをクリックすると false で呼ばれること" で担保
- 2.6 — `item-detail.tsx` line 130-138: `variant="ghost"` の hover:bg-accent + `rounded-full`。
  jsdom ではホバー擬似クラスを評価しないため build / 目視確認の領分（reject 対象外）
- 2.7 — `item-detail.tsx` line 133-135: `aria-label={starLabel}` / `aria-pressed={item.is_starred}` /
  `title={starLabel}`。テスト "状態を示す aria-label と aria-pressed が付与されること
  （未スター時 / スター付き時）" で担保
- 3.1 — `item-detail.tsx` の `hatebuDisplay` 既存ロジック維持
  （`item.hatebu_fetched_at === null ? "-" : String(item.hatebu_count)`）。既存テスト
  "はてなブックマーク数が表示されること" で担保
- 3.2 — 同上ロジックで未取得時は `"-"`。既存テスト "はてブ未取得時は「-」が表示されること" で担保
- 3.3 — `String(0)` → `"0"` を表示。新規テスト "はてブ取得済みかつ 0 件のとき「0」が
  表示されること" で境界値を担保
- 3.4 — `item-detail.tsx` line 117-123: `<span data-testid="hatebu-count">` 内に
  `<Bookmark>` アイコン + テキスト。新規テスト "はてブ数表示にブックマークアイコンと
  ツールチップが付与されること（取得済み）" で担保
- 3.5 — `item-detail.tsx` line 86-88 で `hatebuTitle` を分岐生成し、line 120 で
  `title={hatebuTitle}` として付与。新規テスト "...ツールチップが付与されること" /
  "...未取得時に「未取得」を含むこと" で担保
- 4.1 — 旧アクションバー（`<div className="flex items-center gap-2 flex-wrap">` の 3 要素）が
  diff で完全削除されている。新規テスト "元記事リンクがメタ情報グループの外側（著者行）に
  再配置されていること" で担保
- 4.2 — `item-detail.tsx` line 152-174: `<p className="flex flex-wrap items-center gap-x-2 ...">`
  内に著者 / 中点 / 元記事リンクを順配置。新規テスト "著者名と元記事リンクが同一行に
  中点区切りで配置されること" で担保
- 4.3 — `item-detail.tsx` line 165-167: `target="_blank" rel="noopener noreferrer"`。
  修正された既存テスト "元記事URLへの遷移ボタンが表示されること" で担保
- 4.4 — `item-detail.tsx` line 171: `<ExternalLink className="w-3.5 h-3.5">` をリンク内に配置。
  新規テスト "元記事リンクに外部リンクアイコンが付随表示されること" で担保
- 4.5 — `item-detail.tsx` line 153-160: `{item.author && (<>著者 + 中点</>)}` 条件レンダリング。
  新規テスト "著者情報が存在しない場合は区切り記号を表示せず元記事リンクのみ表示すること" で担保
- 5.1 — 本文サニタイズ / 高さ制限 / 続きを読む / 自動既読化 / 外部リンク属性のロジック・JSX は
  diff で変更されていない。既存テスト群（本文の高さ制限、続きを読む、自動既読化、サニタイズ等
  16 件超）が `item-detail.test.tsx` に保持されている
- 5.2 — 親 `useToggleStar` フックの楽観的更新の反映を、新規テスト "スター付き状態で
  再レンダリングするとアイコンが塗りありに切り替わること" で担保
- 5.3 — 同フックの onError ロールバックの反映を、新規テスト "更新失敗で is_starred が
  元の値に戻されたときアイコン表示が操作前の状態に戻ること" で担保（rerender でロールバックを模倣）
- 5.4 — `data-testid="star-toggle" / "hatebu-count" / "original-link"` をすべて維持
  （`item-list.test.tsx` での参照テストも引き続き有効）。`onToggleStar(itemId, !is_starred)`
  の呼び出し規約も維持
- NFR 1.1 — 旧アクションバー（独立 1 行分の `<div>`）が完全削除され、`space-y-4` 配下の
  4 ブロック構成が 3 ブロックに削減されている（impl-notes.md の差分説明と diff で確認）
- NFR 1.2 — メタグループは `gap-1` + アイコン + 短い数値表示 + 32px ボタン 1 つ。
  現実的なタイトル幅でタイトル領域の半分を超えない構成
- NFR 2.1 — Button が標準 `<button type="button">` を生成し、Enter で onClick 発火。
  新規テスト "スター切替コントロールがキーボード（Enter）で操作できること" で担保
- NFR 2.2 — Button `size="icon-sm"` = `size-8` = 32px（`web/src/components/ui/button.tsx` 30 行目
  で確認: `"icon-sm": "size-8"`）

## Findings

なし

## Summary

requirements.md の全 numeric ID（Req 1.1〜5.4 / NFR 1.1〜2.2）について、`item-detail.tsx` の
実装変更と `item-detail.test.tsx` の新規／修正テスト（17 件追加 + 1 件修正）で観測可能な対応を
確認した。impl-notes.md に `npm test` 232/232 pass / `npm run lint` 0 errors / `npm run build`
success が報告されており、変更は ItemDetail コンポーネント単体に閉じている（boundary 逸脱なし）。
既存テスト識別子（`star-toggle` / `hatebu-count` / `original-link`）と onToggleStar の呼び出し規約も
維持されており、Req 5.4 の互換契約を満たす。

RESULT: approve
