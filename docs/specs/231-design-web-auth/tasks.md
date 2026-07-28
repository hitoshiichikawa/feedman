# Implementation Plan

本タスクリストは **PR #229 の `needs-iteration` 1 回**で反映する差分作業を、独立コミット
可能な粒度で列挙する。中核決定 1.A（`registration/finish` の同一 DB トランザクションで
user・credential・session の 3 行を作成し、commit 後に `Set-Cookie` する **直接 Cookie
session**）に基づく。**登録用 auth_code / 二度目 ceremony / 登録専用 session 交換は導入しない**。

**先行実装の前提（運用順序）**: user + credential を単一トランザクションで INSERT する基盤
（`passkey.RegistrationTx` / `RegistrationTxBeginner` / `RegistrationService.txBeginner` /
`UserWriter.CreateUserOnlyExec` / `PasskeyCredentialWriter.CreateExec` /
`passkeyRegistrationTxBeginnerAdapter`）は **Issue #230（PR #232）が実装済み**である。本タスク群は
その既存トランザクションを **session 行まで拡張**する（新しい tx 実行機構を作らない。
`internal/repository/tx.go` は変更しない）。運用順序は **#230（PR #232）を develop に merge →
PR #229 が本差分を needs-iteration で取り込む**。

**#223 spec（`docs/specs/223-feat-web-web/requirements.md` / `design.md` / `tasks.md` /
`review-notes.md`）および #216 / #230 spec の物理ファイルを書き換えるタスクは含まない**
（NFR 1.1 / 1.3）。反映対象は `internal/**` / `web/src/**` の製品コードとテスト、運用 config
（`.env.sample` / `docker-compose.yml`）、#223 spec の補助（`impl-notes.md` / `context-map.md`）に限定する。

各タスクは実装 + 対応テストを同一 commit で完結させる（`_Requirements_partial:_` は本ドラフトで
宣言なし = 全 AC が同一タスク内テスト必須）。

- [ ] 1. サーバ基盤: session 作成の tx 変種・共有 session factory・共有 Cookie builder・`WebPasskeyAllowedOrigin` config を追加する（#230 の tx 基盤・user/credential Exec 変種は再利用し追加しない）
  - `internal/repository/postgres_session_repo.go` に
    `CreateExec(ctx, q DBTX, s *model.Session) error` を **新規追加**し、既存 `Create` を
    `CreateExec(ctx, r.db, s)` への委譲に変更する（差分等価 / 既存 `DeleteByUserIDExec` の DBTX 変種
    パターンに準拠）。**#230 由来の `CreateUserOnlyExec` / credential `CreateExec` / `tx.go` は変更しない**
  - `internal/auth/service.go` の `generateSessionID` を参照可能にする export helper
    `NewSessionID() (string, error)` を同ファイルに追加する（既存 `createSession` の挙動は差分等価）
  - `internal/auth/session_factory.go` を **新規追加**し、`SessionFactory`（`NewSession(userID) (*model.Session, error)`
    が ID + 単一 `now` + `CreatedAt=now` + `ExpiresAt=now+TTL` を一貫生成）と、RegistrationService が受ける
    最小 IF `SessionFactoryFunc` を定義する。`now` はテスト差し替え可（既定 `time.Now`）
  - `internal/auth/session_exchange.go` の session 構築を `SessionFactory.NewSession` 呼び出しに差し替える
    （`SessionExchangeService` の外部契約・挙動は不変 / 差分等価）
  - `internal/handler/session_cookie.go` を **新規追加**し、canonical builder
    `buildSessionCookie(name, value, domain string, secure bool, maxAge int) *http.Cookie` を定義する。
    既存 `auth_handler.go::Callback` / `native_auth_handler.go::Session` のインライン Cookie 生成を
    本 builder 呼び出しに差し替える（属性は現行と完全一致・差分等価）
  - `internal/config/config.go` に `WebPasskeyAllowedOrigin string` を **新規追加**する
    （`os.Getenv("CORS_ALLOWED_ORIGIN")` の生値。空/未設定 → `""`。localhost へ default しない）。
    既存 `CORSAllowedOrigin`（CORS 層。既定 `http://localhost:3000`）は **変更しない**。新規 env は追加しない
  - テスト:
    - `internal/handler/session_cookie_test.go`（新規）: `buildSessionCookie` の属性
      （Name / HttpOnly / SameSite=Lax / Secure / Path / Domain / Max-Age）が Google OAuth Callback と一致
    - `internal/auth/session_factory_test.go`（新規）: `NewSession` が ID / `CreatedAt` / `ExpiresAt=CreatedAt+TTL`
      を単一 now で返し、ID 生成失敗時に error（部分構築なし）
    - `internal/auth/session_exchange_test.go`（既存編集）: factory 差し替え後も既存アサーションが pass し、
      `CreatedAt` / `ExpiresAt` 検証を追加
    - `internal/config/config_test.go`（既存編集）: `CORS_ALLOWED_ORIGIN` set → `WebPasskeyAllowedOrigin=値` /
      unset・empty → `""`、`CORSAllowedOrigin` は既定 localhost:3000 を維持
    - session repo `CreateExec` の委譲は既存 `Create` テストの差分等価で担保
  - _Requirements: 1.3, 3.6, 4.5, 6.5, NFR 2.1, NFR 3.1_
  - _Boundary: repository/postgres_session_repo, auth/session_factory, auth/session_exchange, auth/service, handler/session_cookie, config/config_

