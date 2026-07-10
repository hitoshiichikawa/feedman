# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-10T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-164-impl-native-auth
- HEAD commit: 64d0b0ca6e252a06c1313d111d9c05beef83c7dd
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `internal/database/migrations/20260610120000_add_native_auth_tables.up.sql` で `CREATE TABLE auth_codes` を追加
- 1.2 — 同 up migration で `CREATE TABLE refresh_token_families` を追加
- 1.3 — 同 up migration で `CREATE TABLE refresh_tokens` を追加（FK `family_id` → `refresh_token_families`、`user_id` → `users` をいずれも `ON DELETE CASCADE`）
- 1.4 — `*_down.sql` が FK 依存順を尊重して `refresh_tokens` → `refresh_token_families` → `auth_codes` の順で `DROP TABLE IF EXISTS` を発行
- 1.5 — up/down migration いずれも `users` / `sessions` 等の既存スキーマには `ALTER`/`DROP`/`CREATE INDEX` を発行していない（diff 確認済）
- 2.1 — `internal/model/auth_code.go` に平文 `Code` フィールド無し、`auth_codes.code_hash` のみ保存。`postgres_auth_code_repo_db_test.go` Case 6 セキュリティ回帰テストで自動検出
- 2.2 — `model.AuthCode` に `UserID` / `PKCEChallenge` / `ExpiresAt` / `Used` を保持、`PostgresAuthCodeRepo.Create` で全て INSERT
- 2.3 — `AuthCodeRepository.Create(ctx, code *model.AuthCode)` のシグネチャで `ExpiresAt` を呼び出し側から受領（repository 内では計算しない）
- 2.4 — `PostgresAuthCodeRepo.FindByHash` で 1 件返却 / not-found は `(nil, nil)`。Case 1, Case 2 で検証
- 2.5 — `MarkUsed` で `used = true` に遷移、`FindByHash` の `Used` 確認まで Case 3 で検証
- 2.6 — `MarkUsed` の WHERE `id = $1 AND used = false AND expires_at > now()` で `RowsAffected = 0` 時に `ErrAuthCodeNotUsable` 返却。Case 3 (二度目) / Case 4 (期限切れ境界) で検証
- 2.7 — model 平文無し / error message 固定文言 / Case 6 平文逆引き SELECT 0 件検証
- 3.1 — `internal/model/refresh_token.go` に平文 `Token` 無し、`refresh_tokens.token_hash` のみ。`postgres_refresh_token_repo_db_test.go` Case 7 セキュリティ回帰で検証
- 3.2 — `model.RefreshToken` / `RefreshTokenFamily` で `FamilyID` / `UserID` / `ExpiresAt` / `RotatedAt` / `RevokedAt` を保持、`CreateFamily` / `CreateToken` で INSERT
- 3.3 — `MarkRotated` で `rotated_at` set、`FindByHash` の `RotatedAt` で識別可能。Case 2 で 1 回目成功 + 2 回目 `ErrRefreshTokenAlreadyRotated` を検証
- 3.4 — `RevokeFamily` が `refresh_token_families.revoked_at` と配下全 token の `revoked_at` を set。Case 3 で検証
- 3.5 — `RefreshTokenRepository.FindByHash` で 1 件 / not-found は `(nil, nil)`。Case 1, Case 6 で検証
- 3.6 — `DeleteByUserID` で family DELETE → FK CASCADE で token も削除。Case 5 で user A / user B 分離（他 user 影響なし）まで検証
- 3.7 — model 平文無し / error message 固定文言 / Case 7 平文逆引き SELECT 0 件検証
- 4.1 — `interfaces.go` に `AuthCodeRepository` と `RefreshTokenRepository` を別 interface として公開
- 4.2 — 各 interface は本 spec 要件で定めた操作のみ（AuthCode: Create/FindByHash/MarkUsed、RefreshToken: CreateFamily/CreateToken/FindByHash/MarkRotated/RevokeFamily/DeleteByUserID）。余分な method なし
- 4.3 — interface 公開により mock 差し替え可能。`tx_test.go` 末尾に `TestPostgresAuthCodeRepo_ImplementsInterface` / `TestPostgresRefreshTokenRepo_ImplementsInterface` を集約。`errors_test.go` の `TestSentinelErrors_AreDistinct` で sentinel error 判別を検証
- 4.4 — 結合テスト（AuthCode 5 ケース + 1 セキュリティ + RefreshToken 6 ケース + 1 セキュリティ）で create / find / mark-used / rotation / family-revoke / user-delete を網羅
- NFR 1.1 — `code_hash` / `token_hash` のみ保存、Case 6 / Case 7 で平文逆引き 0 件検証
- NFR 1.2 — repository 実装の全 error message が固定文言（"failed to create auth_code: %w" 等）で hash / id / user_id を含まない。`errors_test.go` Case 4 で sentinel 文言固定検証
- NFR 1.3 — SHA-256 hash 値の一致比較のみ（`WHERE code_hash = $1`）、復号化処理なし
- NFR 2.1 — 既存 migration / `postgres_session_repo.go` / 既存 interface 群への変更なし（diff 確認済。`tx_test.go` の末尾追記は新 interface 用の compile-time check 集約であり既存テスト挙動を変えない）
- NFR 2.2 — 既存 Cookie session 用テーブル/コードに変更なし、影響なし
- NFR 3.1 — 結合テストは `TEST_DATABASE_URL` + `t.Skip` パターンで外部 NW 依存なし
- NFR 3.2 — 正常系 + 異常系（not-found / 単回利用境界 / 期限切れ境界 / 二重 revoke 境界 / 他 user 影響なし）を各 AC で網羅

