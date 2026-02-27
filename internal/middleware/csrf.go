package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
)

const (
	// csrfCookieName はCSRFトークンを保持するCookieの名前。
	// フロントエンドからJavaScriptで読み取れるよう、HttpOnlyではない。
	csrfCookieName = "csrf_token"

	// csrfHeaderName はリクエストヘッダーからCSRFトークンを読み取る際のヘッダー名。
	csrfHeaderName = "X-CSRF-Token"
)

// CSRFConfig はCSRFミドルウェアの設定。
type CSRFConfig struct {
	CookieSecure bool
	CookieDomain string
}

// NewCSRFMiddleware はCSRFトークンの生成・検証ミドルウェアを返す。
// 安全なメソッド（GET, HEAD, OPTIONS）はトークン検証をスキップし、
// CSRFトークンCookieを設定する。
// 状態変更メソッド（POST, PUT, PATCH, DELETE）はトークン検証を必須とする。
func NewCSRFMiddleware(config CSRFConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 安全なメソッドはトークン検証をスキップ
			if isSafeMethod(r.Method) {
				// CSRFトークンCookieが未設定の場合は設定する
				ensureCSRFCookie(w, r, config)
				next.ServeHTTP(w, r)
				return
			}

			// 状態変更メソッド: CSRFトークンを検証
			cookieToken, err := r.Cookie(csrfCookieName)
			if err != nil || cookieToken.Value == "" {
				slog.Warn("CSRF validation failed: missing cookie token",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}

			headerToken := r.Header.Get(csrfHeaderName)
			if headerToken == "" {
				slog.Warn("CSRF validation failed: missing header token",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}

			if cookieToken.Value != headerToken {
				slog.Warn("CSRF validation failed: token mismatch",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// NewCSRFTokenHandler はCSRFトークン取得エンドポイントのハンドラーを返す。
// GET /api/csrf-token
// 既存のCSRFトークンCookieがある場合はそれを返し、なければ新規生成する。
func NewCSRFTokenHandler(config CSRFConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string

		// 既存のCSRFトークンCookieを確認
		cookie, err := r.Cookie(csrfCookieName)
		if err == nil && cookie.Value != "" {
			token = cookie.Value
		} else {
			// 新規トークンを生成
			token, err = generateCSRFToken()
			if err != nil {
				slog.Error("failed to generate CSRF token", slog.String("error", err.Error()))
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			// CookieにCSRFトークンを設定
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				Domain:   config.CookieDomain,
				MaxAge:   86400, // 24時間
				HttpOnly: false, // フロントエンドから読み取り可能
				Secure:   config.CookieSecure,
				SameSite: http.SameSiteLaxMode,
			})
		}

		// JSONでトークンを返す
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token": token,
		})
	})
}

// isSafeMethod はHTTPメソッドが安全（読み取り専用）かどうかを判定する。
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// ensureCSRFCookie はCSRFトークンCookieが未設定の場合に設定する。
func ensureCSRFCookie(w http.ResponseWriter, r *http.Request, config CSRFConfig) {
	_, err := r.Cookie(csrfCookieName)
	if err == nil {
		// 既にCookieが設定されている
		return
	}

	token, err := generateCSRFToken()
	if err != nil {
		slog.Error("failed to generate CSRF token", slog.String("error", err.Error()))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		Domain:   config.CookieDomain,
		MaxAge:   86400, // 24時間
		HttpOnly: false, // フロントエンドから読み取り可能
		Secure:   config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// generateCSRFToken は暗号的に安全なCSRFトークンを生成する。
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
