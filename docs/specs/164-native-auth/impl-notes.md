# Implementation Notes

## Implementation Notes

### Task 1

- 採用方針: design.md の Physical Data Model に厳密に従い `auth_codes` / `refresh_token_families` / `refresh_tokens` の 3 テーブルを 1 つの migration（`20260610120000_add_native_auth_tables`）で追加した。
- 重要な判断:
  - timestamp は最新既存 migration（`20260528130000_add_user_cross_feed_views`）より後で、本日（2026-06-10 UTC）を反映した `20260610120000` を採用。design.md 例示の `20260609120000` は「PR 作成時に確定する暫定値」と解した。
  - `code_hash` / `token_hash` には `UNIQUE` 列制約を直接付与し、`UNIQUE INDEX` を別建てせずに自動生成インデックス（`auth_codes_code_hash_key` / `refresh_tokens_token_hash_key`）に統一した（design.md の "UNIQUE (code_hash)" 要件を満たし、SQL を簡潔に保つため）。
  - down は FK 依存順を尊重して `refresh_tokens` → `refresh_token_families` → `auth_codes` の順で `DROP TABLE IF EXISTS` を発行。既存 `sessions` / `users` 等は一切触れていない（NFR 2.1）。
- 残存課題: 既存 `internal/database/migrate_test.go` の `setupTestDB` の `cleanupSQL` は新規 3 テーブルを drop 対象に含めていない。fresh DB（CI / docker compose 再生成）では問題ないが、開発機で同一 DB を使い回して `go test` を繰り返す場合、`schema_migrations` 削除後の再 up で `CREATE TABLE auth_codes` が「already exists」で落ちる可能性がある。本 task のスコープでは「既存テストの拡張は不要」と明記されているため見送ったが、Task 4 / Task 5 で `*_db_test.go` を追加する段階で開発機での運用性を見て対応要否を判断する想定。

### Task 2

- 採用方針: design.md §model.AuthCode / §model.RefreshTokenFamily / §model.RefreshToken の Struct Sketch を 1 文字単位で踏襲し、`internal/model/auth_code.go`（AuthCode）と `internal/model/refresh_token.go`（Family + Token を 1 ファイル）に分割配置した。
- 重要な判断:
  - 平文フィールド（`Code` / `Token`）は一切定義せず、doc comment で NFR 1.1（平文を保持しない）を明示。後続 Task 3 の repository interface / Task 4・5 の Postgres 実装でも本方針が前提となる。
  - パッケージコメント `// Package model はドメインモデルを定義する。` は既存 `user.go` 先頭で 1 度だけ宣言される慣習に従い、新規ファイル先頭は型の doc comment から開始した（Go の慣習）。
  - `RotatedAt` / `RevokedAt` は `*time.Time`（pointer）で NULL を表現し、SQL の `TIMESTAMPTZ NULL` 列とそのまま対応させる。`RefreshTokenFamily.UserID` を冗長に保持するのは design.md §Physical Data Model の意図（DeleteByUserID 経路を family JOIN なしで簡潔化）に整合させるため。
- 残存課題: なし。Task 3 で `interfaces.go` に追加する `AuthCodeRepository` / `RefreshTokenRepository` の引数・戻り値型はここで追加した struct を直接参照すれば足りる。

## 確認事項

（現時点で人間判断を仰ぐ事項はなし）
