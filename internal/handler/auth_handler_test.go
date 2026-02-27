package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

type mockAuthService struct {
	getLoginURLFn   func(state string) string
	handleCallbackFn func(ctx context.Context, code string) (*model.Session, error)
	logoutFn         func(ctx context.Context, sessionID string) error
	getCurrentUserFn func(ctx context.Context, sessionID string) (*model.User, error)
}

func (m *mockAuthService) GetLoginURL(state string) string {
	if m.getLoginURLFn != nil {
		return m.getLoginURLFn(state)
	}
	return ""
}

func (m *mockAuthService) HandleCallback(ctx context.Context, code string) (*model.Session, error) {
	if m.handleCallbackFn != nil {
		return m.handleCallbackFn(ctx, code)
	}
	return nil, nil
}

func (m *mockAuthService) Logout(ctx context.Context, sessionID string) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, sessionID)
	}
	return nil
}

func (m *mockAuthService) GetCurrentUser(ctx context.Context, sessionID string) (*model.User, error) {
	if m.getCurrentUserFn != nil {
		return m.getCurrentUserFn(ctx, sessionID)
	}
	return nil, nil
}

// --- テスト ---

func TestAuthHandler_Login_RedirectsToOAuthURL(t *testing.T) {
	svc := &mockAuthService{
		getLoginURLFn: func(state string) string {
			return "https://accounts.google.com/o/oauth2/auth?state=" + state
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		CookieDomain:  "",
		CookieSecure:  false,
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()

	h.Login(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		t.Fatal("expected Location header")
	}
	if !containsStr(location, "accounts.google.com") {
		t.Errorf("Location = %q, should contain google oauth URL", location)
	}
}

func TestAuthHandler_Callback_Success_SetsCookieAndRedirects(t *testing.T) {
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{
				ID:        "session-id-abc",
				UserID:    "user-id-123",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		CookieDomain:  "",
		CookieSecure:  false,
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	// stateの検証のためにcookieを設定
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	w := httptest.NewRecorder()

	h.Callback(w, req)

	resp := w.Result()

	// リダイレクトされること
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	// BaseURLにリダイレクトされること
	location := resp.Header.Get("Location")
	if location != "http://localhost:3000" {
		t.Errorf("Location = %q, want %q", location, "http://localhost:3000")
	}

	// セッションCookieが設定されること
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be set")
	}
	if sessionCookie.Value != "session-id-abc" {
		t.Errorf("session cookie value = %q, want %q", sessionCookie.Value, "session-id-abc")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want %v", sessionCookie.SameSite, http.SameSiteLaxMode)
	}
}

func TestAuthHandler_Callback_MissingCode_ReturnsBadRequest(t *testing.T) {
	h := NewAuthHandler(&mockAuthService{}, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	w := httptest.NewRecorder()

	h.Callback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestAuthHandler_Callback_StateMismatch_ReturnsBadRequest(t *testing.T) {
	h := NewAuthHandler(&mockAuthService{}, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct-state"})
	w := httptest.NewRecorder()

	h.Callback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestAuthHandler_Callback_AuthServiceError_ReturnsInternalError(t *testing.T) {
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return nil, errors.New("auth failed")
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=bad-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	w := httptest.NewRecorder()

	h.Callback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestAuthHandler_Logout_Success_ClearsCookieAndRedirects(t *testing.T) {
	svc := &mockAuthService{
		logoutFn: func(ctx context.Context, sessionID string) error {
			return nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		CookieDomain:  "",
		CookieSecure:  false,
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-to-logout"})
	w := httptest.NewRecorder()

	h.Logout(w, req)

	resp := w.Result()

	// リダイレクトされること
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	// セッションCookieがクリアされること
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be cleared")
	}
	if sessionCookie.MaxAge != -1 {
		t.Errorf("session cookie MaxAge = %d, want -1 (delete)", sessionCookie.MaxAge)
	}
}

func TestAuthHandler_Logout_NoSession_StillRedirects(t *testing.T) {
	h := NewAuthHandler(&mockAuthService{}, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()

	h.Logout(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
}

func TestAuthHandler_Me_Authenticated_ReturnsUserJSON(t *testing.T) {
	svc := &mockAuthService{
		getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
			return &model.User{
				ID:    "user-id-me",
				Email: "me@example.com",
				Name:  "Me User",
			}, nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	h.Me(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
}

func TestAuthHandler_Me_NoSession_ReturnsUnauthorized(t *testing.T) {
	h := NewAuthHandler(&mockAuthService{}, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	w := httptest.NewRecorder()

	h.Me(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// containsStr は文字列sにsubstrが含まれるかチェックするヘルパー。
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
