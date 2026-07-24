package passkey

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// 本 test は RegistrationService の全依存（WebAuthnAdapter / ChallengeStore /
// UserWriter / PasskeyCredentialWriter）を stub 化し、外部ネットワーク非依存で
// 新規登録 / 追加登録の主要ケース（正常系・異常系・境界値）を検証する（NFR 4.1）。
//
// 対応 AC:
//   - 新規登録: Req 1.1 / 1.2 / 1.3 / 1.4 / 1.5 / 1.6 / 1.7, NFR 1.2, NFR 3.1
//   - 追加登録: Req 3.1 / 3.2 / 3.3 / 3.4 / 3.6 / 3.7

// stubWebAuthnAdapter は WebAuthnAdapter interface を差し替えるためのテスト用スタブ。
type stubWebAuthnAdapter struct {
	beginRegistrationFn func(user WebAuthnUser, excludeCredentials [][]byte) (
		[]byte, []byte, []byte, error)
	finishRegistrationFn func(user WebAuthnUser, sessionData []byte, requestBody []byte) (
		*ParsedCredential, error)

	beginRegistrationCalled  int
	finishRegistrationCalled int
	lastExclude              [][]byte
	lastUserID               []byte
}

func (a *stubWebAuthnAdapter) BeginRegistration(user WebAuthnUser, excludeCredentials [][]byte) (
	[]byte, []byte, []byte, error,
) {
	a.beginRegistrationCalled++
	a.lastExclude = excludeCredentials
	a.lastUserID = user.WebAuthnID()
	if a.beginRegistrationFn != nil {
		return a.beginRegistrationFn(user, excludeCredentials)
	}
	return []byte(`{"options":"stub"}`), []byte(`{"session":"stub"}`), []byte("raw-challenge"), nil
}

func (a *stubWebAuthnAdapter) FinishRegistration(user WebAuthnUser, sessionData []byte, requestBody []byte) (
	*ParsedCredential, error,
) {
	a.finishRegistrationCalled++
	a.lastUserID = user.WebAuthnID()
	if a.finishRegistrationFn != nil {
		return a.finishRegistrationFn(user, sessionData, requestBody)
	}
	return &ParsedCredential{
		ID:              []byte("cred-id-default"),
		PublicKey:       []byte("public-key-default"),
		SignCount:       0,
		AttestationType: "none",
	}, nil
}

func (a *stubWebAuthnAdapter) BeginLogin() ([]byte, []byte, []byte, error) {
	return nil, nil, nil, errors.New("BeginLogin not used in registration tests")
}

func (a *stubWebAuthnAdapter) FinishLogin(sessionData []byte, requestBody []byte,
	credentialLookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
) ([]byte, []byte, uint32, error) {
	return nil, nil, 0, errors.New("FinishLogin not used in registration tests")
}

// stubChallengeStoreForRegistration は RegistrationService の challengeStore 依存を
// 差し替えるためのスタブ（本 package 内の challengeStore interface に対応）。
type stubChallengeStoreForRegistration struct {
	issueFn func(ctx context.Context, kind model.PasskeyChallengeKind, userID *string,
		pendingUsername *string, sessionData []byte, rawChallenge []byte) (string, error)
	consumeFn func(ctx context.Context, challengeID string,
		expectedKind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error)

	issueCalled   int
	consumeCalled int
	lastIssueKind model.PasskeyChallengeKind
	lastIssueUser *string
	lastIssuePend *string
}

func (s *stubChallengeStoreForRegistration) Issue(
	ctx context.Context, kind model.PasskeyChallengeKind, userID *string,
	pendingUsername *string, sessionData []byte, rawChallenge []byte,
) (string, error) {
	s.issueCalled++
	s.lastIssueKind = kind
	s.lastIssueUser = userID
	s.lastIssuePend = pendingUsername
	if s.issueFn != nil {
		return s.issueFn(ctx, kind, userID, pendingUsername, sessionData, rawChallenge)
	}
	return "issued-challenge-id", nil
}

