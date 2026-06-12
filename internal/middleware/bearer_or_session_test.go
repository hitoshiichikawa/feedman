package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義（Issue #169） ---

// stubJWTVerifier は JWTVerifier 最小 IF の record-and-return スタブ。
type stubJWTVerifier struct {
	verifyFn    func(tokenString string) (string, error)
	verifyCalls int
	lastToken   string
}

func (s *stubJWTVerifier) VerifyAccessToken(tokenString string) (string, error) {
	s.verifyCalls++
	s.lastToken = tokenString
	if s.verifyFn != nil {
		return s.verifyFn(tokenString)
	}
	return "", errors.New("not configured")
}

// countingSessionFinder は FindByID の呼び出し回数を記録する SessionFinder スタブ。
// Bearer パスで sessionFinder が呼ばれないこと（Req 1.3 / 2.4）の検証に使う。
type countingSessionFinder struct {
	findByIDFn func(ctx context.Context, id string) (*model.Session, error)
	findCalls  int
}

func (m *countingSessionFinder) FindByID(ctx context.Context, id string) (*model.Session, error) {
	m.findCalls++
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

// validSessionFinder は固定の有効セッションを返す countingSessionFinder を生成する。
func validSessionFinder(sessionID, userID string) *countingSessionFinder {
	return &countingSessionFinder{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			if id == sessionID {
				return &model.Session{
					ID:        sessionID,
					UserID:    userID,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return nil, nil
		},
	}
}

// captureHandler は下流到達時に UserIDFromContext の値を捕捉して 200 を返す handler を生成する。
func captureHandler(t *testing.T, capturedUserID *string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := UserIDFromContext(r.Context())
		if err != nil {
			t.Errorf("UserIDFromContext returned error: %v", err)
		}
		*capturedUserID = userID
		w.WriteHeader(http.StatusOK)
	})
}

// --- テスト（design.md Testing Strategy middleware 1〜6） ---

// TestBearerOrSession_ValidBearer_InjectsUserID は有効 Bearer で下流 handler が
// UserIDFromContext で userID を取得でき、sessionFinder が呼ばれないことを検証する
// （Testing Strategy 1 / Req 1.1, 1.3）。
func TestBearerOrSession_ValidBearer_InjectsUserID(t *testing.T) {
	// Arrange
	verifier := &stubJWTVerifier{
		verifyFn: func(tokenString string) (string, error) { return "user-bearer-1", nil },
	}
	finder := &countingSessionFinder{}
	mw := NewBearerOrSessionMiddleware(verifier, finder)

	var capturedUserID string
	handler := mw(captureHandler(t, &capturedUserID))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (Req 1.1)", w.Result().StatusCode, http.StatusOK)
	}
	if capturedUserID != "user-bearer-1" {
		t.Errorf("userID = %q, want %q (Cookie セッションと同一の context 注入)", capturedUserID, "user-bearer-1")
	}
	if verifier.verifyCalls != 1 {
		t.Errorf("VerifyAccessToken calls = %d, want 1", verifier.verifyCalls)
	}
	if verifier.lastToken != "valid-token" {
		t.Errorf("verifier received token = %q, want %q", verifier.lastToken, "valid-token")
	}
	if finder.findCalls != 0 {
		t.Errorf("sessionFinder calls = %d, want 0 (Bearer パスは session を参照しない / Req 1.3)", finder.findCalls)
	}
}

// TestBearerOrSession_InvalidBearerWithValidCookie_Returns401 は無効 Bearer + 有効 Cookie
// 併送で 401 を返し、Cookie へ fallback しない（sessionFinder 不呼び出し）ことを検証する
// （Testing Strategy 2 / Req 2.4 / Req 1.4 の裏面）。
func TestBearerOrSession_InvalidBearerWithValidCookie_Returns401(t *testing.T) {
	// Arrange: verifier は常に失敗、Cookie は有効セッションを指す
	verifier := &stubJWTVerifier{
		verifyFn: func(tokenString string) (string, error) { return "", errors.New("token is expired") },
	}
	finder := validSessionFinder("valid-session-id", "user-cookie-1")
	mw := NewBearerOrSessionMiddleware(verifier, finder)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("downstream handler must not be reached")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session-id"})
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (無効 Bearer は Cookie へ fallback しない / Req 2.4)",
			w.Result().StatusCode, http.StatusUnauthorized)
	}
	if finder.findCalls != 0 {
		t.Errorf("sessionFinder calls = %d, want 0 (fallback 禁止 / Req 2.4)", finder.findCalls)
	}
}

