package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

type mockSessionRepository struct {
	findByIDFn func(ctx context.Context, id string) (*model.Session, error)
}

func (m *mockSessionRepository) FindByID(ctx context.Context, id string) (*model.Session, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

// --- テスト ---

func TestSessionMiddleware_ValidSession_InjectsUserID(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			if id == "valid-session-id" {
				return &model.Session{
					ID:        "valid-session-id",
					UserID:    "user-123",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return nil, nil
		},
	}

	mw := NewSessionMiddleware(repo)

	var capturedUserID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := UserIDFromContext(r.Context())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		capturedUserID = userID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session-id"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if capturedUserID != "user-123" {
		t.Errorf("userID = %q, want %q", capturedUserID, "user-123")
	}
}

func TestSessionMiddleware_NoSessionCookie_Returns401(t *testing.T) {
	repo := &mockSessionRepository{}
	mw := NewSessionMiddleware(repo)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSessionMiddleware_EmptySessionCookie_Returns401(t *testing.T) {
	repo := &mockSessionRepository{}
	mw := NewSessionMiddleware(repo)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: ""})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSessionMiddleware_ExpiredSession_Returns401(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			// セッションが見つからない（期限切れでnilを返すリポジトリの動作をシミュレート）
			return nil, nil
		},
	}
	mw := NewSessionMiddleware(repo)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "expired-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSessionMiddleware_RepositoryError_Returns401(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			return nil, context.DeadlineExceeded
		},
	}
	mw := NewSessionMiddleware(repo)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "some-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestUserIDFromContext_NoValue_ReturnsError(t *testing.T) {
	ctx := context.Background()
	_, err := UserIDFromContext(ctx)
	if err == nil {
		t.Error("expected error for missing user ID in context")
	}
}

func TestUserIDFromContext_ValidValue_ReturnsUserID(t *testing.T) {
	ctx := context.WithValue(context.Background(), userIDContextKey, "user-456")
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if userID != "user-456" {
		t.Errorf("userID = %q, want %q", userID, "user-456")
	}
}
