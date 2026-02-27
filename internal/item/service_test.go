package item

import (
	"context"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- テスト用モック（サービス層用） ---

// mockItemRepoForService はサービステスト用のItemRepositoryモック。
type mockItemRepoForService struct {
	*mockItemRepo
	listByFeedFn func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error)
	findByIDFn   func(ctx context.Context, id string) (*model.Item, error)
}

func newMockItemRepoForService() *mockItemRepoForService {
	return &mockItemRepoForService{
		mockItemRepo: newMockItemRepo(),
	}
}

func (m *mockItemRepoForService) ListByFeed(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
	if m.listByFeedFn != nil {
		return m.listByFeedFn(ctx, feedID, userID, filter, cursor, limit)
	}
	return nil, nil
}

func (m *mockItemRepoForService) FindByID(ctx context.Context, id string) (*model.Item, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return m.mockItemRepo.FindByID(ctx, id)
}

// mockItemStateRepoForService はサービステスト用のItemStateRepositoryモック。
type mockItemStateRepoForService struct {
	states     map[string]*model.ItemState // userID+itemID -> state
	upsertFn   func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error)
}

func newMockItemStateRepoForService() *mockItemStateRepoForService {
	return &mockItemStateRepoForService{
		states: make(map[string]*model.ItemState),
	}
}

func (m *mockItemStateRepoForService) FindByUserAndItem(_ context.Context, userID, itemID string) (*model.ItemState, error) {
	key := userID + "|" + itemID
	state, ok := m.states[key]
	if !ok {
		return nil, nil
	}
	return state, nil
}

func (m *mockItemStateRepoForService) Upsert(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, userID, itemID, isRead, isStarred)
	}
	return nil, nil
}

func (m *mockItemStateRepoForService) DeleteByUserAndFeed(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockItemStateRepoForService) DeleteByUserID(_ context.Context, _ string) error {
	return nil
}

// --- ItemService ListItems テスト ---

// TestItemService_ListItems_ReturnsItems はフィードの記事一覧がpublished_at降順で返されることをテストする。
func TestItemService_ListItems_ReturnsItems(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		if feedID != "feed-1" {
			t.Errorf("feedID = %q, want %q", feedID, "feed-1")
		}
		if userID != "user-123" {
			t.Errorf("userID = %q, want %q", userID, "user-123")
		}
		if filter != model.ItemFilterAll {
			t.Errorf("filter = %q, want %q", filter, model.ItemFilterAll)
		}
		if limit != 51 {
			// limit+1で取得して、HasMoreを判定する
			t.Errorf("limit = %d, want 51 (limit+1)", limit)
		}
		return []model.ItemWithState{
			{
				Item: model.Item{
					ID:          "item-1",
					FeedID:      "feed-1",
					Title:       "記事1",
					Link:        "https://example.com/1",
					PublishedAt: &now,
					HatebuCount: 5,
				},
				IsRead:    false,
				IsStarred: true,
			},
		}, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	result, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Errorf("items count = %d, want 1", len(result.Items))
	}
	if result.HasMore {
		t.Error("expected HasMore to be false (1 item returned, limit was 50)")
	}

	item := result.Items[0]
	if item.ID != "item-1" {
		t.Errorf("item.ID = %q, want %q", item.ID, "item-1")
	}
	if item.IsStarred != true {
		t.Error("expected item to be starred")
	}
}

// TestItemService_ListItems_HasMore は50件超の結果でHasMore=trueが返されることをテストする。
func TestItemService_ListItems_HasMore(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		// limit+1件（51件）を返してHasMoreを検証
		items := make([]model.ItemWithState, limit)
		for i := 0; i < limit; i++ {
			pubTime := now.Add(-time.Duration(i) * time.Hour)
			items[i] = model.ItemWithState{
				Item: model.Item{
					ID:          "item-" + string(rune('A'+i)),
					FeedID:      "feed-1",
					Title:       "記事",
					PublishedAt: &pubTime,
				},
			}
		}
		return items, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	result, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if !result.HasMore {
		t.Error("expected HasMore to be true (51 items returned)")
	}

	// 実際に返される記事数はlimit（50件）
	if len(result.Items) != 50 {
		t.Errorf("items count = %d, want 50", len(result.Items))
	}

	if result.NextCursor == "" {
		t.Error("expected NextCursor to be set when HasMore is true")
	}
}

