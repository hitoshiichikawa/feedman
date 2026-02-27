package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- モック定義 ---

type mockUserRepo struct {
	findByIDFn           func(ctx context.Context, id string) (*model.User, error)
	createWithIdentityFn func(ctx context.Context, user *model.User, identity *model.Identity) error
}

func (m *mockUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockUserRepo) CreateWithIdentity(ctx context.Context, user *model.User, identity *model.Identity) error {
	if m.createWithIdentityFn != nil {
		return m.createWithIdentityFn(ctx, user, identity)
	}
	return nil
}

func (m *mockUserRepo) DeleteByID(_ context.Context, _ string) error {
	return nil
}

type mockIdentityRepo struct {
	findByProviderFn func(ctx context.Context, provider, providerUserID string) (*model.Identity, error)
}

func (m *mockIdentityRepo) FindByProviderAndProviderUserID(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
	if m.findByProviderFn != nil {
		return m.findByProviderFn(ctx, provider, providerUserID)
	}
	return nil, nil
}

type mockSessionRepo struct {
	createFn         func(ctx context.Context, session *model.Session) error
	findByIDFn       func(ctx context.Context, id string) (*model.Session, error)
	deleteByIDFn     func(ctx context.Context, id string) error
	deleteByUserIDFn func(ctx context.Context, userID string) error
}

func (m *mockSessionRepo) Create(ctx context.Context, session *model.Session) error {
	if m.createFn != nil {
		return m.createFn(ctx, session)
	}
	return nil
}

func (m *mockSessionRepo) FindByID(ctx context.Context, id string) (*model.Session, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockSessionRepo) DeleteByID(ctx context.Context, id string) error {
	if m.deleteByIDFn != nil {
		return m.deleteByIDFn(ctx, id)
	}
	return nil
}

func (m *mockSessionRepo) DeleteByUserID(ctx context.Context, userID string) error {
	if m.deleteByUserIDFn != nil {
		return m.deleteByUserIDFn(ctx, userID)
	}
	return nil
}

type mockOAuthProvider struct {
	getLoginURLFn  func(state string) string
	exchangeCodeFn func(ctx context.Context, code string) (*OAuthUserInfo, error)
}

func (m *mockOAuthProvider) GetLoginURL(state string) string {
	if m.getLoginURLFn != nil {
		return m.getLoginURLFn(state)
	}
	return ""
}

func (m *mockOAuthProvider) ExchangeCode(ctx context.Context, code string) (*OAuthUserInfo, error) {
	if m.exchangeCodeFn != nil {
		return m.exchangeCodeFn(ctx, code)
	}
	return nil, nil
}

// --- compile-time interface checks ---
var _ repository.UserRepository = (*mockUserRepo)(nil)
var _ repository.IdentityRepository = (*mockIdentityRepo)(nil)
var _ repository.SessionRepository = (*mockSessionRepo)(nil)
var _ OAuthProvider = (*mockOAuthProvider)(nil)

// --- テスト ---

func TestGetLoginURL_ReturnsOAuthURL(t *testing.T) {
	provider := &mockOAuthProvider{
		getLoginURLFn: func(state string) string {
			return "https://accounts.google.com/o/oauth2/auth?state=" + state
		},
	}
	svc := NewService(provider, nil, nil, nil, ServiceConfig{SessionMaxAge: 86400})

	url := svc.GetLoginURL("test-state")

	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	expected := "https://accounts.google.com/o/oauth2/auth?state=test-state"
	if url != expected {
		t.Errorf("GetLoginURL() = %q, want %q", url, expected)
	}
}

