# Implementation Plan

本タスクリストは **PR #229 の `needs-iteration` 1 回**で反映する差分作業を、独立コミット
可能な粒度で列挙する。**#223 spec（`docs/specs/223-feat-web-web/requirements.md` /
`design.md` / `tasks.md`）および #216 spec の物理ファイルを書き換えるタスクは含まない**
（NFR 1.1 / 1.3）。反映対象は `internal/**` / `web/src/**` の製品コードとテスト、運用 config
（`.env.sample` / `docker-compose.yml`）、および #223 spec の補助（`impl-notes.md` /
`context-map.md`）に限定する。

各タスクは実装 + 対応テストを同一 commit で完結させる（per-task ループ運用の入力契約 /
`_Requirements_partial:_` は本ドラフトで宣言なし）。

- [ ] 1. サーバ: `RegistrationService` に `registrationSession` 封筒を導入し、`FinishRegistrationNew` が auth_code を additive に返す構成に変更する（Delta 1 サーバ側）
  - `internal/passkey/registration_service.go` に `registrationSession` 構造体
    （`{WebAuthnSession json.RawMessage, CodeChallenge string}`）を追加する。既存
    `internal/passkey/authentication_service.go` L57-66 の `authnSession` と同型で作る
  - `BeginRegistrationNew`: 既存の PKCE 早期検証を維持したまま、検証済み `codeChallenge` を
    `registrationSession` に格納し `json.Marshal` で bytes 化した後、既存
    `challenges.Issue(sessionData=envelopeBytes, ...)` に渡す
  - `FinishRegistrationNew` の **シグネチャを変更**:
    `(ctx, challengeID, requestBody) (string, error)` →
    `(ctx, challengeID, requestBody) (userID string, authCodePlain string, err error)`
  - `FinishRegistrationNew` 内部:
    - `challenges.Consume` の戻り値 `SessionData` を `registrationSession` として
      `json.Unmarshal` し、`envelope.WebAuthnSession` を既存 `adapter.FinishRegistration` に
      渡す（既存 attestation 検証ロジックは変更なし）
    - envelope 破損 / `envelope.CodeChallenge == ""` は `ErrRegistrationFailed` に uniform 化
      （既存 authentication_service.go L230-241 と同方針）
    - 既存の user 作成 / credential 保存フローを維持
    - 末尾に auth_code 発行を追加: `plain, _ := auth.GenerateAuthCode()` →
      `codeHash := auth.HashNativeSecret(plain)` →
      `AuthCode{ID: uuid.New().String(), CodeHash: codeHash, UserID: newUser.ID,
      PKCEChallenge: envelope.CodeChallenge, ExpiresAt: s.now().Add(auth.NativeAuthCodeTTL)}` →
      `s.authCodes.Create(ctx, authCode)`（既存 `authentication_service.go` L320-334 と同経路）
    - `slog.Info` に `user_id` と `auth_code_hash[:8]` のみを載せ、平文 auth_code / code_verifier /
      code_challenge をログ・エラーメッセージに一切出さない（NFR 2.1 / 既存
      authentication_service.go L338-341 と同方針）
  - `RegistrationService` 構造体に `authCodes auth.AuthCodeCreator` フィールドを追加、
    `NewRegistrationService` のシグネチャに `authCodes auth.AuthCodeCreator` を追加
    （既存 `AuthenticationService` と同じ具体依存）
  - `internal/passkey/registration_service_test.go` を変更:
    - 既存テストを新シグネチャに追従（すべての `FinishRegistrationNew` 呼び出し箇所で
      3 戻り値受け取りに変更）
    - 新規テスト（正常系）: begin で渡した `codeChallenge` が finish の envelope 復元後に
      `authCodes.Create` に渡された `AuthCode.PKCEChallenge` と一致すること（stub で AuthCode を
      キャプチャして verify）
    - 新規テスト（正常系）: `authCodePlain` 戻り値が非空・base64url 形式（`auth.GenerateAuthCode`
      の返り値形式に整合）
    - 新規テスト（異常系）: `challenges.Consume` が返す SessionData を破損させて
      `json.Unmarshal` を失敗させる → `ErrRegistrationFailed`
    - 新規テスト（異常系）: envelope の `CodeChallenge` を空文字にする → `ErrRegistrationFailed`
    - 新規テスト（異常系）: `authCodes.Create` stub が infra error を返す → wrap されて伝播
  - `internal/app/app.go` を変更: `passkey.NewRegistrationService(...)` の呼び出しに
    `authCodes`（既存 `repository.NewPostgresAuthCodeRepo(...)` の返り値、既に
    `AuthenticationService` に注入されている同じインスタンス）を追加する
  - _Requirements: 1.1, 1.2, 1.3, 3.5, NFR 2.1, NFR 3.2_
  - _Boundary: passkey/RegistrationService, app.go wiring_

