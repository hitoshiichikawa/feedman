# Implementation Plan

本タスクリストは #216（サーバ側パスキー API）を Web に接続する Feedman 固有の実装単位で
構成する。auth_code → Cookie session 合流方式は `POST /api/auth/session` の新規追加で実現し、
Web は既存 shadcn/ui + TanStack Query + `web/src/lib/api.ts` を再利用する（詳細は
`design.md` §Architecture Pattern & Boundary Map / §File Structure Plan）。

per-task ループ運用時のテスト境界: 各タスクは実装 + テスト（境界内 unit / component）を
同一 commit で完結させる。同 task 内テストが困難な場合のみ `_Requirements_partial:_` を
明示する（本ドラフトでは全 task が同 task 内テスト完結）。

- [x] 1. サーバ: `SessionExchangeService` を追加し、auth_code + code_verifier を Cookie session に交換する経路を用意する
  - `internal/auth/session_exchange.go` を新規追加し、`SessionCreator` interface（`Create` のみ）
    と `SessionExchangeService` を定義する（`AuthCodeConsumer` は既存 `token_service.go` の
    interface を再利用 / interface segregation / CLAUDE.md §5）
  - `ExchangeAuthCodeForSession(ctx, authCode, codeVerifier) (*model.Session, error)` を実装。
    処理順序は `HashNativeSecret → AuthCodeConsumer.FindByHash → VerifyPKCES256Verifier →
    AuthCodeConsumer.MarkUsed → session_id 生成（32 バイト crypto random / hex）→
    SessionCreator.Create` の 5 段
  - 拒否は既存 `ErrInvalidGrant` に uniform 化（未検出 / PKCE 不一致 / used / expired）。
    infra エラーは `fmt.Errorf("...: %w", err)` で wrap
  - 平文 `authCode` / `codeVerifier` / `sessionID` を slog・エラーメッセージに残さない
    （hash 先頭 8 文字方針を継承 / NFR 1.1）
  - `internal/auth/session_exchange_test.go` を新規追加し、正常系（1 件）と `ErrInvalidGrant`
    の 3 分岐（FindByHash nil / VerifyPKCES256Verifier false / MarkUsed が
    `ErrAuthCodeNotUsable`）と infra wrap（各層 error）を stub 依存で検証する
  - _Requirements: 3.1, 4.2, NFR 1.1_
  - _Boundary: SessionExchangeService_

- [x] 2. サーバ: `POST /api/auth/session` handler と wiring を追加する（既存 Cookie 属性と厳密一致）
  - `internal/handler/native_auth_handler.go` の `NativeAuthHandler` に `Session(w, r)` メソッドを追加。
    `dec.DisallowUnknownFields()` で `{auth_code, code_verifier}` を厳格 decode（既存 `Token`
    流儀）。必須欠落 / JSON 不正 → 400 INVALID_REQUEST（既存 `invalidRequestError` 相当）。
    `SessionExchangeService.ExchangeAuthCodeForSession` を呼び、`ErrInvalidGrant` は 400
    INVALID_GRANT（既存 `errInvalidGrant` 応答体）へ、infra error は 500 INTERNAL_ERROR
    （既存 `WriteInternalServerError`）へマッピング
  - 成功時は既存 Google OAuth Callback と **完全同一属性**（Name `session_id` / HttpOnly=true /
    SameSite=Lax / Secure=`cookieSecure` / Path=`/` / Domain=`cookieDomain` / Max-Age=`sessionMaxAge`）
    で Set-Cookie して 204 No Content。応答ボディは空
  - `NativeAuthHandler` 構造体に `sessionExchange *auth.SessionExchangeService` / `cookieDomain
    string` / `cookieSecure bool` / `sessionMaxAge int` を追加し、`NewNativeAuthHandler` の
    シグネチャを拡張。既存 `Token` / `Refresh` / `Revoke` メソッドの挙動は完全不変（NFR 2.1）
  - `internal/handler/router.go` の認証不要グループ、既存 `/api/auth/token` 系ブロック内に
    `if deps.NativeAuthHandler != nil { r.With(unauthIPMW, middleware.NewMaxBodyBytesMiddleware(middleware.DefaultMaxBodyBytes)).Post("/api/auth/session", deps.NativeAuthHandler.Session) }`
    を追記（既存 route の順序を変えない / NFR 2.1）
  - `internal/app/app.go` の wiring で `auth.NewSessionExchangeService(authCodeRepo, sessionRepo, sessionTTL)`
    を構築し、`NewNativeAuthHandler` に注入。既存 `NATIVE_AUTH_JWT_SECRET` 未設定分岐で
    NativeAuthHandler が nil のとき本 endpoint も連動 fail-closed（既存 fail-closed 挙動と同期）
  - `internal/handler/native_auth_handler_test.go` に以下 4 ケースを追加:
    204 応答 + Set-Cookie 属性（Name / HttpOnly / SameSite / Secure / Max-Age / Domain 全一致）、
    400 INVALID_REQUEST（JSON 不正 / 必須欠落）、400 INVALID_GRANT（stub が `ErrInvalidGrant` を返した時）、
    500 INTERNAL_ERROR（stub が infra error を返した時）
  - `internal/handler/router_test.go` に `NativeAuthHandler` nil / 非 nil の各分岐で
    `POST /api/auth/session` の登録有無（404 / 204）を検証するケースを追加
  - `internal/handler/router_unauth_ratelimit_test.go` に `POST /api/auth/session` が
    unauthIPMW の閾値超過で 429（既存応答形式）を返すことを検証するケースを追加
  - _Requirements: 3.1, 3.4, 4.2, 4.7, NFR 1.1, NFR 2.1, NFR 2.2_
  - _Boundary: NativeAuthHandler, Router_
  - _Depends: 1_

