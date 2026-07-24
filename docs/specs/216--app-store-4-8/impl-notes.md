# 実装ノート（Issue #216 パスキー + ユーザー名の自社認証・サーバ側）

## 確認事項

レビュワー / 人間運用者の判断が必要な点を列挙する。

- **既存 gofmt 違反 11 件（本 task の範囲外・要判断）**: `gofmt -l internal/` を repo root で
  実行すると、本 task で一切触れていない既存ファイル 11 件がフォーマット違反として検出される
  （`internal/crossfeed/service_test.go` / `internal/handler/crossfeed_handler_test.go` /
  `internal/handler/feed_handler_test.go` / `internal/handler/item_search_handler.go` /
  `internal/handler/router_full_test.go` / `internal/hatebu/batch.go` /
  `internal/itemsearch/service.go` / `internal/itemsearch/service_test.go` /
  `internal/middleware/ratelimit_test.go` / `internal/model/item.go` /
  `internal/security/content_sanitizer_test.go`）。これらは全て `develop` 由来の
  **pre-existing** な違反で、本 task の変更ファイル（migration 2 / model 2 / passkey 2）は
  すべて gofmt clean。tasks.md の Verify ブロック
  `gofmt -l internal/ | (! grep .)` はこの既存違反により本 task 導入前から fail 状態のため、
  stage-a-verify gate が発火する可能性がある。boundary（MigrationSchema / PasskeyModel /
  UsernameValidator）外の修正は本 task では行わなかった。別 Issue での cleanup 起票、または
  gate の許容判断を人間に委ねる。
- **task 4 の codeChallenge 引数（design.md L596-598 と tasks.md L95 の差分 / 実装判断）**:
  design.md の Go シグネチャ L596-598 は `BeginRegistrationNew(ctx, rawUsername string,
  optionalEmail string)` で codeChallenge 引数を持たないが、tasks.md L95 は
  `BeginRegistrationNew(ctx, rawUsername, optionalEmail, codeChallenge)` の 4 引数を
  指示し、design.md L707 の API 契約（`{username, email?, code_challenge}`）と整合する。
  一方、design.md L610 の `FinishRegistrationNew` は `(userID string, err error)` を返し
  **auth_code を発行しない**。task 4 の `_Requirements:_` にも Req 2.x（PKCE / auth_code）
  は含まれない。加えて `model.PasskeyChallenge`（task 1 で確定・変更不可）は PKCE 用
  フィールドを持たず、`SessionData` は go-webauthn.SessionData そのままの契約なので
  PKCE を混ぜられない。以上の制約下で「session_data に PKCE を保存して後段継承」を
  task 4 内で実現する手段は無い。**採用解**: `BeginRegistrationNew` に `codeChallenge
  string` 引数を追加し、`auth.ValidatePKCES256(codeChallenge, "S256")` で **early
  validation のみ**を実施（不正 PKCE で無駄な WebAuthn ceremony 起動を防ぐ）。**PKCE の
  永続化・後段継承は本 task では省略**（FinishRegistrationNew が auth_code を発行しない
  ため事実上 no-op）。task 5 の AuthenticationService 側で `BeginAuthentication` が
  code_challenge を受け取り session に載せて finish 時に PKCEChallenge として auth_code に
  紐付ける設計を tasks.md L135-152 が担うため、新規登録から auth_code 発行までの一連の
  合流は認証フロー側で完結する（新規登録 finish 直後に別途 `/api/passkey/authentication/*`
  を叩く前提）。design/tasks 上の軽微な不整合であり実装は tasks.md に従った。
- **task 5 の BeginAuthentication codeChallenge 引数（design.md L655-656 と tasks.md L135 の差分 / 実装判断）**:
  design.md L655-656 の Go シグネチャは `BeginAuthentication(ctx context.Context) (challengeID
  string, options []byte, err error)` で codeChallenge 引数を持たないが、tasks.md L135-139 は
  「既存 `auth.ValidatePKCES256(codeChallenge, "S256")` を呼び validation → ... session_data には
  codeChallenge を含める」ことを求めており、同 design.md L674 以降の「設計判断: PKCE 相互作用と
  合流方式」節（採用案）は begin リクエストで `code_challenge` を受け取り challenge に紐付けて
  保存する契約を確定している。task 4 の `BeginRegistrationNew` で tasks.md 優先の判断
  （codeChallenge 引数を追加）を採用したのと **同じ判断**を踏襲し、`BeginAuthentication` に
  `codeChallenge string` 引数を追加した。
