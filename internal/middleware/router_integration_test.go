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

// TestRouterIntegration_CORSMiddleware はCORSミドルウェアが
// chi.Routerで正しく動作することを検証する。
func TestRouterIntegration_CORSMiddleware(t *testing.T) {
	r := chi.NewRouter()
	r.Use(NewCORSMiddleware("http://localhost:3000"))

	r.Get("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("GET_returns_CORS_headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
		}
	})

	t.Run("OPTIONS_preflight_returns_204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}
		if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
		}
	})
}

// TestRouterIntegration_ProtectedRoute_WithMiddlewareChain は
// CORS → Session のミドルウェアチェーンがchi.Routerで正しく動作することを検証する。
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
	r.Use(NewCORSMiddleware("http://localhost:3000"))

	// 認証が必要なルートグループ
	r.Group(func(r chi.Router) {
		r.Use(NewSessionMiddleware(repo))

		r.Get("/api/protected", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := UserIDFromContext(r.Context())
			json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
		})

		r.Post("/api/action", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := UserIDFromContext(r.Context())
			json.NewEncoder(w).Encode(map[string]string{"user_id": userID, "action": "done"})
		})
	})

	// テスト1: GET /api/protected は認証ありで通る
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

	// テスト3: POST /api/action は認証ありで通る（CSRF不要）
	t.Run("POST_action_with_session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "router-test-session"})
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

	// テスト4: POST /api/action は認証なしで401
	t.Run("POST_action_no_session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
		}
	})

	// テスト5: CORSヘッダーが認証エラーレスポンスにも付与されること
	t.Run("CORS_headers_on_401_response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
		}
	})
}
