package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

type mockAuthService struct {
	getLoginURLFn          func(state string) string
	handleCallbackFn       func(ctx context.Context, code string) (*model.Session, error)
	handleNativeCallbackFn func(ctx context.Context, code, pkceChallenge string) (string, error)
	logoutFn               func(ctx context.Context, sessionID string) error
	getCurrentUserFn       func(ctx context.Context, sessionID string) (*model.User, error)
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

func (m *mockAuthService) HandleNativeCallback(ctx context.Context, code, pkceChallenge string) (string, error) {
	if m.handleNativeCallbackFn != nil {
		return m.handleNativeCallbackFn(ctx, code, pkceChallenge)
	}
	return "", nil
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

	// リダイレクトされること（POST/GET アクション後の 303 See Other）
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
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

func TestAuthHandler_Callback_RotatesOldSession_RevokesPreviousSessionID(t *testing.T) {
	// Arrange: 旧 session_id Cookie を保持した状態でコールバックに成功する
	var revokedSessionID string
	logoutCalled := false
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{ID: "new-session-id", UserID: "user-id-123"}, nil
		},
		logoutFn: func(ctx context.Context, sessionID string) error {
			logoutCalled = true
			revokedSessionID = sessionID
			return nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "old-session-id"})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert: AC R1.1 旧 session_id に対応する保存済みセッションが無効化されること
	if !logoutCalled {
		t.Fatal("expected old session to be revoked via Logout")
	}
	if revokedSessionID != "old-session-id" {
		t.Errorf("revoked session ID = %q, want %q", revokedSessionID, "old-session-id")
	}
}

func TestAuthHandler_Callback_RotatesOldSession_NewCookieDiffersFromOld(t *testing.T) {
	// Arrange: 旧 session_id を保持した状態でログインに成功する
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{ID: "new-session-id", UserID: "user-id-123"}, nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "old-session-id"})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert: AC R1.3 / R2.2 ログイン後に有効な識別子が旧識別子と異なること
	resp := w.Result()
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be set")
	}
	if sessionCookie.Value == "old-session-id" {
		t.Error("new session cookie value must differ from old session_id")
	}
	if sessionCookie.Value != "new-session-id" {
		t.Errorf("session cookie value = %q, want %q", sessionCookie.Value, "new-session-id")
	}
}

func TestAuthHandler_Callback_NoOldSessionCookie_DoesNotRevokeAndCompletes(t *testing.T) {
	// Arrange: session_id Cookie が存在しない状態でログインに成功する
	logoutCalled := false
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{ID: "new-session-id", UserID: "user-id-123"}, nil
		},
		logoutFn: func(ctx context.Context, sessionID string) error {
			logoutCalled = true
			return nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert: AC R3.1 旧 Cookie 不在時は無効化を試みず正常完了すること
	if logoutCalled {
		t.Error("Logout should not be called when no old session_id cookie exists")
	}
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || sessionCookie.Value != "new-session-id" {
		t.Fatal("expected new session_id cookie to be set")
	}
}

func TestAuthHandler_Callback_RevokeFails_StillCompletesLogin(t *testing.T) {
	// Arrange: 旧セッション無効化が失敗するケース
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{ID: "new-session-id", UserID: "user-id-123"}, nil
		},
		logoutFn: func(ctx context.Context, sessionID string) error {
			return errors.New("delete failed")
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "old-session-id"})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert: AC R3.3 旧セッション無効化失敗でもログインはエラーにならず完了すること
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (login must not fail on revoke error)", resp.StatusCode, http.StatusSeeOther)
	}
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || sessionCookie.Value != "new-session-id" {
		t.Fatal("expected new session_id cookie to be set even when revoke fails")
	}
}