- **task 5 の authentication kind session_data 封筒化（実装判断）**: `model.PasskeyChallenge`
  （task 1 で確定・変更不可）は PKCE 用フィールドを持たず、`SessionData` は go-webauthn.SessionData
  の JSON marshaled バイト列という契約になっている。PKCE codeChallenge を finish 段階まで運ぶ
  経路が他に無いため、authentication kind の SessionData のみを `authnSession` 封筒
  （`{"webauthn_session": <inner>, "code_challenge": <pkce>}`）で包む方式を採用した。
  ChallengeStore はバイト列を opaque に扱うため封筒でも意味的な差分は生じず、inner の webauthn
  session はそのまま無改変で FinishLogin に渡せる。registration kind の SessionData は task 4 で
  素の go-webauthn.SessionData として保存しており、本 task の封筒化は authentication kind のみに
  局所化される（互換性影響なし / NFR 2.1）。
- **design.md File Structure Plan の表記ゆれ（実装は tasks.md に準拠）**: design.md L254 の
  ディレクトリ tree では passkey ドメイン型を `internal/passkey/model.go` と記載しているが、
  同 design.md の Modified Files（L267）・コード内コメント（L390 / L413 の
  `// internal/model/passkey.go`）および tasks.md task 1（L19）は `internal/model/passkey.go`
  を指す。後者が多数派かつ tasks.md が正本のため、モデルは `internal/model/passkey.go` に
  配置した。design 上の軽微な不整合であり実装判断には影響しない。

## Implementation Notes

各 task の実装で得た learning を task ごとに記録する（per-task ループ / Req 4.1, 4.2, 4.4）。

### Task 1

- **採用方針**: 永続化スキーマ（migration up/down）・ドメインモデル（`model.PasskeyCredential`
  / `model.PasskeyChallenge` / `model.PasskeyChallengeKind`・`User` の username 2 カラム）・
  username 検証正規化（`passkey.ValidateAndNormalize`）を、既存 native auth migration /
  `model.AuthCode` の書式・doc comment 流儀に揃えて追加した。
- **重要な判断**:
  - username 検証は Unicode 変換を避け、許容文字が全て ASCII 1 byte である前提で byte 単位
    ループ + lowercase 変換の純粋関数として実装（3〜32 文字・`[a-zA-Z0-9_-]`）。長さ判定を
    byte 長で行っても文字数と一致する。境界値（空 / 2 / 3 / 32 / 33 / 64・Unicode・制御文字・
    空白・許容外記号）を table-driven test で網羅した。
  - `users.username_normalized` は **部分 UNIQUE INDEX**（`WHERE username_normalized IS NOT
    NULL`）とし、既存 Google 由来ユーザー（NULL）を無変更で許容（データ移行不要 / NFR 2.1）。
  - `passkey_credentials` / `passkey_challenges` は user_id FK を `ON DELETE CASCADE` とし、
    退会 cleanup（task 8）の防衛線を schema 側にも確保した。
- **残存課題（次 task への申し送り）**:
  - task 2 の `PostgresUserRepo` 追加メソッド（`FindByNormalizedUsername` / `CreateUserOnly`）
    では、既存 `FindByID` の SELECT が `username` / `username_normalized` 列を scan していない
    点に留意（`FindByID` を拡張するか passkey 専用 method を切るかは task 2 の判断領域）。
  - `FindByNormalizedUsername` の SELECT は部分 UNIQUE の NULL 挙動（NULL 同士は一意衝突しない）
    を前提に、normalized 空文字ではなく NULL を明示的に区別する必要がある。
  - 上記「確認事項」の既存 gofmt 違反 11 件は本 spec と独立の技術債。

### Task 2

- **採用方針**: repository 層（`PasskeyCredentialRepository` / `PasskeyChallengeRepository` の
  新規 2 repo + `UserRepository` 拡張）を、既存 `PostgresAuthCodeRepo`（Create の COALESCE
  デフォルト・`MarkUsed` の atomic UPDATE・`DeleteByUserIDExec` の DBTX パターン）と
  `postgres_feed_repo.go`（`pq.Error.Code` による PG エラーコード判定）の流儀に厳密に揃えて実装した。