func (s *stubChallengeStoreForRegistration) Consume(
	ctx context.Context, challengeID string, expectedKind model.PasskeyChallengeKind,
) (*model.PasskeyChallenge, error) {
	s.consumeCalled++
	if s.consumeFn != nil {
		return s.consumeFn(ctx, challengeID, expectedKind)
	}
	return nil, errors.New("consumeFn not configured")
}

// stubUserWriter は UserWriter interface を差し替えるスタブ。
type stubUserWriter struct {
	findByNormalizedFn func(ctx context.Context, normalized string) (*model.User, error)
	createUserOnlyFn   func(ctx context.Context, u *model.User) error
	findByIDFn         func(ctx context.Context, id string) (*model.User, error)

	createUserOnlyCalled int
	lastCreated          *model.User
}

func (u *stubUserWriter) FindByNormalizedUsername(ctx context.Context, normalized string) (*model.User, error) {
	if u.findByNormalizedFn != nil {
		return u.findByNormalizedFn(ctx, normalized)
	}
	return nil, nil
}

func (u *stubUserWriter) CreateUserOnly(ctx context.Context, user *model.User) error {
	u.createUserOnlyCalled++
	u.lastCreated = user
	if u.createUserOnlyFn != nil {
		return u.createUserOnlyFn(ctx, user)
	}
	return nil
}

func (u *stubUserWriter) FindByID(ctx context.Context, id string) (*model.User, error) {
	if u.findByIDFn != nil {
		return u.findByIDFn(ctx, id)
	}
	return nil, nil
}

// stubCredentialWriter は PasskeyCredentialWriter interface を差し替えるスタブ。
type stubCredentialWriter struct {
	findByCredentialIDFn func(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error)
	listByUserIDFn       func(ctx context.Context, userID string) ([]*model.PasskeyCredential, error)
	createFn             func(ctx context.Context, c *model.PasskeyCredential) error

	createCalled     int
	lastCreatedCred  *model.PasskeyCredential
	listByUserCalled int
}

func (c *stubCredentialWriter) FindByCredentialID(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error) {
	if c.findByCredentialIDFn != nil {
		return c.findByCredentialIDFn(ctx, credentialID)
	}
	return nil, nil
}

func (c *stubCredentialWriter) ListByUserID(ctx context.Context, userID string) ([]*model.PasskeyCredential, error) {
	c.listByUserCalled++
	if c.listByUserIDFn != nil {
		return c.listByUserIDFn(ctx, userID)
	}
	return nil, nil
}

func (c *stubCredentialWriter) Create(ctx context.Context, cred *model.PasskeyCredential) error {
	c.createCalled++
	c.lastCreatedCred = cred
	if c.createFn != nil {
		return c.createFn(ctx, cred)
	}
	return nil
}

// validPKCEChallenge は auth.ValidatePKCES256 の形式（43 文字 base64url）を通過する
// テスト用固定値。
const validPKCEChallenge = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

// newRegistrationServiceFixture は RegistrationService の全依存を stub 化した
// 標準セットを返すヘルパー。テストごとに必要な stub の挙動だけを差し替える。
func newRegistrationServiceFixture(t *testing.T) (
	*RegistrationService,
	*stubWebAuthnAdapter,
	*stubChallengeStoreForRegistration,
	*stubUserWriter,
	*stubCredentialWriter,
) {
	t.Helper()
	adapter := &stubWebAuthnAdapter{}
	challenges := &stubChallengeStoreForRegistration{}
	users := &stubUserWriter{}
	creds := &stubCredentialWriter{}
	now := func() time.Time { return time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC) }
	svc := NewRegistrationService(adapter, challenges, users, creds, now)
	return svc, adapter, challenges, users, creds
}

// ------------------------------------------------------------
// 新規登録 (BeginRegistrationNew / FinishRegistrationNew)
// ------------------------------------------------------------

