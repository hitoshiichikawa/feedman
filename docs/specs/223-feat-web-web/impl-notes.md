# 実装ノート（#223）

Web にパスキー導線（新規作成 / ログイン）を追加する spec の実装ノートを task 単位で
記録する。ログインは auth_code 交換、新規登録は #231 Delta 1 の直接 Cookie session
方式で既存の 2 ペイン UI 認証状態に到達する。前方伝播（先行 task の learning を後続
task で温存）を規律とする。

## Implementation Notes

### Task 1

- **採用方針**: `SessionExchangeService` は `TokenService.ExchangeAuthCode` の前半 3 段
  （`HashNativeSecret` → `AuthCodeConsumer.FindByHash` → `VerifyPKCES256Verifier` →
  `AuthCodeConsumer.MarkUsed`）を再利用し、以降のみ session 発行（`generateSessionID`
  → `SessionCreator.Create`）に差し替える。
- **重要な判断**:
  - `AuthCodeConsumer` は既存 `internal/auth/token_service.go` の interface をそのまま
    流用（新規 interface を作らない / CLAUDE.md §4 コピペ禁止）。`SessionCreator` のみ
    新規に `Create` 1 メソッドの狭い interface として追加した（interface segregation /
    CLAUDE.md §5）。`repository.SessionRepository` が構造的に充足する。
  - session ID 生成は `internal/auth/service.go` の既存 `generateSessionID`（32 バイト
    crypto random / hex）を直接呼び、Google OAuth callback と同じ 32 バイト crypto random
    /hex 方針を維持する。ロジックの複製は行わない。
  - now は `func() time.Time` フィールドで抽象化し `NewSessionExchangeService` で
    `time.Now` を注入。テストは同 package から `svc.now` を上書きして時刻を固定する
    （既存 `TokenService` と同じ idiom）。
  - 拒否は `ErrInvalidGrant` に uniform 化、infra error は `fmt.Errorf("session
    exchange: ...: %w", err)` で wrap して handler が 500 に振り分けられる形にする。
    平文 auth_code / code_verifier / session_id はログ・エラーに出さず、追跡ログは
    `code_hash[:8]` / `session_id_hash[:8]` のみ出力する（既存 `TokenService.ExchangeAuthCode`
    と同方針 / NFR 1.1）。
- **残存課題**: task 2 で `NewSessionExchangeService(authCodeRepo, sessionRepo,
  sessionTTL)` を `internal/app/app.go` で wiring し、`NativeAuthHandler` に注入する
  必要がある。`sessionTTL` は既存 `SessionMaxAge`（秒 int）を `time.Duration` 化して
  渡す（`time.Duration(cfg.SessionMaxAge) * time.Second`）。handler 層で
  `errors.Is(err, auth.ErrInvalidGrant)` により 400 INVALID_GRANT へ、その他 error は
  500 INTERNAL_ERROR へ振り分ける（design.md §Error Handling / task 2 詳細）。

### Task 2

- **採用方針**: `NativeAuthHandler.Session` を既存 `Token` / `Refresh` / `Revoke` と
  同 idiom（`dec.DisallowUnknownFields()` + `invalidRequestError` /
  `invalidGrantError` / `WriteInternalServerError` 再利用）で実装し、成功時のみ
  既存 `sessionCookieName` 定数と OAuth Callback と同一属性で Set-Cookie する。
- **重要な判断**:
  - **Cookie 属性の特定と再利用**: 既存 Google OAuth callback の Cookie 発行箇所は
    `grep 'http.SetCookie'` および `sessionCookieName` の検索で `internal/handler/auth_handler.go`
    の `AuthHandler.Callback` 手順 5（line 193-202）と特定。既存定数
    `sessionCookieName` = `"session_id"`、`http.SameSiteLaxMode` をそのまま参照し、
    Domain / Secure / MaxAge は `AuthHandler` と同じ `cfg.CookieDomain` /
    `cfg.CookieSecure` / `cfg.SessionMaxAge` を `WithSessionExchange` 経由で注入する。
    Path=`"/"` / HttpOnly=`true` は既存 Cookie と同じくハードコード。実装差分は
    `TestNativeAuthHandler_Session_Success` で Domain / MaxAge / HttpOnly / Secure /
    SameSite の全 5 属性を assert して回帰防止済み。
  - **handler が service を受ける interface**: design.md では `sessionExchange
    *auth.SessionExchangeService`（具体型）を想定していたが、既存 `TokenExchangeService`
    と同 idiom で `SessionExchanger` narrow interface（`ExchangeAuthCodeForSession` 1
    メソッドのみ）を handler package に追加し、`*auth.SessionExchangeService` を構造的に
    充足させた（interface segregation / CLAUDE.md §5 / testability 優先）。design.md の
    「Cookie 属性の完全一致」「fail-closed の連動」「NFR 2.1 の既存挙動不変」の 3 契約は
    interface 化によって毀損されない（handler の依存が抽象化されるだけで挙動は不変）。
  - **既存 constructor の後方互換**: `NewNativeAuthHandler` は functional option
    （`NativeAuthHandlerOption` + `WithSessionExchange`）で拡張。既存呼び出し
    `NewNativeAuthHandler(svc)` は variadic のため無変更で compile 通過（NFR 2.1）。
    既存 `native_auth_handler_test.go` / `router_test.go` / `integration_test.go` /
    `native_auth_e2e_db_test.go` / `passkey_e2e_db_test.go` の 15 callsites を編集
    せずに済み、既存 Token / Refresh / Revoke tests が完全不変を維持することを
    `go test ./...` all-green で確認済み。この functional option 採用は既存
    `item.WithMetrics` / `fetchpkg.WithMetrics` と同 idiom で codebase 一貫性も維持。
  - **fail-closed の連動**: 本 route の登録は `deps.NativeAuthHandler != nil` gate に
    含めることで、既存 3 route（token/refresh/revoke）と連動して
    `NATIVE_AUTH_JWT_SECRET` 未設定時に 404 で縮退する（NFR 2.2）。
    `TestNewRouter_NativeAuthSession_NotRegisteredWhenHandlerNil` で確認。
- **残存課題**: なし。task 3（`GET /api/passkey/capability` handler）は Boundary
  `PasskeyHandler, Router` で独立に着手可能。

### Task 3

- **採用方針**: `PasskeyHandler.Capability` は「到達 = 有効」の設計原則に忠実な最小
  実装（env 参照・分岐なし・固定 `{"available": true}` 応答）とし、fail-closed は
  router 側の `deps.PasskeyHandler != nil` gate に任せる（CLAUDE.md §1 レイヤリング）。
- **重要な判断**:
  - **DTO 新設**: 既存 `passkeyBeginResponse` / `authenticationFinishResponse` と同じ
    idiom で `capabilityResponse struct { Available bool \`json:"available"\` }` を
    passkey_handler.go 内に追加。inline map[string]any より型安全で、既存応答 DTO 群と
    表記が揃う。
  - **Cache-Control: no-store の付与**: 既存 `writeJSON` は Content-Type のみ set する
    ため、handler 側で `w.Header().Set("Cache-Control", "no-store")` を writeJSON
    呼び出し**前**に set する（WriteHeader 後は Header 変更が反映されないため順序が
    重要 / design.md §Capability §Cache）。writeJSON 自体は他 endpoint と共有のため
    改変せず、Cache-Control 付与のみ handler 側で行う最小差分方針。
  - **既存 passkey `if deps.PasskeyHandler != nil` ブロックへの相乗り**: router.go
    L255-264 の既存 passkey 未認証 4 route の同一 gate ブロック末尾に 1 行追記する
    形とし、fail-closed の連動（PasskeyHandler nil = passkey 系 5 route すべて未登録 =
    Web は Google 単体構成へ縮退）を自然に実現。新たな `if` を切らず、既存 4 route の
    順序・middleware も不変（NFR 2.1）。
  - **GET なので MaxBodyBytes 不使用**: capability は body を持たない GET のため
    `unauthIPMW` のみ通す（tasks.md L66-67 の指定どおり）。他 4 route の POST は
    `unauthIPMW + MaxBodyBytes` の 2 段。
  - **テスト構造の再利用**: router_test.go では既存
    `TestNewRouter_NativeAuthSession_RegisteredWhenHandlerInjected` /
    `_NotRegisteredWhenHandlerNil`（task 2）を参照 idiom とし、`newPasskeyRouterDeps`
    ヘルパを再利用。router_unauth_ratelimit_test.go では既存
    `TestNewRouter_NativeAuthSessionIPRateLimit_429OnExcess`（task 2）を参照 idiom
    とし、`newPasskeyRateLimitRouter` ヘルパを再利用。既存 helper・stub の再利用で
    重複組み立てを増やさない（CLAUDE.md §4 コピペ禁止）。GET 用の小さな
    `doGetRateLimit` ヘルパは同ファイルの既存 `doPostRateLimit` と対称に追加。
