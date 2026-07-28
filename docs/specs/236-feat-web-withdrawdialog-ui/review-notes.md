# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-8 timestamp=2026-07-28T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-236-impl-feat-web-withdrawdialog-ui
- HEAD commit: 8d09d02f84e6755f1e261acb6669b5f002a63ab1
- Compared to: develop..HEAD
- 補足: 本 spec は `design.md` / `tasks.md` を持たない design-less impl。`_Boundary:_`
  アノテーション不在のため boundary 判定は変更ファイルパスと requirements スコープの
  突き合わせで実施。Feature Flag Protocol は CLAUDE.md 上 `opt-out` のため flag 観点は不適用。

## Verified Requirements

- 1.1 — `app-shell.tsx` ヘッダーに `<AccountSettingsDialog />` を配線。テスト: `app-shell.test.tsx` "ヘッダーにアカウント設定入口が表示されること"、`auth-guard.test.tsx` "認証済み時は...render されること"、`account-settings-dialog.test.tsx` "アカウント設定入口ボタンが表示されること"
- 1.2 — `account-settings-dialog.tsx` が情報表示領域 + 退会導線を含む Dialog を開く。テスト: `account-settings-dialog.test.tsx` "入口ボタンをクリックすると...ダイアログが開くこと"、`app-shell.test.tsx` "アカウント設定入口クリックで..."
- 1.3 — `AuthGuard` が未認証時に children を render しない構造的保証。テスト: `auth-guard.test.tsx` "未認証時は...render されないこと"
- 1.4 — radix-ui `Dialog` の Esc/overlay/close 既定挙動。テスト: `account-settings-dialog.test.tsx` "ダイアログはユーザー操作 (Esc) で明示的に閉じられること"
- 2.1 — `AccountInfoSection` `data-testid="account-info-name"`。テスト: `account-settings-dialog.test.tsx` "表示名と email が表示されること"、`app-shell.test.tsx` 表示名 assert
- 2.2 — `emailIsSet` true 分岐。テスト: `account-settings-dialog.test.tsx` "表示名と email が表示されること"
- 2.3 — `emailIsSet` false 分岐 + `EMAIL_UNSET_LABEL="未設定"`。テスト: `account-settings-dialog.test.tsx` "email が未設定（空文字）のとき「未設定」プレースホルダが表示され..."
- 2.4 — `AccountSettingsBody` の `isError||!data` 分岐で `role="alert"` を表示し空 UI 化を防止。テスト: `account-settings-dialog.test.tsx` "アカウント情報の取得に失敗したときエラー通知が表示され、空 UI にならないこと"
- 3.1 — `WithdrawSection` 内 `<WithdrawDialog />` の trigger。テスト: `withdraw-dialog.test.tsx` "退会ボタンが表示されること"、`account-settings-dialog.test.tsx` 退会導線 assert
- 3.2 — `<AlertDialog>` + description「すべてのデータが削除されます。この操作は取り消せません。」。テスト: `withdraw-dialog.test.tsx` "退会ボタンをクリックすると確認ダイアログが開くこと"、"確認ダイアログに削除・取り消し不能である旨の警告文が表示されること"
- 3.3 — `AlertDialogCancel` 既定 close で送信抑止。テスト: `withdraw-dialog.test.tsx` "キャンセルボタンで退会要求が送信されないこと"、`account-settings-dialog.test.tsx` "退会確認ダイアログでキャンセルすると DELETE を送信せず..."
- 3.4 — `apiClient.delete("/api/users/me")`。実 route を `internal/handler/router.go:382`（`r.Delete("/me", userHandler.Withdraw)`）で確認、旧 `/api/account` 不在も確認。テスト: `withdraw-dialog.test.tsx` "退会実行時に DELETE /api/users/me を送信すること"（旧 endpoint 未呼び出し回帰含む）
- 3.5 — onSuccess で `queryClient.clear()` + `onWithdrawn` → `window.location.assign("/login")`。テスト: `withdraw-dialog.test.tsx` "退会成功時に onWithdrawn が呼ばれ..."、`account-settings-dialog.test.tsx` "退会確定で DELETE...成功時に /login へリダイレクトされること"
- 3.6 — `onError` 未宣言で `mutation.isError` 表示（`role="alert"`）、セッション破棄・リダイレクトを実行しない。テスト: `withdraw-dialog.test.tsx` "退会失敗時（サーバ 500）..."、"退会失敗時（ネットワーク到達不能）..."、`account-settings-dialog.test.tsx` "退会失敗時に /login へリダイレクトせず..."
- 3.7 — 退会フローが email 非依存（Cookie セッションで対象特定）。テスト: `account-settings-dialog.test.tsx` "email 未設定（空文字）のパスキーのみアカウントでも退会が同一フローで完了すること"
- 3.8 — `disabled={withdrawMutation.isPending}` + `event.preventDefault()`。テスト: `withdraw-dialog.test.tsx` "応答待機中は退会確定ボタンが disabled で多重送信されないこと"（2 回目以降 click で DELETE 呼び出しが増えないことを assert）
- NFR 1.1 — `app-shell.tsx` 変更はヘッダー内のみで 2 ペイン分岐に非接触。既存 app-shell テスト群が引き続き pass
- NFR 1.2 — `SubscriptionSettingsDialog` の import / 呼び出し不変
- NFR 1.3 — `LogoutButton` / `ThemeToggle` 残置。テスト: `app-shell.test.tsx` "ヘッダーにアカウント設定入口が表示されること" 内で既存 2 機能存在も併せて assert
- NFR 2.1–2.3 — 上記 Req 2.x / 3.x / 1.1 / 1.3 の自動テストで検証可能な形を担保

## Findings

なし

## Summary

全 numeric ID（Req 1.1〜3.8 / NFR 1.1〜2.3）に観測可能な実装と対応テストを確認。退会エンドポイントは実 route `DELETE /api/users/me` と整合し、旧 `/api/account` 呼び出しの回帰防止テストも存在。変更は `web/src/components/*` に限定され boundary 逸脱なし。impl-notes 記載の検証（536/536 pass）とコード内容が整合。

RESULT: approve
