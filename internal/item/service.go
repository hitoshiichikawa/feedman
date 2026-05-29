// Package item は記事の管理機能を提供する。
package item

import (
	"context"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// ItemService は記事取得・フィルタリングのサービス。
type ItemService struct {
	itemRepo      repository.ItemRepository
	itemStateRepo repository.ItemStateRepository
}

// NewItemService はItemServiceの新しいインスタンスを生成する。
func NewItemService(
	itemRepo repository.ItemRepository,
	itemStateRepo repository.ItemStateRepository,
) *ItemService {
	return &ItemService{
		itemRepo:      itemRepo,
		itemStateRepo: itemStateRepo,
	}
}

// ItemListResult はListItemsの戻り値。
type ItemListResult struct {
	Items      []ItemSummary
	NextCursor string
	HasMore    bool
}

// ItemSummary は記事一覧のサマリー情報。
type ItemSummary struct {
	ID              string
	FeedID          string
	Title           string
	Link            string
	Summary         string // サニタイズ済みの概要テキスト
	PublishedAt     time.Time
	IsDateEstimated bool
	IsRead          bool
	IsStarred       bool
	HatebuCount     int
}

// StarredItemSummary は全フィード横断スター記事一覧のサマリー情報。
// 既存 ItemSummary に FeedTitle を追加して、フロントエンドが
// どのフィードの記事かを表示できるようにする（Requirement 2.4 / 4.10）。
type StarredItemSummary struct {
	ItemSummary
	// FeedTitle は当該記事が所属するフィードのタイトル（feeds.title）。
	FeedTitle string
}

// StarredItemListResult は ListStarredItems の戻り値。
// 形状は ItemListResult と同形だが、Items の型が StarredItemSummary である点が異なる。
type StarredItemListResult struct {
	Items      []StarredItemSummary
	NextCursor string
	HasMore    bool
}

// validFilters は有効なフィルタ値のセット。
var validFilters = map[model.ItemFilter]bool{
	model.ItemFilterAll:     true,
	model.ItemFilterUnread:  true,
	model.ItemFilterStarred: true,
}

// parseItemCursor は RFC3339Nano → RFC3339 の順でカーソル文字列をパースする。
// 空文字列の場合はゼロ値（先頭ページ取得を意味する）を返す。
// パース不能な場合は model.NewInvalidFilterError を返す。
// 本ヘルパは ListItems / ListStarredItems で共有され、横断 API のカーソル規約を
// 既存単一フィード API と完全に同一に保つ（Requirement 4.5 / 4.8 / NFR 3.1）。
func parseItemCursor(cursorStr string) (time.Time, error) {
	if cursorStr == "" {
		return time.Time{}, nil
	}
	cursor, err := time.Parse(time.RFC3339Nano, cursorStr)
	if err != nil {
		// RFC3339でもパースを試みる
		cursor, err = time.Parse(time.RFC3339, cursorStr)
		if err != nil {
			return time.Time{}, model.NewInvalidFilterError("無効なカーソル値: " + cursorStr)
		}
	}
	return cursor, nil
}

// toItemSummary は model.ItemWithState を ItemSummary に変換する。
// PublishedAt が nil の場合はゼロ値の time.Time を採用する。
func toItemSummary(item model.ItemWithState) ItemSummary {
	pubAt := time.Time{}
	if item.PublishedAt != nil {
		pubAt = *item.PublishedAt
	}
	return ItemSummary{
		ID:              item.ID,
		FeedID:          item.FeedID,
		Title:           item.Title,
		Link:            item.Link,
		Summary:         item.Summary,
		PublishedAt:     pubAt,
		IsDateEstimated: item.IsDateEstimated,
		IsRead:          item.IsRead,
		IsStarred:       item.IsStarred,
		HatebuCount:     item.HatebuCount,
	}
}

// buildItemListResult は limit+1件取得の結果から HasMore 判定・NextCursor 算出・
// サマリー変換を行い ItemListResult を組み立てる。
// items は limit+1 件以下を想定し、items の件数が limit を超える場合に HasMore=true
// として末尾を切り詰める。NextCursor は最後尾の PublishedAt を RFC3339Nano で
// フォーマットしたもの（HasMore=true のときのみ非空）。
func buildItemListResult(items []model.ItemWithState, limit int) *ItemListResult {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit] // 余分な1件を除外
	}

	summaries := make([]ItemSummary, len(items))
	for i, item := range items {
		summaries[i] = toItemSummary(item)
	}

	var nextCursor string
	if hasMore && len(summaries) > 0 {
		lastItem := summaries[len(summaries)-1]
		nextCursor = lastItem.PublishedAt.Format(time.RFC3339Nano)
	}

	return &ItemListResult{
		Items:      summaries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
}

