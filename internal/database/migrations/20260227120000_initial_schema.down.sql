-- 初期スキーマのロールバック: 全テーブルの削除
-- 外部キーの依存関係を考慮した順序で削除

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS item_states;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS feeds;
DROP TABLE IF EXISTS identities;
DROP TABLE IF EXISTS users;
