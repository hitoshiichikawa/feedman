package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// failingHealthChecker はPingContextで常にエラーを返すヘルスチェッカー。
// /health で 503 (5xx) を発生させ、ログのステータス整合性を検証するために使う。
type failingHealthChecker struct{}

func (failingHealthChecker) PingContext(ctx context.Context) error {
	return context.DeadlineExceeded
}

// createTestRouterWithLogger はテスト用ロガーを注入したルーターを構築するヘルパー。
// 返り値の *bytes.Buffer にアクセスログのJSONが書き込まれる。
func createTestRouterWithLogger(t *testing.T, health HealthChecker) (http.Handler, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

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
		HealthChecker:     health,
		Logger:            logger,
		SessionFinder:     sessionFinder,
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService: &mockAuthService{
			getLoginURLFn: func(state string) string {
				return "https://accounts.google.com?state=" + state
			},
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				return &model.User{ID: "user-test-1", Email: "test@example.com", Name: "Test"}, nil
			},
		},
		AuthConfig: AuthHandlerConfig{BaseURL: "http://localhost:3000", SessionMaxAge: 86400},
		FeedService: &mockFeedService{
			getFeedFn: func(ctx context.Context, userID, feedID string) (*model.Feed, error) {
				return &model.Feed{ID: feedID, FeedURL: "https://example.com/feed.xml", Title: "Test"}, nil
			},
		},
		SubscriptionDeleter: &mockSubscriptionDeleter{},
		ItemService:         &mockItemService{},
		ItemStateService:    &mockItemStateService{},
		SubscriptionService: &mockSubscriptionService{
			listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
				return []subscriptionResponse{}, nil
			},
		},
		UserService: &mockUserService{},
	}

	return NewRouter(deps), &buf
}

// parseLogEntries は buf に書き込まれた1行1JSONのログをパースして返す。
func parseLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]interface{} {
	t.Helper()

	var entries []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse JSON log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// accessLogEntries は "http_request" メッセージのアクセスログのみを抽出する。
func accessLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]interface{} {
	t.Helper()

	var access []map[string]interface{}
	for _, e := range parseLogEntries(t, buf) {
		if e["msg"] == "http_request" {
			access = append(access, e)
		}
	}
	return access
}

// TestNewRouter_Logging_EmitsSingleAccessLogPerEndpoint は
// /health・/auth/*・/api/* の各登録済みエンドポイントで
// アクセスログがちょうど1件出力される（二重ログにならない）ことを検証する。
// AC 1.2, 1.3, 1.4, 1.5
func TestNewRouter_Logging_EmitsSingleAccessLogPerEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		cookie bool
	}{
		{"health endpoint", http.MethodGet, "/health", false},
		{"auth route", http.MethodGet, "/auth/me", true},
		{"api route", http.MethodGet, "/api/subscriptions", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			router, buf := createTestRouterWithLogger(t, nil)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.cookie {
				req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
			}
			w := httptest.NewRecorder()

			// Act
			router.ServeHTTP(w, req)

			// Assert
			access := accessLogEntries(t, buf)
			if len(access) != 1 {
				t.Fatalf("%s %s: access log count = %d, want 1 (entries: %s)",
					tt.method, tt.path, len(access), buf.String())
			}
			if access[0]["path"] != tt.path {
				t.Errorf("logged path = %q, want %q", access[0]["path"], tt.path)
			}
		})
	}
}

