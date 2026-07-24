-- パスキー（WebAuthn）認証（Issue #216）向けに 2 テーブル追加と users への 2 カラム追加を行う。
-- 既存 Google OAuth / Cookie session / native auth 用スキーマは変更しない（NFR 2.1 / 2.2）。
-- 保存対象は検証に必要な情報（公開鍵・credential 識別子・カウンタ値等）のみで、
-- パスキー本体の秘密情報および生 challenge 値は保存しない（NFR 1.1 / 1.2）。

-- ============================================================
-- users テーブル拡張
-- 新規パスキーアカウントの一意性判定用に username / username_normalized カラムを追加する。
-- 既存 Google 由来ユーザーは NULL のまま（部分 UNIQUE により NULL 同士は重複扱いにならない）。
-- ============================================================
ALTER TABLE users
    ADD COLUMN username VARCHAR(64),
    ADD COLUMN username_normalized VARCHAR(64);

CREATE UNIQUE INDEX idx_users_username_normalized ON users(username_normalized)
    WHERE username_normalized IS NOT NULL;

-- ============================================================
-- passkey_credentials テーブル
-- WebAuthn credential（公開鍵・credential_id・sign counter 等）を永続化する。
-- 1 user に複数 credential 可（同一 user_id 複数行）、credential_id は全体で一意。
-- FK CASCADE により退会時の防衛線を確保する。
-- ============================================================
CREATE TABLE passkey_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    public_key BYTEA NOT NULL,
    sign_count BIGINT NOT NULL DEFAULT 0,
    attestation_type VARCHAR(32) NOT NULL DEFAULT '',
    aaguid BYTEA,
    transports VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_passkey_credentials_user_id ON passkey_credentials(user_id);

-- ============================================================
-- passkey_challenges テーブル
-- 登録 / 追加登録 / 認証の 3 種 challenge を統一的に保存する。
-- 生 challenge 値は保存せず SHA-256 hex（challenge_hash）のみを保持する（NFR 1.2）。
-- consumed への遷移は atomic UPDATE で単回利用を保証する（Req 4.3）。
-- ============================================================
CREATE TABLE passkey_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_hash VARCHAR(128) NOT NULL UNIQUE,
    kind VARCHAR(32) NOT NULL,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    pending_username VARCHAR(64),
    session_data BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_passkey_challenges_expires_at ON passkey_challenges(expires_at);