func TestRegistrationService_BeginRegistrationNew(t *testing.T) {
	ctx := context.Background()

	t.Run("有効な username と PKCE で challenge_id と options を返す", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _ := newRegistrationServiceFixture(t)
		challenges.issueFn = func(ctx context.Context, kind model.PasskeyChallengeKind, userID *string,
			pendingUsername *string, sessionData []byte, rawChallenge []byte,
		) (string, error) {
			return "issued-new-id", nil
		}

		// Act
		challengeID, options, err := svc.BeginRegistrationNew(ctx, "Alice", "", validPKCEChallenge)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if challengeID != "issued-new-id" {
			t.Errorf("challengeID = %q, want issued-new-id", challengeID)
		}
		if len(options) == 0 {
			t.Errorf("options should be non-empty")
		}
		if adapter.beginRegistrationCalled != 1 {
			t.Errorf("BeginRegistration called %d times, want 1", adapter.beginRegistrationCalled)
		}
		if users.createUserOnlyCalled != 0 {
			t.Errorf("CreateUserOnly must not be called on begin, got %d", users.createUserOnlyCalled)
		}
		if challenges.lastIssueKind != model.PasskeyChallengeKindRegistrationNew {
			t.Errorf("Issue kind = %q, want registration_new", challenges.lastIssueKind)
		}
		if challenges.lastIssueUser == nil || *challenges.lastIssueUser == "" {
			t.Errorf("Issue userID must be a non-empty pending UUID, got %v", challenges.lastIssueUser)
		}
		if challenges.lastIssuePend == nil || *challenges.lastIssuePend != "alice" {
			t.Errorf("Issue pendingUsername = %v, want &alice", challenges.lastIssuePend)
		}
		// 仮 UUID の byte 列と WebAuthnUser.WebAuthnID が一致することを検証
		// （task 5 credentialLookup 整合性の担保）。
		if string(adapter.lastUserID) != *challenges.lastIssueUser {
			t.Errorf("WebAuthnID bytes = %q, want %q (pending UUID)",
				string(adapter.lastUserID), *challenges.lastIssueUser)
		}
	})

	t.Run("username 形式不正 (空) は ErrInvalidUsername を返し ceremony を起動しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _ := newRegistrationServiceFixture(t)

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "", "", validPKCEChallenge)

		// Assert
		if !errors.Is(err, ErrInvalidUsername) {
			t.Fatalf("expected ErrInvalidUsername, got %v", err)
		}
		if adapter.beginRegistrationCalled != 0 {
			t.Errorf("BeginRegistration must not be called on invalid username")
		}
		if challenges.issueCalled != 0 {
			t.Errorf("Issue must not be called on invalid username")
		}
		if users.createUserOnlyCalled != 0 {
			t.Errorf("CreateUserOnly must not be called on invalid username")
		}
	})

	t.Run("username 形式不正 (許容外文字) は ErrInvalidUsername を返す", func(t *testing.T) {
		// Arrange
		svc, _, _, _, _ := newRegistrationServiceFixture(t)

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "invalid space", "", validPKCEChallenge)

		// Assert
		if !errors.Is(err, ErrInvalidUsername) {
			t.Fatalf("expected ErrInvalidUsername, got %v", err)
		}
	})

	t.Run("username 重複は ErrUsernameTaken を返し WebAuthn を起動しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _ := newRegistrationServiceFixture(t)
		users.findByNormalizedFn = func(ctx context.Context, normalized string) (*model.User, error) {
			// 既存 user を返す = 重複
			return &model.User{ID: "existing-user-id", UsernameNormalized: normalized}, nil
		}

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "Alice", "", validPKCEChallenge)

		// Assert
		if !errors.Is(err, ErrUsernameTaken) {
			t.Fatalf("expected ErrUsernameTaken, got %v", err)
		}
		if adapter.beginRegistrationCalled != 0 {
			t.Errorf("BeginRegistration must not be called on username duplicate")
		}
		if challenges.issueCalled != 0 {
			t.Errorf("Issue must not be called on username duplicate")
		}
	})

	t.Run("PKCE 形式不正は ErrRegistrationFailed を返し ceremony を起動しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, _, _ := newRegistrationServiceFixture(t)

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "alice", "", "invalid-pkce")

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if adapter.beginRegistrationCalled != 0 {
			t.Errorf("BeginRegistration must not be called on invalid PKCE")
		}
		if challenges.issueCalled != 0 {
			t.Errorf("Issue must not be called on invalid PKCE")
		}
	})

	t.Run("email 未指定 (empty) でも begin は成功する (Req 1.6 の前提)", func(t *testing.T) {
		// Arrange
		svc, _, _, _, _ := newRegistrationServiceFixture(t)

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "alice", "", validPKCEChallenge)

		// Assert
		if err != nil {
			t.Fatalf("expected no error for empty email, got %v", err)
		}
	})

	t.Run("FindByNormalizedUsername infra エラーは wrap して返す", func(t *testing.T) {
		// Arrange
		svc, _, _, users, _ := newRegistrationServiceFixture(t)
		infraErr := errors.New("db down")
		users.findByNormalizedFn = func(ctx context.Context, normalized string) (*model.User, error) {
			return nil, infraErr
		}

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "alice", "", validPKCEChallenge)

		// Assert
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, infraErr) {
			t.Errorf("expected wrapped %v, got %v", infraErr, err)
		}
		if errors.Is(err, ErrRegistrationFailed) || errors.Is(err, ErrUsernameTaken) {
			t.Errorf("infra error must not be re-mapped to uniform sentinel: %v", err)
		}
	})
}

