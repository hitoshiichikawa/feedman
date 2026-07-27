# 実装ノート（#223）

Web にパスキー導線（新規作成 / ログイン）を追加し、いずれの導線でも auth_code → Cookie
session の合流経路を通じて既存の 2 ペイン UI 認証状態に到達させる spec の実装ノートを
task 単位で記録する。前方伝播（先行 task の learning を後続 task で温存）を規律とする。

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