// TestBearerOrSession_401SameShapeAsSessionMiddleware は Bearer 拒否の 401 応答が
// 既存 SessionMiddleware の未認証応答と同一（status / body / Content-Type）であることを
// 検証する（Testing Strategy 3 / Req 2.6）。
func TestBearerOrSession_401SameShapeAsSessionMiddleware(t *testing.T) {
	// Arrange: 比較基準として既存 SessionMiddleware の未認証応答を取得する
	sessionMW := NewSessionMiddleware(&countingSessionFinder{})
	sessionHandler := sessionMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseReq := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	baseW := httptest.NewRecorder()
	sessionHandler.ServeHTTP(baseW, baseReq)
	baseResp := baseW.Result()
	baseBody, _ := io.ReadAll(baseResp.Body)

	// Bearer 拒否側の応答
	verifier := &stubJWTVerifier{
		verifyFn: func(tokenString string) (string, error) { return "", errors.New("signature is invalid") },
	}
	mw := NewBearerOrSessionMiddleware(verifier, &countingSessionFinder{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: status / body / Content-Type が完全一致
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != baseResp.StatusCode {
		t.Errorf("status = %d, want %d (SessionMiddleware と同形 / Req 2.6)", resp.StatusCode, baseResp.StatusCode)
	}
	if string(body) != string(baseBody) {
		t.Errorf("body = %q, want %q (Req 2.6)", string(body), string(baseBody))
	}
	if got, want := resp.Header.Get("Content-Type"), baseResp.Header.Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q (Req 2.6)", got, want)
	}
}

// TestBearerOrSession_NoAuthorizationHeader_DelegatesToSession は Authorization 無しの
// 要求が既存 SessionMiddleware に委譲されることを検証する（Testing Strategy 4 /
// Req 3.1, 3.3, 3.4）。
func TestBearerOrSession_NoAuthorizationHeader_DelegatesToSession(t *testing.T) {
	t.Run("有効 Cookie のとき従来どおり 200", func(t *testing.T) {
		// Arrange
		verifier := &stubJWTVerifier{}
		finder := validSessionFinder("valid-session-id", "user-cookie-1")
		mw := NewBearerOrSessionMiddleware(verifier, finder)

		var capturedUserID string
		handler := mw(captureHandler(t, &capturedUserID))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session-id"})
		w := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(w, req)

		// Assert
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d (Req 3.1 / 3.3)", w.Result().StatusCode, http.StatusOK)
		}
		if capturedUserID != "user-cookie-1" {
			t.Errorf("userID = %q, want %q", capturedUserID, "user-cookie-1")
		}
		if verifier.verifyCalls != 0 {
			t.Errorf("VerifyAccessToken calls = %d, want 0 (Bearer 不在では検証しない)", verifier.verifyCalls)
		}
	})

	t.Run("Cookie 無しのとき従来どおり 401", func(t *testing.T) {
		// Arrange
		mw := NewBearerOrSessionMiddleware(&stubJWTVerifier{}, &countingSessionFinder{})
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("downstream handler must not be reached")
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		w := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(w, req)

		// Assert
		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d (Req 3.4)", w.Result().StatusCode, http.StatusUnauthorized)
		}
	})
}