func TestAuthHandler_Callback_PreservesCookieAttributes(t *testing.T) {
	// Arrange: Cookie 属性の後方互換（NFR 2）を検証する
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{ID: "new-session-id", UserID: "user-id-123"}, nil
		},
	}
	h := NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		CookieDomain:  "feedman.example.com",
		CookieSecure:  true,
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "old-session-id"})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert: NFR 2.1 Cookie 名・属性が維持されること / NFR 2.2 リダイレクト先が維持されること
	resp := w.Result()
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be set")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if !sessionCookie.Secure {
		t.Error("session cookie should be Secure when CookieSecure is true")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want %v", sessionCookie.SameSite, http.SameSiteLaxMode)
	}
	if sessionCookie.Domain != "feedman.example.com" {
		t.Errorf("session cookie Domain = %q, want %q", sessionCookie.Domain, "feedman.example.com")
	}
	if sessionCookie.MaxAge != 86400 {
		t.Errorf("session cookie MaxAge = %d, want %d", sessionCookie.MaxAge, 86400)
	}
	if loc := resp.Header.Get("Location"); loc != "http://localhost:3000" {
		t.Errorf("Location = %q, want %q", loc, "http://localhost:3000")
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

	// リダイレクトされること（POST ログアウト後の 303 See Other）
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
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
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
}

// TestAuthHandler_Logout_JSONAccept_ReturnsNoContent は Accept: application/json を送る
// fetch/XHR クライアント（Web 版 useLogout など）に対して 204 No Content を返し、
// Location ヘッダを付与しないことを検証する（Issue #235 Requirement 1.1 / 1.2）。
//
// 従来 303 See Other + Location を返していたため、fetch の既定 redirect: "follow" で
// クライアントが遷移先 HTML を取得し JSON パーサが SyntaxError を起こしてログアウトが
// 失敗扱いになる不具合の中核修正。
func TestAuthHandler_Logout_JSONAccept_ReturnsNoContent(t *testing.T) {
	// Arrange
	logoutCalledWith := ""
	svc := &mockAuthService{
		logoutFn: func(ctx context.Context, sessionID string) error {
			logoutCalledWith = sessionID
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
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-json-client"})
	w := httptest.NewRecorder()

	// Act
	h.Logout(w, req)

	// Assert: 204 No Content + Location ヘッダ不在（Req 1.1 / 1.2）
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Errorf("Location = %q, want empty (json client should not be redirected)", loc)
	}

	// セッション破棄が行われること（Req 1.1）
	if logoutCalledWith != "session-json-client" {
		t.Errorf("service.Logout called with %q, want %q", logoutCalledWith, "session-json-client")
	}

	// セッションCookieがクリアされること（NFR 1 / Req 6.1 の Cookie 属性維持）
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be cleared even for JSON client")
	}
	if sessionCookie.MaxAge != -1 {
		t.Errorf("session cookie MaxAge = %d, want -1 (delete)", sessionCookie.MaxAge)
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie should remain HttpOnly (Req 6.1)")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want %v (Req 6.1)", sessionCookie.SameSite, http.SameSiteLaxMode)
	}
}

// TestAuthHandler_Logout_JSONAccept_NoSession_ReturnsNoContent は Accept: application/json
// で session Cookie を持たないクライアントに対しても 204 を返すことを検証する
// （Req 5.3 セッション期限切れ・未存在時もログイン画面表示に到達）。
func TestAuthHandler_Logout_JSONAccept_NoSession_ReturnsNoContent(t *testing.T) {
	// Arrange: Cookie を付与しない
	h := NewAuthHandler(&mockAuthService{}, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	// Act
	h.Logout(w, req)

	// Assert: 204 でリダイレクトしない
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Errorf("Location = %q, want empty", loc)
	}
}

// TestAuthHandler_Logout_FormPost_StillRedirects は Accept ヘッダに application/json を
// 含まない従来クライアント（HTML form POST 経由等）に対しては、これまで通り 303 See Other +
// Location を返すことを検証する（Req 6.2 の後方互換性維持）。
func TestAuthHandler_Logout_FormPost_StillRedirects(t *testing.T) {
	cases := []struct {
		name   string
		accept string
	}{
		{name: "Accept ヘッダ不在", accept: ""},
		{name: "HTML を要求する form POST", accept: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		{name: "text/plain のみ", accept: "text/plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockAuthService{
				logoutFn: func(ctx context.Context, sessionID string) error {
					return nil
				},
			}
			h := NewAuthHandler(svc, AuthHandlerConfig{
				BaseURL:       "http://localhost:3000",
				SessionMaxAge: 86400,
			})
			req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-form-post"})
			w := httptest.NewRecorder()

			// Act
			h.Logout(w, req)

			// Assert: 従来通り 303 See Other + Location: BaseURL（Req 6.2）
			resp := w.Result()
			if resp.StatusCode != http.StatusSeeOther {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
			}
			if loc := resp.Header.Get("Location"); loc != "http://localhost:3000" {
				t.Errorf("Location = %q, want %q", loc, "http://localhost:3000")
			}
		})
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

// --- flow=native（#165）のテスト ---

// nativeTestChallenge は S256 challenge 形式（base64url no-pad 43 文字）の有効なテスト値。
const nativeTestChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

// newNativeTestHandler は native テスト用の AuthHandler を生成する。
func newNativeTestHandler(svc *mockAuthService) *AuthHandler {
	return NewAuthHandler(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		CookieDomain:  "",
		CookieSecure:  false,
		SessionMaxAge: 86400,
	})
}

