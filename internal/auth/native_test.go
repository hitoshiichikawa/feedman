package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// mockAuthCodeCreator は AuthCodeCreator のテスト用モック。
type mockAuthCodeCreator struct {
	createFn func(ctx context.Context, code *model.AuthCode) error
	created  *model.AuthCode
}

func (m *mockAuthCodeCreator) Create(ctx context.Context, code *model.AuthCode) error {
	m.created = code
	if m.createFn != nil {
		return m.createFn(ctx, code)
	}
	return nil
}

var _ AuthCodeCreator = (*mockAuthCodeCreator)(nil)

// nativeTestProvider は ExchangeCode が固定ユーザー情報を返す mock provider を生成する。
func nativeTestProvider() *mockOAuthProvider {
	return &mockOAuthProvider{
		exchangeCodeFn: func(_ context.Context, _ string) (*OAuthUserInfo, error) {
			return &OAuthUserInfo{
				ProviderUserID: "google-uid-1",
				Email:          "user@example.com",
				Name:           "Test User",
				Provider:       "google",
			}, nil
		},
	}
}

// TestHandleNativeCallback_ExistingUser_IssuesHashedAuthCode は既存ユーザーの native callback で
// auth_code が hash 保存され、平文が返ることを検証する（Req 2.2, 2.4, 2.5 / NFR 1.1）。
func TestHandleNativeCallback_ExistingUser_IssuesHashedAuthCode(t *testing.T) {
	// Arrange: 既存 identity が見つかるユーザーと auth_code 保存先モック
	identRepo := &mockIdentityRepo{
		findByProviderFn: func(_ context.Context, provider, providerUserID string) (*model.Identity, error) {
			return &model.Identity{ID: "ident-1", UserID: "user-1", Provider: provider, ProviderUserID: providerUserID}, nil
		},
	}
	codeRepo := &mockAuthCodeCreator{}
	svc := NewService(nativeTestProvider(), &mockUserRepo{}, identRepo, &mockSessionRepo{}, codeRepo, ServiceConfig{SessionMaxAge: 86400})

	challenge := strings.Repeat("c", 43)
	before := time.Now()

	// Act
	plain, err := svc.HandleNativeCallback(context.Background(), "oauth-code", challenge)

	// Assert
	if err != nil {
		t.Fatalf("HandleNativeCallback returned error: %v", err)
	}
	if plain == "" {
		t.Fatal("plain auth code should not be empty")
	}
	if codeRepo.created == nil {
		t.Fatal("auth code should be stored")
	}
	if codeRepo.created.CodeHash != HashNativeSecret(plain) {
		t.Errorf("stored CodeHash = %q, want HashNativeSecret(plain) = %q", codeRepo.created.CodeHash, HashNativeSecret(plain))
	}
	if codeRepo.created.CodeHash == plain {
		t.Error("stored CodeHash must not equal plain code (must be hashed)")
	}
	if codeRepo.created.UserID != "user-1" {
		t.Errorf("stored UserID = %q, want %q", codeRepo.created.UserID, "user-1")
	}
	if codeRepo.created.PKCEChallenge != challenge {
		t.Errorf("stored PKCEChallenge = %q, want %q", codeRepo.created.PKCEChallenge, challenge)
	}
	if codeRepo.created.Used {
		t.Error("stored Used should be false")
	}
	// ExpiresAt ≈ now + 60s（テスト実行時間の揺らぎとして ±5 秒許容）
	wantExpiry := before.Add(NativeAuthCodeTTL)
	if codeRepo.created.ExpiresAt.Before(wantExpiry.Add(-5*time.Second)) ||
		codeRepo.created.ExpiresAt.After(wantExpiry.Add(5*time.Second)) {
		t.Errorf("stored ExpiresAt = %v, want ≈ %v (now+60s)", codeRepo.created.ExpiresAt, wantExpiry)
	}
}

// TestHandleNativeCallback_NewUser_CreatesUserAndIssuesCode は未登録ユーザーの native callback で
// users / identities が自動作成され auth_code が発行されることを検証する（Req 2.4）。
func TestHandleNativeCallback_NewUser_CreatesUserAndIssuesCode(t *testing.T) {
	// Arrange: identity が見つからず、CreateWithIdentity が成功するモック
	var createdUser *model.User
	userRepo := &mockUserRepo{
		createWithIdentityFn: func(_ context.Context, user *model.User, _ *model.Identity) error {
			createdUser = user
			return nil
		},
	}
	codeRepo := &mockAuthCodeCreator{}
	svc := NewService(nativeTestProvider(), userRepo, &mockIdentityRepo{}, &mockSessionRepo{}, codeRepo, ServiceConfig{SessionMaxAge: 86400})

	// Act
	plain, err := svc.HandleNativeCallback(context.Background(), "oauth-code", strings.Repeat("c", 43))

	// Assert
	if err != nil {
		t.Fatalf("HandleNativeCallback returned error: %v", err)
	}
	if plain == "" {
		t.Fatal("plain auth code should not be empty")
	}
	if createdUser == nil {
		t.Fatal("new user should be created")
	}
	if codeRepo.created == nil || codeRepo.created.UserID != createdUser.ID {
		t.Errorf("auth code should be bound to the newly created user")
	}
}

