package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// ListStarredByUser は指定ユーザーがスター付与した記事を全フィード横断・published_at降順で取得する。
// items と item_states と feeds を INNER JOIN し、feed_title を付与する。
// cursor がゼロ値の場合は先頭から取得する。
// SQL 形状は既存 idx_item_states_user_starred (user_id, is_starred) WHERE is_starred = true
// 部分インデックスを利用可能（NFR 1.1 / NFR 1.2）。
func (r *PostgresItemRepo) ListStarredByUser(
	ctx context.Context,
	userID string,
	cursor time.Time,
	limit int,
) ([]StarredItemRow, error) {
	// ベースクエリ: items INNER JOIN item_states INNER JOIN feeds
	// INNER JOIN を採用（スター付き = item_states 行存在が前提なので LEFT JOIN は不要）。
	// f.title AS feed_title を SELECT に含める（Requirement 2.4 / 4.10）。
	baseQuery := `
		SELECT i.id, i.feed_id, i.guid_or_id, i.title, i.link, i.summary, i.author,
		       i.published_at, i.is_date_estimated, i.fetched_at,
		       i.hatebu_count, i.created_at, i.updated_at,
		       COALESCE(s.is_read, false) AS is_read,
		       true AS is_starred,
		       f.title AS feed_title
		FROM items i
		INNER JOIN item_states s ON i.id = s.item_id
		INNER JOIN feeds f ON i.feed_id = f.id
		WHERE s.user_id = $1
		  AND s.is_starred = true`

	args := []interface{}{userID}
	argIndex := 2

	// カーソルベースページネーション
	if !cursor.IsZero() {
		baseQuery += fmt.Sprintf(" AND i.published_at < $%d", argIndex)
		args = append(args, cursor)
		argIndex++
	}

	// ソートとリミット（既存 ListByFeed と同じ published_at DESC）
	baseQuery += fmt.Sprintf(" ORDER BY i.published_at DESC LIMIT $%d", argIndex)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("スター記事一覧の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var items []StarredItemRow
	for rows.Next() {
		var row StarredItemRow
		var publishedAt sql.NullTime
		var guidOrID, link, summary, author sql.NullString

		if err := rows.Scan(
			&row.ID, &row.FeedID, &guidOrID, &row.Title, &link,
			&summary, &author,
			&publishedAt, &row.IsDateEstimated, &row.FetchedAt,
			&row.HatebuCount, &row.CreatedAt, &row.UpdatedAt,
			&row.IsRead, &row.IsStarred,
			&row.FeedTitle,
		); err != nil {
			return nil, fmt.Errorf("スター記事行の読み取りに失敗しました: %w", err)
		}

		row.GuidOrID = nullStringValue(guidOrID)
		row.Link = nullStringValue(link)
		row.Summary = nullStringValue(summary)
		row.Author = nullStringValue(author)
		if publishedAt.Valid {
			row.PublishedAt = &publishedAt.Time
		}

		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("スター記事一覧の走査に失敗しました: %w", err)
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

// itemSelectColumns は records 取得時に共通利用するカラム列。
const itemSelectColumns = `id, feed_id, guid_or_id, title, link, content, summary, author,
	published_at, is_date_estimated, fetched_at, content_hash,
	hatebu_count, hatebu_fetched_at, created_at, updated_at`

// scanItem は items テーブルの 1 行を model.Item にスキャンする。
// itemSelectColumns の列順に対応する。
func scanItem(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.Item, error) {
	item := &model.Item{}
	var publishedAt sql.NullTime
	var hatebuFetchedAt sql.NullTime
	var guidOrID, link, content, summary, author, contentHash sql.NullString

	if err := scanner.Scan(
		&item.ID, &item.FeedID, &guidOrID, &item.Title, &link,
		&content, &summary, &author,
		&publishedAt, &item.IsDateEstimated, &item.FetchedAt, &contentHash,
		&item.HatebuCount, &hatebuFetchedAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
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

// FindExistingForUpsert は同一性判定に必要な既存記事を一括取得する。
// guids / links / hashes それぞれを 1 回ずつ（合計最大 3 回）のバッチ SELECT で引くため、
// DB 往復回数は記事件数に比例しない定数オーダーになる。
func (r *PostgresItemRepo) FindExistingForUpsert(
	ctx context.Context,
	feedID string,
	guids, links, hashes []string,
) (*ExistingItems, error) {
	result := &ExistingItems{
		ByGUID:        make(map[string]*model.Item),
		ByLink:        make(map[string]*model.Item),
		ByContentHash: make(map[string]*model.Item),
	}

	if err := r.queryItemsByColumn(ctx, feedID, "guid_or_id", guids, result.ByGUID, func(i *model.Item) string { return i.GuidOrID }); err != nil {
		return nil, fmt.Errorf("GUID による既存記事の一括取得に失敗しました: %w", err)
	}
	if err := r.queryItemsByColumn(ctx, feedID, "link", links, result.ByLink, func(i *model.Item) string { return i.Link }); err != nil {
		return nil, fmt.Errorf("link による既存記事の一括取得に失敗しました: %w", err)
	}
	if err := r.queryItemsByColumn(ctx, feedID, "content_hash", hashes, result.ByContentHash, func(i *model.Item) string { return i.ContentHash }); err != nil {
		return nil, fmt.Errorf("content_hash による既存記事の一括取得に失敗しました: %w", err)
	}

	return result, nil
}

// queryItemsByColumn は feed_id と指定カラムの IN 句で既存記事をまとめて取得し、keyFn で索引する。
// values が空のときは DB へアクセスしない。
func (r *PostgresItemRepo) queryItemsByColumn(
	ctx context.Context,
	feedID, column string,
	values []string,
	dest map[string]*model.Item,
	keyFn func(*model.Item) string,
) error {
	if len(values) == 0 {
		return nil
	}

	args := make([]interface{}, 0, len(values)+1)
	args = append(args, feedID)
	placeholders := make([]string, len(values))
	for i, v := range values {
		args = append(args, v)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}

	query := fmt.Sprintf(
		`SELECT %s FROM items WHERE feed_id = $1 AND %s IN (%s)`,
		itemSelectColumns, column, strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			return scanErr
		}
		key := keyFn(item)
		// 同一キーに複数行が該当する場合は先頭行を採用（既存の単一取得と整合）。
		if _, exists := dest[key]; !exists {
			dest[key] = item
		}
	}
	return rows.Err()
}

// BulkUpsert は新規記事の一括 INSERT と既存記事の一括 UPDATE を単一トランザクションで実行する。
// 途中でエラーが発生した場合はトランザクションをロールバックし、当該バッチを 1 件も永続化しない。
func (r *PostgresItemRepo) BulkUpsert(ctx context.Context, toCreate, toUpdate []*model.Item) error {
	if len(toCreate) == 0 && len(toUpdate) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("バルク UPSERT のトランザクション開始に失敗しました: %w", err)
	}
	defer tx.Rollback()

	if err := bulkInsertItems(ctx, tx, toCreate); err != nil {
		return fmt.Errorf("記事の一括挿入に失敗しました: %w", err)
	}
	if err := bulkUpdateItems(ctx, tx, toUpdate); err != nil {
		return fmt.Errorf("記事の一括更新に失敗しました: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("バルク UPSERT のコミットに失敗しました: %w", err)
	}
	return nil
}

// bulkInsertItems は複数記事を 1 回の INSERT（複数行 VALUES）で挿入する。
func bulkInsertItems(ctx context.Context, tx *sql.Tx, items []*model.Item) error {
	if len(items) == 0 {
		return nil
	}

	const colsPerRow = 16
	args := make([]interface{}, 0, len(items)*colsPerRow)
	rowClauses := make([]string, len(items))
	for i, item := range items {
		base := i * colsPerRow
		ph := make([]string, colsPerRow)
		for j := 0; j < colsPerRow; j++ {
			ph[j] = fmt.Sprintf("$%d", base+j+1)
		}
		rowClauses[i] = "(" + strings.Join(ph, ", ") + ")"
		args = append(args,
			item.ID, item.FeedID, nullString(item.GuidOrID), item.Title,
			nullString(item.Link), nullString(item.Content), nullString(item.Summary),
			nullString(item.Author), item.PublishedAt, item.IsDateEstimated, item.FetchedAt,
			nullString(item.ContentHash), item.HatebuCount, item.HatebuFetchedAt,
			item.CreatedAt, item.UpdatedAt,
		)
	}

	query := `INSERT INTO items (id, feed_id, guid_or_id, title, link, content, summary, author,
		published_at, is_date_estimated, fetched_at, content_hash,
		hatebu_count, hatebu_fetched_at, created_at, updated_at)
		VALUES ` + strings.Join(rowClauses, ", ")

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
}

// bulkUpdateItems は複数記事を 1 回の UPDATE（VALUES 由来の派生テーブルと JOIN）で更新する。
// 更新カラムは既存の Update と同一（guid_or_id / title / link / content / summary /
// author / published_at / is_date_estimated / content_hash / updated_at）。
func bulkUpdateItems(ctx context.Context, tx *sql.Tx, items []*model.Item) error {
	if len(items) == 0 {
		return nil
	}

	const colsPerRow = 11
	args := make([]interface{}, 0, len(items)*colsPerRow)
	rowClauses := make([]string, len(items))
	for i, item := range items {
		base := i * colsPerRow
		// 型を明示するため id::uuid 等のキャストは VALUES の最初の行で行わず、
		// UPDATE 側の比較で items.id（uuid）と v.id（text）を ::text 比較する方針を取る。
		rowClauses[i] = fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6,
			base+7, base+8, base+9, base+10, base+11,
		)
		args = append(args,
			item.ID, nullString(item.GuidOrID), item.Title, nullString(item.Link),
			nullString(item.Content), nullString(item.Summary), nullString(item.Author),
			item.PublishedAt, item.IsDateEstimated, nullString(item.ContentHash),
			item.UpdatedAt,
		)
	}

	// VALUES 由来の派生テーブルではプレースホルダの型推論が NULL 値で失敗し得るため、
	// SELECT で各カラムに明示キャストを付与してから JOIN する。
	query := `UPDATE items SET
		guid_or_id = v.guid_or_id,
		title = v.title,
		link = v.link,
		content = v.content,
		summary = v.summary,
		author = v.author,
		published_at = v.published_at,
		is_date_estimated = v.is_date_estimated,
		content_hash = v.content_hash,
		updated_at = v.updated_at
	FROM (
		SELECT
			t.id::uuid AS id,
			t.guid_or_id::text AS guid_or_id,
			t.title::text AS title,
			t.link::text AS link,
			t.content::text AS content,
			t.summary::text AS summary,
			t.author::text AS author,
			t.published_at::timestamptz AS published_at,
			t.is_date_estimated::boolean AS is_date_estimated,
			t.content_hash::text AS content_hash,
			t.updated_at::timestamptz AS updated_at
		FROM (VALUES ` + strings.Join(rowClauses, ", ") + `) AS t(id, guid_or_id, title, link, content, summary, author, published_at, is_date_estimated, content_hash, updated_at)
	) AS v
	WHERE items.id = v.id`

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
}

// compile-time interface check
var _ ItemRepository = (*PostgresItemRepo)(nil)
var _ HatebuItemRepository = (*PostgresItemRepo)(nil)