// findCookie は response の Set-Cookie から名前一致の Cookie を返す。
func findCookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestAuthHandler_Login_Native_SetsChallengeCookieAndRedirects は flow=native + 有効な PKCE で
// state / native challenge 両 Cookie が設定され OAuth リダイレクトすることを検証する（Req 1.1）。
func TestAuthHandler_Login_Native_SetsChallengeCookieAndRedirects(t *testing.T) {
	// Arrange
	svc := &mockAuthService{
		getLoginURLFn: func(state string) string {
			return "https://accounts.google.com/o/oauth2/auth?state=" + state
		},
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet,
		"/auth/google/login?flow=native&code_challenge="+nativeTestChallenge+"&code_challenge_method=S256", nil)
	w := httptest.NewRecorder()

	// Act
	h.Login(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	if !containsStr(resp.Header.Get("Location"), "accounts.google.com") {
		t.Errorf("Location = %q, should contain google oauth URL", resp.Header.Get("Location"))
	}
	stateCookie := findCookie(resp, "oauth_state")
	if stateCookie == nil || stateCookie.Value == "" {
		t.Error("oauth_state cookie should be set")
	}
	nativeCookie := findCookie(resp, "oauth_native_challenge")
	if nativeCookie == nil {
		t.Fatal("oauth_native_challenge cookie should be set")
	}
	if nativeCookie.Value != nativeTestChallenge {
		t.Errorf("native cookie value = %q, want %q", nativeCookie.Value, nativeTestChallenge)
	}
	if !nativeCookie.HttpOnly {
		t.Error("native cookie should be HttpOnly")
	}
}

// TestAuthHandler_Login_Native_InvalidPKCE_Rejects は PKCE 欠落・不正時に 400 を返し
// Cookie 設定もリダイレクトも行わないことを検証する（Req 1.2, 1.3, 1.4 / NFR 1.2）。
func TestAuthHandler_Login_Native_InvalidPKCE_Rejects(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{name: "challenge 欠落のとき 400", query: "flow=native&code_challenge_method=S256"},
		{name: "method 欠落のとき 400", query: "flow=native&code_challenge=" + nativeTestChallenge},
		{name: "method=plain のとき 400", query: "flow=native&code_challenge=" + nativeTestChallenge + "&code_challenge_method=plain"},
		{name: "challenge 形式不正のとき 400", query: "flow=native&code_challenge=short&code_challenge_method=S256"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			h := newNativeTestHandler(&mockAuthService{})
			req := httptest.NewRequest(http.MethodGet, "/auth/google/login?"+tc.query, nil)
			w := httptest.NewRecorder()

			// Act
			h.Login(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
			if len(resp.Cookies()) != 0 {
				t.Errorf("no cookies should be set on rejection, got %d", len(resp.Cookies()))
			}
			if resp.Header.Get("Location") != "" {
				t.Error("no redirect should happen on rejection")
			}
		})
	}
}

