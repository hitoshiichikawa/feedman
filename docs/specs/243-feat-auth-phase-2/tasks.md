# Implementation Plan

- [ ] 1. DB migration + recovery_codes model / repository + sessions.must_register_passkey 追加
  - `internal/database/migrations/20260817120000_add_recovery_codes.{up,down}.sql` を作成し、
    `recovery_codes` テーブル（`id / user_id FK CASCADE / code_hash / issued_at / used /
    used_at / UNIQUE(user_id, code_hash) / INDEX(user_id)`）を追加する
  - `internal/database/migrations/20260817120100_add_sessions_must_register_passkey.{up,down}.sql`
    を作成し、`sessions.must_register_passkey BOOLEAN NOT NULL DEFAULT false` を追加する
  - `internal/model/recovery.go` に `RecoveryCode` ドメイン型を追加し、`internal/model/user.go`
    の `Session` に `MustRegisterPasskey bool` フィールドを追加する
  - `internal/repository/interfaces.go` に `RecoveryCodeRepository` interface と
    `ErrRecoveryCodeNotUsable` を追加する
  - `internal/repository/postgres_recovery_code_repo.go` に `BulkInsertExec` /
    `FindByUserAndHash` / `MarkUsedExec` / `CountUnusedByUserID` / `LatestIssuedAtByUserID` /
    `DeleteByUserIDExec` を実装する（既存 `PostgresAuthCodeRepo` の scan / atomic UPDATE
    パターンを踏襲、hash / user_id をエラーメッセージに含めない）
  - `internal/repository/postgres_session_repo.go` の `Create` / `CreateExec` / `FindByID` を
    `must_register_passkey` 列に対応させ、`ClearMustRegisterPasskeyByID(ctx, sessionID)` を
    新設する（既存 caller は `false` を渡す形で無変更等価に保つ）
  - `internal/repository/postgres_recovery_code_repo_db_test.go` で bulk insert / atomic 単回
    消費 / FK CASCADE を検証する
  - _Requirements: 1.4, 3.3, 5.1, 5.2, 7.1, 7.2, 7.3, NFR 1.1, NFR 2.1, NFR 2.2, NFR 4.1_
  - _Boundary: RecoveryCodeRepository_

- [ ] 2. Recovery code generator + RecoveryCodeService の Generate / Status 実装
  - `internal/recovery/code.go` に (a) Crockford Base32 subset (`0-9A-Z` 除く `I/L/O/U`) の
    10 文字コードを N 個生成する `Generate(n int) ([]string, error)`（`crypto/rand` 利用、
    相互異なる保証は生成時 dedupe）、(b) 入力正規化 `Normalize(raw string) (string, error)`
    （hyphen/空白除去、lowercase→uppercase、許容文字集合検証、10 文字 length 検証、失敗時
    `ErrInvalidRecoveryCode`）を追加する
  - `internal/recovery/code_test.go` で相互異なる 10 個生成 / 文字集合 / 正規化ラウンドトリップ
    / 拒否ケースを検証する
  - `internal/recovery/service.go` に `RecoveryCodeService` 構造体と `Generate(ctx, userID)` /
    `Status(ctx, userID)` を実装する（`Generate` は 1 tx 内で `DeleteByUserIDExec` → 10 個の
    `BulkInsertExec` を実行し、Plain スライスを返して DB には SHA-256 hex のみ保存）
  - `internal/recovery/service_test.go` で `Generate` の全無効化 → 新規発行の順序、`Status` の
    未発行 / 発行済み分岐、生値がログ・エラー・戻り値以外に残らないことを検証する
  - _Requirements: 1.1, 1.3, 1.4, 5.2, 5.4, NFR 1.1, NFR 1.2, NFR 3.1, NFR 3.2, NFR 4.1_
  - _Boundary: RecoveryCodeService_
  - _Depends: 1_