- **残存課題**: なし（本 spec の Boundary `PasskeyHandler, Router` で完結）。task 4 以降
  は Web 側（`web/src/lib/pkce.ts` 他）で backend 側の残存課題は特に無い。app.go の
  wiring（Issue #216 で完了済み）は `PasskeyHandler` を既に注入しており、本 task で
  追加した Capability route も注入済み環境で自動的に有効化される。

### Task 4

- **採用方針**: `web/src/lib/pkce.ts` を純粋 utility として実装し、`generatePkcePair()`
  と `deriveCodeChallengeS256(codeVerifier)` の 2 関数を export。ヘルパ `bytesToBase64url`
  はローカル private とし、base64url 共有化は task 5（`web/src/lib/webauthn.ts`）の責務に
  委ねる（Boundary `lib/pkce` を逸脱しないため）。
- **重要な判断**:
  - **SHA-256 の入力はサーバと対称化**: サーバ側 `internal/auth/pkce.go` の
    `VerifyPKCES256Verifier` は `sha256.Sum256([]byte(verifier))` を計算する。ここで
    `verifier` は code_verifier "文字列" そのもの（43 文字 ASCII）。したがって Web 側でも
    生の 32 バイト crypto random を直接 SHA-256 に食わせず、まず base64url 化して
    code_verifier 文字列（43 文字）を作り、その文字列を `TextEncoder` でエンコードした
    バイト列を SHA-256 に入力する。これにより Web が生成した challenge がサーバの
    verifier 検証と一致し、パスキー登録・ログインの合流経路が成立する。RFC 7636
    Appendix B の既知ベクトル（`dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk` →
    `E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM`）でこの互換性を回帰テスト化した。
  - **`deriveCodeChallengeS256` の追加 export**: design.md §pkce.ts の Contracts では
    `generatePkcePair` + `PkcePair` のみが例示されているが、乱数入力で動く
    `generatePkcePair` だけでは既知ベクトルによる SHA-256 派生の正しさを検証できない
    ため、S256 派生を担う純粋関数を同 file から `deriveCodeChallengeS256` として追加
    export した。Boundary `lib/pkce` 内で完結する testability 向上のみで、design.md
    契約（`generatePkcePair` の signature / `PkcePair` の shape）は毀損していない。
  - **base64url 変換のローカル private 化**: task 5 でも同種の変換が必要になるが、
    本 task の Boundary は `lib/pkce` に限定されており、`web/src/lib/webauthn.ts` を今
    作成すると task 5 の責務を先取りして boundary 逸脱になる。したがって `bytesToBase64url`
    は pkce.ts 内の private helper として保持し、task 5 実装時に共有化を検討する運用と
    した（CLAUDE.md §4 の共有ヘルパ抽出は task 5 の責務）。
  - **NFR 1.1 遵守**: 生成値は返却値としてのみ渡し、モジュール内変数に保持しない。
    `console.*` 呼び出しなし、storage / URL への書き込みなし。WebCrypto rejection は
    握り潰さず呼び出し側に伝播（副作用なし・throw なし方針）。
- **残存課題**: task 5（`web/src/lib/webauthn.ts`）で base64url ↔ ArrayBuffer 変換の
  共有化を検討する（同 file 内で `Uint8Array → base64url` を再実装するか、`pkce.ts` の
  private helper を export に格上げして共有するかは task 5 実装時に判断）。人間 Reviewer
  による design.md との整合性確認（`deriveCodeChallengeS256` の追加 export）を推奨。

### Task 5

- **採用方針**: `web/src/lib/webauthn.ts` を純粋 utility として実装し、
  `base64urlToArrayBuffer` / `arrayBufferToBase64url` / `decodeCreationOptions` /
  `decodeRequestOptions` / `encodeAttestationResponse` / `encodeAssertionResponse`
  の 6 関数を export。副作用なし・throw なし（形式不正はドメイン境界で `TypeError`
  を throw）。base64url ↔ Uint8Array の変換ロジックは `pkce.ts` の private helper
  `bytesToBase64url` と類似するが、共有化は行わず本 file 内に閉じ込める（Boundary
  `lib/webauthn` に閉じるため）。
- **重要な判断**:
  - **base64url 共有化を見送り本 file 内実装とした根拠**: task 5 の Boundary は
    `lib/webauthn` に限定されており、`web/src/lib/pkce.ts`（Boundary 外）を編集して
    private helper `bytesToBase64url` を export に格上げする変更は boundary 逸脱に
    なる。したがって同種変換を本 file 内に独立実装した（CLAUDE.md §4 の共有ヘルパ
    抽出との緊張関係あり）。両モジュールは実装方針（`btoa` + `String.fromCharCode`
    のバイナリ文字列組み立て + base64→base64url 差分置換）を同一にし、後日 spec 別
    共有化 PR（`web/src/lib/base64url.ts` 等の切り出し）を行えるよう関数シグネチャを
    互換に保った（`arrayBufferToBase64url` は `ArrayBuffer | Uint8Array` を受ける
    superset で、`bytesToBase64url(bytes: Uint8Array)` の呼び出しをそのまま置換
    可能）。
  - **encode 関数の JSON shape をサーバ実装で確認した結果**: サーバ側
    `internal/passkey/webauthn_adapter.go` の `ParseCredentialCreationResponseBytes`
    / `ParseCredentialRequestResponseBytes` は go-webauthn `protocol` package
    （v0.17.4）に委譲され、内部 `CredentialCreationResponse.Parse()` /
    `CredentialAssertionResponse.Parse()` が **`ccr.ID == ""` と
    `ccr.Type != "public-key"` を明示的に reject** する（`protocol/credential.go`
    L129-144）。design.md §Contracts の関数ヘッダは `rawId` / `clientDataJSON` /
    `attestationObject` / `authenticatorData` / `signature` / `userHandle` のみを
    列挙していたが、実サーバ実装は **top-level `id` (base64url 文字列) と
    `type: "public-key"`** も必須である。したがって encode 関数は WebAuthn IDL の
    `PublicKeyCredentialJSON` に準じ、`{ id, type, rawId, response: {...} }` の
    4 top-level フィールドを出力する形とした（`id` は browser の
    `PublicKeyCredential.id` が既に base64url 文字列なのでそのまま採用、`type` は
    常に "public-key"）。assertion 側の `userHandle` は go-webauthn の
    `URLEncodedBase64` + `omitempty` 相当（`UnmarshalJSON` が `null` を許容）
    のため、`null` / `undefined` のとき出力から省略する。この乖離は design.md
    契約を毀損しない（新規追加フィールドで既存契約と整合的）。
  - **NFR 1.1 遵守**: 本 file 内で `console.*` を一切呼ばない（純粋関数として
    副作用を持たせない）。サーバから受け取った base64url 生値・credential 生バイトを
    ログ・storage・URL に書かない。テストは jsdom 環境の標準 `atob` / `btoa` /
    `ArrayBuffer` / `Uint8Array` のみを使用（追加依存なし）。
  - **`_test.ts` での `PublicKeyCredential` 構築**: jsdom は `PublicKeyCredential`
    の runtime 実装を持たないため、encode 関数が参照するフィールドだけを持つ
    duck-typed オブジェクトを組み立てて `as unknown as PublicKeyCredential` で
    cast する。これは encode 関数の入力契約が「4 フィールド (`id` / `type` /
    `rawId` / `response.*`) を持つ」ことのみに依存する純粋関数だから許容できる
    （フルモックは不要 / NFR 3.1）。
- **残存課題**:
  - **base64url helper の共有化提案**: `pkce.ts` の private `bytesToBase64url` と
    `webauthn.ts` の `arrayBufferToBase64url` が同一ロジックを持つため、
    `web/src/lib/base64url.ts` を新設して両 file から import する統合 PR を別 spec
    として起票することを推奨（本 task では Boundary 制約下で見送り。関数シグネチャは
    互換にしてある）。
  - **人間 Reviewer への確認事項**: design.md §Contracts の encode 関数ヘッダ
    （`rawId` / `clientDataJSON` / `attestationObject` / …）に対し、実装では
    サーバ go-webauthn の必須検証（`id` / `type` の存在）を満たすため
    top-level `id` / `type` を追加出力している。設計意図（"透過的にコピー" の暗黙前提）
    と整合するが、design.md 本文には明記が無かった点は下記「確認事項」節にも別途記載する。
  - task 6 以降（`use-passkey-*` hooks）で本 file の encode/decode 関数を呼ぶ際、
    `code_verifier` / `auth_code` / assertion 生バイトが hook 側の closure 変数の
    みに保持されて storage / console に漏れないこと（NFR 1.1）を hook 側テストで
    別途担保する必要がある（本 file はあくまで純粋変換のみで、保持責務は持たない）。

### Task 6

- **採用方針**: 3 モジュールを同一 task で追加し、責務を明確に分離する。
  `web/src/types/passkey.ts` は 8 型を snake_case で定義（サーバ DTO と対称）、
  `web/src/lib/passkey-capability.ts` は `isPasskeyBrowserSupported()` を純粋
  関数として export（副作用なし・throw なし・boolean のみ）、
  `web/src/hooks/use-passkey-capability.ts` は TanStack Query の `useQuery` で
  server capability と browser support を合成した `PasskeyCapability` を返す。
