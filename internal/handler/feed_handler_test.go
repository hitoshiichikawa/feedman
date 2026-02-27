package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockFeedService はFeedServiceInterfaceのモック実装。
type mockFeedService struct {
	registerFeedFn  func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error)
	getFeedFn       func(ctx context.Context, feedID string) (*model.Feed, error)
	updateFeedURLFn func(ctx context.Context, feedID, newURL string) (*model.Feed, error)
}

func (m *mockFeedService) RegisterFeed(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
	if m.registerFeedFn != nil {
		return m.registerFeedFn(ctx, userID, inputURL)
	}
	return nil, nil, nil
}

func (m *mockFeedService) GetFeed(ctx context.Context, feedID string) (*model.Feed, error) {
	if m.getFeedFn != nil {
		return m.getFeedFn(ctx, feedID)
	}
	return nil, nil
}

func (m *mockFeedService) UpdateFeedURL(ctx context.Context, feedID, newURL string) (*model.Feed, error) {
	if m.updateFeedURLFn != nil {
		return m.updateFeedURLFn(ctx, feedID, newURL)
	}
	return nil, nil
}

// mockSubscriptionDeleter はSubscriptionDeleterのモック実装。
type mockSubscriptionDeleter struct {
	deleteByUserAndFeedFn func(ctx context.Context, userID, feedID string) error
}

func (m *mockSubscriptionDeleter) DeleteByUserAndFeed(ctx context.Context, userID, feedID string) error {
	if m.deleteByUserAndFeedFn != nil {
		return m.deleteByUserAndFeedFn(ctx, userID, feedID)
	}
	return nil
}

// --- テストヘルパー ---

// withUserID はテスト用にリクエストコンテキストにユーザーIDを注入するヘルパー。
func withUserID(r *http.Request, userID string) *http.Request {
	ctx := middleware.ContextWithUserID(r.Context(), userID)
	return r.WithContext(ctx)
}

// withChiURLParam はテスト用にchiのURLパラメータを注入するヘルパー。
func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	return r.WithContext(ctx)
}

// parseAPIErrorResponse はレスポンスボディからAPIErrorレスポンスをパースするヘルパー。
func parseAPIErrorResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	return result
}

// --- POST /api/feeds テスト ---

func TestFeedHandler_RegisterFeed_Success(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if inputURL != "https://example.com/feed.xml" {
				t.Errorf("inputURL = %q, want %q", inputURL, "https://example.com/feed.xml")
			}
			return &model.Feed{
				ID:      "feed-id-1",
				FeedURL: "https://example.com/feed.xml",
				SiteURL: "https://example.com",
				Title:   "Example Feed",
			}, &model.Subscription{
				ID:     "sub-id-1",
				UserID: "user-123",
				FeedID: "feed-id-1",
			}, nil
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["id"] != "feed-id-1" {
		t.Errorf("id = %v, want %q", result["id"], "feed-id-1")
	}
	if result["feed_url"] != "https://example.com/feed.xml" {
		t.Errorf("feed_url = %v, want %q", result["feed_url"], "https://example.com/feed.xml")
	}
}

func TestFeedHandler_RegisterFeed_EmptyURL_ReturnsBadRequest(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{}, &mockSubscriptionDeleter{})

	body := `{"url": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] == "" {
		t.Error("expected error code in response")
	}
	if errResp["category"] == "" {
		t.Error("expected category in response")
	}
	if errResp["action"] == "" {
		t.Error("expected action in response")
	}
}

func TestFeedHandler_RegisterFeed_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{}, &mockSubscriptionDeleter{})

	body := `{invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestFeedHandler_RegisterFeed_SubscriptionLimit_ReturnsConflict(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			return nil, nil, model.NewSubscriptionLimitError()
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeSubscriptionLimit {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeSubscriptionLimit)
	}
}

func TestFeedHandler_RegisterFeed_FeedNotDetected_ReturnsUnprocessableEntity(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			return nil, nil, model.NewFeedNotDetectedError("https://example.com")
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeFeedNotDetected {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeFeedNotDetected)
	}
}