- [ ] 3. RecoveryCodeService.Recover + 復旧 tx（全 credential/session/token 失効 + 復旧 session 発行）
  - `internal/recovery/service.go` に `Recover(ctx, rawUsername, rawCode)` を実装する
  - 手順: (1) `Normalize` で code 正規化、(2) `UserReader.FindByNormalizedUsername`、(3)
    `RecoveryCodeRepository.FindByUserAndHash`、いずれかの拒否事由でも `ErrRecoveryFailed` に
    uniform 化（early return は避け timing oracle を作らない）、(4) tx 開始 → `MarkUsedExec`
    相当（実装は該当行 DELETE で単回消費を成立させる）→ `PCRepo.DeleteByUserIDExec` /
    `RTRepo.DeleteByUserIDExec` / `ACRepo.DeleteByUserIDExec` / `SessionRepo.DeleteByUserIDExec` /
    `RCRepo.DeleteByUserIDExec` を順に呼び出し、(5) `SessionFactory.NewSession(userID)` +
    `SessionRepo.CreateExec(..., mustRegisterPasskey=true)` で復旧 session を発行、(6) commit
    後に返却
  - `internal/recovery/service_test.go` に (a) 未存在 user / 未検出 hash / used / 別 user /
    形式不正のすべてで `ErrRecoveryFailed` を返す、(b) 成功時に 5 repo の `Delete*Exec` が
    同一 tx で呼ばれ、SessionRepo.CreateExec の引数に `must_register_passkey=true` が渡る、
    (c) tx 途中失敗で全 rollback される、を検証する
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 4.1, 4.2, 4.6, 5.1, 5.3, 7.4, NFR 1.1, NFR 1.2, NFR 1.4, NFR 3.1, NFR 3.2, NFR 4.1_
  - _Boundary: RecoveryCodeService_
  - _Depends: 1, 2_

- [ ] 4. RecoveryHandler + router 登録 + wiring
  - `internal/handler/recovery_handler.go` に `Generate` (POST /api/recovery-codes/generate) /
    `Status` (GET /api/recovery-codes/status) / `Recover` (POST /api/recovery/session) の 3
    endpoint と `RecoveryHandlerOption`（Cookie 属性 / allowedOrigin 注入）を追加する
  - `Recover` の CSRF 対策として既存 `hasJSONContentType` / Origin allowlist を再利用し、
    Cookie は既存 `buildSessionCookie` で発行する
  - `internal/model/errors.go` に `ErrCodeRecoveryFailed` / `ErrCodeRecoveryRegistrationRequired`
    と対応する `NewRecoveryFailedError()` / `NewRecoveryRegistrationRequiredError()` を追加する
  - `internal/handler/router.go` の `RouterDeps` に `RecoveryHandler *RecoveryHandler` を追加
    し、認証必須グループに `POST /api/recovery-codes/generate` / `GET /api/recovery-codes/status`
    を、未認証グループに `unauthIPMW + MaxBodyBytes` を通して `POST /api/recovery/session` を
    登録する（PasskeyHandler と同じ縮退パターンで nil のときは登録しない = fail-closed）
  - `internal/app/app.go` に `recoveryCodeRepo` / `recoverySvc` / `recoveryHandler` を配線する
  - `internal/handler/recovery_handler_test.go` で 3 endpoint の 200 系 / 401 / 400
    RECOVERY_FAILED / 403 FORBIDDEN_ORIGIN / 415 / 500 のマッピングと、429 が既存
    `IPRateLimiter` で発火することを検証する
  - _Requirements: 1.1, 1.3, 1.5, 1.6, 3.1, 3.2, 3.4, 6.1, 6.2, 6.3, 6.4, NFR 1.2, NFR 1.4, NFR 2.2_
  - _Boundary: RecoveryHandler_
  - _Depends: 1, 2, 3_