- **重要な判断**:
  - **queryFn は 404 / reject を catch して server=false に集約**: capability
    endpoint は fail-closed で「未登録 = 404 = サーバがパスキー機能を未提供」を
    意味する（task 3 で確定）。この 404 を React Query の `isError` に落とすと
    「エラー状態」表現が Google 単体構成の正常な縮退経路（Requirement 5.2 / 5.3）
    と一致しないため、queryFn 内で `ApiError`（4xx/5xx）と `TypeError`（fetch 失敗
    相当）の両方を catch し `server=false` を返して query 自体を success として
    扱う設計にした。予期しない例外も安全側に false へ集約（NFR 2.1 縮退時挙動の
    維持を優先）。結果として消費側の `isError` 分岐は不要になり、`isLoading` /
    `available` の 2 フィールドだけで導線表示ロジックが書ける。
  - **queryKey は `["passkey", "capability"]` の 2 要素配列**: 既存 `use-auth.ts`
    の `["auth", "me"]` と同じ domain-scoped 命名慣習を踏襲。将来 passkey 系の
    他 query（credential list 等）を追加する際も namespace が衝突しない。
  - **`retry: false` / `staleTime: Infinity` / `gcTime: Infinity`**: capability は
    env 由来の不変値（tab セッションを跨いで安定）。retry しても 404 → 404 で
    無意味、staleTime/gcTime を無限にして同一 tab 内では再問い合わせを起こさない
    設計（design.md §use-passkey-capability.ts と一致）。
  - **`browserSupported = isPasskeyBrowserSupported()` は render ごとに呼ぶ**: 実
    ブラウザ環境では戻り値が unchanging だが、テスト（module mock で戻り値を
    切り替え）や SSR / hydration 境界で環境が変化するケースを含めて素直に render
    時に評価する。純粋関数のため副作用なし・パフォーマンス懸念なし。
  - **テスト方針は「実物 integration」+「戦略的 mock」の使い分け**:
    - `passkey-capability.test.ts` は `vi.stubGlobal("PublicKeyCredential", ...)` /
      `vi.stubGlobal("window", undefined)` でブラウザ環境を制御し実物関数を呼ぶ
      （4 ケース: function 存在 / undefined / object / SSR）。`afterEach` で
      `vi.unstubAllGlobals()` により副作用を漏らさない。
    - `use-passkey-capability.test.tsx` は 2 軸（サーバ応答 × ブラウザ対応）を
      独立制御したいため `@/lib/passkey-capability`（`isPasskeyBrowserSupported`）
      と `@/lib/api`（`apiClient.get`）の両モジュールを `vi.mock` で差し替える。
      `ApiError` は実物を `vi.importActual` で再 export し、404 相当の catch 経路
      をリアルな型で通す。既存 `use-feeds.test.tsx` / `use-auth.test.tsx` の
      `createWrapper()` idiom（`QueryClient` with `retry: false`）を踏襲。
  - **NFR 1.4 遵守**: `apiClient.get("/api/passkey/capability")` は相対パスで
    同一オリジンに閉じる（`API_BASE_URL=""` の `web/src/lib/api.ts` を経由）。
    絶対 URL は書かない。
  - **NFR 1.1 系（機密情報の非漏出）**: capability 応答は `{available: boolean}`
    のみで機密情報を含まない。本モジュール群は `console.*` を一切呼ばず、
    localStorage / sessionStorage / URL への書き込みも無い。
- **残存課題**:
  - task 10（`PasskeyButtons` / `LoginPage` 統合）で `usePasskeyCapability()` の
    `isLoading` 中 / `available === false` の表示戦略（`null` 返却で非表示）を
    実装する。タイミング的には「初回 mount で isLoading=true → 200/404 解決後に
    available が確定」の 1 tick 遷移で、`PasskeyButtons` は `isLoading` 中も
    `null` を返してちらつきを防ぐ想定（tasks.md L263 の指定）。
  - task 7 / 8（`use-passkey-authentication` / `use-passkey-registration`）は
    本 hook を直接呼び出さないが、`PasskeyButtons` 経由で gating される前提
    （design.md §Preconditions: `usePasskeyCapability().available === true`）。

### Task 7

- **採用方針**: `usePasskeyAuthentication` を `useMutation<void, PasskeyAuthError, void>`
  として実装し、design.md §Flows「ログインフロー」の 5 段 chain（PKCE →
  begin → navigator.credentials.get → finish → session）を単一の `mutationFn`
  内で closure 変数（`codeVerifier`）を保持したまま実行する。step index を数値
  カウンタで追跡し、catch では `classifyError(err, step)` に集約して `PasskeyAuthError`
  へ変換する（step 5 の失敗のみ `session_exchange_failed` に振り分けるロジックを
  一箇所に閉じ込めるため）。
- **重要な判断**:
  - **`PasskeyAuthErrorKind` に `session_exchange_failed` を追加**: design.md
    §Components（use-passkey-authentication.ts）の Contracts では 4 種のみ
    （`cancelled` / `server_error` / `server_rejected` / `network_error`）が
    例示されていたが、tasks.md L155（`POST /api/auth/session` の 400/500 →
    `session_exchange_failed`）および design.md §Error Handling L1042 の
    「System Errors」で明示的に列挙された `session_exchange_failed` を含めた
    5 種として実装した。design.md §Components と §Error Handling の間に軽微な
    非整合があるが、tasks.md（実装レベルの正本）と §Error Handling が同期
    しているため後者を採用（下記「確認事項」に記載）。
  - **`PasskeyAuthError` はカスタム Error クラス**: CLAUDE.md §「エラーは
    独自 Error クラスで wrap」に従い、`kind` プロパティを持つ具象 class として
    定義。`readonly kind` として immutable にし、UI 側は enum 判別のみで文言・
    復帰動作を決定できる。`ApiError.body` の内部詳細は message に反射しない
    （NFR 1.2）ため、`server_rejected` テストで `error.message` に
    "AUTHENTICATION_FAILED" が含まれないことを assert して回帰防止。
  - **step index による分類の一元化**: 「同じ 400 ApiError でも step 4
    （finish）なら server_rejected、step 5（session）なら session_exchange_failed」
    という要件を満たすため、catch を分割せず 1 箇所の `try` 内で step 番号を
    incremental に更新し、単一 catch で `classifyError(err, step)` を呼ぶ形に
    集約した。分岐が catch 側の 1 関数に閉じ、chain 本体の可読性を保てる。
    テストの `session_exchange_failed` ケースで「同じ 400 が step 4 では
    `server_rejected` に、step 5 では `session_exchange_failed` に振り分けられる」
    ことを別ケースの対比で確認済み。
  - **`navigator.credentials.get` null 返却時の cancel 集約**: 一部のブラウザは
    キャンセル時に throw ではなく `null` を resolve するケースがある（WebAuthn 仕様上、
    実装依存の余地あり）ため、`if (!cred) throw new PasskeyAuthError("cancelled")`
    で明示的に cancel カテゴリに集約する（Req 4.5「画面を壊さずに戻す」の網羅性向上）。
  - **DOMException 判別のフォールバック**: jsdom / 本番ブラウザ双方で DOMException が
    存在する前提だが、SSR / 古いランタイムで `typeof DOMException === "undefined"` の
    ケースを想定し、`err instanceof DOMException` に加えて `err instanceof Error &&
    CANCEL_ERROR_NAMES.has(err.name)` を fallback として持たせた（NFR 3.1 の
    「外部ネットワーク依存なしに検証可能」を jsdom で安定させる副次効果）。
  - **依存モジュールの mock 戦略**: `apiClient` / `generatePkcePair` / `webauthn`
    encode/decode の 3 モジュールを `vi.mock` で差し替え、`navigator.credentials`
    のみ `vi.stubGlobal("navigator", { credentials: { get: mockGet } })` で
    差し込む。jsdom は WebAuthn 実装を持たないため実物を呼ぶ意義が薄く、mock 化が
    NFR 3.1 の「外部ネットワーク・ブラウザ依存なしに検証可能」を満たす最短経路。
    `ApiError` は `vi.importActual` で実物を再 export し、hook 側の `instanceof
    ApiError` 判定がリアルな型で通ることを保証。
  - **`afterEach` での `vi.unstubAllGlobals()`**: 各テストで `vi.stubGlobal` した
    navigator を次テストへ漏らさないため、`use-passkey-capability.test.tsx` /
    `passkey-capability.test.ts` と同 idiom で teardown する。
  - **`invalidateQueries` の spy**: `createWrapper()` から `queryClient` を
    返却し、`vi.spyOn(queryClient, "invalidateQueries")` で `["auth", "me"]` の
    invalidate を assert する（既存 `use-manual-refresh.test.tsx` と同 idiom）。
  - **NFR 1.1 遵守**: hook 実装では `console.*` を一切呼ばず、`code_verifier` /
    `auth_code` / assertion 生値をモジュール変数・storage・URL に一切残さない。
    mutation 終了で closure が GC 対象になる前提で、明示的な変数 clear は不要
    （React Query が mutation state を管理）。
  - **NFR 1.4 遵守**: `apiClient` 経由の相対パス呼び出しで同一オリジン
    （`API_BASE_URL = ""`）に閉じる。テストの mock 検証で 3 endpoint への
    call がすべて相対パスであることを assert 経由で確認。
