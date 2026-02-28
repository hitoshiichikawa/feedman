// Package handler はHTTPハンドラーを提供する。
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/hitoshi/feedman/internal/model"
)

const (
	sessionCookieName = "session_id"
	oauthStateCookie  = "oauth_state"
)

// AuthServiceInterface は認証ハンドラーが必要とするサービスインターフェース。
type AuthServiceInterface interface {
	GetLoginURL(state string) string
	HandleCallback(ctx context.Context, code string) (*model.Session, error)
	Logout(ctx context.Context, sessionID string) error
	GetCurrentUser(ctx context.Context, sessionID string) (*model.User, error)
}

// AuthHandlerConfig は認証ハンドラーの設定。
type AuthHandlerConfig struct {
	BaseURL       string
	CookieDomain  string
	CookieSecure  bool
	SessionMaxAge int // セッションCookieの有効期間（秒）
}

// AuthHandler はOAuth認証関連のHTTPハンドラー。
type AuthHandler struct {
	service AuthServiceInterface
	config  AuthHandlerConfig
}

// NewAuthHandler はAuthHandlerを生成する。
func NewAuthHandler(service AuthServiceInterface, config AuthHandlerConfig) *AuthHandler {
	return &AuthHandler{
		service: service,
		config:  config,
	}
}

// Login はGoogle OAuthフローを開始する。
// GET /auth/google/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		slog.Error("failed to generate oauth state", slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// stateをCookieに保存（CSRF対策）
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10分
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	url := h.service.GetLoginURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Callback はOAuthコールバックを処理する。
// GET /auth/google/callback?code=xxx&state=yyy
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// 1. stateの検証（CSRF対策）
	state := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value != state {
		slog.Warn("oauth state mismatch",
			slog.String("query_state", state),
		)
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}

	// stateクッキーを削除
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// 2. 認可コードの取得
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	// 3. 認証処理
	session, err := h.service.HandleCallback(r.Context(), code)
	if err != nil {
		slog.Error("oauth callback failed", slog.String("error", err.Error()))
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// 4. セッションCookieを設定（HTTP Only）
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Domain:   h.config.CookieDomain,
		MaxAge:   h.config.SessionMaxAge,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// 5. フロントエンドにリダイレクト
	http.Redirect(w, r, h.config.BaseURL, http.StatusTemporaryRedirect)
}

// Logout はセッションを破棄する。
// POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// セッションCookieの取得
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		// セッションをDBから削除
		if logoutErr := h.service.Logout(r.Context(), cookie.Value); logoutErr != nil {
			slog.Error("failed to logout", slog.String("error", logoutErr.Error()))
			// ログアウト失敗してもCookieはクリアする
		}
	}

	// セッションCookieをクリア
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   h.config.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.config.BaseURL, http.StatusTemporaryRedirect)
}

// Me は現在のログインユーザー情報を返す。
// GET /auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.service.GetCurrentUser(r.Context(), cookie.Value)
	if err != nil {
		slog.Error("failed to get current user", slog.String("error", err.Error()))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
		"name":  user.Name,
	})
}

// generateState はCSRF対策用のランダムなstate値を生成する。
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