- [ ] 5. RecoveryGateMiddleware + パスキー追加登録での復旧 session 解除 + 退会 tx への recovery_codes 削除段
  - `internal/middleware/recovery_gate.go` に `NewRecoveryGateMiddleware(sessionFinder)` を実装
    する（Session ミドルウェア通過後に session を再解決して `must_register_passkey=true` を
    検知したら `/api/passkey/registration/add/*` / `/api/users/me` /
    `/api/recovery-codes/status` 以外の全 API を 403 `RECOVERY_REGISTRATION_REQUIRED` で拒否）
  - `internal/middleware/session.go` に `SessionIDFromContext(ctx)` を追加し（`UserIDFromContext`
    と同型）、`SessionMiddleware` が cookie value を context に注入するよう拡張する
  - `internal/passkey/registration_service.go` の `RegistrationService` に
    `SessionMustRegisterPasskeyClearer`（`ClearMustRegisterPasskeyByID`）最小 interface を
    追加し、`FinishAddCredential` の credential INSERT 成功後に `SessionIDFromContext` から
    取得した ID で `Clear` を呼び出す
  - `internal/user/service.go` に `TxRecoveryCodeDeleter` interface を追加し、`withdrawTx` の
    削除順序を `item_states → subscriptions → sessions → recovery_codes → passkey_credentials
    → auth_codes → refresh_token_families → user` に更新する
  - `internal/handler/router.go` の認証必須グループの `BearerOrSession` 直後に
    `RecoveryGateMiddleware` を挿入する（`RecoveryHandler != nil` のときのみ）
  - `internal/app/app.go` の退会 tx wiring に `TxRecoveryCodeDeleter` を追加する
  - `internal/middleware/recovery_gate_test.go` で allowlist path 以外が 403、
    `must_register_passkey=false` では素通し、Bearer 経路（session 不在）では素通しを検証する
  - `internal/passkey/registration_service_test.go` の拡張で `FinishAddCredential` 成功時に
    `ClearMustRegisterPasskeyByID` が該当 session ID で呼ばれることを検証する
  - `internal/user/service_test.go` の拡張で退会 tx が `recovery_codes` を含む 7 段全てを呼び、
    途中失敗時に全 rollback することを検証する
  - _Requirements: 4.3, 4.4, 4.5, 7.1, 7.2, 7.3, NFR 2.1, NFR 2.2, NFR 2.3, NFR 4.1_
  - _Boundary: RecoveryGateMiddleware, RegistrationService, user.Service_
  - _Depends: 1, 4_

- [ ] 6. Web types + hooks（status / generate / recover）+ hooks テスト
  - `web/src/types/recovery.ts` に `RecoveryCodesStatus` / `RecoveryCodesGenerateResponse` /
    `RecoverySessionRequest` の型定義を追加する
  - `web/src/hooks/use-recovery-codes-status.ts` に `useRecoveryCodesStatus()` を実装する
    （`queryKey: ["recovery", "status"]`、401 は retry しない）
  - `web/src/hooks/use-generate-recovery-codes.ts` に `useGenerateRecoveryCodes()` mutation を
    実装する（成功時 `queryClient.invalidateQueries(["recovery", "status"])`）
  - `web/src/hooks/use-recover-with-code.ts` に `useRecoverWithCode()` mutation と
    `RecoveryLoginError` / `RecoveryLoginErrorKind` を実装する（既存 `PasskeyAuthError` /
    `classifyError` と同 idiom、生値は closure のみ・console 出力禁止）
  - `web/src/hooks/use-recovery-codes-status.test.tsx` / `use-generate-recovery-codes.test.tsx`
    / `use-recover-with-code.test.tsx` で成功 / 401 / 429 / 500 / network error のパスを検証、
    生値が `console.*` / `localStorage` / `sessionStorage` に到達しないことをスパイで確認する
  - _Requirements: 1.3, 2.1, 2.2, 3.1, 3.4, NFR 1.3, NFR 1.4, NFR 4.1_
  - _Boundary: useRecoveryCodesStatus, useGenerateRecoveryCodes, useRecoverWithCode_
  - _Depends: 4_

- [ ] 7. Web: RecoveryCodesSection + RecoveryCodesIssueDialog（発行 UI + 1 度限り全文表示）
  - `web/src/components/recovery-codes-section.tsx` に `<RecoveryCodesSection />` を実装する
    （`useRecoveryCodesStatus` の分岐で「未発行 + 発行ボタン」/「発行済み + 発行日時 + 残数 +
    再発行ボタン」の 2 面。生値の再表示は行わない / Req 1.3）
  - `web/src/components/recovery-codes-issue-dialog.tsx` に `<RecoveryCodesIssueDialog />` を
    実装する（コード一覧の等幅表示 / Clipboard API コピー / .txt ダウンロード（Blob +
    createObjectURL / close 時 revoke）/ 「安全な場所に保管しました」チェックボックスで close
    有効化 / 閉じたら親 state から生値を破棄）
  - `web/src/components/account-settings-dialog.tsx` の `AccountSettingsBody` に
    `<RecoveryCodesSection />` を `<PasskeyAddSection />` の直後で挿入する（既存 layout を
    崩さない / Req 7.3）
  - `web/src/components/recovery-codes-section.test.tsx` と `recovery-codes-issue-dialog.test.tsx`
    で状態別描画・1 度限り表示・チェックボックス gating・生値が DOM 以外の保存領域に流出
    しないことを検証する（既存 `passkey-add-section.test.tsx` と同 idiom）
  - _Requirements: 1.1, 1.2, 1.3, 5.4, 7.3, NFR 1.3, NFR 4.1_
  - _Boundary: RecoveryCodesSection, RecoveryCodesIssueDialog_
  - _Depends: 6_