- **残存課題**:
  - **api.ts の 204 レスポンス非対応**: `web/src/lib/api.ts` の `request<T>` は
    `response.json()` を無条件で呼ぶため、`/api/auth/session` の 204 No Content
    応答（design.md L859 で規定）に対して runtime で `SyntaxError` を throw する
    可能性がある。Task 7 の Boundary は `hooks/use-passkey-authentication` に
    限定されており api.ts 側を修正できないため、実装は tasks.md L147 の指定
    どおり `apiClient.post` を使い、テストは apiClient を mock して runtime 挙動
    を回避している。production 統合前に api.ts の 204 handling を別 spec / PR
    で追加する必要がある（下記「確認事項」に記載）。
  - task 8（`use-passkey-registration`）は同ファイルの `PasskeyAuthError` /
    `classifyError` パターンを踏襲するのが自然だが、Boundary
    `hooks/use-passkey-registration` を逸脱しないため、共有 utility 抽出は
    task 8 側で判断する（error kind の superset に `invalid_username` /
    `username_taken` が追加される想定）。
  - task 10（`PasskeyButtons` / `LoginPage` 統合）で本 hook の呼び出し側から
    見た挙動（authentication mutation loading / error kind 別 UI 表示 / disable
    処理）を検証する。本 hook 単体では UI 挙動を担わない。

### Task 8

- **採用方針**: `usePasskeyRegistration` を `useMutation<void,
  PasskeyRegistrationError, {username: string}>` として実装し、design.md §Flows
  「新規作成フロー」の 8 段 chain（PKCE → registration/begin → create →
  registration/finish → authentication/begin → get → authentication/finish →
  session 交換）を単一の `mutationFn` 内で closure 変数（`codeVerifier`）を
  保持したまま実行する。task 7（`use-passkey-authentication.ts`）の
  `classifyError(err, step)` + `isCancelledError` + `CANCEL_ERROR_NAMES` +
  step 定数の設計を踏襲し、登録側固有の 2 kind（`invalid_username` /
  `username_taken`）を classifier に加えた superset として実装した。
- **重要な判断**:
  - **task 7 との共有 utility 抽出を見送り本 file 内実装とした根拠**: task 8 の
    Boundary は `hooks/use-passkey-registration` に限定されており、task 7 の
    `use-passkey-authentication.ts` を編集して `classifyError` / `isCancelledError`
    / `CANCEL_ERROR_NAMES` を共有化する変更は boundary 逸脱になる（impl-notes
    Task 7 の残存課題にも同旨の記載あり）。したがって同 idiom を本 file 内に
    独立コピーした（CLAUDE.md §4 の共有ヘルパ抽出との緊張関係あり）。両モジュール
    は実装方針を厳密同一に保ち、後日 spec 別共有化 PR（例:
    `web/src/hooks/passkey-error.ts` や `web/src/lib/webauthn-cancel.ts` の
    切り出し）が行いやすいよう関数シグネチャと定数集合を対称化してある。
    共有化提案は下記「確認事項」節に残す。
  - **`PasskeyRegistrationErrorKind` の 7 種**: tasks.md L182-193 と
    design.md §Error Handling L1036-1045 の列挙に厳密従い、7 種
    （`invalid_username` / `username_taken` / `cancelled` / `server_rejected` /
    `session_exchange_failed` / `server_error` / `network_error`）として
    定義した。design.md §Components（use-passkey-registration.ts）の
    Contracts でも本 7 種が明示されており、task 7 の §Components / §Error
    Handling 間の非整合（4 種 vs 5 種）と異なり task 8 側は文書間で整合が
    取れている。
  - **step index の 8 段設計**: task 7 の 5 段（PKCE / begin / get / finish /
    session）を、task 8 では登録と認証を分離した 8 段（PKCE /
    reg_begin / nav_create / reg_finish / auth_begin / nav_get / auth_finish /
    session_exchange）に拡張した。`classifyError` は `STEP_REG_BEGIN` のみ
    分岐して `invalid_username` / `username_taken` を返し、`STEP_SESSION_EXCHANGE`
    は `session_exchange_failed` に振り分け、それ以外の step の `ApiError` は
    task 7 と同じ `server_rejected` / `server_error` の 2 分類にまとめる。
    「同じ 400 が step 4（reg_finish）では `server_rejected`、step 8
    （session）では `session_exchange_failed`」となる分岐を回帰テスト
    （`server_rejected` ケース + `session_exchange_failed` ケースの並置）で
    検証済み。
  - **`extractApiErrorCode` を専用 helper 化**: `ApiError.body.code === "INVALID_USERNAME"`
    の判定は `body` が任意 shape の `unknown` であるため、`typeof === "object"` /
    non-null / `"code" in body` / `typeof code === "string"` の 4 段 guard を
    通す。判定を classifier 本体から `extractApiErrorCode` に切り出すことで
    `classifyError` の可読性を保ち、code 以外のフィールド（stack / query 等の
    NFR 1.2 反射禁止対象）を参照しない契約を型システムで明示した。
  - **authentication chain の `code_challenge` 再送**: 登録 chain と認証 chain
    は同一の `codeChallenge` を再利用する（step 2 と step 5 で同じ値を送信）。
    サーバ側 `authentication/finish` は登録 finish で発行された `auth_code` を
    `MarkUsed` + `VerifyPKCES256Verifier` する経路であり、`code_challenge` の
    再利用は許容される（サーバ設計 #216 に依存）。code_verifier / code_challenge
    は mutation 内で 1 度だけ生成し、両 chain で共有することで generatePkcePair
    の副呼び出しを避けている。
  - **`navigator.credentials.create` / `.get` の null 集約**: task 7 と同様、
    WebAuthn 実装依存で throw ではなく `null` を resolve するブラウザケースを
    考慮し、`if (!cred) throw new PasskeyRegistrationError("cancelled")` を
    両呼び出し後に配置した（Req 2.7 の網羅性向上）。
  - **テスト戦略の再利用**: task 7 テスト（`use-passkey-authentication.test.tsx`）と
    同 idiom で `@/lib/api` / `@/lib/pkce` / `@/lib/webauthn` の 3 モジュールを
    `vi.mock` で差し替え、`ApiError` は `vi.importActual` で実物を再 export、
    `navigator.credentials` は `vi.stubGlobal` で `create` / `get` を両方 stub
    する。`afterEach(vi.unstubAllGlobals)` で teardown。`createWrapper()` +
    `invalidateQueries` spy も同 idiom。既存 test suite（469 テスト）を破壊
    せず 6 ケース追加で all-green。
  - **NFR 1.1 遵守**: 本 hook 実装では `console.*` を一切呼ばず、`codeVerifier` /
    `authCode` / attestation 生値 / assertion 生値をモジュール変数・storage・
    URL に一切残さない。mutation 終了で closure が GC 対象になる前提で、明示的
    な変数 clear は不要（React Query が mutation state を管理）。
  - **NFR 1.2 遵守**: `PasskeyRegistrationError` は `kind` の enum のみを保持し、
    `ApiError.body` を保持しない。`invalid_username` / `username_taken` /
    `server_rejected` の各テストで error.message に "INVALID_USERNAME" /
    "USERNAME_TAKEN" / "REGISTRATION_FAILED" が反射されないことを assert
    経由で回帰防止。
  - **NFR 1.4 遵守**: `apiClient` 経由の相対パス呼び出しで同一オリジン
    （`API_BASE_URL = ""` / `web/src/lib/api.ts` の既存契約）に閉じる。「正常系」
    テストで 5 endpoint がいずれも相対パスで呼ばれることを `toHaveBeenNthCalledWith`
    の URL 引数として assert。
- **残存課題**:
  - **error 分類の共有 utility 抽出**: task 7 と task 8 で `classifyError` /
    `isCancelledError` / `CANCEL_ERROR_NAMES` / step 定数の同種ロジックが 2 file
    に重複している。Boundary 制約下で本 task では見送り、共有化 PR
    （例: `web/src/hooks/passkey-error.ts` に共通 base classifier を切り出し
    task 7 / 8 の hook からそれぞれの kind 型で継承する形）の別 spec 起票を推奨。
  - **api.ts の 204 No Content 非対応**（task 7 と同一の残存課題）:
    `web/src/lib/api.ts` の `request<T>` は `response.json()` を無条件で呼ぶため、
    `/api/auth/session` の 204 No Content 応答で runtime `SyntaxError` を throw
    する可能性がある。task 8 の Boundary 内では対処できないため、production
    統合前に api.ts の 204 handling を別 spec / PR で追加する必要がある。
    task 8 のテストは apiClient を mock して runtime 挙動を回避しており、
    実装は tasks.md L179 の指定どおり `apiClient.post` を使う。
  - task 9（`PasskeySignupDialog`）で本 hook を `usePasskeyRegistration().mutate({username})`
    経由で呼び出し、`isPending` / `isError` の各 `error.kind` 分岐に対応する
    UI 文言・復帰動作（cancelled は `mutation.reset()`、session_exchange_failed
    は Dialog を閉じる 等）を実装する。本 hook 単体では UI 挙動を担わない。
  - task 10（`PasskeyButtons` / `LoginPage` 統合）は task 9 の Dialog を親から
    open 管理する形になるため、本 hook からは独立に着手可能。

