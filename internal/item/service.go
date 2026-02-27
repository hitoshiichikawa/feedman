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
	PublishedAt     time.Time
	IsDateEstimated bool
	IsRead          bool
	IsStarred       bool
	HatebuCount     int
}

// validFilters は有効なフィルタ値のセット。
var validFilters = map[model.ItemFilter]bool{
	model.ItemFilterAll:     true,
	model.ItemFilterUnread:  true,
	model.ItemFilterStarred: true,
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
	var cursor time.Time
	if cursorStr != "" {
		var err error
		cursor, err = time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			// RFC3339でもパースを試みる
			cursor, err = time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				return nil, model.NewInvalidFilterError("無効なカーソル値: " + cursorStr)
			}
		}
	}

	// limit+1件を取得してHasMoreを判定する
	fetchLimit := limit + 1
	items, err := s.itemRepo.ListByFeed(ctx, feedID, userID, filter, cursor, fetchLimit)
	if err != nil {
		return nil, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit] // 余分な1件を除外
	}

	// 結果をサマリーに変換
	summaries := make([]ItemSummary, len(items))
	var nextCursor string
	for i, item := range items {
		pubAt := time.Time{}
		if item.PublishedAt != nil {
			pubAt = *item.PublishedAt
		}
		summaries[i] = ItemSummary{
			ID:              item.ID,
			FeedID:          item.FeedID,
			Title:           item.Title,
			Link:            item.Link,
			PublishedAt:     pubAt,
			IsDateEstimated: item.IsDateEstimated,
			IsRead:          item.IsRead,
			IsStarred:       item.IsStarred,
			HatebuCount:     item.HatebuCount,
		}
	}

	// HasMoreの場合、最後の記事のpublished_atをNextCursorに設定
	if hasMore && len(summaries) > 0 {
		lastItem := summaries[len(summaries)-1]
		nextCursor = lastItem.PublishedAt.Format(time.RFC3339Nano)
	}

	return &ItemListResult{
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