- [x] 3. サーバ: `GET /api/passkey/capability` handler を追加する（fail-closed で non-nil = 有効判定を Web に提供）
  - `internal/handler/passkey_handler.go` の `PasskeyHandler` に `Capability(w, r)` メソッドを追加。
    常に `Content-Type: application/json` + `Cache-Control: no-store` で 200 `{"available":true}` を返す。
    RP ID / origins / IOS App ID 等の env 由来値を一切ボディ / ヘッダに露出させない（NFR 1.1）
  - `internal/handler/router.go` の認証不要グループ、既存 `/api/passkey/registration|authentication/*`
    ブロックの直後に `if deps.PasskeyHandler != nil { r.With(unauthIPMW).Get("/api/passkey/capability", deps.PasskeyHandler.Capability) }`
    を追記（PasskeyHandler nil = 未登録 = 404 = Web は「非提供」と判定 / Requirement 5.2）
  - `internal/handler/passkey_handler_test.go` に以下 3 ケースを追加:
    200 応答形式（`{available: true}`）、`Content-Type: application/json`、`Cache-Control: no-store` の各ヘッダ検証
  - `internal/handler/router_test.go` に `PasskeyHandler` nil / 非 nil の各分岐で
    `GET /api/passkey/capability` の登録有無（404 / 200）を検証するケースを追加
  - `internal/handler/router_unauth_ratelimit_test.go` に `GET /api/passkey/capability` が
    unauthIPMW の閾値超過で 429（既存応答形式）を返すことを検証するケースを追加
  - _Requirements: 5.2, NFR 1.1, NFR 2.1_
  - _Boundary: PasskeyHandler, Router_

- [x] 4. Web: `web/src/lib/pkce.ts` を追加し、PKCE code_verifier / code_challenge (S256) 生成の純粋 utility を実装する
  - `generatePkcePair(): Promise<PkcePair>` を実装。`crypto.getRandomValues(new Uint8Array(32))`
    → base64url（`+/=` 除去 / padding なし）で code_verifier（43 文字 = 32 バイト base64url 化）。
    `crypto.subtle.digest("SHA-256", verifierBytes)` → base64url で code_challenge（43 文字）
  - 副作用なし・throw なし（WebCrypto API の rejection は呼び出し側に伝播）
  - `web/src/lib/pkce.test.ts` を追加し、生成値が RFC 7636 §4.1（`[A-Za-z0-9._~-]{43,128}`）と
    §4.2（`[A-Za-z0-9_-]{43}`）の正規表現に一致することを 3 回サンプル検証、既知
    verifier（固定入力）→ 既知 challenge の対応 1 件（SHA-256 派生の正しさ）
  - vitest jsdom 環境の `globalThis.crypto.subtle` を利用（Node 20+ / Vitest 4 標準搭載）
  - _Requirements: 2.2, 4.2, NFR 1.1_
  - _Boundary: lib/pkce_

