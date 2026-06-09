# Implementation Plan

- [x] 1. Migration: native auth 用 3 テーブルの up/down を追加
  - `internal/database/migrations/<timestamp>_add_native_auth_tables.up.sql` を新規作成し、
    `auth_codes` / `refresh_token_families` / `refresh_tokens` を design.md の Physical Data
    Model どおりに作成（カラム / 型 / UNIQUE 制約 / INDEX / `users.id` への FK
    `ON DELETE CASCADE` / `refresh_tokens.family_id` への FK `ON DELETE CASCADE`）
  - 対称な `.down.sql` を作成し、`refresh_tokens` → `refresh_token_families` → `auth_codes`
    の順で `DROP TABLE IF EXISTS` する（既存 `sessions` / `users` 等は触れない）
  - timestamp は既存 migration の昇順を維持する値（`20260227120000` 系の後）で確定
  - `internal/database/migrate_test.go` が新 migration を含めた状態で up/down/up が成功する
    ことを目視確認（既存テストの拡張は本タスクでは不要）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

- [x] 2. Model: AuthCode / RefreshTokenFamily / RefreshToken の struct を追加
- [x] 2.1 `internal/model/auth_code.go` を新規作成 (P)
  - `model.AuthCode` を design.md の Struct Sketch どおりに定義（`CodeHash` / `UserID` /
    `PKCEChallenge` / `ExpiresAt` / `Used` / `CreatedAt`、平文 `Code` フィールドは持たない）
  - doc comment は「平文 code は保持しない」旨を明記（NFR 1.1）
  - _Requirements: 2.1, 2.2, 2.3, 2.5_
  - _Boundary: model.AuthCode_

- [x] 2.2 `internal/model/refresh_token.go` を新規作成 (P)
  - `model.RefreshTokenFamily` と `model.RefreshToken` を design.md の Struct Sketch どおりに
    定義（`RotatedAt` / `RevokedAt` は `*time.Time`、平文 `Token` フィールドは持たない）
  - doc comment に rotation/revocation の semantics を明記
  - _Requirements: 3.1, 3.2_
  - _Boundary: model.RefreshTokenFamily, model.RefreshToken_

- [x] 3. Repository interface と sentinel error を `interfaces.go` に追加
  - `internal/repository/interfaces.go` に `AuthCodeRepository` interface（`Create` /
    `FindByHash` / `MarkUsed`）と `RefreshTokenRepository` interface（`CreateFamily` /
    `CreateToken` / `FindByHash` / `MarkRotated` / `RevokeFamily` / `DeleteByUserID`）を
    追加（design.md の Service Interface セクションのシグネチャに準拠）
  - sentinel error `ErrAuthCodeNotUsable` / `ErrRefreshTokenAlreadyRotated` を同パッケージに
    `var ErrXxx = errors.New(...)` 形式で export（メッセージに機密値を含めない、NFR 1.2）
  - 既存 interface 群（`UserRepository` 等）には触れない
  - _Requirements: 2.4, 2.5, 2.6, 3.3, 3.4, 3.5, 3.6, 4.1, 4.2, 4.3_
  - _Depends: 2.1, 2.2_

- [x] 4. PostgresAuthCodeRepo の実装と DB 結合テスト
  - `internal/repository/postgres_auth_code_repo.go` を新規作成し、`AuthCodeRepository` の
    全メソッドを実装（`*sql.DB` field + `NewPostgresAuthCodeRepo` + `var _ AuthCodeRepository
    = (*PostgresAuthCodeRepo)(nil)`）
  - `MarkUsed` は `UPDATE ... WHERE id = $1 AND used = false AND expires_at > now()` で
    `RowsAffected = 0` のとき `ErrAuthCodeNotUsable` を返す（race 安全）
  - `FindByHash` は `(nil, nil)` for not-found（既存パターン整合）
  - エラー wrap は `fmt.Errorf("...: %w", err)`、message に hash や平文を含めない
  - `internal/repository/postgres_auth_code_repo_db_test.go` を新規作成し、design.md の
    Testing Strategy の AuthCodeRepo 5 ケース（正常 / not-found / 単回利用境界 / 期限切れ
    境界 / users 削除時 cascade）を実装。`TEST_DATABASE_URL` 未接続時は `t.Skip`
  - _Requirements: 2.1, 2.4, 2.5, 2.6, 2.7, 4.4, NFR 1.1, NFR 1.2, NFR 3.1, NFR 3.2_
  - _Depends: 1, 3_

- [x] 5. PostgresRefreshTokenRepo の実装と DB 結合テスト
  - `internal/repository/postgres_refresh_token_repo.go` を新規作成し、`RefreshTokenRepository`
    の全メソッドを実装（compile-time check 含む）
  - `MarkRotated` は `UPDATE ... WHERE id = $1 AND rotated_at IS NULL` で `RowsAffected = 0`
    のとき `ErrRefreshTokenAlreadyRotated` を返す
  - `RevokeFamily` は `refresh_token_families.revoked_at` set と `refresh_tokens.revoked_at`
    set の 2 UPDATE を実行（二重 revoke 冪等）
  - `DeleteByUserID` は `DELETE FROM refresh_token_families WHERE user_id = $1`
    （`refresh_tokens` は FK cascade で削除）
  - `internal/repository/postgres_refresh_token_repo_db_test.go` を新規作成し、design.md の
    Testing Strategy の RefreshTokenRepo 6 ケース（CRUD 正常 / rotation 二度目失敗 /
    family revoke / 二重 revoke 冪等 / DeleteByUserID 範囲 / not-found）を実装
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 4.4, NFR 1.1, NFR 1.2, NFR 3.1, NFR 3.2_
  - _Depends: 1, 3_

- [x] 6. セキュリティ回帰テストと interface compile-time check の集約
  - `internal/repository/postgres_auth_code_repo_db_test.go` または `postgres_refresh_token_repo_db_test.go`
    の中で、平文 `"plain-code-xxx"` / `"plain-token-xxx"` で `SELECT` しても 0 件が返ることを
    確認するセキュリティ回帰ケースを 1 件追加（NFR 1.1 の自動検出）
  - `interface 満足` の compile-time check を `tx_test.go` 同様の集約箇所
    （新規 `*_test.go` でも可）に追加: `TestPostgresAuthCodeRepo_ImplementsInterface` /
    `TestPostgresRefreshTokenRepo_ImplementsInterface`
  - sentinel error が `errors.Is` で互いに区別されることを確認する unit test を 1 件追加
  - _Requirements: 4.3, NFR 1.1, NFR 1.2_
  - _Depends: 4, 5_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。`go test ./...` で migration を含むテスト用 PostgreSQL 結合
テストは `TEST_DATABASE_URL` 未接続時に `t.Skip` され、unit テスト + 静的解析のみで
green 判定される（CI でも同条件）。

<!-- stage-a-verify -->
```sh
go test ./... && go vet ./...
```
