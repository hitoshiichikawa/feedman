# Implementation Plan

## 実装順序の指針

タスクは「永続化層 → ドメイン層 → adapter 境界 → service（登録 / 認証） → E2E 回帰」の順に
下から積み上げる。各タスクは独立してコミット可能で、コンパイル・既存テストを壊さないように
`_Depends:_` を明示する。behavior-changing task には対応する regression / 単体テスト追加を
同タスク内に含める（`_Requirements:_` に列挙した AC のテストを task 内で完結させる）。

- [x] 1. `passkey_credentials` に BE/BS 列を追加する migration と schema regression テスト更新
  - `internal/database/migrations/20260728120000_add_passkey_credential_backup_flags.up.sql` を新規作成
    - `ALTER TABLE passkey_credentials ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT false,
      ADD COLUMN backup_state BOOLEAN NOT NULL DEFAULT false`
    - 既存列の削除・型変更・rename は行わない（NFR 1.1）
    - 追加インデックスは張らない
  - `internal/database/migrations/20260728120000_add_passkey_credential_backup_flags.down.sql` を新規作成
    - `ALTER TABLE passkey_credentials DROP COLUMN IF EXISTS backup_state, DROP COLUMN IF EXISTS backup_eligible`
    - 既存データを不変で復元する（既存 down migration と同流儀）
  - `internal/repository/postgres_passkey_credential_repo_db_test.go` Case 8
    「保存対象が検証情報のみに限定される_NFR1.1回帰」の `want` map に `backup_eligible` /
    `backup_state` を追加（migration 適用後に列数チェックが green を維持する regression）
  - _Requirements: NFR 1.1, NFR 1.2, NFR 1.3_

- [x] 2. Repository と domain model に BE/BS を通す
  - `internal/model/passkey.go` の `PasskeyCredential` に `BackupEligible bool` /
    `BackupState bool` を追加（doc comment に「Req 4.3: BE は認証成功時に上書き更新しない」旨を明記）
  - `internal/repository/interfaces.go` の `PasskeyCredentialRepository` から
    `UpdateSignCount(id, signCount, lastUsedAt) error` を除去し、
    `UpdateAuthenticationState(ctx, id, signCount, backupState, lastUsedAt) error` を追加
    （doc comment に「backup_eligible は UPDATE 対象に含めない = Req 4.3 保証」を明記）
  - `internal/repository/postgres_passkey_credential_repo.go` を修正
    - `CreateExec` の INSERT 列並びと VALUES に `backup_eligible, backup_state` を追加、Go 側は
      `c.BackupEligible, c.BackupState` をバインド
    - `FindByCredentialID` / `ListByUserID` の SELECT 列並びに 2 列を追加し、`Scan` で `*bool` を
      受けて `c.BackupEligible` / `c.BackupState` に反映（scan ヘルパー抽出は本 spec で行わない：
      既存インラインパターンを踏襲）
    - `UpdateSignCount` を除去し `UpdateAuthenticationState` を実装
      （`UPDATE passkey_credentials SET sign_count = $2, backup_state = $3, last_used_at = $4 WHERE id = $1`。
      `backup_eligible` を SET 句に含めない）
    - エラーメッセージには credential_id / public_key / user_id 等の機密値を含めない（NFR 1.2 継承）
  - `internal/repository/postgres_passkey_credential_repo_db_test.go` を修正
    - Case 1 (`Create_FindByCredentialIDで保存値が同値復元される`) に BE=true / BS=true を含む
      round-trip 検証を追加（保存値と復元値が同値）
    - Case 5（旧 `UpdateSignCount_sign_countとlast_used_atが反映される`）を
      `UpdateAuthenticationState_sign_count_backup_state_last_used_atが反映される` に更新
      （SignCount + BackupState + LastUsedAt の反映確認、`BackupEligible` は不変であることを assert）
    - 追加検証: 旧 credential 行（BE/BS 列がゼロ値のまま）を直接 INSERT した後 FindByCredentialID
      で BE=false / BS=false のまま読み出されエラーにならないこと（NFR 1.3 regression）
  - _Requirements: 2.1, 4.1, 4.2, 4.3, NFR 1.2, NFR 1.3_
  - _Boundary: PasskeyCredentialRepository, PasskeyCredentialModel_
  - _Depends: 1_