func TestFeedHandler_RegisterFeed_DuplicateSubscription_ReturnsConflict(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			return nil, nil, model.NewDuplicateSubscriptionError()
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestFeedHandler_RegisterFeed_InternalError_ReturnsInternalServerError(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			return nil, nil, errors.New("database connection failed")
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestFeedHandler_RegisterFeed_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{}, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// --- GET /api/feeds/:id テスト ---

func TestFeedHandler_GetFeed_Success(t *testing.T) {
	svc := &mockFeedService{
		getFeedFn: func(ctx context.Context, feedID string) (*model.Feed, error) {
			if feedID != "feed-id-1" {
				t.Errorf("feedID = %q, want %q", feedID, "feed-id-1")
			}
			return &model.Feed{
				ID:      "feed-id-1",
				FeedURL: "https://example.com/feed.xml",
				SiteURL: "https://example.com",
				Title:   "Example Feed",
			}, nil
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-id-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.GetFeed(w, req)

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

	if result["id"] != "feed-id-1" {
		t.Errorf("id = %v, want %q", result["id"], "feed-id-1")
	}
}

func TestFeedHandler_GetFeed_NotFound(t *testing.T) {
	svc := &mockFeedService{
		getFeedFn: func(ctx context.Context, feedID string) (*model.Feed, error) {
			return nil, nil
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/nonexistent", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.GetFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] == "" {
		t.Error("expected error code in response")
	}
}

func TestFeedHandler_GetFeed_ServiceError_ReturnsInternalServerError(t *testing.T) {
	svc := &mockFeedService{
		getFeedFn: func(ctx context.Context, feedID string) (*model.Feed, error) {
			return nil, errors.New("database error")
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-id-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.GetFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// --- PATCH /api/feeds/:id テスト ---

func TestFeedHandler_UpdateFeedURL_Success(t *testing.T) {
	svc := &mockFeedService{
		updateFeedURLFn: func(ctx context.Context, feedID, newURL string) (*model.Feed, error) {
			if feedID != "feed-id-1" {
				t.Errorf("feedID = %q, want %q", feedID, "feed-id-1")
			}
			if newURL != "https://example.com/new-feed.xml" {
				t.Errorf("newURL = %q, want %q", newURL, "https://example.com/new-feed.xml")
			}
			return &model.Feed{
				ID:      "feed-id-1",
				FeedURL: "https://example.com/new-feed.xml",
				SiteURL: "https://example.com",
				Title:   "Example Feed",
			}, nil
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"feed_url": "https://example.com/new-feed.xml"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/feeds/feed-id-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.UpdateFeedURL(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["feed_url"] != "https://example.com/new-feed.xml" {
		t.Errorf("feed_url = %v, want %q", result["feed_url"], "https://example.com/new-feed.xml")
	}
}

func TestFeedHandler_UpdateFeedURL_EmptyURL_ReturnsBadRequest(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{}, &mockSubscriptionDeleter{})

	body := `{"feed_url": ""}`
	req := httptest.NewRequest(http.MethodPatch, "/api/feeds/feed-id-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.UpdateFeedURL(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestFeedHandler_UpdateFeedURL_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{}, &mockSubscriptionDeleter{})

	body := `{invalid`
	req := httptest.NewRequest(http.MethodPatch, "/api/feeds/feed-id-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.UpdateFeedURL(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestFeedHandler_UpdateFeedURL_FeedNotFound_ReturnsNotFound(t *testing.T) {
	svc := &mockFeedService{
		updateFeedURLFn: func(ctx context.Context, feedID, newURL string) (*model.Feed, error) {
			return nil, &model.APIError{
				Code:     "FEED_NOT_FOUND",
				Message:  "Feed not found",
				Category: "feed",
				Action:   "Check feed ID",
			}
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"feed_url": "https://example.com/new-feed.xml"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/feeds/nonexistent", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.UpdateFeedURL(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// --- DELETE /api/feeds/:id テスト ---

func TestFeedHandler_DeleteFeed_Success(t *testing.T) {
	deleteCalled := false
	deleter := &mockSubscriptionDeleter{
		deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
			deleteCalled = true
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if feedID != "feed-id-1" {
				t.Errorf("feedID = %q, want %q", feedID, "feed-id-1")
			}
			return nil
		},
	}

	h := NewFeedHandler(&mockFeedService{}, deleter)

	req := httptest.NewRequest(http.MethodDelete, "/api/feeds/feed-id-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.DeleteFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if !deleteCalled {
		t.Error("expected DeleteByUserAndFeed to be called")
	}
}

func TestFeedHandler_DeleteFeed_NotFound_ReturnsNotFound(t *testing.T) {
	deleter := &mockSubscriptionDeleter{
		deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
			return &model.APIError{
				Code:     "SUBSCRIPTION_NOT_FOUND",
				Message:  "Subscription not found",
				Category: "feed",
				Action:   "Check feed ID",
			}
		},
	}

	h := NewFeedHandler(&mockFeedService{}, deleter)

	req := httptest.NewRequest(http.MethodDelete, "/api/feeds/nonexistent", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.DeleteFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestFeedHandler_DeleteFeed_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewFeedHandler(&mockFeedService{}, &mockSubscriptionDeleter{})

	req := httptest.NewRequest(http.MethodDelete, "/api/feeds/feed-id-1", nil)
	// ユーザーIDを注入しない
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.DeleteFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestFeedHandler_DeleteFeed_InternalError_ReturnsInternalServerError(t *testing.T) {
	deleter := &mockSubscriptionDeleter{
		deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
			return errors.New("database error")
		},
	}

	h := NewFeedHandler(&mockFeedService{}, deleter)

	req := httptest.NewRequest(http.MethodDelete, "/api/feeds/feed-id-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "feed-id-1")
	w := httptest.NewRecorder()

	h.DeleteFeed(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// --- 統一エラーフォーマットのテスト ---

func TestFeedHandler_ErrorResponse_ContainsAllFields(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			return nil, nil, model.NewSubscriptionLimitError()
		},
	}

	h := NewFeedHandler(svc, &mockSubscriptionDeleter{})

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.RegisterFeed(w, req)

	errResp := parseAPIErrorResponse(t, w)

	// 統一エラーフォーマット（code, message, category, action）の4フィールドを検証
	requiredFields := []string{"code", "message", "category", "action"}
	for _, field := range requiredFields {
		if errResp[field] == "" {
			t.Errorf("expected non-empty %q field in error response", field)
		}
	}
}

// --- ルーティングテスト ---

func TestSetupFeedRoutes_RegisterEndpoint(t *testing.T) {
	svc := &mockFeedService{
		registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
			return &model.Feed{
				ID:      "feed-1",
				FeedURL: "https://example.com/feed.xml",
				Title:   "Test",
			}, &model.Subscription{
				ID:     "sub-1",
				UserID: userID,
				FeedID: "feed-1",
			}, nil
		},
	}

	router := SetupFeedRoutes(svc, &mockSubscriptionDeleter{}, nil)

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST /api/feeds status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestSetupFeedRoutes_GetEndpoint(t *testing.T) {
	svc := &mockFeedService{
		getFeedFn: func(ctx context.Context, feedID string) (*model.Feed, error) {
			return &model.Feed{
				ID:      feedID,
				FeedURL: "https://example.com/feed.xml",
				Title:   "Test",
			}, nil
		},
	}

	router := SetupFeedRoutes(svc, &mockSubscriptionDeleter{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/feed-id-1", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/feeds/:id status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupFeedRoutes_PatchEndpoint(t *testing.T) {
	svc := &mockFeedService{
		updateFeedURLFn: func(ctx context.Context, feedID, newURL string) (*model.Feed, error) {
			return &model.Feed{
				ID:      feedID,
				FeedURL: newURL,
				Title:   "Test",
			}, nil
		},
	}

	router := SetupFeedRoutes(svc, &mockSubscriptionDeleter{}, nil)

	body := `{"feed_url": "https://example.com/new-feed.xml"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/feeds/feed-id-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("PATCH /api/feeds/:id status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupFeedRoutes_DeleteEndpoint(t *testing.T) {
	deleter := &mockSubscriptionDeleter{
		deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
			return nil
		},
	}

	router := SetupFeedRoutes(&mockFeedService{}, deleter, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/feeds/feed-id-1", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /api/feeds/:id status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestSetupFeedRoutes_UnknownRoute_Returns404Or405(t *testing.T) {
	router := SetupFeedRoutes(&mockFeedService{}, &mockSubscriptionDeleter{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	// /api/feeds への GET は定義されていない（一覧ではないため）
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/feeds status = %d, want 404 or 405", resp.StatusCode)
	}
}