- [x] 5. Web: `web/src/lib/webauthn.ts` を追加し、base64url ↔ ArrayBuffer 変換と WebAuthn options / response の JSON 相互変換を実装する
  - `base64urlToArrayBuffer(s: string): ArrayBuffer` と `arrayBufferToBase64url(buf: ArrayBuffer | Uint8Array): string`
    をブラウザ標準 `atob` / `btoa` で実装（`+/=` の base64 → base64url 変換 / padding 除去）
  - `decodeCreationOptions(raw: unknown): CredentialCreationOptions` を実装。
    `raw.publicKey.challenge` / `raw.publicKey.user.id` / `raw.publicKey.excludeCredentials[].id`
    を base64url → `ArrayBuffer` に変換、他フィールドは透過的にコピー
  - `decodeRequestOptions(raw: unknown): CredentialRequestOptions` を実装。
    `raw.publicKey.challenge` / `raw.publicKey.allowCredentials[].id` を変換
  - `encodeAttestationResponse(cred: PublicKeyCredential): unknown` を実装。
    `cred.rawId` / `cred.response.clientDataJSON` / `cred.response.attestationObject` を
    base64url 化した JSON（サーバの `protocol.ParseCredentialCreationResponseBytes` が期待する形式）
  - `encodeAssertionResponse(cred: PublicKeyCredential): unknown` を実装。
    `cred.rawId` / `cred.response.clientDataJSON` / `cred.response.authenticatorData` /
    `cred.response.signature` / `cred.response.userHandle` を base64url 化
  - `web/src/lib/webauthn.test.ts` を追加し、base64url 変換の境界値（0 / 31 / 32 バイト /
    RFC 4648 §10 の "M" / "Ma" / "Man" 例値）、`decodeCreationOptions` /
    `decodeRequestOptions` の変換フィールドを個別に assert、`encodeAttestationResponse` /
    `encodeAssertionResponse` の JSON shape を assert（`PublicKeyCredential` は最小限の
    テスト用オブジェクトを組み立て）
  - サーバから受け取った base64url 生値をログ・storage に書かない（本ファイル内で
    `console.*` を呼ばない / NFR 1.1）
  - _Requirements: 2.2, 2.7, 4.1, 4.5, NFR 1.1_
  - _Boundary: lib/webauthn_

- [ ] 6. Web: `web/src/types/passkey.ts` + `web/src/lib/passkey-capability.ts` + `web/src/hooks/use-passkey-capability.ts` を追加し、サーバ / ブラウザ合成の capability 判定を提供する
  - `web/src/types/passkey.ts` を新規追加し、`RegistrationBeginRequest` /
    `PasskeyBeginResponse` / `PasskeyFinishRequest` / `RegistrationFinishResponse` /
    `AuthenticationBeginRequest` / `AuthenticationFinishResponse` / `SessionExchangeRequest` /
    `CapabilityResponse` の 8 型を定義（design.md §types/passkey.ts の列挙）
  - `web/src/lib/passkey-capability.ts` を新規追加し、
    `isPasskeyBrowserSupported(): boolean` を実装。`typeof window !== "undefined" &&
    typeof window.PublicKeyCredential === "function"` を返す純粋関数
  - `web/src/hooks/use-passkey-capability.ts` を新規追加。TanStack Query の `useQuery` で
    `GET /api/passkey/capability` を呼ぶ。`retry: false` / `staleTime: Infinity` /
    `gcTime: Infinity`。404 と ネットワークエラー（`ApiError` catch）は server=false、200 は
    server=true。`available = serverAvailable && isPasskeyBrowserSupported()` の AND
  - `web/src/lib/passkey-capability.test.ts` を追加し、
    - `window.PublicKeyCredential` が function として存在するとき true
    - undefined のとき false
    - 存在するが function 以外（例: object）のとき false
    - SSR 環境相当（`window` を stub で除去）で false
    の 4 ケースを検証
  - `web/src/hooks/use-passkey-capability.test.tsx` を追加し、
    - サーバ 200 + browser あり → `available: true`
    - サーバ 404 + browser あり → `available: false`
    - サーバ 200 + browser なし → `available: false`
    - サーバ fetch reject + browser あり → `available: false`
    の 4 ケースを `QueryClientProvider` + `apiClient.get` mock で検証
  - _Requirements: 5.1, 5.2, 5.3, 5.4, NFR 1.4_
  - _Boundary: types/passkey, lib/passkey-capability, hooks/use-passkey-capability_

