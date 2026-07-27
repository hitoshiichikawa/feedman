package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"

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

// TestNewRouter_BearerAuth_IssuerVerifierRoundTrip は #166 auth.JWTIssuer で発行した
// 実物 token を auth.JWTVerifier 注入済み NewRouter へ Bearer 提示し、Cookie 無しで
// 既存 API ルートの認証が成立することを検証する（Testing Strategy router 3 /
// Req 1.1, 4.1: 同一 secret での発行 ↔ 検証の通し）。
func TestNewRouter_BearerAuth_IssuerVerifierRoundTrip(t *testing.T) {
	// Arrange: 発行と検証で同一 secret を共有する実物ペア
	secret := []byte("router-roundtrip-secret-32bytes-x")
	issuer := auth.NewJWTIssuer(secret, "v1")
	tokenString, err := issuer.IssueAccessToken("user-roundtrip-1")
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}
	deps := newBearerAuthDeps(auth.NewJWTVerifier(secret), map[string]*model.Session{})
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (実物 issuer ↔ verifier の通し / Req 4.1)",
			w.Result().StatusCode, http.StatusOK)
	}
}

// TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401 は期限切れ token +
// 有効 Cookie 併送で 401 になる（Cookie へ fallback しない）ことを router 通しで検証する
// （Testing Strategy router 通し / Req 2.2, 2.4）。
func TestNewRouter_BearerAuth_ExpiredTokenWithValidCookie_Returns401(t *testing.T) {
	// Arrange: exp が過去の token を同一 secret で直接組み立てる
	secret := []byte("router-roundtrip-secret-32bytes-x")
	past := time.Now().Add(-1 * time.Hour)
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       "user-roundtrip-1",
		"iat":       past.Add(-15 * time.Minute).Unix(),
		"exp":       past.Unix(),
		"token_use": "access",
	})
	tokenString, err := expiredToken.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString returned error: %v", err)
	}

	sessions := map[string]*model.Session{
		"valid-session": {
			ID:        "valid-session",
			UserID:    "user-test-1",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
	}
	router := NewRouter(newBearerAuthDeps(auth.NewJWTVerifier(secret), sessions))

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: 有効 Cookie が併送されていても fallback せず 401（Req 2.4）
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d (期限切れ Bearer は Cookie へ fallback しない / Req 2.2, 2.4)",
			w.Result().StatusCode, http.StatusUnauthorized)
	}
}

// --- native auth 3 ルートの IP 単位レート制限（Issue #171 / design.md Testing Strategy） ---

// newNativeAuthRateLimitRouter は NativeAuthHandler + UnauthIPRateLimiter（指定 burst）を
// 注入した router を構築する。burst=1 なら同一 IP の 2 回目で必ず 429 になる。
func newNativeAuthRateLimitRouter(burst int) (http.Handler, *alwaysSucceedExchangeService, *middleware.IPRateLimiter) {
	svc := &alwaysSucceedExchangeService{}
	nh := NewNativeAuthHandler(svc)
	deps := newMinimalDepsForNativeAuth(nh)
	deps.UnauthIPRateLimiter = middleware.NewIPRateLimiter(middleware.IPRateLimiterConfig{
		Rate:            rate.Limit(1),
		Burst:           burst,
		CleanupInterval: 1 * time.Minute,
	})
	return NewRouter(deps), svc, deps.UnauthIPRateLimiter
}

// nativeAuthRouteCases は 3 ルートの path / リクエストボディ / 通過時 status / service
// 呼び出し回数の参照を共通化する table。
type nativeAuthRouteCase struct {
	name       string
	path       string
	body       string
	wantStatus int
	calls      func(svc *alwaysSucceedExchangeService) int
}

func nativeAuthRouteCases() []nativeAuthRouteCase {
	return []nativeAuthRouteCase{
		{
			name:       "token",
			path:       "/api/auth/token",
			body:       `{"auth_code":"a","code_verifier":"v"}`,
			wantStatus: http.StatusOK,
			calls:      func(svc *alwaysSucceedExchangeService) int { return svc.callCount },
		},
		{
			name:       "refresh",
			path:       "/api/auth/refresh",
			body:       `{"refresh_token":"x"}`,
			wantStatus: http.StatusOK,
			calls:      func(svc *alwaysSucceedExchangeService) int { return svc.rotateCalls },
		},
		{
			name:       "revoke",
			path:       "/api/auth/revoke",
			body:       `{"refresh_token":"x"}`,
			wantStatus: http.StatusNoContent,
			calls:      func(svc *alwaysSucceedExchangeService) int { return svc.revokeCalls },
		},
	}
}

