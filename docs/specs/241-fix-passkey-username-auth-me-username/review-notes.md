# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-07-29T17:49:00Z -->

## Reviewed Scope

- Branch: claude/issue-241-impl-fix-passkey-username-auth-me-username
- HEAD commit: 6b3341f95d3e9f1079df58b9f1a5bf7190efa251
- Compared to: develop..HEAD

## Feature Flag Protocol

- CLAUDE.md `## Feature Flag Protocol` の `**採否**: opt-out` を確認。flag 観点の細目判定は
  適用せず、通常の 3 カテゴリ（AC 未カバー / missing test / boundary 逸脱）で判定した。

## Verified Requirements

- 1.1 — `registration_service.go` `FinishRegistrationNew` の `newUser` に `Name: normalized` を追加。
  Unit: `registration_service_test.go` 成功サブテストに `users.lastCreated.Name != pendingUsername`
  assert / DB-backed: `postgres_passkey_registration_tx_db_test.go` `found.Name != normalized` assert
- 1.2 — INSERT 値を 1 つ追加するのみで `Username` / `UsernameNormalized` の代入は無変更。既存
  `UsernameNormalized` assert が引き続き pass（`registration_service_test.go`）
- 1.3 — 失敗系 3 サブテスト（session factory 失敗 / credential 重複 / infra 障害）が
  `commitCalled=0` / `rollbackCalled=1` を検証。Name 追加後も Rollback 経路で users 行非永続化を担保
- 1.4 — `Name: normalized` は既存 tx オーケストレーション（`BeginTx → CreateUserOnlyExec → Commit`）
  の `newUser` に含まれ、追加 tx 制御なしで同一トランザクション内に含まれる
- 2.1 — `auth_handler.go` `meResponse` struct `Username *string json:"username"`（omitempty なし）。
  `Cookie_Present_UsernameUnset_ReturnsNull` がキー存在を検証
- 2.2 — handler が `user.Username != ""` 時に `*string` を assign。
  `Cookie_Present_UsernameSet_ReturnsUsernameString` が string 型 "alice" を検証
- 2.3 — 空文字時に nil を assign → JSON null。`Cookie_Present_UsernameUnset_ReturnsNull` が nil を検証
- 2.4 — `id/email/name` の field tag / 型を維持。`Cookie_Present_ReturnsExistingShape` が継続検証
- 2.5 — 401 経路（`http.Error`）・200 経路の分岐は無変更。`NoCookie_ReturnsUnauthorized` 無変更で pass
- 2.6 — Cookie 不在時 401 text/plain のみ。`NoCookie_ReturnsUnauthorized` で担保
- 3.1 — 既存 `account-info-name` 表示ロジック無変更。`account-settings-dialog.test.tsx` の既存/新規
  テストが `Alice` / `Passkey User` / `Bob` の表示名を検証
- 3.2 — `AccountInfoSection` に `account-info-username` span を条件描画。
  `username が非 null / 非空のとき...` テストがラベル「ユーザー名」+ 値表示を検証
- 3.3 — `user.username != null && user.username !== ""` 条件描画。null / 空文字の 2 テストが
  `queryByTestId("account-info-username")` 非存在を検証
- 3.4 — 既存 testID（name / email / email-unset / withdraw 系）の描画ロジックは無変更。
  新規テストが既存 name / email 表示の維持を assert
- 4.1 — `Cookie_Present_UsernameUnset_ReturnsNull`（Google 由来相当）が id/email/name を既存通り返し
  username のみ null を検証
- 4.2 — `username が null のとき...` テストが email 未設定プレースホルダ維持を検証
- 4.3 — Google OAuth 経路（`auth/service.go` / `CreateWithIdentity`）は diff に含まれず無変更
- NFR 1.1 — `meResponse` field tag `id/email/name` を既存と厳密一致。`Cookie_Present_ReturnsExistingShape`
  が継続保護
- NFR 1.2 — `Content-Type: application/json` 設定行は無変更。新規 2 サブテストが Content-Type を検証
- NFR 2.1 — `Cookie_Present_ReturnsExistingShape` の forbidden set / allowed set 厳密検査を維持
  （struct 化によりキー漏出を構造的に排除）
- NFR 2.2 — `registration_service.go` の変更は `newUser` 構築 1 箇所のみ。新規ログ追加なし
- NFR 3.1 — INSERT の VALUES 追加のみで UPDATE / migration script を追加していない

## Boundary Check

- Task 1（`_Boundary: passkey.RegistrationService, PostgresPasskeyRegistrationTx_`）:
  `internal/passkey/registration_service.go` / `..._test.go` /
  `internal/repository/postgres_passkey_registration_tx_db_test.go` のみ変更 — 境界内
- Task 2（`_Boundary: AuthHandler_`）: `internal/handler/auth_handler.go` / `..._test.go` のみ変更 — 境界内
- Task 3（`_Boundary: types/auth.ts, useCurrentUser hook_`）: `web/src/types/auth.ts` /
  `web/src/hooks/use-auth.test.tsx` のみ変更 — 境界内
- Task 4（`_Boundary: AccountSettingsDialog_`）: `web/src/components/account-settings-dialog.tsx` /
  `..._test.tsx` のみ変更 — 境界内
- `tasks.md`（checkbox `- [ ]` → `- [x]` の進捗追跡）/ `impl-notes.md` は Developer の progress
  tracking / 補足記録であり spec 本文の書き換えではない — 逸脱なし

## Test Execution

- `go test ./internal/handler/... ./internal/passkey/...`: ok（pass）
- `npx vitest run account-settings-dialog.test.tsx use-auth.test.tsx`: 2 files / 23 tests passed

## Findings

なし

## Summary

Req 1〜4 の全 numeric ID および NFR 1〜3 が実装 + 対応テストでカバーされ、変更ファイルは
全 task の `_Boundary:_` 内に収まっている。Go / Web ともにテスト green。boundary 逸脱・AC 未カバー・
missing test のいずれも検出されなかった。

RESULT: approve
