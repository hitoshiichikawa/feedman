package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFMiddleware_GETRequest_PassesThroughWithoutToken(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("handler should have been called for GET request")
	}
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
}

func TestCSRFMiddleware_HEADRequest_PassesThroughWithoutToken(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("handler should have been called for HEAD request")
	}
}

func TestCSRFMiddleware_OPTIONSRequest_PassesThroughWithoutToken(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("handler should have been called for OPTIONS request")
	}
}

func TestCSRFMiddleware_POSTRequest_NoCookie_Returns403(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_POSTRequest_NoHeader_Returns403(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token-abc"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_POSTRequest_MismatchToken_Returns403(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token-abc"})
	req.Header.Set(csrfHeaderName, "wrong-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_POSTRequest_ValidToken_PassesThrough(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "valid-token"})
	req.Header.Set(csrfHeaderName, "valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("handler should have been called with valid token")
	}
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
}

func TestCSRFMiddleware_PUTRequest_ValidToken_PassesThrough(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "valid-token"})
	req.Header.Set(csrfHeaderName, "valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("handler should have been called for PUT with valid token")
	}
}

func TestCSRFMiddleware_PATCHRequest_NoToken_Returns403(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPatch, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_DELETERequest_NoToken_Returns403(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_GETRequest_SetsCSRFCookie(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "example.com",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	var csrfCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}

	if csrfCookie == nil {
		t.Fatal("expected CSRF cookie to be set on GET request")
	}
	if csrfCookie.Value == "" {
		t.Error("CSRF cookie value should not be empty")
	}
	if csrfCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("CSRF cookie SameSite = %v, want %v", csrfCookie.SameSite, http.SameSiteLaxMode)
	}
	if csrfCookie.HttpOnly {
		t.Error("CSRF cookie should NOT be HttpOnly (frontend needs to read it)")
	}
	if csrfCookie.Path != "/" {
		t.Errorf("CSRF cookie Path = %q, want %q", csrfCookie.Path, "/")
	}
}

func TestCSRFMiddleware_GETRequest_ExistingCookie_DoesNotReplace(t *testing.T) {
	mw := NewCSRFMiddleware(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "existing-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	// 既存のCookieがある場合、新しいCookieは設定しない
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookieName {
			t.Error("CSRF cookie should not be re-set when already present")
		}
	}
}

// --- CSRFトークン取得エンドポイントのテスト ---

func TestCSRFTokenHandler_SetsTokenCookieAndReturnsJSON(t *testing.T) {
	h := NewCSRFTokenHandler(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Content-Typeの確認
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	// レスポンスボディからトークンを取得
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Token == "" {
		t.Error("expected non-empty token in response")
	}

	// CSRFCookieが設定されていること
	var csrfCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected CSRF cookie to be set")
	}
	if csrfCookie.Value != body.Token {
		t.Errorf("cookie value = %q, response token = %q; should match", csrfCookie.Value, body.Token)
	}
	if csrfCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", csrfCookie.SameSite)
	}
}

func TestCSRFTokenHandler_ExistingCookie_ReturnsSameToken(t *testing.T) {
	h := NewCSRFTokenHandler(CSRFConfig{
		CookieSecure: false,
		CookieDomain: "",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "existing-csrf-token"})
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	resp := w.Result()
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Token != "existing-csrf-token" {
		t.Errorf("token = %q, want %q (existing token should be returned)", body.Token, "existing-csrf-token")
	}
}

// --- 状態変更メソッドのテスト ---

func TestCSRFMiddleware_AllStateMutatingMethods_RequireToken(t *testing.T) {
	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			mw := NewCSRFMiddleware(CSRFConfig{
				CookieSecure: false,
				CookieDomain: "",
			})

			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called for " + method + " without token")
			}))

			req := httptest.NewRequest(method, "/api/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Result().StatusCode != http.StatusForbidden {
				t.Errorf("%s: status = %d, want %d", method, w.Result().StatusCode, http.StatusForbidden)
			}
		})
	}
}