// ListItems はフィードの記事一覧をフィルタ・ページネーション付きで返す。
// カーソルベースページネーションを使用し、published_at降順でソートする。
// limit+1件を取得してHasMoreを判定する。
func (s *ItemService) ListItems(
	ctx context.Context,
	userID, feedID string,
	filter model.ItemFilter,
	cursorStr string,
	limit int,
) (*ItemListResult, error) {
	// フィルタのバリデーション
	if !validFilters[filter] {
		return nil, model.NewInvalidFilterError(string(filter))
	}

	// カーソルのパース
	cursor, err := parseItemCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	// limit+1件を取得してHasMoreを判定する
	fetchLimit := limit + 1
	items, err := s.itemRepo.ListByFeed(ctx, feedID, userID, filter, cursor, fetchLimit)
	if err != nil {
		return nil, err
	}

	return buildItemListResult(items, limit), nil
}

// ListStarredItems はユーザーの全フィード横断スター記事一覧を返す。
// カーソルベースページネーションを使用し、published_at 降順でソートする。
// cursorStr が空文字列の場合は先頭ページを返す。
// 不正な cursorStr は model.NewInvalidFilterError（code: INVALID_FILTER）を返す。
// 戻り値の形状は ItemListResult と同形だが、Items の各要素に FeedTitle を併記する
// （Requirement 2.4 / 4.10）。
func (s *ItemService) ListStarredItems(
	ctx context.Context,
	userID string,
	cursorStr string,
	limit int,
) (*StarredItemListResult, error) {
	// カーソルのパース（既存 ListItems と完全同一の規約 / Requirement 4.5 / 4.8）
	cursor, err := parseItemCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	// limit+1件を取得してHasMoreを判定する（既存 ListItems と同形 / Requirement 4.3 / NFR 3.1）
	fetchLimit := limit + 1
	rows, err := s.itemRepo.ListStarredByUser(ctx, userID, cursor, fetchLimit)
	if err != nil {
		return nil, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit] // 余分な1件を除外
	}

	summaries := make([]StarredItemSummary, len(rows))
	for i, row := range rows {
		summaries[i] = StarredItemSummary{
			ItemSummary: toItemSummary(row.ItemWithState),
			FeedTitle:   row.FeedTitle,
		}
	}

	var nextCursor string
	if hasMore && len(summaries) > 0 {
		lastItem := summaries[len(summaries)-1]
		nextCursor = lastItem.PublishedAt.Format(time.RFC3339Nano)
	}

	return &StarredItemListResult{
		Items:      summaries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// GetItem は記事詳細をユーザーの状態付きで返す。
func (s *ItemService) GetItem(
	ctx context.Context,
	userID, itemID string,
) (*ItemDetail, error) {
	item, err := s.itemRepo.FindByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, model.NewItemNotFoundError(itemID)
	}

	// ユーザーの記事状態を取得
	state, err := s.itemStateRepo.FindByUserAndItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	isRead := false
	isStarred := false
	if state != nil {
		isRead = state.IsRead
		isStarred = state.IsStarred
	}

	pubAt := time.Time{}
	if item.PublishedAt != nil {
		pubAt = *item.PublishedAt
	}

	return &ItemDetail{
		ItemSummary: ItemSummary{
			ID:              item.ID,
			FeedID:          item.FeedID,
			Title:           item.Title,
			Link:            item.Link,
			PublishedAt:     pubAt,
			IsDateEstimated: item.IsDateEstimated,
			IsRead:          isRead,
			IsStarred:       isStarred,
			HatebuCount:     item.HatebuCount,
		},
		Content: item.Content,
		Summary: item.Summary,
		Author:  item.Author,
	}, nil
}

// ItemDetail は記事詳細情報。
type ItemDetail struct {
	ItemSummary
	Content string
	Summary string
	Author  string
}