// TestItemService_ListItems_InvalidFilter は無効なフィルタでエラーが返されることをテストする。
func TestItemService_ListItems_InvalidFilter(t *testing.T) {
	repo := newMockItemRepoForService()
	svc := NewItemService(repo, newMockItemStateRepoForService())

	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilter("invalid"), "", 50)
	if err == nil {
		t.Fatal("expected error for invalid filter")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeInvalidFilter {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeInvalidFilter)
	}
}

// TestItemService_ListItems_CursorParsing はカーソル文字列が正しくパースされることをテストする。
func TestItemService_ListItems_CursorParsing(t *testing.T) {
	var receivedCursor time.Time
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedCursor = cursor
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	cursorStr := "2026-02-27T10:00:00Z"
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, cursorStr, 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	expectedCursor, _ := time.Parse(time.RFC3339, cursorStr)
	if !receivedCursor.Equal(expectedCursor) {
		t.Errorf("cursor = %v, want %v", receivedCursor, expectedCursor)
	}
}

// TestItemService_ListItems_EmptyCursor は空カーソルでゼロ値が渡されることをテストする。
func TestItemService_ListItems_EmptyCursor(t *testing.T) {
	var receivedCursor time.Time
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedCursor = cursor
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if !receivedCursor.IsZero() {
		t.Errorf("expected zero cursor, got %v", receivedCursor)
	}
}

// TestItemService_ListItems_UnreadFilter は未読フィルタがリポジトリに渡されることをテストする。
func TestItemService_ListItems_UnreadFilter(t *testing.T) {
	var receivedFilter model.ItemFilter
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedFilter = filter
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterUnread, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if receivedFilter != model.ItemFilterUnread {
		t.Errorf("filter = %q, want %q", receivedFilter, model.ItemFilterUnread)
	}
}

// TestItemService_ListItems_StarredFilter はスターフィルタがリポジトリに渡されることをテストする。
func TestItemService_ListItems_StarredFilter(t *testing.T) {
	var receivedFilter model.ItemFilter
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedFilter = filter
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterStarred, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if receivedFilter != model.ItemFilterStarred {
		t.Errorf("filter = %q, want %q", receivedFilter, model.ItemFilterStarred)
	}
}

// --- ItemService GetItem テスト ---

// TestItemService_GetItem_ReturnsDetail は記事詳細が正しく取得されることをテストする。
func TestItemService_GetItem_ReturnsDetail(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		if id != "item-1" {
			t.Errorf("id = %q, want %q", id, "item-1")
		}
		return &model.Item{
			ID:              "item-1",
			FeedID:          "feed-1",
			Title:           "テスト記事",
			Link:            "https://example.com/article",
			Content:         "<p>サニタイズ済み</p>",
			Summary:         "サマリー",
			Author:          "著者",
			PublishedAt:     &now,
			IsDateEstimated: false,
			HatebuCount:     5,
		}, nil
	}

	stateRepo := newMockItemStateRepoForService()
	stateRepo.states["user-123|item-1"] = &model.ItemState{
		UserID:    "user-123",
		ItemID:    "item-1",
		IsRead:    true,
		IsStarred: true,
	}

	svc := NewItemService(repo, stateRepo)
	detail, err := svc.GetItem(context.Background(), "user-123", "item-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}

	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if detail.ID != "item-1" {
		t.Errorf("detail.ID = %q, want %q", detail.ID, "item-1")
	}
	if detail.Content != "<p>サニタイズ済み</p>" {
		t.Errorf("detail.Content = %q, want sanitized content", detail.Content)
	}
	if detail.Link != "https://example.com/article" {
		t.Errorf("detail.Link = %q, want %q", detail.Link, "https://example.com/article")
	}
	if detail.Author != "著者" {
		t.Errorf("detail.Author = %q, want %q", detail.Author, "著者")
	}
	if !detail.IsRead {
		t.Error("expected detail.IsRead to be true")
	}
	if !detail.IsStarred {
		t.Error("expected detail.IsStarred to be true")
	}
}

