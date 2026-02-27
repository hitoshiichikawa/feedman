package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// mockSessionFinderForRouter はRouter統合テスト用のSessionFinderモック。
type mockSessionFinderForRouter struct {
	sessions map[string]*model.Session
}

func (m *mockSessionFinderForRouter) FindByID(ctx context.Context, id string) (*model.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, nil
}

// createTestRouter はテスト用の完全なルーターを構築するヘルパー。
func createTestRouter() (http.Handler, *mockSessionFinderForRouter) {
	sessionFinder := &mockSessionFinderForRouter{
		sessions: map[string]*model.Session{
			"valid-session": {
				ID:        "valid-session",
				UserID:    "user-test-1",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
		},
	}

	deps := &RouterDeps{
		SessionFinder: sessionFinder,
		CSRFConfig:    middleware.CSRFConfig{CookieSecure: false},
		RateLimiter:   middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService:   &mockAuthService{
			getLoginURLFn: func(state string) string {
				return "https://accounts.google.com?state=" + state
			},
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				return &model.User{ID: "user-test-1", Email: "test@example.com", Name: "Test"}, nil
			},
		},
		AuthConfig:          AuthHandlerConfig{BaseURL: "http://localhost:3000", SessionMaxAge: 86400},
		FeedService: &mockFeedService{
			registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
				return &model.Feed{
					ID:      "feed-test-1",
					FeedURL: inputURL,
					SiteURL: "https://example.com",
					Title:   "Test Feed",
				}, &model.Subscription{
					ID:     "sub-test-1",
					UserID: userID,
					FeedID: "feed-test-1",
				}, nil
			},
			getFeedFn: func(ctx context.Context, feedID string) (*model.Feed, error) {
				return &model.Feed{
					ID:      feedID,
					FeedURL: "https://example.com/feed.xml",
					SiteURL: "https://example.com",
					Title:   "Test Feed",
				}, nil
			},
			updateFeedURLFn: func(ctx context.Context, feedID, newURL string) (*model.Feed, error) {
				return &model.Feed{
					ID:      feedID,
					FeedURL: newURL,
					SiteURL: "https://example.com",
					Title:   "Test Feed",
				}, nil
			},
		},
		SubscriptionDeleter: &mockSubscriptionDeleter{
			deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
				return nil
			},
		},
		ItemService: &mockItemService{
			listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
				return &itemListResult{Items: []itemSummaryResponse{}, HasMore: false}, nil
			},
			getItemFn: func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
				return &itemDetailResponse{
					itemSummaryResponse: itemSummaryResponse{
						ID:    itemID,
						Title: "Test Item",
					},
					Content: "<p>Test</p>",
				}, nil
			},
		},
		ItemStateService: &mockItemStateService{
			updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
				return &model.ItemState{UserID: userID, ItemID: itemID}, nil
			},
		},
		SubscriptionService: &mockSubscriptionService{
			listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
				return []subscriptionResponse{}, nil
			},
		},
		UserService: &mockUserService{},
	}

	router := NewRouter(deps)
	return router, sessionFinder
}

// TestNewRouter_CSRFTokenEndpoint_NoAuthRequired は
// CSRFトークン取得エンドポイントが認証不要であることを検証する。
func TestNewRouter_CSRFTokenEndpoint_NoAuthRequired(t *testing.T) {
	router, _ := createTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /api/csrf-token status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}

	var body map[string]string
	json.NewDecoder(w.Result().Body).Decode(&body)
	if body["token"] == "" {
		t.Error("expected non-empty CSRF token")
	}
}

// TestNewRouter_AuthRoutes_LoginEndpoint は認証ルートが正しく設定されていることを検証する。
func TestNewRouter_AuthRoutes_LoginEndpoint(t *testing.T) {
	router, _ := createTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("GET /auth/google/login status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
}

// TestNewRouter_AuthRoutes_MeEndpoint はGET /auth/meが正しくルーティングされることを検証する。
func TestNewRouter_AuthRoutes_MeEndpoint(t *testing.T) {
	router, _ := createTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /auth/me status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestNewRouter_ProtectedRoute_NoSession_Returns401 は
// 認証保護ルートにセッションなしでアクセスすると401が返ることを検証する。
func TestNewRouter_ProtectedRoute_NoSession_Returns401(t *testing.T) {
	router, _ := createTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/subscriptions (no session) status = %d, want %d",
			w.Result().StatusCode, http.StatusUnauthorized)
	}
}

