package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresItemRepo はPostgreSQLを使用した記事リポジトリ。
type PostgresItemRepo struct {
	db *sql.DB
}

// NewPostgresItemRepo はPostgresItemRepoを生成する。
func NewPostgresItemRepo(db *sql.DB) *PostgresItemRepo {
	return &PostgresItemRepo{db: db}
}

// FindByID は指定IDの記事を取得する。見つからない場合はnilを返す。
func (r *PostgresItemRepo) FindByID(ctx context.Context, id string) (*model.Item, error) {
	item := &model.Item{}
	var publishedAt sql.NullTime
	var hatebuFetchedAt sql.NullTime
	var guidOrID, link, content, summary, author, contentHash sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, feed_id, guid_or_id, title, link, content, summary, author,
		        published_at, is_date_estimated, fetched_at, content_hash,
		        hatebu_count, hatebu_fetched_at, created_at, updated_at
		 FROM items WHERE id = $1`,
		id,
	).Scan(
		&item.ID, &item.FeedID, &guidOrID, &item.Title, &link,
		&content, &summary, &author,
		&publishedAt, &item.IsDateEstimated, &item.FetchedAt, &contentHash,
		&item.HatebuCount, &hatebuFetchedAt, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("記事の取得に失敗しました: %w", err)
	}

	item.GuidOrID = nullStringValue(guidOrID)
	item.Link = nullStringValue(link)
	item.Content = nullStringValue(content)
	item.Summary = nullStringValue(summary)
	item.Author = nullStringValue(author)
	item.ContentHash = nullStringValue(contentHash)
	if publishedAt.Valid {
		item.PublishedAt = &publishedAt.Time
	}
	if hatebuFetchedAt.Valid {
		item.HatebuFetchedAt = &hatebuFetchedAt.Time
	}

	return item, nil
}

// FindByFeedAndGUID はfeed_idとguid_or_idで記事を検索する。
func (r *PostgresItemRepo) FindByFeedAndGUID(ctx context.Context, feedID, guid string) (*model.Item, error) {
	item := &model.Item{}
	var publishedAt sql.NullTime
	var hatebuFetchedAt sql.NullTime
	var guidOrID, link, content, summary, author, contentHash sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, feed_id, guid_or_id, title, link, content, summary, author,
		        published_at, is_date_estimated, fetched_at, content_hash,
		        hatebu_count, hatebu_fetched_at, created_at, updated_at
		 FROM items WHERE feed_id = $1 AND guid_or_id = $2`,
		feedID, guid,
	).Scan(
		&item.ID, &item.FeedID, &guidOrID, &item.Title, &link,
		&content, &summary, &author,
		&publishedAt, &item.IsDateEstimated, &item.FetchedAt, &contentHash,
		&item.HatebuCount, &hatebuFetchedAt, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GUID による記事の検索に失敗しました: %w", err)
	}

	item.GuidOrID = nullStringValue(guidOrID)
	item.Link = nullStringValue(link)
	item.Content = nullStringValue(content)
	item.Summary = nullStringValue(summary)
	item.Author = nullStringValue(author)
	item.ContentHash = nullStringValue(contentHash)
	if publishedAt.Valid {
		item.PublishedAt = &publishedAt.Time
	}
	if hatebuFetchedAt.Valid {
		item.HatebuFetchedAt = &hatebuFetchedAt.Time
	}

	return item, nil
}