// TestItemService_GetItem_NotFound は存在しない記事でエラーが返されることをテストする。
func TestItemService_GetItem_NotFound(t *testing.T) {
	repo := newMockItemRepoForService()
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.GetItem(context.Background(), "user-123", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent item")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeItemNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeItemNotFound)
	}
}

// TestItemService_GetItem_NoState は記事状態が未設定の場合にデフォルト値が返されることをテストする。
func TestItemService_GetItem_NoState(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{
			ID:          "item-1",
			FeedID:      "feed-1",
			Title:       "記事",
			PublishedAt: &now,
		}, nil
	}

	// item_statesにレコードなし
	svc := NewItemService(repo, newMockItemStateRepoForService())
	detail, err := svc.GetItem(context.Background(), "user-123", "item-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}

	// 状態未設定の場合、デフォルトで未読・未スター
	if detail.IsRead {
		t.Error("expected detail.IsRead to be false (no state)")
	}
	if detail.IsStarred {
		t.Error("expected detail.IsStarred to be false (no state)")
	}
}

// --- ItemStateService テスト ---

// TestItemStateService_UpdateState_SetRead は既読状態の設定をテストする。
func TestItemStateService_UpdateState_SetRead(t *testing.T) {
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		if userID != "user-123" {
			t.Errorf("userID = %q, want %q", userID, "user-123")
		}
		if itemID != "item-1" {
			t.Errorf("itemID = %q, want %q", itemID, "item-1")
		}
		if isRead == nil || !*isRead {
			t.Error("expected isRead to be true")
		}
		if isStarred != nil {
			t.Error("expected isStarred to be nil")
		}
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    true,
			IsStarred: false,
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isRead := true
	state, err := svc.UpdateState(context.Background(), "user-123", "item-1", &isRead, nil)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	if !state.IsRead {
		t.Error("expected state.IsRead to be true")
	}
}

// TestItemStateService_UpdateState_SetStarred はスター状態の設定をテストする。
func TestItemStateService_UpdateState_SetStarred(t *testing.T) {
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		if isStarred == nil || !*isStarred {
			t.Error("expected isStarred to be true")
		}
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    false,
			IsStarred: true,
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isStarred := true
	state, err := svc.UpdateState(context.Background(), "user-123", "item-1", nil, &isStarred)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	if !state.IsStarred {
		t.Error("expected state.IsStarred to be true")
	}
}

// TestItemStateService_UpdateState_NilFieldsNotChanged はnilフィールドが変更されないことをテストする。
func TestItemStateService_UpdateState_NilFieldsNotChanged(t *testing.T) {
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		// isReadのみ指定されている
		if isRead == nil {
			t.Error("expected isRead to be non-nil")
		}
		// isStarredはnil（変更しない）
		if isStarred != nil {
			t.Error("expected isStarred to be nil (not changed)")
		}
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    *isRead,
			IsStarred: true, // 既存の値を維持
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isRead := false
	state, err := svc.UpdateState(context.Background(), "user-123", "item-1", &isRead, nil)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	if state.IsRead {
		t.Error("expected state.IsRead to be false")
	}
	if !state.IsStarred {
		t.Error("expected state.IsStarred to remain true (unchanged)")
	}
}

// TestItemStateService_UpdateState_ItemNotFound は存在しない記事でエラーが返されることをテストする。
func TestItemStateService_UpdateState_ItemNotFound(t *testing.T) {
	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return nil, nil // 記事が存在しない
	}

	svc := NewItemStateService(itemRepo, newMockItemStateRepoForService())
	isRead := true
	_, err := svc.UpdateState(context.Background(), "user-123", "nonexistent", &isRead, nil)
	if err == nil {
		t.Fatal("expected error for non-existent item")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeItemNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeItemNotFound)
	}
}

// TestItemStateService_UpdateState_UserDataIsolation はユーザーデータ分離が強制されることをテストする。
func TestItemStateService_UpdateState_UserDataIsolation(t *testing.T) {
	receivedUserID := ""
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		receivedUserID = userID
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    true,
			IsStarred: false,
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isRead := true
	_, err := svc.UpdateState(context.Background(), "user-456", "item-1", &isRead, nil)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	// user_idが正しくリポジトリに渡されることを検証
	if receivedUserID != "user-456" {
		t.Errorf("received userID = %q, want %q", receivedUserID, "user-456")
	}
}
