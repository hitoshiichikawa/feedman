package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware_SetsHeaders(t *testing.T) {
	mw := NewCORSMiddleware("http://localhost:3000")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	tests := []struct {
		header string
		want   string
	}{
		{"Access-Control-Allow-Origin", "http://localhost:3000"},
		{"Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS"},
		{"Access-Control-Allow-Headers", "Content-Type"},
		{"Access-Control-Allow-Credentials", "true"},
		{"Access-Control-Max-Age", "86400"},
	}

	for _, tt := range tests {
		got := resp.Header.Get(tt.header)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestCORSMiddleware_OptionsRequest_Returns204(t *testing.T) {
	mw := NewCORSMiddleware("http://localhost:3000")

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if handlerCalled {
		t.Error("next handler should not be called for OPTIONS preflight")
	}

	// CORSヘッダーもOPTIONSレスポンスに含まれること
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
	}
}

func TestCORSMiddleware_POSTRequest_PassesThroughWithHeaders(t *testing.T) {
	mw := NewCORSMiddleware("https://app.example.com")

	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/feeds", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if !handlerCalled {
		t.Error("next handler should be called for POST request")
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
}