- **重要な判断**:
  - **UNIQUE 衝突検出**は `pgErrCodeUniqueViolation = "23505"` を定義し `errors.As(&pq.Error)` で
    判定。credential_id 衝突 → `ErrCredentialAlreadyRegistered`、username_normalized 衝突 →
    `ErrUsernameTaken`。各 INSERT で現実に発生し得る UNIQUE が 1 つに限られるため constraint 名
    分岐は省略（doc comment に将来の分岐指針を明記）。
  - **`FindByID` の SELECT を拡張**し `username` / `username_normalized` 列も scan するようにした
    （task 1 impl-notes が「拡張するか passkey 専用 method を切るかは task 2 の判断領域」と
    委任した点への回答）。NULL は `sql.NullString` で受け空文字にマップするため既存呼び出し側は
    無影響（返す型・既存フィールド値とも不変 / NFR 2.1）。design.md の「FindByID 無変更」表記とは
    軽微に相違するが、非破壊拡張と判断（Reviewer 確認事項として PR 本文で明示予定）。
  - **`FindByID` を `PasskeyChallengeRepository` に追加**（interface + 実装 + テスト）。tasks.md
    task 3 の「id で FindByID を challenge repo に追加（task 2 スコープ調整）」に従い、opaque
    challenge_id での逆引きを task 2 側で完成させ、task 3（boundary=ChallengeStore/WebAuthnAdapter）
    が repository 境界に触れずに済むようにした。未存在は `(nil, nil)`。
  - **`UpdateSignCount(ctx, id, signCount uint32, lastUsedAt time.Time)`** を採用（PK id で
    sign_count + last_used_at を同時更新。DB は BIGINT なので `int64()` 昇格）。**transports** は
    カンマ区切り直列化（WebAuthn の固定 enum はカンマを含まない / 空スライス↔空文字）。
    **aaguid / last_used_at** は nullable として `len()==0`→NULL / `sql.NullTime`+既存 `nullTimeValue`
    で扱う。
  - 検証: 自分の変更 18 ファイルは gofmt clean、`go vet` / `go build` / `go test ./...`（DB 無しは
    skip、DB 有りで新規 22 subtests 全 PASS + 既存契約テスト green）。副次整合として 9 テスト
    ファイルの cleanup SQL に passkey 2 テーブルの `DROP TABLE IF EXISTS ... CASCADE` を追加し、
    既存 mock UserRepository（auth/user service_test）に no-op stub を追加（interface 充足）。
- **残存課題（次 task への申し送り）**:
  - **`ErrChallengeNotUsable` sentinel の二重定義に注意**: 本 task で `repository.ErrChallengeNotUsable`
    を定義済み。task 3 が `internal/passkey/errors.go` に `passkey.ErrChallengeNotUsable` を別途
    集約する場合、`ChallengeStore.Consume` 内で `errors.Is(err, repository.ErrChallengeNotUsable)`
    を `passkey` 側 sentinel に変換（wrap / re-map）して 1 本化すること。片側だけで `errors.Is`
    判定すると取りこぼす。
  - task 3 `ChallengeStore.Consume(challengeID, expectedKind)` は `FindByID`（未存在→nil）→
    service 層で `now().After(ExpiresAt)` / `Consumed` / `Kind` を判定 → `MarkConsumed`（atomic で
    二重消費・期限切れも `ErrChallengeNotUsable`）の順で素直に組める。`Create` は呼び出し側で
    `HashNativeSecret(rawChallenge)` 済みの `ChallengeHash` と `json.Marshal(webauthn.SessionData)`
    済みの `SessionData` を渡す前提。
  - task 5 は `UpdateSignCount(ctx, cred.ID, newCount, now())` を呼ぶ。counter 後退検出（NFR 1.4）は
    go-webauthn の `Authenticator.CloneWarning` 側で行い、repository 層では判定しない。

### Task 3

- **採用方針**: `internal/passkey/` に `errors.go`（sentinel 集約） / `webauthn_adapter.go`（go-webauthn wrap） /
  `challenge_store.go`（TTL 付き単回利用 challenge の Issue/Consume）を追加し、design.md L458-557 の契約と
  L868-909 の Error Handling を素直に写した。単体テストは対象コード近傍に配置し、challenge_store は stub repo、
  webauthn_adapter は仮想 authenticator（github.com/descope/virtualwebauthn）で外部ネットワーク非依存に
  round trip を回した（NFR 4.1）。
