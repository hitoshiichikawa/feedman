package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"

	"golang.org/x/time/rate"
)

// createTestRouterWithIPRateLimit は UnauthIPRateLimiter を burst=1 で設定したルーターを構築する。
// 2 回目のリクエストで必ず IP レート制限に引っかかる構成にする。
func createTestRouterWithIPRateLimit() (http.Handler, *middleware.IPRateLimiter) {
	sessionFinder := &mockSessionFinderForRouter{
		sessions: map[string]*model.Session{
			"valid-session": {
				ID:        "valid-session",
				UserID:    "user-test-1",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
		},
	}

	ipRL := middleware.NewIPRateLimiter(middleware.IPRateLimiterConfig{
		Rate:            rate.Limit(1),
		Burst:           1,
		CleanupInterval: 1 * time.Minute,
	})

	deps := &RouterDeps{
		SessionFinder:       sessionFinder,
		CORSAllowedOrigin:   "http://localhost:3000",
		RateLimiter:         middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		UnauthIPRateLimiter: ipRL,
		AuthService: &mockAuthService{
			getLoginURLFn: func(state string) string {
				return "https://accounts.google.com?state=" + state
			},
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				return &model.User{ID: "user-test-1"}, nil
			},
		},
		AuthConfig:          AuthHandlerConfig{BaseURL: "http://localhost:3000", SessionMaxAge: 86400},
		FeedService:         &mockFeedService{},
		SubscriptionDeleter: &mockSubscriptionDeleter{},
		ItemService:         &mockItemService{},
		ItemStateService:    &mockItemStateService{},
		SubscriptionService: &mockSubscriptionService{},
		UserService:         &mockUserService{},
	}

	return NewRouter(deps), ipRL
}

// doRouterReq は指定 method・path・remoteAddr でルーターにリクエストを送る。
func doRouterReq(router http.Handler, method, path, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Req 1.1, 1.2, 1.3: login・callback・health の 3 ルートで同一 IP 超過時に 429 を返す。
func TestNewRouter_UnauthIPRateLimit_429OnExcess(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"login", "/auth/google/login"},
		{"callback", "/auth/google/callback"},
		{"health", "/health"},
	}
	for _, tt := range cases {
		t.Run(tt.name+"で同一IP超過のとき429を返す", func(t *testing.T) {
			// Arrange
			router, ipRL := createTestRouterWithIPRateLimit()
			defer ipRL.Stop()
			const addr = "203.0.113.10:50000"

			// Act: 1 回目は通過（429 以外）、2 回目は 429。
			w1 := doRouterReq(router, http.MethodGet, tt.path, addr)
			w2 := doRouterReq(router, http.MethodGet, tt.path, addr)

			// Assert
			if w1.Result().StatusCode == http.StatusTooManyRequests {
				t.Errorf("1回目: status = 429, want 通過")
			}
			if w2.Result().StatusCode != http.StatusTooManyRequests {
				t.Errorf("2回目: status = %d, want %d", w2.Result().StatusCode, http.StatusTooManyRequests)
			}
		})
	}
}

// Req 4: logout・me は IP 単位レート制限の対象外（同一 IP から連続でも 429 にならない）。
func TestNewRouter_UnauthIPRateLimit_NotAppliedToLogoutAndMe(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"logout", http.MethodPost, "/auth/logout"},
		{"me", http.MethodGet, "/auth/me"},
	}
	for _, tt := range cases {
		t.Run(tt.name+"はIP制限対象外で連続リクエストでも429にならない", func(t *testing.T) {
			// Arrange
			router, ipRL := createTestRouterWithIPRateLimit()
			defer ipRL.Stop()
			const addr = "203.0.113.20:50000"

			// Act: 同一 IP から 3 回連続。
			for i := 0; i < 3; i++ {
				w := doRouterReq(router, tt.method, tt.path, addr)
				// Assert
				if w.Result().StatusCode == http.StatusTooManyRequests {
					t.Errorf("request %d: %s %s が 429 を返した（IP制限対象外であるべき）", i, tt.method, tt.path)
				}
			}
		})
	}
}

// Req 1.5: ルーター経由でも異なる IP は独立カウントされる。
func TestNewRouter_UnauthIPRateLimit_IsolatesPerIP(t *testing.T) {
	// Arrange
	router, ipRL := createTestRouterWithIPRateLimit()
	defer ipRL.Stop()

	// Act: IP-A で burst 消費後 2 回目は 429。IP-B の 1 回目は影響を受けず通過。
	doRouterReq(router, http.MethodGet, "/health", "198.51.100.1:40000")
	wA2 := doRouterReq(router, http.MethodGet, "/health", "198.51.100.1:40000")
	wB1 := doRouterReq(router, http.MethodGet, "/health", "198.51.100.2:40000")

	// Assert
	if wA2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("IP-A 2回目: status = %d, want %d", wA2.Result().StatusCode, http.StatusTooManyRequests)
	}
	if wB1.Result().StatusCode == http.StatusTooManyRequests {
		t.Errorf("IP-B 1回目: status = 429, want 通過（IP独立カウント）")
	}
}

// NFR 2 / Req 4: UnauthIPRateLimiter が nil のとき IP 制限を適用せず既存挙動を保つ（後方互換）。
func TestNewRouter_UnauthIPRateLimit_NilLimiter_NoRestriction(t *testing.T) {
	// Arrange: createTestRouter は UnauthIPRateLimiter を設定しない（nil）。
	router, _ := createTestRouter()
	const addr = "203.0.113.30:50000"

	// Act: 同一 IP から /health に 3 回連続。
	for i := 0; i < 3; i++ {
		w := doRouterReq(router, http.MethodGet, "/health", addr)
		// Assert
		if w.Result().StatusCode == http.StatusTooManyRequests {
			t.Errorf("request %d: nil limiter で 429 を返した（制限なしであるべき）", i)
		}
	}
}
