# Implementation Plan

本タスクリストは **PR #229 の `needs-iteration` 1 回**で反映する差分作業を、独立コミット
可能な粒度で列挙する。中核決定 1.A（`registration/finish` の同一 DB トランザクションで
user・credential・session の 3 行を作成し、commit 後に `Set-Cookie` する **直接 Cookie
session**）に基づく。**登録用 auth_code / 二度目 ceremony / 登録専用 session 交換は導入しない**。

**#223 spec（`docs/specs/223-feat-web-web/requirements.md` / `design.md` / `tasks.md`）および
#216 spec の物理ファイルを書き換えるタスクは含まない**（NFR 1.1 / 1.3）。反映対象は
`internal/**` / `web/src/**` の製品コードとテスト、運用 config（`.env.sample` /
`docker-compose.yml`）、#223 spec の補助（`impl-notes.md` / `context-map.md`）に限定する。

各タスクは実装 + 対応テストを同一 commit で完結させる（`_Requirements_partial:_` は本ドラフトで
宣言なし = 全 AC が同一タスク内テスト必須）。

- [ ] 1. サーバ基盤: トランザクション実行ヘルパー・各 repo の DBTX 変種 Create・共有 Cookie builder・session ID 生成器の共有化を追加する
  - `internal/repository/tx.go` に closure 実行ヘルパー
    `func (b *SQLTxBeginner) WithinTx(ctx context.Context, fn func(q DBTX) error) error` を追加する
    （`BeginTx` → `fn(tx.Querier())` → 正常時 `Commit` / エラー・panic 時 `Rollback`）。既存
    `SQLTx` / `BeginTx` / `Querier` / `Commit` / `Rollback` は変更しない
  - `internal/repository/postgres_session_repo.go` に
    `CreateExec(ctx, q DBTX, s *model.Session) error` を追加し、既存 `Create` を
    `CreateExec(ctx, r.db, s)` への委譲に変更する（差分等価 / 既存 `DeleteByUserIDExec` の DBTX 変種
    パターンに準拠）
  - `internal/repository/postgres_user_repo.go` に
    `CreateUserOnlyExec(ctx, q DBTX, u *model.User) error` を追加し、既存 `CreateUserOnly` を委譲に
    変更する（`ErrUsernameTaken` 正規化を変えない）
  - `internal/repository/postgres_passkey_credential_repo.go` に
    `CreateExec(ctx, q DBTX, c *model.PasskeyCredential) error` を追加し、既存 `Create` を委譲に
    変更する（`ErrCredentialAlreadyRegistered` 正規化を変えない）
  - `internal/handler/session_cookie.go` を新規追加し、canonical builder
    `buildSessionCookie(name, value, domain string, secure bool, maxAge int) *http.Cookie` を定義する。
    既存 `auth_handler.go::Callback` / `native_auth_handler.go::Session` のインライン Cookie 生成を
    本 builder 呼び出しに差し替える（属性は現行と完全一致・差分等価）
  - `internal/auth/session_exchange.go` の session ID 生成器を `auth.GenerateSessionID` として export
    （または既存 export を確認）し、`SessionExchangeService` と後続の RegistrationService が同一生成器を
    共有できるようにする（`SessionExchangeService` の外部契約は不変）
  - テスト: `internal/handler/session_cookie_test.go` を新規追加し、`buildSessionCookie` の属性
    （Name / HttpOnly / SameSite=Lax / Secure / Path / Domain / Max-Age）が Google OAuth Callback と
    一致することを検証する。各 `*_Exec` 変種の委譲は既存 repo テストの差分等価（既存 `Create` /
    `CreateUserOnly` テストが引き続き pass）で担保する
  - _Requirements: 1.3, 4.5, NFR 2.1, NFR 3.1_
  - _Boundary: repository/tx, repository/postgres_session_repo, repository/postgres_user_repo, repository/postgres_passkey_credential_repo, handler/session_cookie, auth/session_exchange_

