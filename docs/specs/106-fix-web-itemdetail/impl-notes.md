# 実装ノート: #106 ItemDetail 配線不具合の修正

## 実装サマリ

実装済みだがどこからもレンダリングされていなかった `ItemDetail` コンポーネントを
`ItemList` に配線し、記事行クリック時に選択記事直下へ記事詳細をアコーディオン展開表示
できるようにした。あわせて、一覧 API がサマリー（本文なし）のみ返すことに対応し、展開時に
本文を別取得する `useItemDetail` フックを新規追加した。

design-less impl（design.md / tasks.md なし）。requirements.md の AC を直接テストケースへ
落とし込んで実装した。

### 主な実装方針

1. `useItemDetail(itemId)`: `GET /api/items/:id` を TanStack `useQuery` で取得。
   `queryKey: ["item", itemId]`、`enabled: itemId !== null`（未展開時はリクエストしない）。
   `useItems` の書き方に倣った。
2. `ItemList` の配線:
   - 各行を `<div>` でラップし、`ItemRow`（`<button>`）の **直後に兄弟要素**として詳細エリアを
     描画（button の内側にネストしない）。
   - 詳細エリアは `ItemDetailArea` サブコンポーネントに切り出し、取得状態に応じて
     ローディング表示（`item-detail-loading`）／エラー表示（`item-detail-error`）／
     `<ItemDetail>` を出し分ける。展開枠は取得完了を待たず同期的に描画（NFR 2.1）。
   - 別記事へ切り替えた直後に前記事の古い `detail` が残るケースを避けるため、
     `detail.id !== detailItemId` の間はローディング表示にフォールバックする。
   - 既読化は `useMarkAsRead().mutate`、スター切替は `useToggleStar().mutate({ itemId, isStarred })`
     を `ItemDetail` の props へ渡す（既存フックを再利用）。
3. `ItemDetail` コンポーネント本体（見た目・props）は **変更していない**（要件 Out of Scope）。
4. 自動既読化・スター楽観的更新・排他トグル（reducer の EXPAND_ITEM）は既存実装を活用。
   `app-shell.tsx` のコメント（「ItemList 配下で統合される」）は実態と一致したため変更不要。

## 追加・変更ファイル

| ファイル | 種別 | 内容 |
|---|---|---|
| `web/src/hooks/use-items.ts` | 変更 | `useItemDetail(itemId)` フックを追加 |
| `web/src/hooks/use-items.test.tsx` | 変更 | `useItemDetail` のテスト（正常取得・null 無効化・エラー）を追加 |
| `web/src/components/item-list.tsx` | 変更 | `ItemDetail` の配線、`ItemDetailArea` サブコンポーネント追加 |
| `web/src/components/item-list.test.tsx` | 変更 | 詳細展開・ローディング・エラー・既読化・スター・トグル開閉のテストを追加 |

## テスト結果

実行コマンド（node v22.11.0 環境のため、vite7/vitest4 の `require(ESM)` を有効化する
`NODE_OPTIONS=--experimental-require-module` を付与して実行）:

- `npm test`（`vitest run`）: **26 ファイル / 200 テスト 全 pass**
  - `src/hooks/use-items.test.tsx`: 8 テスト pass（うち `useItemDetail` 3 件追加）
  - `src/components/item-list.test.tsx`: 19 テスト pass（既存 9 + 詳細展開 10 件追加）
- `eslint src/`: **0 errors**（6 warnings は全て既存ファイル由来。本変更で新規 error なし）
- `tsc --noEmit`: 本変更ファイル（item-list.tsx / use-items.ts / 各 test）で **型エラーなし**
  - `src/lib/rewrites.test.ts` に既存の `ProcessEnv` 型エラーが残存するが、本 Issue で
    未変更のファイルであり対象外（確認事項参照）。

## AC とテストの対応