### Task 9

- **採用方針**: `PasskeySignupDialog` を `{open, onOpenChange}` の制御コンポーネント
  として実装（`DialogTrigger` 未使用）。mutation 状態別分岐を `useEffect` 3 本 +
  `ERROR_MESSAGES` マップの宣言的パターンで表現し、副作用（Dialog 開閉 / reset）と
  表示（`role="alert"`）を疎結合にした。入力は username のみで recovery email 欄を
  設けない（Req 2.4 の「欠落として扱わない」を UI 上で確定）。既存
  `feed-register-dialog.tsx` の `role="alert"` idiom を踏襲し、shadcn/ui の
  `Dialog` / `Input` / `Label` / `Button` を再利用（CLAUDE.md §4）。
- **重要な判断**:
  - **`useEffect` 依存配列は primitive フィールドのみ**: `mutation` オブジェクト全体を
    deps に入れず `isSuccess` / `errorKind` / `reset` の primitive・memoized 参照のみを
    渡すことで、react-query の re-render で `reset()` が無限ループする回帰を防止した。
    `isSuccess` → `onOpenChange(false)`（Req 3.1/3.2）、`error.kind === "cancelled"`
    → `mutation.reset()`（Req 2.7）、`error.kind === "session_exchange_failed"`
    → 文言表示 + `onOpenChange(false)`（Req 3.4）の 3 副作用を各 useEffect に分離。
  - **kind → 固定文言マップの `Partial<Record<...>>` 型化**: `cancelled` をキー不在に
    することで「表示なし」を型システム上でも明示（NFR 1.2 の内部詳細反射禁止と整合）。
    `error.message` / `ApiError.body` は DOM に一切出さず、`kind` 対応の固定文言のみを
    表示する。`server_rejected` / `server_error` / `network_error` は同一汎用文言に
    集約（Req 2.8）。
  - **`session_exchange_failed` の同時表示 + 閉じ**: tasks.md L220-221 / design.md
    §PasskeySignupDialog の指示（文言表示 + Dialog を閉じる）を素直に実装。Radix
    Dialog は portal 経由でアンマウントされるため文言は瞬間的にしか可視化されない
    （下記「残存課題」参照）。
  - **テスト**: `usePasskeyRegistration` を `vi.mock` で差し替え、`PasskeyRegistrationError`
    は `vi.importActual` で実物クラスを再 export。tasks.md L226-232 の 6 ケースを
    Arrange/Act/Assert 分離・BDD 命名で検証（recovery email 欄不在 / mutate 引数 /
    invalid_username / username_taken / cancelled で汎用文言非表示 + reset 呼出 /
    isSuccess で onOpenChange(false)）。`npm test` 全体 475 tests pass（Task 8 の
    469 から +6）、`npm run lint` は新規ファイル由来の警告なし。
- **残存課題（task 10 に影響）**:
  - **Signup Dialog のマウント方式**: task 10（LoginPage 統合）で open 管理する際、
    「閉じたら次回は空欄で開く」UX を担保するためアンマウント方式
    （`{signupOpen && <PasskeySignupDialog ... />}`）を推奨。内部 `username` state と
    mutation state が close 時に破棄され、再 open で初期化される。
  - **`session_exchange_failed` 時の UX 分担**: 現状は Dialog 内で alert 文言を
    瞬間表示 + close するため、閉じた後のログイン画面には何も残らない。Req 3.4 の
    「汎用エラーを提示してログイン画面に復帰」の趣旨をより忠実に満たすには、task 10 の
    LoginPage 統合時に親側 toast 化（`onSessionExchangeFailed?` callback 等の Props
    拡張）を検討する余地がある。本 task では Props 拡張の裁量を持たなかったため未実装。
    design.md 本文でも「Dialog を閉じる」以外の UX 責務分担が未記載であり、人間
    Reviewer による整合性確認を推奨（既存「確認事項」節の api.ts 204 課題と同様、
    本 spec の Boundary 制約下では task 内解消不可）。

### Task 10

- **採用方針**: `PasskeyButtons` を capability 判定に基づく null 返却 + mutation 状態表示の
  純関数コンポーネントとして実装し、`LoginPage` は state 管理 + 3 導線の合成のみを担う薄い
  親に留める（責務分離 / CLAUDE.md §1）。`PasskeyButtons` は task 9 の `PasskeySignupDialog`
  と同 idiom で `useEffect` による cancelled reset と `Partial<Record<...>>` 型の
  `ERROR_MESSAGES` マップを採用し、NFR 1.2（内部詳細反射禁止）を型システム上でも表現した。
- **重要な判断**:
  - **`LoginPage.test` の hook モジュールモック方針**: 既存 4 テスト（Google 導線 / href /
    アプリ名 / 説明文）を `render(<LoginPage />)` の呼び出しを一字一句変えずに維持する
    ため（Req 6.1 / 6.4）、`use-passkey-capability` / `use-passkey-authentication` /
    `use-passkey-registration` の 3 hook をモジュール冒頭で `vi.mock` してデフォルトを
    idle mutation + `available: false` に固定した。これにより QueryClientProvider を
    render tree に追加する必要がなくなり、既存 test body が完全不変で通る。`ApiError` /
    `PasskeyAuthError` / `PasskeyRegistrationError` は `vi.importActual` で実物を再 export
    し、production 同等の instanceof / kind 判定を許容する（既存
    `passkey-signup-dialog.test.tsx` と同 idiom）。
  - **React 19 の JSX namespace 非露出への対応**: design.md L719 の Contracts では戻り値
    型を `JSX.Element | null` と例示しているが、React 19 + TypeScript 5 + Next.js 15 環境
    では global `JSX` namespace が既定で提供されず `tsc` (Next.js build) が
    "Cannot find namespace 'JSX'" を throw する。既存 web/ 配下の component も
    `login-page.tsx` / `passkey-signup-dialog.tsx` / `auth-guard.tsx` すべて明示的な戻り値
    型を持たない（TS 推論に委ねる）ため、本 file も同慣習に揃えて型注釈を省略した。
    design.md の contract（`(props: PasskeyButtonsProps) => JSX.Element | null` の意味論）は
    実装上「null または JSX を返す関数コンポーネント」として厳密に維持されており、
    `if (isLoading || !available) return null;` の分岐がそれを表現している。
  - **フック呼び出し順序と early return の分離**: React hooks のルールを守り
    `usePasskeyCapability` / `usePasskeyAuthentication` / `useEffect`（cancelled reset）を
    early return より **前**に無条件で呼ぶ。early return は最後の hook 呼び出しの後に
    置く（Task 9 の `PasskeySignupDialog` と同型の設計）。
  - **既存 Google 導線の DOM 完全不変**: `<Button asChild>` + `<a>` の nesting、`href` /
    class / 文言（`Googleアカウントでログイン`）と説明文（「初回ログイン時にアカウントが
    自動作成されます」）、外側の `flex min-h-screen items-center justify-center` /
    `w-full max-w-sm space-y-8 text-center` の container 構造をすべて維持し、
    `<PasskeyButtons>` と `<PasskeySignupDialog>` を Google ボタンブロックの直後に追加
    する形にした（Req 6.1 / 6.2 / 6.3 / 6.4）。パスキー導線非表示時（capability false）は
    `PasskeyButtons` が null 返却して DOM に何も残さないため、Google 単体構成の見た目は
    本 spec 導入前と同一を保つ。
  - **`ERROR_MESSAGES` 文言の 4 kind 集約**: `session_exchange_failed` / `server_rejected` /
    `server_error` / `network_error` はいずれも「認証に失敗しました。時間をおいて再度
    お試しください」の同一汎用文言に集約（tasks.md L250-251 の指定どおり / Req 4.6 / 4.7）。
    `cancelled` はキー不在で表示なし + `mutation.reset()`（Req 4.5）。design.md
    §PasskeyButtons Responsibilities L710-712 の「button disabled + 汎用エラー」に忠実に
    対応。
  - **テストは QueryClientProvider 不要**: `passkey-buttons.test.tsx` では
    `usePasskeyCapability` / `usePasskeyAuthentication` を両方 `vi.mock` し、
    `PasskeyAuthError` は `vi.importActual` で実物を再 export。既存
    `passkey-signup-dialog.test.tsx` の `buildMutation` / `mockRegistration` / `buildError`
    と同 idiom で `buildMutation` / `mockCapability` / `mockAuthentication` / `buildError`
    を組み立て、6 ケース（available true / false / isLoading / mutate 呼出 / onSignupClick
    呼出 / cancelled で alert 非表示 + reset）を Arrange/Act/Assert 分離・BDD 命名で検証。
    `container.firstChild === null` の assert で「本当に何も render されない」ことを回帰
    防止した（tasks.md L263-264 の「null 返却」の意味を厳密に検証）。
  - **NFR 1.1 遵守**: 本 component は `console.*` を一切呼ばず、`code_verifier` /
    `auth_code` / assertion 生バイトは `usePasskeyAuthentication` の mutation 内 closure に
    閉じ込められる（本 component からは触らない）。localStorage / sessionStorage / URL に
    書き込む経路もない。
  - **NFR 1.2 遵守**: `PasskeyAuthError` は `kind` の enum のみを DOM に反射させ、
    `ApiError.body` / `error.message` を一切表示しない（`ERROR_MESSAGES[kind]` の固定文言
    のみ）。`server_rejected` / `server_error` の内部区別を UI 上で識別不能にしている点も
    Req 4.6「拒否理由の内部区別を反射しない」を厳密に満たす。
