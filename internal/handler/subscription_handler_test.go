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
	"github.com/hitoshi/feedman/internal/repository"
)

// --- モック定義 ---

// mockSubscriptionService はSubscriptionServiceInterfaceのモック実装。
type mockSubscriptionService struct {
	listSubscriptionsFn func(ctx context.Context, userID string) ([]subscriptionResponse, error)
	updateSettingsFn    func(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error)
	unsubscribeFn       func(ctx context.Context, userID, subscriptionID string) error
	resumeFetchFn       func(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error)
}

func (m *mockSubscriptionService) ListSubscriptions(ctx context.Context, userID string) ([]subscriptionResponse, error) {
	if m.listSubscriptionsFn != nil {
		return m.listSubscriptionsFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockSubscriptionService) UpdateSettings(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
	if m.updateSettingsFn != nil {
		return m.updateSettingsFn(ctx, userID, subscriptionID, minutes)
	}
	return nil, nil
}

func (m *mockSubscriptionService) Unsubscribe(ctx context.Context, userID, subscriptionID string) error {
	if m.unsubscribeFn != nil {
		return m.unsubscribeFn(ctx, userID, subscriptionID)
	}
	return nil
}

func (m *mockSubscriptionService) ResumeFetch(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
	if m.resumeFetchFn != nil {
		return m.resumeFetchFn(ctx, userID, subscriptionID)
	}
	return nil, nil
}

// --- GET /api/subscriptions テスト ---

func TestSubscriptionHandler_ListSubscriptions_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			return []subscriptionResponse{
				{
					ID:                   "sub-1",
					UserID:               "user-123",
					FeedID:               "feed-1",
					FeedTitle:            "Example Feed",
					FeedURL:              "https://example.com/feed.xml",
					FetchIntervalMinutes: 60,
					FeedStatus:           "active",
					UnreadCount:          5,
					CreatedAt:            now,
				},
			}, nil
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.ListSubscriptions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("result length = %d, want 1", len(result))
	}

	sub := result[0]
	if sub["id"] != "sub-1" {
		t.Errorf("id = %v, want %q", sub["id"], "sub-1")
	}
	if sub["feed_title"] != "Example Feed" {
		t.Errorf("feed_title = %v, want %q", sub["feed_title"], "Example Feed")
	}
	if sub["feed_url"] != "https://example.com/feed.xml" {
		t.Errorf("feed_url = %v, want %q", sub["feed_url"], "https://example.com/feed.xml")
	}
	if sub["feed_status"] != "active" {
		t.Errorf("feed_status = %v, want %q", sub["feed_status"], "active")
	}
	if int(sub["unread_count"].(float64)) != 5 {
		t.Errorf("unread_count = %v, want 5", sub["unread_count"])
	}
}

func TestSubscriptionHandler_ListSubscriptions_EmptyList(t *testing.T) {
	svc := &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			return []subscriptionResponse{}, nil
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.ListSubscriptions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}
}