- **重要な判断**:
  - **go-webauthn の Finish は低レベル API を採用**: `wa.FinishRegistration` / `FinishLogin` は `*http.Request` を
    要求するが、handler 層で request を再構築するのは煩雑なため、`protocol.ParseCredentialCreationResponseBytes`
    / `ParseCredentialRequestResponseBytes` + `wa.CreateCredential` / `ValidateDiscoverableLogin` の bytes 版を
    採用した。sessionData は `json.Marshal(webauthn.SessionData)` の byte 列で Issue と Consume 間を運ぶ。
  - **counter 後退（NFR 1.4）は `cred.Authenticator.CloneWarning` で検出**し `ErrAuthenticationFailed` に
    正規化する。go-webauthn の `UpdateCounter` は `authDataCount <= a.SignCount && (authDataCount != 0 ||
    a.SignCount != 0)` で CloneWarning を立てる仕様。stored SignCount=10 / assertion Counter=1 の状態で
    ValidateDiscoverableLogin 成功後に CloneWarning=true が返ることを test で担保。
  - **round-trip テストに test-only 依存 `github.com/descope/virtualwebauthn` v1.0.5 を採用**: design.md の
    Technology Stack は本番依存の go-webauthn のみを列挙しているため、これは design 記載外の追加。仮想
    authenticator は本番 binary には含まれない test-only 依存（`// indirect` にも入らず require 側で明示）で、
    NFR 4.1（外部ネットワーク非依存 / synthetic authenticator）を最小コストで達成する手段として合理的と
    判断した。virtualwebauthn は go-webauthn の対抗実装として同 test/ ディレクトリで round-trip サンプルを
    保守しており、v1.0.5 は go 1.25 でも問題なく動作した。
  - **`WebAuthnUser` は `webauthn.User` の型エイリアス**（`type WebAuthnUser = webauthn.User`）とした。design.md
    は同一メソッド集合の独立 interface として書いているが、`wa.BeginRegistration` / `CreateCredential` /
    handler へそのまま渡す前提を踏まえるとエイリアスの方が摩擦がなく、interface 二重管理も回避できる。
    RegistrationService / AuthenticationService（task 4/5）は本エイリアスを実装するだけで良い。
  - **`ErrInvalidUsername` は再定義しない**: username.go（task 1）に既存 sentinel があるため errors.go では
    残り 5 sentinel（`ErrUsernameTaken` / `ErrRegistrationFailed` / `ErrAuthenticationFailed` /
    `ErrChallengeNotUsable` / `ErrCredentialAlreadyRegistered`）のみを集約した。重複宣言によるコンパイル
    エラーを避ける（本 prompt 事前調査で明示された事項）。
  - **`ChallengeStore.Consume` の re-map**: `MarkConsumed` が返す `repository.ErrChallengeNotUsable` を
    `errors.Is` で受け止め、`passkey.ErrChallengeNotUsable` に変換して呼び出し側へ返す。二層で個別に
    `errors.Is` する取りこぼしを防ぎ、上位（RegistrationService / AuthenticationService）は passkey 側
    sentinel の判定のみで uniform 拒否を実装できる（task 2 impl-notes 参照）。事前判定（未存在 / kind 不一致 /
    期限切れ / consumed）も同じ sentinel に統一する。
  - **`ChallengeStore` の依存**: `challengeRepo` interface を `Create` / `FindByID` / `MarkConsumed` の 3 メソッドに
    絞った unexported interface として宣言し（interface segregation）、`repository.PostgresPasskeyChallengeRepo`
    は構造的にこれを充足する。testable かつ wiring 時に具体型をそのまま渡せる。
  - **RP 設定検証**: `webauthn.New` は `RPOrigins` が空だと error を返す仕様なので、`NewGoWebAuthnAdapter`
    が空 origins を渡した際に error を返すことを test で確認（config wiring / task 7 の fail-closed 縮退の
    下支え）。
- **残存課題（次 task への申し送り）**:
  - **task 4 (RegistrationService)**: WebAuthnUser 実装は「pending username を name / display に持ち、
    UUID などから WebAuthnID を生成する」構造体を Service 側に用意する。新規登録では credential が未確定なので
    `WebAuthnCredentials()` は空スライスを返せば良い。追加登録は既存 credential を反映した User を組み立てる。
    `BeginRegistration` の `excludeCredentials` には既存 credential ID を渡す。
  - **task 5 (AuthenticationService)**: `credentialLookup` は `repository.PasskeyCredentialRepository.FindByCredentialID`
    → `UserRepository.FindByID` の順で解決し、`WebAuthnUser`（`WebAuthnID = user_id bytes` / `WebAuthnCredentials`
    に該当 credential 1 件）と `ParsedCredential` を組み立てて返す。ParsedCredential → webauthn.Credential の
    再構築は本 test で示した通り `ID` / `PublicKey` / `Authenticator{AAGUID, SignCount}` を写せば十分（
    AttestationType / Transport は validation では不要）。成功後は `UpdateSignCount(ctx, cred.ID,
    updatedSignCount, now())` を呼ぶ。
  - **user.WebAuthnID() と userHandle の整合**: 認証応答の userHandle は go-webauthn の内部で
    `user.WebAuthnID()` と `bytes.Equal` 比較される（validateLogin Step 2）。task 5 は WebAuthnUser 実装の
    WebAuthnID を「credentials テーブル参照時に判明する user_id を byte 列化した値」に統一する必要がある。
    task 4 の RegistrationService も同じ user handle を UUID から生成し、credentials 保存時に userHandle を
    永続化することを検討する（現行 model.PasskeyCredential は user_id を持つが user_handle 生値は持たない。
    UUID を直接 handle として使うか、UUID bytes を base64url 化するかは task 4/5 の実装判断）。

### Task 4

- **採用方針**: `internal/passkey/registration_service.go` に `RegistrationService` を追加し、
  新規登録 (Begin/FinishRegistrationNew) と追加登録 (Begin/FinishAddCredential) の 4 メソッドを
  設計通り実装した。依存はすべて最小 interface（`WebAuthnAdapter` 既存 / `challengeStore`
  内部宣言 / `UserWriter` / `PasskeyCredentialWriter`）として受け、テスト（22 サブテスト）は
  全依存を stub 化した table-driven 形式で外部ネットワーク非依存に主要ケース（正常系・
  異常系・境界値）を検証した（NFR 4.1）。
