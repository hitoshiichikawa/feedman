-- フィード横断新着一覧（Issue #121）向けに、ユーザーごとの最終閲覧時刻を保持する表を追加する
CREATE TABLE user_cross_feed_views (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_seen_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
