# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-07-27T16:50:00Z -->

## Reviewed Scope

- Branch: claude/issue-223-impl-feat-web-web
- HEAD commit: 446ca5c77227bac4732eea8fb2ee7368c0e79a7c
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `login-page.test.tsx`「capability true でパスキー導線が render される」/ `passkey-buttons.test.tsx`（Google + 2 パスキーボタンを同一画面に提示）
- 1.2 — `PasskeyButtons` が Google 以外の 2 ボタンを追加提示（`login-page.tsx` L43 / `passkey-buttons.test.tsx` available:true ケース）
- 1.3 — `passkey-buttons.test.tsx`「パスキーでログインクリックで mutate 呼出」/ `passkey-buttons.tsx` L99
- 1.4 — `passkey-buttons.test.tsx`「アカウント新規作成クリックで onSignupClick 呼出」/ `login-page.tsx` L43
- 1.5 — `login-page.test.tsx`「href=/auth/google/login」既存テスト維持（Google 導線不変）
- 2.1 — `passkey-signup-dialog.test.tsx`「username 入力欄 +「作成」、recovery email 欄なし」
- 2.2 — `use-passkey-registration.test.tsx` 正常系（`navigator.credentials.create`）/ `pkce.ts` + `webauthn.ts`
- 2.3 — `use-passkey-registration.ts` の登録→認証→session 連鎖 chain（追加操作なし合流）+ 正常系テスト
- 2.4 — `passkey-signup-dialog.tsx`（email 入力欄不在）+ `use-passkey-registration.ts` L218 `email: ""` 常時送信
- 2.5 — `use-passkey-registration.test.tsx` invalid_username / `passkey-signup-dialog.test.tsx` invalid_username 文言
- 2.6 — `use-passkey-registration.test.tsx` username_taken / `passkey-signup-dialog.test.tsx` username_taken 文言
- 2.7 — `use-passkey-registration.test.tsx` cancelled / `passkey-signup-dialog.test.tsx` cancelled で汎用文言非表示 + reset
- 2.8 — `use-passkey-registration.test.tsx` server_rejected（NFR 1.2 反射なし assert）/ dialog 汎用文言
- 3.1 — `session_exchange_test.go` `TestExchangeAuthCodeForSession_Success` / `native_auth_handler_test.go` 204+Set-Cookie / registration hook happy path
- 3.2 — `passkey-signup-dialog.test.tsx` isSuccess→onOpenChange(false) / hook `invalidateQueries(["auth","me"])`（AuthGuard 再判定）
- 3.3 — 既存機能一式は無変更（NFR 2.1）。`login-page.test.tsx` 既存 4 テスト維持で回帰なし
- 3.4 — `use-passkey-registration.test.tsx` session_exchange_failed / `passkey-signup-dialog.tsx` L109-113 Dialog 閉じ
- 4.1 — `use-passkey-authentication.test.tsx` 正常系（`navigator.credentials.get`、username 送信なし）
- 4.2 — `session_exchange_test.go` / `native_auth_handler_test.go` Session 204+Cookie / auth hook happy path
- 4.3 — auth hook `invalidateQueries(["auth","me"])`（既存 AuthGuard で 2 ペイン UI 到達）
- 4.4 — username-less discoverable login が既存不変の `/api/passkey/authentication/*`（#216）を使用。Web 側に iOS/Web 作成 credential の区別なく 4.2 と同一コードパスで到達（design.md Traceability 4.4）
- 4.5 — `use-passkey-authentication.test.tsx` cancelled / `passkey-buttons.test.tsx` cancelled で alert 非表示 + reset
- 4.6 — `use-passkey-authentication.test.tsx` server_rejected（拒否理由の内部区別を反射せず uniform）
- 4.7 — `use-passkey-authentication.test.tsx` server_error / session_exchange_failed
- 5.1 — `passkey-capability.test.ts`（browser 判定 4 ケース）/ `use-passkey-capability.test.tsx` / `passkey-buttons.test.tsx` null 返却
- 5.2 — `use-passkey-capability.test.tsx` 404→available:false / `passkey_handler.go` Capability + `router.go` fail-closed nil gate
- 5.3 — `login-page.test.tsx`「capability false で Google のみ、パスキー導線 DOM 不在」
- 5.4 — `PasskeyButtons` は available:false で null 返却（パスキー操作トリガ自体が render されない）+ `passkey-buttons.test.tsx`
- 6.1 — `login-page.test.tsx` 既存 Google href / 表示位置テスト維持
- 6.2 — 既存 AuthGuard / AppShell 無変更
- 6.3 — 既存認証・UI・機能に破壊的変更なし（差分は追加のみ）
- 6.4 — `login-page.test.tsx` 既存 4 テスト green 維持（web 80 tests / go 全 pass 確認）
- NFR 1.1 — `session_exchange_test.go` `_DoesNotLeakPlainSecretsInError` / hooks は console・storage・URL に平文を残さない（実装 + テスト assert）
- NFR 1.2 — error kind enum のみ反射、`ApiError.body` を DOM/message に出さない（hooks + components + テスト assert）
- NFR 1.3 — CSP / sanitize 設定に変更なし（差分に該当ファイルなし）
- NFR 1.4 — `apiClient` 経由の同一オリジン相対パス（`API_BASE_URL=""`）
- NFR 2.1 — `router_test.go` NativeAuthHandler/PasskeyHandler nil 分岐（fail-closed）/ 既存挙動不変
- NFR 2.2 — `NATIVE_AUTH_JWT_SECRET` 未設定で /api/auth/session 未登録（404 縮退）テスト
- NFR 3.1 — 全テストが jsdom + mock / Go stub で外部依存なしに検証

## Findings

なし

## Summary

全 10 task の実装差分を develop..HEAD で確認。Requirement 1〜6 と NFR の全 numeric ID に対し観測可能な実装 + テストが存在し、変更ファイルはすべて各 task の `_Boundary_`（`app.go` wiring は task 2 の詳細・design File Structure Plan で明示）に収まり、境界逸脱なし。Go テスト全 pass・web パスキー関連 80 テスト green を再実行で確認。

参考（reject 理由ではない / informational）: `web/src/lib/api.ts` の `request<T>` が 204 No Content でも `response.json()` を無条件で呼ぶため、`POST /api/auth/session` の 204 応答に対し production では正常系 chain が失敗し得る。ただし `api.ts` は全 task の `_Boundary_` 外かつ design が「無変更」と明示しており、Developer 権限内で修正すると逆に境界逸脱になる spec/design gap（impl-notes「確認事項」に記載済み）。3 カテゴリ外のため本レビューでは reject 事由とせず、別 spec / PR での api.ts 204 handling 追加を人間 / PjM の判断に委ねる。

RESULT: approve