- [ ] 7. Web: `web/src/hooks/use-passkey-authentication.ts` を追加し、ログイン用の mutation chain（begin → get → finish → session）を提供する
  - `usePasskeyAuthentication()` を `useMutation<void, PasskeyAuthError, void>` で実装。
    内部 chain は design.md §Flows「ログインフロー」の全 5 段:
    1. `generatePkcePair()`
    2. `apiClient.post<PasskeyBeginResponse>("/api/passkey/authentication/begin", {code_challenge})`
    3. `decodeRequestOptions(options)` → `navigator.credentials.get({publicKey})`
    4. `encodeAssertionResponse(cred)` → `apiClient.post<AuthenticationFinishResponse>("/api/passkey/authentication/finish", {challenge_id, credential})`
    5. `apiClient.post("/api/auth/session", {auth_code, code_verifier})`
  - 成功時に `queryClient.invalidateQueries({queryKey: ["auth","me"]})` を呼び AuthGuard を
    再判定させる（Requirement 4.2 / 4.3）
  - error 分類（`PasskeyAuthError.kind`）:
    - `DOMException.name` が `NotAllowedError` / `AbortError` / `InvalidStateError` → `cancelled`
    - fetch failure（`TypeError`）→ `network_error`
    - `ApiError.status === 400/409` → `server_rejected`
    - `ApiError.status === 500` → `server_error`
    - `POST /api/auth/session` の 400/500 → `session_exchange_failed`
    分類判定は step index を追跡して行う（step 5 の失敗のみ `session_exchange_failed` にする）
  - `code_verifier` は mutation function の closure 変数のみに保持、`code_verifier` /
    `auth_code` / assertion 生値を `console.*` / storage / URL に書かない（NFR 1.1）
  - `web/src/hooks/use-passkey-authentication.test.tsx` を追加し、5 ケースを検証:
    1. 正常系（各 API を順序どおり呼び、invalidateQueries が呼ばれる）
    2. `cancelled`（`navigator.credentials.get` が NotAllowedError を throw）
    3. `server_rejected`（authentication/finish が 400 AUTHENTICATION_FAILED を返す）
    4. `session_exchange_failed`（`/api/auth/session` が 400 INVALID_GRANT を返す）
    5. `server_error`（authentication/begin が 500 を返す）
    `navigator.credentials` は `vi.stubGlobal` で mock、fetch は `apiClient` を `vi.mock` で mock
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, NFR 1.1, NFR 1.2, NFR 1.4_
  - _Boundary: hooks/use-passkey-authentication_
  - _Depends: 4, 5_