func TestRegistrationService_FinishRegistrationNew(t *testing.T) {
	ctx := context.Background()
	pendingUserID := "pending-uuid-123"
	pendingUsername := "alice"

	newConsumedFn := func() func(ctx context.Context, challengeID string,
		expectedKind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
		return func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			uid := pendingUserID
			pend := pendingUsername
			return &model.PasskeyChallenge{
				ID:              challengeID,
				Kind:            expectedKind,
				UserID:          &uid,
				PendingUsername: &pend,
				SessionData:     []byte(`{"session":"stub"}`),
			}, nil
		}
	}

	t.Run("成功: user 行と credential 行を作成し userID を返す (email は空でも作成 / Req 1.6)", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()

		// Act
		userID, err := svc.FinishRegistrationNew(ctx, "challenge-id", []byte("attestation-body"))

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if userID != pendingUserID {
			t.Errorf("userID = %q, want %q (pending UUID promoted to users.id)", userID, pendingUserID)
		}
		if users.createUserOnlyCalled != 1 {
			t.Errorf("CreateUserOnly called %d times, want 1", users.createUserOnlyCalled)
		}
		if users.lastCreated == nil {
			t.Fatalf("lastCreated user is nil")
		}
		if users.lastCreated.ID != pendingUserID {
			t.Errorf("created user.ID = %q, want %q (仮 UUID がそのまま users.id)", users.lastCreated.ID, pendingUserID)
		}
		if users.lastCreated.Email != "" {
			t.Errorf("created user.Email = %q, want '' (Req 1.6: メール未指定許容)", users.lastCreated.Email)
		}
		if users.lastCreated.UsernameNormalized != pendingUsername {
			t.Errorf("created user.UsernameNormalized = %q, want %q",
				users.lastCreated.UsernameNormalized, pendingUsername)
		}
		if creds.createCalled != 1 {
			t.Errorf("credential.Create called %d times, want 1", creds.createCalled)
		}
		if creds.lastCreatedCred == nil || creds.lastCreatedCred.UserID != pendingUserID {
			t.Errorf("credential.UserID mismatch: %+v", creds.lastCreatedCred)
		}
		if string(adapter.lastUserID) != pendingUserID {
			t.Errorf("FinishRegistration user WebAuthnID = %q, want %q",
				string(adapter.lastUserID), pendingUserID)
		}
	})

	t.Run("challenge 期限切れ (Consume が ErrChallengeNotUsable) は ErrRegistrationFailed に正規化", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			return nil, ErrChallengeNotUsable
		}

		// Act
		_, err := svc.FinishRegistrationNew(ctx, "expired-id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if users.createUserOnlyCalled != 0 || creds.createCalled != 0 {
			t.Errorf("no writes should occur on expired challenge")
		}
	})

	t.Run("attestation 検証失敗 (adapter が ErrRegistrationFailed) は そのまま返し user/credential を作らない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		adapter.finishRegistrationFn = func(user WebAuthnUser, sessionData []byte, requestBody []byte) (
			*ParsedCredential, error,
		) {
			return nil, ErrRegistrationFailed
		}

		// Act
		_, err := svc.FinishRegistrationNew(ctx, "id", []byte("bad-body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if users.createUserOnlyCalled != 0 {
			t.Errorf("CreateUserOnly must not be called after attestation failure")
		}
		if creds.createCalled != 0 {
			t.Errorf("credential.Create must not be called after attestation failure")
		}
	})

	t.Run("CreateUserOnly の UNIQUE 衝突 (race) は ErrRegistrationFailed に正規化 (Req 1.7)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		users.createUserOnlyFn = func(ctx context.Context, u *model.User) error {
			return repository.ErrUsernameTaken
		}

		// Act
		_, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed (uniform), got %v", err)
		}
		if creds.createCalled != 0 {
			t.Errorf("credential.Create must not be called after user race conflict")
		}
	})

	t.Run("credential 重複 (ErrCredentialAlreadyRegistered) は ErrRegistrationFailed に正規化 (Req 1.7)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		creds.createFn = func(ctx context.Context, c *model.PasskeyCredential) error {
			return repository.ErrCredentialAlreadyRegistered
		}

		// Act
		_, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
	})

	t.Run("challenge に UserID / PendingUsername が nil の場合は ErrRegistrationFailed", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, _ := newRegistrationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			return &model.PasskeyChallenge{
				ID:          challengeID,
				Kind:        expectedKind,
				SessionData: []byte(`{}`),
				// UserID / PendingUsername = nil (契約違反シミュレート)
			}, nil
		}

		// Act
		_, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed on malformed challenge, got %v", err)
		}
	})
}