- [ ] 2. サーバ: `PasskeyHandler.RegistrationFinish` レスポンスに auth_code を additive 追加し、router.go で capability endpoint 登録条件を両 handler 非 nil に強化する（Delta 1 handler + Delta 3 router）
  - `internal/handler/passkey_handler.go` を変更:
    - `RegistrationFinish` ハンドラのレスポンス型を additive に拡張:
      `{user_id string, auth_code string}`（現行は `{user_id}` のみ）。**`user_id` フィールドは
      名称・位置ともに維持し、`auth_code` を末尾に追加**（iOS #216 の JSON decoder が未知
      フィールドを無視する慣行に依拠 / NFR 3.2）
    - `RegistrationService.FinishRegistrationNew` の新シグネチャ（3 戻り値）に追従し、
      2 番目返り値 `authCodePlain` をレスポンス body の `AuthCode` フィールドに設定
    - 既存の error mapping（`ErrRegistrationFailed` → 400 REGISTRATION_FAILED / infra → 500）は
      維持
  - `internal/handler/router.go` を変更:
    - Web 用 `GET /api/passkey/capability` の登録条件を
      `if deps.PasskeyHandler != nil && deps.NativeAuthHandler != nil` に強化する
      （現行は `PasskeyHandler != nil` のみ想定）
    - iOS 用 `/api/passkey/registration/*` および `/api/passkey/authentication/*` の登録条件は
      **`PasskeyHandler != nil` のみを維持し変更しない**（#216 契約破壊禁止 / NFR 3.2）
    - `/api/auth/session`（NativeAuthHandler.Session）の登録条件は既存 `NativeAuthHandler !=
      nil` を維持
  - `internal/handler/passkey_handler_test.go` を変更:
    - `TestPasskeyHandler_RegistrationFinish` の期待レスポンス body に `auth_code` を追加、
      `user_id` フィールドが引き続き存在することを assert（iOS ignore-unknown 互換の回帰）
    - auth_code フィールドが非空 base64url 形式（`^[A-Za-z0-9_-]+$`）であること
    - サーバ側の `RegistrationService` は本テストでは stub 化し、`authCodePlain` を
      特定値で返して handler がそのまま body に含めることを検証
  - `internal/handler/router_test.go` を変更:
    - Delta 3 §fail-closed 表の 4 環境行それぞれで route 登録有無を検証する table-driven テストを追加:
      1. `PasskeyHandler != nil && NativeAuthHandler != nil` → session 204 / capability 200 /
         iOS registration/*+authentication/* 200
      2. `PasskeyHandler != nil && NativeAuthHandler == nil` → session 404 / **capability 404
         （強化条件の回帰）** / iOS registration/*+authentication/* 200 **（iOS 継続稼働の回帰）**
      3. `PasskeyHandler == nil && NativeAuthHandler != nil` → session 204 / capability 404 /
         iOS registration/*+authentication/* 404
      4. 両 nil → 全 404
  - _Requirements: 1.1, 1.3, 3.1, 3.2, 3.3, 3.4, 3.5, NFR 3.2_
  - _Boundary: handler/PasskeyHandler, handler/router_
  - _Depends: 1_

- [ ] 3. Web: `use-passkey-registration.ts` から二度目 WebAuthn ceremony を除去し、finish 応答の auth_code を直接 session 交換に渡す構成に変更する。`registration_uncertain` kind を追加する（Delta 1 Web + Delta 5 Hook）
  - `web/src/types/passkey.ts` の `RegistrationFinishResponse` 型に `auth_code: string`
    フィールドを additive に追加（`user_id` は既存維持）
  - `web/src/hooks/use-passkey-registration.ts` の `mutationFn` chain を書き換え:
    - 現行 6 段（begin → create → finish → authentication/begin → get → authentication/finish
      → session）から **4 段（begin → create → finish → session）** に短縮
    - `POST /api/passkey/registration/finish` の応答 `RegistrationFinishResponse` から
      `auth_code` を取り出し、直後の `POST /api/auth/session` request body の `auth_code` に
      直接渡す（`code_verifier` は closure 変数で保持済み）
    - `authentication/begin` / `navigator.credentials.get` / `authentication/finish` の
      3 呼び出しを **完全に削除**（二度目 ceremony の除去 / Req 1.1）
  - `PasskeyRegistrationErrorKind` 型に `"registration_uncertain"` を追加
  - error 分類ロジックを step index ベースで整理し、finish 段階の失敗を以下 3 分岐に細分化
    （Delta 5 §Hook 側の判定ロジックに準拠）:
    - `ApiError` かつ `status >= 400 && status < 500` かつ `body.code === "REGISTRATION_FAILED"`
      → `server_rejected`
    - `ApiError` かつ `status >= 500` → **`registration_uncertain`**（サーバが commit したか
      不確定）
    - `TypeError`（fetch reject）→ **`registration_uncertain`**（送出済み・応答なし）
    - `DOMException` かつ `name === "AbortError"` → **`registration_uncertain`**
    - その他 → `server_error`
  - begin 段階 / create 段階 / session 段階の error 分類は既存挙動を維持（`registration_uncertain`
    にしない / Delta 5 §安全側 fail 原則）
  - `code_verifier` / `auth_code` / attestation 生値を `console.*` / storage / URL に書かない
    （NFR 2.1）
  - `web/src/hooks/use-passkey-registration.test.ts` を変更:
    - 正常系: `authentication/begin` / `authentication/finish` が呼ばれないことを assert
      （二度目 ceremony 除去の回帰）
    - 正常系: finish 応答 body の `auth_code` が session request body の `auth_code` として
      渡ることを assert
    - 新規: `registration_uncertain` × fetch reject（finish が `TypeError` を throw）
    - 新規: `registration_uncertain` × 500（finish が 500 応答）
    - 新規: `registration_uncertain` × AbortError（finish が AbortError を throw）
    - 回帰: `server_rejected`（finish が 400 REGISTRATION_FAILED）が `registration_uncertain`
      に誤分類されない
    - 既存 kind の分岐テスト（`invalid_username` / `username_taken` / `cancelled` /
      `session_exchange_failed`）は新 chain に合わせて updates
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 5.1, 5.5, NFR 2.1, NFR 3.1_
  - _Boundary: hooks/use-passkey-registration, types/passkey_
  - _Depends: 2_

- [ ] 4. Web: `passkey-signup-dialog.tsx` に `registration_uncertain` の UI 分岐と復旧導線（「ログインで確認する」「再度作成する」の 2 ボタン）を追加する（Delta 5 UI）
  - `web/src/components/passkey-signup-dialog.tsx` を変更:
    - `error.kind === "registration_uncertain"` 分岐を追加（既存の `invalid_username` /
      `username_taken` / `cancelled` / `server_rejected` / `session_exchange_failed` /
      `server_error` / `network_error` 分岐と並列に追加）
    - 表示メッセージは Delta 5 §UI 文言と復旧導線 の canonical 文言に準拠。特に「登録が完了
      したかどうかを確認できませんでした」の見出しで、他 kind と識別可能にする（Req 5.6）
    - 2 ボタンの実装:
      - 「ログインで確認する」: `onOpenChange(false)` を呼び Dialog を閉じる（LoginPage 側の
        `PasskeyButtons` からユーザーが再度パスキーログインを試みる導線に戻す / Req 5.2 / 5.3）
      - 「再度作成する」: `mutation.reset()` を呼び Dialog を Idle 状態に戻す（同じ username で
        再試行すると `username_taken` になることを Delta 5 §UI 文言 で案内済 / Req 5.4）
    - メッセージ本文に `error.body` の内容・スタックトレース・SQL / DB 名などのサーバ内部詳細を
      **含めない**（Req 5.5 / NFR 2.1）
    - `console.*` の呼び出しは追加しない（NFR 2.1）
  - `web/src/components/passkey-signup-dialog.test.tsx` を変更:
    - 新規: `error.kind === "registration_uncertain"` で見出し「登録が完了したかどうかを
      確認できませんでした」が表示される
    - 新規: 「ログインで確認する」ボタン押下で `onOpenChange(false)` が呼ばれる
    - 新規: 「再度作成する」ボタン押下で `mutation.reset()` が呼ばれる
    - 新規: 文言中に既存 error kind の代表文言（「ユーザー名の形式が不正です」「このユーザー名は
      既に使用されています」「認証に失敗しました」等）が含まれない（Req 5.6 の弁別性回帰）
    - 新規: 文言中にサーバ内部詳細を想起させる文字列（`Error:`, `Stack:`, `SQL`, スタック
      トレース由来のプレフィックス）が含まれない（Req 5.5 の回帰）
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, NFR 2.1_
  - _Boundary: components/passkey-signup-dialog_
  - _Depends: 3_

- [ ] 5. Config: `.env.sample` と `docker-compose.yml` に既存 WebAuthn env 群（新規 env なし）の documentation と passthrough を追加する（Delta 6 / Requirement 6）
  - `.env.sample` を変更: Native Auth 節（現行 L119-130 付近）の後に、Delta 6 §反映内容 §.env.sample の
    canonical コメントブロックを追加する。5 変数を **すべてコメントアウト行**として documentation
    のみ記載し、デフォルト値の代入は行わない:
    - `# WEBAUTHN_RP_ID=`
    - `# WEBAUTHN_RP_DISPLAY_NAME=Feedman`
    - `# WEBAUTHN_ORIGINS=`
    - `# WEBAUTHN_IOS_APP_ID=`
    - `# PASSKEY_CHALLENGE_TTL_SECONDS=300`
  - 各コメントに fail-closed 挙動と例値・意味を明記（Delta 6 §反映内容 §.env.sample のサンプルを
    参照）。RP_ID / ORIGINS / IOS_APP_ID の例値は運用者が誤ってコピペしないよう「例:」プレフィクスを
    必ず付ける
  - `docker-compose.yml` の `api` サービス `environment` セクションに以下 5 行を追加する
    （既存 `NATIVE_AUTH_JWT_SECRET=${NATIVE_AUTH_JWT_SECRET:-}` の直後を推奨位置とする）:
    - `- WEBAUTHN_RP_ID=${WEBAUTHN_RP_ID:-}`
    - `- WEBAUTHN_RP_DISPLAY_NAME=${WEBAUTHN_RP_DISPLAY_NAME:-Feedman}`
    - `- WEBAUTHN_ORIGINS=${WEBAUTHN_ORIGINS:-}`
    - `- WEBAUTHN_IOS_APP_ID=${WEBAUTHN_IOS_APP_ID:-}`
    - `- PASSKEY_CHALLENGE_TTL_SECONDS=${PASSKEY_CHALLENGE_TTL_SECONDS:-300}`
  - `worker` サービスの `environment` には **追加しない**（worker は passkey に依存しない）
  - **新規 env は追加しない**。追加は既存 env（config.go / #216 で既に読み込み済み）の
    documentation と container passthrough に限定する（Req 6.5）
  - config 変更に対する動作回帰は既存の `internal/config/config_test.go` の
    `TestLoad_Passkey`（L411 以降）で保証されており、本タスクでは `internal/config/*` を
    追加変更しない（既存テストが env 読込を検証済み）
  - 手動確認手順（tasks 完了時に PR 本文の「確認事項」で言及するのみ、自動テスト化は不要）:
    - `.env.sample` をコピーして `WEBAUTHN_RP_ID` などを実値に設定し `docker compose up` した
      とき、api コンテナ内で当該 env が期待どおり参照できることを README 記載の起動手順で確認
  - _Requirements: 2.2, 6.1, 6.2, 6.3, 6.4, 6.5, NFR 3.1_
  - _Boundary: .env.sample, docker-compose.yml_

- [ ] 6. Docs: `docs/specs/223-feat-web-web/impl-notes.md` と `context-map.md` に #231 normative delta の適用結果を追記する（#223 spec の requirements.md / design.md / tasks.md は書き換えない）
  - `docs/specs/223-feat-web-web/impl-notes.md` を変更:
    - 冒頭または「確認事項」節に「#231 normative delta 適用後の実装状態」旨のセクションを追加
    - Delta 1〜5 それぞれについて 1〜3 行で「本 PR で反映した内容」と「#231 design.md の
      該当 Delta セクションへの参照」を記載
    - `#223 design.md` §Flows「新規作成フロー（Sequence / 抜粋）」の該当箇所は **#231 design.md
      Delta 1 で supersede** されている旨を明示する
  - `docs/specs/223-feat-web-web/context-map.md` を変更:
    - 新規作成フローの経路図から「二度目 WebAuthn ceremony」を削除し、finish → session の
      直接遷移に更新
    - fail-closed 表を #231 design.md Delta 3 §補正後の fail-closed 表 のリンクに置換または
      supersede 注釈を追加
    - 完了不明状態（`registration_uncertain`）の遷移を hook / component / UI 文言の各層に
      追加
  - **`docs/specs/223-feat-web-web/requirements.md` / `design.md` / `tasks.md` は書き換えない**
    （NFR 1.1）
  - **`docs/specs/216--app-store-4-8/` 配下のいかなるファイルも書き換えない**（NFR 1.3）
  - _Requirements: 2.3, 2.4, 2.5, NFR 1.1, NFR 1.2, NFR 1.3_
  - _Boundary: docs/specs/223-feat-web-web/impl-notes, docs/specs/223-feat-web-web/context-map_

## Verify

本 spec の実装反映後、watcher（stage-a-verify gate）が独立に再実行して build / test / lint を
検証する。サーバ側は `go test ./...` + `go vet ./...`、Web 側は `npm test` + `npm run lint` +
`npm run build`。config 変更（`.env.sample` / `docker-compose.yml`）の妥当性は既存
`internal/config/config_test.go` の `TestLoad_Passkey` および docker-compose YAML の schema で
担保される。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./... && cd web && npm test && npm run lint && npm run build
```
