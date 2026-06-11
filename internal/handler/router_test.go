package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

func TestSetupAuthRoutes_LoginEndpoint(t *testing.T) {
	svc := &mockAuthService{
		getLoginURLFn: func(state string) string {
			return "https://accounts.google.com/o/oauth2/auth?state=" + state
		},
	}
	router := SetupAuthRoutes(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("GET /auth/google/login status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
}

func TestSetupAuthRoutes_CallbackEndpoint(t *testing.T) {
	svc := &mockAuthService{
		handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
			return &model.Session{
				ID:        "session-123",
				UserID:    "user-123",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
	}
	router := SetupAuthRoutes(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test&state=valid", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "valid"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("GET /auth/google/callback status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
}

func TestSetupAuthRoutes_LogoutEndpoint(t *testing.T) {
	svc := &mockAuthService{
		logoutFn: func(ctx context.Context, sessionID string) error {
			return nil
		},
	}
	router := SetupAuthRoutes(svc, AuthHandlerConfig{
		BaseURL:       "http://localhost:3000",
		SessionMaxAge: 86400,
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-123"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("POST /auth/logout status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
}

func TestSetupAuthRoutes_MeEndpoint(t *testing.T) {
	svc := &mockAuthService{
		getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
			return &model.User{
				ID:    "user-me",
				Email: "me@example.com",
				Name:  "Me",
			}, nil
		},
	}
	router := SetupAuthRoutes(svc, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /auth/me status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSetupAuthRoutes_UnknownRoute_Returns404Or405(t *testing.T) {
	router := SetupAuthRoutes(&mockAuthService{}, AuthHandlerConfig{
		BaseURL: "http://localhost:3000",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/unknown", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	// 存在しないルートには404か405が返ること
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /auth/unknown status = %d, want 404 or 405", resp.StatusCode)
	}
}

// --- Native Auth Token 交換ルーティング（Issue #166 / Req 1.5, 3.2） ---

// alwaysSucceedExchangeService は service 層に到達したかを判定するための固定成功モック。
// 200 応答が返れば handler に到達したことが確認できる。
type alwaysSucceedExchangeService struct {
	callCount int
}

func (s *alwaysSucceedExchangeService) ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
	s.callCount++
	return &auth.TokenPair{
		AccessToken:  "ok-access",
		RefreshToken: "ok-refresh",
		ExpiresIn:    900,
	}, nil
}

// newMinimalDepsForNativeAuth は Native Auth 関連ルートのみを検証するための最小 deps を返す。
// 既存ルートは挙動を変えない前提で（NFR 2.1）、未使用 service は nil ヌル安全前提で省略する
// と nil panic するため、空のモックを注入する。
func newMinimalDepsForNativeAuth(nativeHandler *NativeAuthHandler) *RouterDeps {
	return &RouterDeps{
		SessionFinder:     &mockSessionFinderForRouter{sessions: map[string]*model.Session{}},
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService:       &mockAuthService{},
		AuthConfig:        AuthHandlerConfig{BaseURL: "http://localhost:3000"},
		NativeAuthHandler: nativeHandler,
	}
}

// TestNewRouter_NativeAuthToken_RegisteredWhenHandlerInjected は NativeAuthHandler を
// 注入したとき POST /api/auth/token がセッション無しで到達し、200 が返ることを検証する
// （Req 1.5: Cookie / Bearer なしで呼び出し可能）。
func TestNewRouter_NativeAuthToken_RegisteredWhenHandlerInjected(t *testing.T) {
	// Arrange
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	router := NewRouter(newMinimalDepsForNativeAuth(nh))

	body := `{"auth_code":"plain-auth-code","code_verifier":"plain-verifier"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (handler 到達 / Req 1.5)", resp.StatusCode, http.StatusOK)
	}
	if svc.callCount != 1 {
		t.Errorf("service called %d times, want 1 (handler に到達していない可能性)", svc.callCount)
	}
}

// TestNewRouter_NativeAuthToken_NotRegisteredWhenHandlerNil は NativeAuthHandler が
// nil のとき POST /api/auth/token がルートとして登録されず 404 が返ることを検証する
// （Req 3.2: 署名鍵未設定環境の fail-closed）。
func TestNewRouter_NativeAuthToken_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange: NativeAuthHandler nil
	router := NewRouter(newMinimalDepsForNativeAuth(nil))

	body := `{"auth_code":"a","code_verifier":"v"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: fail-closed として 404
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (NativeAuthHandler nil で fail-closed / Req 3.2)",
			w.Result().StatusCode, http.StatusNotFound)
	}
}

// TestNewRouter_NativeAuthToken_DoesNotRequireSession は注入時に Cookie 無しでも
// 401 を返さず handler まで到達することを検証する（Req 1.5: Session middleware 通らない）。
// 既存の認証必須ルートでは Cookie 無し = 401 になるため、同じ抜き打ちが本ルートでは起きない
// ことを直接確認する。
func TestNewRouter_NativeAuthToken_DoesNotRequireSession(t *testing.T) {
	// Arrange
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	router := NewRouter(newMinimalDepsForNativeAuth(nh))

	body := `{"auth_code":"a","code_verifier":"v"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// セッション Cookie 無し
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: 401 ではなく 200（Session middleware を経由していない）
	resp := w.Result()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("status = 401, want non-401 (Req 1.5: Cookie 無しで呼び出し可能)")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestNewRouter_NativeAuthToken_WrongMethod_Returns405 は POST 以外の method で
// 405 が返ることを確認する（chi の method routing 標準挙動の確認、handler 経由ではない）。
func TestNewRouter_NativeAuthToken_WrongMethod_Returns405(t *testing.T) {
	// Arrange
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	router := NewRouter(newMinimalDepsForNativeAuth(nh))

	req := httptest.NewRequest(http.MethodGet, "/api/auth/token", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if got := w.Result().StatusCode; got != http.StatusMethodNotAllowed && got != http.StatusNotFound {
		t.Errorf("status = %d, want 405 or 404 (POST 以外は不可)", got)
	}
	if svc.callCount != 0 {
		t.Errorf("service called %d times, want 0 (GET は handler に到達しない)", svc.callCount)
	}
}
