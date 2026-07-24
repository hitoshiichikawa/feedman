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