- [ ] 2. サーバ: `RegistrationService.FinishRegistrationNew` を既存 #230 tx クロージャ内の session INSERT に拡張し、wiring を更新する
  - `internal/passkey/registration_service.go`:
    - 最小 IF を追加: `SessionWriter{CreateExec}`（session repo の tx 変種）。**#230 既存の `UserWriter` /
      `PasskeyCredentialWriter` / `RegistrationTxBeginner` は再宣言しない**
    - 構造体に `sessions SessionWriter` / `sessionFactory auth.SessionFactoryFunc` を追加する
      （**auth_code 関連依存は追加しない**）。`NewRegistrationService` のシグネチャに追加し、
      `WebSessionReady() bool`（sessions / sessionFactory / txBeginner が非 nil）を追加する
    - `FinishRegistrationNew` のシグネチャを
      `(ctx, challengeID, requestBody) (string, error)` →
      `(ctx, challengeID, requestBody, issueWebSession bool) (userID string, webSession *model.Session, err error)`
      に変更する。既存 challenge consume / `adapter.FinishRegistration` / `txBeginner.BeginTx` /
      defer Rollback / `CreateUserOnlyExec` / credential `CreateExec` / `Commit` は **#230 のまま維持**する
    - credential `CreateExec` の **後・`Commit` の前**に、`issueWebSession` のとき
      `sessionFactory.NewSession(newUser.ID)` で session を作り `sessions.CreateExec(ctx, tx.Querier(), sess)` で
      同一 tx へ INSERT する。session 生成失敗 / INSERT 失敗 / commit 失敗は既存 defer Rollback により
      全取消し `webSession=nil` で伝播する（3 行 atomic / design §Delta 1）
    - session ID 生値・auth_code 平文をログ・エラー・レスポンスに残さない（NFR 2.1）
  - `internal/app/app.go`: `passkey.NewRegistrationService(...)` に `sessionRepo`（SessionWriter）と
    `auth.NewSessionFactory(sessionTTL)`（`sessionTTL = time.Duration(cfg.SessionMaxAge) * time.Second`）を
    注入する。これらは `NATIVE_AUTH_JWT_SECRET` に依存せず `WEBAUTHN_*` 設定時（passkey handler 生成時）に
    常時配線する。既存の `passkeyRegistrationTxBeginnerAdapter`（#230）注入はそのまま維持する
  - `internal/passkey/registration_service_test.go`:
    - 既存テストを新シグネチャ（4 引数 / 3 戻り値）に追従させる
    - 正常系（Web mode）: `issueWebSession=true` で fake tx の user → credential → session の呼び出し順を検証し、
      `webSession` に factory 生成の ID / `CreatedAt` / `ExpiresAt` が入る
    - 正常系（iOS mode）: `issueWebSession=false` で session を作らず `webSession=nil`
    - 異常系: user UNIQUE 衝突 → `ErrRegistrationFailed`（session 未作成）
    - 異常系: credential 重複 → `ErrRegistrationFailed`（session 未作成）
    - 異常系: session INSERT 失敗 / session factory の ID 生成失敗 → 全 Rollback・`webSession=nil`・エラー伝播
  - _Requirements: 1.1, 1.2, 1.3, 3.5, NFR 2.1, NFR 3.2_
  - _Boundary: passkey/RegistrationService, app.go wiring_
  - _Depends: 1_

