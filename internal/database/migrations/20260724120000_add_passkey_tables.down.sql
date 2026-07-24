-- パスキー（WebAuthn）認証（Issue #216）追加分の rollback。
-- FK 依存関係を尊重して passkey_challenges → passkey_credentials の順で DROP する。
-- 続いて users への追加カラム（username_normalized / username）と部分 UNIQUE INDEX を撤去する。
-- 既存 users 行の他カラム（id / email / name / created_at / updated_at）には触れず、
-- 既存データを不変で復元する（NFR 2.1）。

DROP TABLE IF EXISTS passkey_challenges;
DROP TABLE IF EXISTS passkey_credentials;

DROP INDEX IF EXISTS idx_users_username_normalized;

ALTER TABLE users
    DROP COLUMN IF EXISTS username_normalized,
    DROP COLUMN IF EXISTS username;
