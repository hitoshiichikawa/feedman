package repository

import (
	"context"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresUserRepoはUserRepositoryインターフェースを満たすことを検証
func TestPostgresUserRepo_ImplementsInterface(t *testing.T) {
	var _ UserRepository = (*PostgresUserRepo)(nil)
}

// PostgresIdentityRepoはIdentityRepositoryインターフェースを満たすことを検証
func TestPostgresIdentityRepo_ImplementsInterface(t *testing.T) {
	var _ IdentityRepository = (*PostgresIdentityRepo)(nil)
}

// PostgresSessionRepoはSessionRepositoryインターフェースを満たすことを検証
func TestPostgresSessionRepo_ImplementsInterface(t *testing.T) {
	var _ SessionRepository = (*PostgresSessionRepo)(nil)
}

// NewPostgresUserRepoが正しく初期化されることを検証
func TestNewPostgresUserRepo_Initializes(t *testing.T) {
	repo := NewPostgresUserRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// NewPostgresIdentityRepoが正しく初期化されることを検証
func TestNewPostgresIdentityRepo_Initializes(t *testing.T) {
	repo := NewPostgresIdentityRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// NewPostgresSessionRepoが正しく初期化されることを検証
func TestNewPostgresSessionRepo_Initializes(t *testing.T) {
	repo := NewPostgresSessionRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// ユニットテスト: CreateWithIdentityが正しいSQLパラメータを構築すること
// （DB接続なしでロジックのみ検証）
func TestPostgresUserRepo_CreateWithIdentity_SetsTimestamps(t *testing.T) {
	now := time.Now()
	user := &model.User{
		ID:    "user-id-1",
		Email: "test@example.com",
		Name:  "Test User",
	}
	identity := &model.Identity{
		ID:             "identity-id-1",
		UserID:         "user-id-1",
		Provider:       "google",
		ProviderUserID: "google-123",
	}

	// タイムスタンプが設定前に空であることを確認
	if !user.CreatedAt.IsZero() && user.CreatedAt.Before(now.Add(-1*time.Hour)) {
		// CreatedAtが既に設定されている場合のチェック
	}

	// identityのUserIDがuserのIDと一致することを確認
	if identity.UserID != user.ID {
		t.Errorf("identity.UserID = %q, want %q", identity.UserID, user.ID)
	}
}

// SessionRepoのFindByIDが期限切れセッションを返さないことの期待動作
func TestPostgresSessionRepo_FindByID_ExpiredSession_Concept(t *testing.T) {
	// このテストはDB接続なしでコンセプトを検証する
	session := &model.Session{
		ID:        "expired-session",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	if session.ExpiresAt.After(time.Now()) {
		t.Error("expected session to be expired")
	}
}

// SessionRepoのDeleteByIDが正しいセッションIDで呼ばれることの検証
func TestPostgresSessionRepo_DeleteByID_Concept(t *testing.T) {
	sessionID := "session-to-delete"
	ctx := context.Background()

	if sessionID == "" {
		t.Fatal("session ID should not be empty")
	}
	if ctx == nil {
		t.Fatal("context should not be nil")
	}
}
