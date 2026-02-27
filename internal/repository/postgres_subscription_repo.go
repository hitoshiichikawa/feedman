package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresSubscriptionRepo はPostgreSQLを使用した購読リポジトリ。
type PostgresSubscriptionRepo struct {
	db *sql.DB
}

// NewPostgresSubscriptionRepo はPostgresSubscriptionRepoを生成する。
func NewPostgresSubscriptionRepo(db *sql.DB) *PostgresSubscriptionRepo {
	return &PostgresSubscriptionRepo{db: db}
}

// FindByID は指定IDの購読を取得する。見つからない場合はnilを返す。
func (r *PostgresSubscriptionRepo) FindByID(ctx context.Context, id string) (*model.Subscription, error) {
	sub := &model.Subscription{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, feed_id, fetch_interval_minutes, created_at, updated_at
		 FROM subscriptions WHERE id = $1`,
		id,
	).Scan(&sub.ID, &sub.UserID, &sub.FeedID, &sub.FetchIntervalMinutes, &sub.CreatedAt, &sub.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("購読の取得に失敗しました: %w", err)
	}

	return sub, nil
}

// FindByUserAndFeed はユーザーIDとフィードIDで購読を検索する。見つからない場合はnilを返す。
func (r *PostgresSubscriptionRepo) FindByUserAndFeed(ctx context.Context, userID, feedID string) (*model.Subscription, error) {
	sub := &model.Subscription{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, feed_id, fetch_interval_minutes, created_at, updated_at
		 FROM subscriptions WHERE user_id = $1 AND feed_id = $2`,
		userID, feedID,
	).Scan(&sub.ID, &sub.UserID, &sub.FeedID, &sub.FetchIntervalMinutes, &sub.CreatedAt, &sub.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ユーザーとフィードによる購読の検索に失敗しました: %w", err)
	}

	return sub, nil
}

// CountByUserID はユーザーの購読数を返す。
func (r *PostgresSubscriptionRepo) CountByUserID(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE user_id = $1`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("購読数の取得に失敗しました: %w", err)
	}
	return count, nil
}

// Create は購読を作成する。
func (r *PostgresSubscriptionRepo) Create(ctx context.Context, sub *model.Subscription) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subscriptions (id, user_id, feed_id, fetch_interval_minutes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		sub.ID, sub.UserID, sub.FeedID, sub.FetchIntervalMinutes, sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("購読の作成に失敗しました: %w", err)
	}
	return nil
}

// ListByUserID はユーザーの購読一覧を返す。
func (r *PostgresSubscriptionRepo) ListByUserID(ctx context.Context, userID string) ([]*model.Subscription, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, feed_id, fetch_interval_minutes, created_at, updated_at
		 FROM subscriptions WHERE user_id = $1 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("購読一覧の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var subs []*model.Subscription
	for rows.Next() {
		sub := &model.Subscription{}
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.FeedID, &sub.FetchIntervalMinutes, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("購読行の読み取りに失敗しました: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("購読一覧の走査に失敗しました: %w", err)
	}
	return subs, nil
}

// MinFetchIntervalByFeedID は指定フィードの全購読者の中で最小のfetch_interval_minutesを返す。
func (r *PostgresSubscriptionRepo) MinFetchIntervalByFeedID(ctx context.Context, feedID string) (int, error) {
	var minInterval int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(fetch_interval_minutes), 0)
		 FROM subscriptions WHERE feed_id = $1`,
		feedID,
	).Scan(&minInterval)
	if err != nil {
		return 0, fmt.Errorf("最小フェッチ間隔の取得に失敗しました: %w", err)
	}
	return minInterval, nil
}

// UpdateFetchInterval は購読のフェッチ間隔を更新する。
func (r *PostgresSubscriptionRepo) UpdateFetchInterval(ctx context.Context, id string, minutes int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE subscriptions SET fetch_interval_minutes = $2, updated_at = NOW() WHERE id = $1`,
		id, minutes,
	)
	if err != nil {
		return fmt.Errorf("フェッチ間隔の更新に失敗しました: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("更新結果の取得に失敗しました: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("購読が見つかりません: %s", id)
	}
	return nil
}

// Delete は指定IDの購読を削除する。
func (r *PostgresSubscriptionRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("購読の削除に失敗しました: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("削除結果の取得に失敗しました: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("購読が見つかりません: %s", id)
	}
	return nil
}

// DeleteByUserID はユーザーの全購読を削除する。
func (r *PostgresSubscriptionRepo) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("ユーザーの全購読の削除に失敗しました: %w", err)
	}
	return nil
}

// ListByUserIDWithFeedInfo はユーザーの購読一覧をフィード情報と未読数付きで返す。
// feeds, items, item_statesとJOINして、フィードタイトル、favicon、フェッチステータス、未読数を取得する。
func (r *PostgresSubscriptionRepo) ListByUserIDWithFeedInfo(ctx context.Context, userID string) ([]SubscriptionWithFeedInfo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT
			s.id, s.user_id, s.feed_id, s.fetch_interval_minutes, s.created_at, s.updated_at,
			f.title, f.feed_url, f.favicon_data, f.favicon_mime, f.fetch_status, COALESCE(f.error_message, ''),
			COALESCE(unread.cnt, 0)
		 FROM subscriptions s
		 JOIN feeds f ON s.feed_id = f.id
		 LEFT JOIN (
		     SELECT i.feed_id, COUNT(*) AS cnt
		     FROM items i
		     LEFT JOIN item_states ist ON ist.item_id = i.id AND ist.user_id = $1
		     WHERE i.feed_id IN (SELECT feed_id FROM subscriptions WHERE user_id = $1)
		       AND (ist.is_read IS NULL OR ist.is_read = false)
		     GROUP BY i.feed_id
		 ) unread ON unread.feed_id = s.feed_id
		 WHERE s.user_id = $1
		 ORDER BY s.created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("購読一覧（フィード情報付き）の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var results []SubscriptionWithFeedInfo
	for rows.Next() {
		var info SubscriptionWithFeedInfo
		if err := rows.Scan(
			&info.ID, &info.UserID, &info.FeedID, &info.FetchIntervalMinutes, &info.CreatedAt, &info.UpdatedAt,
			&info.FeedTitle, &info.FeedURL, &info.FaviconData, &info.FaviconMime, &info.FetchStatus, &info.ErrorMessage,
			&info.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("購読行（フィード情報付き）の読み取りに失敗しました: %w", err)
		}
		results = append(results, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("購読一覧（フィード情報付き）の走査に失敗しました: %w", err)
	}
	return results, nil
}

// compile-time interface check
var _ SubscriptionRepository = (*PostgresSubscriptionRepo)(nil)