// ------------------------------------------------------------
// 追加登録 (BeginAddCredential / FinishAddCredential)
// ------------------------------------------------------------

func TestRegistrationService_BeginAddCredential(t *testing.T) {
	ctx := context.Background()
	authedUserID := "authed-user-uuid"

	t.Run("既存 credential を excludeCredentials として WebAuthnAdapter に渡す", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds := newRegistrationServiceFixture(t)
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Username: "bob"}, nil
		}
		creds.listByUserIDFn = func(ctx context.Context, userID string) ([]*model.PasskeyCredential, error) {
			return []*model.PasskeyCredential{
				{ID: "cred-1", UserID: userID, CredentialID: []byte("existing-cred-a")},
				{ID: "cred-2", UserID: userID, CredentialID: []byte("existing-cred-b")},
			}, nil
		}

		// Act
		challengeID, options, err := svc.BeginAddCredential(ctx, authedUserID)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if challengeID == "" || len(options) == 0 {
			t.Errorf("expected non-empty challengeID and options")
		}
		if adapter.beginRegistrationCalled != 1 {
			t.Errorf("BeginRegistration called %d times, want 1", adapter.beginRegistrationCalled)
		}
		if len(adapter.lastExclude) != 2 {
			t.Fatalf("excludeCredentials len = %d, want 2", len(adapter.lastExclude))
		}
		if string(adapter.lastExclude[0]) != "existing-cred-a" ||
			string(adapter.lastExclude[1]) != "existing-cred-b" {
			t.Errorf("excludeCredentials mismatch: %v", adapter.lastExclude)
		}
		if challenges.lastIssueKind != model.PasskeyChallengeKindRegistrationAdd {
			t.Errorf("Issue kind = %q, want registration_add", challenges.lastIssueKind)
		}
		if challenges.lastIssueUser == nil || *challenges.lastIssueUser != authedUserID {
			t.Errorf("Issue userID = %v, want &%q", challenges.lastIssueUser, authedUserID)
		}
		// WebAuthnID = []byte(users.id) 整合性
		if string(adapter.lastUserID) != authedUserID {
			t.Errorf("WebAuthnID = %q, want %q (users.id bytes)", string(adapter.lastUserID), authedUserID)
		}
	})

	t.Run("user 未存在 (FindByID が nil) は ErrRegistrationFailed", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _ := newRegistrationServiceFixture(t)
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return nil, nil
		}

		// Act
		_, _, err := svc.BeginAddCredential(ctx, authedUserID)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if adapter.beginRegistrationCalled != 0 || challenges.issueCalled != 0 {
			t.Errorf("no downstream calls should happen when user not found")
		}
	})
}