- **残存課題（Reviewer への確認事項）**:
  - **design.md contract と TS 型注釈の乖離**: design.md L716-719 の
    `export function PasskeyButtons(props: PasskeyButtonsProps): JSX.Element | null;` は
    React 19 + Next.js 15 環境で build 不能のため、実装は明示戻り値型を省略した（既存
    codebase の component 慣習に合わせた）。design.md の意味論契約は毀損されない（null
    または JSX を返す）。design.md 本文の Contracts サンプルコード側の update は本 task の
    Boundary（spec 書き換え禁止）外のため未対応。人間 Reviewer への設計整合性確認を推奨。
  - **`session_exchange_failed` 発火経路の UX**: `usePasskeyAuthentication` が
    `session_exchange_failed` を返した場合、本 component では汎用エラー文言を alert 表示
    するのみで、Signup Dialog 側（task 9 の残存課題）と異なり Dialog 閉じの副作用は無い。
    ログインフローでは Dialog を扱わないため妥当だが、Reviewer は Req 3.4 の「ログイン画面
    に復帰」相当の UX が本 spec 内で担保されているかを task 9 の残存課題と併せて確認する
    こと。
  - **api.ts の 204 No Content 非対応**（task 7 / task 8 と同一の既知課題）: production
    統合時に `/api/auth/session` の 204 応答で `request<T>` の `response.json()` が runtime
    `SyntaxError` を throw する可能性がある。本 task では apiClient を mock している範囲で
    露出しないが、production 統合前に api.ts の 204 handling を別 spec / PR で追加する
    必要がある（既存「確認事項」の task 7 追記および task 8 追記を参照）。
  - **本 task 完了により spec 内の全 10 task が完了**: 追加の未完了マーカーは無く、
    Web 側パスキー導線導入は task 単位で完結。人間 Reviewer は 10 task 分の commit を
    まとめてレビューする形になる（本 spec の per-task ループ運用 = commit 単位が最終
    レビュー単位）。

## AC トレース

Task 1 で担保した AC は以下:

- **3.1（新規作成後のセッション合流）** / **4.2（ログイン成功後のセッション合流）**:
  `ExchangeAuthCodeForSession` の 5 段（find → verify → markUsed → generate → create）
  および正常系テスト `TestExchangeAuthCodeForSession_Success` で担保。拒否時の
  session 非永続化は `TestExchangeAuthCodeForSession_AuthCodeNotFound` /
  `_VerifierMismatch` / `_MarkUsedNotUsable` で担保。
- **NFR 1.1（機密情報の非漏出）**:
  `TestExchangeAuthCodeForSession_DoesNotLeakPlainSecretsInError` が ErrInvalidGrant /
  infra wrap の両経路で error メッセージに平文 authCode / codeVerifier が含まれない
  ことを検証。実装側は追跡ログを `code_hash[:8]` / `session_id_hash[:8]` に留めており、
  平文値を slog にも出さない（既存 `TokenService` と同方針）。

Task 2 で担保した AC は以下:

- **3.1（新規作成後のセッション合流）** / **4.2（ログイン成功後のセッション合流）**:
  `NativeAuthHandler.Session` の 204 応答 + Set-Cookie 発行を
  `TestNativeAuthHandler_Session_Success` および
  `TestNewRouter_NativeAuthSession_RegisteredWhenHandlerInjected` で担保。
  Cookie 属性は既存 Google OAuth Callback（`AuthHandler.Callback`）と同一
  （Name=`session_id` / Path=`/` / Domain=`CookieDomain` / MaxAge=`SessionMaxAge` /
  HttpOnly / Secure / SameSite=Lax）で、Web は追加操作なしに Cookie セッション認証状態に
  到達する。
- **3.4（合流失敗時にセッションに到達させない）** / **4.7（サーバエラー時に認証状態に
  到達させない）**: 400 INVALID_GRANT を `TestNativeAuthHandler_Session_InvalidGrant`、
  500 INTERNAL_ERROR を `TestNativeAuthHandler_Session_InternalError` で担保。両者とも
  Set-Cookie 未発行を assert し、handler 経路で session 発行が起きないことを検証。
- **NFR 1.1（機密情報の非漏出）**: 応答 message に内部詳細
  （`db connection refused` / `verifier` / `expired` / `mismatch` 等の拒否理由語彙）が
  反射されないことを `TestNativeAuthHandler_Session_InvalidGrant` /
  `_InternalError` の両テストで assert。handler は slog に平文 authCode /
  codeVerifier / sessionID を出さない実装（追跡は service 層の hash log に一元化）。
- **NFR 2.1（既存挙動不変）**: 既存 `Token` / `Refresh` / `Revoke` methods は完全不変
  （signature も含めて未編集）で、`go test ./...` all-green を確認。
  `NewNativeAuthHandler` の signature 拡張は variadic option 採用により既存 15
  callsites の書き換えを不要にした。route 登録は既存 3 route と同じ
  `NativeAuthHandler != nil` gate と `unauthIPMW` + `MaxBodyBytes` 順序で行い、
  route 順序も末尾追加のみ。
- **NFR 2.2（縮退時の既存挙動維持）**: `NATIVE_AUTH_JWT_SECRET` 未設定 →
  `NativeAuthHandler = nil` → `/api/auth/session` route 未登録 → 404 を
  `TestNewRouter_NativeAuthSession_NotRegisteredWhenHandlerNil` で担保。
  `TestNewRouter_NativeAuthSessionIPRateLimit_429OnExcess` および
  `_SameShapeAsExistingRoutes` で `unauthIPMW` の閾値超過時の 429 応答が既存
  `/health` 429 応答と body / header レベルで同一形式であることを assert。

Task 6 で担保した AC は以下:

- **5.1（ブラウザ非対応時の導線縮退）**: `isPasskeyBrowserSupported()` の 4 ケース
  （`window.PublicKeyCredential` = function / undefined / object / SSR 環境相当）を
  `web/src/lib/passkey-capability.test.ts` で担保。関数の boolean 出力が
  `use-passkey-capability` の `available` に AND として反映される（消費側の
  `PasskeyButtons` は task 10 で `available === false` を `null` 返却に落とし込む
  想定）。
- **5.2（サーバ非提供時の導線縮退）**: `usePasskeyCapability` が
  `GET /api/passkey/capability` の 404 (`ApiError`) を catch して server=false を
  返し、`available: false` に集約することを `use-passkey-capability.test.tsx` の
  「サーバ 404 + browser あり → available: false」ケースで担保。fetch reject
  （`TypeError`）も同経路で false に落ちることを別ケースで assert し、Web は
  サーバ非提供構成を Google 単体構成に静かに縮退する。
- **5.3（縮退時も Google 導線は不変）**: 本 hook は `PasskeyButtons` の gating に
  のみ関与し、Google 導線を制御しない。実装は `available === false` を導線非表示
  の trigger にする形で、Google 導線側は無関係（task 10 で `LoginPage` 統合時に
  最終確認）。本 task 内では hook が `available` boolean を過不足なく返すことで
  5.3 の前提を作る。
- **5.4（導線無効環境で試行しても Google 案内に留まる）**: 5.1 / 5.2 の 4 ケース
  全てで `available === false` が確定するため、消費側は「導線をレンダしない」
  経路を選択できる。パスキー処理の起動は `PasskeyButtons` の onClick 経由に
  限定される設計で、`available === false` 時は button 自体が render されないため
  試行不能（task 10 で最終確認）。
- **NFR 1.4（同一オリジン限定）**: `apiClient.get("/api/passkey/capability")` は
  `API_BASE_URL=""` の相対パス経由で同一オリジンに閉じる（`web/src/lib/api.ts` の
  既存契約に依存）。テストの mock 検証で `expect(apiClient.get).toHaveBeenCalledWith("/api/passkey/capability")`
  として絶対 URL を用いていないことを回帰的に assert（正常系ケース）。

Task 7 で担保した AC は以下（`web/src/hooks/use-passkey-authentication.test.tsx` の
5 ケースで検証）:

- **4.1（ブラウザのパスキー選択 UI 起動 / username 不要）** / **4.2（成功で追加操作なく
  Cookie セッションに到達）** / **4.3（既存機能一式）** / **4.4（iOS 由来パスキーでも
  同一経路）**: 「正常系」ケースが PKCE → begin → get → finish → session の順序を assert し、
  `queryClient.invalidateQueries({queryKey: ["auth", "me"]})` の呼び出しを spy で検証。
  hook は username を送らず discoverable login として `authentication/begin` を
  `{code_challenge}` のみで呼ぶ（4.1 の「username 入力不要」を実装契約で担保）。4.4 は
  同一 endpoint 経由のため個別ケース不要（サーバ側 `AuthenticationService` が iOS 由来
  credential も同一 flow で解決する）。
- **4.5（ブラウザ UI キャンセルで復帰）**: 「cancelled」ケースが `navigator.credentials.get`
  の `NotAllowedError` DOMException を `PasskeyAuthError.kind === "cancelled"` に分類する
  ことを検証。以降の finish / session が呼ばれないことも assert し、認証状態に到達
  させない挙動を担保。
- **4.6（サーバ拒否理由の内部区別を反射せず汎用エラー）**: 「server_rejected」ケースが
  authentication/finish の 400 AUTHENTICATION_FAILED を `kind === "server_rejected"` に
  分類することと、`error.message` に "AUTHENTICATION_FAILED" 文字列が含まれないことを
  assert（NFR 1.2 の反射禁止と同時担保）。
- **4.7（サーバエラー時に認証状態に到達させない）**: 「server_error」ケースが
  authentication/begin の 500 を `kind === "server_error"` に分類し、以降の endpoint が
  呼ばれないことを assert。
- **3.4（合流失敗時に認証状態に到達させない）**（partial: session 側の合流失敗判別に該当）:
  「session_exchange_failed」ケースが `/api/auth/session` の 400 INVALID_GRANT を step 5 の
  失敗として `kind === "session_exchange_failed"` に振り分けることを検証。同じ 400 が
  step 4（finish）では `server_rejected` に振り分けられるという対比を「server_rejected」
  ケースとの並置で確認。
- **NFR 1.1（機密情報の非漏出）**: 実装で `console.*` を一切呼ばず、`code_verifier` /
  `auth_code` / assertion 生値をモジュール変数・storage・URL に残さない。テストは
  encode/decode を mock 化しているため生値の露出経路がない。runtime 挙動としては
  mutation の closure が終了時に GC 対象になる前提。
- **NFR 1.2（サーバ内部詳細を UI・console に反射しない）**: `PasskeyAuthError` は
  `kind` の enum のみを持ち、`ApiError.body` を保持しない。「server_rejected」テストで
  error.message に "AUTHENTICATION_FAILED" が反射されないことを assert 経由で回帰防止。
- **NFR 1.4（同一オリジン限定）**: `apiClient` 経由の相対パス呼び出しで
  `API_BASE_URL = ""`（`web/src/lib/api.ts` の既存契約）に閉じる。「正常系」テストで
  3 endpoint がいずれも相対パスで呼ばれることを `toHaveBeenNthCalledWith` の URL 引数
  として assert。

Task 8 で担保した AC は以下（`web/src/hooks/use-passkey-registration.test.tsx` の
6 ケースで検証）:

- **2.1（作成開始操作の提示）**（partial: hook 契約側）: hook は
  `usePasskeyRegistration().mutate({username})` を作成開始 API として提供する。
  UI 側の入力欄・作成ボタン提示は task 9（`PasskeySignupDialog`）の責務のため、
  hook 単体テストでは mutation 契約（`{username: string}` 引数を受け取り実行する）
  で 2.1 の前提を作る。「正常系」ケースで `mutate({username: "alice"})` の受理と
  chain 開始が確認できる。
- **2.2（ブラウザのパスキー作成 UI 起動）**: 「正常系」ケースが
  PKCE → registration/begin → `navigator.credentials.create` の順序を assert し、
  `decodeCreationOptions` が begin レスポンスの `options` を受け取ることを
  `toHaveBeenCalledWith({publicKey: {}})` として検証。
- **2.3（作成成功後、追加操作なしで合流フローへ）**: 「正常系」ケースで
  registration/finish 成功後、追加のユーザー操作を挟まずに authentication/begin →
  get → authentication/finish → session 交換の 5 endpoint が順に呼ばれる
  ことを `toHaveBeenNthCalledWith` の 3〜5 番目として assert。
- **2.4（recovery email 未指定を欠落として扱わない）**: 「正常系」ケースが
  registration/begin 呼び出しの payload に `email: ""` を含むことを
  `toHaveBeenNthCalledWith(1, "/api/passkey/registration/begin", {username, email: "", code_challenge})`
  として assert。UI 側は task 9 で email 入力欄を設けない実装で 2.4 を UI で
  確定させる（本 hook はサーバ契約側で `email: ""` を必ず送る形で担保）。
- **2.5（username 形式不正の表示）**（partial: 分類側）: 「invalid_username」
  ケースが registration/begin の 400 with `code: "INVALID_USERNAME"` を
  `kind === "invalid_username"` に分類することを検証。UI 表示文言は task 9 の
  責務のため、本 task では kind の分類までを担保。
- **2.6（username 重複の表示）**（partial: 分類側）: 「username_taken」
  ケースが registration/begin の 409 を `kind === "username_taken"` に分類する
  ことを検証。UI 表示文言は task 9 の責務。
- **2.7（ブラウザ UI キャンセルで復帰）**（partial: 分類側）: 「cancelled」
  ケースが `navigator.credentials.create` の `NotAllowedError` DOMException を
  `kind === "cancelled"` に分類することと、以降の registration/finish が
  呼ばれないことを assert。「画面を壊さず戻す」の UI 実装は task 9 の
  `mutation.reset()` 経路で確定。
- **2.8（サーバエラーの汎用表示・内部詳細非反射）**（partial: 分類側）:
  「server_rejected」ケースが registration/finish の 400 REGISTRATION_FAILED を
  `kind === "server_rejected"` に分類することと、`error.message` に
  "REGISTRATION_FAILED" が反射されないことを assert。NFR 1.2 と同時担保。
- **3.1（追加操作なしで Cookie セッションに到達）**: 「正常系」ケースが
  `/api/auth/session` を chain の最終 step として呼び、`queryClient.invalidateQueries({queryKey: ["auth", "me"]})`
  が呼ばれることを spy で検証。
- **3.2（2 ペイン UI を初期表示）**: `invalidateQueries` の呼び出しにより
  `AuthGuard` が再判定して認証済み分岐へ遷移する経路を hook 側で作る。
  実際の 2 ペイン UI 描画は既存 `AuthGuard` / `AppShell` の責務。
- **3.4（合流失敗時にセッションに到達させない）**: 「session_exchange_failed」
  ケースが `/api/auth/session` の 400 INVALID_GRANT を step 8 の失敗として
  `kind === "session_exchange_failed"` に振り分けることを検証。同じ 400 が
  step 4（reg_finish）では `server_rejected` に振り分けられるという対比を
  「server_rejected」ケースとの並置で確認。
- **NFR 1.1（機密情報の非漏出）**: 実装で `console.*` を一切呼ばず、
  `codeVerifier` / `authCode` / attestation 生値 / assertion 生値を
  モジュール変数・storage・URL に残さない。テストは encode/decode を mock 化
  しているため生値の露出経路がない。
- **NFR 1.2（サーバ内部詳細を UI・console に反射しない）**:
  `PasskeyRegistrationError` は `kind` の enum のみを持ち、`ApiError.body` を
  保持しない。「invalid_username」/「username_taken」/「server_rejected」の
  3 ケースで error.message に "INVALID_USERNAME" / "USERNAME_TAKEN" /
  "REGISTRATION_FAILED" が反射されないことを assert 経由で回帰防止。
- **NFR 1.4（同一オリジン限定）**: `apiClient` 経由の相対パス呼び出しで
  `API_BASE_URL = ""` に閉じる。「正常系」テストで 5 endpoint がいずれも
  相対パスで呼ばれることを `toHaveBeenNthCalledWith` の URL 引数として assert。

## 確認事項

- Task 1 時点: 現時点でなし。design.md § SessionExchangeService の Contracts と実装は一致。
- Task 2 追記: design.md §NativeAuthHandler.Session の struct field 例示
  （`sessionExchange *auth.SessionExchangeService` の具体型）に対し、実装は
  `SessionExchanger` narrow interface を追加してそこに `*auth.SessionExchangeService`
  を構造的に充足させる形にした（interface segregation / CLAUDE.md §5 / 既存
  `TokenExchangeService` と同 idiom / testability 向上）。API 契約
  （POST /api/auth/session / Cookie 属性 / status code / error code）および
  fail-closed の連動は不変。人間 Reviewer による design.md との整合性確認を推奨。