// TestBearerOrSession_SchemeAndTokenEdgeCases は scheme / token 部の境界ケースを検証する
// （Testing Strategy 5 / Req 2.5, 3.2 / 小文字 bearer の受理）。
func TestBearerOrSession_SchemeAndTokenEdgeCases(t *testing.T) {
	cases := []struct {
		name          string
		authorization string
		wantStatus    int
		wantDelegate  bool // true なら session 委譲（Cookie 無しのため 401 になる）
		wantVerify    int  // VerifyAccessToken の期待呼び出し回数
	}{
		{
			name:          "Basic scheme のとき session へ委譲（Req 3.2）",
			authorization: "Basic dXNlcjpwYXNz",
			wantStatus:    http.StatusUnauthorized, // Cookie 無しのため委譲先で 401
			wantDelegate:  true,
			wantVerify:    0,
		},
		{
			name:          "Bearer 単独（token 部なし）のとき 401（Req 2.5）",
			authorization: "Bearer",
			wantStatus:    http.StatusUnauthorized,
			wantDelegate:  false,
			wantVerify:    0,
		},
		{
			name:          "Bearer + 空白のみのとき 401（Req 2.5）",
			authorization: "Bearer ",
			wantStatus:    http.StatusUnauthorized,
			wantDelegate:  false,
			wantVerify:    0,
		},
		{
			name:          "小文字 bearer のとき Bearer として受理（RFC 9110 §11.1）",
			authorization: "bearer valid-token",
			wantStatus:    http.StatusOK,
			wantDelegate:  false,
			wantVerify:    1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			verifier := &stubJWTVerifier{
				verifyFn: func(tokenString string) (string, error) { return "user-bearer-1", nil },
			}
			finder := &countingSessionFinder{}
			mw := NewBearerOrSessionMiddleware(verifier, finder)
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Authorization", tc.authorization)
			w := httptest.NewRecorder()

			// Act
			handler.ServeHTTP(w, req)

			// Assert
			if w.Result().StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Result().StatusCode, tc.wantStatus)
			}
			if verifier.verifyCalls != tc.wantVerify {
				t.Errorf("VerifyAccessToken calls = %d, want %d", verifier.verifyCalls, tc.wantVerify)
			}
			if tc.wantDelegate && finder.findCalls != 0 {
				// Cookie 無しの委譲は FindByID 到達前に 401 になるため 0 回で正しい。
				// （委譲自体の検証は応答形状 = SessionMiddleware の 401 で担保）
				t.Errorf("sessionFinder calls = %d, want 0 (Cookie 無しは FindByID 前に 401)", finder.findCalls)
			}
		})
	}
}

// TestBearerOrSession_NilVerifier_FallsBackToSessionMiddleware は verifier nil のとき
// Bearer 付き要求も Cookie で評価され、本機能導入前と同一挙動になることを検証する
// （Testing Strategy 6 / Req 4.2, 4.3）。
func TestBearerOrSession_NilVerifier_FallsBackToSessionMiddleware(t *testing.T) {
	t.Run("Bearer 付き + 有効 Cookie のとき Cookie で認証成立（Bearer は評価されない）", func(t *testing.T) {
		// Arrange: verifier nil。Bearer token は不正値だが評価されないため影響しない
		finder := validSessionFinder("valid-session-id", "user-cookie-1")
		mw := NewBearerOrSessionMiddleware(nil, finder)

		var capturedUserID string
		handler := mw(captureHandler(t, &capturedUserID))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer totally-invalid-token")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session-id"})
		w := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(w, req)

		// Assert: Cookie 認証で成立（Req 4.3: token を評価せず従来同一処理）
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d (Req 4.2 / 4.3)", w.Result().StatusCode, http.StatusOK)
		}
		if capturedUserID != "user-cookie-1" {
			t.Errorf("userID = %q, want %q (Cookie 由来)", capturedUserID, "user-cookie-1")
		}
		if finder.findCalls != 1 {
			t.Errorf("sessionFinder calls = %d, want 1 (従来どおり Cookie 評価)", finder.findCalls)
		}
	})

	t.Run("Bearer 付き + Cookie 無しのとき従来どおり 401", func(t *testing.T) {
		// Arrange
		mw := NewBearerOrSessionMiddleware(nil, &countingSessionFinder{})
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("downstream handler must not be reached")
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		w := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(w, req)

		// Assert
		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d (導入前と同一の未認証応答)", w.Result().StatusCode, http.StatusUnauthorized)
		}
	})
}