- [ ] 3. サーバ: `PasskeyHandler.RegistrationFinish` に Origin ベース Web/iOS mode 判定・readiness・Web mode の Set-Cookie を追加し、実 PostgreSQL で 3 行原子性を検証する
  - `internal/handler/passkey_handler.go`:
    - `PasskeyHandler` に `allowedOrigin string`（= `WebPasskeyAllowedOrigin`）と Cookie 設定
      （domain / secure / maxAge）を Option で注入する（既存 `NativeAuthHandler` の `WithSession*` Option idiom を
      踏襲）。`webRegistrationReady()`（`allowedOrigin != "" && cookieMaxAge > 0 && regService.WebSessionReady()`。
      Domain 空・Secure=false は有効な構成として readiness 条件に含めない）を追加する
    - `RegistrationFinish` に mode 判定を追加する（challenge consume / mutation より前）:
      - `Origin` 不在 → native/iOS mode（`issueWebSession=false`、既存挙動 / JSON Content-Type 追加検証なし）
      - `Origin == allowedOrigin`（allowedOrigin 非空）→ Web mode。JSON Content-Type 非一致は 415、
        `!webRegistrationReady()` は fail-closed（500 相当）、いずれも mutation 前に返す
      - `Origin` 非空不一致 or allowedOrigin 未設定（空）→ 403 FORBIDDEN_ORIGIN（mutation なし）
    - `FinishRegistrationNew(ctx, challengeID, credential, webMode)` を呼び、`webSession != nil` のとき
      `buildSessionCookie(...)`（Task 1）で `Set-Cookie` する。レスポンスは Web / iOS とも 200 `{user_id}`（不変）
  - `internal/handler/passkey_handler_test.go`:
    - Origin 一致 → 200 `{user_id}` + `Set-Cookie session_id`（属性一致）
    - Origin 不在（iOS）→ 200 `{user_id}`・`Set-Cookie` **なし**（#216 回帰）
    - Origin 非空不一致 / allowedOrigin 未設定 → 403、regService stub が呼ばれない（mutation なし）
    - Web mode で Content-Type 非 JSON → 415、mutation 呼ばれない
    - `webRegistrationReady()` false（session writer / factory 未配線）→ 500、mutation 呼ばれない
  - `internal/handler/passkey_e2e_db_test.go`（実 PostgreSQL / 既存編集）:
    - Web mode finish 成功で users / passkey_credentials / sessions に **3 行がすべて存在**
    - session INSERT を失敗させた場合（重複 session ID 注入等）に users / passkey_credentials にも
      **行が残らない（3 行 atomic rollback / orphan なし）かつ Set-Cookie が出ない**
    - iOS mode finish 成功で users / passkey_credentials の 2 行のみ存在（sessions なし）
    - Web session の `CreatedAt` / `ExpiresAt` が Cookie の Max-Age（= SessionMaxAge）と整合する
  - _Requirements: 1.1, 1.3, 1.4, 3.5, 3.6, 4.5, NFR 2.1, NFR 3.2_
  - _Boundary: handler/PasskeyHandler, handler/passkey_e2e_db_test_
  - _Depends: 2_