| AC | 担保するテスト |
|---|---|
| 1.1 選択記事直下に詳細展開 | item-list.test「選択中の記事行の直下に記事詳細エリアを展開表示すること」（`row.nextElementSibling` に `item-content` が含まれ、button 内にネストしないことを検証） |
| 1.2 サニタイズ済み本文表示 | 同上（本文テキストの表示を検証）。本文サニタイズ自体は ItemDetail 側の既存テストで担保 |
| 1.3 スター/はてブ数/元記事リンク表示 | item-list.test「展開中の記事詳細にスター切替・はてブ数・元記事リンクが表示されること」 |
| 1.4 選択行のハイライト | 既存 item-list.test「記事をクリックするとonSelectItemが呼ばれること」＋ `isExpanded` による `bg-accent` 付与（既存実装） |
| 1.5 未選択時は詳細非表示 | item-list.test「いずれの記事も選択されていない場合は記事詳細エリアを表示しないこと」（`item-content`/`item-detail-loading` 非表示・詳細 fetch しない） |
| 2.1 展開時に詳細取得 | item-list.test「選択中の記事行の直下に…展開表示」「未読記事の…既読化リクエスト」（`/api/items/item-1` への取得を検証）＋ use-items.test「itemIdが指定された場合に記事詳細を取得できること」 |
| 2.2 取得中ローディング表示 | item-list.test「記事詳細の取得が完了していない間はローディング表示を提示すること」（遅延レスポンスで検証） |
| 2.3 取得失敗時エラー表示 | item-list.test「記事詳細の取得に失敗した場合はエラー表示を提示すること」＋ use-items.test「APIエラー時はエラー状態になること」 |
| 2.4 完了後にローディング除去し本文表示 | item-list.test「選択中の記事行の直下に…展開表示」（ローディング消失 → 本文表示） |
| 3.1 未読展開時に既読化送信 | item-list.test「未読記事の詳細を展開すると既読化リクエストを送信すること」 |
| 3.2 既読展開時は既読化しない | item-list.test「既読記事の詳細を展開しても既読化リクエストを送信しないこと」 |
| 3.3 同一記事再描画で重複送信しない | ItemDetail 側 useEffect（`[item.id]` 依存）の既存挙動。ItemDetail の既存テストで担保 |
| 4.1 スター反転更新要求 | item-list.test「詳細のスター切替ボタン押下でスター反転の更新リクエストを送信すること」 |
| 4.2 一覧へスター即時反映 | `useToggleStar` の楽観的更新（既存）。use-item-state.test の既存テストで担保 |
| 4.3 失敗時ロールバック | `useToggleStar` の onError ロールバック（既存）。use-item-state.test の既存テストで担保 |
| 5.1 再クリックで詳細を閉じる | item-list.test「展開中の記事を閉じる（expandedItemId=null）と詳細エリアが消えること」 |
| 5.2 閉じるとハイライト解除 | 同上＋ `isExpanded=false` で `bg-accent` 非付与（既存実装） |
| 5.3 別記事クリックで前を閉じ新規展開 | item-list.test「別の記事を選択すると直前の詳細を閉じて新たな記事詳細を展開すること」 |
| 5.4 同時 2 件以上展開しない | 同上（`getAllByTestId("item-content")` が 1 件であることを検証）＋ reducer の排他トグル（既存） |
| NFR 1.1 一覧挙動の非回帰 | 既存 item-list.test 9 件が全 pass（一覧表示・無限スクロール sentinel・フィルタ切替・推定日付） |
| NFR 1.2 サニタイズ済み描画 | ItemDetail 側 `sanitizeContentHtml`（既存・本変更で不変）。ItemDetail の既存テストで担保 |
| NFR 2.1 200ms 以内に展開枠表示 | item-list.test「取得が完了していない間はローディング表示」（取得完了を待たず同期描画で枠表示） |

> AC 4.2 / 4.3 / 3.3 / NFR 1.2 は本 Issue で再利用した既存フック・既存 ItemDetail コンポーネントの
> 責務であり、それぞれの近傍既存テスト（`use-item-state.test.tsx` / `item-detail.test.tsx`）で
> 既に担保されている。本配線では「展開時に該当 mutation/取得が呼ばれること」を ItemList 経由で
> 検証する方針とした（重複テストの肥大化を避けるため）。

## 確認事項

- **`src/lib/rewrites.test.ts` の既存型エラー**: `tsc --noEmit` で当該ファイルに
  `ProcessEnv` の `NODE_ENV` 欠落型エラーが複数出る。これは本 Issue で未変更のファイルに
  元から存在する問題であり、本配線とは無関係（本変更ファイルには型エラーなし）。
  CI で型チェックを別途行う場合は別 Issue として切り出すのが妥当か、レビュワー判断を仰ぎたい。
- **node 実行環境**: 当該開発環境の node は v22.11.0 で、vite7/vitest4 の `require(ESM)` が
  デフォルト無効のため、テスト実行時に `NODE_OPTIONS=--experimental-require-module` を付与した。
  CI（`.github/workflows/ci.yml`）の node が 22.12+ または 20.19+ であれば追加フラグなしで
  `npm test` が通る想定。CI 側 node バージョンの確認をレビュワー/PjM に依頼したい
  （本変更はコード側の問題ではなくランタイム要件の確認事項）。
- **ローディング表示のテキスト**: 詳細エリアのローディング/エラー文言は既存の一覧側文言
  （「読み込み中...」「記事の読み込みに失敗しました」）と整合する日本語にした。デザイン刷新は
  Out of Scope のため最小限の表示にとどめている。

STATUS: complete
