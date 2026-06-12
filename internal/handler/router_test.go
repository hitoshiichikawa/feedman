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
// RotateRefreshToken は Issue #167 で、RevokeRefreshToken は Issue #168 で interface に
// 追加されたため本モックでも実装する。
type alwaysSucceedExchangeService struct {
	callCount   int
	rotateCalls int
	revokeCalls int
}

func (s *alwaysSucceedExchangeService) ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
	s.callCount++
	return &auth.TokenPair{
		AccessToken:  "ok-access",
		RefreshToken: "ok-refresh",
		ExpiresIn:    900,
	}, nil
}

func (s *alwaysSucceedExchangeService) RotateRefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	s.rotateCalls++
	return &auth.TokenPair{
		AccessToken:  "ok-rotated-access",
		RefreshToken: "ok-rotated-refresh",
		ExpiresIn:    900,
	}, nil
}

func (s *alwaysSucceedExchangeService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	s.revokeCalls++
	return nil
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

// --- Refresh ルーティング（Issue #167 / Req 1.5, NFR 2.2） ---

// TestNewRouter_NativeAuthRefresh_RegisteredWhenHandlerInjected は NativeAuthHandler を
// 注入したとき POST /api/auth/refresh がセッション無しで到達し、200 が返ることを検証する
// （Req 1.5: Cookie / Bearer なしで呼び出し可能）。
func TestNewRouter_NativeAuthRefresh_RegisteredWhenHandlerInjected(t *testing.T) {
	// Arrange
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	router := NewRouter(newMinimalDepsForNativeAuth(nh))

	body := `{"refresh_token":"plain-refresh-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (handler 到達 / Req 1.5)", resp.StatusCode, http.StatusOK)
	}
	if svc.rotateCalls != 1 {
		t.Errorf("service.RotateRefreshToken called %d times, want 1 (handler に到達していない可能性)",
			svc.rotateCalls)
	}
}

// TestNewRouter_NativeAuthRefresh_NotRegisteredWhenHandlerNil は NativeAuthHandler が
// nil のとき POST /api/auth/refresh がルートとして登録されず 404 が返ることを検証する
// （NFR 2.2: 署名鍵未設定環境の fail-closed）。
func TestNewRouter_NativeAuthRefresh_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange: NativeAuthHandler nil
	router := NewRouter(newMinimalDepsForNativeAuth(nil))

	body := `{"refresh_token":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: fail-closed として 404
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (NativeAuthHandler nil で fail-closed / NFR 2.2)",
			w.Result().StatusCode, http.StatusNotFound)
	}
}

// TestNewRouter_NativeAuthRefresh_DoesNotRequireSession は注入時に Cookie 無しでも
// 401 を返さず handler まで到達することを検証する（Req 1.5: Session middleware 通らない）。
func TestNewRouter_NativeAuthRefresh_DoesNotRequireSession(t *testing.T) {
	// Arrange
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	router := NewRouter(newMinimalDepsForNativeAuth(nh))

	body := `{"refresh_token":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
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

// --- Revoke ルーティング（Issue #168 / Req 2.4, NFR 2.2 / design.md Testing Strategy 9） ---

// TestNewRouter_NativeAuthRevoke_RegisteredWhenHandlerInjected は NativeAuthHandler を
// 注入したとき POST /api/auth/revoke がセッション無しで到達し、204 が返ることを検証する
// （Req 2.4: Cookie / Bearer なしで呼び出し可能 = token 所持自体が失効権限）。
func TestNewRouter_NativeAuthRevoke_RegisteredWhenHandlerInjected(t *testing.T) {
	// Arrange
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	router := NewRouter(newMinimalDepsForNativeAuth(nh))

	body := `{"refresh_token":"plain-refresh-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// セッション Cookie 無し
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: Cookie 無しで 204（Session middleware を経由していない）
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d (handler 到達 / Req 2.4)", resp.StatusCode, http.StatusNoContent)
	}
	if svc.revokeCalls != 1 {
		t.Errorf("service.RevokeRefreshToken called %d times, want 1 (handler に到達していない可能性)",
			svc.revokeCalls)
	}
}