- Task 4 追記: design.md §lib/pkce.ts の Contracts では `generatePkcePair` + `PkcePair`
  のみが例示されているが、実装では S256 派生を担う純粋関数を `deriveCodeChallengeS256`
  として追加 export した（Boundary `lib/pkce` 内で完結する testability 向上のため。
  RFC 7636 Appendix B の既知ベクトルでサーバ `sha256.Sum256([]byte(verifier))` との
  対称性を回帰テスト化する目的）。`generatePkcePair` の signature と `PkcePair` の
  shape は design.md 契約と厳密一致で毀損なし。人間 Reviewer による design.md との
  整合性確認を推奨。
- Task 5 追記: design.md §lib/webauthn.ts の `encodeAttestationResponse` /
  `encodeAssertionResponse` の Contracts は `cred.rawId` / `cred.response.clientDataJSON`
  / `cred.response.attestationObject`（および assertion 側の `authenticatorData` /
  `signature` / `userHandle`）の base64url 化のみを列挙している。しかし実サーバ
  （go-webauthn v0.17.4 の `CredentialCreationResponse.Parse()` /
  `CredentialAssertionResponse.Parse()` in `protocol/credential.go` L129-144）は
  `ccr.ID == ""` と `ccr.Type != "public-key"` を明示的に reject するため、実装では
  WebAuthn IDL `PublicKeyCredentialJSON` に準じ **top-level `id` / `type` を追加出力**
  している（`id` は cred.id の base64url 文字列そのまま、`type` は cred.type = "public-key"）。
  design.md 契約の「他フィールドは透過的にコピー」の暗黙前提と整合すると解釈しているが、
  design.md 本文には明記が無かった点は人間 Reviewer による整合性確認を推奨する。
  加えて base64url 変換ヘルパは `lib/pkce.ts` の private `bytesToBase64url` と重複
  実装になっているため（Boundary `lib/webauthn` 制約下での判断）、共有化 PR
  （`web/src/lib/base64url.ts` 新設）の別 spec 起票を推奨する。
- Task 6 時点: design.md § types/passkey.ts / lib/passkey-capability.ts /
  use-passkey-capability.ts の 3 セクションと実装は厳密一致（新規追加関数・追加
  export・型追加は無し）。`queryFn` 内で予期しない例外（`ApiError` / `TypeError`
  以外）も安全側に false へ集約している点は design.md 本文の明記対象外だが、
  Requirement 5.2「サーバ非提供構成での縮退」の趣旨（fail-closed / 縮退時の
  Google 単体構成到達を保証）と整合的と解釈している。人間 Reviewer による
  design.md との整合性確認を推奨。

- Task 7 追記:
  - **`PasskeyAuthErrorKind` の kinds 数**: design.md §Components
    （use-passkey-authentication.ts）の Contracts では 4 種のみが例示されているが、
    実装は tasks.md L155（`POST /api/auth/session` の 400/500 →
    `session_exchange_failed`）と design.md §Error Handling L1042 の「System Errors」
    列挙に従い、5 種（`cancelled` / `server_error` / `server_rejected` /
    `network_error` / `session_exchange_failed`）として実装した。design.md 内で
    §Components と §Error Handling の間に軽微な非整合があるため、人間 Reviewer に
    よる整合性確認を推奨する（実装は tasks.md 側の要件を満たす形）。
  - **api.ts の 204 No Content 非対応**: `web/src/lib/api.ts` の `request<T>` は
    `response.json()` を無条件で呼ぶため、design.md L859 で規定された
    `/api/auth/session` の 204 No Content 応答に対して runtime で `SyntaxError` を
    throw する可能性がある。Task 7 の Boundary は `hooks/use-passkey-authentication`
    に限定されており api.ts 側を修正できないため、実装は tasks.md L147 の指定どおり
    `apiClient.post` を使用し、テストは apiClient を mock して runtime 挙動を回避
    している。**production 統合前に api.ts の 204 handling を別 spec / PR で追加する
    必要がある**（例: `request<T>` が `response.status === 204` のときに
    `undefined as unknown as T` を返す分岐を追加）。task 8（新規作成 mutation）でも
    同じ endpoint を呼ぶため、同時に修正することを推奨。
  - **`navigator.credentials.get` null 返却時の cancel 扱い**: WebAuthn 仕様上、
    キャンセル時に throw ではなく `null` を resolve するブラウザ実装の余地がある
    ため、`if (!cred) throw new PasskeyAuthError("cancelled")` で明示的に cancel
    カテゴリに集約している。design.md §Error Handling には明記が無いが、
    Req 4.5「画面を壊さずに戻す」の網羅性向上として実装判断で追加した。

- Task 8 追記:
  - **task 7 との共有 utility 抽出見送り**: `classifyError` / `isCancelledError` /
    `CANCEL_ERROR_NAMES` / step 定数の同種ロジックが task 7
    （`use-passkey-authentication.ts`）と task 8 （`use-passkey-registration.ts`）で
    2 file に重複している。task 8 の Boundary は `hooks/use-passkey-registration`
    に限定されており、task 7 の hook file を編集する共有化変更は boundary 逸脱に
    なるため見送った（impl-notes Task 7 の残存課題にも同旨の記載あり）。共有化
    PR（例: `web/src/hooks/passkey-error.ts` を新設して task 7 / 8 の hook から
    継承する形）の別 spec 起票を推奨する。関数シグネチャ・定数集合は対称に
    保ってあるため、後日の抽出は機械的に行える見込み。
  - **task 7 の非整合との対比**: task 7 では design.md §Components
    （use-passkey-authentication.ts）Contracts と §Error Handling の間に kind 数
    の軽微な非整合（4 vs 5）があったが、task 8 側では design.md §Components
    （use-passkey-registration.ts）と §Error Handling の kind 列挙が 7 種で整合
    している。tasks.md L182-193 の 7 種と厳密一致で実装した。
  - **api.ts の 204 No Content 非対応**（task 7 と同一の残存課題）:
    `/api/auth/session` の 204 応答で `request<T>` が `response.json()` を無条件
    に呼ぶことによる runtime `SyntaxError` の可能性は本 task でも解消できていない
    （Boundary 制約下）。task 7 追記に既に記載した通り、production 統合前に
    api.ts の 204 handling を別 spec / PR で追加する必要がある。task 9 / 10 では
    hook を UI から呼び出すが、テストは apiClient を mock している限り露出しない。
  - **`code_challenge` の再利用**: 登録 chain の step 2（registration/begin）と
    認証 chain の step 5（authentication/begin）で同じ `codeChallenge` を再送
    する実装にした。サーバ側 #216 の `authentication/finish` は `MarkUsed` +
    `VerifyPKCES256Verifier` で `auth_code` に紐づく PKCE 検証を行うため、
    challenge 再利用が許容されるという解釈に依拠している（design.md §Flows
    「新規作成フロー」の 8 段 chain と整合）。人間 Reviewer による設計意図の
    整合性確認を推奨する。

## #231 normative delta 適用後の実装状態

以下は `docs/specs/231-design-web-auth/design.md` の Delta 1〜6 を PR #229 に適用した
最終状態であり、上記の旧 task 記録と競合する箇所を supersede する。

- **Delta 1 — 直接 Cookie session**: #223 design の「新規作成フロー（Sequence）」を
  supersede し、`registration/finish` 内の #230 user + credential tx を session まで拡張した。
  Web は 3 行を同一 tx で commit 後に Set-Cookie、iOS は従来どおり 2 行・Cookie なし。
- **Delta 2 — File Plan 補正**: #231 File Structure Plan の exact path を実差分へ反映し、
  session factory / Cookie builder / Origin config / API error 型と各回帰テストを追加した。
  例外として [U] 記録だった `internal/handler/router_unauth_ratelimit_test.go` は、Origin
  fail-closed と capability readiness 強化後も既存 rate-limit テストを同じ成功経路へ到達
  させる fixture 追従が不可避だったため編集した（production 挙動・閾値の変更なし）。
- **Delta 3 — fail-closed/readiness**: capability は login session、明示 exact Origin、
  direct-registration readiness の全成立時だけ 200。iOS registration/authentication の
  router gate は `PasskeyHandler != nil` のまま維持した。
- **Delta 4 — CSRF/PKCE**: Web registration と `/api/auth/session` は Origin を
  fail-closed にし、Cookie builder を共通化した。PKCE は login auth_code 交換だけに使い、
  直接登録は authorization gesture + exact Origin + JSON/CORS/SameSite で防御する。
- **Delta 5 — 完了不明状態**: request preparation、fetch reject、2xx parse failure を型で
  区別し、finish の送達・commit が不明な場合は `registration_uncertain` とする。
  UI は discoverable login を先に提示し、finish 400 `AUTHENTICATION_FAILED` 後だけ再作成を出す。
- **Delta 6 — 運用 config**: `CORS_ALLOWED_ORIGIN` から strict exact Origin を派生し、
  malformed/未設定は Web パスキーを fail-closed にする。Compose の未設定既定は空へ変更し、
  CORS ミドルウェア自身の localhost 既定は維持した。

STATUS: complete
