# Implementation Plan

## 実装順序の指針

各タスクは独立コミット可能な粒度で、下から上へ（永続化層 → ドメイン層 → HTTP 層 → wiring）
積み上げる方針とする。並列可能（`(P)`）を明示したタスクは異なる `_Boundary:_` を担当する。

- [ ] 1. マイグレーション追加とドメインモデル / username 正規化の導入
  - `internal/database/migrations/20260724120000_add_passkey_tables.up.sql` を新規作成
    - `passkey_credentials` テーブル（id / user_id / credential_id UNIQUE / public_key /
      sign_count / attestation_type / aaguid / transports / created_at / last_used_at）
    - `passkey_challenges` テーブル（id / challenge_hash UNIQUE / kind / user_id nullable /
      pending_username / session_data / expires_at / consumed / created_at）
    - `users` に `username VARCHAR(64) NULL` / `username_normalized VARCHAR(64) NULL` を追加
    - `CREATE UNIQUE INDEX idx_users_username_normalized ON users(username_normalized) WHERE username_normalized IS NOT NULL`
    - 補助インデックス（`idx_passkey_credentials_user_id` / `idx_passkey_challenges_expires_at`）
  - `internal/database/migrations/20260724120000_add_passkey_tables.down.sql` を新規作成
    （テーブル DROP + カラム DROP + インデックス DROP。既存データを不変で復元）
  - `internal/model/passkey.go` を新規作成し `PasskeyCredential` / `PasskeyChallenge` /
    `PasskeyChallengeKind` を定義
  - `internal/model/user.go` の `User` 構造体に `Username string` / `UsernameNormalized string`
    を追加（既存 `Email` / `Name` / `CreatedAt` / `UpdatedAt` は無変更）
  - `internal/passkey/username.go` を新規作成し `ValidateAndNormalize(raw) (normalized, err)` を
    実装（`ErrInvalidUsername` sentinel、`[a-zA-Z0-9_-]` の 3〜32 文字、lowercase 正規化）
  - `internal/passkey/username_test.go` で境界値テスト（空 / 2 / 3 / 32 / 33 文字 / Unicode /
    制御文字 / 空白混在）を追加
  - _Requirements: 1.4, 1.5, 4.1, 4.2, 4.5, NFR 1.1, NFR 2.1_
  - _Boundary: MigrationSchema, PasskeyModel, UsernameValidator_

- [ ] 2. Repository 層の追加（credential / challenge / users 拡張）
  - `internal/repository/interfaces.go` に以下を追加
    - `PasskeyCredentialRepository` interface（Create / FindByCredentialID / ListByUserID /
      UpdateSignCount / DeleteByUserID / DeleteByUserIDExec）
    - `PasskeyChallengeRepository` interface（Create / FindByHash / MarkConsumed）
    - `ErrChallengeNotUsable` / `ErrCredentialAlreadyRegistered` / `ErrUsernameTaken` sentinel
    - `UserRepository` を拡張し `FindByNormalizedUsername` / `CreateUserOnly` を追加
      （既存 `FindByID` / `CreateWithIdentity` / `DeleteByID` は破壊的変更しない）
  - `internal/repository/postgres_passkey_credential_repo.go` を新規作成し
    `PostgresPasskeyCredentialRepo` を実装（credential_id UNIQUE 衝突を
    `ErrCredentialAlreadyRegistered` に変換、`DeleteByUserIDExec(ctx, q, userID)` は既存
    `PostgresAuthCodeRepo.DeleteByUserIDExec` と同型）
  - `internal/repository/postgres_passkey_challenge_repo.go` を新規作成し
    `PostgresPasskeyChallengeRepo` を実装（`MarkConsumed` は
    `UPDATE ... WHERE id=$1 AND consumed=false AND expires_at > now()` で atomic 判定、
    0 rows → `ErrChallengeNotUsable`）
  - `internal/repository/postgres_user_repo.go` に `FindByNormalizedUsername` /
    `CreateUserOnly`（identity 無し・users のみ INSERT）を追加、`username_normalized` UNIQUE
    衝突を `ErrUsernameTaken` に変換
  - `internal/repository/postgres_passkey_credential_repo_db_test.go` を新規作成しテスト用
    PostgreSQL で以下を検証（既存 `postgres_auth_code_repo_db_test.go` と同流儀）:
    Create 成功 / credential_id UNIQUE 衝突 / FindByCredentialID / ListByUserID /
    UpdateSignCount / DeleteByUserID の 0 件残存
  - `internal/repository/postgres_passkey_challenge_repo_db_test.go` を新規作成:
    Create / FindByHash / MarkConsumed（成功・二重消費で 0 rows → sentinel・期限切れで
    0 rows → sentinel）
  - `internal/repository/postgres_user_repo_test.go` を拡張し `FindByNormalizedUsername` /
    `CreateUserOnly` / username UNIQUE 衝突を検証
  - _Requirements: 1.2, 1.3, 1.4, 3.2, 3.4, 3.6, 4.1, 4.2, 4.3, 4.4, NFR 1.1, NFR 1.2_
  - _Boundary: PasskeyCredentialRepository, PasskeyChallengeRepository, UserRepository_
  - _Depends: 1_