## Boundary 確認

tasks.md の `_Boundary:_` は Task 2.1 / 2.2 (model.AuthCode / model.RefreshTokenFamily, model.RefreshToken) のみ宣言。
変更ファイルは:

- `internal/database/migrations/20260610120000_add_native_auth_tables.{up,down}.sql`（Task 1）
- `internal/model/auth_code.go` / `internal/model/refresh_token.go`（Task 2 — `_Boundary:_` と一致）
- `internal/repository/interfaces.go`（Task 3、既存 interface 群は不変、追加のみ）
- `internal/repository/postgres_auth_code_repo.go` + `_db_test.go`（Task 4）
- `internal/repository/postgres_refresh_token_repo.go` + `_db_test.go`（Task 5）
- `internal/repository/errors_test.go`（新規・Task 6）/ `internal/repository/tx_test.go`（末尾追記・Task 6）
- `docs/specs/164-native-auth/{tasks,impl-notes}.md`（進捗追跡・learning 追記）

いずれも本 spec の File Structure Plan に列挙された範囲内。`tx_test.go` への末尾追記は
Task 6 の指示「`tx_test.go` 同様の集約箇所にも追加」に明示準拠した interface drift guard で、
既存テスト関数の改変はなく、`SessionRepository` 等の既存 interface コードにも一切手を入れていない。
境界逸脱はなし。

## Feature Flag Protocol 確認

CLAUDE.md の `## Feature Flag Protocol` 節は `**採否**: opt-out` のため、flag 観点の細目チェックは
適用しない（通常の 3 カテゴリ判定のみ）。

## Findings

なし

## Summary

Tasks 1〜6 の全実装が完了し、requirements.md の全 numeric ID（1.1〜4.4 / NFR 1.1〜3.2）が
実装またはテストで verifiable な状態にある。境界は tasks.md の `_Boundary:_` および
design.md の File Structure Plan に準拠し、既存 Cookie session 用スキーマ / コードへの
変更は一切なし。平文非保存は struct field 不在 + 平文逆引き SELECT 0 件回帰の 2 重防御で
担保されており、sentinel error の `errors.Is` 判別と固定文言も unit test で guard されている。

RESULT: approve
