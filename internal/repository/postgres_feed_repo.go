package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresFeedRepo はPostgreSQLを使用したフィードリポジトリ。
type PostgresFeedRepo struct {
	db *sql.DB
}

// NewPostgresFeedRepo はPostgresFeedRepoを生成する。
func NewPostgresFeedRepo(db *sql.DB) *PostgresFeedRepo {
	return &PostgresFeedRepo{db: db}
}

// FindByID は指定IDのフィードを取得する。見つからない場合はnilを返す。
func (r *PostgresFeedRepo) FindByID(ctx context.Context, id string) (*model.Feed, error) {
	feed := &model.Feed{}
	var faviconData []byte
	var faviconMime, siteURL, etag, lastModified, errorMessage sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, feed_url, site_url, title, favicon_data, favicon_mime,
		        etag, last_modified, fetch_status, consecutive_errors,
		        error_message, next_fetch_at, created_at, updated_at
		 FROM feeds WHERE id = $1`,
		id,
	).Scan(
		&feed.ID, &feed.FeedURL, &siteURL, &feed.Title,
		&faviconData, &faviconMime,
		&etag, &lastModified, &feed.FetchStatus, &feed.ConsecutiveErrors,
		&errorMessage, &feed.NextFetchAt, &feed.CreatedAt, &feed.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("フィードの取得に失敗しました: %w", err)
	}

	feed.FaviconData = faviconData
	feed.FaviconMime = nullStringValue(faviconMime)
	feed.SiteURL = nullStringValue(siteURL)
	feed.ETag = nullStringValue(etag)
	feed.LastModified = nullStringValue(lastModified)
	feed.ErrorMessage = nullStringValue(errorMessage)

	return feed, nil
}

// FindByFeedURL はフィードURLでフィードを検索する。見つからない場合はnilを返す。
func (r *PostgresFeedRepo) FindByFeedURL(ctx context.Context, feedURL string) (*model.Feed, error) {
	feed := &model.Feed{}
	var faviconData []byte
	var faviconMime, siteURL, etag, lastModified, errorMessage sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, feed_url, site_url, title, favicon_data, favicon_mime,
		        etag, last_modified, fetch_status, consecutive_errors,
		        error_message, next_fetch_at, created_at, updated_at
		 FROM feeds WHERE feed_url = $1`,
		feedURL,
	).Scan(
		&feed.ID, &feed.FeedURL, &siteURL, &feed.Title,
		&faviconData, &faviconMime,
		&etag, &lastModified, &feed.FetchStatus, &feed.ConsecutiveErrors,
		&errorMessage, &feed.NextFetchAt, &feed.CreatedAt, &feed.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("フィードURLによるフィードの検索に失敗しました: %w", err)
	}

	feed.FaviconData = faviconData
	feed.FaviconMime = nullStringValue(faviconMime)
	feed.SiteURL = nullStringValue(siteURL)
	feed.ETag = nullStringValue(etag)
	feed.LastModified = nullStringValue(lastModified)
	feed.ErrorMessage = nullStringValue(errorMessage)

	return feed, nil
}

// Create はフィードを作成する。
func (r *PostgresFeedRepo) Create(ctx context.Context, feed *model.Feed) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO feeds (id, feed_url, site_url, title, favicon_data, favicon_mime,
		                    etag, last_modified, fetch_status, consecutive_errors,
		                    error_message, next_fetch_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		feed.ID, feed.FeedURL, nullString(feed.SiteURL), feed.Title,
		feed.FaviconData, nullString(feed.FaviconMime),
		nullString(feed.ETag), nullString(feed.LastModified),
		feed.FetchStatus, feed.ConsecutiveErrors,
		nullString(feed.ErrorMessage), feed.NextFetchAt,
		feed.CreatedAt, feed.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("フィードの作成に失敗しました: %w", err)
	}
	return nil
}

