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

// --- Passkey 未認証 4 endpoint の IP 単位レート制限（Issue #216 / Req 6.1〜6.5） ---

// newPasskeyRateLimitRouter は PasskeyHandler + UnauthIPRateLimiter を指定 burst で
// 注入した router を構築する。burst=1 なら同一 IP の 2 回目で 429 になる。
func newPasskeyRateLimitRouter(burst int) (http.Handler, *stubPasskeyRouterRegistration,
	*stubPasskeyRouterAuthentication, *middleware.IPRateLimiter) {
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	ipRL := middleware.NewIPRateLimiter(middleware.IPRateLimiterConfig{
		Rate:            rate.Limit(1),
		Burst:           burst,
		CleanupInterval: 1 * time.Minute,
	})
	deps.UnauthIPRateLimiter = ipRL
	return NewRouter(deps), reg, authn, ipRL
}

// passkeyUnauthRouteCase は passkey 未認証 4 endpoint の path / リクエストボディ / 通過時
// 期待 status を共通化する table。
type passkeyUnauthRouteCase struct {
	name       string
	path       string
	body       string
	wantStatus int
}

func passkeyUnauthRouteCases() []passkeyUnauthRouteCase {
	return []passkeyUnauthRouteCase{
		{
			name: "registration_begin", path: "/api/passkey/registration/begin",
			body:       `{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "registration_finish", path: "/api/passkey/registration/finish",
			body:       `{"challenge_id":"c","credential":{"raw":true}}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "authentication_begin", path: "/api/passkey/authentication/begin",
			body:       `{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "authentication_finish", path: "/api/passkey/authentication/finish",
			body:       `{"challenge_id":"c","credential":{"raw":true}}`,
			wantStatus: http.StatusOK,
		},
	}
}

// doPostRateLimit は指定 path / body / remoteAddr で POST リクエストを送るヘルパ。
func doPostRateLimit(router http.Handler, path, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestNewRouter_PasskeyUnauthIPRateLimit_429OnExcess は passkey 未認証 4 endpoint が
// 同一 IP 閾値超過で 429 を返し、handler 以降に到達しないことを検証する
// （Req 6.1 / 6.2 / 6.5）。
func TestNewRouter_PasskeyUnauthIPRateLimit_429OnExcess(t *testing.T) {
	for _, tc := range passkeyUnauthRouteCases() {
		t.Run(tc.name+"で同一IP超過のとき429を返し handler に到達しない", func(t *testing.T) {
			// Arrange: burst=1（1 回目で枯渇）
			router, _, _, ipRL := newPasskeyRateLimitRouter(1)
			defer ipRL.Stop()

			// Act 1: 1 回目は閾値以内なので通常応答（Req 6.3）
			w1 := doPostRateLimit(router, tc.path, tc.body, "203.0.113.10:60000")
			if w1.Result().StatusCode != tc.wantStatus {
				t.Fatalf("1st status = %d, want %d (閾値以内は通過 / Req 6.3)",
					w1.Result().StatusCode, tc.wantStatus)
			}

			// Act 2: 2 回目は超過 → 429
			w2 := doPostRateLimit(router, tc.path, tc.body, "203.0.113.10:60001")

			// Assert: 429 + Retry-After（既存未認証ルートと同一形式 / Req 6.5）
			resp := w2.Result()
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("2nd status = %d, want %d (Req 6.1 / 6.2)",
					resp.StatusCode, http.StatusTooManyRequests)
			}
			if resp.Header.Get("Retry-After") == "" {
				t.Error("Retry-After header is empty (Req 6.5: 既存応答形式)")
			}
		})
	}
}

// TestNewRouter_PasskeyUnauthIPRateLimit_SameShapeAsExistingRoutes は passkey 429 応答が
// 既存未認証 route（/health）の 429 応答と同一形式（status / Content-Type / body /
// Retry-After 有無）を持つことを検証する（Req 6.5）。
func TestNewRouter_PasskeyUnauthIPRateLimit_SameShapeAsExistingRoutes(t *testing.T) {
	// Arrange: 基準となる /health の 429 応答を取得
	router, _, _, ipRL := newPasskeyRateLimitRouter(1)
	defer ipRL.Stop()
	doRouterReq(router, http.MethodGet, "/health", "203.0.113.70:60000")
	baseW := doRouterReq(router, http.MethodGet, "/health", "203.0.113.70:60001")
	baseResp := baseW.Result()
	if baseResp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("health 2nd status = %d, want 429", baseResp.StatusCode)
	}

	// Act: passkey 未認証 route の 429 応答（別 IP で burst 消費 → 超過）
	body := `{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`
	doPostRateLimit(router, "/api/passkey/authentication/begin", body, "203.0.113.80:60000")
	w := doPostRateLimit(router, "/api/passkey/authentication/begin", body, "203.0.113.80:60001")
	resp := w.Result()

	// Assert: status / Content-Type / body / Retry-After が /health と一致（Req 6.5）
	if resp.StatusCode != baseResp.StatusCode {
		t.Errorf("status = %d, want %d", resp.StatusCode, baseResp.StatusCode)
	}
	if got, want := resp.Header.Get("Content-Type"), baseResp.Header.Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After header is empty")
	}
	if got, want := w.Body.String(), baseW.Body.String(); got != want {
		t.Errorf("429 body = %q, want %q (既存未認証 route と同一形式 / Req 6.5)", got, want)
	}
}

// TestNewRouter_PasskeyUnauthIPRateLimit_IndependentPerIP は passkey 未認証 route の
// レート制限が IP ごとに独立にカウントされることを検証する（Req 6.4）。
func TestNewRouter_PasskeyUnauthIPRateLimit_IndependentPerIP(t *testing.T) {
	// Arrange: burst=1
	router, _, authn, ipRL := newPasskeyRateLimitRouter(1)
	defer ipRL.Stop()
	body := `{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`

	// IP A 1 回目通過
	if w := doPostRateLimit(router, "/api/passkey/authentication/begin", body, "203.0.113.90:60000"); w.Result().StatusCode != http.StatusOK {
		t.Fatalf("IP A 1st status = %d, want 200", w.Result().StatusCode)
	}
	// IP A 2 回目 429
	if w := doPostRateLimit(router, "/api/passkey/authentication/begin", body, "203.0.113.90:60001"); w.Result().StatusCode != http.StatusTooManyRequests {
		t.Fatalf("IP A 2nd status = %d, want 429", w.Result().StatusCode)
	}

	// Act: 別 IP B からの要求
	w := doPostRateLimit(router, "/api/passkey/authentication/begin", body, "198.51.100.50:60000")

	// Assert: IP B は独立カウントで通過する（Req 6.4）
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("IP B status = %d, want 200 (IP独立カウント / Req 6.4)", w.Result().StatusCode)
	}
	if authn.beginCalled != 2 {
		t.Errorf("authn.beginCalled = %d, want 2 (IP A 1回 + IP B 1回)", authn.beginCalled)
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
