-- Native auth 永続化レイヤー（Issue #164）の rollback。
-- FK 依存関係を尊重して refresh_tokens → refresh_token_families → auth_codes の順で DROP する。
-- 既存 Cookie session 用スキーマ（sessions / users 他）には触れない（NFR 2.1）。

DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS refresh_token_families;
DROP TABLE IF EXISTS auth_codes;