- [ ] 2. サーバ: `RegistrationService.FinishRegistrationNew` を単一 tx で user → credential →（Web mode のみ）session の 3 行作成に拡張し、wiring を更新する
  - `internal/passkey/registration_service.go`:
    - 最小 IF を追加/拡張: `SessionWriter{CreateExec}`、`UserWriter` に `CreateUserOnlyExec`、
      `PasskeyCredentialWriter` に `CreateExec`、tx 実行 IF `txRunner{WithinTx}`
    - 構造体に `sessions SessionWriter` / `tx txRunner` / `newSessionID func() (string, error)` /
      `sessionTTL time.Duration` を追加する（**auth_code 関連依存は追加しない**）。
      `NewRegistrationService` のシグネチャに追加し、`WebSessionReady() bool` を追加する
    - `FinishRegistrationNew` のシグネチャを
      `(ctx, challengeID, requestBody) (string, error)` →
      `(ctx, challengeID, requestBody, issueWebSession bool) (userID string, webSession *model.Session, err error)`
      に変更する。既存の challenge consume / `adapter.FinishRegistration`（attestation 検証）は変更しない
    - user 作成 / credential 保存 / （`issueWebSession` 時の）session 生成・保存を **`tx.WithinTx` の
      単一クロージャ内**で実行する。user UNIQUE 衝突 → `ErrRegistrationFailed`、credential 重複 →
      `ErrRegistrationFailed`（既存正規化を維持）、session ID 生成失敗 / session INSERT 失敗 / commit 失敗は
      全 ROLLBACK し `webSession=nil` で伝播する（3 行 atomic / design §Delta 1）
    - session ID 生値・auth_code 平文をログ・エラー・レスポンスに残さない（NFR 2.1）
  - `internal/app/app.go`: `passkey.NewRegistrationService(...)` に `sessionRepo`（SessionWriter）/
    tx runner（`db` 由来の `SQLTxBeginner`）/ `auth.GenerateSessionID` / `sessionTTL`
    （`time.Duration(cfg.SessionMaxAge) * time.Second`）を注入する。これらは
    `NATIVE_AUTH_JWT_SECRET` に依存せず `WEBAUTHN_*` 設定時（passkey handler 生成時）に常時配線する
  - `internal/passkey/registration_service_test.go`:
    - 既存テストを新シグネチャ（4 引数 / 3 戻り値）に追従させる
    - 正常系（Web mode）: `issueWebSession=true` で `WithinTx` 内の user → credential → session の呼び出し順を
      fake で検証し、`webSession` に生成 ID と `now+sessionTTL` の期限が入る
    - 正常系（iOS mode）: `issueWebSession=false` で session を作らず `webSession=nil`
    - 異常系: user UNIQUE 衝突 → `ErrRegistrationFailed`（session 未作成）
    - 異常系: credential 重複 → `ErrRegistrationFailed`（session 未作成）
    - 異常系: session ID 生成失敗 / session INSERT 失敗 → 全 ROLLBACK・`webSession=nil`・エラー伝播
  - _Requirements: 1.1, 1.2, 1.3, 3.5, NFR 2.1, NFR 3.2_
  - _Boundary: passkey/RegistrationService, app.go wiring_
  - _Depends: 1_

