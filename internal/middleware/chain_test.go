package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// TestMiddlewareChain_Session_GETRequest は
// Session ミドルウェアでGETリクエストが通ることを検証する。
func TestMiddlewareChain_Session_GETRequest(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			return &model.Session{
				ID:        "valid-session",
				UserID:    "user-chain-test",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	sessionMW := NewSessionMiddleware(repo)

	var capturedUserID string
	handler := sessionMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := UserIDFromContext(r.Context())
		capturedUserID = userID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if capturedUserID != "user-chain-test" {
		t.Errorf("userID = %q, want %q", capturedUserID, "user-chain-test")
	}
}

// TestMiddlewareChain_Session_POSTRequest_WithValidSession は
// Session ミドルウェアでPOSTリクエストがセッション付きで通ることを検証する。
func TestMiddlewareChain_Session_POSTRequest_WithValidSession(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			return &model.Session{
				ID:        "valid-session",
				UserID:    "user-post-test",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	sessionMW := NewSessionMiddleware(repo)

	handlerCalled := false
	handler := sessionMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

// TestMiddlewareChain_NoSession_Returns401 は
// セッションがない場合に401が返されることを検証する。
func TestMiddlewareChain_NoSession_Returns401(t *testing.T) {
	repo := &mockSessionRepository{}

	sessionMW := NewSessionMiddleware(repo)

	handler := sessionMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// セッション未認証で401が返ること
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
	}
}