- [ ] 4. サーバ: `NativeAuthHandler.Session` の Origin 検証を fail-closed へ補正し、router の fail-closed 表 5 列・partial wiring を実装・検証する
  - `internal/handler/native_auth_handler.go`: `Session` の Origin 検証を
    「`allowedOrigin == ""` → 403 / `Origin` 不在 → 403 / `Origin != allowedOrigin` → 403 /
    `Origin == allowedOrigin` のみ通過」に補正する（design §Delta 4）。注入する許可 Origin は
    `WebPasskeyAllowedOrigin`。Content-Type application/json 必須は既存維持
  - `internal/handler/router.go`: capability route 条件に `deps.WebPasskeyAllowedOrigin != ""` と
    `deps.PasskeyHandler.webRegistrationReady()` を **追加**する（`NativeAuthHandler.SessionReady()` は
    **維持し弱めない**）。`/api/auth/session` 条件は `NativeAuthHandler != nil && SessionReady()` を維持。
    iOS 用 `/api/passkey/registration/*` / `/api/passkey/authentication/*`（および Web と共有する
    `registration/finish`）は `PasskeyHandler != nil` を維持（#216 契約 / NFR 3.2）
  - `internal/handler/native_auth_handler_test.go`: Origin 不在・不一致・許可 Origin 未設定で 403 の異常系を追加
  - `internal/handler/router_test.go`: fail-closed 表 行 1〜5 の各 endpoint（列 A capability / 列 B session /
    列 C registration route / 列 D iOS registration/* / 列 E iOS authentication/*）の登録有無を table-driven で
    検証する。特に行 2（W✓ N✗ C✓）で iOS registration/*・authentication/* が 200・capability が 404、
    行 3（W✓ N✓ C✗）で capability 404、partial wiring（`webRegistrationReady()` 未充足）で capability が
    未登録になることを assert する
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 4.1, NFR 3.2_
  - _Boundary: handler/native_auth_handler, handler/router_
  - _Depends: 1_

- [ ] 5. Web: `api.ts` に 2xx parse 失敗の型付き正規化を追加し、`use-passkey-registration.ts` から二度目 ceremony を除去して `registration_uncertain` を分類する
  - `web/src/lib/api.ts`: `ResponseParseError`（`status: number`）を追加し、`request<T>` 末尾の
    `response.json()` を try/catch で包んで 2xx の parse 失敗を `ResponseParseError` に正規化する
    （204/205 分岐・`credentials: "include"` は不変 / 差分等価）。これにより dispatch 前の plain `TypeError` と
    「fetch 成功後の 2xx body parse 失敗」を呼出側が区別できる
  - `web/src/lib/api.test.ts`: 2xx body 欠損/途中切断/parse 不能 → `ResponseParseError`（status 2xx）、
    dispatch 前 serialize 失敗 → plain `TypeError` で両者が区別できることを検証する
  - `web/src/types/passkey.ts`: `PasskeyRegistrationErrorKind` に `"registration_uncertain"` を追加する。
    `RegistrationFinishResponse` は `{user_id}` のまま **auth_code を追加しない**
  - `web/src/hooks/use-passkey-registration.ts`:
    - `mutationFn` chain を「begin → create → finish → invalidateQueries」に短縮し、
      `authentication/begin` / `navigator.credentials.get` / `authentication/finish` / 登録用
      `POST /api/auth/session` を **完全に削除**する（Req 1.1 / 1.2）。`code_challenge` は #216 契約維持のため
      送るが `code_verifier` は使用しない。registration 経路は `session_exchange_failed` を発生させない
    - finish 段の error を分類する: `ResponseParseError`（2xx body 不能）/ `AbortError` / 5xx /
      送出後 `TypeError`（fetch reject）→ `registration_uncertain`。**全 4xx（400/401/403/404/409/422 等）→
      `server_rejected`（拒否確定。400 だけに限定しない）**。dispatch 前ローカル失敗 → `server_error`
      （design §Delta 5 §安全側 fail 原則）
    - `code_verifier` / attestation 生値を `console.*` / storage / URL に書かない（NFR 2.1）
  - `web/src/hooks/use-passkey-registration.test.tsx`:
    - 正常系: begin → create → finish の 3 呼び出しのみで `authentication/*` / 登録用 `/api/auth/session` が
      呼ばれない（二度目 ceremony 除去の回帰）
    - `registration_uncertain` × fetch reject / 5xx / AbortError / `ResponseParseError`(2xx) の 4 サブケース
    - 回帰: 全 4xx（400/401/403/404/409/422）が `server_rejected` で uncertain に誤分類されない
    - dispatch 前 `TypeError` が `server_error`（非-uncertain）に分類される
    - begin 段の `invalid_username` / `username_taken` / `cancelled` は既存挙動維持
  - _Requirements: 1.1, 1.2, 1.4, 1.5, 5.1, 5.5, NFR 2.1, NFR 3.1_
  - _Boundary: lib/api, hooks/use-passkey-registration, types/passkey_
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
    - login の cancel / network / 5xx では初期画面に「再度作成する」が出ない（一律 AUTHENTICATION_FAILED の後のみ）
    - 文言中にサーバ内部詳細 / 他 kind の代表文言が含まれない（Req 5.5 / 5.6）
  - `web/src/components/login-page-recovery.test.tsx`: `registration_uncertain` →「ログインで確認する」→
    discoverable ログイン成功 → 2 ペイン UI 到達（Req 5.3。unit で組めない場合は E2E 委譲を PR 本文に明記）
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, NFR 2.1_
  - _Boundary: components/passkey-signup-dialog, components/login-page-recovery_
  - _Depends: 5_

- [ ] 7. Config: `.env.sample` に CORS_ALLOWED_ORIGIN の fail-closed 接続を明記し、`docker-compose.yml` の `api` CORS 既定を空へ変更する（新規 env なし）
  - `.env.sample`: 既存 CORS 節に「`CORS_ALLOWED_ORIGIN` は Web パスキーの exact Origin 検証にも使用され、
    未設定（空）だと capability=404 / `/api/auth/session`・Web registration=403（fail-closed）になり Google 単体へ
    縮退する」旨をコメントで明記する。WebAuthn env ブロックは PR #229 で追加済みのため記述整備のみ
  - `docker-compose.yml`: `api` サービスの `CORS_ALLOWED_ORIGIN=${CORS_ALLOWED_ORIGIN:-http://localhost:3000}` を
    `${CORS_ALLOWED_ORIGIN:-}` に変更する（未設定 → 空 → fail-closed 到達可能。CORS ミドルウェアは config.go の
    既定で localhost:3000 に fall back / NFR 3.1）。WebAuthn passthrough（PR #229 追加済み）は変更しない
  - **新規 env は追加しない**（既存 `CORS_ALLOWED_ORIGIN` env の documentation / 既定変更に限定 / Req 6.5）
  - `internal/config/config_test.go` の `WebPasskeyAllowedOrigin` テストは Task 1 で追加済み（本タスクは env ファイルのみ）
  - _Requirements: 2.2, 6.1, 6.2, 6.3, 6.4, 6.5, NFR 3.1_
  - _Boundary: .env.sample, docker-compose.yml_

- [ ] 8. Docs: `docs/specs/223-feat-web-web/impl-notes.md` と `context-map.md` に #231 delta の適用結果を追記する（#223 / #216 / #230 の本体 3 文書は不変）
  - `impl-notes.md`: 「#231 normative delta 適用後の実装状態」節を追加し、Delta 1〜6 について「本 PR で反映した
    内容」と「#231 design.md 該当 Delta への参照」を 1〜3 行で記載する。#223 design の「新規作成フロー
    （Sequence）」が #231 Delta 1（直接 Cookie session）で supersede される旨と、#230 tx を session まで拡張した旨を明示する
  - `context-map.md`: 新規作成フロー経路図を「二度目 ceremony 除去 → `finish → 3 行 tx → commit →
    Set-Cookie` の直接 session」に更新する。fail-closed 表を #231 Delta 3 の 5 列版で supersede し、
    Delta 4 の CSRF 主防御・残余リスク（同一オリジン XSS / login CSRF）と完了不明状態（`registration_uncertain`）の
    遷移を追加する
  - **`docs/specs/223-feat-web-web/requirements.md` / `design.md` / `tasks.md` / `review-notes.md` は書き換えない**（NFR 1.1）
  - **`docs/specs/216--app-store-4-8/` / `docs/specs/230-fix-passkey-credential/` 配下のいかなるファイルも書き換えない**（NFR 1.3）
  - _Requirements: 2.1, 2.3, 2.4, 2.5, 4.2, 4.3, 4.4, 4.6, 4.7, NFR 1.1, NFR 1.2, NFR 1.3_
  - _Boundary: docs/specs/223-feat-web-web/impl-notes, docs/specs/223-feat-web-web/context-map_

## Verify

本 spec の実装反映後、watcher（stage-a-verify gate）が独立に再実行して build / test / lint を
検証する。サーバ側は `go test ./...` + `go vet ./...`、Web 側は `npm test` + `npm run lint` +
`npm run build`。加えて `docker-compose.yml` の CORS 既定変更は **Go の config テストでは検証されない**
ため、代表 env 値を与えた `docker compose config`（YAML + env 置換の妥当性検証）を Verify に含める。

`docker compose config` は `SESSION_SECRET` / `POSTGRES_PASSWORD` を必須（`:?...required`）とするため、
両者に代表値を与える。`GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` も既定を持たず空だと警告になるため
代表値を与える（いずれもダミー値で config 検証のみ / 秘密値ではない）。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./... && \
SESSION_SECRET=dummy-session-secret POSTGRES_PASSWORD=dummy-postgres-pass \
GOOGLE_CLIENT_ID=dummy GOOGLE_CLIENT_SECRET=dummy \
CORS_ALLOWED_ORIGIN=https://example.com NATIVE_AUTH_JWT_SECRET=dummy-secret \
WEBAUTHN_RP_ID=example.com WEBAUTHN_ORIGINS=https://example.com \
docker compose config >/dev/null && \
( cd web && npm test && npm run lint && npm run build )
```
