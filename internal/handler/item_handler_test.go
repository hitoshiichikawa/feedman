package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockItemService はItemServiceInterfaceのモック実装。
type mockItemService struct {
	listItemsFn        func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error)
	getItemFn          func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error)
	listStarredItemsFn func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error)
}

func (m *mockItemService) ListItems(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
	if m.listItemsFn != nil {
		return m.listItemsFn(ctx, userID, feedID, filter, cursor, limit)
	}
	return &itemListResult{}, nil
}

func (m *mockItemService) GetItem(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
	if m.getItemFn != nil {
		return m.getItemFn(ctx, userID, itemID)
	}
	return nil, nil
}

func (m *mockItemService) ListStarredItems(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
	if m.listStarredItemsFn != nil {
		return m.listStarredItemsFn(ctx, userID, cursor, limit)
	}
	return &starredItemListResult{}, nil
}

// mockItemStateService はItemStateServiceInterfaceのモック実装。
type mockItemStateService struct {
	updateStateFn func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error)
}

func (m *mockItemStateService) UpdateState(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
	if m.updateStateFn != nil {
		return m.updateStateFn(ctx, userID, itemID, isRead, isStarred)
	}
	return nil, nil
}

// --- GET /api/feeds/:id/items テスト ---

func TestItemHandler_ListItems_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if feedID != "feed-1" {
				t.Errorf("feedID = %q, want %q", feedID, "feed-1")
			}
			if filter != model.ItemFilterAll {
				t.Errorf("filter = %q, want %q", filter, model.ItemFilterAll)
			}
			if limit != 50 {
				t.Errorf("limit = %d, want %d", limit, 50)
			}
			return &itemListResult{
				Items: []itemSummaryResponse{
					{
						ID:              "item-1",
						FeedID:          "feed-1",
						Title:           "テスト記事1",
						Link:            "https://example.com/1",
						PublishedAt:     now,
						IsDateEstimated: false,
						IsRead:          false,
						IsStarred:       true,
						HatebuCount:     10,
					},
				},
				NextCursor: now.Format(time.RFC3339Nano),
				HasMore:    true,
			}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array in response")
	}
	if len(items) != 1 {
		t.Errorf("items length = %d, want 1", len(items))
	}

	hasMore, ok := result["has_more"].(bool)
	if !ok || !hasMore {
		t.Error("expected has_more to be true")
	}

	if _, ok := result["next_cursor"]; !ok {
		t.Error("expected next_cursor in response")
	}
}

// TestItemHandler_ListItems_IncludesSummary は記事一覧レスポンスに概要(summary)が
// 含まれること、および空概要でもフィールドが省略されず空文字列で返ることを検証する。
// Req 1.1 / 1.3 / NFR 1.1 に対応。
func TestItemHandler_ListItems_IncludesSummary(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	cases := []struct {
		name        string
		summary     string
		wantSummary string
	}{
		{name: "概要が存在するとき概要テキストを返す", summary: "記事の概要テキスト", wantSummary: "記事の概要テキスト"},
		{name: "概要が空のとき空文字列を返す", summary: "", wantSummary: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockItemService{
				listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
					return &itemListResult{
						Items: []itemSummaryResponse{
							{
								ID:          "item-1",
								FeedID:      "feed-1",
								Title:       "テスト記事",
								Link:        "https://example.com/1",
								Summary:     tc.summary,
								PublishedAt: now,
							},
						},
					}, nil
				},
			}
			h := NewItemHandler(svc, &mockItemStateService{})

			req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
			req = withUserID(req, "user-123")
			req = withChiURLParam(req, "id", "feed-1")
			w := httptest.NewRecorder()

			// Act
			h.ListItems(w, req)

			// Assert
			if w.Result().StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
			}

			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("items length = %d, want 1", len(result.Items))
			}

			// summaryフィールドが必ず存在すること（空でも省略しない）
			summaryVal, ok := result.Items[0]["summary"]
			if !ok {
				t.Fatal("expected summary field in item response (must not be omitted)")
			}
			if summaryVal != tc.wantSummary {
				t.Errorf("summary = %v, want %q", summaryVal, tc.wantSummary)
			}
		})
	}
}

