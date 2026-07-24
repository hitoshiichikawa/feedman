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