// Update はフィード情報を更新する。
func (r *PostgresFeedRepo) Update(ctx context.Context, feed *model.Feed) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE feeds SET
		    feed_url = $2, site_url = $3, title = $4,
		    etag = $5, last_modified = $6, fetch_status = $7,
		    consecutive_errors = $8, error_message = $9,
		    next_fetch_at = $10, updated_at = $11
		 WHERE id = $1`,
		feed.ID, feed.FeedURL, nullString(feed.SiteURL), feed.Title,
		nullString(feed.ETag), nullString(feed.LastModified),
		feed.FetchStatus, feed.ConsecutiveErrors,
		nullString(feed.ErrorMessage), feed.NextFetchAt, feed.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("フィードの更新に失敗しました: %w", err)
	}
	return nil
}

// UpdateFavicon はフィードのfaviconデータを更新する。
func (r *PostgresFeedRepo) UpdateFavicon(ctx context.Context, feedID string, faviconData []byte, faviconMime string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE feeds SET favicon_data = $2, favicon_mime = $3, updated_at = now() WHERE id = $1`,
		feedID, faviconData, nullString(faviconMime),
	)
	if err != nil {
		return fmt.Errorf("faviconの更新に失敗しました: %w", err)
	}
	return nil
}

// nullString は空文字列をsql.NullStringに変換する。
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullStringValue はsql.NullStringから文字列を取得する。
func nullStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// ListDueForFetch はフェッチ対象のフィードを取得する。
// next_fetch_at <= now() かつ fetch_status = 'active' かつ購読者が存在するフィードを
// FOR UPDATE SKIP LOCKEDで排他的に取得する。
func (r *PostgresFeedRepo) ListDueForFetch(ctx context.Context) ([]*model.Feed, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT f.id, f.feed_url, f.site_url, f.title, f.favicon_data, f.favicon_mime,
		        f.etag, f.last_modified, f.fetch_status, f.consecutive_errors,
		        f.error_message, f.next_fetch_at, f.created_at, f.updated_at
		 FROM feeds f
		 INNER JOIN subscriptions s ON f.id = s.feed_id
		 WHERE f.next_fetch_at <= now()
		   AND f.fetch_status = 'active'
		 ORDER BY f.next_fetch_at ASC
		 FOR UPDATE OF f SKIP LOCKED`,
	)
	if err != nil {
		return nil, fmt.Errorf("フェッチ対象フィードの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var feeds []*model.Feed
	for rows.Next() {
		feed := &model.Feed{}
		var faviconData []byte
		var faviconMime, siteURL, etag, lastModified, errorMessage sql.NullString

		if err := rows.Scan(
			&feed.ID, &feed.FeedURL, &siteURL, &feed.Title,
			&faviconData, &faviconMime,
			&etag, &lastModified, &feed.FetchStatus, &feed.ConsecutiveErrors,
			&errorMessage, &feed.NextFetchAt, &feed.CreatedAt, &feed.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("フェッチ対象フィードの読み取りに失敗しました: %w", err)
		}

		feed.FaviconData = faviconData
		feed.FaviconMime = nullStringValue(faviconMime)
		feed.SiteURL = nullStringValue(siteURL)
		feed.ETag = nullStringValue(etag)
		feed.LastModified = nullStringValue(lastModified)
		feed.ErrorMessage = nullStringValue(errorMessage)

		feeds = append(feeds, feed)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("フェッチ対象フィードの走査に失敗しました: %w", err)
	}

	return feeds, nil
}

// UpdateFetchState はフィードのフェッチ状態を更新する。
func (r *PostgresFeedRepo) UpdateFetchState(ctx context.Context, feed *model.Feed) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE feeds SET
		    fetch_status = $2,
		    consecutive_errors = $3,
		    error_message = $4,
		    next_fetch_at = $5,
		    etag = $6,
		    last_modified = $7,
		    updated_at = now()
		 WHERE id = $1`,
		feed.ID,
		feed.FetchStatus,
		feed.ConsecutiveErrors,
		nullString(feed.ErrorMessage),
		feed.NextFetchAt,
		nullString(feed.ETag),
		nullString(feed.LastModified),
	)
	if err != nil {
		return fmt.Errorf("フェッチ状態の更新に失敗しました: %w", err)
	}
	return nil
}

// compile-time interface check
var _ FeedRepository = (*PostgresFeedRepo)(nil)
