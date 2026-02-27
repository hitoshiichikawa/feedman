package user

import (
	"context"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック ---

type mockUserRepo struct {
	findByIDFn   func(ctx context.Context, id string) (*model.User, error)
	deleteByIDFn func(ctx context.Context, id string) error
}

func (m *mockUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockUserRepo) CreateWithIdentity(ctx context.Context, user *model.User, identity *model.Identity) error {
	return nil
}
func (m *mockUserRepo) DeleteByID(ctx context.Context, id string) error {
	return m.deleteByIDFn(ctx, id)
}

type mockSessionRepo struct {
	deleteByUserIDFn func(ctx context.Context, userID string) error
}

func (m *mockSessionRepo) Create(ctx context.Context, session *model.Session) error {
	return nil
}
func (m *mockSessionRepo) FindByID(ctx context.Context, id string) (*model.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) DeleteByID(ctx context.Context, id string) error {
	return nil
}
func (m *mockSessionRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return m.deleteByUserIDFn(ctx, userID)
}

type mockSubRepo struct {
	deleteByUserIDFn func(ctx context.Context, userID string) error
}

func (m *mockSubRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return m.deleteByUserIDFn(ctx, userID)
}

type mockItemStateRepo struct {
	deleteByUserIDFn func(ctx context.Context, userID string) error
}

func (m *mockItemStateRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return m.deleteByUserIDFn(ctx, userID)
}

// --- テスト ---

// TestService_Withdraw は退会処理が全関連データを削除することを検証する。
func TestService_Withdraw(t *testing.T) {
	userDeleteCalled := false
	sessionDeleteCalled := false
	subDeleteCalled := false
	itemStateDeleteCalled := false

	userRepo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Email: "test@example.com"}, nil
		},
		deleteByIDFn: func(ctx context.Context, id string) error {
			userDeleteCalled = true
			return nil
		},
	}
	sessionRepo := &mockSessionRepo{
		deleteByUserIDFn: func(ctx context.Context, userID string) error {
			sessionDeleteCalled = true
			return nil
		},
	}
	subRepo := &mockSubRepo{
		deleteByUserIDFn: func(ctx context.Context, userID string) error {
			subDeleteCalled = true
			return nil
		},
	}
	itemStateRepo := &mockItemStateRepo{
		deleteByUserIDFn: func(ctx context.Context, userID string) error {
			itemStateDeleteCalled = true
			return nil
		},
	}

	svc := NewService(userRepo, sessionRepo, subRepo, itemStateRepo)

	err := svc.Withdraw(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}
	if !itemStateDeleteCalled {
		t.Error("expected item_states DeleteByUserID to be called")
	}
	if !subDeleteCalled {
		t.Error("expected subscriptions DeleteByUserID to be called")
	}
	if !sessionDeleteCalled {
		t.Error("expected sessions DeleteByUserID to be called")
	}
	if !userDeleteCalled {
		t.Error("expected user DeleteByID to be called")
	}
}

// TestService_Withdraw_UserNotFound は存在しないユーザーの退会がエラーになることを検証する。
func TestService_Withdraw_UserNotFound(t *testing.T) {
	userRepo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return nil, nil
		},
	}

	svc := NewService(userRepo, nil, nil, nil)

	err := svc.Withdraw(context.Background(), "nonexistent-user")
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
}
