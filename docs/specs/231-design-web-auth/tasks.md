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
  - `internal/auth/session_factory.go` を **新規追加**し、`SessionFactory`（`NewSession(userID) (*model.Session, error)`
    が ID + 単一 `now` + `CreatedAt=now` + `ExpiresAt=now+TTL` を一貫生成）と、RegistrationService が受ける
    最小 IF `SessionFactoryFunc` を定義する。`now`（既定 `time.Now`）と **`newID func() (string, error)`（既定 =
    同一 package の unexported `generateSessionID`）** をいずれもテスト差し替え可能な seam として持たせる。
    **`internal/auth/service.go` は変更しない**（`generateSessionID` は unexported のまま同一 package から参照する。
    薄い public `NewSessionID` は追加しない / Blocker #2）
  - `internal/auth/session_exchange.go` の `now` / `sessionTTL` fields を
    `sessionFactory SessionFactoryFunc` に置き換え、constructor を
    `NewSessionExchangeService(authCodes, sessions, sessionFactory)` に変更する。session 構築を
    `sessionFactory.NewSession(stored.UserID)` 呼び出しに差し替える
    （auth_code 交換の外部挙動は不変 / 差分等価）
  - `internal/handler/session_cookie.go` を **新規追加**し、canonical builder
    `buildSessionCookie(name, value, domain string, secure bool, maxAge int) *http.Cookie` を定義する。
    既存 `auth_handler.go::Callback` / `native_auth_handler.go::Session` のインライン Cookie 生成を
    本 builder 呼び出しに差し替える（属性は現行と完全一致・差分等価）
  - `internal/config/config.go` に `WebPasskeyAllowedOrigin string` を **新規追加**する
    （`os.Getenv("CORS_ALLOWED_ORIGIN")` を **trim + strict exact-Origin validation** に通した値。
    前後空白だけは trim して採用する。trim 後空 / scheme が http(s) 以外 / host を欠く /
    userinfo・opaque・path・RawPath・query・fragment・ForceQuery 付き / 末尾スラッシュ /
    trim 後の内部空白・カンマで複数 origin を含む / `scheme://host` の canonical serialization と不一致、
    のいずれかは **`""` に倒す**（fail-closed）。localhost へ default しない / Blocker #6）。
    invalid warn は generic とし **生 Origin 値をログしない**
    既存 `CORSAllowedOrigin`（CORS 層。既定 `http://localhost:3000`）は **変更しない**。新規 env は追加しない
  - テスト:
    - `internal/handler/session_cookie_test.go`（新規）: `buildSessionCookie` の属性
      （Name / HttpOnly / SameSite=Lax / Secure / Path / Domain / Max-Age）が Google OAuth Callback と一致
    - `internal/auth/session_factory_test.go`（新規）: `NewSession` が ID / `CreatedAt` / `ExpiresAt=CreatedAt+TTL`
      を単一 `now` で返し、**`newID` seam に失敗関数を注入**して ID 生成失敗時に error（部分構築なし）を検証する
      （`crypto/rand.Reader` の global 差替えをしない / Blocker #2）
    - `internal/auth/session_exchange_test.go`（既存編集）: constructor に fake factory を注入し、
      `NewSession(stored.UserID)` の呼出し・生成 session の保存を検証する。factory 失敗時は保存せず error を返し、
      既存 auth_code 交換アサーションも pass する
    - `internal/config/config_test.go`（既存編集）: `CORS_ALLOWED_ORIGIN` valid set → `WebPasskeyAllowedOrigin=正規化値` /
      前後空白付き valid → trim 済み値 / unset・empty・whitespace-only → `""` /
      **malformed（内部空白 / 末尾スラッシュ / path・RawPath・query・fragment・userinfo・opaque・ForceQuery 付き /
      複数 origin）→ `""`**（fail-closed / Blocker #6）、`CORSAllowedOrigin` は既定 localhost:3000 を維持し、
      invalid warn に生値を含めない
    - session repo `CreateExec` の委譲は既存 `Create` テストの差分等価で担保
  - _Requirements: 1.3, 3.6, 4.5, 6.5, NFR 2.1, NFR 3.1_
  - _Boundary: repository/postgres_session_repo, auth/session_factory, auth/session_exchange, handler/session_cookie, handler/auth_handler, handler/native_auth_handler, config/config_

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
  - `internal/app/app.go`: `sessionTTL := time.Duration(cfg.SessionMaxAge) * time.Second` と
    `sessionFactory := auth.NewSessionFactory(sessionTTL)` を **`NATIVE_AUTH_JWT_SECRET` 条件の外で 1 回だけ**
    構築する。NATIVE secret 設定時は同じ `sessionFactory` を
    `auth.NewSessionExchangeService(authCodeRepo, sessionRepo, sessionFactory)` へ渡し、WEBAUTHN 設定時は
    `passkey.NewRegistrationService(...)` へ `sessionRepo`（SessionWriter）と **同一 instance** を渡す。
    RegistrationService の配線は NATIVE secret に依存しない。既存の
    `passkeyRegistrationTxBeginnerAdapter`（#230）注入はそのまま維持する。
    Google OAuth の `auth.Service.createSession` は既存の独立実装を変更しない
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
  - `web/src/lib/api.ts`: `ResponseParseError`（`status: number`）と `RequestPreparationError` を追加する。
    `request<T>` の **request preparation（`JSON.stringify` / URL 構築）を try/catch で囲んで `RequestPreparationError`**
    に、**末尾 `response.json()` を try/catch で囲んで 2xx parse 失敗を `ResponseParseError`** に正規化する
    （204/205 分岐・`credentials: "include"` は不変 / 差分等価。`fetch()` reject の plain `TypeError` はそのまま透過）。
    これにより「dispatch 前の preparation 失敗（commit 不成立確定）/ fetch reject
    （request の実送達可否が不明・不確定）/ 2xx parse 失敗（不確定）」の 3 起点を呼出側が
    **plain TypeError の発生位置を推測せず** 区別できる（Blocker #1）
  - `web/src/lib/api.test.ts`: 2xx body 欠損/途中切断/parse 不能 → `ResponseParseError`（status 2xx）、
    dispatch 前の preparation 失敗（`JSON.stringify` 循環参照 / URL 構築失敗）→ `RequestPreparationError`、
    `fetch()` reject → plain `TypeError` の **3 起点が機械的に区別できる**ことを検証する（Blocker #1）
  - `web/src/types/passkey.ts`: PR #229 の API DTO 定義を維持する。
    `RegistrationFinishResponse` は `{user_id}` のまま **auth_code を追加しない**。
    domain error kind は実在する各 hook 内の定義を拡張し、本 DTO ファイルへ移動・重複定義しない
  - `web/src/hooks/use-passkey-registration.ts`:
    - 本ファイル内の `PasskeyRegistrationErrorKind` に `"registration_uncertain"` を追加する
    - `mutationFn` chain を「begin → create → finish → invalidateQueries」に短縮し、
      `authentication/begin` / `navigator.credentials.get` / `authentication/finish` / 登録用
      `POST /api/auth/session` を **完全に削除**する（Req 1.1 / 1.2）。`code_challenge` は #216 契約維持のため
      送るが `code_verifier` は使用しない。registration 経路は `session_exchange_failed` を発生させない
    - finish 段の error を分類する（`RequestPreparationError` を最優先で判定）: `RequestPreparationError`
      （dispatch 前 = commit 不成立確定）→ `server_error`。`ResponseParseError`（2xx body 不能）/ `AbortError` / 5xx /
      **`fetch()` reject の plain `TypeError`（実送達可否不明）** → `registration_uncertain`。**全 4xx（400/401/403/404/409/422 等）→
      `server_rejected`（拒否確定。400 だけに限定しない）**（design §Delta 5 §安全側 fail 原則 / Blocker #1）
    - `code_verifier` / attestation 生値を `console.*` / storage / URL に書かない（NFR 2.1）
  - `web/src/hooks/use-passkey-registration.test.tsx`:
    - 正常系: begin → create → finish の 3 呼び出しのみで `authentication/*` / 登録用 `/api/auth/session` が
      呼ばれない（二度目 ceremony 除去の回帰）
    - `registration_uncertain` × fetch reject / 5xx / AbortError / `ResponseParseError`(2xx) の 4 サブケース
    - 回帰: 全 4xx（400/401/403/404/409/422）が `server_rejected` で uncertain に誤分類されない
    - dispatch 前の `RequestPreparationError` が `server_error`（非-uncertain）に分類され、fetch reject の
      plain `TypeError`（uncertain）と取り違えられない（Blocker #1）
    - begin 段の `invalid_username` / `username_taken` / `cancelled` は既存挙動維持
  - _Requirements: 1.1, 1.2, 1.4, 1.5, 5.1, 5.5, NFR 2.1, NFR 3.1_
  - _Boundary: lib/api, hooks/use-passkey-registration（types/passkey は DTO 契約確認のみ・変更なし）_
  - _Depends: 3_

