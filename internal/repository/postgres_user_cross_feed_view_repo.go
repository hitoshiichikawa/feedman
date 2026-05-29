package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresUserCrossFeedViewRepo は PostgreSQL を使用した UserCrossFeedView リポジトリ。
// ユーザーごとの「最後にフィード横断新着一覧を開いた時刻」を user_cross_feed_views 表で管理する。
type PostgresUserCrossFeedViewRepo struct {
	db *sql.DB
}

// NewPostgresUserCrossFeedViewRepo は PostgresUserCrossFeedViewRepo を生成する。
func NewPostgresUserCrossFeedViewRepo(db *sql.DB) *PostgresUserCrossFeedViewRepo {
	return &PostgresUserCrossFeedViewRepo{db: db}
}

// Get は当該ユーザーの最終閲覧時刻記録を取得する。
// 記録が存在しない場合は (nil, nil) を返す（初回利用ユーザー扱い）。
func (r *PostgresUserCrossFeedViewRepo) Get(ctx context.Context, userID string) (*model.UserCrossFeedView, error) {
	view := &model.UserCrossFeedView{}
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, last_seen_at, updated_at
		 FROM user_cross_feed_views WHERE user_id = $1`,
		userID,
	).Scan(&view.UserID, &view.LastSeenAt, &view.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ユーザー横断新着閲覧時刻の取得に失敗しました: %w", err)
	}

	return view, nil
}

// Upsert は user_id をキーに last_seen_at を冪等に上書き保存する。
// 既存行が無ければ新規挿入、存在すれば last_seen_at と updated_at を更新する。
// updated_at は DB 側で now() を採用し時刻のドリフトを避ける。
func (r *PostgresUserCrossFeedViewRepo) Upsert(ctx context.Context, userID string, lastSeenAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_cross_feed_views (user_id, last_seen_at, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (user_id) DO UPDATE
		   SET last_seen_at = EXCLUDED.last_seen_at,
		       updated_at   = now()`,
		userID, lastSeenAt,
	)
	if err != nil {
		return fmt.Errorf("ユーザー横断新着閲覧時刻のUpsertに失敗しました: %w", err)
	}
	return nil
}

// compile-time interface check
var _ UserCrossFeedViewRepository = (*PostgresUserCrossFeedViewRepo)(nil)