- [ ] 8. Web: `web/src/hooks/use-passkey-registration.ts` を追加し、新規作成 mutation（登録 → 認証 → session の連鎖）を提供する
  - `usePasskeyRegistration()` を `useMutation<void, PasskeyRegistrationError, {username: string}>`
    で実装。内部 chain は design.md §Flows「新規作成フロー」に準拠:
    1. `generatePkcePair()`
    2. `apiClient.post<PasskeyBeginResponse>("/api/passkey/registration/begin", {username, email:"", code_challenge})`
    3. `decodeCreationOptions(options)` → `navigator.credentials.create({publicKey})`
    4. `encodeAttestationResponse(cred)` → `apiClient.post<RegistrationFinishResponse>("/api/passkey/registration/finish", {challenge_id, credential})`
    5. **直後に認証 chain**: `apiClient.post("/api/passkey/authentication/begin", {code_challenge})`
       → `navigator.credentials.get` → `apiClient.post("/api/passkey/authentication/finish", ...)`
    6. `apiClient.post("/api/auth/session", {auth_code, code_verifier})`
    7. `queryClient.invalidateQueries({queryKey: ["auth","me"]})`
  - email は常に `""` を送信（Requirement 2.4「recovery email 未指定を欠落として扱わない」）
  - error 分類（`PasskeyRegistrationError.kind`）:
    - begin の 400 で `error.body.code === "INVALID_USERNAME"` → `invalid_username`
    - begin の 409 → `username_taken`
    - `navigator.credentials.create` の DOMException（NotAllowedError / AbortError /
      InvalidStateError）→ `cancelled`
    - finish の 400 REGISTRATION_FAILED → `server_rejected`
    - authentication chain 内のいずれかで失敗（400 REGISTRATION_FAILED /
      AUTHENTICATION_FAILED を区別しない）→ `server_rejected`（NFR 1.2）
    - `/api/auth/session` の 400/500 → `session_exchange_failed`（Requirement 3.4）
    - fetch reject → `network_error`
    - その他 500 → `server_error`
    step index を追跡し `session_exchange_failed` を step 6 の失敗のみに限定する
  - `code_verifier` / `auth_code` / attestation 生値を `console.*` / storage / URL に
    書かない（NFR 1.1）
  - `web/src/hooks/use-passkey-registration.test.tsx` を追加し、6 ケースを検証:
    1. 正常系（登録 → 認証 → session の全 API を順序どおり呼び invalidateQueries が呼ばれる）
    2. `invalid_username`（begin が `code: "INVALID_USERNAME"` の 400）
    3. `username_taken`（begin が 409 USERNAME_TAKEN）
    4. `cancelled`（`create` が NotAllowedError）
    5. `server_rejected`（finish が 400 REGISTRATION_FAILED）
    6. `session_exchange_failed`（`/api/auth/session` が 400 INVALID_GRANT）
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 3.1, 3.2, 3.4, NFR 1.1, NFR 1.2, NFR 1.4_
  - _Boundary: hooks/use-passkey-registration_
  - _Depends: 4, 5_

- [ ] 9. Web: `web/src/components/passkey-signup-dialog.tsx` を追加し、username 入力 UI とエラー表示を提供する
  - shadcn/ui の `Dialog` / `DialogContent` / `DialogHeader` / `DialogTitle` /
    `DialogDescription` / `Input` / `Label` / `Button` を既存 `components/ui/*` から再利用
    （CLAUDE.md §4「共有 UI の再利用」）
  - `PasskeySignupDialogProps { open, onOpenChange }` で親から open state を受け取る
  - 入力欄は username のみ（recovery email 入力欄を設けない / Requirement 2.4）
  - 「作成」ボタン押下で `usePasskeyRegistration().mutate({username})` を呼ぶ
  - mutation の各状態表示:
    - `isPending`: 「作成」ボタン disabled + 「作成中...」表示
    - `isError` の `error.kind` 分岐:
      - `invalid_username`: 「ユーザー名の形式が不正です（3〜32 文字、英数字とハイフン・アンダースコアのみ）」
      - `username_taken`: 「このユーザー名は既に使用されています」
      - `cancelled`: エラー表示なし、`mutation.reset()` で state を初期化（Requirement 2.7）
      - `session_exchange_failed`: 「ログインに問題が発生しました。ログイン画面から再度お試しください」
        + `onOpenChange(false)` で Dialog を閉じる（Requirement 3.4）
      - `server_rejected` / `server_error` / `network_error`: 「エラーが発生しました。時間をおいて再度お試しください」
        （Requirement 2.8）
    - `isSuccess`: `onOpenChange(false)` で Dialog を閉じる（`AuthGuard` が
      `queryClient.invalidateQueries` で再判定するため router 遷移は不要）
  - `web/src/components/passkey-signup-dialog.test.tsx` を追加し、6 ケース検証:
    1. Dialog open で username 入力欄と「作成」ボタンが表示される（recovery email 入力欄がない）
    2. 「作成」クリックで `mutate({username: <入力値>})` が呼ばれる
    3. `error.kind === "invalid_username"` で対応文言が表示される
    4. `error.kind === "username_taken"` で対応文言が表示される
    5. `error.kind === "cancelled"` で汎用エラーが **表示されない**（reset される）
    6. `isSuccess` 時に `onOpenChange(false)` が呼ばれる
    `usePasskeyRegistration` は `vi.mock` でモック
  - _Requirements: 2.1, 2.4, 2.5, 2.6, 2.7, 2.8, 3.1, 3.4_
  - _Boundary: components/passkey-signup-dialog_
  - _Depends: 8_