// TestAuthHandler_Login_Web_ClearsStaleNativeCookie は flow=native なしの Web ログインで
// 残存する native flow 文脈が破棄されることを検証する（Req 1.6）。
func TestAuthHandler_Login_Web_ClearsStaleNativeCookie(t *testing.T) {
	// Arrange: 過去の native flow 文脈（cookie）が残存している
	svc := &mockAuthService{
		getLoginURLFn: func(state string) string { return "https://accounts.google.com/o/oauth2/auth" },
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_native_challenge", Value: nativeTestChallenge})
	w := httptest.NewRecorder()

	// Act
	h.Login(w, req)

	// Assert: 削除 Set-Cookie（MaxAge < 0）が発行される
	resp := w.Result()
	nativeCookie := findCookie(resp, "oauth_native_challenge")
	if nativeCookie == nil {
		t.Fatal("expected deletion Set-Cookie for oauth_native_challenge")
	}
	if nativeCookie.MaxAge >= 0 {
		t.Errorf("native cookie MaxAge = %d, want negative (deletion)", nativeCookie.MaxAge)
	}
}

// TestAuthHandler_Login_Web_NoNativeCookie_HeadersUnchanged は残存 native 文脈が無い
// Web ログインで native cookie の Set-Cookie が発行されない（応答不変）ことを検証する（NFR 2.1）。
func TestAuthHandler_Login_Web_NoNativeCookie_HeadersUnchanged(t *testing.T) {
	// Arrange
	svc := &mockAuthService{
		getLoginURLFn: func(state string) string { return "https://accounts.google.com/o/oauth2/auth" },
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()

	// Act
	h.Login(w, req)

	// Assert: oauth_state のみが設定され、native cookie のヘッダは存在しない
	resp := w.Result()
	if findCookie(resp, "oauth_native_challenge") != nil {
		t.Error("oauth_native_challenge Set-Cookie must not be sent for plain web login")
	}
	if findCookie(resp, "oauth_state") == nil {
		t.Error("oauth_state cookie should still be set")
	}
}

// TestAuthHandler_Callback_Native_RedirectsToAppSchemeWithoutSession は native callback 成功時に
// アプリスキームへ auth_code 付きで 303 し、セッション Cookie を発行しないことを検証する
// （Req 2.1, 2.3, 3.3）。
func TestAuthHandler_Callback_Native_RedirectsToAppSchemeWithoutSession(t *testing.T) {
	// Arrange
	handleCallbackCalled := false
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			handleCallbackCalled = true
			return &model.Session{ID: "should-not-be-issued"}, nil
		},
		handleNativeCallbackFn: func(ctx context.Context, code, pkceChallenge string) (string, error) {
			if code != "test-code" {
				t.Errorf("code = %q, want %q", code, "test-code")
			}
			if pkceChallenge != nativeTestChallenge {
				t.Errorf("pkceChallenge = %q, want %q", pkceChallenge, nativeTestChallenge)
			}
			return "plain-auth-code-123", nil
		},
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_native_challenge", Value: nativeTestChallenge})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	location := resp.Header.Get("Location")
	wantPrefix := "feedman://auth/callback?auth_code="
	if !containsStr(location, wantPrefix) {
		t.Errorf("Location = %q, want prefix %q", location, wantPrefix)
	}
	if !containsStr(location, "plain-auth-code-123") {
		t.Errorf("Location = %q, should contain issued auth code", location)
	}
	if findCookie(resp, "session_id") != nil {
		t.Error("session_id cookie must not be set for native flow")
	}
	nativeCookie := findCookie(resp, "oauth_native_challenge")
	if nativeCookie == nil || nativeCookie.MaxAge >= 0 {
		t.Error("oauth_native_challenge cookie should be deleted after native callback")
	}
	if handleCallbackCalled {
		t.Error("web HandleCallback must not be called for native flow")
	}
}

// TestAuthHandler_Callback_Native_ServiceError_Returns500 は native callback の処理失敗時に
// 500 を返し auth_code リダイレクトを行わないことを検証する（Req 3.4）。
func TestAuthHandler_Callback_Native_ServiceError_Returns500(t *testing.T) {
	// Arrange
	svc := &mockAuthService{
		handleNativeCallbackFn: func(ctx context.Context, code, pkceChallenge string) (string, error) {
			return "", context.DeadlineExceeded
		},
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_native_challenge", Value: nativeTestChallenge})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if containsStr(resp.Header.Get("Location"), "feedman://") {
		t.Error("must not redirect to app scheme on failure")
	}
}

// TestAuthHandler_Callback_Native_InvalidState_RejectsWithoutIssuingCode は native 文脈があっても
// state 検証失敗時は 400 で拒否し auth_code を発行しないことを検証する（Req 3.1）。
func TestAuthHandler_Callback_Native_InvalidState_RejectsWithoutIssuingCode(t *testing.T) {
	// Arrange
	nativeCalled := false
	svc := &mockAuthService{
		handleNativeCallbackFn: func(ctx context.Context, code, pkceChallenge string) (string, error) {
			nativeCalled = true
			return "should-not-be-issued", nil
		},
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=evil-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "expected-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_native_challenge", Value: nativeTestChallenge})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if nativeCalled {
		t.Error("HandleNativeCallback must not be called when state validation fails")
	}
}