// TestNewRouter_ProtectedRoute_WithSession_GET_Succeeds は
// 認証保護ルートにセッション付きGETリクエストが成功することを検証する。
func TestNewRouter_ProtectedRoute_WithSession_GET_Succeeds(t *testing.T) {
	router, _ := createTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /api/subscriptions status = %d, want %d",
			w.Result().StatusCode, http.StatusOK)
	}
}

// TestNewRouter_ProtectedRoute_POST_RequiresCSRF は
// POSTリクエストにCSRFトークンが必須であることを検証する。
func TestNewRouter_ProtectedRoute_POST_RequiresCSRF(t *testing.T) {
	router, _ := createTestRouter()

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("POST /api/feeds (no CSRF) status = %d, want %d",
			w.Result().StatusCode, http.StatusForbidden)
	}
}

// TestNewRouter_ProtectedRoute_POST_WithCSRF_Succeeds は
// POSTリクエストにCSRFトークン付きでアクセスが成功することを検証する。
func TestNewRouter_ProtectedRoute_POST_WithCSRF_Succeeds(t *testing.T) {
	router, _ := createTestRouter()

	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "test-token"})
	req.Header.Set("X-CSRF-Token", "test-token")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// サービスモックが空なのでnilが返るが、CSRF検証は通過していること
	status := w.Result().StatusCode
	if status == http.StatusForbidden || status == http.StatusUnauthorized {
		t.Errorf("POST /api/feeds (with CSRF) status = %d, should not be 403 or 401", status)
	}
}

// TestNewRouter_MiddlewareOrder_SessionBeforeCSRF は
// セッション検証がCSRF検証より先に実行されることを検証する。
func TestNewRouter_MiddlewareOrder_SessionBeforeCSRF(t *testing.T) {
	router, _ := createTestRouter()

	body := `{"url": "https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("POST (no session, no CSRF) status = %d, want %d (session check before CSRF)",
			w.Result().StatusCode, http.StatusUnauthorized)
	}
}

// TestNewRouter_FeedRoutes_AllEndpoints はフィード関連の全エンドポイントが登録されていることを検証する。
func TestNewRouter_FeedRoutes_AllEndpoints(t *testing.T) {
	router, _ := createTestRouter()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/feeds/feed-1"},
		{http.MethodPatch, "/api/feeds/feed-1"},
		{http.MethodDelete, "/api/feeds/feed-1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var body *strings.Reader
			if tt.method == http.MethodPatch {
				body = strings.NewReader(`{"feed_url": "https://example.com/new.xml"}`)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
			req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "test-token"})
			req.Header.Set("X-CSRF-Token", "test-token")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Result().StatusCode == http.StatusNotFound {
				t.Errorf("%s %s returned 404, route not found", tt.method, tt.path)
			}
		})
	}
}

// TestNewRouter_ItemRoutes_AllEndpoints は記事関連の全エンドポイントが登録されていることを検証する。
func TestNewRouter_ItemRoutes_AllEndpoints(t *testing.T) {
	router, _ := createTestRouter()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/feeds/feed-1/items"},
		{http.MethodGet, "/api/items/item-1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Result().StatusCode == http.StatusNotFound {
				t.Errorf("%s %s returned 404, route not found", tt.method, tt.path)
			}
		})
	}
}

// TestNewRouter_SubscriptionRoutes_AllEndpoints は購読関連の全エンドポイントが登録されていることを検証する。
func TestNewRouter_SubscriptionRoutes_AllEndpoints(t *testing.T) {
	router, _ := createTestRouter()

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/subscriptions", ""},
		{http.MethodDelete, "/api/subscriptions/sub-1", ""},
		{http.MethodPut, "/api/subscriptions/sub-1/settings", `{"fetch_interval_minutes": 60}`},
		{http.MethodPost, "/api/subscriptions/sub-1/resume", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
			req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "test-token"})
			req.Header.Set("X-CSRF-Token", "test-token")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Result().StatusCode == http.StatusNotFound {
				t.Errorf("%s %s returned 404, route not found", tt.method, tt.path)
			}
		})
	}
}

// TestNewRouter_UserRoutes_WithdrawEndpoint は退会エンドポイントが登録されていることを検証する。
func TestNewRouter_UserRoutes_WithdrawEndpoint(t *testing.T) {
	router, _ := createTestRouter()

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "test-token"})
	req.Header.Set("X-CSRF-Token", "test-token")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode == http.StatusNotFound {
		t.Errorf("DELETE /api/users/me returned 404, route not found")
	}
	if w.Result().StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /api/users/me status = %d, want %d", w.Result().StatusCode, http.StatusNoContent)
	}
}