- [ ] 3. ChallengeStore と WebAuthnAdapter の実装
  - `internal/passkey/errors.go` を新規作成し `ErrInvalidUsername` / `ErrUsernameTaken` /
    `ErrRegistrationFailed` / `ErrAuthenticationFailed` / `ErrChallengeNotUsable` sentinel を集約
  - `go.mod` に `github.com/go-webauthn/webauthn` を追加（`go get github.com/go-webauthn/webauthn@latest`）
  - `internal/passkey/webauthn_adapter.go` を新規作成
    - `WebAuthnAdapter` interface（`BeginRegistration` / `FinishRegistration` / `BeginLogin` /
      `FinishLogin` / `WebAuthnUser` / `ParsedCredential`）を定義
    - `GoWebAuthnAdapter` を実装（`webauthn.New(config)` を保持し、上記 4 メソッドを wrap）
    - 内部の `webauthn.Config` は Adapter 生成時に RPID / RPDisplayName / RPOrigins を固定
    - `FinishLogin` は counter 後退（`Authenticator.CloneWarning`）を `ErrAuthenticationFailed` に変換
    - 平文 request body / attestation を `slog` に出さない
  - `internal/passkey/challenge_store.go` を新規作成
    - `ChallengeStore` struct（`repo` / `ttl` / `now`）
    - `Issue(ctx, kind, userID, pendingUsername, sessionData, rawChallenge) (challengeID, err)`:
      `HashNativeSecret` で hash 化して `PasskeyChallengeRepository.Create`
    - `Consume(ctx, challengeID, expectedKind) (*model.PasskeyChallenge, error)`:
      FindByHash（内部で id → hash 逆引きが必要なため、id で FindByID メソッドを challenge repo
      に追加）→ TTL / kind / consumed を検証 → `MarkConsumed` を atomic 呼び出し
    - **repo interface の追加調整**: challenge_id での lookup 用に `FindByID` を task 2 の
      interface に追加する（task 2 スコープ調整）
  - `internal/passkey/challenge_store_test.go` を新規作成（stub repo で発行 → 消費、期限切れ、
    kind 不一致、二重消費、consumed 中の失敗を検証）
  - `internal/passkey/webauthn_adapter_test.go` を新規作成（synthetic authenticator で register
    → login round trip を検証、counter 後退で `ErrAuthenticationFailed`。外部ネットワーク非依存
    / NFR 4.1）
  - _Requirements: 1.1, 1.2, 2.1, 2.2, 2.5, 3.1, 3.2, 4.1, 4.2, 4.3, 4.4, 4.5, NFR 1.4, NFR 4.1_
  - _Boundary: WebAuthnAdapter, ChallengeStore_
  - _Depends: 2_