- [ ] 3. サーバ: `PasskeyHandler.RegistrationFinish` に Origin ベース Web/iOS mode 判定・Web mode の Set-Cookie を追加し、実 PostgreSQL で 3 行原子性を検証する
  - `internal/handler/passkey_handler.go`:
    - `PasskeyHandler` に `allowedOrigin string` と Cookie 設定（domain / secure / maxAge）を Option で
      注入する（既存 `NativeAuthHandler` の `WithSession*` Option idiom を踏襲）。`webRegistrationReady()`
      （`allowedOrigin != "" && Cookie 設定済み && regService.WebSessionReady()`）を追加する
    - `RegistrationFinish` に mode 判定を追加する（challenge consume / mutation より前）:
      - `Origin` 不在 → native/iOS mode（`issueWebSession=false`、既存挙動 / JSON Content-Type 追加検証なし）
      - `Origin == allowedOrigin`（allowedOrigin 設定済）→ Web mode。JSON Content-Type 非一致は 415、
        `!webRegistrationReady()` は fail-closed（500 相当）、いずれも mutation 前に返す
      - `Origin` 非空不一致 or allowedOrigin 未設定 → 403 FORBIDDEN_ORIGIN（mutation なし）
    - `FinishRegistrationNew(ctx, challengeID, credential, webMode)` を呼び、`webSession != nil` のとき
      `buildSessionCookie(...)`（Task 1）で `Set-Cookie` する。レスポンスは Web / iOS とも 200 `{user_id}`（不変）
  - `internal/handler/passkey_handler_test.go`:
    - Origin 一致 → 200 `{user_id}` + `Set-Cookie session_id`（属性一致）
    - Origin 不在（iOS）→ 200 `{user_id}`・`Set-Cookie` **なし**（#216 回帰）
    - Origin 非空不一致 / allowedOrigin 未設定 → 403、regService stub が呼ばれない（mutation なし）
    - Web mode で Content-Type 非 JSON → 415、mutation 呼ばれない
  - `internal/handler/passkey_e2e_db_test.go`（実 PostgreSQL）:
    - Web mode finish 成功で users / passkey_credentials / sessions に **3 行がすべて存在**
    - session INSERT を失敗させた場合（重複 session ID 注入等）に users / passkey_credentials にも
      **行が残らない（3 行 atomic rollback / orphan なし）**
    - iOS mode finish 成功で users / passkey_credentials の 2 行のみ存在（sessions なし）
  - _Requirements: 1.1, 1.3, 1.4, 3.5, 4.5, NFR 2.1, NFR 3.2_
  - _Boundary: handler/PasskeyHandler, handler/passkey_e2e_db_test_
  - _Depends: 2_