- [ ] 8. Web: RecoveryReminderBanner + AppShell 統合
  - `web/src/components/recovery-reminder-banner.tsx` に `<RecoveryReminderBanner />` を実装
    する（`useRecoveryCodesStatus` で `issued=false` かつ `mustRegisterPasskey=false` のときの
    み描画、機密情報を含まない固定文言、「発行画面を開く」ボタンで
    `AccountSettingsDialog` を open）
  - `AccountSettingsDialog` に外部 `open` / `onOpenChange` prop を追加（既存 internal state と
    後方互換で共存させる）または `useAppState` に `settingsOpen` を追加して共有する。どちらか
    を choose し、既存呼び出し箇所を破壊しない
  - `web/src/components/app-shell.tsx` の最上部に `<RecoveryReminderBanner />` を配置する
    （既存 2 ペイン layout は不変 / Req 7.3）
  - `web/src/components/recovery-reminder-banner.test.tsx` で `issued=true` 時に非表示、
    `issued=false` 時に表示、`mustRegisterPasskey=true` 時に非表示、リンククリックで
    `AccountSettingsDialog` open callback が発火することを検証する
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 7.3, NFR 4.1_
  - _Boundary: RecoveryReminderBanner_
  - _Depends: 6, 7_

- [ ] 9. Web: RecoveryLoginForm + LoginPage への復旧導線追加
  - `web/src/components/recovery-login-form.tsx` に `<RecoveryLoginForm />` を実装する
    （username + recovery_code の入力、`useRecoverWithCode` mutation 起動、送信中の disable、
    拒否時に `RecoveryLoginError.kind` を固定文言に射影して alert 表示、成功時に
    `queryClient.invalidateQueries({queryKey: ["auth", "me"]})` で AuthGuard 再判定）
  - `web/src/components/login-page.tsx` に「リカバリコードで復旧」ボタンを追加し、クリックで
    `<RecoveryLoginForm />` を展開する（既存 Google / パスキーボタンは不変 / Req 7.3）
  - `web/src/components/recovery-login-form.test.tsx` と `login-page.test.tsx` の拡張で、
    フォーム描画 / 送信 / 拒否時の固定文言 / 内部 body 非反射 / 成功時の invalidate 発火を
    検証する（既存 `login-page-recovery.test.tsx` と衝突しないファイル名を選ぶ）
  - _Requirements: 3.1, 3.4, 3.5, 3.6, 4.2, 7.3, NFR 1.4, NFR 4.1_
  - _Boundary: RecoveryLoginForm_
  - _Depends: 6_

- [ ] 10. Web: RecoveryRegistrationGate + AppShell ラップ + 復旧完了 → 通常 UI 遷移
  - `web/src/components/recovery-registration-gate.tsx` に `<RecoveryRegistrationGate>` を実装
    する（`useRecoveryCodesStatus` の `mustRegisterPasskey=true` を検知したら children を差し
    替えて中央に「復旧処理が完了しました。新しいパスキーを登録してください」文言 + 既存
    `<PasskeyAddSection />` のみを描画。ログアウト・退会・フィード操作等の導線は表示しない /
    Req 4.5）
  - `AppShell` の children レンダリングを `<RecoveryRegistrationGate>...</RecoveryRegistrationGate>`
    でラップする（通常状態では pass-through / Req 7.3）
  - `PasskeyAddSection` の mutation 成功時に `queryClient.invalidateQueries({queryKey:
    ["recovery", "status"]})` が発火するよう `usePasskeyAddRegistration` に副作用を追加する
    （または gate 側で invalidate を仕込む）
  - `web/src/components/recovery-registration-gate.test.tsx` で (a) `mustRegisterPasskey=true`
    時に 2 ペイン UI ではなく登録画面のみ描画、(b) 登録成功で `useRecoveryCodesStatus` が
    invalidate されて通常 UI に遷移、(c) `mustRegisterPasskey=false` 時は pass-through、を
    検証する
  - _Requirements: 4.3, 4.4, 4.5, NFR 4.1_
  - _Boundary: RecoveryRegistrationGate_
  - _Depends: 6, 8_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行する build/test/lint コマンドを以下の
構造化ブロックで宣言する。Go / TypeScript 両系統の build・test を通し、既存契約テスト（Req 7.x
/ NFR 2.3）と新規追加テストが同時に green であることを確認する。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./... && cd web && npm test
```