- **重要な判断**:
  - **user handle と UUID→bytes 方式（task 3 申し送り解消）**: WebAuthnID は
    **`[]byte(uuidString)`（36 バイト = UUID の文字列を byte 列化）** で統一した。新規登録の
    Begin では `uuid.New().String()` で仮 UUID を発行し、Finish 時に users.id として
    そのまま採用する（CreateUserOnly の `u.ID = 仮UUID`）。これにより task 5 の
    credentialLookup が `[]byte(users.id)` を WebAuthnID として使う実装と bytes.Equal で
    整合する。追加登録は既存 `users.id` をそのまま `[]byte(u.ID)` として使う。
  - **仮 UUID の challenge への保存経路**: model.PasskeyChallenge を変更禁止のため、
    `challenge.UserID` フィールドに仮 UUID を保存する形で Finish 時に復元可能にした
    （design.md では registration_new は UserID=nil を想定しているが、Finish で
    WebAuthnUser を再構築するために UserID を保存する運用差分。実装判断として選択）。
  - **codeChallenge 引数の追加**: 上記「確認事項」節に記載の通り、design.md L596-598 の
    シグネチャに対し tasks.md L95 と design API 契約 L707 を優先し、`BeginRegistrationNew`
    に `codeChallenge string` 引数を追加。`auth.ValidatePKCES256(codeChallenge, "S256")` で
    early validation のみ実施し、PKCE の永続化・後段継承は task 5 の AuthenticationService
    側に委譲した（本 task では no-op）。
  - **エラー正規化**: username 形式不正→`ErrInvalidUsername`（handler で 400
    INVALID_USERNAME）、username 重複→`ErrUsernameTaken`（409）、それ以外の登録拒否
    （PKCE 形式不正 / challenge 期限切れ = `ErrChallengeNotUsable` / attestation 失敗
    / `repository.ErrCredentialAlreadyRegistered` / `repository.ErrUsernameTaken` の
    Finish 段階 race / userID mismatch / user 未存在）はすべて `ErrRegistrationFailed`
    に uniform 化（Req 1.7 / 3.6 / 3.7）。DB 障害等の infra エラーは
    `fmt.Errorf("...: %w", err)` で wrap して伝播（handler で 500）。拒否ログは
    `slog.Warn` + `challenge_id_prefix`（先頭 8 文字）のみで、平文 challenge / attestation
    / requestBody / username 生値は出さない（NFR 1.2 / 3.2）。
  - **追加登録の Req 3.6 二段防衛**: `FindByCredentialID` による pre-check（attestation
    検証**後**に他 user 既登録を明示的に拒否）と `Create` の UNIQUE 衝突 race による最終
    防衛線（`repository.ErrCredentialAlreadyRegistered` → `ErrRegistrationFailed`）の
    2 段で担保。attestation 検証後 pre-check にした理由は、requestBody 内の credential.rawId は
    parse 前は取り出せないため。
  - **依存の interface segregation**: `challengeStore` interface を service ファイル内で
    `Issue` / `Consume` の 2 メソッドに絞って宣言。具体型 `*passkey.ChallengeStore` が
    構造的にこれを充足するため wiring 時にそのまま渡せ、テストでは stub を差し込める
    （CLAUDE.md §5 準拠）。
- **残存課題（task 5 への申し送り）**:
  - **PKCE 継承**: task 5 の AuthenticationService は begin で codeChallenge を受け取り、
    session に載せて finish 時に AuthCode.PKCEChallenge へ設定する（tasks.md L135-152）。
    task 4 側では PKCE を permanent 保存しないため、認証と登録の PKCE 経路は完全に
    独立する（登録直後に別途認証 begin/finish を呼ぶ想定）。
  - **credentialLookup 実装**: task 5 は `FindByCredentialID` → `FindByID` の順で解決し、
    WebAuthnUser の `WebAuthnID = []byte(users.id)` として組み立てること。本 task の
    実装（追加登録の WebAuthnUser 組み立てと同じパターン）を参照。
  - **user 表示名の扱い**: 追加登録の `registrationUser.name / displayName` は
    `displayNameFor(u)`（Username → Name → Email → users.id の順で fallback）で確定。
    task 5 の WebAuthnUser 組み立ても同 helper を再利用するか、独自の short helper を
    切るかは task 5 の判断。表示名は認証判定には関わらないため、非空であればよい。

### Task 5

- **採用方針**: `internal/passkey/authentication_service.go` に `AuthenticationService` を追加
  し、`BeginAuthentication(ctx, codeChallenge)` / `FinishAuthentication(ctx, requestBody,
  challengeID)` の 2 メソッドで discoverable ログイン ceremony を担う。成功時は既存 native auth
  の `GenerateAuthCode` / `HashNativeSecret` / `NativeAuthCodeTTL` / `AuthCodeCreator` を流用して
  auth_code を発行し、既存 token 交換 endpoint に合流する（NFR 2.1）。依存はすべて最小
  interface（`WebAuthnAdapter` 既存 / `challengeStore` 既存 unexported / `PasskeyCredentialReader`
  新規 2 メソッド / `UserReader` 新規 1 メソッド / `auth.AuthCodeCreator` 既存流用）で受ける
  （CLAUDE.md §5）。テストは全依存 stub 化の table-driven 15 subtests。