// TestNewRouter_Logging_IncludesRequestFields は
// アクセスログに method・path・status・duration_ms が含まれることを検証する。
// AC 2.1
func TestNewRouter_Logging_IncludesRequestFields(t *testing.T) {
	// Arrange
	router, buf := createTestRouterWithLogger(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	access := accessLogEntries(t, buf)
	if len(access) != 1 {
		t.Fatalf("access log count = %d, want 1", len(access))
	}
	entry := access[0]
	if entry["method"] != http.MethodGet {
		t.Errorf("method = %q, want %q", entry["method"], http.MethodGet)
	}
	if entry["path"] != "/health" {
		t.Errorf("path = %q, want %q", entry["path"], "/health")
	}
	if _, ok := entry["status"]; !ok {
		t.Error("expected 'status' field in access log")
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected 'duration_ms' field in access log")
	}
}

// TestNewRouter_Logging_AuthenticatedRequest_IncludesUserID は
// 認証済み /api/* リクエストのアクセスログに user_id が含まれることを検証する。
// AC 2.2
func TestNewRouter_Logging_AuthenticatedRequest_IncludesUserID(t *testing.T) {
	// Arrange
	router, buf := createTestRouterWithLogger(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	access := accessLogEntries(t, buf)
	if len(access) != 1 {
		t.Fatalf("access log count = %d, want 1", len(access))
	}
	if access[0]["user_id"] != "user-test-1" {
		t.Errorf("user_id = %q, want %q", access[0]["user_id"], "user-test-1")
	}
}

// TestNewRouter_Logging_UnauthenticatedRequest_OmitsUserID は
// 未認証リクエストのアクセスログで user_id が空または非出力であることを検証する。
// AC 2.3
func TestNewRouter_Logging_UnauthenticatedRequest_OmitsUserID(t *testing.T) {
	// Arrange — /health は認証を通らないので user_id は付かない
	router, buf := createTestRouterWithLogger(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	access := accessLogEntries(t, buf)
	if len(access) != 1 {
		t.Fatalf("access log count = %d, want 1", len(access))
	}
	if val, ok := access[0]["user_id"]; ok && val != "" {
		t.Errorf("user_id should be empty/absent for unauthenticated request, got %q", val)
	}
}

// TestNewRouter_Logging_5xxStatusMatchesResponse は
// 5xx レスポンス時にログのステータスが実際に返された 5xx と一致することを検証する。
// AC 2.4
func TestNewRouter_Logging_5xxStatusMatchesResponse(t *testing.T) {
	// Arrange — ヘルスチェック失敗で /health が 503 を返す
	router, buf := createTestRouterWithLogger(t, failingHealthChecker{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("response status = %d, want %d", w.Result().StatusCode, http.StatusServiceUnavailable)
	}
	access := accessLogEntries(t, buf)
	if len(access) != 1 {
		t.Fatalf("access log count = %d, want 1", len(access))
	}
	status, ok := access[0]["status"].(float64)
	if !ok {
		t.Fatalf("status field missing or not numeric: %v", access[0]["status"])
	}
	if int(status) != http.StatusServiceUnavailable {
		t.Errorf("logged status = %d, want %d", int(status), http.StatusServiceUnavailable)
	}
}

// TestNewRouter_Logging_NilLogger_FallsBackToDefault は
// Logger が nil でも NewRouter が panic せず動作する（slog.Default フォールバック）ことを検証する。
// NFR 3.1 / 後方互換性
func TestNewRouter_Logging_NilLogger_FallsBackToDefault(t *testing.T) {
	// Arrange — Logger を設定しない（既存テストと同じ経路）
	router, _ := createTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Act / Assert — panic しないこと
	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
}

// TestNewRouter_Logging_ProtectedRoute_NoSession_NoUserIDLog は
// 認証必須ルートにセッション無しでアクセスした場合（401）、ログに user_id が付かないことを検証する。
// AC 2.3（境界: ログはチェーン内で出るが Session 適用前のため user_id 無し）
func TestNewRouter_Logging_ProtectedRoute_NoSession_NoUserIDLog(t *testing.T) {
	// Arrange
	router, buf := createTestRouterWithLogger(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert — 401 のため認証は通っておらず、ログが出る場合でも user_id は無い
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
	}
	for _, e := range accessLogEntries(t, buf) {
		if val, ok := e["user_id"]; ok && val != "" {
			t.Errorf("user_id should be empty for unauthenticated request, got %q", val)
		}
	}
}