- [x] 3. WebAuthn adapter に BE/BS の双方向 propagate を実装
  - `internal/passkey/webauthn_adapter.go` を修正
    - `ParsedCredential` に `BackupEligible bool` / `BackupState bool` を追加
    - `toParsedCredential` で `cred.Flags.BackupEligible` / `cred.Flags.BackupState` を propagate
    - `WebAuthnAdapter.FinishLogin` の戻り値に `updatedBackupState bool` を追加
      （位置は `(userHandle, credentialID, updatedSignCount, updatedBackupState, err)`）
    - `GoWebAuthnAdapter.FinishLogin` 実装で `cred.Flags.BackupState`（library が
      `ValidateDiscoverableLogin` 成功後に `NewCredentialFlags(assertion.AuthenticatorData.Flags)`
      で更新した値）を `updatedBackupState` に代入
    - doc comment に「library の login validation が BE 一致を要求するため、service 層は lookup で
      stored BE/BS を反映した `webauthn.Credential.Flags` を返す責務を持つ」旨を追記
  - `internal/passkey/authentication_service.go` の `FinishLogin` 呼び出し箇所を新戻り値対応に
    差し替える（本タスクでは受け取った `updatedBackupState` を `_` で捨てる暫定接続。Task 5 で
    `UpdateAuthenticationState` へ渡す実配線に切り替える）
  - `internal/passkey/authentication_service_test.go` の `stubAuthnAdapter.FinishLogin` および
    `finishLoginFn` のシグネチャを新戻り値対応に更新（テストコードのコンパイルを維持）
  - `internal/passkey/webauthn_adapter_test.go` を修正
    - `TestWebAuthnAdapter_RegisterAndLoginRoundTrip` の既存 assert に加え、
      `parsed.BackupEligible=false` / `parsed.BackupState=false`（default synthetic）を確認
    - 新規テスト `TestWebAuthnAdapter_RegisterAndLoginRoundTrip_BackupEligible` を追加
      （`authenticator.Options.BackupEligible = true` + `BackupState = true` に設定し、
      `parsed.BackupEligible=true` / `updatedBackupState=true` を検証）
    - 既存の `TestWebAuthnAdapter_FinishLogin_DetectsCounterRegression` および malformed 系テストを
      新戻り値対応に更新
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 4.2, NFR 2.1_
  - _Boundary: WebAuthnAdapter, AuthenticationServiceCompileGlue_
  - _Depends: 2_

- [ ] 4. Registration service で BE/BS を永続化
  - `internal/passkey/registration_service.go` を修正
    - `FinishRegistrationNew` 内の `&model.PasskeyCredential{...}` リテラルに
      `BackupEligible: parsed.BackupEligible` / `BackupState: parsed.BackupState` を追加
    - `FinishAddCredential` 内の同様のリテラルにも 2 フィールドを追加
    - 既存 1-tx オーケストレーション（`RegistrationTxBeginner` + `CreateUserOnlyExec` +
      `CreateExec` + optional `sessions.CreateExec` + `tx.Commit`）は無変更。BE/BS 列追加による
      新規失敗経路は発生しない（NOT NULL DEFAULT により INSERT 側でエラー要因を持たない）
  - `internal/passkey/registration_service_test.go` を修正
    - `TestFinishRegistrationNew` 系テストで、stub adapter が返す `ParsedCredential` に
      `BackupEligible=true` / `BackupState=true` を含めるケースを追加
    - stub credential writer（`CreateExec` を捕捉する既存 stub）が受け取る
      `*model.PasskeyCredential` の BE/BS が true であることを assert（Req 1.1, 1.3）
    - BE=false / BS=false ケースも独立 subtest で追加し assert（Req 1.4）
    - `TestFinishAddCredential` 系テストにも同様の subtest を追加（Req 1.2, 1.3, 1.4）
    - Req 1.5 は既存 tx rollback テストが引き続き通ることを確認（新規失敗経路は導入しないため
      既存 assertion で足りる。追加テストは不要）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, NFR 2.1_
  - _Boundary: RegistrationService_
  - _Depends: 3_