- **重要な判断**:
  - **PKCE 継承のための authnSession 封筒方式**: `model.PasskeyChallenge` と `WebAuthnAdapter` は
    task 1〜3 で確定・変更不可のため、authentication kind の `SessionData` のみを
    `{"webauthn_session": <inner bytes>, "code_challenge": <pkce>}` の JSON 封筒で包む方式を
    採用（上記「確認事項」参照）。inner の webauthn.SessionData はそのまま無改変で `FinishLogin`
    に渡せ、`ChallengeStore` はバイト列を opaque に扱うため契約破壊なし。registration kind
    （task 4）は素の SessionData を保存しており本封筒化は authentication kind に局所化される。
  - **`generateAuthCode` → `GenerateAuthCode` の rename**: passkey パッケージから参照するため
    exported にした（tasks.md L159-160）。既存 usage は `HandleNativeCallback` の 1 箇所のみで、
    実装内容・生成規則は不変（NFR 2.1）。auth.GenerateAuthCode / HashNativeSecret /
    NativeAuthCodeTTL / AuthCodeCreator を流用することで、パスキー由来 auth_code は既存
    `POST /api/auth/token` で無変更に受理される（Req 2.4 の合流を達成）。
  - **authnUser の SignCount 反映（NFR 1.4）**: credentialLookup が組み立てる WebAuthnUser の
    `WebAuthnCredentials()` に `webauthn.Credential{ Authenticator: webauthn.Authenticator{
    SignCount: cred.SignCount } }` を含める必要がある。これは task 4 の `registrationUser` が
    creds に SignCount を詰めていない（登録段階では counter が未確定）のと対照的で、library の
    `ValidateDiscoverableLogin` が assertion Counter と stored SignCount を比較して CloneWarning
    を判定するため authentication では必須。authentication 専用の `authnUser` 型を別途用意し、
    display name は既存 `displayNameFor` helper を再利用した。
  - **credentialLookup 内で解決した credential をキャプチャ**: `FinishLogin` 成功後に
    `UpdateSignCount(ctx, cred.ID, updatedSignCount, s.now())` を呼ぶ必要があるため、lookup
    クロージャで解決した `*model.PasskeyCredential` を外側変数にキャプチャする（adapter 側の
    resolvedUser キャプチャと同型パターン）。cred.ID（PK / UUID）を UpdateSignCount の対象に
    することで cred.CredentialID（raw bytes）に依存しない更新経路が組める。
  - **拒否の uniform 化と infra エラーの区別**: PKCE 検証失敗 / challenge 期限切れ / credential
    未検出 / user 未検出 / assertion 不正 / counter 後退はすべて `ErrAuthenticationFailed` に
    uniform 化（Req 2.5 / 2.6）。一方、ChallengeStore.Consume の DB 障害 / adapter.BeginLogin の
    library 内部エラー / AuthCodeCreator.Create の DB 障害 / UpdateSignCount の infra エラーは
    `fmt.Errorf("...: %w", err)` で wrap して伝播（handler で 500）。テストで `errors.Is` に
    よる区別を担保した。
  - **ログ衛生（NFR 1.2 / 3.2）**: 拒否ログは `slog.Warn` + `challenge_id_prefix`（`shortID` の
    先頭 8 文字。registration_service.go の package-level helper を再利用）のみ。成功ログは
    `slog.Info` + `user_id` + `auth_code_hash` 先頭 8 文字（既存 `HandleNativeCallback` と同流儀）。
    平文 assertion / requestBody / challenge / auth_code は一切ログに出さない。