func TestHandleCallback_NewUser_CreatesUserAndIdentityAndSession(t *testing.T) {
	ctx := context.Background()

	var createdUser *model.User
	var createdIdentity *model.Identity
	var createdSession *model.Session

	provider := &mockOAuthProvider{
		exchangeCodeFn: func(ctx context.Context, code string) (*OAuthUserInfo, error) {
			return &OAuthUserInfo{
				ProviderUserID: "google-user-123",
				Email:          "test@example.com",
				Name:           "Test User",
				Provider:       "google",
			}, nil
		},
	}

	userRepo := &mockUserRepo{
		createWithIdentityFn: func(ctx context.Context, user *model.User, identity *model.Identity) error {
			createdUser = user
			createdIdentity = identity
			return nil
		},
	}

	identityRepo := &mockIdentityRepo{
		findByProviderFn: func(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
			// ユーザーが見つからない（新規ユーザー）
			return nil, nil
		},
	}

	sessionRepo := &mockSessionRepo{
		createFn: func(ctx context.Context, session *model.Session) error {
			createdSession = session
			return nil
		},
	}

	svc := NewService(provider, userRepo, identityRepo, sessionRepo, ServiceConfig{SessionMaxAge: 86400})

	session, err := svc.HandleCallback(ctx, "auth-code-123")
	if err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	// セッションが返されること
	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if session.UserID == "" {
		t.Error("expected non-empty user ID in session")
	}

	// ユーザーが作成されること
	if createdUser == nil {
		t.Fatal("expected user to be created")
	}
	if createdUser.Email != "test@example.com" {
		t.Errorf("user email = %q, want %q", createdUser.Email, "test@example.com")
	}
	if createdUser.Name != "Test User" {
		t.Errorf("user name = %q, want %q", createdUser.Name, "Test User")
	}

	// identityが作成されること
	if createdIdentity == nil {
		t.Fatal("expected identity to be created")
	}
	if createdIdentity.Provider != "google" {
		t.Errorf("identity provider = %q, want %q", createdIdentity.Provider, "google")
	}
	if createdIdentity.ProviderUserID != "google-user-123" {
		t.Errorf("identity providerUserID = %q, want %q", createdIdentity.ProviderUserID, "google-user-123")
	}

	// セッションが作成されること
	if createdSession == nil {
		t.Fatal("expected session to be created")
	}
	if createdSession.UserID != createdUser.ID {
		t.Errorf("session userID = %q, want %q", createdSession.UserID, createdUser.ID)
	}
	if createdSession.ExpiresAt.Before(time.Now()) {
		t.Error("session should not be expired")
	}
}

func TestHandleCallback_ExistingUser_LogsInAndCreatesSession(t *testing.T) {
	ctx := context.Background()

	existingUserID := "existing-user-id-456"
	var createdSession *model.Session

	provider := &mockOAuthProvider{
		exchangeCodeFn: func(ctx context.Context, code string) (*OAuthUserInfo, error) {
			return &OAuthUserInfo{
				ProviderUserID: "google-user-789",
				Email:          "existing@example.com",
				Name:           "Existing User",
				Provider:       "google",
			}, nil
		},
	}

	userRepo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{
				ID:    existingUserID,
				Email: "existing@example.com",
				Name:  "Existing User",
			}, nil
		},
	}

	identityRepo := &mockIdentityRepo{
		findByProviderFn: func(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
			// 既存ユーザーのidentityが見つかる
			return &model.Identity{
				ID:             "identity-id-1",
				UserID:         existingUserID,
				Provider:       "google",
				ProviderUserID: "google-user-789",
			}, nil
		},
	}

	sessionRepo := &mockSessionRepo{
		createFn: func(ctx context.Context, session *model.Session) error {
			createdSession = session
			return nil
		},
	}

	svc := NewService(provider, userRepo, identityRepo, sessionRepo, ServiceConfig{SessionMaxAge: 86400})

	session, err := svc.HandleCallback(ctx, "auth-code-existing")
	if err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.UserID != existingUserID {
		t.Errorf("session userID = %q, want %q", session.UserID, existingUserID)
	}

	// 既存ユーザーにCreateWithIdentityは呼ばれないこと
	// （mockUserRepoのcreateWithIdentityFnがnilなので、呼ばれたらpanicする）

	// セッションが作成されること
	if createdSession == nil {
		t.Fatal("expected session to be created")
	}
	if createdSession.UserID != existingUserID {
		t.Errorf("session userID = %q, want %q", createdSession.UserID, existingUserID)
	}
}