- [ ] 4. RegistrationService の実装（新規登録 + 追加登録）
  - `internal/passkey/registration_service.go` を新規作成
    - `RegistrationService` struct（依存: `WebAuthnAdapter` / `ChallengeStore` /
      `UserWriter`（最小 IF）/ `PasskeyCredentialWriter`（最小 IF）/ `now func() time.Time`）
    - `BeginRegistrationNew(ctx, rawUsername, optionalEmail, codeChallenge)`:
      username 検証 → 正規化 → `FindByNormalizedUsername` で重複チェック（`ErrUsernameTaken`）
      → `WebAuthnAdapter.BeginRegistration`（user handle には仮 UUID を使用、finish 時に
      user 行を確定）→ `ChallengeStore.Issue(kind=registration_new, pendingUsername=&normalized)`
      → returns (challengeID, optionsJSON)
      ※ codeChallenge の検証は既存 `auth.ValidatePKCES256` を呼び、challenge メタとして
      session_data の JSON に含める（後段 auth_code 発行時に PKCE 継承）
    - `FinishRegistrationNew(ctx, challengeID, requestBody)`:
      `ChallengeStore.Consume(challengeID, kind=registration_new)` →
      `WebAuthnAdapter.FinishRegistration(user, sessionData, requestBody)` → user 行 INSERT
      (`CreateUserOnly` / username UNIQUE 衝突は `ErrRegistrationFailed`) → credential INSERT
      (`ErrCredentialAlreadyRegistered` は `ErrRegistrationFailed` に正規化)
    - `BeginAddCredential(ctx, authenticatedUserID)`:
      `FindByID` で既存 user 取得 → `ListByUserID` で既存 credential 取得 →
      `WebAuthnAdapter.BeginRegistration(excludeCredentials=[]byte)` →
      `ChallengeStore.Issue(kind=registration_add, userID=&authenticatedUserID)`
    - `FinishAddCredential(ctx, authenticatedUserID, challengeID, requestBody)`:
      `ChallengeStore.Consume(challengeID, kind=registration_add)` → 紐付 userID と
      `authenticatedUserID` の一致を検証（不一致 → `ErrRegistrationFailed`）→
      `FindByCredentialID` で他 user 既登録なら `ErrRegistrationFailed`（Req 3.6）→
      `WebAuthnAdapter.FinishRegistration` → credential INSERT（同 user 複数登録可 / Req 3.4）
    - 拒否は全て `ErrRegistrationFailed` に正規化（Req 1.7 / 3.7）。ログは slog.Warn で内部詳細のみ
      （NFR 1.2 / 1.3 / 3.2）
  - `internal/passkey/registration_service_test.go` を新規作成し以下を網羅（WebAuthnAdapter /
    ChallengeStore / User / Credential IF を全てモック / スタブ化、NFR 4.1）:
    新規: 成功、username 形式不正 → `ErrInvalidUsername`、username 重複 → `ErrUsernameTaken`、
    challenge 期限切れ → `ErrRegistrationFailed`、attestation 失敗 → `ErrRegistrationFailed`、
    email 未指定でも user 行作成（Req 1.6）
    追加: 成功（別 credential 追加）、未認証 (BearerOrSession が返す 401 は handler テストで検証)、
    別 user credential 提示 → `ErrRegistrationFailed`、同一 user 複数登録許可
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 3.1, 3.2, 3.3, 3.4, 3.6, 3.7, NFR 1.2, NFR 1.3, NFR 3.1, NFR 3.2, NFR 4.1_
  - _Boundary: RegistrationService_
  - _Depends: 3_

- [ ] 5. AuthenticationService の実装（既存 auth_code 発行への合流）
  - `internal/passkey/authentication_service.go` を新規作成
    - `AuthenticationService` struct（依存: `WebAuthnAdapter` / `ChallengeStore` /
      `PasskeyCredentialReader`（最小 IF: FindByCredentialID / UpdateSignCount）/
      `UserReader`（最小 IF: FindByID）/ `AuthCodeCreator`（**既存 `auth.AuthCodeCreator`
      をそのまま流用**、新規 IF 作成禁止）/ `now func() time.Time`）
    - `BeginAuthentication(ctx, codeChallenge)`:
      既存 `auth.ValidatePKCES256(codeChallenge, "S256")` を呼び validation →
      `WebAuthnAdapter.BeginLogin`（discoverable / allowCredentials 空 / Req 2.6 の
      存在有無非開示に整合）→ `ChallengeStore.Issue(kind=authentication, userID=nil,
      session_data には codeChallenge を含める)` → returns (challengeID, optionsJSON)
    - `FinishAuthentication(ctx, challengeID, requestBody)`:
      `ChallengeStore.Consume(challengeID, kind=authentication)` →
      `WebAuthnAdapter.FinishLogin(sessionData, requestBody, credentialLookup)`
      （credentialLookup 内で `FindByCredentialID` → `FindByID` で user 解決、
      未検出は library 側 error → `ErrAuthenticationFailed`。counter 後退も同 sentinel）→
      credential.sign_count / last_used_at を `UpdateSignCount` →
      `generateAuthCode`（既存 `auth.HashNativeSecret` を使うため小関数を `internal/passkey/` 内で
      再実装せず、`internal/auth/` の既存 `generateAuthCode` を exported にするか、passkey 側で
      同等コードを持つ。**選択: `internal/auth/native.go` の `generateAuthCode` を大文字化して
      exported にする**（他パッケージ利用のため / 既存挙動は不変）→
      `AuthCodeCreator.Create(&model.AuthCode{CodeHash: HashNativeSecret(plain), UserID: userID,
      PKCEChallenge: <session_data から復元した codeChallenge>, ExpiresAt: now+60s})` →
      returns 平文 auth_code
    - 拒否は全て `ErrAuthenticationFailed` に正規化（Req 2.5 / 2.6）。credential_id / user_id は
      hash 先頭 8 文字ログのみ（NFR 1.2）
  - `internal/passkey/authentication_service_test.go` を新規作成:
    成功（既存 auth_code が Create される + 平文が返る）、challenge 期限切れ → `ErrAuthenticationFailed`、
    credential 未検出 → `ErrAuthenticationFailed`、counter 後退 → `ErrAuthenticationFailed`、
    PKCE 検証失敗 → `ErrAuthenticationFailed`、AuthCode.PKCEChallenge が begin 時の値と一致
  - `internal/auth/native.go` の `generateAuthCode` を `GenerateAuthCode` に rename（既存 usage
    を修正、既存挙動は完全に不変 / NFR 2.1）
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, NFR 1.2, NFR 1.4, NFR 2.1, NFR 4.1_
  - _Boundary: AuthenticationService_
  - _Depends: 3, 4_