// TestHandleNativeCallback_DoesNotCreateSession は native callback が Web 用セッションを
// 作成しないことを検証する（Req 2.3）。
func TestHandleNativeCallback_DoesNotCreateSession(t *testing.T) {
	// Arrange: セッション作成を検知するモック
	sessionCreated := false
	sessionRepo := &mockSessionRepo{
		createFn: func(_ context.Context, _ *model.Session) error {
			sessionCreated = true
			return nil
		},
	}
	identRepo := &mockIdentityRepo{
		findByProviderFn: func(_ context.Context, provider, providerUserID string) (*model.Identity, error) {
			return &model.Identity{ID: "ident-1", UserID: "user-1", Provider: provider, ProviderUserID: providerUserID}, nil
		},
	}
	svc := NewService(nativeTestProvider(), &mockUserRepo{}, identRepo, sessionRepo, &mockAuthCodeCreator{}, ServiceConfig{SessionMaxAge: 86400})

	// Act
	if _, err := svc.HandleNativeCallback(context.Background(), "oauth-code", strings.Repeat("c", 43)); err != nil {
		t.Fatalf("HandleNativeCallback returned error: %v", err)
	}

	// Assert
	if sessionCreated {
		t.Error("native callback must not create a web session")
	}
}

// TestHandleNativeCallback_OAuthExchangeFails_ReturnsError は OAuth 交換失敗時にエラーを返し
// auth_code を保存しないことを検証する（Req 3.4）。
func TestHandleNativeCallback_OAuthExchangeFails_ReturnsError(t *testing.T) {
	// Arrange
	provider := &mockOAuthProvider{
		exchangeCodeFn: func(_ context.Context, _ string) (*OAuthUserInfo, error) {
			return nil, errors.New("exchange failed")
		},
	}
	codeRepo := &mockAuthCodeCreator{}
	svc := NewService(provider, &mockUserRepo{}, &mockIdentityRepo{}, &mockSessionRepo{}, codeRepo, ServiceConfig{SessionMaxAge: 86400})

	// Act
	_, err := svc.HandleNativeCallback(context.Background(), "bad-code", strings.Repeat("c", 43))

	// Assert
	if err == nil {
		t.Fatal("expected error when oauth exchange fails")
	}
	if codeRepo.created != nil {
		t.Error("auth code must not be stored when oauth exchange fails")
	}
}

// TestHandleNativeCallback_StoreFails_ReturnsErrorWithoutPlaintext は auth_code 保存失敗時に
// エラーを返し、エラーメッセージに平文 code を含めないことを検証する（Req 2.5, 3.4 / NFR 1.3）。
func TestHandleNativeCallback_StoreFails_ReturnsErrorWithoutPlaintext(t *testing.T) {
	// Arrange
	identRepo := &mockIdentityRepo{
		findByProviderFn: func(_ context.Context, provider, providerUserID string) (*model.Identity, error) {
			return &model.Identity{ID: "ident-1", UserID: "user-1", Provider: provider, ProviderUserID: providerUserID}, nil
		},
	}
	var hashedAtStore string
	codeRepo := &mockAuthCodeCreator{
		createFn: func(_ context.Context, code *model.AuthCode) error {
			hashedAtStore = code.CodeHash
			return errors.New("db unavailable")
		},
	}
	svc := NewService(nativeTestProvider(), &mockUserRepo{}, identRepo, &mockSessionRepo{}, codeRepo, ServiceConfig{SessionMaxAge: 86400})

	// Act
	plain, err := svc.HandleNativeCallback(context.Background(), "oauth-code", strings.Repeat("c", 43))

	// Assert
	if err == nil {
		t.Fatal("expected error when auth code store fails")
	}
	if plain != "" {
		t.Error("plain auth code must be empty on failure")
	}
	// 保存しようとした hash からは平文を逆引きできないが、メッセージ自体への混入も防ぐ。
	if hashedAtStore != "" && strings.Contains(err.Error(), hashedAtStore) {
		t.Errorf("error message must not contain code hash: %v", err)
	}
}

// TestHashNativeSecret_IsDeterministicSHA256Hex は HashNativeSecret が決定論的な
// SHA-256 hex (lowercase 64 文字) を返すことを検証する。
func TestHashNativeSecret_IsDeterministicSHA256Hex(t *testing.T) {
	h1 := HashNativeSecret("secret-value")
	h2 := HashNativeSecret("secret-value")
	if h1 != h2 {
		t.Errorf("hash must be deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64 (sha256 hex)", len(h1))
	}
	if h1 != strings.ToLower(h1) {
		t.Errorf("hash must be lowercase hex: %q", h1)
	}
	if HashNativeSecret("other-value") == h1 {
		t.Error("different inputs must produce different hashes")
	}
}
