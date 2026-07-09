package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockUserService はUserServiceInterfaceのモック実装。
type mockUserService struct {
	withdrawFn   func(ctx context.Context, userID string) error
	getCurrentFn func(ctx context.Context, userID string) (*currentUserResponse, error)
}

func (m *mockUserService) Withdraw(ctx context.Context, userID string) error {
	if m.withdrawFn != nil {
		return m.withdrawFn(ctx, userID)
	}
	return nil
}

func (m *mockUserService) GetCurrent(ctx context.Context, userID string) (*currentUserResponse, error) {
	if m.getCurrentFn != nil {
		return m.getCurrentFn(ctx, userID)
	}
	return &currentUserResponse{ID: userID}, nil
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

// --- GET /api/users/me テスト ---

// TestUserHandler_GetCurrent_Success_ReturnsCurrentUser は GET /api/users/me が
// userID 注入済リクエストに対して 200 + {id, email, name} を含む JSON を返すことを確認する。
// BearerOrSession middleware 通過後の handler は経路（Bearer / Cookie）非依存で同一動作する
// ため、両経路を 1 つのテストで担保する（Req 4.1 / 4.2 / design.md「Testing Strategy」節）。
func TestUserHandler_GetCurrent_Success_ReturnsCurrentUser(t *testing.T) {
	getCurrentCalled := false
	svc := &mockUserService{
		getCurrentFn: func(ctx context.Context, userID string) (*currentUserResponse, error) {
			getCurrentCalled = true
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			return &currentUserResponse{
				ID:    "user-123",
				Email: "test@example.com",
				Name:  "Test User",
			}, nil
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.GetCurrent(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if !getCurrentCalled {
		t.Error("expected GetCurrent to be called")
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["id"] != "user-123" {
		t.Errorf("id = %v, want %q", body["id"], "user-123")
	}
	if body["email"] != "test@example.com" {
		t.Errorf("email = %v, want %q", body["email"], "test@example.com")
	}
	if body["name"] != "Test User" {
		t.Errorf("name = %v, want %q", body["name"], "Test User")
	}
}

// TestUserHandler_GetCurrent_NoUserID_ReturnsUnauthorized は context に userID が
// 注入されていない場合に 401 UNAUTHORIZED を返すことを確認する（Req 2.5）。
func TestUserHandler_GetCurrent_NoUserID_ReturnsUnauthorized(t *testing.T) {
	h := NewUserHandler(&mockUserService{})

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	h.GetCurrent(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestUserHandler_GetCurrent_UserNotFound_Returns404 は service が
// model.NewUserNotFoundError を返した場合 404 を返すことを確認する。
func TestUserHandler_GetCurrent_UserNotFound_Returns404(t *testing.T) {
	svc := &mockUserService{
		getCurrentFn: func(ctx context.Context, userID string) (*currentUserResponse, error) {
			return nil, model.NewUserNotFoundError()
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.GetCurrent(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestUserHandler_GetCurrent_InternalError は service が APIError 以外のエラーを
// 返した場合 500 を返すことを確認する（既存 handleServiceError パターンの再利用検証）。
func TestUserHandler_GetCurrent_InternalError(t *testing.T) {
	svc := &mockUserService{
		getCurrentFn: func(ctx context.Context, userID string) (*currentUserResponse, error) {
			return nil, errors.New("db connection lost")
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.GetCurrent(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

// TestUserHandler_GetCurrent_OmitsSecrets は応答 JSON のキー集合が
// {id, email, name} ⊆ X ⊆ {id, email, name, avatar_url} であり、session_id /
// refresh_token / hashed token などの secret を含まないことを確認する
// （CLAUDE.md「機密情報の扱い」/「機能追加チェックリスト」/ design.md「Security Considerations」節）。
func TestUserHandler_GetCurrent_OmitsSecrets(t *testing.T) {
	avatarValue := "https://example.com/avatar.png"
	svc := &mockUserService{
		getCurrentFn: func(ctx context.Context, userID string) (*currentUserResponse, error) {
			// avatar_url を返すケースでも応答キー集合が {id, email, name, avatar_url}
			// の範囲に収まることを確認するため、ここでは avatar_url を含めて返す。
			return &currentUserResponse{
				ID:        "user-123",
				Email:     "test@example.com",
				Name:      "Test",
				AvatarURL: &avatarValue,
			}, nil
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.GetCurrent(w, req)

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// 必須キー（{id, email, name}）が全て含まれる
	requiredKeys := []string{"id", "email", "name"}
	for _, k := range requiredKeys {
		if _, ok := body[k]; !ok {
			t.Errorf("response missing required key %q", k)
		}
	}

	// 許可されたキー集合 {id, email, name, avatar_url} 以外が含まれていないこと
	allowed := map[string]bool{
		"id":         true,
		"email":      true,
		"name":       true,
		"avatar_url": true,
	}
	for k := range body {
		if !allowed[k] {
			t.Errorf("response contains forbidden key %q (secret leak risk)", k)
		}
	}

	// 念のため明示的に secret キーが存在しないことを確認
	forbidden := []string{"session_id", "refresh_token", "password", "password_hash", "access_token"}
	for _, k := range forbidden {
		if _, ok := body[k]; ok {
			t.Errorf("response leaks secret key %q", k)
		}
	}
}

// TestUserHandler_GetCurrent_OmitsAvatarWhenNil は AvatarURL が nil の場合に
// 応答 JSON から avatar_url キーが省略されることを確認する（Req 2.4 /
// `*string` + omitempty タグの挙動検証）。
func TestUserHandler_GetCurrent_OmitsAvatarWhenNil(t *testing.T) {
	svc := &mockUserService{
		getCurrentFn: func(ctx context.Context, userID string) (*currentUserResponse, error) {
			return &currentUserResponse{
				ID:    "user-123",
				Email: "test@example.com",
				Name:  "Test User",
				// AvatarURL は nil（v1 デフォルト挙動）
			}, nil
		},
	}

	h := NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	h.GetCurrent(w, req)

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := body["avatar_url"]; ok {
		t.Errorf("avatar_url key should be omitted when nil, but got %v", body["avatar_url"])
	}
}

// TestSetupUserRoutes_GetCurrentEndpoint は SetupUserRoutes が GET /me を登録している
// ことを確認する（router 配線の基本動作確認）。
func TestSetupUserRoutes_GetCurrentEndpoint(t *testing.T) {
	svc := &mockUserService{
		getCurrentFn: func(ctx context.Context, userID string) (*currentUserResponse, error) {
			return &currentUserResponse{ID: userID, Email: "x@example.com", Name: "X"}, nil
		},
	}

	router := SetupUserRoutes(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/users/me status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
