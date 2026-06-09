-- Native auth 永続化レイヤー（Issue #164）向けに 3 テーブルを追加する。
-- 既存 Cookie session 用スキーマ（sessions / users 他）は変更しない（NFR 2.1 / 2.2）。
-- 平文の auth_code / refresh_token は保存しない。code_hash / token_hash のみを保存する（NFR 1.1）。

-- ============================================================
-- auth_codes テーブル
-- OAuth callback で発行する一時 auth_code の hash・PKCE challenge・TTL・single-use を保持
-- ============================================================
CREATE TABLE auth_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash VARCHAR(128) NOT NULL UNIQUE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pkce_challenge VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_codes_user_id ON auth_codes(user_id);
CREATE INDEX idx_auth_codes_expires_at ON auth_codes(expires_at);

-- ============================================================
-- refresh_token_families テーブル
-- 同一 refresh chain（rotation 系列）の束（family）。revoke は family 単位
-- ============================================================
CREATE TABLE refresh_token_families (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_refresh_token_families_user_id ON refresh_token_families(user_id);

-- ============================================================
-- refresh_tokens テーブル
-- rotation 単位の refresh token。token 本体は hash で保存
-- ============================================================
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID NOT NULL REFERENCES refresh_token_families(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    rotated_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