// TestNewRouter_NativeAuthRevoke_NotRegisteredWhenHandlerNil は NativeAuthHandler が
// nil のとき POST /api/auth/revoke がルートとして登録されず 404 が返ることを検証する
// （NFR 2.2: 署名鍵未設定環境の fail-closed）。
func TestNewRouter_NativeAuthRevoke_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange: NativeAuthHandler nil
	router := NewRouter(newMinimalDepsForNativeAuth(nil))

	body := `{"refresh_token":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: fail-closed として 404
	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (NativeAuthHandler nil で fail-closed / NFR 2.2)",
			w.Result().StatusCode, http.StatusNotFound)
	}
}

// --- Bearer-or-Session 認証ルーティング（Issue #169 / design.md Testing Strategy router 1〜4） ---

// newBearerAuthDeps は認証必須ルート（/api/subscriptions）への Bearer / Cookie 認証を
// 検証するための最小 deps を返す。verifier / sessions を差し替えて各ケースを構成する。
func newBearerAuthDeps(verifier middleware.JWTVerifier, sessions map[string]*model.Session) *RouterDeps {
	deps := newMinimalDepsForNativeAuth(nil)
	deps.JWTVerifier = verifier
	deps.SessionFinder = &mockSessionFinderForRouter{sessions: sessions}
	deps.SubscriptionService = &mockSubscriptionService{
		listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
			return []subscriptionResponse{}, nil
		},
	}
	return deps
}

// stubRouterJWTVerifier は router テスト用の固定結果 JWTVerifier スタブ。
type stubRouterJWTVerifier struct {
	userID string
	err    error
}

func (s *stubRouterJWTVerifier) VerifyAccessToken(tokenString string) (string, error) {
	return s.userID, s.err
}

// TestNewRouter_BearerAuth_ReachesAPIWithoutCookie は JWTVerifier 注入時に Cookie 無し +
// Bearer で既存の認証必須ルートに到達できることを検証する（Testing Strategy router 1 /
// Req 1.1, 1.2: 下流ルートは変更ゼロで透過動作）。
func TestNewRouter_BearerAuth_ReachesAPIWithoutCookie(t *testing.T) {
	// Arrange
	deps := newBearerAuthDeps(&stubRouterJWTVerifier{userID: "user-test-1"}, map[string]*model.Session{})
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.Header.Set("Authorization", "Bearer stub-valid-token")
	// Cookie 無し
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: Bearer 認証で既存ルートに到達し 200
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (Bearer で認証必須ルート到達 / Req 1.1)",
			w.Result().StatusCode, http.StatusOK)
	}
}

// TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior は JWTVerifier nil のとき
// 従来構成（Cookie セッション認証のみ）と同一挙動になることを検証する
// （Testing Strategy router 2 / Req 4.2 / NFR 2.2）。
func TestNewRouter_BearerAuth_NilVerifierKeepsLegacyBehavior(t *testing.T) {
	sessions := map[string]*model.Session{
		"valid-session": {
			ID:        "valid-session",
			UserID:    "user-test-1",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}

	t.Run("有効 Cookie のとき従来どおり 200", func(t *testing.T) {
		// Arrange
		router := NewRouter(newBearerAuthDeps(nil, sessions))
		req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		w := httptest.NewRecorder()

		// Act
		router.ServeHTTP(w, req)

		// Assert
		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d (NFR 2.1: 既存 Cookie 認証は不変)", w.Result().StatusCode, http.StatusOK)
		}
	})

	t.Run("Bearer 付き + Cookie 無しのとき従来どおり 401（token は評価されない）", func(t *testing.T) {
		// Arrange
		router := NewRouter(newBearerAuthDeps(nil, sessions))
		req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
		req.Header.Set("Authorization", "Bearer anything")
		w := httptest.NewRecorder()

		// Act
		router.ServeHTTP(w, req)

		// Assert: 導入前と同一の未認証応答（Req 4.3）
		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d (Req 4.2 / 4.3)", w.Result().StatusCode, http.StatusUnauthorized)
		}
	})
}
