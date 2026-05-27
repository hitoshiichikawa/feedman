# 実装ノート: Issue #112 ItemDetail 本文の高さ制限と「続きを読む」トグル

## 対象

- 実装: `web/src/components/item-detail.tsx`
- テスト追記: `web/src/components/item-detail.test.tsx`

## 採用方式

記事本文表示エリア（`data-testid="item-content"`）に対し、初期高さ制限（300px）と
「続きを読む」/「折りたたむ」トグルを追加した。本文 HTML は文字列で切り取らず（NFR 1.1）、
既存の `sanitizeContentHtml` + `dangerouslySetInnerHTML` を維持したまま（NFR 1.2）、CSS の
`max-h-[300px] overflow-hidden` で DOM ツリーを保ったまま視覚的にクリップする方式とした。

### 高さ測定の仕組み

- `contentRef`（`useRef<HTMLDivElement>`）で本文コンテナを参照する。
- `useLayoutEffect` 内で `contentRef.current.scrollHeight` を測定し、閾値
  `COLLAPSED_MAX_HEIGHT_PX = 300` を **超える** 場合のみ `isOverflowing = true` とする。
  - `>` 比較のため「ちょうど 300px」は折りたたまない（境界値）。
- 依存配列に `sanitizedContent` を含めることで、`item.content` が変わったら再測定し、
  同時に `isExpanded` を初期（折りたたみ）へリセットする（要件「item.content が変わったら再測定」）。
- `ResizeObserver` は使わず、描画後の `scrollHeight` を 1 回測定する方式とした。本文は
  記事切替時のみ変化し、コンテナ幅変化による高さ再計算の要件は requirements.md に無いため、
  `useLayoutEffect` + 単発測定で十分と判断した（過剰な抽象化の回避 / CLAUDE.md）。
  - `useLayoutEffect` を採用したのは、描画直後・ブラウザ paint 前に測定してフェードアウト/
    ボタンのちらつきを避けるため。

### 表示状態の導出

- `isCollapsed = isOverflowing && !isExpanded`（折りたたみ表示中）。
  - `isCollapsed` のとき本文コンテナに `max-h-[300px] overflow-hidden` を付与してクリップし、
    下から上へのグラデーション（`bg-gradient-to-t from-background`）の absolute レイヤー
    （`data-testid="content-fade"`）を重ねる。本文コンテナの親を `relative` にして重ねている。
- トグルボタン（`<Button data-testid="content-toggle">`）は `isOverflowing` のときのみ表示し、
  文言は `isExpanded ? "折りたたむ" : "続きを読む"`。押下で `isExpanded` を反転する。
- `isOverflowing === false`（300px 未満/ちょうど）のときはボタンもフェードアウトも出さず、
  高さ制限クラスも付与しない（Req 4）。

## マジックナンバーの定数化

300px は `COLLAPSED_MAX_HEIGHT_PX` 定数として宣言した（CLAUDE.md コード規約）。なお Tailwind の
クラス名は文字列リテラルである都合上 `max-h-[300px]` をクラス文字列として直接記述しているが、
高さ判定ロジック側の閾値は定数で一元管理している。

## テストでの jsdom モック方針

jsdom はレイアウト計算を行わず `scrollHeight` が常に 0 を返すため、テストでは
`HTMLElement.prototype` の `scrollHeight` ゲッターを `Object.defineProperty` で差し替える
ヘルパー `mockContentScrollHeight(value)` を `item-detail.test.tsx` 内に用意した。

- `data-testid="item-content"` 要素に対してのみ指定値を返し、他要素は 0 を返す。
- 各テストの `finally` で元のプロパティ記述子を復元（元が無ければ削除）し、テスト間の汚染を防ぐ。
- 「300px 超（500px）」「300px 未満（200px）」「ちょうど 300px」の 3 パターンを検証する。
- `ResizeObserver` は使用していないため global スタブは不要。

## 受入基準とテストの対応

| AC | 検証テスト（`it` 名の要旨） |
|---|---|
| Req 1.1 / 1.2 / 1.4 | 「本文の高さが300pxを超えるとき折りたたまれ…」(max-h-[300px] overflow-hidden 付与を確認) |
| Req 1.3 | 同上（`content-fade` 表示を確認） |
| Req 2.1 | 同上（`content-toggle` が「続きを読む」表示） |
| Req 2.2 | 「『続きを読む』ボタンを押下すると全文表示に切り替わり…」(max-h-[300px] 非付与) |
| Req 2.3 | 同上（`content-fade` 消失） |
| Req 2.4 | 同上（文言が「折りたたむ」） |
| Req 3.1 / 3.2 | 「全文表示中に『折りたたむ』ボタンを押下すると…」(max-h-[300px] 再付与) |
| Req 3.3 | 同上（`content-fade` 再表示） |
| Req 3.4 | 同上（文言が「続きを読む」に復帰） |
| Req 4.1 / 4.2 / 4.3 | 「本文の高さが300px未満のとき…」(ボタン・フェードアウト・高さ制限なし) |
| Req 4.1 境界値 | 「本文の高さがちょうど300pxのとき折りたたまず…」 |
| NFR 1.1 / 1.2 | 既存テスト「scriptがサニタイズされて描画」「onerror がサニタイズ」「空文字列で空領域」を非回帰で維持（`item-content` の DOM/サニタイズ挙動を確認） |
| NFR 2.1 / 2.2 | 既存テスト（タイトル/著者/元記事リンク/はてブ数/スター切替/自動既読化）を全て非回帰で維持 |

NFR 1 / 2 は本機能で挙動を変えていないため既存テスト群（合計 15 ケース）が引き続き green で
あることをもって担保している。

## 検証結果

- `npx vitest run`（= `npm test`）: 26 ファイル / 214 テスト 全 pass（item-detail.test.tsx は 19 ケース）。
- ESLint（変更ファイル `item-detail.tsx` / `item-detail.test.tsx`）: 警告・エラーなし（exit 0）。
- `npx tsc --noEmit`: 変更ファイルに型エラーなし。`src/lib/rewrites.test.ts` に既存の型エラー
  （`ProcessEnv` の `NODE_ENV` 欠落、本 Issue と無関係）が残存するが、本変更で導入したものでは
  なく、本 Issue のスコープ外。

### 環境メモ

本ワークツリーの `web/node_modules` は未インストール状態だったため `npm ci` で導入した。
ローカル node は v22.11.0 で、vite 7 の CJS→ESM `require` 解決のため vitest 実行時に
`NODE_OPTIONS=--experimental-require-module` を付与している（CI は node 20 系で別経路）。
この点はテスト実行環境固有の事情であり、実装・テストコードには影響しない。

## 確認事項

- `web/src/lib/rewrites.test.ts` に既存の `tsc` 型エラー（`ProcessEnv` 型不整合）が存在する。
  本 Issue のスコープ外のため未修整。別 Issue として切り出すのが望ましい。

STATUS: complete