// TestItemHandler_ListItems_PreservesExistingFields は概要追加後も既存フィールドが
// 変更されずに保持されることを検証する。Req 1.4 / NFR 1.1 に対応。
func TestItemHandler_ListItems_PreservesExistingFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			return &itemListResult{
				Items: []itemSummaryResponse{
					{
						ID:              "item-1",
						FeedID:          "feed-1",
						Title:           "テスト記事",
						Link:            "https://example.com/1",
						Summary:         "概要",
						PublishedAt:     now,
						IsDateEstimated: true,
						IsRead:          true,
						IsStarred:       true,
						HatebuCount:     7,
					},
				},
			}, nil
		},
	}
	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	var result struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(result.Items))
	}

	item := result.Items[0]
	// 既存フィールドが全て存在し変更されていないこと
	wantFields := map[string]interface{}{
		"id":                "item-1",
		"feed_id":           "feed-1",
		"title":             "テスト記事",
		"link":              "https://example.com/1",
		"is_date_estimated": true,
		"is_read":           true,
		"is_starred":        true,
		"hatebu_count":      float64(7),
	}
	for k, want := range wantFields {
		if got := item[k]; got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	if _, ok := item["published_at"]; !ok {
		t.Error("expected published_at field in item response")
	}
}

func TestItemHandler_ListItems_WithUnreadFilter(t *testing.T) {
	receivedFilter := model.ItemFilterAll
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			receivedFilter = filter
			return &itemListResult{Items: []itemSummaryResponse{}}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items?filter=unread", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if receivedFilter != model.ItemFilterUnread {
		t.Errorf("filter = %q, want %q", receivedFilter, model.ItemFilterUnread)
	}
}

func TestItemHandler_ListItems_WithStarredFilter(t *testing.T) {
	receivedFilter := model.ItemFilterAll
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			receivedFilter = filter
			return &itemListResult{Items: []itemSummaryResponse{}}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items?filter=starred", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if receivedFilter != model.ItemFilterStarred {
		t.Errorf("filter = %q, want %q", receivedFilter, model.ItemFilterStarred)
	}
}

func TestItemHandler_ListItems_InvalidFilter_ReturnsBadRequest(t *testing.T) {
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			return nil, model.NewInvalidFilterError("invalid")
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items?filter=invalid", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestItemHandler_ListItems_WithCursor(t *testing.T) {
	receivedCursor := ""
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			receivedCursor = cursor
			return &itemListResult{Items: []itemSummaryResponse{}}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	cursorValue := "2026-02-27T10:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items?cursor="+cursorValue, nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if receivedCursor != cursorValue {
		t.Errorf("cursor = %q, want %q", receivedCursor, cursorValue)
	}
}

func TestItemHandler_ListItems_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewItemHandler(&mockItemService{}, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
	req = withChiURLParam(req, "id", "feed-1")
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestItemHandler_ListItems_DefaultFilterIsAll(t *testing.T) {
	receivedFilter := model.ItemFilter("")
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			receivedFilter = filter
			return &itemListResult{Items: []itemSummaryResponse{}}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	// filterパラメータなし
	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	if receivedFilter != model.ItemFilterAll {
		t.Errorf("default filter = %q, want %q", receivedFilter, model.ItemFilterAll)
	}
}

func TestItemHandler_ListItems_EmptyResult(t *testing.T) {
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			return &itemListResult{
				Items:   []itemSummaryResponse{},
				HasMore: false,
			}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-1")
	w := httptest.NewRecorder()

	h.ListItems(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array in response")
	}
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0", len(items))
	}

	hasMore, ok := result["has_more"].(bool)
	if !ok || hasMore {
		t.Error("expected has_more to be false")
	}
}

// --- GET /api/items/:id テスト ---