// doNativeAuthPost は指定 path へ JSON POST を送る（RemoteAddr 指定付き）。
func doNativeAuthPost(router http.Handler, path, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestNewRouter_NativeAuthIPRateLimit_429OnExcess は同一 IP の閾値超過時に 3 ルートが
// 429 + Retry-After で応答し、service（handler 以降）に到達しないことを検証する
// （Testing Strategy 1〜3 / Req 1.1, 1.2, 1.3, 1.4, 1.5）。
func TestNewRouter_NativeAuthIPRateLimit_429OnExcess(t *testing.T) {
	for _, tc := range nativeAuthRouteCases() {
		t.Run(tc.name+"で同一IP超過のとき429を返しserviceに到達しない", func(t *testing.T) {
			// Arrange: burst=1（1 回目で枯渇）
			router, svc, ipRL := newNativeAuthRateLimitRouter(1)
			defer ipRL.Stop()

			// Act 1: 1 回目は閾値以内なので通常応答（Req 1.4）
			w1 := doNativeAuthPost(router, tc.path, tc.body, "203.0.113.10:50000")
			if w1.Result().StatusCode != tc.wantStatus {
				t.Fatalf("1st status = %d, want %d (閾値以内は通過 / Req 1.4)",
					w1.Result().StatusCode, tc.wantStatus)
			}
			if got := tc.calls(svc); got != 1 {
				t.Fatalf("1st service calls = %d, want 1", got)
			}

			// Act 2: 2 回目は超過 → 429
			w2 := doNativeAuthPost(router, tc.path, tc.body, "203.0.113.10:50001")

			// Assert: 429 + Retry-After、service 未到達（Req 1.1〜1.3, 1.5）
			resp := w2.Result()
			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("2nd status = %d, want %d (Req 1.1-1.3)", resp.StatusCode, http.StatusTooManyRequests)
			}
			if resp.Header.Get("Retry-After") == "" {
				t.Error("Retry-After header is empty (Req 1.5)")
			}
			if got := tc.calls(svc); got != 1 {
				t.Errorf("service calls after 429 = %d, want 1 (429 は handler 到達前に遮断)", got)
			}
		})
	}
}

// TestNewRouter_NativeAuthIPRateLimit_IndependentPerIP は別 IP からの要求が超過 IP の
// 影響を受けないことを検証する（Testing Strategy 4 / Req 1.6）。
func TestNewRouter_NativeAuthIPRateLimit_IndependentPerIP(t *testing.T) {
	// Arrange: burst=1 で IP A を枯渇させる
	router, svc, ipRL := newNativeAuthRateLimitRouter(1)
	defer ipRL.Stop()
	body := `{"auth_code":"a","code_verifier":"v"}`

	if w := doNativeAuthPost(router, "/api/auth/token", body, "203.0.113.10:50000"); w.Result().StatusCode != http.StatusOK {
		t.Fatalf("IP A 1st status = %d, want 200", w.Result().StatusCode)
	}
	if w := doNativeAuthPost(router, "/api/auth/token", body, "203.0.113.10:50001"); w.Result().StatusCode != http.StatusTooManyRequests {
		t.Fatalf("IP A 2nd status = %d, want 429", w.Result().StatusCode)
	}

	// Act: 別 IP B からの要求
	w := doNativeAuthPost(router, "/api/auth/token", body, "198.51.100.20:50000")

	// Assert: IP B は独立カウントのため通過する（Req 1.6）
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("IP B status = %d, want 200 (IP ごとに独立カウント / Req 1.6)", w.Result().StatusCode)
	}
	if svc.callCount != 2 {
		t.Errorf("service calls = %d, want 2 (IP A 1回 + IP B 1回)", svc.callCount)
	}
}

