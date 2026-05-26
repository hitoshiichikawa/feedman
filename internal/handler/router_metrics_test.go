package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// newMetricsTestDeps はメトリクス登録テスト用の最小 RouterDeps を構築する。
// metricsHandler / metricsMiddleware を任意に注入できる。
func newMetricsTestDeps(metricsHandler http.Handler, metricsMiddleware func(http.Handler) http.Handler) *RouterDeps {
	sessionFinder := &mockSessionFinderForRouter{
		sessions: map[string]*model.Session{
			"valid-session": {
				ID:        "valid-session",
				UserID:    "user-test-1",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
		},
	}
	return &RouterDeps{
		SessionFinder:     sessionFinder,
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService: &mockAuthService{
			getLoginURLFn: func(state string) string {
				return "https://accounts.google.com?state=" + state
			},
			getCurrentUserFn: func(_ context.Context, _ string) (*model.User, error) {
				return &model.User{ID: "user-test-1", Email: "test@example.com", Name: "Test"}, nil
			},
		},
		AuthConfig:          AuthHandlerConfig{BaseURL: "http://localhost:3000", SessionMaxAge: 86400},
		FeedService:         &mockFeedService{},
		SubscriptionDeleter: &mockSubscriptionDeleter{},
		ItemService:         &mockItemService{},
		ItemStateService:    &mockItemStateService{},
		SubscriptionService: &mockSubscriptionService{},
		UserService:         &mockUserService{},
		MetricsHandler:      metricsHandler,
		MetricsMiddleware:   metricsMiddleware,
	}
}

// metricsTestBody は /metrics ハンドラが返すテスト用ボディ。
const metricsTestBody = "feedman_fetch_success_total 0"

// newMetricsTestHandler はメトリクス本文を返すテスト用ハンドラを返す。
func newMetricsTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(metricsTestBody))
	})
}

// TestNewRouter_Metrics_NonNilHandler_InRange_Returns200 は MetricsHandler 非 nil かつ
// CIDR 範囲内のとき /metrics が 200 でメトリクス本文を返すことを検証する（Requirement 1.1）。
func TestNewRouter_Metrics_NonNilHandler_InRange_Returns200(t *testing.T) {
	// Arrange
	mw := middleware.NewTrustedCIDRMiddleware([]string{"127.0.0.0/8"})
	router := NewRouter(newMetricsTestDeps(newMetricsTestHandler(), mw))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /metrics (範囲内) status = %d, want 200", w.Result().StatusCode)
	}
}

// TestNewRouter_Metrics_NonNilHandler_OutOfRange_Returns403 は CIDR 範囲外のとき
// /metrics が 403 を返しメトリクス本文を含めないことを検証する（Requirement 4.1, 5.1）。
func TestNewRouter_Metrics_NonNilHandler_OutOfRange_Returns403(t *testing.T) {
	// Arrange
	mw := middleware.NewTrustedCIDRMiddleware([]string{"127.0.0.0/8"})
	router := NewRouter(newMetricsTestDeps(newMetricsTestHandler(), mw))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "10.0.0.1:50000"
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("GET /metrics (範囲外) status = %d, want 403", w.Result().StatusCode)
	}
	if got := w.Body.String(); strings.Contains(got, metricsTestBody) {
		t.Errorf("403 応答にメトリクス本文が含まれている: %q", got)
	}
}

// TestNewRouter_Metrics_NilHandler_NotRegistered は MetricsHandler nil のとき
// /metrics が登録されず 404 になることを検証する（Requirement 5.1 後方互換）。
func TestNewRouter_Metrics_NilHandler_NotRegistered(t *testing.T) {
	// Arrange
	router := NewRouter(newMetricsTestDeps(nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("MetricsHandler nil のとき GET /metrics status = %d, want 404", w.Result().StatusCode)
	}
}

// TestNewRouter_Metrics_NilHandler_ExistingRoutesUnchanged は MetricsHandler nil のとき
// 既存ルート（/health・/auth/*・/api/*）のルーティング挙動が不変であることを検証する（Requirement 5.1）。
func TestNewRouter_Metrics_NilHandler_ExistingRoutesUnchanged(t *testing.T) {
	router := NewRouter(newMetricsTestDeps(nil, nil))

	cases := []struct {
		name       string
		method     string
		path       string
		withCookie bool
		wantStatus int
	}{
		{"health は 200", http.MethodGet, "/health", false, http.StatusOK},
		{"auth login は 307 リダイレクト", http.MethodGet, "/auth/google/login", false, http.StatusTemporaryRedirect},
		{"api はセッションなしで 401", http.MethodGet, "/api/subscriptions", false, http.StatusUnauthorized},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			// Act
			router.ServeHTTP(w, req)

			// Assert
			if w.Result().StatusCode != tt.wantStatus {
				t.Errorf("%s %s status = %d, want %d", tt.method, tt.path, w.Result().StatusCode, tt.wantStatus)
			}
		})
	}
}

// TestNewRouter_Metrics_NonNilHandler_ExistingRoutesUnchanged は MetricsHandler 非 nil でも
// 既存ルートのルーティング挙動が不変であることを検証する（Requirement 5.1）。
func TestNewRouter_Metrics_NonNilHandler_ExistingRoutesUnchanged(t *testing.T) {
	mw := middleware.NewTrustedCIDRMiddleware([]string{"127.0.0.0/8"})
	router := NewRouter(newMetricsTestDeps(newMetricsTestHandler(), mw))

	// /health は CIDR 制限なしでアクセスできる（/metrics 以外に CIDR mw が漏れていないこと）
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "10.0.0.1:50000" // /metrics では拒否される範囲外 IP
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("MetricsHandler 非 nil 時の GET /health status = %d, want 200（CIDR mw は /metrics のみ）", w.Result().StatusCode)
	}
}