- **残存課題（task 6 への申し送り）**:
  - **HTTP handler 側の endpoint 命名**: task 6 では `POST /api/passkey/authentication/begin`
    が JSON body に `code_challenge`（S256 base64url 43 文字）を要求する契約になる。
    `PasskeyHandler` は `dec.DisallowUnknownFields()`（既存 NativeAuthHandler 流儀）で
    受け取り `AuthenticationService.BeginAuthentication(ctx, codeChallenge)` に渡す。
    `POST /api/passkey/authentication/finish` は `challenge_id` と `assertion_body` を受け
    取って `FinishAuthentication(ctx, requestBody, challengeID)` を呼ぶ設計（引数順は
    design.md L669-671 に合わせて `(ctx, requestBody, challengeID)`）。
  - **エラーマッピング**: `ErrAuthenticationFailed` → 400 `AUTHENTICATION_FAILED`
    （tasks.md L176）。infra エラー wrap（`%w`）は handler 側で 500 `INTERNAL_ERROR`。
    APIError 生成関数は `model.NewAuthenticationFailedError` を task 6 で追加する。
  - **PKCE の共有点**: `model.AuthCode.PKCEChallenge` は既存 native auth と同じフィールド。
    既存 `POST /api/auth/token` の `VerifyPKCES256Verifier(codeVerifier, stored.PKCEChallenge)`
    が無変更でパスキー由来 auth_code の PKCE 検証に使える（Req 2.4 完全合流 / NFR 2.3）。
    iOS クライアントは begin 時に生成した `code_verifier` を token 交換時に送信するだけで良い。
  - **wiring（task 7 への申し送り）**: `passkey.NewAuthenticationService(webAuthnAdapter,
    challengeStore, passkeyCredentialRepo, userRepo, authCodeRepo, nil)` の順で組み、
    `authCodeRepo` は既存 native auth の wiring と共用する（tasks.md L237-238）。now=nil で
    time.Now が既定採用される。

### Task 6

- **採用方針**: `internal/handler/passkey_handler.go` に `PasskeyHandler`（6 endpoint:
  RegistrationBegin/Finish, RegistrationAddBegin/Finish, AuthenticationBegin/Finish）を、
  `internal/handler/aasa_handler.go` に `AASAHandler`（`/.well-known/apple-app-site-association`）
  を新規追加し、`internal/handler/router.go` の `RouterDeps` に両 handler フィールドを
  追加。認証不要グループに AASA + Passkey 未認証 4 route（既存 `unauthIPMW` +
  `NewMaxBodyBytesMiddleware(DefaultMaxBodyBytes)` を通過）、認証必須グループに
  Passkey 追加登録 2 route を登録。既存 `NativeAuthHandler` / `MetricsHandler` と
  同じ fail-closed nil 縮退パターンを踏襲した（NFR 2.2）。エラー APIError 生成関数
  4 種（`NewInvalidUsernameError` / `NewUsernameTakenError` / `NewRegistrationFailedError`
  / `NewAuthenticationFailedError`）を `internal/model/errors.go` に追加し、既存
  `NewFeedNotFoundError` 系の書式に厳密に揃えた。