func TestSubscriptionHandler_ListSubscriptions_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	w := httptest.NewRecorder()

	h.ListSubscriptions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSubscriptionHandler_ListSubscriptions_ServiceError(t *testing.T) {
	svc := &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			return nil, errors.New("database error")
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.ListSubscriptions(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// --- PUT /api/subscriptions/:id/settings テスト ---

func TestSubscriptionHandler_UpdateSettings_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockSubscriptionService{
		updateSettingsFn: func(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if subscriptionID != "sub-1" {
				t.Errorf("subscriptionID = %q, want %q", subscriptionID, "sub-1")
			}
			if minutes != 120 {
				t.Errorf("minutes = %d, want %d", minutes, 120)
			}
			return &subscriptionResponse{
				ID:                   "sub-1",
				UserID:               "user-123",
				FeedID:               "feed-1",
				FeedTitle:            "Example Feed",
				FeedURL:              "https://example.com/feed.xml",
				FetchIntervalMinutes: 120,
				FeedStatus:           "active",
				UnreadCount:          3,
				CreatedAt:            now,
			}, nil
		},
	}

	h := NewSubscriptionHandler(svc)

	body := `{"fetch_interval_minutes": 120}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if int(result["fetch_interval_minutes"].(float64)) != 120 {
		t.Errorf("fetch_interval_minutes = %v, want 120", result["fetch_interval_minutes"])
	}
}

func TestSubscriptionHandler_UpdateSettings_InvalidInterval_TooLow(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	body := `{"fetch_interval_minutes": 15}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSubscriptionHandler_UpdateSettings_InvalidInterval_TooHigh(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	body := `{"fetch_interval_minutes": 750}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSubscriptionHandler_UpdateSettings_InvalidInterval_NotMultipleOf30(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	body := `{"fetch_interval_minutes": 45}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSubscriptionHandler_UpdateSettings_ValidIntervals(t *testing.T) {
	validIntervals := []int{30, 60, 90, 120, 150, 180, 360, 720}
	for _, interval := range validIntervals {
		svc := &mockSubscriptionService{
			updateSettingsFn: func(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
				return &subscriptionResponse{
					FetchIntervalMinutes: minutes,
				}, nil
			},
		}

		h := NewSubscriptionHandler(svc)

		body, _ := json.Marshal(map[string]int{"fetch_interval_minutes": interval})
		req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = withUserID(req, "user-123")
		req = withChiURLParam(req, "id", "sub-1")
		w := httptest.NewRecorder()

		h.UpdateSettings(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("interval=%d: status = %d, want %d", interval, resp.StatusCode, http.StatusOK)
		}
	}
}

func TestSubscriptionHandler_UpdateSettings_InvalidJSON(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	body := `{invalid`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSubscriptionHandler_UpdateSettings_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	body := `{"fetch_interval_minutes": 60}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSubscriptionHandler_UpdateSettings_SubscriptionNotFound(t *testing.T) {
	svc := &mockSubscriptionService{
		updateSettingsFn: func(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
			return nil, model.NewSubscriptionNotFoundError(subscriptionID)
		},
	}

	h := NewSubscriptionHandler(svc)

	body := `{"fetch_interval_minutes": 60}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/nonexistent/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.UpdateSettings(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// --- DELETE /api/subscriptions/:id テスト ---

func TestSubscriptionHandler_Unsubscribe_Success(t *testing.T) {
	unsubscribeCalled := false
	svc := &mockSubscriptionService{
		unsubscribeFn: func(ctx context.Context, userID, subscriptionID string) error {
			unsubscribeCalled = true
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if subscriptionID != "sub-1" {
				t.Errorf("subscriptionID = %q, want %q", subscriptionID, "sub-1")
			}
			return nil
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions/sub-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.Unsubscribe(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if !unsubscribeCalled {
		t.Error("expected Unsubscribe to be called")
	}
}

func TestSubscriptionHandler_Unsubscribe_NotFound(t *testing.T) {
	svc := &mockSubscriptionService{
		unsubscribeFn: func(ctx context.Context, userID, subscriptionID string) error {
			return model.NewSubscriptionNotFoundError(subscriptionID)
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions/nonexistent", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.Unsubscribe(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestSubscriptionHandler_Unsubscribe_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions/sub-1", nil)
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.Unsubscribe(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSubscriptionHandler_Unsubscribe_InternalError(t *testing.T) {
	svc := &mockSubscriptionService{
		unsubscribeFn: func(ctx context.Context, userID, subscriptionID string) error {
			return errors.New("database error")
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions/sub-1", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.Unsubscribe(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// --- ルーティングテスト ---

func TestSetupSubscriptionRoutes_ListEndpoint(t *testing.T) {
	svc := &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			return []subscriptionResponse{}, nil
		},
	}

	router := SetupSubscriptionRoutes(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/subscriptions status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupSubscriptionRoutes_UpdateSettingsEndpoint(t *testing.T) {
	svc := &mockSubscriptionService{
		updateSettingsFn: func(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
			return &subscriptionResponse{FetchIntervalMinutes: minutes}, nil
		},
	}

	router := SetupSubscriptionRoutes(svc)

	body := `{"fetch_interval_minutes": 60}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("PUT /api/subscriptions/:id/settings status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupSubscriptionRoutes_DeleteEndpoint(t *testing.T) {
	svc := &mockSubscriptionService{
		unsubscribeFn: func(ctx context.Context, userID, subscriptionID string) error {
			return nil
		},
	}

	router := SetupSubscriptionRoutes(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions/sub-1", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /api/subscriptions/:id status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// subscriptionResponse のFaviconURL (omitempty) テスト
func TestSubscriptionHandler_ListSubscriptions_WithFaviconURL(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	faviconURL := "/api/feeds/feed-1/favicon"
	svc := &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			return []subscriptionResponse{
				{
					ID:                   "sub-1",
					UserID:               "user-123",
					FeedID:               "feed-1",
					FeedTitle:            "Example Feed",
					FeedURL:              "https://example.com/feed.xml",
					FaviconURL:           &faviconURL,
					FetchIntervalMinutes: 60,
					FeedStatus:           "active",
					UnreadCount:          0,
					CreatedAt:            now,
				},
			}, nil
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.ListSubscriptions(w, req)

	var result []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("result length = %d, want 1", len(result))
	}

	if result[0]["favicon_url"] != faviconURL {
		t.Errorf("favicon_url = %v, want %q", result[0]["favicon_url"], faviconURL)
	}
}

// --- subscriptionResponse のエラーメッセージテスト ---
func TestSubscriptionHandler_ListSubscriptions_WithErrorMessage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	errMsg := "404 Not Found"
	svc := &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			return []subscriptionResponse{
				{
					ID:                   "sub-1",
					UserID:               "user-123",
					FeedID:               "feed-1",
					FeedTitle:            "Dead Feed",
					FeedURL:              "https://example.com/dead.xml",
					FetchIntervalMinutes: 60,
					FeedStatus:           "stopped",
					ErrorMessage:         &errMsg,
					UnreadCount:          0,
					CreatedAt:            now,
				},
			}, nil
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.ListSubscriptions(w, req)

	var result []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result[0]["feed_status"] != "stopped" {
		t.Errorf("feed_status = %v, want %q", result[0]["feed_status"], "stopped")
	}
	if result[0]["error_message"] != errMsg {
		t.Errorf("error_message = %v, want %q", result[0]["error_message"], errMsg)
	}
}

// --- バリデーションのエッジケーステスト ---
func TestSubscriptionHandler_UpdateSettings_BoundaryValues(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		wantCode int
	}{
		{"min valid (30)", 30, http.StatusOK},
		{"max valid (720)", 720, http.StatusOK},
		{"just below min (29)", 29, http.StatusBadRequest},
		{"just above max (721)", 721, http.StatusBadRequest},
		{"zero", 0, http.StatusBadRequest},
		{"negative", -30, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockSubscriptionService{
				updateSettingsFn: func(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error) {
					return &subscriptionResponse{FetchIntervalMinutes: minutes}, nil
				},
			}

			h := NewSubscriptionHandler(svc)

			body, _ := json.Marshal(map[string]int{"fetch_interval_minutes": tt.interval})
			req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/sub-1/settings", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = withUserID(req, "user-123")
			req = withChiURLParam(req, "id", "sub-1")
			w := httptest.NewRecorder()

			h.UpdateSettings(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.wantCode {
				t.Errorf("interval=%d: status = %d, want %d", tt.interval, resp.StatusCode, tt.wantCode)
			}
		})
	}
}

// --- POST /api/subscriptions/:id/resume テスト ---

func TestSubscriptionHandler_ResumeFetch_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockSubscriptionService{
		resumeFetchFn: func(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if subscriptionID != "sub-1" {
				t.Errorf("subscriptionID = %q, want %q", subscriptionID, "sub-1")
			}
			return &subscriptionResponse{
				ID:                   "sub-1",
				UserID:               "user-123",
				FeedID:               "feed-1",
				FeedTitle:            "Resumed Feed",
				FeedURL:              "https://example.com/feed.xml",
				FetchIntervalMinutes: 60,
				FeedStatus:           "active",
				UnreadCount:          3,
				CreatedAt:            now,
			}, nil
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/sub-1/resume", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.ResumeFetch(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["feed_status"] != "active" {
		t.Errorf("feed_status = %v, want %q", result["feed_status"], "active")
	}
}

func TestSubscriptionHandler_ResumeFetch_NotFound(t *testing.T) {
	svc := &mockSubscriptionService{
		resumeFetchFn: func(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
			return nil, model.NewSubscriptionNotFoundError(subscriptionID)
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/nonexistent/resume", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	h.ResumeFetch(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestSubscriptionHandler_ResumeFetch_FeedNotStopped(t *testing.T) {
	svc := &mockSubscriptionService{
		resumeFetchFn: func(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
			return nil, model.NewFeedNotStoppedError()
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/sub-1/resume", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.ResumeFetch(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestSubscriptionHandler_ResumeFetch_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewSubscriptionHandler(&mockSubscriptionService{})

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/sub-1/resume", nil)
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.ResumeFetch(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSubscriptionHandler_ResumeFetch_InternalError(t *testing.T) {
	svc := &mockSubscriptionService{
		resumeFetchFn: func(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
			return nil, errors.New("database error")
		},
	}

	h := NewSubscriptionHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/sub-1/resume", nil)
	req = withUserID(req, "user-123")
	req = withChiURLParam(req, "id", "sub-1")
	w := httptest.NewRecorder()

	h.ResumeFetch(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestSetupSubscriptionRoutes_ResumeEndpoint(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockSubscriptionService{
		resumeFetchFn: func(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error) {
			return &subscriptionResponse{
				ID:         subscriptionID,
				FeedStatus: "active",
				CreatedAt:  now,
			}, nil
		},
	}

	router := SetupSubscriptionRoutes(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/sub-1/resume", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /api/subscriptions/:id/resume status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// --- unused import guard for repository (needed for SubscriptionWithFeedInfo type) ---
var _ = repository.SubscriptionWithFeedInfo{}