- [ ] 6. HTTP handlers + router 配線 + IP rate limit
  - `internal/handler/passkey_handler.go` を新規作成
    - `PasskeyHandler` struct（依存: `PasskeyRegistrationService` / `PasskeyAuthenticationService`
      の最小 IF）
    - 6 endpoint（RegistrationBegin / Finish / RegistrationAddBegin / Finish /
      AuthenticationBegin / Finish）を実装
    - リクエスト JSON は `dec.DisallowUnknownFields()`（既存 `NativeAuthHandler` 流儀）
    - エラー → APIError マッピング:
      `ErrInvalidUsername` → 400 `INVALID_USERNAME` /
      `ErrUsernameTaken` → 409 `USERNAME_TAKEN` /
      `ErrRegistrationFailed` → 400 `REGISTRATION_FAILED` /
      `ErrAuthenticationFailed` → 400 `AUTHENTICATION_FAILED` /
      その他 → 500 `INTERNAL_ERROR`
    - 追加登録 2 endpoint は `middleware.UserIDFromContext` で userID 取得（無ければ 401）
    - JSON 応答は既存 `middleware.WriteErrorResponse` / `WriteInternalServerError` を再利用
      （NFR 1.3）
  - `internal/model/errors.go` に APIError 生成関数を追加（`NewInvalidUsernameError` /
    `NewUsernameTakenError` / `NewRegistrationFailedError` / `NewAuthenticationFailedError`。
    メッセージは固定・入力反射なし）
  - `internal/handler/aasa_handler.go` を新規作成
    - `AASAHandler` struct（依存: `iOSAppID string`。生成時に immutable JSON byte を pre-marshal）
    - `Serve(w, r)`: Content-Type `application/json` + `Cache-Control: public, max-age=3600` +
      pre-marshal 済みバイト列書き出し
    - iOS App ID 空なら handler 生成しない（fail-closed）
  - `internal/handler/service_adapter.go` に `PasskeyServiceAdapter` を追加
    （domain 型 → handler DTO への必要最小の変換のみ。認可・ビジネスロジックは持たない）
  - `internal/handler/router.go` を拡張:
    - `RouterDeps` に `PasskeyHandler *PasskeyHandler` / `AASAHandler *AASAHandler` を追加
    - 認証不要グループ配下に AASA 1 route + Passkey 未認証 4 route を追加
      （Passkey は既存 `unauthIPMW` + `NewMaxBodyBytesMiddleware(DefaultMaxBodyBytes)` を通す。
      Req 6.1〜6.5 / AASA は `unauthIPMW` の外側 / Req 5.4）
    - 認証必須グループ配下に Passkey 追加登録 2 route を追加
    - いずれの `deps.*Handler` も nil の場合は登録しない（既存 fail-closed パターン踏襲 /
      NFR 2.2）
  - `internal/handler/passkey_handler_test.go` を新規作成:
    正常応答（begin: 200 + challenge_id + options / finish: 200 + auth_code or user_id）、
    エラーマッピング（invalid username / taken / registration failed / auth failed / unauthorized）、
    JSON body 上限超過（400 INVALID_REQUEST）、追加登録 endpoint への未認証呼び出し（401）
  - `internal/handler/aasa_handler_test.go` を新規作成:
    200 応答 / Content-Type / Cache-Control / JSON 構造（webcredentials.apps 配列に指定 App ID
    が含まれる）/ ユーザー情報が含まれない（Req 5.3）
  - `internal/handler/router_unauth_ratelimit_test.go` を拡張し passkey 未認証 4 endpoint も
    閾値超過で 429 + 既存応答形式を返すことを検証（Req 6.5）
  - `internal/handler/router_test.go` を拡張し fail-closed（handler nil）で passkey route が
    404 になること、AASA が独立に fail-closed になることを検証（NFR 2.2）
  - _Requirements: 1.1, 1.2, 1.4, 1.5, 1.7, 2.1, 2.2, 2.5, 2.6, 3.1, 3.2, 3.5, 3.6, 3.7, 5.1, 5.2, 5.3, 5.4, 6.1, 6.2, 6.3, 6.4, 6.5, NFR 1.3, NFR 2.1, NFR 2.2_
  - _Boundary: PasskeyHandler, AASAHandler, Router_
  - _Depends: 4, 5_