func TestRegistrationService_FinishAddCredential(t *testing.T) {
	ctx := context.Background()
	authedUserID := "authed-user-uuid"
	otherUserID := "other-user-uuid"

	newAddConsumedFn := func(userIDInChallenge string) func(ctx context.Context, challengeID string,
		expectedKind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
		return func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			uid := userIDInChallenge
			return &model.PasskeyChallenge{
				ID:          challengeID,
				Kind:        expectedKind,
				UserID:      &uid,
				SessionData: []byte(`{}`),
			}, nil
		}
	}

	t.Run("成功: 別 credential を追加登録し credential 行を作成 (同 user 複数登録可 / Req 3.4)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newAddConsumedFn(authedUserID)
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Username: "bob"}, nil
		}
		creds.listByUserIDFn = func(ctx context.Context, userID string) ([]*model.PasskeyCredential, error) {
			return []*model.PasskeyCredential{
				{ID: "cred-1", UserID: userID, CredentialID: []byte("existing-cred-a")},
			}, nil
		}
		// FindByCredentialID は他 user 既登録なしを返す
		creds.findByCredentialIDFn = func(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error) {
			return nil, nil
		}

		// Act
		err := svc.FinishAddCredential(ctx, authedUserID, "challenge-id", []byte("body"))

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds.createCalled != 1 {
			t.Errorf("credential.Create called %d times, want 1", creds.createCalled)
		}
		if creds.lastCreatedCred == nil || creds.lastCreatedCred.UserID != authedUserID {
			t.Errorf("credential.UserID mismatch: %+v", creds.lastCreatedCred)
		}
	})

	t.Run("challenge userID と authenticatedUserID の不一致は ErrRegistrationFailed", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds := newRegistrationServiceFixture(t)
		// challenge には otherUserID が入っているが、context は authedUserID を提示
		challenges.consumeFn = newAddConsumedFn(otherUserID)
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		}

		// Act
		err := svc.FinishAddCredential(ctx, authedUserID, "challenge-id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if adapter.finishRegistrationCalled != 0 {
			t.Errorf("FinishRegistration must not be called on userID mismatch")
		}
		if creds.createCalled != 0 {
			t.Errorf("credential.Create must not be called on userID mismatch")
		}
	})

	t.Run("別 user に既登録の credential 提示は ErrRegistrationFailed に正規化 (Req 3.6)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newAddConsumedFn(authedUserID)
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		}
		// 既登録: 同じ credential ID が別 user に紐付いている
		creds.findByCredentialIDFn = func(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error) {
			return &model.PasskeyCredential{
				UserID:       otherUserID,
				CredentialID: credentialID,
			}, nil
		}

		// Act
		err := svc.FinishAddCredential(ctx, authedUserID, "challenge-id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if creds.createCalled != 0 {
			t.Errorf("credential.Create must not be called when another user owns the credential")
		}
	})

	t.Run("challenge 期限切れは ErrRegistrationFailed に正規化", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			return nil, ErrChallengeNotUsable
		}

		// Act
		err := svc.FinishAddCredential(ctx, authedUserID, "expired-id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		// downstream に届いてはいけない
		if users.createUserOnlyCalled != 0 || creds.createCalled != 0 ||
			adapter.finishRegistrationCalled != 0 {
			t.Errorf("no downstream calls should occur on expired challenge")
		}
	})

	t.Run("credential.Create の UNIQUE 衝突 race は ErrRegistrationFailed に正規化", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds := newRegistrationServiceFixture(t)
		challenges.consumeFn = newAddConsumedFn(authedUserID)
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		}
		creds.createFn = func(ctx context.Context, c *model.PasskeyCredential) error {
			return repository.ErrCredentialAlreadyRegistered
		}

		// Act
		err := svc.FinishAddCredential(ctx, authedUserID, "challenge-id", []byte("body"))

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
	})
}