// TestNewRouter_NativeAuthIPRateLimit_SameShapeAsExistingRoutes は native auth ルートの
// 429 応答（status / Retry-After / Content-Type / JSON ボディ）が既存未認証ルート
// （/health）の 429 と同一形式であることを検証する（Testing Strategy 5 / Req 2.3）。
func TestNewRouter_NativeAuthIPRateLimit_SameShapeAsExistingRoutes(t *testing.T) {
	// Arrange: 基準となる /health の 429 応答を取得する
	router, _, ipRL := newNativeAuthRateLimitRouter(1)
	defer ipRL.Stop()
	doRouterReq(router, http.MethodGet, "/health", "203.0.113.30:50000") // burst 消費
	baseW := doRouterReq(router, http.MethodGet, "/health", "203.0.113.30:50001")
	baseResp := baseW.Result()
	if baseResp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("health 2nd status = %d, want 429", baseResp.StatusCode)
	}

	// Act: native auth ルートの 429 応答（別 IP で burst 消費 → 超過）
	body := `{"auth_code":"a","code_verifier":"v"}`
	doNativeAuthPost(router, "/api/auth/token", body, "203.0.113.40:50000")
	w := doNativeAuthPost(router, "/api/auth/token", body, "203.0.113.40:50001")
	resp := w.Result()

	// Assert: status / Content-Type / ボディが /health の 429 と一致（Req 2.3）
	if resp.StatusCode != baseResp.StatusCode {
		t.Errorf("status = %d, want %d", resp.StatusCode, baseResp.StatusCode)
	}
	if got, want := resp.Header.Get("Content-Type"), baseResp.Header.Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After header is empty")
	}
	if got, want := w.Body.String(), baseW.Body.String(); got != want {
		t.Errorf("429 body = %q, want %q (既存未認証ルートと同一形式 / Req 2.3)", got, want)
	}
}

// --- Passkey 系ルーティング（Issue #216 / Req 3.5, 5.4, 6.5, NFR 2.2） ---

// stubPasskeyRouterRegistration は router 統合テスト用に PasskeyRegistrationService の
// 4 メソッドを固定成功として実装するスタブ。route 到達判定にのみ使う。
type stubPasskeyRouterRegistration struct {
	beginNewCalled  int
	finishNewCalled int
	beginAddCalled  int
	finishAddCalled int
	lastAuthUserID  string
}

func (s *stubPasskeyRouterRegistration) BeginRegistrationNew(ctx context.Context,
	rawUsername, optionalEmail, codeChallenge string,
) (string, []byte, error) {
	s.beginNewCalled++
	return "chal-router-new", []byte(`{}`), nil
}

func (s *stubPasskeyRouterRegistration) FinishRegistrationNew(ctx context.Context,
	challengeID string, requestBody []byte,
) (string, error) {
	s.finishNewCalled++
	return "user-router-new", nil
}

func (s *stubPasskeyRouterRegistration) BeginAddCredential(ctx context.Context,
	authenticatedUserID string,
) (string, []byte, error) {
	s.beginAddCalled++
	s.lastAuthUserID = authenticatedUserID
	return "chal-router-add", []byte(`{}`), nil
}

func (s *stubPasskeyRouterRegistration) FinishAddCredential(ctx context.Context,
	authenticatedUserID, challengeID string, requestBody []byte,
) error {
	s.finishAddCalled++
	s.lastAuthUserID = authenticatedUserID
	return nil
}

// stubPasskeyRouterAuthentication は router 統合テスト用の認証サービススタブ。
type stubPasskeyRouterAuthentication struct {
	beginCalled  int
	finishCalled int
}

func (s *stubPasskeyRouterAuthentication) BeginAuthentication(ctx context.Context,
	codeChallenge string,
) (string, []byte, error) {
	s.beginCalled++
	return "chal-router-authn", []byte(`{}`), nil
}

func (s *stubPasskeyRouterAuthentication) FinishAuthentication(ctx context.Context,
	requestBody []byte, challengeID string,
) (string, error) {
	s.finishCalled++
	return "plain-router-auth-code", nil
}

