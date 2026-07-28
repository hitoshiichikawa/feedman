// Package handler はHTTPハンドラーを提供する。
package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/model"
)

const (
	sessionCookieName = "session_id"
	oauthStateCookie  = "oauth_state"

	// oauthNativeChallengeCookie は flow=native（#165）の PKCE challenge を
	// OAuth round-trip をまたいで保持する HttpOnly Cookie。
	// 存在 = native flow の文脈として callback が解釈する。
	oauthNativeChallengeCookie = "oauth_native_challenge"
	// nativeAuthCallbackURL は native flow 完了時に auth_code を返すアプリスキーム
	// （feedman-ios SERVER.md §1.2 で固定）。
	nativeAuthCallbackURL = "feedman://auth/callback"
)

// AuthServiceInterface は認証ハンドラーが必要とするサービスインターフェース。
type AuthServiceInterface interface {
	GetLoginURL(state string) string
	HandleCallback(ctx context.Context, code string) (*model.Session, error)
	// HandleNativeCallback は native flow の callback を処理し、平文 auth_code を返す。
	// セッションは作成しない。
	HandleNativeCallback(ctx context.Context, code, pkceChallenge string) (string, error)
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
// GET /auth/google/login?flow=native&code_challenge=...&code_challenge_method=S256（#165）
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("flow") == "native" {
		// native flow: PKCE S256 パラメータを検証し、challenge を callback まで
		// Cookie で保持する。不合格時は OAuth リダイレクトを開始しない。
		challenge := r.URL.Query().Get("code_challenge")
		method := r.URL.Query().Get("code_challenge_method")
		if err := auth.ValidatePKCES256(challenge, method); err != nil {
			slog.Warn("native login rejected: invalid pkce parameters")
			http.Error(w, "invalid pkce parameters", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     oauthNativeChallengeCookie,
			Value:    challenge,
			Path:     "/",
			MaxAge:   600, // oauth_state と同じ 10 分
			HttpOnly: true,
			Secure:   h.config.CookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
	} else if _, err := r.Cookie(oauthNativeChallengeCookie); err == nil {
		// Web flow: 過去の native flow 文脈が残存している場合のみ破棄する
		// （残存がない通常リクエストの応答ヘッダは従来と完全一致に保つ）。
		h.clearNativeChallengeCookie(w)
	}

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

// clearNativeChallengeCookie は native flow 文脈 Cookie を削除する。
func (h *AuthHandler) clearNativeChallengeCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthNativeChallengeCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Callback はOAuthコールバックを処理する。
// GET /auth/google/callback?code=xxx&state=yyy
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// 1. stateの検証（CSRF対策）
	state := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie(oauthStateCookie)
	// state の比較はタイミング攻撃を避けるため定数時間比較を用いる（CWE-208）。
	if err != nil || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
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

	// native flow（#165）: login で保持した PKCE challenge Cookie が存在する場合は
	// Web セッションを発行せず auth_code をアプリスキームへ返す分岐に入る。
	// Cookie 不在時は従来の Web flow として処理する（fail-safe）。
	if nativeCookie, cookieErr := r.Cookie(oauthNativeChallengeCookie); cookieErr == nil {
		h.handleNativeCallback(w, r, code, nativeCookie.Value)
		return
	}

	// 3. 認証処理
	session, err := h.service.HandleCallback(r.Context(), code)
	if err != nil {
		slog.Error("oauth callback failed", slog.String("error", err.Error()))
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// 4. セッション固定攻撃対策: 旧 session_id を無効化して識別子を旋回する。
	//    ログイン前から存在する session_id Cookie に対応する保存済みセッションを破棄し、
	//    手順 5 で発行する新しい session_id のみが有効になるようにする。
	//    旧セッションが存在しない（Cookie 不在・期限切れ・削除済み）場合や無効化に
	//    失敗した場合でも、ログイン自体はエラーにせず継続する。
	if oldCookie, cookieErr := r.Cookie(sessionCookieName); cookieErr == nil &&
		oldCookie.Value != "" && oldCookie.Value != session.ID {
		if revokeErr := h.service.Logout(r.Context(), oldCookie.Value); revokeErr != nil {
			// 無効化失敗は運用者が追跡できるよう記録するが、ログインは継続する。
			slog.Error("failed to revoke old session on login rotation",
				slog.String("error", revokeErr.Error()),
			)
		}
	}

	// 5. セッションCookieを設定（HTTP Only）。
	//    canonical builder（session_cookie.go）で属性を集約し、パスキー経路
	//    （native_auth_handler.go::Session / passkey_handler.go）と完全同一属性を保証する。
	http.SetCookie(w, buildSessionCookie(
		sessionCookieName, session.ID,
		h.config.CookieDomain, h.config.CookieSecure, h.config.SessionMaxAge,
	))

	// 6. フロントエンドにリダイレクト（GET 化のため 303 See Other）
	http.Redirect(w, r, h.config.BaseURL, http.StatusSeeOther)
}

// handleNativeCallback は native flow の OAuth callback を処理する（#165）。
//
// state 検証通過後に呼ばれる前提。native flow 文脈 Cookie は成否に関わらず破棄して
// 単回性を保証し、auth_code 発行成功時はアプリスキーム（feedman://auth/callback）へ
// 303 リダイレクトする。Web セッション・session_id Cookie は一切発行しない。
func (h *AuthHandler) handleNativeCallback(w http.ResponseWriter, r *http.Request, code, challenge string) {
	// native flow 文脈は単回利用: 後続の成否に関わらずここで破棄する。
	h.clearNativeChallengeCookie(w)

	// Cookie 改ざん・破損への defensive 検査（login 時と同一の検証関数）。
	// 不正値を auth_codes に保存しない。
	if err := auth.ValidatePKCES256(challenge, "S256"); err != nil {
		slog.Warn("native callback rejected: invalid pkce challenge in cookie")
		http.Error(w, "invalid pkce parameters", http.StatusBadRequest)
		return
	}

	plainCode, err := h.service.HandleNativeCallback(r.Context(), code, challenge)
	if err != nil {
		slog.Error("native oauth callback failed", slog.String("error", err.Error()))
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// アプリスキームへ auth_code を返す（GET 化のため 303 See Other）。
	// 平文 code はこのリダイレクト URL にのみ現れる（ログには出さない）。
	redirectURL := nativeAuthCallbackURL + "?auth_code=" + url.QueryEscape(plainCode)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
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

	// POST ログアウト後はリダイレクトを GET 化するため 303 See Other を用いる
	// （307 だと method を保持し BaseURL へ再 POST してしまう）。
	http.Redirect(w, r, h.config.BaseURL, http.StatusSeeOther)
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