- [ ] 6. Web: discoverable ログイン結果を machine-readable kind に正規化し、`passkey-signup-dialog.tsx` に完了不明状態 UI と段階提示の復旧導線を追加する
  - `web/src/hooks/use-passkey-authentication.ts`（既存編集 / Blocker #3）:
    - 本ファイル内の `PasskeyAuthErrorKind` に `authentication_failed` を追加する。
      **`step === STEP_AUTH_FINISH && err instanceof ApiError && err.status === 400 &&
      extractApiErrorCode(err) === "AUTHENTICATION_FAILED"` の完全一致だけ**をこの kind に分類する。
      code は安全な shape guard で文字列だけを読み、**raw body / 内部理由（credential 未解決 等）を
      error に含めない**（NFR 2.1）
    - begin の同じ 400/code / finish の異なる status または code / cancel（`NotAllowedError`）/
      network（fetch reject）/ 5xx（同じ code を含む）/ session 交換段
      （`/api/auth/session`）失敗は **`authentication_failed` 以外の別 kind** に保つ（再作成条件に該当させない）
  - `web/src/hooks/use-passkey-authentication.test.tsx`（既存編集 / Blocker #3）:
    - finish + status 400 + code `AUTHENTICATION_FAILED` → `authentication_failed` kind
    - begin の同一 400/code、finish の異なる status/code、cancel、network、同じ code の 5xx、
      session 交換失敗が別 kind になる negative test
    - error に内部理由 / raw body が含まれない（NFR 2.1）
  - `web/src/components/passkey-buttons.tsx`: `authentication_failed` を既存 `server_rejected` と同じ
    generic 固定文言へ明示 map する（`Partial<Record>` の mapping 欠落で通常ログインのエラー表示を
    消さない）。raw code / body は DOM に反射しない
  - `web/src/components/passkey-buttons.test.tsx`: 通常ログインの `authentication_failed` で generic 文言が
    表示され、`AUTHENTICATION_FAILED` / raw body が表示されないことを検証する
  - `web/src/components/passkey-signup-dialog.tsx`:
    - `error.kind === "registration_uncertain"` 分岐を追加する。初期表示は見出し「登録が完了したかどうかを
      確認できませんでした」+ 「ログインで確認する」ボタンのみとし、**「再度作成する」を同列に並置しない**（Req 5.2）
    - PR #229 既存の `isError && registered` 自動 close effect より uncertain 分岐を優先し、
      `registration_uncertain` を当該 effect から明示除外する。初期 uncertain では
      `onAccountCreatedNeedsLogin` / `onOpenChange(false)` / `mutation.reset()` を呼ばず Dialog を開いたままにする。
      直接 flow で到達不能になる旧 post-finish エラーとの互換用に `registered` field を残す場合も、この優先順位を守る
    - 「ログインで確認する」押下で `usePasskeyAuthentication` の **discoverable ログイン**
      （ユーザー名入力なし）を起動する（Req 5.2）。成功時は通常の Cookie セッションに到達（Req 5.3）
    - discoverable ログインの結果 error kind が `authentication_failed`
      （finish の 400 `AUTHENTICATION_FAILED` / §auth hook）の **後にのみ**
      「再度作成する」を提示する。押下時は registration mutation と recovery authentication mutation の
      **双方を reset** して Idle に戻し、古い `authentication_failed` を次の uncertain 表示へ持ち越さない
      （Req 5.4）。他 kind では提示しない
    - 文言にサーバ内部詳細（スタックトレース / SQL / DB 名 / session ID 生値）を含めない（Req 5.5 / NFR 2.1）
  - `web/src/components/passkey-signup-dialog.test.tsx`:
    - `registration_uncertain`（legacy `registered=true` を含む）で専用見出しが表示され、初期に
      「再度作成する」が表示されず、`onAccountCreatedNeedsLogin` / `onOpenChange(false)` /
      `mutation.reset()` が呼ばれない（Req 5.2）
    - 「ログインで確認する」押下で discoverable ログインが起動
    - discoverable ログインが finish の 400 `AUTHENTICATION_FAILED` になった後に
      「再度作成する」が表示され、押下で registration / recovery authentication の両 mutation が reset される。
      その後の新しい uncertain 初期表示に古い auth error が持ち越されない（Req 5.4）
    - 復旧ログインの kind が `authentication_failed` のときのみ「再度作成する」が出る。begin 拒否 / cancel /
      network / 5xx / session 交換失敗では初期画面にも失敗後にも出ない
      （finish の 400 `AUTHENTICATION_FAILED` の後のみ / Blocker #3）
    - 文言中にサーバ内部詳細 / 他 kind の代表文言が含まれない（Req 5.5 / 5.6）
  - `web/src/components/login-page-recovery.test.tsx`: `registration_uncertain` →「ログインで確認する」→
    discoverable ログイン成功 → 2 ペイン UI 到達（Req 5.3。unit で組めない場合は E2E 委譲を PR 本文に明記）
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, NFR 2.1_
  - _Boundary: hooks/use-passkey-authentication, components/passkey-buttons,
    components/passkey-signup-dialog, components/login-page-recovery_
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
`npm run build`。加えて `docker-compose.yml` の CORS 既定変更（`${CORS_ALLOWED_ORIGIN:-}` の
default-empty 経路）は **Go の config テストでは検証されない**ため、代表 env 値を与えた
`docker compose config` を **`CORS_ALLOWED_ORIGIN` set / unset の 2 通り**で実行し、api サービスへ
展開された値（set=明示 origin / unset=空）を **exit 0 だけでなく実値で assertion** する（Blocker #7）。