- [ ] 7. Config 拡張と app.go wiring（fail-closed 縮退の完成）
  - `internal/config/config.go` を拡張:
    - `Config` に `WebAuthnRPID string` / `WebAuthnRPDisplayName string` /
      `WebAuthnOrigins []string` / `WebAuthnIOSAppID string` /
      `PasskeyChallengeTTL time.Duration` を追加
    - `Load()` で以下を読み込み（既存 `NativeAuthJWTSecret` パターン踏襲）:
      `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_DISPLAY_NAME`（既定 `"Feedman"`）/ `WEBAUTHN_ORIGINS`
      （カンマ区切り、既存 `parseCommaSeparated` 再利用）/ `WEBAUTHN_IOS_APP_ID` /
      `PASSKEY_CHALLENGE_TTL_SECONDS`（既定 300 秒）
    - 全 env が任意（未設定でも起動継続）
  - `internal/config/config_test.go` を拡張し既定値・env 未設定・不正値フォールバックを検証
  - `internal/app/app.go` の `runServe` を拡張:
    - `WebAuthnRPID != ""` かつ `len(WebAuthnOrigins) > 0` のときのみ以下を組み立てる
      （どちらか欠けたら `slog.Warn` を出して skip / fail-closed）:
      - `passkeyCredentialRepo := repository.NewPostgresPasskeyCredentialRepo(db)`
      - `passkeyChallengeRepo := repository.NewPostgresPasskeyChallengeRepo(db)`
      - `webAuthnAdapter := passkey.NewGoWebAuthnAdapter(cfg.WebAuthnRPID,
        cfg.WebAuthnRPDisplayName, cfg.WebAuthnOrigins)`
      - `challengeStore := passkey.NewChallengeStore(passkeyChallengeRepo,
        cfg.PasskeyChallengeTTL)`
      - `registrationSvc := passkey.NewRegistrationService(webAuthnAdapter, challengeStore,
        userRepo, passkeyCredentialRepo)`
      - `authenticationSvc := passkey.NewAuthenticationService(webAuthnAdapter, challengeStore,
        passkeyCredentialRepo, userRepo, authCodeRepo)` — **authCodeRepo は既存 native auth の
        wiring と共用**
      - `passkeyHandler := handler.NewPasskeyHandler(registrationSvc, authenticationSvc)`
      - `deps.PasskeyHandler = passkeyHandler` に代入
    - `WebAuthnIOSAppID != ""` のときのみ `aasaHandler := handler.NewAASAHandler(cfg.WebAuthnIOSAppID)` →
      `deps.AASAHandler = aasaHandler` に代入
    - 未設定時は `slog.Warn("passkey is disabled (WEBAUTHN_RP_ID or WEBAUTHN_ORIGINS not set)")`
      / `slog.Warn("AASA is disabled (WEBAUTHN_IOS_APP_ID not set)")` を 1 回記録
  - `internal/app/app_test.go` の起動テストを拡張し、env 未設定時に既存挙動と等価に起動すること
    を検証（NFR 2.2）
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 8.1, 8.2, 8.3, 8.4, 8.5, NFR 2.1, NFR 2.2_
  - _Boundary: Config, AppWiring_
  - _Depends: 6_