- [ ] 10. Web: `web/src/components/passkey-buttons.tsx` を追加し、`web/src/components/login-page.tsx` にパスキー導線と Signup Dialog を統合する（既存 Google 導線・既存テストを完全不変）
  - `web/src/components/passkey-buttons.tsx` を新規追加:
    - `usePasskeyCapability()` で capability を取得。`isLoading` の間は placeholder（`null` 返却でも可）、
      `available === false` のとき `null` を返し非表示（Requirement 5.1 / 5.2）
    - 「パスキーでログイン」ボタン: shadcn/ui `Button variant="outline"`、クリックで
      `usePasskeyAuthentication().mutate()`
    - 「アカウント新規作成」ボタン: shadcn/ui `Button variant="outline"`、クリックで
      `props.onSignupClick()`（親が Dialog を管理）
    - authentication mutation の状態:
      - `isPending`: 両ボタン disabled + 「認証中...」表示
      - `isError` で `error.kind === "cancelled"`: エラー表示なし、`mutation.reset()`
        （Requirement 4.5）
      - `isError` でその他: 「認証に失敗しました。時間をおいて再度お試しください」
        （Requirement 4.6 / 4.7）
    - 内部で mutation 経路のみを使用し、Google 導線とは完全に独立（Google 側 `<a>` の DOM は
      本コンポーネントの外側）
  - `web/src/components/login-page.tsx` を変更:
    - 既存 Google `<a href="/auth/google/login">Googleアカウントでログイン</a>` の DOM /
      class / `href` / 文言をそのまま維持（Requirement 6.1 / 6.2 / 6.3）
    - `useState<boolean>` で Signup Dialog の open 管理し、Google ボタン直下に
      `<PasskeyButtons onSignupClick={() => setSignupOpen(true)} />` と
      `<PasskeySignupDialog open={signupOpen} onOpenChange={setSignupOpen} />` を配置
    - 既存の `LoginPage` export シグネチャは変更しない（`AuthGuard` からの呼び出しを不変に保つ）
  - `web/src/components/passkey-buttons.test.tsx` を新規追加し、6 ケース検証:
    1. `available: true` で「パスキーでログイン」「アカウント新規作成」の 2 ボタン表示
    2. `available: false` で null 返却（DOM に何も出ない）
    3. `isLoading: true` の間は null 返却（初期表示ちらつき防止）
    4. 「パスキーでログイン」クリックで `usePasskeyAuthentication.mutate` が呼ばれる
    5. 「アカウント新規作成」クリックで `onSignupClick` callback が呼ばれる
    6. mutation の `error.kind === "cancelled"` ではエラー文言が **表示されない**
    `usePasskeyCapability` / `usePasskeyAuthentication` は `vi.mock` でモック
  - `web/src/components/login-page.test.tsx` を変更:
    - 既存 4 テスト（Google ボタン表示 / href / アプリ名 / 説明文）は完全維持（Requirement 6.1 / 6.4）
    - 新規: capability true で PasskeyButtons が render される
    - 新規: capability false（`usePasskeyCapability` mock で `available: false`）で
      Google 導線のみ表示され、パスキー導線が DOM に存在しない（Requirement 5.3 / 6.1）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 3.3, 4.5, 4.6, 4.7, 5.1, 5.2, 5.3, 5.4, 6.1, 6.2, 6.3, 6.4, NFR 1.3, NFR 2.2, NFR 3.1_
  - _Boundary: components/passkey-buttons, components/login-page_
  - _Depends: 6, 7, 9_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が Web / サーバの両方を独立再実行して
build/test/lint を検証する。Web は Vitest + ESLint + Next.js build、サーバは `go test` +
`go vet` を通す。

<!-- stage-a-verify -->
```sh
cd web && npm test && npm run lint && npm run build && cd .. && go test ./... && go vet ./...
```