- **重要な判断**:
  - **サービス依存の受け方（interface segregation / CLAUDE.md §5）**: `PasskeyHandler` は
    `PasskeyRegistrationService`（4 method）/ `PasskeyAuthenticationService`（2 method）
    という **handler 内 unexported ではなく exported な最小 interface** を宣言する形で
    service に依存する。`*passkey.RegistrationService` / `*passkey.AuthenticationService`
    は構造的にこれを充足し、wiring 時にそのまま渡せる。テストでは stub を差し込み外部
    ネットワーク非依存で検証（NFR 4.1）。`NativeAuthHandler` の `TokenExchangeService`
    と同型のパターンで、handler 側の adapter は不要と判断した（後述 PasskeyServiceAdapter
    スキップ理由を参照）。
  - **PasskeyServiceAdapter を作らなかった理由（tasks.md L189-190 との差分 / 実装判断）**:
    tasks.md は `service_adapter.go` に `PasskeyServiceAdapter` を追加するよう指示するが、
    「必要最小の変換のみ」という但し書きに従うと、passkey service は既に primitive 値
    （string / []byte）を返すため、handler 側で追加の domain→DTO 変換は不要（options は
    `json.RawMessage` として pass-through、challenge_id / user_id / auth_code は string の
    ままレスポンス DTO に詰めるだけ）。既存 `NativeAuthHandler` も同型のパターンで
    adapter を持たない（`TokenExchangeService` interface + `*auth.TokenService` を直接渡す）。
    投機的抽象化（CLAUDE.md §7 のアンチパターン）を避けるため、adapter は作成せず interface
    直渡しとした。design.md L189-190 との軽微な差分だが、機能等価。task 7 の wiring では
    `handler.NewPasskeyHandler(registrationSvc, authenticationSvc)` として直接注入すればよい。
  - **リクエスト DTO 設計**: begin 系レスポンスは `passkeyBeginResponse{ChallengeID,
    Options json.RawMessage}` に統一し、service が生成した options JSON をそのまま透過
    （navigator.credentials.create / .get に渡せる形式）。finish 系リクエストは
    `registrationFinishRequest{ChallengeID, Credential json.RawMessage}` に統一し、
    credential 部を `[]byte` として service に転送（parse は WebAuthnAdapter 側）。
    `RegistrationFinish` / `RegistrationAddFinish` / `AuthenticationFinish` の 3 endpoint は
    request 構造が同一のため DTO を共有（DRY）。
  - **RegistrationAddBegin の空 body 許容**: design.md L709 の API 契約は `{}` を想定するが、
    Content-Length 0 で送られる可能性を考慮し `decodeOptionalEmpty` helper を導入。
    `io.EOF` は正常として扱い、非空なら DisallowUnknownFields で unknown フィールドを
    拒否する。他 5 endpoint は `decodeRequest`（body 必須の厳格 decode）で処理し、既存
    `NativeAuthHandler` の流儀と揃えた。
  - **認証必須 endpoint の 401 二段防衛**: 追加登録 2 endpoint は BearerOrSession middleware
    が 401 を返す前提だが、context 経由の userID 取得失敗時にも handler 側で 401 を返す
    防衛層を設けた（`middleware.UserIDFromContext` が error を返した場合は
    `http.Error(w, "unauthorized", 401)` で応答。既存 SessionMiddleware / BearerOrSession
    と同一メッセージ）。router 統合テストで middleware 側の 401（Cookie 無し）と handler
    単体テストで context 無しの 401 の双方を検証。
  - **エラーマッピングの uniform 化（Req 1.7 / 2.5 / 2.6 / 3.6 / 3.7 / NFR 1.3）**:
    `passkey.ErrRegistrationFailed` は 400 REGISTRATION_FAILED、`ErrAuthenticationFailed`
    は 400 AUTHENTICATION_FAILED に固定射影し、判別可能な情報を返さない（Req 2.6 の
    存在有無非開示に整合）。DB 障害等の infra エラーは `WriteInternalServerError` の 500
    INTERNAL_ERROR に合流し、`err.Error()` は slog のみに載せてクライアントに反射しない。
    `passkey_handler_test.go` の `TestPasskeyHandler_AuthenticationFinish_UniformRejection`
    で「異なる拒否理由でも応答本文が完全一致する」ことをテーブル駆動で検証。
  - **AASAHandler の pre-marshal / immutable body**: 生成時に一度だけ `json.Marshal` を
    実行し、以降は同一 byte 列を書き出す。`TestAASAHandler_Serve_ImmutableBody` で
    複数リクエスト間の body 一致を検証。iOS App ID 空なら `NewAASAHandler` が nil を
    返し、router 側で nil handler を検出して route 登録を skip（fail-closed / NFR 2.2）。
    Passkey Handler と AASA Handler は独立に nil 判定されるため、`WEBAUTHN_IOS_APP_ID`
    だけ未設定の環境でも Passkey の 6 endpoint は動作する（
    `TestNewRouter_Passkey_AASA_IndependentFailClose` で網羅）。
  - **共通 helper 名の衝突回避**: `internal/handler` package には既存の
    `invalidRequestError`（`NativeAuthHandler` 専用の action 文言）があるため、passkey 系
    は `passkeyInvalidRequestError` として新規定義した（action 文言を汎用化）。同様に
    `decodeRequest` / `decodeOptionalEmpty` / `writeJSON` / `writeRegistrationError` /
    `writeAuthenticationError` の 5 helper を passkey_handler.go 内に追加。package
    内の既存 helper とは名前衝突しない。
  - **既存テストへの影響**: 既存 router / handler の統合テストは全て pass（既存
    `SessionFinder` / `AuthService` / 他 mock を流用した `newPasskeyRouterDeps` により、
    passkey 追加後も既存 route 挙動が変わらないことが router_test.go の全 subtest で
    暗黙的に担保される）。既存 `TestNewRouter_UnauthIPRateLimit_*` / `TestNewRouter_
    NativeAuthIPRateLimit_*` は無変更。
- **残存課題（task 7 / 8 への申し送り）**:
  - **wiring（task 7）**: `handler.NewPasskeyHandler(registrationSvc, authenticationSvc)`
    と `handler.NewAASAHandler(cfg.WebAuthnIOSAppID)` を `deps.PasskeyHandler` /
    `deps.AASAHandler` に代入する。`NewAASAHandler` は空文字なら nil を返すため空判定は不要。
    `WEBAUTHN_RP_ID` / `WEBAUTHN_ORIGINS` のいずれかが未設定なら Passkey 関連 handler を
    生成せず `deps.PasskeyHandler = nil` のままにする（fail-closed / NFR 2.2）。
  - **RouterDeps 追加フィールド**: `PasskeyHandler *PasskeyHandler` /
    `AASAHandler *AASAHandler`（2 フィールド追加のみ、既存フィールド順序は不変）。
  - **APIError 追加関数**: `model.NewInvalidUsernameError()` /
    `model.NewUsernameTakenError()` / `model.NewRegistrationFailedError()` /
    `model.NewAuthenticationFailedError()` の 4 関数を追加（他 domain と同じ Category
    分類: validation / auth）。エラーコード定数も同 file に追加（`ErrCodeInvalidUsername`
    等 4 種）。
  - **task 8（退会 tx cleanup）に影響なし**: 本 task は handler / router 層に閉じており、
    `internal/user/service.go` の `withdrawTx` や `internal/app/withdraw_wiring.go` に
    触れていない。task 8 は tasks.md L253-263 の順序通り拡張すればよい。