`docker compose config` は `SESSION_SECRET` / `POSTGRES_PASSWORD` を必須（`:?...required`）とするため、
両者に代表値を与える。`GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` も既定を持たず空だと警告になるため
代表値を与える（いずれもダミー値で config 検証のみ / 秘密値ではない）。set ケースは
`CORS_ALLOWED_ORIGIN=https://example.com` を与えて api の展開値が明示 origin であること、unset ケースは
`CORS_ALLOWED_ORIGIN` を **unset** して `${CORS_ALLOWED_ORIGIN:-}` の default-empty を通り api の展開値が
空になることを検証する（従来の明示設定のみの Verify では default-empty 経路を一度も通らなかった / Blocker #7）。

<!-- stage-a-verify -->
```sh
set -eu
go test ./...
go vet ./...
COMMON="SESSION_SECRET=dummy-session-secret POSTGRES_PASSWORD=dummy-postgres-pass GOOGLE_CLIENT_ID=dummy GOOGLE_CLIENT_SECRET=dummy NATIVE_AUTH_JWT_SECRET=dummy-secret WEBAUTHN_RP_ID=example.com WEBAUTHN_ORIGINS=https://example.com"
# set: 明示 origin が api の CORS_ALLOWED_ORIGIN へ展開される（実値 assertion）
set_cfg=$(env $COMMON CORS_ALLOWED_ORIGIN=https://example.com docker compose config)
printf '%s\n' "$set_cfg" | grep -E '^[[:space:]]*CORS_ALLOWED_ORIGIN:[[:space:]]*"?https://example\.com"?[[:space:]]*$'
# unset: ${CORS_ALLOWED_ORIGIN:-} の default-empty を通り api の値が空へ展開される（実値 assertion）
unset_cfg=$(env -u CORS_ALLOWED_ORIGIN $COMMON docker compose config)
printf '%s\n' "$unset_cfg" | grep -E '^[[:space:]]*CORS_ALLOWED_ORIGIN:[[:space:]]*("")?[[:space:]]*$'
cd web
npm test
npm run lint
npm run build
```