func TestItemHandler_GetItem_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockItemService{
		getItemFn: func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if itemID != "item-1" {
				t.Errorf("itemID = %q, want %q", itemID, "item-1")
			}
			return &itemDetailResponse{
				itemSummaryResponse: itemSummaryResponse{
					ID:              "item-1",
					FeedID:          "feed-1",
					Title:           "テスト記事",
					Link:            "https://example.com/article",
					PublishedAt:     now,
					IsDateEstimated: false,
					IsRead:          true,
					IsStarred:       false,
					HatebuCount:     42,
				},
				Content: "<p>サニタイズ済みコンテンツ</p>",
				Summary: "記事のサマリー",
				Author:  "著者名",
			}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.GetItem(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["id"] != "item-1" {
		t.Errorf("id = %v, want %q", result["id"], "item-1")
	}
	if result["content"] != "<p>サニタイズ済みコンテンツ</p>" {
		t.Errorf("content = %v, want sanitized content", result["content"])
	}
	if result["link"] != "https://example.com/article" {
		t.Errorf("link = %v, want %q", result["link"], "https://example.com/article")
	}
	if result["author"] != "著者名" {
		t.Errorf("author = %v, want %q", result["author"], "著者名")
	}
}

func TestItemHandler_GetItem_NotFound_ReturnsNotFound(t *testing.T) {
	svc := &mockItemService{
		getItemFn: func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
			return nil, model.NewItemNotFoundError(itemID)
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/items/nonexistent", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.GetItem(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeItemNotFound {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeItemNotFound)
	}
}

func TestItemHandler_GetItem_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewItemHandler(&mockItemService{}, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil)
	// ユーザーIDを注入しない
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.GetItem(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestItemHandler_GetItem_ServiceError_ReturnsInternalServerError(t *testing.T) {
	svc := &mockItemService{
		getItemFn: func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
			return nil, errors.New("database error")
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.GetItem(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// --- PUT /api/items/:id/state テスト ---

func TestItemHandler_UpdateItemState_SetRead_Success(t *testing.T) {
	stateSvc := &mockItemStateService{
		updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
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
				t.Error("expected isStarred to be nil (not specified)")
			}
			return &model.ItemState{
				ItemID:    "item-1",
				UserID:    "user-123",
				IsRead:    true,
				IsStarred: false,
			}, nil
		},
	}

	h := NewItemHandler(&mockItemService{}, stateSvc)

	body := `{"is_read": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["item_id"] != "item-1" {
		t.Errorf("item_id = %v, want %q", result["item_id"], "item-1")
	}
	if result["is_read"] != true {
		t.Errorf("is_read = %v, want true", result["is_read"])
	}
}

func TestItemHandler_UpdateItemState_SetStarred_Success(t *testing.T) {
	stateSvc := &mockItemStateService{
		updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
			if isStarred == nil || !*isStarred {
				t.Error("expected isStarred to be true")
			}
			if isRead != nil {
				t.Error("expected isRead to be nil (not specified)")
			}
			return &model.ItemState{
				ItemID:    "item-1",
				UserID:    "user-123",
				IsRead:    false,
				IsStarred: true,
			}, nil
		},
	}

	h := NewItemHandler(&mockItemService{}, stateSvc)

	body := `{"is_starred": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["is_starred"] != true {
		t.Errorf("is_starred = %v, want true", result["is_starred"])
	}
}

func TestItemHandler_UpdateItemState_BothFields_Success(t *testing.T) {
	stateSvc := &mockItemStateService{
		updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
			if isRead == nil || !*isRead {
				t.Error("expected isRead to be true")
			}
			if isStarred == nil || !*isStarred {
				t.Error("expected isStarred to be true")
			}
			return &model.ItemState{
				ItemID:    "item-1",
				UserID:    "user-123",
				IsRead:    true,
				IsStarred: true,
			}, nil
		},
	}

	h := NewItemHandler(&mockItemService{}, stateSvc)

	body := `{"is_read": true, "is_starred": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestItemHandler_UpdateItemState_EmptyBody_ReturnsBadRequest(t *testing.T) {
	h := NewItemHandler(&mockItemService{}, &mockItemStateService{})

	// is_readもis_starredも指定しない
	body := `{}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestItemHandler_UpdateItemState_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	h := NewItemHandler(&mockItemService{}, &mockItemStateService{})

	body := `{invalid json`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestItemHandler_UpdateItemState_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewItemHandler(&mockItemService{}, &mockItemStateService{})

	body := `{"is_read": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// ユーザーIDを注入しない
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestItemHandler_UpdateItemState_ItemNotFound_ReturnsNotFound(t *testing.T) {
	stateSvc := &mockItemStateService{
		updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
			return nil, model.NewItemNotFoundError(itemID)
		},
	}

	h := NewItemHandler(&mockItemService{}, stateSvc)

	body := `{"is_read": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "item-1")
	w := httptest.NewRecorder()

	h.UpdateItemState(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestItemHandler_UpdateItemState_Idempotent(t *testing.T) {
	// 同じ状態を2回設定しても同じ結果が返されることを検証（冪等性）
	callCount := 0
	stateSvc := &mockItemStateService{
		updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
			callCount++
			return &model.ItemState{
				ItemID:    "item-1",
				UserID:    "user-123",
				IsRead:    true,
				IsStarred: false,
			}, nil
		},
	}

	h := NewItemHandler(&mockItemService{}, stateSvc)

	for i := 0; i < 2; i++ {
		body := `{"is_read": true}`
		req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req = withUserID(req, "user-123")
		req = withChiURLParam(req, "id", "item-1")
		w := httptest.NewRecorder()

		h.UpdateItemState(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("call %d: status = %d, want %d", i+1, resp.StatusCode, http.StatusOK)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("call %d: failed to decode response: %v", i+1, err)
		}
		if result["is_read"] != true {
			t.Errorf("call %d: is_read = %v, want true", i+1, result["is_read"])
		}
	}

	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

// --- ルーティングテスト ---

func TestSetupItemRoutes_ListItemsEndpoint(t *testing.T) {
	svc := &mockItemService{
		listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
			return &itemListResult{Items: []itemSummaryResponse{}}, nil
		},
	}

	router := SetupItemRoutes(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-1/items", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/feeds/:id/items status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupItemRoutes_GetItemEndpoint(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockItemService{
		getItemFn: func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
			return &itemDetailResponse{
				itemSummaryResponse: itemSummaryResponse{
					ID:          itemID,
					FeedID:      "feed-1",
					Title:       "テスト",
					PublishedAt: now,
				},
				Content: "<p>コンテンツ</p>",
			}, nil
		},
	}

	router := SetupItemRoutes(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/items/:id status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupItemRoutes_UpdateStateEndpoint(t *testing.T) {
	stateSvc := &mockItemStateService{
		updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
			return &model.ItemState{
				ItemID:    itemID,
				UserID:    userID,
				IsRead:    true,
				IsStarred: false,
			}, nil
		},
	}

	router := SetupItemRoutes(&mockItemService{}, stateSvc)

	body := `{"is_read": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("PUT /api/items/:id/state status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// --- GET /api/feeds/starred/items テスト ---

// TestItemHandler_ListStarredItems_NoUserID_ReturnsUnauthorized は未認証リクエストが
// HTTP 401 を返し、応答ボディに記事データを含めないことを検証する（Requirement 4.6）。
func TestItemHandler_ListStarredItems_NoUserID_ReturnsUnauthorized(t *testing.T) {
	// Arrange
	h := NewItemHandler(&mockItemService{}, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	// Act
	h.ListStarredItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// 応答ボディは APIError 形式で記事データを含まない
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if _, ok := result["items"]; ok {
		t.Error("expected no items field in 401 response")
	}
}

// TestItemHandler_ListStarredItems_Success は認証ありで service 層が
// starredItemListResult を返したときに 200 で JSON 形状（items / next_cursor /
// has_more / 各 item に feed_title）を返すことを検証する。
// Requirement 4.1 / 4.3 / 4.10 / NFR 3.1 に対応。
func TestItemHandler_ListStarredItems_Success(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockItemService{
		listStarredItemsFn: func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if limit != 50 {
				t.Errorf("limit = %d, want %d", limit, 50)
			}
			return &starredItemListResult{
				Items: []starredItemSummaryResponse{
					{
						itemSummaryResponse: itemSummaryResponse{
							ID:              "item-a",
							FeedID:          "feed-1",
							Title:           "テスト記事A",
							Link:            "https://example.com/a",
							Summary:         "サマリーA",
							PublishedAt:     now,
							IsDateEstimated: false,
							IsRead:          false,
							IsStarred:       true,
							HatebuCount:     5,
						},
						FeedTitle: "Feed One",
					},
					{
						itemSummaryResponse: itemSummaryResponse{
							ID:          "item-b",
							FeedID:      "feed-2",
							Title:       "テスト記事B",
							Link:        "https://example.com/b",
							PublishedAt: now.Add(-time.Hour),
							IsStarred:   true,
						},
						FeedTitle: "Feed Two",
					},
				},
				NextCursor: now.Add(-time.Hour).Format(time.RFC3339Nano),
				HasMore:    true,
			}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListStarredItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array in response")
	}
	if len(items) != 2 {
		t.Fatalf("items length = %d, want 2", len(items))
	}

	hasMore, ok := result["has_more"].(bool)
	if !ok || !hasMore {
		t.Error("expected has_more=true in response")
	}
	if _, ok := result["next_cursor"]; !ok {
		t.Error("expected next_cursor in response")
	}

	// 各 item に feed_title が含まれる（Requirement 4.10 / 2.4）
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected items[0] to be an object")
	}
	if first["feed_title"] != "Feed One" {
		t.Errorf("items[0].feed_title = %v, want %q", first["feed_title"], "Feed One")
	}
	if first["feed_id"] != "feed-1" {
		t.Errorf("items[0].feed_id = %v, want %q", first["feed_id"], "feed-1")
	}
	if first["is_starred"] != true {
		t.Errorf("items[0].is_starred = %v, want true", first["is_starred"])
	}

	second, ok := items[1].(map[string]interface{})
	if !ok {
		t.Fatal("expected items[1] to be an object")
	}
	if second["feed_title"] != "Feed Two" {
		t.Errorf("items[1].feed_title = %v, want %q", second["feed_title"], "Feed Two")
	}
}

// TestItemHandler_ListStarredItems_WithCursor は ?cursor= クエリパラメータが
// そのまま service 層に伝搬することを検証する（Requirement 4.5）。
func TestItemHandler_ListStarredItems_WithCursor(t *testing.T) {
	// Arrange
	receivedCursor := ""
	svc := &mockItemService{
		listStarredItemsFn: func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
			receivedCursor = cursor
			return &starredItemListResult{Items: []starredItemSummaryResponse{}}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	cursorValue := "2026-02-27T10:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items?cursor="+cursorValue, nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListStarredItems(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if receivedCursor != cursorValue {
		t.Errorf("cursor = %q, want %q", receivedCursor, cursorValue)
	}
}

// TestItemHandler_ListStarredItems_InvalidCursor_ReturnsBadRequest は service 層が
// model.NewInvalidFilterError を返したときに 400 にマップされることを検証する
// （Requirement 4.8）。
func TestItemHandler_ListStarredItems_InvalidCursor_ReturnsBadRequest(t *testing.T) {
	// Arrange
	svc := &mockItemService{
		listStarredItemsFn: func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
			return nil, model.NewInvalidFilterError("無効なカーソル値: " + cursor)
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items?cursor=not-a-date", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListStarredItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeInvalidFilter {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeInvalidFilter)
	}
}

// TestItemHandler_ListStarredItems_EmptyResult はスター 0 件で
// items=[] / has_more=false が返ることを検証する（Requirement 4.7）。
func TestItemHandler_ListStarredItems_EmptyResult(t *testing.T) {
	// Arrange
	svc := &mockItemService{
		listStarredItemsFn: func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
			return &starredItemListResult{
				Items:   []starredItemSummaryResponse{},
				HasMore: false,
			}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListStarredItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array in response")
	}
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0", len(items))
	}

	hasMore, ok := result["has_more"].(bool)
	if !ok || hasMore {
		t.Error("expected has_more=false in response")
	}

	// next_cursor は has_more=false のとき omit される（omitempty）
	if v, ok := result["next_cursor"]; ok && v != "" {
		t.Errorf("expected next_cursor to be absent or empty, got %v", v)
	}
}

// TestItemHandler_ListStarredItems_EmptyResult_NilItems は service 層が
// Items=nil の starredItemListResult を返したときも、JSON では空配列として
// `"items": []` を返すことを検証する（Requirement 4.7 / NFR 3.1: 既存応答スキーマと
// 区別不能 = items は常に配列であり null にならない）。
func TestItemHandler_ListStarredItems_EmptyResult_NilItems(t *testing.T) {
	// Arrange
	svc := &mockItemService{
		listStarredItemsFn: func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
			return &starredItemListResult{Items: nil, HasMore: false}, nil
		},
	}

	h := NewItemHandler(svc, &mockItemStateService{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListStarredItems(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}

	// JSON 上 items は null でなく [] であること
	bodyBytes := w.Body.Bytes()
	if !bytes.Contains(bodyBytes, []byte(`"items":[]`)) {
		t.Errorf("expected items=[] in JSON, got %s", string(bodyBytes))
	}
}
