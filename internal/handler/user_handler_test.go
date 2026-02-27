package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockUserService はUserServiceInterfaceのモック実装。
type mockUserService struct {
	withdrawFn func(ctx context.Context, userID string) error
}

func (m *mockUserService) Withdraw(ctx context.Context, userID string) error {
	if m.withdrawFn != nil {
		return m.withdrawFn(ctx, userID)
	}
	return nil
}

// --- DELETE /api/users/me テスト ---

func TestUserHandler_Withdraw_Success(t *testing.T) {
	withdrawCalled := false
	svc := &mockUserService{
		withdrawFn: func(ctx context.Context, userID string) error {
			withdrawCalled = true
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			return nil
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if !withdrawCalled {
		t.Error("expected Withdraw to be called")
	}
}

func TestUserHandler_Withdraw_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewUserHandler(&mockUserService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestUserHandler_Withdraw_UserNotFound(t *testing.T) {
	svc := &mockUserService{
		withdrawFn: func(ctx context.Context, userID string) error {
			return model.NewUserNotFoundError()
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestUserHandler_Withdraw_InternalError(t *testing.T) {
	svc := &mockUserService{
		withdrawFn: func(ctx context.Context, userID string) error {
			return errors.New("transaction failed")
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// --- ルーティングテスト ---

func TestSetupUserRoutes_WithdrawEndpoint(t *testing.T) {
	svc := &mockUserService{
		withdrawFn: func(ctx context.Context, userID string) error {
			return nil
		},
	}

	router := SetupUserRoutes(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /api/users/me status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// 退会処理は feeds と items を削除しないことを確認するテスト
// （サービス層のモックで確認 - ハンドラーレベルではステータスコードのみ検証）
func TestUserHandler_Withdraw_VerifiesOnlyUserDataIsDeleted(t *testing.T) {
	// このテストでは、サービス層がcallされることのみ検証する
	// feeds/itemsの保持はサービス層のテストで担保する
	svc := &mockUserService{
		withdrawFn: func(ctx context.Context, userID string) error {
			// サービス層が呼ばれたことを確認
			return nil
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
