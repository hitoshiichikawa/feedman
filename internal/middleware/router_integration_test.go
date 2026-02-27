package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hitoshi/feedman/internal/model"
)

// TestRouterIntegration_CSRFTokenEndpoint はCSRFトークン取得エンドポイントが
// chi.Routerで正しく動作することを検証する。
func TestRouterIntegration_CSRFTokenEndpoint(t *testing.T) {
	r := chi.NewRouter()

	csrfConfig := CSRFConfig{CookieSecure: false}
	r.Get("/api/csrf-token", NewCSRFTokenHandler(csrfConfig).ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Token == "" {
		t.Error("expected non-empty token")
	}
}

// TestRouterIntegration_ProtectedRoute_WithMiddlewareChain は
// Session -> CSRF のミドルウェアチェーンがchi.Routerで正しく動作することを検証する。
func TestRouterIntegration_ProtectedRoute_WithMiddlewareChain(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			if id == "router-test-session" {
				return &model.Session{
					ID:        "router-test-session",
					UserID:    "user-router-test",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return nil, nil
		},
	}

	r := chi.NewRouter()

	csrfConfig := CSRFConfig{CookieSecure: false}

	// CSRFトークン取得エンドポイント（認証不要）
	r.Get("/api/csrf-token", NewCSRFTokenHandler(csrfConfig).ServeHTTP)

	// 認証が必要なルートグループ
	r.Group(func(r chi.Router) {
		r.Use(NewSessionMiddleware(repo))
		r.Use(NewCSRFMiddleware(csrfConfig))

		r.Get("/api/protected", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := UserIDFromContext(r.Context())
			json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
		})

		r.Post("/api/action", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := UserIDFromContext(r.Context())
			json.NewEncoder(w).Encode(map[string]string{"user_id": userID, "action": "done"})
		})
	})

	// テスト1: GET /api/protected は認証あり + CSRFなしで通る
	t.Run("GET_protected_with_session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "router-test-session"})
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
		}
	})

	// テスト2: GET /api/protected は認証なしで401
	t.Run("GET_protected_no_session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
		}
	})

	// テスト3: POST /api/action は認証あり + CSRFトークンで通る
	t.Run("POST_action_with_session_and_csrf", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "router-test-session"})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf-token"})
		req.Header.Set(csrfHeaderName, "test-csrf-token")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
		}

		var body map[string]string
		json.NewDecoder(w.Result().Body).Decode(&body)
		if body["user_id"] != "user-router-test" {
			t.Errorf("user_id = %q, want %q", body["user_id"], "user-router-test")
		}
	})

	// テスト4: POST /api/action は認証あり + CSRFトークンなしで403
	t.Run("POST_action_without_csrf", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "router-test-session"})
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
		}
	})

	// テスト5: POST /api/action は認証なしで401（CSRFチェックの前にセッションチェック）
	t.Run("POST_action_no_session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
		}
	})

	// テスト6: CSRFトークンエンドポイントは認証不要
	t.Run("CSRF_token_endpoint_no_auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
		}
	})
}