func TestHandleCallback_OAuthError_ReturnsError(t *testing.T) {
	ctx := context.Background()

	provider := &mockOAuthProvider{
		exchangeCodeFn: func(ctx context.Context, code string) (*OAuthUserInfo, error) {
			return nil, errors.New("oauth exchange failed")
		},
	}

	svc := NewService(provider, nil, nil, nil, ServiceConfig{SessionMaxAge: 86400})

	_, err := svc.HandleCallback(ctx, "bad-code")
	if err == nil {
		t.Fatal("expected error from HandleCallback")
	}
}

func TestHandleCallback_UserCreationError_ReturnsError(t *testing.T) {
	ctx := context.Background()

	provider := &mockOAuthProvider{
		exchangeCodeFn: func(ctx context.Context, code string) (*OAuthUserInfo, error) {
			return &OAuthUserInfo{
				ProviderUserID: "google-user-err",
				Email:          "error@example.com",
				Name:           "Error User",
				Provider:       "google",
			}, nil
		},
	}

	identityRepo := &mockIdentityRepo{
		findByProviderFn: func(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
			return nil, nil // 新規ユーザー
		},
	}

	userRepo := &mockUserRepo{
		createWithIdentityFn: func(ctx context.Context, user *model.User, identity *model.Identity) error {
			return errors.New("db error")
		},
	}

	svc := NewService(provider, userRepo, identityRepo, nil, ServiceConfig{SessionMaxAge: 86400})

	_, err := svc.HandleCallback(ctx, "auth-code-err")
	if err == nil {
		t.Fatal("expected error from HandleCallback")
	}
}

func TestLogout_DeletesSession(t *testing.T) {
	ctx := context.Background()

	var deletedSessionID string

	sessionRepo := &mockSessionRepo{
		deleteByIDFn: func(ctx context.Context, id string) error {
			deletedSessionID = id
			return nil
		},
	}

	svc := NewService(nil, nil, nil, sessionRepo, ServiceConfig{SessionMaxAge: 86400})

	err := svc.Logout(ctx, "session-to-delete")
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if deletedSessionID != "session-to-delete" {
		t.Errorf("deleted session ID = %q, want %q", deletedSessionID, "session-to-delete")
	}
}

func TestLogout_EmptySessionID_ReturnsError(t *testing.T) {
	ctx := context.Background()

	svc := NewService(nil, nil, nil, nil, ServiceConfig{SessionMaxAge: 86400})

	err := svc.Logout(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}
}

func TestGetCurrentUser_ValidSession_ReturnsUser(t *testing.T) {
	ctx := context.Background()

	userID := "user-id-123"

	sessionRepo := &mockSessionRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			return &model.Session{
				ID:        "session-valid",
				UserID:    userID,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil
		},
	}

	userRepo := &mockUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{
				ID:    userID,
				Email: "user@example.com",
				Name:  "Test User",
			}, nil
		},
	}

	svc := NewService(nil, userRepo, nil, sessionRepo, ServiceConfig{SessionMaxAge: 86400})

	user, err := svc.GetCurrentUser(ctx, "session-valid")
	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if user.ID != userID {
		t.Errorf("user ID = %q, want %q", user.ID, userID)
	}
}

func TestGetCurrentUser_ExpiredSession_ReturnsError(t *testing.T) {
	ctx := context.Background()

	sessionRepo := &mockSessionRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			// 期限切れセッション -> リポジトリはnilを返す
			return nil, nil
		},
	}

	svc := NewService(nil, nil, nil, sessionRepo, ServiceConfig{SessionMaxAge: 86400})

	_, err := svc.GetCurrentUser(ctx, "expired-session")
	if err == nil {
		t.Fatal("expected error for expired session")
	}
}

func TestGetCurrentUser_EmptySessionID_ReturnsError(t *testing.T) {
	ctx := context.Background()

	svc := NewService(nil, nil, nil, nil, ServiceConfig{SessionMaxAge: 86400})

	_, err := svc.GetCurrentUser(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}
}
