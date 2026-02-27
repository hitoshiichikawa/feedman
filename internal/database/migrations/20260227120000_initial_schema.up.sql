-- 初期スキーママイグレーション: 全テーブルの作成
-- feedman RSS Reader アプリケーション

-- UUID生成用の拡張機能を有効化
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- users テーブル
-- ============================================================
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- identities テーブル
-- 複数IdP対応のための外部アカウント紐付けテーブル
-- ============================================================
CREATE TABLE identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_identities_provider_user UNIQUE (provider, provider_user_id)
);

CREATE INDEX idx_identities_user_id ON identities(user_id);

-- ============================================================
-- feeds テーブル
-- フィードのメタ情報とフェッチ状態を管理
-- ============================================================
CREATE TABLE feeds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_url TEXT NOT NULL UNIQUE,
    site_url TEXT,
    title VARCHAR(500) NOT NULL,
    favicon_data BYTEA,
    favicon_mime VARCHAR(100),
    etag VARCHAR(500),
    last_modified VARCHAR(500),
    fetch_status VARCHAR(20) NOT NULL DEFAULT 'active',
    consecutive_errors INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    next_fetch_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 部分インデックス: アクティブなフィードのフェッチスケジュール検索用
CREATE INDEX idx_feeds_next_fetch_at_active ON feeds(next_fetch_at) WHERE fetch_status = 'active';

-- ============================================================
-- items テーブル
-- フィードから取得した記事を保持
-- ============================================================
CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_id UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid_or_id VARCHAR(2000),
    link TEXT,
    title VARCHAR(1000) NOT NULL,
    content TEXT,
    summary TEXT,
    author VARCHAR(500),
    published_at TIMESTAMPTZ,
    is_date_estimated BOOLEAN NOT NULL DEFAULT false,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    content_hash VARCHAR(64),
    hatebu_count INTEGER NOT NULL DEFAULT 0,
    hatebu_fetched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 部分ユニーク制約: guid_or_idがnon-NULLの場合のみユニーク
CREATE UNIQUE INDEX idx_items_feed_guid ON items(feed_id, guid_or_id) WHERE guid_or_id IS NOT NULL;

-- 記事の同一性判定用インデックス
CREATE INDEX idx_items_feed_link ON items(feed_id, link);
CREATE INDEX idx_items_feed_content_hash ON items(feed_id, content_hash);

-- 記事一覧のソート・ページネーション用インデックス
CREATE INDEX idx_items_feed_published_at ON items(feed_id, published_at DESC);

-- はてなブックマーク取得対象の検索用部分インデックス
CREATE INDEX idx_items_hatebu_fetched_at ON items(hatebu_fetched_at) WHERE hatebu_fetched_at IS NOT NULL;

-- ============================================================
-- subscriptions テーブル
-- ユーザーとフィードの購読関係を管理
-- ============================================================
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    fetch_interval_minutes INTEGER NOT NULL DEFAULT 60,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_subscriptions_user_feed UNIQUE (user_id, feed_id)
);

CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_feed_id ON subscriptions(feed_id);

-- ============================================================
-- item_states テーブル
-- ユーザーごとの記事の既読・スター状態を管理
-- ============================================================
CREATE TABLE item_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    is_read BOOLEAN NOT NULL DEFAULT false,
    is_starred BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_item_states_user_item UNIQUE (user_id, item_id)
);

-- 部分インデックス: 未読記事のフィルタリング用
CREATE INDEX idx_item_states_user_unread ON item_states(user_id, is_read) WHERE is_read = false;

-- 部分インデックス: スター付き記事のフィルタリング用
CREATE INDEX idx_item_states_user_starred ON item_states(user_id, is_starred) WHERE is_starred = true;

-- ============================================================
-- user_settings テーブル
-- ユーザーごとの設定（テーマなど）を管理
-- ============================================================
CREATE TABLE user_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    theme VARCHAR(20) NOT NULL DEFAULT 'light',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- sessions テーブル
-- HTTP Cookieセッションのサーバーサイド管理
-- ============================================================
CREATE TABLE sessions (
    id VARCHAR(255) PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    data BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
