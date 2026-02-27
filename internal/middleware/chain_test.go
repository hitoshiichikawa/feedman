package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// TestMiddlewareChain_SessionThenCSRF_GETRequest は
// Session -> CSRF のミドルウェアチェーンでGETリクエストが通ることを検証する。
func TestMiddlewareChain_SessionThenCSRF_GETRequest(t *testing.T) {
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
	csrfMW := NewCSRFMiddleware(CSRFConfig{CookieSecure: false})

	var capturedUserID string
	handler := sessionMW(csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := UserIDFromContext(r.Context())
		capturedUserID = userID
		w.WriteHeader(http.StatusOK)
	})))

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

// TestMiddlewareChain_SessionThenCSRF_POSTRequest_WithValidToken は
// Session -> CSRF のミドルウェアチェーンでPOSTリクエストがCSRFトークン付きで通ることを検証する。
func TestMiddlewareChain_SessionThenCSRF_POSTRequest_WithValidToken(t *testing.T) {
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
	csrfMW := NewCSRFMiddleware(CSRFConfig{CookieSecure: false})

	handlerCalled := false
	handler := sessionMW(csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-token-123"})
	req.Header.Set(csrfHeaderName, "csrf-token-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

// TestMiddlewareChain_SessionThenCSRF_POSTRequest_NoCSRFToken は
// Session -> CSRF のミドルウェアチェーンで、セッション有効だがCSRFトークンなしのPOSTが403になることを検証する。
func TestMiddlewareChain_SessionThenCSRF_POSTRequest_NoCSRFToken(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			return &model.Session{
				ID:        "valid-session",
				UserID:    "user-no-csrf",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	sessionMW := NewSessionMiddleware(repo)
	csrfMW := NewCSRFMiddleware(CSRFConfig{CookieSecure: false})

	handler := sessionMW(csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
}

// TestMiddlewareChain_NoSession_Returns401BeforeCSRF は
// セッションがない場合に401がCSRF検証の前に返されることを検証する。
func TestMiddlewareChain_NoSession_Returns401BeforeCSRF(t *testing.T) {
	repo := &mockSessionRepository{}

	sessionMW := NewSessionMiddleware(repo)
	csrfMW := NewCSRFMiddleware(CSRFConfig{CookieSecure: false})

	handler := sessionMW(csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// セッション未認証で401が返ること（403ではない）
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
	}
}

// TestCSRFTokenEndpoint_FullFlow はCSRFトークンエンドポイントからトークンを取得し、
// そのトークンで状態変更リクエストが成功するフローを検証する。
func TestCSRFTokenEndpoint_FullFlow(t *testing.T) {
	csrfConfig := CSRFConfig{CookieSecure: false}

	// 1. CSRFトークンを取得
	tokenHandler := NewCSRFTokenHandler(csrfConfig)
	tokenReq := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	tokenW := httptest.NewRecorder()
	tokenHandler.ServeHTTP(tokenW, tokenReq)

	tokenResp := tokenW.Result()
	var tokenBody struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenBody); err != nil {
		t.Fatalf("failed to decode token response: %v", err)
	}

	// Cookieからもトークンを取得
	var csrfCookieValue string
	for _, c := range tokenResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookieValue = c.Value
			break
		}
	}

	if tokenBody.Token != csrfCookieValue {
		t.Fatalf("token mismatch: body=%q, cookie=%q", tokenBody.Token, csrfCookieValue)
	}

	// 2. 取得したトークンを使ってPOSTリクエスト
	csrfMW := NewCSRFMiddleware(csrfConfig)
	handlerCalled := false
	protectedHandler := csrfMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	postReq := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfCookieValue})
	postReq.Header.Set(csrfHeaderName, tokenBody.Token)
	postW := httptest.NewRecorder()

	protectedHandler.ServeHTTP(postW, postReq)

	if postW.Result().StatusCode != http.StatusOK {
		t.Errorf("POST with valid token: status = %d, want %d", postW.Result().StatusCode, http.StatusOK)
	}
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}