// FindByFeedAndLink はfeed_idとlinkで記事を検索する。
func (r *PostgresItemRepo) FindByFeedAndLink(ctx context.Context, feedID, link string) (*model.Item, error) {
	item := &model.Item{}
	var publishedAt sql.NullTime
	var hatebuFetchedAt sql.NullTime
	var guidOrID, linkVal, content, summary, author, contentHash sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, feed_id, guid_or_id, title, link, content, summary, author,
		        published_at, is_date_estimated, fetched_at, content_hash,
		        hatebu_count, hatebu_fetched_at, created_at, updated_at
		 FROM items WHERE feed_id = $1 AND link = $2`,
		feedID, link,
	).Scan(
		&item.ID, &item.FeedID, &guidOrID, &item.Title, &linkVal,
		&content, &summary, &author,
		&publishedAt, &item.IsDateEstimated, &item.FetchedAt, &contentHash,
		&item.HatebuCount, &hatebuFetchedAt, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("link による記事の検索に失敗しました: %w", err)
	}

	item.GuidOrID = nullStringValue(guidOrID)
	item.Link = nullStringValue(linkVal)
	item.Content = nullStringValue(content)
	item.Summary = nullStringValue(summary)
	item.Author = nullStringValue(author)
	item.ContentHash = nullStringValue(contentHash)
	if publishedAt.Valid {
		item.PublishedAt = &publishedAt.Time
	}
	if hatebuFetchedAt.Valid {
		item.HatebuFetchedAt = &hatebuFetchedAt.Time
	}

	return item, nil
}

// FindByContentHash はfeed_idとcontent_hashで記事を検索する。
func (r *PostgresItemRepo) FindByContentHash(ctx context.Context, feedID, contentHash string) (*model.Item, error) {
	item := &model.Item{}
	var publishedAt sql.NullTime
	var hatebuFetchedAt sql.NullTime
	var guidOrID, link, content, summary, author, contentHashVal sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, feed_id, guid_or_id, title, link, content, summary, author,
		        published_at, is_date_estimated, fetched_at, content_hash,
		        hatebu_count, hatebu_fetched_at, created_at, updated_at
		 FROM items WHERE feed_id = $1 AND content_hash = $2`,
		feedID, contentHash,
	).Scan(
		&item.ID, &item.FeedID, &guidOrID, &item.Title, &link,
		&content, &summary, &author,
		&publishedAt, &item.IsDateEstimated, &item.FetchedAt, &contentHashVal,
		&item.HatebuCount, &hatebuFetchedAt, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("content_hash による記事の検索に失敗しました: %w", err)
	}

	item.GuidOrID = nullStringValue(guidOrID)
	item.Link = nullStringValue(link)
	item.Content = nullStringValue(content)
	item.Summary = nullStringValue(summary)
	item.Author = nullStringValue(author)
	item.ContentHash = nullStringValue(contentHashVal)
	if publishedAt.Valid {
		item.PublishedAt = &publishedAt.Time
	}
	if hatebuFetchedAt.Valid {
		item.HatebuFetchedAt = &hatebuFetchedAt.Time
	}

	return item, nil
}

