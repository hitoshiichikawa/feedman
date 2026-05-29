# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-28T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-114-impl-
- HEAD commit: de31dd85ddcefa9109f216d27a2c86f28ef180ae
- Compared to: develop..HEAD
- 注: 本 spec は design-less impl（`tasks.md` / `design.md` が存在しない）。`_Boundary:_` 宣言は無いため、Issue タイトル・requirements.md の "Feed Register Dialog" スコープを境界の正本として参照。
- Feature Flag Protocol: 採否 `opt-out`（CLAUDE.md 宣言値）→ flag 観点は適用しない。

## Verified Requirements

- 1.1 — `feed-register-dialog.tsx:57` で `setOpen(false)` により成功応答受領直後にダイアログを非表示遷移。テスト `feed-register-dialog.test.tsx:149-153`（`queryByPlaceholderText(...).not.toBeInTheDocument()`）で検証。
- 1.2 — `feed-register-dialog.tsx:59` で `queryClient.invalidateQueries({ queryKey: ["feeds"] })` を呼び出し、左ペインのフィード一覧キャッシュを無効化。本変更で挙動温存（既存挙動の保持）。
- 1.3 — `feed-register-dialog.tsx:61` で `onRegistered(data)` を 1 回伝搬。テスト `feed-register-dialog.test.tsx:165-166`（`toHaveBeenCalledTimes(1)` + `toHaveBeenCalledWith(feedResponse)`）で検証。
- 1.4 — `feed-register-dialog.tsx:85-92` の `handleOpenChange` で `isOpen=true` 時に `setUrl("")` と `setDialogState({ phase: "input" })` および `registerMutation.reset()` を実行。テスト `feed-register-dialog.test.tsx:212-219`（成功→自動 close 後に再オープンし `urlInput.value).toBe("")` を assert）で検証。
- 2.1 — `feed-register-dialog.tsx:115-118` で `DialogTitle` を「フィードを登録」固定に変更し、「登録完了」/「フィードが正常に登録されました」/ 登録済み URL 表示欄 /「閉じる」ボタンの 4 要素を JSX から物理削除。テスト `feed-register-dialog.test.tsx:156` で `queryByText("登録完了")` 不在を確認。
- 2.2 — `feed-register-dialog.tsx:137-153` で success phase 用のフィード URL 表示 Input を物理削除。テスト `feed-register-dialog.test.tsx:158-159`（`queryByDisplayValue("https://example.com/feed.xml")` 不在）で検証。
- 2.3 — `feed-register-dialog.tsx:28-31` の `DialogState` 型から `success` phase を撤去（型レベル enforcement）。「閉じる」ボタンも JSX から削除。テスト `feed-register-dialog.test.tsx:160-162`（`queryByRole("button", { name: "閉じる" })` 不在）で検証。
- 3.1 — `feed-register-dialog.tsx:63-69` の `onError` ハンドラで `setDialogState({ phase: "error", error: apiError })` によりエラー表示し、`setOpen(false)` を呼ばないためダイアログは開いたまま。既存テスト `feed-register-dialog.test.tsx:222-265` がそのまま green。
- 3.2 — 同 `onError` ハンドラのパスを共用。既存テスト `feed-register-dialog.test.tsx:267-309` がそのまま green。
- 3.3 — `feed-register-dialog.tsx:70-80` の fallback で `code:"UNKNOWN"` の汎用エラーオブジェクトを生成し `phase: "error"` へ遷移（`setOpen(false)` を呼ばない）。本 Issue で挙動非変更（impl-notes に「既存実装パスを温存」と記述）。
- 3.4 — `feed-register-dialog.tsx:98-99` で `setDialogState({ phase: "loading" })` を経由し、`feed-register-dialog.tsx:149` で `disabled={!url.trim() || isSubmitting}` によりボタン操作不可。本 Issue で挙動非変更（要件 "Out of Scope" の「ローディング表現の刷新を維持」と整合）。
- 3.5 — `feed-register-dialog.tsx:95-100` の `handleSubmit` で再実行時に `setDialogState({ phase: "loading" })` がエラー状態を上書きし、`registerMutation.mutate(url.trim())` を再呼び出し。本 Issue で挙動非変更。
- 4.1 — `feed-register-dialog.tsx:105` の `<Dialog open={open} onOpenChange={handleOpenChange}>` により Radix Dialog の標準挙動（背景クリック・Esc・close 操作）が `handleOpenChange(false)` 経由で発火し `setOpen(false)`。本 Issue で挙動非変更。
- 4.2 — `feed-register-dialog.tsx:85-92` で `isOpen=true` 時に URL 入力欄をリセット。新テスト `feed-register-dialog.test.tsx:169-220` で「成功→自動 close→再オープン」経路を検証しており、`handleOpenChange(true)` パスを通じて要件 4.2 を間接的に検証している（成功 close 経路を間接的にカバー）。

## Findings

なし

## Summary

本 Issue は design-less impl で `tasks.md` / `design.md` が無く `_Boundary:_` 宣言は無いが、変更ファイルは `web/src/components/feed-register-dialog.{tsx,test.tsx}` の 2 つに限定され、requirements.md の "Feed Register Dialog" スコープ内に収まっている。新規追加された挙動（要件 1.x / 2.x）には対応するテストが追加されており、既存挙動を温存する要件（3.x / 4.x）は既存テストでカバーまたは Out of Scope として要件側で「現状維持」と明示されているため missing test には該当しない。Developer の self-test（`npm test` で 215 tests pass / `npm run lint` で 0 errors）も impl-notes に記載済み。

RESULT: approve