- [ ] 5. Authentication service で stored BE/BS を lookup 反映し BS を最新化
  - `internal/passkey/authentication_service.go` を修正
    - `lookup` closure 内の `webauthn.Credential` 組み立てに
      `Flags: webauthn.CredentialFlags{BackupEligible: cred.BackupEligible, BackupState: cred.BackupState}`
      を追加（UserPresent / UserVerified はゼロ値のまま。library は login validation で BE のみ
      比較するため / login.go:371）
    - Task 3 で `_` で受けていた `updatedBackupState` を実受け取りに切替
    - 既存 `credentials.UpdateSignCount(ctx, resolvedCred.ID, updatedSignCount, s.now())` を
      `credentials.UpdateAuthenticationState(ctx, resolvedCred.ID, updatedSignCount, updatedBackupState, s.now())`
      に差し替え（Req 4.2 / Req 4.3 を interface レベルで担保）
    - 拒否時ログの `logRejection("passkey authentication finish rejected: webauthn assertion", ...)`
      は無変更（NFR 2.1 / 2.2 / 2.3 継承）
  - `internal/passkey/authentication_service_test.go` を修正
    - `stubCredentialReader` の `UpdateSignCount` メソッドを
      `UpdateAuthenticationState(ctx, id, signCount, backupState, lastUsedAt)` に置換し、
      `lastUpdatedBackupState bool` を捕捉するフィールドを追加
    - 既存テスト「成功時に UpdateSignCount が正しい引数で呼ばれる」を
      「成功時に UpdateAuthenticationState が正しい引数で呼ばれる」に更新（`backupState` が
      `updatedBackupState` と一致することを assert / Req 4.2）
    - 新規テスト: `stubAuthnAdapter.finishLoginFn` 内で `lookup` を呼び出し、返された
      `WebAuthnUser.WebAuthnCredentials()[0].Flags.BackupEligible` および `BackupState` が
      stored 値（stub credential reader で BE=true / BS=true を返した場合）と一致することを検証
      （Req 2.1, 2.2, 2.3。lookup で反映されない regression を CI で検知する保険）
    - 新規テスト: stub adapter が `updatedBackupState=true` を返した際に
      `UpdateAuthenticationState` に `backupState=true` が渡ることを検証（Req 4.2）
    - 新規テスト: 拒否時（stub adapter が `ErrAuthenticationFailed` を返す）に
      `UpdateAuthenticationState` が呼ばれないことを検証（既存の「UpdateSignCount must not be
      called on rejected assertion」を新 method 名で維持 / Req 4.1 / 4.3）
    - BE 不一致による library 拒否（Req 3.4）は adapter レベルで `ErrAuthenticationFailed` に
      正規化されるため、service レベルでは stub が sentinel を返す既存経路で表現する
      （実際の library 動作は Task 6 の E2E で検証）
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 3.4, 3.5, 4.1, 4.2, 4.3, 5.3, NFR 2.1, NFR 2.2, NFR 2.3_
  - _Boundary: AuthenticationService_
  - _Depends: 4_

- [ ] 6. E2E DB-backed regression: BE=1 全動線と BE=0 baseline の同時 green を担保
  - `internal/handler/passkey_e2e_db_test.go` を修正
    - 既存 `TestE2E_PasskeyFullFlow_DBBacked` は無変更で維持し、本修正後も BE=0 baseline で
      green を保持することを CI で確認する（Req 3.3, 5.2, 5.4）
    - 新規テスト `TestE2E_PasskeyFullFlow_DBBacked_BackupEligible` を追加
      - `authenticator := virtualwebauthn.NewAuthenticator()` の直後に
        `authenticator.Options.BackupEligible = true` と `authenticator.Options.BackupState = true` を設定
      - 既存 `TestE2E_PasskeyFullFlow_DBBacked` と同一の 7 ステップ（登録 begin → 登録 finish →
        認証 begin → 認証 finish → auth_code 発行 → token 交換 → Bearer で保護 API 到達）を通す
      - DB sanity として `SELECT backup_eligible, backup_state FROM passkey_credentials WHERE user_id = $1`
        で登録直後に BE=true / BS=true が保存されていることを assert（Req 1.1, 1.3）
      - 認証 finish 後に `backup_state` が最新観測値と一致すること、`backup_eligible` が true
        のまま不変であることを DB から SELECT して assert（Req 4.2, 4.3）
      - Req 3.1 / 3.2: 同一 lookup 経路を Web / native 両クライアントが共有するため、本 E2E で
        Web 経路を通せば native 経路も同一修正で成立する旨をテストコメントに残す
    - 新規テスト（optional / uniform reject 側の regression）:
      `TestE2E_PasskeyFullFlow_DBBacked_BackupEligibleMismatch` を追加し、BE=1 で登録した後に
      BE=0 の synthetic authenticator で assertion を返すと 400 uniform 拒否となることを assert
      （Req 3.4, 5.3。実装コストが高い場合は Task 5 の adapter integration 経路にとどめ、
      本 subtest は deferrable として task 6.1 に分離してよい）
  - _Requirements: 3.1, 3.2, 3.3, 5.1, 5.2, 5.4_
  - _Boundary: PasskeyE2ETestSuite_
  - _Depends: 5_

- [ ]* 6.1 uniform reject 側の追加 E2E regression（deferrable）
  - `TestE2E_PasskeyFullFlow_DBBacked_BackupEligibleMismatch` を独立 test 関数として実装
  - virtualwebauthn 上で BE=1 authenticator による登録後、BE=0 の別 authenticator で assertion を返すと
    HTTP 400 uniform 拒否が返り、DB 上の `backup_eligible` / `backup_state` が変更されていないことを assert
  - _Requirements: 3.4, 5.3_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。migration 適用と Postgres 結合テスト（`TEST_DATABASE_URL` 経由）を
含む passkey 関連パッケージのユニット・結合・E2E を全て回し、`go vet` と `gofmt` 差分ゼロで
締める。

<!-- stage-a-verify -->
```sh
go test ./internal/model/... ./internal/repository/... ./internal/passkey/... ./internal/handler/... && go vet ./... && test -z "$(gofmt -l internal/)"
```