// TestAuthHandler_Callback_Native_TamperedChallenge_Rejects は native cookie の challenge が
// 改ざんされた（S256 形式でない）場合に 400 で拒否することを検証する（Req 1.4 defensive）。
func TestAuthHandler_Callback_Native_TamperedChallenge_Rejects(t *testing.T) {
	// Arrange
	nativeCalled := false
	svc := &mockAuthService{
		handleNativeCallbackFn: func(ctx context.Context, code, pkceChallenge string) (string, error) {
			nativeCalled = true
			return "", nil
		},
	}
	h := newNativeTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_native_challenge", Value: "tampered"})
	w := httptest.NewRecorder()

	// Act
	h.Callback(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if nativeCalled {
		t.Error("HandleNativeCallback must not be called with tampered challenge")
	}
}

// TestAuthHandler_Me_CookiePathUnchanged は /auth/me の Cookie 経路応答が本 spec（#207）
// 導入で変化していないことを保護する non-regression テスト（Req 2.6 / 4.3 / NFR 1.1）。
//
// 既存 TestAuthHandler_Me_Authenticated_ReturnsUserJSON は status と Content-Type のみを
// 検査するが、本テストは応答 JSON 本文のキー集合まで踏み込み:
//
//   - サブテスト Cookie_Present_ReturnsExistingShape:
//     応答 200 / Content-Type: application/json / JSON のキー集合が **厳密に**
//     {id, email, name} のみであること（avatar_url / session_id / refresh_token 等の
//     新フィールド・secret は含まれない）と、各フィールドの値が mock の返り値と一致
//     することを集合演算的に assert する。
//   - サブテスト NoCookie_ReturnsUnauthorized:
//     Cookie 不在で 401 を返す既存挙動が変化していないことを保護する。
//
// 本 task は実装変更を伴わない。`/api/users/me`（#207 で新設）と `/auth/me`（既存 Web Cookie 経路）の
// 棲み分けを維持し、後者に新フィールドが漏れて返らないことを将来の変更で機械的に検出する gate。
func TestAuthHandler_Me_CookiePathUnchanged(t *testing.T) {
	t.Run("Cookie_Present_ReturnsExistingShape", func(t *testing.T) {
		// Arrange: 既存 mockAuthService パターンで getCurrentUser を注入。
		// model.User には ID/Email/Name のみ存在し、AvatarURL フィールドは無い
		// （avatar_url が応答に漏れないことの構造的保証は handler 側の map literal にも依存）。
		wantUser := &model.User{
			ID:    "user-id-cookie",
			Email: "cookie@example.com",
			Name:  "Cookie User",
		}
		svc := &mockAuthService{
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				if sessionID != "valid-session" {
					t.Errorf("sessionID = %q, want %q", sessionID, "valid-session")
				}
				return wantUser, nil
			},
		}
		h := NewAuthHandler(svc, AuthHandlerConfig{
			BaseURL: "http://localhost:3000",
		})

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		w := httptest.NewRecorder()

		// Act
		h.Me(w, req)

		// Assert: status / Content-Type は既存仕様通り
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		// JSON 本文を decode してキー集合と値を検査
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// 1. 必須キー {id, email, name} が全て存在し、値が一致する
		if got, ok := body["id"].(string); !ok || got != wantUser.ID {
			t.Errorf("id = %v (ok=%v), want %q", body["id"], ok, wantUser.ID)
		}
		if got, ok := body["email"].(string); !ok || got != wantUser.Email {
			t.Errorf("email = %v (ok=%v), want %q", body["email"], ok, wantUser.Email)
		}
		if got, ok := body["name"].(string); !ok || got != wantUser.Name {
			t.Errorf("name = %v (ok=%v), want %q", body["name"], ok, wantUser.Name)
		}

		// 2. キー集合は **厳密に** {id, email, name, username} のみであること
		//    （Issue #241 で username を追加。avatar_url や将来追加されるフィールドが
		//    /auth/me から漏れないことを引き続き保護する）
		allowed := map[string]bool{
			"id":       true,
			"email":    true,
			"name":     true,
			"username": true,
		}
		if len(body) != len(allowed) {
			t.Errorf("response key count = %d, want %d (keys=%v)", len(body), len(allowed), keysOf(body))
		}
		for k := range body {
			if !allowed[k] {
				t.Errorf("response contains forbidden key %q (shape regression: /auth/me should keep {id, email, name, username})", k)
			}
		}

		// 3. 明示的に新フィールド / secret キーが含まれないことを assert（regression net の二重化）
		//    grep でも検出可能にするため individual key check を残す
		forbidden := []string{"avatar_url", "session_id", "refresh_token", "password", "password_hash", "access_token"}
		for _, k := range forbidden {
			if _, ok := body[k]; ok {
				t.Errorf("response leaks forbidden key %q from /auth/me (non-regression violation)", k)
			}
		}
	})

	t.Run("Cookie_Present_UsernameSet_ReturnsUsernameString", func(t *testing.T) {
		// Arrange: Username を持つパスキー登録ユーザーを注入（Issue #241 / Req 2.1 / 2.2 / 2.4）。
		// 想定シナリオ: パスキー新規登録経路（Task 1 の #241）で users.name / users.username が
		// 同時に初期化されたユーザーが /auth/me を呼ぶケース。
		wantUser := &model.User{
			ID:       "user-id-with-username",
			Email:    "alice@example.com",
			Name:     "alice",
			Username: "alice",
		}
		svc := &mockAuthService{
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				return wantUser, nil
			},
		}
		h := NewAuthHandler(svc, AuthHandlerConfig{
			BaseURL: "http://localhost:3000",
		})

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		w := httptest.NewRecorder()

		// Act
		h.Me(w, req)

		// Assert: 200 / Content-Type / username が string 型で mock 値と一致
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		got, ok := body["username"].(string)
		if !ok {
			t.Fatalf("username = %v (type=%T), want string (Req 2.2)", body["username"], body["username"])
		}
		if got != "alice" {
			t.Errorf("username = %q, want %q (Req 2.4)", got, "alice")
		}
	})

	t.Run("Cookie_Present_UsernameUnset_ReturnsNull", func(t *testing.T) {
		// Arrange: Username 未設定（空文字）ユーザー = Google OAuth 由来ユーザー相当を注入
		// （Issue #241 / Req 2.3 / Req 4.1 / NFR 2.1）。
		wantUser := &model.User{
			ID:       "user-id-google",
			Email:    "google@example.com",
			Name:     "Google User",
			Username: "",
		}
		svc := &mockAuthService{
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				return wantUser, nil
			},
		}
		h := NewAuthHandler(svc, AuthHandlerConfig{
			BaseURL: "http://localhost:3000",
		})

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		w := httptest.NewRecorder()

		// Act
		h.Me(w, req)

		// Assert: 200 / Content-Type / "username" キーは存在しつつ値は nil（JSON 上 null）
		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Req 2.3 / Req 4.1: "username" キーは応答に **必ず存在** すること（omitempty 禁止）
		v, ok := body["username"]
		if !ok {
			t.Fatalf("response missing key \"username\" (Req 2.3 / Req 4.1: omitempty をつけずキーは常に存在する必要がある)")
		}
		// 値は nil（JSON 上 null）であること
		if v != nil {
			t.Errorf("username = %v (type=%T), want nil (JSON null / Req 2.3)", v, v)
		}
	})

	t.Run("NoCookie_ReturnsUnauthorized", func(t *testing.T) {
		// Arrange: session_id Cookie 不在
		h := NewAuthHandler(&mockAuthService{}, AuthHandlerConfig{
			BaseURL: "http://localhost:3000",
		})

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		w := httptest.NewRecorder()

		// Act
		h.Me(w, req)

		// Assert: 既存仕様通り 401 を返す
		resp := w.Result()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
}
