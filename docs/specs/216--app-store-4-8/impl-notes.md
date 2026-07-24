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