// newPasskeyRouterDeps は passkey / AASA handler を差し替え可能に組み立てた最小 deps を返す。
// passkeyHandler / aasaHandler の nil 指定で fail-closed 挙動を検証できる。
func newPasskeyRouterDeps(passkeyHandler *PasskeyHandler, aasaHandler *AASAHandler) *RouterDeps {
	deps := newMinimalDepsForNativeAuth(nil)
	deps.PasskeyHandler = passkeyHandler
	deps.AASAHandler = aasaHandler
	deps.SessionFinder = &mockSessionFinderForRouter{
		sessions: map[string]*model.Session{
			"valid-session": {
				ID:        "valid-session",
				UserID:    "user-passkey-1",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
		},
	}
	return deps
}

// TestNewRouter_Passkey_UnauthEndpoints_RegisteredWhenHandlerInjected は passkey 未認証 4
// endpoint がハンドラ注入時に到達可能で 200/2xx を返すことを検証する（Req 1.1 / 2.1）。
func TestNewRouter_Passkey_UnauthEndpoints_RegisteredWhenHandlerInjected(t *testing.T) {
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	router := NewRouter(deps)

	cases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{"registration_begin", "/api/passkey/registration/begin",
			`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`, http.StatusOK},
		{"registration_finish", "/api/passkey/registration/finish",
			`{"challenge_id":"c","credential":{"raw":true}}`, http.StatusOK},
		{"authentication_begin", "/api/passkey/authentication/begin",
			`{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`, http.StatusOK},
		{"authentication_finish", "/api/passkey/authentication/finish",
			`{"challenge_id":"c","credential":{"raw":true}}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name+"到達で 200", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if got := w.Result().StatusCode; got != tc.wantStatus {
				t.Errorf("%s status = %d, want %d", tc.path, got, tc.wantStatus)
			}
		})
	}
}

// TestNewRouter_Passkey_UnauthEndpoints_DoesNotRequireSession は passkey 未認証 route が
// Cookie 無しで到達することを検証する（Req 5.4 と同じ性質 / 未認証グループ配置）。
func TestNewRouter_Passkey_UnauthEndpoints_DoesNotRequireSession(t *testing.T) {
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/authentication/begin",
		strings.NewReader(`{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`))
	req.Header.Set("Content-Type", "application/json")
	// Cookie / Bearer 無し
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if got := w.Result().StatusCode; got == http.StatusUnauthorized {
		t.Errorf("status = 401, want non-401 (Cookie 無しで到達可能 / 未認証グループ)")
	}
	if authn.beginCalled != 1 {
		t.Errorf("service called %d times, want 1", authn.beginCalled)
	}
}

// TestNewRouter_PasskeyCapability_RegisteredWhenHandlerInjected は PasskeyHandler
// 注入時に GET /api/passkey/capability がセッション無しで到達し、200 + JSON
// `{"available": true}` を返すことを検証する（Issue #223 / task 3 / Req 5.2）。
func TestNewRouter_PasskeyCapability_RegisteredWhenHandlerInjected(t *testing.T) {
	// Arrange
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/passkey/capability", nil)
	// Cookie / Bearer 無し（未認証グループ配下）
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: 200 + body / header
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (handler 到達 / Req 5.2)", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(w.Body.String(), `"available":true`) {
		t.Errorf("body = %q, want to contain {\"available\":true}", w.Body.String())
	}
}

// TestNewRouter_PasskeyCapability_NotRegisteredWhenHandlerNil は PasskeyHandler が nil の
// とき GET /api/passkey/capability がルートとして登録されず 404 が返ることを検証する
// （Issue #223 / task 3 / Req 5.2: fail-closed = Web は「パスキー非提供」判定へ縮退 / NFR 2.1）。
func TestNewRouter_PasskeyCapability_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange: PasskeyHandler nil
	deps := newPasskeyRouterDeps(nil, nil)
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/passkey/capability", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: fail-closed として 404
	if got := w.Result().StatusCode; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (PasskeyHandler nil で fail-closed / Req 5.2 / NFR 2.1)", got)
	}
}

// TestNewRouter_Passkey_NotRegisteredWhenHandlerNil は passkey handler が nil のとき
// 6 route すべてが 404 を返すことを検証する（NFR 2.2: WEBAUTHN_RP_ID 未設定環境の fail-closed）。
func TestNewRouter_Passkey_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange
	deps := newPasskeyRouterDeps(nil, nil)
	router := NewRouter(deps)

	paths := []string{
		"/api/passkey/registration/begin",
		"/api/passkey/registration/finish",
		"/api/passkey/registration/add/begin",
		"/api/passkey/registration/add/finish",
		"/api/passkey/authentication/begin",
		"/api/passkey/authentication/finish",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			// 認証必須 route も対象。Cookie 有りで送っても handler nil なら 404 が返る。
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if got := w.Result().StatusCode; got != http.StatusNotFound {
				t.Errorf("%s status = %d, want 404 (PasskeyHandler nil / NFR 2.2)", path, got)
			}
		})
	}
}

// TestNewRouter_Passkey_AddEndpoints_Require401WithoutCookie は追加登録 2 endpoint が
// Cookie / Bearer 無しで 401 を返すことを検証する（Req 3.5）。
func TestNewRouter_Passkey_AddEndpoints_Require401WithoutCookie(t *testing.T) {
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	router := NewRouter(deps)

	paths := []string{
		"/api/passkey/registration/add/begin",
		"/api/passkey/registration/add/finish",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			body := `{}`
			if strings.HasSuffix(path, "finish") {
				body = `{"challenge_id":"c","credential":{"raw":true}}`
			}
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			// Cookie / Bearer 無し
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if got := w.Result().StatusCode; got != http.StatusUnauthorized {
				t.Errorf("%s status = %d, want 401 (Req 3.5)", path, got)
			}
			if reg.beginAddCalled != 0 || reg.finishAddCalled != 0 {
				t.Errorf("service should not be called on unauthenticated request")
			}
		})
	}
}

// TestNewRouter_Passkey_AddEndpoints_ReachesServiceWithValidCookie は追加登録 2 endpoint
// が有効セッションで到達し、context 経由の userID が service に伝わることを検証する
// （Req 3.1 / 3.2）。
func TestNewRouter_Passkey_AddEndpoints_ReachesServiceWithValidCookie(t *testing.T) {
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	router := NewRouter(deps)

	// begin
	{
		req := httptest.NewRequest(http.MethodPost, "/api/passkey/registration/add/begin",
			strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if got := w.Result().StatusCode; got != http.StatusOK {
			t.Errorf("add/begin status = %d, want 200", got)
		}
		if reg.beginAddCalled != 1 {
			t.Errorf("service called %d times, want 1", reg.beginAddCalled)
		}
		if reg.lastAuthUserID != "user-passkey-1" {
			t.Errorf("service received authUserID = %q, want user-passkey-1", reg.lastAuthUserID)
		}
	}
	// finish
	{
		req := httptest.NewRequest(http.MethodPost, "/api/passkey/registration/add/finish",
			strings.NewReader(`{"challenge_id":"c","credential":{"raw":true}}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if got := w.Result().StatusCode; got != http.StatusNoContent {
			t.Errorf("add/finish status = %d, want 204", got)
		}
		if reg.finishAddCalled != 1 {
			t.Errorf("service called %d times, want 1", reg.finishAddCalled)
		}
	}
}

// TestNewRouter_AASA_RegisteredWhenHandlerInjected は AASA handler 注入時に GET が
// 200 で応答することを検証する（Req 5.1 / 5.4: 認証・IP 制限の外側）。
func TestNewRouter_AASA_RegisteredWhenHandlerInjected(t *testing.T) {
	deps := newPasskeyRouterDeps(nil, NewAASAHandler("TEAM1234.com.example.feedman"))
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	// Cookie / Bearer 無し
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if got := w.Result().StatusCode; got != http.StatusOK {
		t.Errorf("status = %d, want 200 (認証・IP 制限の外側 / Req 5.4)", got)
	}
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// TestNewRouter_AASA_NotRegisteredWhenHandlerNil は AASA handler が nil のとき 404 を
// 返すことを検証する（NFR 2.2: WEBAUTHN_IOS_APP_ID 未設定環境の fail-closed）。
// また PasskeyHandler が非 nil でも AASA が独立に無効化されることを確認する。
func TestNewRouter_AASA_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange: PasskeyHandler は生きているが AASA は nil
	reg := &stubPasskeyRouterRegistration{}
	authn := &stubPasskeyRouterAuthentication{}
	deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	if got := w.Result().StatusCode; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (AASA handler nil / NFR 2.2)", got)
	}
}

// TestNewRouter_Passkey_AASA_IndependentFailClose は PasskeyHandler と AASAHandler が
// 独立に fail-closed であることを検証する（片方だけ nil のパターン 2 通り）。
func TestNewRouter_Passkey_AASA_IndependentFailClose(t *testing.T) {
	t.Run("Passkey生存 + AASA nil", func(t *testing.T) {
		reg := &stubPasskeyRouterRegistration{}
		authn := &stubPasskeyRouterAuthentication{}
		deps := newPasskeyRouterDeps(NewPasskeyHandler(reg, authn), nil)
		router := NewRouter(deps)

		// passkey 未認証 route は到達可能
		req := httptest.NewRequest(http.MethodPost, "/api/passkey/authentication/begin",
			strings.NewReader(`{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("passkey status = %d, want 200 (Passkey は生存)", w.Result().StatusCode)
		}
		// AASA は 404
		req2 := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		if w2.Result().StatusCode != http.StatusNotFound {
			t.Errorf("AASA status = %d, want 404 (AASA は独立に無効)", w2.Result().StatusCode)
		}
	})

	t.Run("Passkey nil + AASA生存", func(t *testing.T) {
		deps := newPasskeyRouterDeps(nil, NewAASAHandler("TEAM1234.com.example.feedman"))
		router := NewRouter(deps)

		// passkey は 404
		req := httptest.NewRequest(http.MethodPost, "/api/passkey/authentication/begin",
			strings.NewReader(`{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Result().StatusCode != http.StatusNotFound {
			t.Errorf("passkey status = %d, want 404 (Passkey は独立に無効)", w.Result().StatusCode)
		}
		// AASA は 200
		req2 := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		if w2.Result().StatusCode != http.StatusOK {
			t.Errorf("AASA status = %d, want 200 (AASA は生存)", w2.Result().StatusCode)
		}
	})
}

// TestNewRouter_NativeAuthIPRateLimit_Degradations は縮退構成での後方互換を検証する
// （Testing Strategy 6 / Req 2.4）。
func TestNewRouter_NativeAuthIPRateLimit_Degradations(t *testing.T) {
	t.Run("NativeAuthHandler nilのとき3ルートは404のまま（本変更はno-op）", func(t *testing.T) {
		// Arrange: handler なし + limiter あり
		deps := newMinimalDepsForNativeAuth(nil)
		ipRL := middleware.NewIPRateLimiter(middleware.IPRateLimiterConfig{
			Rate: rate.Limit(1), Burst: 1, CleanupInterval: 1 * time.Minute,
		})
		defer ipRL.Stop()
		deps.UnauthIPRateLimiter = ipRL
		router := NewRouter(deps)

		// Act & Assert: 3 ルートとも 404（fail-closed のまま / Req 2.4）
		for _, path := range []string{"/api/auth/token", "/api/auth/refresh", "/api/auth/revoke"} {
			w := doNativeAuthPost(router, path, `{}`, "203.0.113.50:50000")
			if w.Result().StatusCode != http.StatusNotFound {
				t.Errorf("%s status = %d, want 404 (NativeAuthHandler nil / Req 2.4)", path, w.Result().StatusCode)
			}
		}
	})

	t.Run("UnauthIPRateLimiter nilのとき3ルートは制限なしで到達する", func(t *testing.T) {
		// Arrange: handler あり + limiter なし（既存縮退規約: 素通し no-op）
		svc := &alwaysSucceedExchangeService{}
		deps := newMinimalDepsForNativeAuth(NewNativeAuthHandler(svc))
		router := NewRouter(deps)
		body := `{"auth_code":"a","code_verifier":"v"}`

		// Act: 同一 IP から連続リクエスト
		for i := 0; i < 5; i++ {
			w := doNativeAuthPost(router, "/api/auth/token", body, "203.0.113.60:50000")
			// Assert: すべて通過（制限なし）
			if w.Result().StatusCode != http.StatusOK {
				t.Fatalf("request %d status = %d, want 200 (limiter nil は素通し)", i+1, w.Result().StatusCode)
			}
		}
		if svc.callCount != 5 {
			t.Errorf("service calls = %d, want 5", svc.callCount)
		}
	})
}

// --- Session route（Issue #223 / task 2 / design.md §Router 追加） ---

// alwaysSucceedSessionExchange は SessionExchanger 最小 IF の固定成功モック（route 到達判定用）。
type alwaysSucceedSessionExchange struct {
	callCount int
}

func (s *alwaysSucceedSessionExchange) ExchangeAuthCodeForSession(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
	s.callCount++
	return &model.Session{
		ID:        "route-test-session-id",
		UserID:    "user-route-test",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}, nil
}

// newDepsForSessionRoute は Session route 到達確認用に NativeAuthHandler
// （SessionExchanger 注入済み）を組み立てた最小 deps を返す。
func newDepsForSessionRoute(sessionSvc SessionExchanger) *RouterDeps {
	nh := NewNativeAuthHandler(
		&alwaysSucceedExchangeService{},
		WithSessionExchange(sessionSvc, "example.com", true, 86400),
	)
	return newMinimalDepsForNativeAuth(nh)
}

// TestNewRouter_NativeAuthSession_RegisteredWhenHandlerInjected は NativeAuthHandler を
// 注入したとき POST /api/auth/session がセッション無しで到達し、204 が返ることを検証する
// （Req 3.1 / 4.2: Cookie / Bearer なしで呼び出し可能）。
func TestNewRouter_NativeAuthSession_RegisteredWhenHandlerInjected(t *testing.T) {
	// Arrange
	sessionSvc := &alwaysSucceedSessionExchange{}
	router := NewRouter(newDepsForSessionRoute(sessionSvc))

	body := `{"auth_code":"plain-auth-code","code_verifier":"plain-verifier"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d (handler 到達 / Req 3.1 / 4.2)",
			resp.StatusCode, http.StatusNoContent)
	}
	if sessionSvc.callCount != 1 {
		t.Errorf("session service called %d times, want 1 (handler に到達していない可能性)",
			sessionSvc.callCount)
	}
	// Set-Cookie が発行されている（既存 OAuth Callback と同等の挙動 / Req 3.1 / 4.2）
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			found = true
			break
		}
	}
	if !found {
		t.Error("session_id cookie was not set (Req 3.1 / 4.2: 成功時 Set-Cookie 必須)")
	}
}

// TestNewRouter_NativeAuthSession_NotRegisteredWhenHandlerNil は NativeAuthHandler が
// nil のとき POST /api/auth/session がルートとして登録されず 404 が返ることを検証する
// （NFR 2.2: 署名鍵未設定環境の fail-closed / 既存 3 route と連動）。
func TestNewRouter_NativeAuthSession_NotRegisteredWhenHandlerNil(t *testing.T) {
	// Arrange: NativeAuthHandler nil
	router := NewRouter(newMinimalDepsForNativeAuth(nil))

	body := `{"auth_code":"a","code_verifier":"v"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: fail-closed として 404
	if got := w.Result().StatusCode; got != http.StatusNotFound {
		t.Errorf("status = %d, want %d (NativeAuthHandler nil で fail-closed / NFR 2.2)",
			got, http.StatusNotFound)
	}
}

// TestNewRouter_NativeAuthSession_DoesNotRequireSession は注入時に Cookie 無しでも
// 401 を返さず handler まで到達することを検証する（Req 3.1 / 4.2: Session middleware 通らない）。
func TestNewRouter_NativeAuthSession_DoesNotRequireSession(t *testing.T) {
	// Arrange
	sessionSvc := &alwaysSucceedSessionExchange{}
	router := NewRouter(newDepsForSessionRoute(sessionSvc))

	body := `{"auth_code":"a","code_verifier":"v"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// セッション Cookie 無し
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: 401 ではなく 204（Session middleware を経由していない）
	resp := w.Result()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("status = 401, want non-401 (Req 3.1 / 4.2: Cookie 無しで呼び出し可能)")
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