- [ ] 8. 退会 tx cleanup 統合と既存契約 regression 検証
  - `internal/user/service.go` を拡張:
    - `TxPasskeyCredentialDeleter` interface を追加（`DeleteByUserIDTx(ctx, tx, userID) error`）
    - `Service.txPasskeyCredentialDeleter TxPasskeyCredentialDeleter` フィールドを追加
    - `NewServiceWithTx` シグネチャに `passkeyCredentialDeleter TxPasskeyCredentialDeleter` を
      末尾追加（既存 6 引数の後）
    - `withdrawTx` の `sessions` 削除の直後・`auth_codes` 削除の直前に
      passkey credential 削除 1 段を挿入（Req 7.1〜7.5 / 削除順序: item_states → subscriptions
      → sessions → **passkey_credentials** → auth_codes → refresh_token_families → user）
    - `txPasskeyCredentialDeleter == nil` の場合はスキップ（既存 nil ガード方針に整合 / NFR 2.2:
      env 未設定環境で `NewServiceWithTx` の呼び出し側から nil が渡っても既存挙動と等価）
    - 失敗時は `return fmt.Errorf("passkey credential の削除に失敗しました: %w", err)`（既存
      パターン踏襲、削除失敗 → tx 全体 rollback）
  - `internal/user/service_test.go` を拡張し以下を検証:
    passkey deleter が nil でも既存挙動と等価に完了する、passkey deleter が呼ばれる、
    失敗時に tx 全体が rollback される（他削除も反映されない）、順序が要件通り
  - `internal/app/withdraw_wiring.go` を拡張:
    - `txPasskeyCredentialDeleterAdapter` struct + `DeleteByUserIDTx` を追加
    - `newTxUserService` の引数末尾に `passkeyCredentialRepo *repository.PostgresPasskeyCredentialRepo`
      を追加し `&txPasskeyCredentialDeleterAdapter{repo: passkeyCredentialRepo}` を注入
    - compile-time check に `_ user.TxPasskeyCredentialDeleter = (*txPasskeyCredentialDeleterAdapter)(nil)`
      を追加
  - `internal/app/withdraw_wiring_test.go` を拡張し 8 段の削除順序・全成功でコミット・
    途中失敗で rollback を検証
  - `internal/app/app.go` の `runServe` で `newTxUserService(..., authCodeRepo, refreshTokenRepo,
    passkeyCredentialRepo)` を呼ぶよう修正（`passkeyCredentialRepo` はパスキー env 未設定でも
    常に作成しておき、passkey handler が nil でも退会 cleanup は動く / Req 7.2 と NFR 2.2 の
    両立）
  - `internal/repository/postgres_withdraw_integration_db_test.go` を拡張:
    退会後に `passkey_credentials` が 0 件になることを DB 直接 SELECT で検証（Req 7.1, 7.4 /
    NFR 4.2）、退会後に同一 username_normalized で新規登録が可能（Req 7.5、user 削除で
    UNIQUE 制約解放）を DB 直接 INSERT + `FindByNormalizedUsername` で検証
  - 既存契約 regression 検証:
    `docs/specs/172-native-auth-contract-tests/contract-notes.md` の対照表が本 spec 導入後も
    無変更のまま整合することを目視確認（`TestContract_*` / `TestE2E_NativeAuthFullFlow_DBBacked`
    が failing しないこと。本 task で `go test ./...` を実行し全 green を担保）
  - Passkey フロー全体の E2E 契約テストを 1 件追加（`internal/handler/passkey_e2e_db_test.go`）:
    passkey 登録 → 認証 begin/finish → 既存 `POST /api/auth/token` で auth_code 交換 → Bearer
    access token で保護 API 到達までの通し（synthetic authenticator 使用）
  - _Requirements: 2.4, 7.1, 7.2, 7.3, 7.4, 7.5, 8.1, 8.2, 8.3, 8.4, 8.5, NFR 2.1, NFR 2.3, NFR 4.1, NFR 4.2_
  - _Boundary: UserService, WithdrawWiring, WithdrawIntegration, ContractRegression_
  - _Depends: 7_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が独立に再実行する verify コマンドを以下に
構造化ブロックで宣言する。Go プロジェクトの build / test / vet / format の 4 段を含める。

<!-- stage-a-verify -->
```sh
gofmt -l internal/ | (! grep .) && go vet ./... && go build ./... && go test ./...
```
