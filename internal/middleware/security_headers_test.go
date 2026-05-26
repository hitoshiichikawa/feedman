package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newSecurityHeadersHandler はテスト用に HSTS フラグ・X-Forwarded-Proto を組み合わせて
// セキュリティヘッダーミドルウェアを適用したハンドラを返す。
func newSecurityHeadersHandler(hstsEnabled bool) http.Handler {
	mw := NewSecurityHeadersMiddleware(hstsEnabled)
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

// TestSecurityHeaders_CSP は CSP ヘッダーが全レスポンスに付与されることを検証する。
// Requirement 1（1.1/1.2/1.3/1.4）に対応。
func TestSecurityHeaders_CSP(t *testing.T) {
	const wantCSP = "default-src 'none'; frame-ancestors 'none'"

	t.Run("HTTPリクエストのときCSPが付与される", func(t *testing.T) {
		// Arrange
		handler := newSecurityHeadersHandler(false)
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Forwarded-Proto", "http")
		w := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(w, req)

		// Assert
		got := w.Result().Header.Get("Content-Security-Policy")
		if got != wantCSP {
			t.Errorf("Content-Security-Policy = %q, want %q", got, wantCSP)
		}
	})

	t.Run("HTTPS判定のときHTTPと同一のCSPが付与される", func(t *testing.T) {
		// Arrange
		handler := newSecurityHeadersHandler(true)
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(w, req)

		// Assert
		got := w.Result().Header.Get("Content-Security-Policy")
		if got != wantCSP {
			t.Errorf("Content-Security-Policy = %q, want %q", got, wantCSP)
		}
	})
}

// TestSecurityHeaders_HSTS は HSTS ヘッダーの条件付き付与を検証する。
// Requirement 2（2.1/2.2/2.3/2.4）と Requirement 3（3.1/3.2）に対応。
func TestSecurityHeaders_HSTS(t *testing.T) {
	const wantHSTS = "max-age=31536000; includeSubDomains"

	tests := []struct {
		name            string
		hstsEnabled     bool
		forwardedProto  string // 空文字はヘッダー欠落を表す
		wantHSTSPresent bool
		wantHSTSValue   string
	}{
		{
			name:            "HSTS有効かつXForwardedProtoがhttpsのときHSTSが付与される",
			hstsEnabled:     true,
			forwardedProto:  "https",
			wantHSTSPresent: true,
			wantHSTSValue:   wantHSTS,
		},
		{
			name:            "HSTS有効かつXForwardedProtoがhttpのときHSTSが付与されない",
			hstsEnabled:     true,
			forwardedProto:  "http",
			wantHSTSPresent: false,
		},
		{
			name:            "HSTS有効かつXForwardedProtoが欠落のときHSTSが付与されない",
			hstsEnabled:     true,
			forwardedProto:  "",
			wantHSTSPresent: false,
		},
		{
			name:            "HSTS無効かつHTTPS判定でもHSTSが付与されない",
			hstsEnabled:     false,
			forwardedProto:  "https",
			wantHSTSPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			handler := newSecurityHeadersHandler(tt.hstsEnabled)
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			w := httptest.NewRecorder()

			// Act
			handler.ServeHTTP(w, req)

			// Assert
			got, present := w.Result().Header["Strict-Transport-Security"]
			if present != tt.wantHSTSPresent {
				t.Fatalf("Strict-Transport-Security present = %v, want %v", present, tt.wantHSTSPresent)
			}
			if tt.wantHSTSPresent && got[0] != tt.wantHSTSValue {
				t.Errorf("Strict-Transport-Security = %q, want %q", got[0], tt.wantHSTSValue)
			}
		})
	}
}

// TestSecurityHeaders_HSTS_NoPreload は HSTS 値に preload が含まれないことを検証する。
// requirements.md Open Questions 確認事項 1 の「preload 非付与」方針に対応。
func TestSecurityHeaders_HSTS_NoPreload(t *testing.T) {
	// Arrange
	handler := newSecurityHeadersHandler(true)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	got := w.Result().Header.Get("Strict-Transport-Security")
	if got == "" {
		t.Fatal("Strict-Transport-Security should be present for HTTPS with HSTS enabled")
	}
	if strings.Contains(got, "preload") {
		t.Errorf("Strict-Transport-Security = %q, should not contain preload", got)
	}
}

// TestSecurityHeaders_HSTS_DirectTLS は r.TLS による直結 TLS 検知のフォールバックを検証する。
// Requirement 2.1（HTTPS 配信判定）に対応。
func TestSecurityHeaders_HSTS_DirectTLS(t *testing.T) {
	const wantHSTS = "max-age=31536000; includeSubDomains"

	// Arrange
	handler := newSecurityHeadersHandler(true)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.TLS = &tls.ConnectionState{}

	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	got := w.Result().Header.Get("Strict-Transport-Security")
	if got != wantHSTS {
		t.Errorf("Strict-Transport-Security = %q, want %q (direct TLS should be detected as HTTPS)", got, wantHSTS)
	}
}

// TestSecurityHeaders_ExistingHeaders は既存 4 ヘッダーの値が維持されることを検証する。
// Requirement 4（4.1/4.2/4.3/4.4）と NFR 1.1 に対応。
func TestSecurityHeaders_ExistingHeaders(t *testing.T) {
	// Arrange
	handler := newSecurityHeadersHandler(false)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
	}
	for _, tt := range tests {
		got := resp.Header.Get(tt.header)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

// TestSecurityHeaders_NoDuplicateHeaders は各セキュリティヘッダーが 1 値のみ設定されることを検証する。
// NFR 2.2（重複ヘッダーを生成しない）に対応。
func TestSecurityHeaders_NoDuplicateHeaders(t *testing.T) {
	// Arrange
	handler := newSecurityHeadersHandler(true)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	headers := []string{
		"Content-Security-Policy",
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
	}
	for _, h := range headers {
		if n := len(resp.Header[h]); n != 1 {
			t.Errorf("header %s appears %d times, want 1", h, n)
		}
	}
}