// ListByFeed はフィードの記事一覧をユーザーの状態とJOINして取得する。
// published_at降順でカーソルベースページネーションを使用する。
// cursorがゼロ値の場合は先頭から取得する。
// filter: "all"=全件, "unread"=未読のみ, "starred"=スターのみ
func (r *PostgresItemRepo) ListByFeed(
	ctx context.Context,
	feedID, userID string,
	filter model.ItemFilter,
	cursor time.Time,
	limit int,
) ([]model.ItemWithState, error) {
	// ベースクエリ: items LEFT JOIN item_states
	baseQuery := `
		SELECT i.id, i.feed_id, i.guid_or_id, i.title, i.link, i.summary, i.author,
		       i.published_at, i.is_date_estimated, i.fetched_at,
		       i.hatebu_count, i.created_at, i.updated_at,
		       COALESCE(s.is_read, false) AS is_read,
		       COALESCE(s.is_starred, false) AS is_starred
		FROM items i
		LEFT JOIN item_states s ON i.id = s.item_id AND s.user_id = $1
		WHERE i.feed_id = $2`

	args := []interface{}{userID, feedID}
	argIndex := 3

	// カーソルベースページネーション
	if !cursor.IsZero() {
		baseQuery += fmt.Sprintf(" AND i.published_at < $%d", argIndex)
		args = append(args, cursor)
		argIndex++
	}

	// フィルタ条件
	switch filter {
	case model.ItemFilterUnread:
		// 未読: item_statesにレコードがない、またはis_read=false
		baseQuery += " AND COALESCE(s.is_read, false) = false"
	case model.ItemFilterStarred:
		// スター付き: is_starred=true
		baseQuery += " AND COALESCE(s.is_starred, false) = true"
	case model.ItemFilterAll:
		// 全件: 追加条件なし
	}

	// ソートとリミット
	baseQuery += fmt.Sprintf(" ORDER BY i.published_at DESC LIMIT $%d", argIndex)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("記事一覧の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var items []model.ItemWithState
	for rows.Next() {
		var iws model.ItemWithState
		var publishedAt sql.NullTime
		var guidOrID, link, summary, author sql.NullString

		if err := rows.Scan(
			&iws.ID, &iws.FeedID, &guidOrID, &iws.Title, &link,
			&summary, &author,
			&publishedAt, &iws.IsDateEstimated, &iws.FetchedAt,
			&iws.HatebuCount, &iws.CreatedAt, &iws.UpdatedAt,
			&iws.IsRead, &iws.IsStarred,
		); err != nil {
			return nil, fmt.Errorf("記事行の読み取りに失敗しました: %w", err)
		}

		iws.GuidOrID = nullStringValue(guidOrID)
		iws.Link = nullStringValue(link)
		iws.Summary = nullStringValue(summary)
		iws.Author = nullStringValue(author)
		if publishedAt.Valid {
			iws.PublishedAt = &publishedAt.Time
		}

		items = append(items, iws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("記事一覧の走査に失敗しました: %w", err)
	}

	return items, nil
}

// Create は新規記事を作成する。
func (r *PostgresItemRepo) Create(ctx context.Context, item *model.Item) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO items (id, feed_id, guid_or_id, title, link, content, summary, author,
		                    published_at, is_date_estimated, fetched_at, content_hash,
		                    hatebu_count, hatebu_fetched_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		item.ID, item.FeedID, nullString(item.GuidOrID), item.Title,
		nullString(item.Link), nullString(item.Content), nullString(item.Summary),
		nullString(item.Author), item.PublishedAt, item.IsDateEstimated, item.FetchedAt,
		nullString(item.ContentHash), item.HatebuCount, item.HatebuFetchedAt,
		item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("記事の作成に失敗しました: %w", err)
	}
	return nil
}

// Update は既存記事を上書き更新する。履歴は保持しない。
func (r *PostgresItemRepo) Update(ctx context.Context, item *model.Item) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE items SET
		    guid_or_id = $2, title = $3, link = $4, content = $5,
		    summary = $6, author = $7, published_at = $8,
		    is_date_estimated = $9, content_hash = $10, updated_at = $11
		 WHERE id = $1`,
		item.ID, nullString(item.GuidOrID), item.Title, nullString(item.Link),
		nullString(item.Content), nullString(item.Summary), nullString(item.Author),
		item.PublishedAt, item.IsDateEstimated, nullString(item.ContentHash),
		item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("記事の更新に失敗しました: %w", err)
	}
	return nil
}

// ListNeedingHatebuFetch ははてなブックマーク数の取得が必要な記事を取得する。
// hatebu_fetched_at IS NULL（未取得）を優先し、次にhatebu_fetched_atが古い順に処理する。
func (r *PostgresItemRepo) ListNeedingHatebuFetch(ctx context.Context, limit int) ([]*model.Item, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, feed_id, guid_or_id, title, link, content, summary, author,
		        published_at, is_date_estimated, fetched_at, content_hash,
		        hatebu_count, hatebu_fetched_at, created_at, updated_at
		 FROM items
		 WHERE hatebu_fetched_at IS NULL
		    OR hatebu_fetched_at < now() - interval '24 hours'
		 ORDER BY
		    CASE WHEN hatebu_fetched_at IS NULL THEN 0 ELSE 1 END,
		    hatebu_fetched_at ASC NULLS FIRST
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("はてブ取得対象記事の一覧取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var items []*model.Item
	for rows.Next() {
		item := &model.Item{}
		var publishedAt sql.NullTime
		var hatebuFetchedAt sql.NullTime
		var guidOrID, link, content, summary, author, contentHash sql.NullString

		if err := rows.Scan(
			&item.ID, &item.FeedID, &guidOrID, &item.Title, &link,
			&content, &summary, &author,
			&publishedAt, &item.IsDateEstimated, &item.FetchedAt, &contentHash,
			&item.HatebuCount, &hatebuFetchedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("はてブ取得対象記事の行読み取りに失敗しました: %w", err)
		}

		item.GuidOrID = nullStringValue(guidOrID)
		item.Link = nullStringValue(link)
		item.Content = nullStringValue(content)
		item.Summary = nullStringValue(summary)
		item.Author = nullStringValue(author)
		item.ContentHash = nullStringValue(contentHash)
		if publishedAt.Valid {
			item.PublishedAt = &publishedAt.Time
		}
		if hatebuFetchedAt.Valid {
			item.HatebuFetchedAt = &hatebuFetchedAt.Time
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("はてブ取得対象記事の走査に失敗しました: %w", err)
	}

	return items, nil
}

// UpdateHatebuCount は記事のはてなブックマーク数と取得日時を更新する。
func (r *PostgresItemRepo) UpdateHatebuCount(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE items SET hatebu_count = $2, hatebu_fetched_at = $3, updated_at = now()
		 WHERE id = $1`,
		itemID, count, fetchedAt,
	)
	if err != nil {
		return fmt.Errorf("はてなブックマーク数の更新に失敗しました: %w", err)
	}
	return nil
}

// compile-time interface check
var _ ItemRepository = (*PostgresItemRepo)(nil)
var _ HatebuItemRepository = (*PostgresItemRepo)(nil)