- [ ] 4. サーバ: `NativeAuthHandler.Session` の Origin 検証を fail-closed へ補正し、router の fail-closed 表 5 列を実装・検証する
  - `internal/handler/native_auth_handler.go`: `Session` の Origin 検証を
    「`allowedOrigin == ""` → 403 / `Origin` 不在 → 403 / `Origin != allowedOrigin` → 403 /
    `Origin == allowedOrigin` のみ通過」に補正する（design §Delta 4）。Content-Type application/json 必須は
    既存維持
  - `internal/handler/router.go`: capability route 条件に `deps.CORSAllowedOrigin != ""` を **追加** する
    （`NativeAuthHandler.SessionReady()` は **維持し弱めない**）。`/api/auth/session` 条件は
    `NativeAuthHandler != nil && SessionReady()` を維持。iOS 用 `/api/passkey/registration/*` /
    `/api/passkey/authentication/*`（および Web と共有する `registration/finish`）は `PasskeyHandler != nil` を
    維持（#216 契約 / NFR 3.2）
  - `internal/handler/native_auth_handler_test.go`: Origin 不在・不一致・allowedOrigin 未設定で 403 の異常系を追加
  - `internal/handler/router_test.go`: fail-closed 表 行 1〜5 の各 endpoint（列 A capability / 列 B session /
    列 C registration route / 列 D iOS registration/* / 列 E iOS authentication/*）の登録有無を table-driven で
    検証する。特に行 2（W✓ N✗ C✓）で iOS registration/*・authentication/* が 200、capability が 404、
    行 3（W✓ N✓ C✗）で capability 404 を assert する
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 4.1, NFR 3.2_
  - _Boundary: handler/native_auth_handler, handler/router_
  - _Depends: 1_

- [ ] 5. Web: `use-passkey-registration.ts` から二度目 WebAuthn ceremony と登録用 session 交換を除去し、`registration_uncertain` を追加する
  - `web/src/types/passkey.ts`: `PasskeyRegistrationErrorKind` に `"registration_uncertain"` を追加する。
    `RegistrationFinishResponse` は `{user_id}` のまま **auth_code を追加しない**
  - `web/src/hooks/use-passkey-registration.ts`:
    - `mutationFn` chain を「begin → create → finish（2xx + Set-Cookie）→ invalidateQueries」に短縮し、
      `authentication/begin` / `navigator.credentials.get` / `authentication/finish` / 登録用
      `POST /api/auth/session` を **完全に削除**する（Req 1.1 / 1.2）。`code_challenge` は #216 契約維持のため
      送るが `code_verifier` は使用しない
    - finish 段の error を分類する: `TypeError`（fetch reject）/ `AbortError` / 5xx / 2xx で body parse 不能 →
      `registration_uncertain`。400 REGISTRATION_FAILED → `server_rejected`。送出前ローカル失敗 → `server_error`
      （design §Delta 5 §安全側 fail 原則）。registration 経路は `session_exchange_failed` を発生させない
    - `code_verifier` / attestation 生値を `console.*` / storage / URL に書かない（NFR 2.1）
  - `web/src/hooks/use-passkey-registration.test.ts`:
    - 正常系: begin → create → finish の 3 呼び出しのみで `authentication/*` / 登録用 `/api/auth/session` が
      呼ばれない（二度目 ceremony 除去の回帰）
    - `registration_uncertain` × fetch reject / 5xx / AbortError / 2xx body parse 不能 の 4 サブケース
    - 回帰: `server_rejected`（400 REGISTRATION_FAILED）が uncertain に誤分類されない
    - begin 段の `invalid_username` / `username_taken` / `cancelled` は既存挙動維持
  - _Requirements: 1.1, 1.2, 1.4, 1.5, 5.1, 5.5, NFR 2.1, NFR 3.1_
  - _Boundary: hooks/use-passkey-registration, types/passkey_
  - _Depends: 3_

- [ ] 6. Web: `passkey-signup-dialog.tsx` に完了不明状態 UI と discoverable ログイン復旧導線（段階提示）を追加する
  - `web/src/components/passkey-signup-dialog.tsx`:
    - `error.kind === "registration_uncertain"` 分岐を追加する。初期表示は見出し「登録が完了したかどうかを
      確認できませんでした」+ 「ログインで確認する」ボタンのみとし、**「再度作成する」を同列に並置しない**（Req 5.2）
    - 「ログインで確認する」押下で `usePasskeyAuthentication` の **discoverable ログイン**
      （ユーザー名入力なし）を起動する（Req 5.2）。成功時は通常の Cookie セッションに到達（Req 5.3）
    - discoverable ログインが一律 `AUTHENTICATION_FAILED`（内部理由を区別しない）で失敗した **後にのみ**
      「再度作成する」を提示し、押下で `mutation.reset()` で Idle に戻す（Req 5.4）
    - 文言にサーバ内部詳細（スタックトレース / SQL / DB 名 / session ID 生値）を含めない（Req 5.5 / NFR 2.1）
  - `web/src/components/passkey-signup-dialog.test.tsx`:
    - `registration_uncertain` で専用見出しが表示され、初期に「再度作成する」が表示されない（Req 5.2）
    - 「ログインで確認する」押下で discoverable ログインが起動
    - discoverable ログイン一律失敗の後に「再度作成する」が表示され `mutation.reset()` が呼ばれる（Req 5.4）
    - 文言中にサーバ内部詳細 / 他 kind の代表文言が含まれない（Req 5.5 / 5.6）
  - `web/src/components/login-page-recovery.test.tsx`: `registration_uncertain` →「ログインで確認する」→
    discoverable ログイン成功 → 2 ペイン UI 到達（Req 5.3。unit で組めない場合は E2E 委譲を PR 本文に明記）
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, NFR 2.1_
  - _Boundary: components/passkey-signup-dialog, components/login-page-recovery_
  - _Depends: 5_

- [ ] 7. Config: `.env.sample` と `docker-compose.yml` に既存 WebAuthn env 群の documentation / passthrough と CORS_ALLOWED_ORIGIN の fail-closed 接続を追加する（新規 env なし）
  - `.env.sample`: Native Auth / CORS 節の後に、design §Delta 6 §反映内容 の canonical コメントブロックを
    追加する。5 変数（`WEBAUTHN_RP_ID` / `WEBAUTHN_RP_DISPLAY_NAME` / `WEBAUTHN_ORIGINS` /
    `WEBAUTHN_IOS_APP_ID` / `PASSKEY_CHALLENGE_TTL_SECONDS`）をコメントアウト行で documentation し、
    例値には「例:」プレフィクスを付ける。`CORS_ALLOWED_ORIGIN` 未設定時に capability=404 /
    `/api/auth/session`・Web registration=403（fail-closed）になる旨をコメントで明記する
  - `docker-compose.yml`: `api` サービスの `environment` に 5 変数の passthrough を
    `${VAR:-<既定 or 空>}` 形式で追加する（`worker` には追加しない）。`CORS_ALLOWED_ORIGIN` は既存 passthrough を
    維持（新規追加しない）
  - **新規 env は追加しない**（config.go / #216 で既読み込み済みの既存 env の documentation / passthrough に限定 / Req 6.5）
  - 既存 `internal/config/config_test.go` の env 読込テストは変更しない（本タスクは `internal/config/*` を触らない）
  - _Requirements: 2.2, 6.1, 6.2, 6.3, 6.4, 6.5, NFR 3.1_
  - _Boundary: .env.sample, docker-compose.yml_

- [ ] 8. Docs: `docs/specs/223-feat-web-web/impl-notes.md` と `context-map.md` に #231 delta の適用結果を追記する（#223 / #216 の本体 3 文書は不変）
  - `impl-notes.md`: 「#231 normative delta 適用後の実装状態」節を追加し、Delta 1〜6 について「本 PR で反映した
    内容」と「#231 design.md 該当 Delta への参照」を 1〜3 行で記載する。#223 design の「新規作成フロー
    （Sequence）」が #231 Delta 1（直接 Cookie session）で supersede される旨を明示する
  - `context-map.md`: 新規作成フロー経路図を「二度目 ceremony 除去 → `finish → 3 行 tx → commit →
    Set-Cookie` の直接 session」に更新する。fail-closed 表を #231 Delta 3 の 5 列版で supersede し、
    Delta 4 の CSRF 主防御・残余リスク（同一オリジン XSS / login CSRF）と完了不明状態（`registration_uncertain`）の
    遷移を追加する
  - **`docs/specs/223-feat-web-web/requirements.md` / `design.md` / `tasks.md` は書き換えない**（NFR 1.1）
  - **`docs/specs/216--app-store-4-8/` 配下のいかなるファイルも書き換えない**（NFR 1.3）
  - _Requirements: 2.1, 2.3, 2.4, 2.5, 4.2, 4.3, 4.4, 4.6, NFR 1.1, NFR 1.2, NFR 1.3_
  - _Boundary: docs/specs/223-feat-web-web/impl-notes, docs/specs/223-feat-web-web/context-map_

## Verify

本 spec の実装反映後、watcher（stage-a-verify gate）が独立に再実行して build / test / lint を
検証する。サーバ側は `go test ./...` + `go vet ./...`、Web 側は `npm test` + `npm run lint` +
`npm run build`。加えて `docker-compose.yml` の env 追加は **Go の config テストでは検証されない**
ため、代表 env 値を与えた `docker compose config`（YAML + env 置換の妥当性検証）を Verify に含める。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./... && \
CORS_ALLOWED_ORIGIN=https://example.com NATIVE_AUTH_JWT_SECRET=dummy-secret WEBAUTHN_RP_ID=example.com WEBAUTHN_ORIGINS=https://example.com docker compose config >/dev/null && \
( cd web && npm test && npm run lint && npm run build )
```
