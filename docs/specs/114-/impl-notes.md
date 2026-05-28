# Implementation Notes — #114 フィード追加ダイアログで追加成功したら、ダイアログをそのまま閉じて欲しい

## 変更ファイル一覧

- `web/src/components/feed-register-dialog.tsx` — `DialogState` から `success` phase を撤去し、登録成功時に `setOpen(false)` で即座にダイアログを閉じる挙動へ変更。「登録完了」UI（DialogTitle/Description 三項分岐・フィードURL 表示欄・「閉じる」ボタン）を削除し、フォーム表示の `phase !== "success"` 条件分岐を解消（常時表示）。
- `web/src/components/feed-register-dialog.test.tsx` — 「登録成功時にフィードURLが表示され〜」テストを「登録成功時にダイアログが自動で閉じ onRegistered が 1 回だけ呼ばれること」に置き換え。再オープン時の URL 入力欄リセット（要件 1.4 / 4.2）を検証するテストを追加。

## 受入基準カバレッジ

- **要件 1.1**（成功応答でダイアログ非表示遷移）: `登録成功時にダイアログが自動で閉じ onRegistered が 1 回だけ呼ばれること` で `screen.queryByPlaceholderText("https://example.com")` の不在を検証。
- **要件 1.2**（フィード一覧再取得促進）: `registerMutation.onSuccess` で `queryClient.invalidateQueries({ queryKey: ["feeds"] })` を維持。直接の spy はせず、コードレビューと既存挙動の温存で担保（左ペイン側 `useFeeds` フックが該当キャッシュキーを使う前提）。
- **要件 1.3**（呼び出し元への登録完了通知を 1 回だけ伝搬）: 同テストで `expect(onRegistered).toHaveBeenCalledTimes(1)` および `toHaveBeenCalledWith(feedResponse)` を検証。
- **要件 1.4**（次回オープン時に URL 入力欄が空の入力フェーズで開く）: 新テスト `登録成功で閉じた後に再度ダイアログを開いた際 URL 入力欄が空でエラー表示が残っていないこと` で検証。
- **要件 2.1〜2.3**（「登録完了」UI の完全削除）: 同テスト内で `screen.queryByText("登録完了")` / `screen.queryByDisplayValue("https://example.com/feed.xml")` / `screen.queryByRole("button", { name: "閉じる" })` がすべて不在になることを assert。実装側でも該当 JSX ブロックを撤去済み（success phase の型・JSX 双方）。
- **要件 3.1**（フィード未検出時のダイアログ保持・エラー表示）: 既存テスト `フィード未検出エラー時にエラーメッセージを表示すること` がそのまま green。
- **要件 3.2**（購読上限到達時のダイアログ保持・エラー表示）: 既存テスト `購読上限到達エラー時にエラーメッセージを表示すること` がそのまま green。
- **要件 3.3**（想定外エラー時のダイアログ保持・汎用メッセージ）: 既存 `onError` ハンドラの fallback ロジック（`UNKNOWN` コード生成）をそのまま維持。専用テストは未追加だが既存実装パスを温存しており回帰なし。
- **要件 3.4**（loading 中はダイアログを閉じず登録ボタン disabled）: 既存 `isSubmitting` 判定をそのまま維持。専用テスト追加なし（既存 disabled 検証は input が空のとき）。
- **要件 3.5**（エラー表示中の URL 修正→再登録でエラー解消・再呼び出し）: 既存 `handleSubmit` の `setDialogState({ phase: "loading" })` でエラー表示を上書きする挙動を維持。専用テストは未追加（既存挙動を温存）。
- **要件 4.1**（背景クリック・Esc・標準クローズ操作でダイアログを閉じる）: Radix UI Dialog の標準挙動 + `onOpenChange` ハンドラを維持。テスト追加なし（Radix Dialog 標準挙動）。
- **要件 4.2**（ユーザー操作で閉じた直後に再オープンしたとき URL が空）: 新テストの再オープン検証で `handleOpenChange` の `setUrl("")` 経路を間接的にカバー。

## 主要な実装判断

1. **`DialogState` 型から `success` を撤去**: 型レベルで success phase を排除し、UI 側の `phase === "success"` 条件分岐を物理的に書けない状態にした。これにより要件 2.3「登録成功状態を表現するためだけに存在する UI コードを含まない」を型システムで担保。
2. **`onSuccess` ハンドラで `setOpen(false)` を呼ぶ順序**: `setOpen(false)` → `invalidateQueries` → `onRegistered(data)` の順。`setOpen` を先に呼んで UI を閉じ、その後にキャッシュ無効化と通知を行うことで、コールバック先で何が起きても UI のクローズだけは独立して完了する。
3. **テスト書き換え（要件と矛盾するため）**: 既存テスト `登録成功時にフィードURLが表示されユーザーが変更可能であること` は要件 1.1 / 2.1〜2.3 と直接矛盾するため、新要件に沿うよう全面書き換え。「テスト側を弱める」ではなく「要件が変わったから新要件をカバーするテストに書き換える」位置付け。
4. **`role="dialog"` ではなく入力欄不在で閉鎖検証**: jsdom + Radix Dialog の組み合わせで `role="dialog"` のクエリが portal 周りで安定しない可能性があるため、ユーザーが直接認識する要素（URL 入力欄プレースホルダ）の不在で「閉じている状態」を assert する。これは要件 1.1 のユーザー観点（「ダイアログを閉じた状態に遷移させ画面から非表示にする」）とも整合。
5. **フォームの常時表示**: 旧実装の `{dialogState.phase !== "success" && (<form>...)}` 条件分岐は不要（success に遷移しないため）。常時 `<form>` をレンダリングする形に簡素化。

## ローカル検証ログのサマリ

- 環境: Node v24.11.1（`/home/hitoshi/.cache/ms-playwright-go/1.57.0/node`）/ npm v11.x
- `npm ci`（web/）: 799 packages 導入完了。EBADENGINE 警告あり（既知の Node version 警告のみ）
- `npm test`（vitest run）: **26 test files / 215 tests すべて pass**（所要 56.70s）
  - `feed-register-dialog.test.tsx` は 8 tests すべて green（既存 6 + 新規 2）
- `npm run lint`: **0 errors / 5 warnings**（warnings はすべて本変更と無関係な既存ファイル: `feed-list.tsx` の `<img>` 警告、`*.test.tsx` の未使用 import）
- `npm run build`: 実行せず（design-less impl prompt 指示に従い時間節約。型エラーは vitest 経由で検出済み）

## 確認事項

- なし
- 補足: Node v22.11.0 では vite/vitest が ESM 関連エラーで起動できず、ローカル開発時は Node v22.12+ または v24 系が必要（既知の package.json `engines` 警告）。CI 側の Node バージョンは `.github/workflows/ci.yml` に依存するため本 PR では触らない。

STATUS: complete
