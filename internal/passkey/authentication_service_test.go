package passkey

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/hitoshi/feedman/internal/model"
)

// 本 test は AuthenticationService の全依存（WebAuthnAdapter / ChallengeStore /
// PasskeyCredentialReader / UserReader / auth.AuthCodeCreator）を stub 化し、外部
// ネットワーク非依存で認証フロー（正常系・異常系・境界値）を検証する（NFR 4.1）。
//
// 対応 AC:
//   - 認証: Req 2.1 / 2.2 / 2.3 / 2.4 / 2.5 / 2.6
//   - Counter 後退: NFR 1.4
//   - ログ衛生: NFR 1.2
//   - 既存 auth_code 発行契約への合流（GenerateAuthCode / HashNativeSecret /
//     NativeAuthCodeTTL の再利用）: NFR 2.1

// ------------------------------------------------------------
// stubs（本 test file 専用。registration_service_test.go の stub とは
// 命名を分離して混在を避ける）
// ------------------------------------------------------------

// stubAuthnAdapter は WebAuthnAdapter interface を差し替えるテスト用スタブ。
// BeginRegistration / FinishRegistration は authentication テストで使わないため
// error を返す no-op として実装する。
type stubAuthnAdapter struct {
	beginLoginFn func() ([]byte, []byte, []byte, error)
	// finishLoginFn は library の挙動（lookup 呼び出し + 成否 + updatedSignCount +
	// updatedBackupState）を模擬する。test 側で lookup を呼びたい場合はここで
	// credentialLookup(...) を実行する。updatedBackupState は Issue #234 task 3 で追加した
	// 4 番目の戻り値（Req 4.2 / Flags.BackupState 最新化）。
	finishLoginFn func(sessionData, requestBody []byte,
		lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
	) ([]byte, []byte, uint32, bool, error)

	beginLoginCalled  int
	finishLoginCalled int
	lastSessionData   []byte
}

func (a *stubAuthnAdapter) BeginRegistration(user WebAuthnUser, excludeCredentials [][]byte) (
	[]byte, []byte, []byte, error,
) {
	return nil, nil, nil, errors.New("BeginRegistration not used in authentication tests")
}

func (a *stubAuthnAdapter) FinishRegistration(user WebAuthnUser, sessionData []byte, requestBody []byte) (
	*ParsedCredential, error,
) {
	return nil, errors.New("FinishRegistration not used in authentication tests")
}

func (a *stubAuthnAdapter) BeginLogin() ([]byte, []byte, []byte, error) {
	a.beginLoginCalled++
	if a.beginLoginFn != nil {
		return a.beginLoginFn()
	}
	return []byte(`{"login":"opts"}`), []byte(`{"webauthn":"session"}`), []byte("raw-challenge"), nil
}

func (a *stubAuthnAdapter) FinishLogin(sessionData []byte, requestBody []byte,
	lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
) ([]byte, []byte, uint32, bool, error) {
	a.finishLoginCalled++
	a.lastSessionData = sessionData
	if a.finishLoginFn != nil {
		return a.finishLoginFn(sessionData, requestBody, lookup)
	}
	return nil, nil, 0, false, errors.New("finishLoginFn not configured")
}

// stubChallengeStoreForAuthn は challengeStore interface（本 package 内 unexported）を
// 差し替えるスタブ。デフォルトでは in-memory map に Issue の内容を保存し Consume で
// そのまま返すため、Begin → Finish の E2E ペアテストが素直に組める。
type stubChallengeStoreForAuthn struct {
	issueFn func(ctx context.Context, kind model.PasskeyChallengeKind, userID *string,
		pendingUsername *string, sessionData []byte, rawChallenge []byte) (string, error)
	consumeFn func(ctx context.Context, challengeID string,
		expectedKind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error)

	nextID string // default 発行時に用いる ID（未設定なら "issued-authn-id"）
	stored map[string]*model.PasskeyChallenge

	issueCalled      int
	consumeCalled    int
	lastIssueKind    model.PasskeyChallengeKind
	lastIssueUser    *string
	lastIssuePending *string
	lastIssueSession []byte
	lastConsumeKind  model.PasskeyChallengeKind
	lastConsumedID   string
	lastConsumedChID string
}

func (s *stubChallengeStoreForAuthn) Issue(
	ctx context.Context, kind model.PasskeyChallengeKind, userID *string,
	pendingUsername *string, sessionData []byte, rawChallenge []byte,
) (string, error) {
	s.issueCalled++
	s.lastIssueKind = kind
	s.lastIssueUser = userID
	s.lastIssuePending = pendingUsername
	s.lastIssueSession = sessionData
	if s.issueFn != nil {
		return s.issueFn(ctx, kind, userID, pendingUsername, sessionData, rawChallenge)
	}
	id := s.nextID
	if id == "" {
		id = "issued-authn-id"
	}
	if s.stored == nil {
		s.stored = make(map[string]*model.PasskeyChallenge)
	}
	s.stored[id] = &model.PasskeyChallenge{
		ID:          id,
		Kind:        kind,
		UserID:      userID,
		SessionData: sessionData,
	}
	return id, nil
}

func (s *stubChallengeStoreForAuthn) Consume(
	ctx context.Context, challengeID string, expectedKind model.PasskeyChallengeKind,
) (*model.PasskeyChallenge, error) {
	s.consumeCalled++
	s.lastConsumeKind = expectedKind
	s.lastConsumedChID = challengeID
	if s.consumeFn != nil {
		return s.consumeFn(ctx, challengeID, expectedKind)
	}
	if s.stored == nil {
		return nil, ErrChallengeNotUsable
	}
	ch, ok := s.stored[challengeID]
	if !ok {
		return nil, ErrChallengeNotUsable
	}
	if ch.Kind != expectedKind {
		return nil, ErrChallengeNotUsable
	}
	s.lastConsumedID = ch.ID
	return ch, nil
}

// stubCredentialReader は PasskeyCredentialReader interface を差し替えるスタブ。
type stubCredentialReader struct {
	findByCredentialIDFn        func(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error)
	updateAuthenticationStateFn func(ctx context.Context, id string, signCount uint32, backupState bool, lastUsedAt time.Time) error

	findCalled             int
	updateCalled           int
	lastUpdatedID          string
	lastUpdatedSignCount   uint32
	lastUpdatedBackupState bool
	lastUpdatedLastUsed    time.Time
}

func (r *stubCredentialReader) FindByCredentialID(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error) {
	r.findCalled++
	if r.findByCredentialIDFn != nil {
		return r.findByCredentialIDFn(ctx, credentialID)
	}
	return nil, nil
}

func (r *stubCredentialReader) UpdateAuthenticationState(
	ctx context.Context, id string, signCount uint32, backupState bool, lastUsedAt time.Time,
) error {
	r.updateCalled++
	r.lastUpdatedID = id
	r.lastUpdatedSignCount = signCount
	r.lastUpdatedBackupState = backupState
	r.lastUpdatedLastUsed = lastUsedAt
	if r.updateAuthenticationStateFn != nil {
		return r.updateAuthenticationStateFn(ctx, id, signCount, backupState, lastUsedAt)
	}
	return nil
}

// stubUserReader は UserReader interface を差し替えるスタブ。
type stubUserReader struct {
	findByIDFn func(ctx context.Context, id string) (*model.User, error)

	findCalled int
}

func (r *stubUserReader) FindByID(ctx context.Context, id string) (*model.User, error) {
	r.findCalled++
	if r.findByIDFn != nil {
		return r.findByIDFn(ctx, id)
	}
	return nil, nil
}

// stubAuthCodeCreator は auth.AuthCodeCreator interface を差し替えるスタブ。
type stubAuthCodeCreator struct {
	createFn func(ctx context.Context, code *model.AuthCode) error

	createCalled int
	created      *model.AuthCode
}

func (c *stubAuthCodeCreator) Create(ctx context.Context, code *model.AuthCode) error {
	c.createCalled++
	c.created = code
	if c.createFn != nil {
		return c.createFn(ctx, code)
	}
	return nil
}

// ------------------------------------------------------------
// fixture
// ------------------------------------------------------------

// authnFixedNow はテスト用の決定論的な現在時刻。
// challenge_store_test.go の `fixedNow(t time.Time) func() time.Time` ヘルパー関数と
// 名前衝突しないよう prefix を付ける。
var authnFixedNow = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

// newAuthenticationServiceFixture は AuthenticationService の全依存を stub 化した
// 標準セットを返すヘルパー。テストごとに必要な stub の挙動だけを差し替える。
func newAuthenticationServiceFixture(t *testing.T) (
	*AuthenticationService,
	*stubAuthnAdapter,
	*stubChallengeStoreForAuthn,
	*stubCredentialReader,
	*stubUserReader,
	*stubAuthCodeCreator,
) {
	t.Helper()
	adapter := &stubAuthnAdapter{}
	challenges := &stubChallengeStoreForAuthn{}
	creds := &stubCredentialReader{}
	users := &stubUserReader{}
	codes := &stubAuthCodeCreator{}
	now := func() time.Time { return authnFixedNow }
	svc := NewAuthenticationService(adapter, challenges, creds, users, codes, now)
	return svc, adapter, challenges, creds, users, codes
}

// ------------------------------------------------------------
// BeginAuthentication
// ------------------------------------------------------------

func TestAuthenticationService_BeginAuthentication(t *testing.T) {
	ctx := context.Background()

	t.Run("有効な PKCE で challenge_id と options を返す (kind=authentication, userID=nil)", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, _, _, _ := newAuthenticationServiceFixture(t)

		// Act
		challengeID, options, err := svc.BeginAuthentication(ctx, validPKCEChallenge)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if challengeID == "" {
			t.Errorf("challengeID must be non-empty")
		}
		if len(options) == 0 {
			t.Errorf("options must be non-empty")
		}
		if adapter.beginLoginCalled != 1 {
			t.Errorf("BeginLogin called %d times, want 1", adapter.beginLoginCalled)
		}
		if challenges.issueCalled != 1 {
			t.Errorf("Issue called %d times, want 1", challenges.issueCalled)
		}
		if challenges.lastIssueKind != model.PasskeyChallengeKindAuthentication {
			t.Errorf("Issue kind = %q, want authentication", challenges.lastIssueKind)
		}
		if challenges.lastIssueUser != nil {
			t.Errorf("Issue userID must be nil for authentication, got %v", *challenges.lastIssueUser)
		}
		if challenges.lastIssuePending != nil {
			t.Errorf("Issue pendingUsername must be nil for authentication, got %v", *challenges.lastIssuePending)
		}
	})

	t.Run("session_data (envelope) に codeChallenge が含まれる (PKCE 継承経路)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, _, _ := newAuthenticationServiceFixture(t)

		// Act
		_, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Assert
		var envelope authnSession
		if err := json.Unmarshal(challenges.lastIssueSession, &envelope); err != nil {
			t.Fatalf("session envelope should be JSON, unmarshal failed: %v", err)
		}
		if envelope.CodeChallenge != validPKCEChallenge {
			t.Errorf("envelope.CodeChallenge = %q, want %q", envelope.CodeChallenge, validPKCEChallenge)
		}
		if len(envelope.WebAuthnSession) == 0 {
			t.Errorf("envelope.WebAuthnSession must be non-empty (adapter session bytes preserved)")
		}
	})

	t.Run("PKCE 形式不正は ErrAuthenticationFailed を返し ceremony を起動しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, _, _, _ := newAuthenticationServiceFixture(t)

		// Act
		_, _, err := svc.BeginAuthentication(ctx, "invalid-pkce")

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
		}
		if adapter.beginLoginCalled != 0 {
			t.Errorf("BeginLogin must not be called on invalid PKCE")
		}
		if challenges.issueCalled != 0 {
			t.Errorf("Issue must not be called on invalid PKCE")
		}
	})

	t.Run("PKCE 空文字も ErrAuthenticationFailed", func(t *testing.T) {
		// Arrange
		svc, _, _, _, _, _ := newAuthenticationServiceFixture(t)

		// Act
		_, _, err := svc.BeginAuthentication(ctx, "")

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed on empty PKCE, got %v", err)
		}
	})

	t.Run("adapter.BeginLogin の infra エラーは wrap して伝播 (uniform 化しない)", func(t *testing.T) {
		// Arrange
		svc, adapter, _, _, _, _ := newAuthenticationServiceFixture(t)
		infraErr := errors.New("webauthn library down")
		adapter.beginLoginFn = func() ([]byte, []byte, []byte, error) {
			return nil, nil, nil, infraErr
		}

		// Act
		_, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)

		// Assert
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, infraErr) {
			t.Errorf("expected wrapped %v, got %v", infraErr, err)
		}
		if errors.Is(err, ErrAuthenticationFailed) {
			t.Errorf("infra error must not be re-mapped to uniform sentinel: %v", err)
		}
	})
}

// ------------------------------------------------------------
// FinishAuthentication
// ------------------------------------------------------------

// buildSuccessfulFinishLogin は「lookup を呼び lookup が返す user handle と credential ID を
// そのまま返して updatedSignCount と updatedBackupState を返す」stub adapter を組み立てる
// ヘルパー。テストが提示する credentialID を lookup に渡す。updatedBackupState は
// Issue #234 task 3 で追加された 4 番目の戻り値であり、library が assertion authenticatorData
// から取り出した最新 BS 値を模擬する（Req 4.2 の観測点）。
func buildSuccessfulFinishLogin(
	presentedCredentialID []byte, updatedSignCount uint32, updatedBackupState bool,
) func(
	sessionData, requestBody []byte,
	lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
) ([]byte, []byte, uint32, bool, error) {
	return func(sessionData, requestBody []byte,
		lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
	) ([]byte, []byte, uint32, bool, error) {
		user, parsed, err := lookup(presentedCredentialID)
		if err != nil {
			// adapter は lookup エラーを ErrAuthenticationFailed に正規化する仕様
			return nil, nil, 0, false, ErrAuthenticationFailed
		}
		if user == nil || parsed == nil {
			return nil, nil, 0, false, ErrAuthenticationFailed
		}
		return user.WebAuthnID(), parsed.ID, updatedSignCount, updatedBackupState, nil
	}
}

func TestAuthenticationService_FinishAuthentication(t *testing.T) {
	ctx := context.Background()
	targetUserID := "user-uuid-authn-123"
	credentialID := []byte("credential-id-bytes")
	credRowID := "cred-row-uuid-abc"

	setupExistingCredential := func(creds *stubCredentialReader, users *stubUserReader) {
		creds.findByCredentialIDFn = func(ctx context.Context, id []byte) (*model.PasskeyCredential, error) {
			if string(id) != string(credentialID) {
				return nil, nil
			}
			return &model.PasskeyCredential{
				ID:              credRowID,
				UserID:          targetUserID,
				CredentialID:    credentialID,
				PublicKey:       []byte("public-key"),
				SignCount:       5,
				AttestationType: "none",
				AAGUID:          []byte("aaguid-bytes"),
				Transports:      []string{"internal"},
			}, nil
		}
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			if id != targetUserID {
				return nil, nil
			}
			return &model.User{ID: id, Username: "alice", UsernameNormalized: "alice"}, nil
		}
	}

	t.Run("成功: 認証成功で auth_code が Create され平文が返る (Req 2.2 / 2.3)", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, creds, users, codes := newAuthenticationServiceFixture(t)
		setupExistingCredential(creds, users)
		adapter.finishLoginFn = buildSuccessfulFinishLogin(credentialID, 10, false)

		// Act — Begin して発行した challenge を Consume して Finish に流す
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		plain, err := svc.FinishAuthentication(ctx, []byte("assertion-body"), challengeID)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if plain == "" {
			t.Errorf("plain auth_code must be non-empty (returned to caller / Req 2.3)")
		}
		if codes.createCalled != 1 {
			t.Fatalf("AuthCodeCreator.Create called %d times, want 1", codes.createCalled)
		}
		if codes.created == nil {
			t.Fatal("created auth_code is nil")
		}
		if codes.created.UserID != targetUserID {
			t.Errorf("AuthCode.UserID = %q, want %q", codes.created.UserID, targetUserID)
		}
		if codes.created.CodeHash == "" {
			t.Errorf("AuthCode.CodeHash must be non-empty (SHA-256 of plain / NFR 1.2)")
		}
		if codes.created.CodeHash == plain {
			t.Errorf("AuthCode.CodeHash must not equal plain auth_code (must be hashed / NFR 1.2)")
		}
		wantExpiry := authnFixedNow.Add(60 * time.Second)
		if !codes.created.ExpiresAt.Equal(wantExpiry) {
			t.Errorf("AuthCode.ExpiresAt = %v, want %v (NativeAuthCodeTTL = 60s / Req 2.3)",
				codes.created.ExpiresAt, wantExpiry)
		}
		if challenges.consumeCalled != 1 {
			t.Errorf("Consume called %d times, want 1", challenges.consumeCalled)
		}
		if challenges.lastConsumeKind != model.PasskeyChallengeKindAuthentication {
			t.Errorf("Consume kind = %q, want authentication", challenges.lastConsumeKind)
		}
	})

	t.Run("成功時に AuthCode.PKCEChallenge が begin 時の codeChallenge と一致 (Req 2.4 の合流準備)", func(t *testing.T) {
		// Arrange
		svc, adapter, _, creds, users, codes := newAuthenticationServiceFixture(t)
		setupExistingCredential(creds, users)
		adapter.finishLoginFn = buildSuccessfulFinishLogin(credentialID, 10, false)

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		_, err = svc.FinishAuthentication(ctx, []byte("body"), challengeID)
		if err != nil {
			t.Fatalf("FinishAuthentication: %v", err)
		}

		// Assert
		if codes.created == nil {
			t.Fatal("created auth_code is nil")
		}
		if codes.created.PKCEChallenge != validPKCEChallenge {
			t.Errorf("AuthCode.PKCEChallenge = %q, want %q (envelope must carry PKCE)",
				codes.created.PKCEChallenge, validPKCEChallenge)
		}
	})

	t.Run("成功時に UpdateAuthenticationState が正しい引数で呼ばれる (Req 4.1, 4.2, NFR 1.4)", func(t *testing.T) {
		// Arrange
		svc, adapter, _, creds, users, _ := newAuthenticationServiceFixture(t)
		setupExistingCredential(creds, users)
		// updatedBackupState=true を adapter に返させ、Req 4.2 の最新化経路も同時に検証する
		adapter.finishLoginFn = buildSuccessfulFinishLogin(credentialID, 42, true)

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		if _, err := svc.FinishAuthentication(ctx, []byte("body"), challengeID); err != nil {
			t.Fatalf("FinishAuthentication: %v", err)
		}

		// Assert
		if creds.updateCalled != 1 {
			t.Fatalf("UpdateAuthenticationState called %d times, want 1", creds.updateCalled)
		}
		if creds.lastUpdatedID != credRowID {
			t.Errorf("UpdateAuthenticationState id = %q, want %q (must use credential PK / not credentialID)",
				creds.lastUpdatedID, credRowID)
		}
		if creds.lastUpdatedSignCount != 42 {
			t.Errorf("UpdateAuthenticationState signCount = %d, want 42", creds.lastUpdatedSignCount)
		}
		if !creds.lastUpdatedBackupState {
			t.Errorf("UpdateAuthenticationState backupState = %v, want true (must reflect updatedBackupState / Req 4.2)",
				creds.lastUpdatedBackupState)
		}
		if !creds.lastUpdatedLastUsed.Equal(authnFixedNow) {
			t.Errorf("UpdateAuthenticationState lastUsedAt = %v, want %v (must use service now())",
				creds.lastUpdatedLastUsed, authnFixedNow)
		}
	})

	t.Run("成功時に UpdateAuthenticationState には backupState=false も伝播する (Req 4.2 の反対系)", func(t *testing.T) {
		// Arrange: adapter が updatedBackupState=false を返した場合、DB 更新側も false になる
		svc, adapter, _, creds, users, _ := newAuthenticationServiceFixture(t)
		setupExistingCredential(creds, users)
		adapter.finishLoginFn = buildSuccessfulFinishLogin(credentialID, 7, false)

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		if _, err := svc.FinishAuthentication(ctx, []byte("body"), challengeID); err != nil {
			t.Fatalf("FinishAuthentication: %v", err)
		}

		// Assert
		if creds.updateCalled != 1 {
			t.Fatalf("UpdateAuthenticationState called %d times, want 1", creds.updateCalled)
		}
		if creds.lastUpdatedBackupState {
			t.Errorf("UpdateAuthenticationState backupState = true, want false (must reflect adapter value / Req 4.2)")
		}
	})

	t.Run("lookup が返す webauthn.Credential.Flags に stored BE/BS が反映される (Req 2.1, 2.2, 2.3)", func(t *testing.T) {
		// Arrange: stored credential を BE=true / BS=true で組み立てる
		svc, adapter, _, creds, users, _ := newAuthenticationServiceFixture(t)
		creds.findByCredentialIDFn = func(ctx context.Context, id []byte) (*model.PasskeyCredential, error) {
			return &model.PasskeyCredential{
				ID:              credRowID,
				UserID:          targetUserID,
				CredentialID:    credentialID,
				PublicKey:       []byte("public-key"),
				SignCount:       5,
				AttestationType: "none",
				AAGUID:          []byte("aaguid-bytes"),
				Transports:      []string{"internal"},
				BackupEligible:  true,
				BackupState:     true,
			}, nil
		}
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Username: "alice", UsernameNormalized: "alice"}, nil
		}

		// spy: adapter 側で lookup を呼び出し、返された WebAuthnUser の Flags を捕捉する。
		// これが library の login validation（v0.17.4 login.go:371）に流れる Flags そのもの。
		var capturedFlags webauthn.CredentialFlags
		var lookupCalled int
		adapter.finishLoginFn = func(sessionData, requestBody []byte,
			lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
		) ([]byte, []byte, uint32, bool, error) {
			user, parsed, lookupErr := lookup(credentialID)
			if lookupErr != nil {
				return nil, nil, 0, false, ErrAuthenticationFailed
			}
			lookupCalled++
			wcreds := user.WebAuthnCredentials()
			if len(wcreds) != 1 {
				return nil, nil, 0, false, errors.New("expected 1 webauthn credential")
			}
			capturedFlags = wcreds[0].Flags
			return user.WebAuthnID(), parsed.ID, 10, true, nil
		}

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		if _, err := svc.FinishAuthentication(ctx, []byte("body"), challengeID); err != nil {
			t.Fatalf("FinishAuthentication: %v", err)
		}

		// Assert
		if lookupCalled != 1 {
			t.Fatalf("lookup should be called exactly once, got %d", lookupCalled)
		}
		if !capturedFlags.BackupEligible {
			t.Errorf("webauthn.Credential.Flags.BackupEligible = false, want true " +
				"(stored BE must be propagated to lookup / Req 2.1〜2.3, login.go:371 の一致判定用)")
		}
		if !capturedFlags.BackupState {
			t.Errorf("webauthn.Credential.Flags.BackupState = false, want true " +
				"(stored BS も同時に propagate する必要がある / Req 2.1〜2.3)")
		}
	})

	t.Run("stored BE=false のとき Flags.BackupEligible=false が lookup に反映される (Req 2.1 反対系)", func(t *testing.T) {
		// Arrange: 既存 credential 行（BE=false / BS=false）を模擬
		svc, adapter, _, creds, users, _ := newAuthenticationServiceFixture(t)
		creds.findByCredentialIDFn = func(ctx context.Context, id []byte) (*model.PasskeyCredential, error) {
			return &model.PasskeyCredential{
				ID:             credRowID,
				UserID:         targetUserID,
				CredentialID:   credentialID,
				PublicKey:      []byte("public-key"),
				BackupEligible: false,
				BackupState:    false,
			}, nil
		}
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Username: "bob", UsernameNormalized: "bob"}, nil
		}

		var capturedFlags webauthn.CredentialFlags
		adapter.finishLoginFn = func(sessionData, requestBody []byte,
			lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
		) ([]byte, []byte, uint32, bool, error) {
			user, parsed, lookupErr := lookup(credentialID)
			if lookupErr != nil {
				return nil, nil, 0, false, ErrAuthenticationFailed
			}
			capturedFlags = user.WebAuthnCredentials()[0].Flags
			return user.WebAuthnID(), parsed.ID, 3, false, nil
		}

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		if _, err := svc.FinishAuthentication(ctx, []byte("body"), challengeID); err != nil {
			t.Fatalf("FinishAuthentication: %v", err)
		}

		// Assert
		if capturedFlags.BackupEligible {
			t.Errorf("webauthn.Credential.Flags.BackupEligible = true, want false (stored BE=false 値の propagate)")
		}
		if capturedFlags.BackupState {
			t.Errorf("webauthn.Credential.Flags.BackupState = true, want false")
		}
	})

	t.Run("challenge 期限切れ (Consume が ErrChallengeNotUsable) は ErrAuthenticationFailed に正規化", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, creds, _, codes := newAuthenticationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, id string, kind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
			return nil, ErrChallengeNotUsable
		}

		// Act
		_, err := svc.FinishAuthentication(ctx, []byte("body"), "expired-id")

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
		}
		if adapter.finishLoginCalled != 0 {
			t.Errorf("FinishLogin must not be called after challenge rejected")
		}
		if creds.updateCalled != 0 || codes.createCalled != 0 {
			t.Errorf("no writes should occur after challenge rejected")
		}
	})

	t.Run("credential 未検出は ErrAuthenticationFailed (lookup が nil → adapter が正規化)", func(t *testing.T) {
		// Arrange
		svc, adapter, _, creds, users, codes := newAuthenticationServiceFixture(t)
		// credentials.FindByCredentialID は常に nil を返す（credential 未検出）
		creds.findByCredentialIDFn = func(ctx context.Context, id []byte) (*model.PasskeyCredential, error) {
			return nil, nil
		}
		users.findByIDFn = func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		}
		adapter.finishLoginFn = buildSuccessfulFinishLogin(credentialID, 10, false)

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		_, err = svc.FinishAuthentication(ctx, []byte("body"), challengeID)

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
		}
		if codes.createCalled != 0 {
			t.Errorf("auth_code must not be issued when credential not found")
		}
	})

	t.Run("counter 後退 (adapter が ErrAuthenticationFailed) は そのまま返し auth_code を作らない", func(t *testing.T) {
		// Arrange
		svc, adapter, _, creds, users, codes := newAuthenticationServiceFixture(t)
		setupExistingCredential(creds, users)
		// adapter が CloneWarning などで ErrAuthenticationFailed を返すケース
		adapter.finishLoginFn = func(sessionData, requestBody []byte,
			lookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
		) ([]byte, []byte, uint32, bool, error) {
			// lookup は呼ぶが最終的に counter 後退で reject
			_, _, _ = lookup(credentialID)
			return nil, nil, 0, false, ErrAuthenticationFailed
		}

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		_, err = svc.FinishAuthentication(ctx, []byte("body"), challengeID)

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
		}
		if creds.updateCalled != 0 {
			t.Errorf("UpdateAuthenticationState must not be called on rejected assertion (Req 4.1 / 4.3)")
		}
		if codes.createCalled != 0 {
			t.Errorf("auth_code must not be issued on rejected assertion")
		}
	})

	t.Run("malformed session envelope は ErrAuthenticationFailed", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, _, _, codes := newAuthenticationServiceFixture(t)
		// Consume が envelope 形式ではない SessionData を返すケース
		challenges.consumeFn = func(ctx context.Context, id string, kind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
			return &model.PasskeyChallenge{
				ID:          id,
				Kind:        kind,
				SessionData: []byte("not-a-json"),
			}, nil
		}

		// Act
		_, err := svc.FinishAuthentication(ctx, []byte("body"), "some-id")

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
		}
		if adapter.finishLoginCalled != 0 {
			t.Errorf("FinishLogin must not be called on malformed envelope")
		}
		if codes.createCalled != 0 {
			t.Errorf("auth_code must not be issued on malformed envelope")
		}
	})

	t.Run("envelope に codeChallenge が空でも ErrAuthenticationFailed (incomplete envelope 防衛)", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, _, _, _ := newAuthenticationServiceFixture(t)
		// Consume が code_challenge 空の envelope を返すケース
		envelope := authnSession{
			WebAuthnSession: json.RawMessage(`{"webauthn":"session"}`),
			CodeChallenge:   "", // missing
		}
		envelopeBytes, _ := json.Marshal(envelope)
		challenges.consumeFn = func(ctx context.Context, id string, kind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
			return &model.PasskeyChallenge{
				ID:          id,
				Kind:        kind,
				SessionData: envelopeBytes,
			}, nil
		}

		// Act
		_, err := svc.FinishAuthentication(ctx, []byte("body"), "some-id")

		// Assert
		if !errors.Is(err, ErrAuthenticationFailed) {
			t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
		}
		if adapter.finishLoginCalled != 0 {
			t.Errorf("FinishLogin must not be called on incomplete envelope")
		}
	})

	t.Run("Consume の infra エラーは wrap して伝播 (uniform 化しない)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, _, _ := newAuthenticationServiceFixture(t)
		infraErr := errors.New("db down")
		challenges.consumeFn = func(ctx context.Context, id string, kind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
			return nil, infraErr
		}

		// Act
		_, err := svc.FinishAuthentication(ctx, []byte("body"), "id")

		// Assert
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, infraErr) {
			t.Errorf("expected wrapped %v, got %v", infraErr, err)
		}
		if errors.Is(err, ErrAuthenticationFailed) {
			t.Errorf("infra error must not be re-mapped to uniform sentinel: %v", err)
		}
	})

	t.Run("AuthCodeCreator.Create の infra エラーは wrap して伝播 (uniform 化しない)", func(t *testing.T) {
		// Arrange
		svc, adapter, _, creds, users, codes := newAuthenticationServiceFixture(t)
		setupExistingCredential(creds, users)
		adapter.finishLoginFn = buildSuccessfulFinishLogin(credentialID, 10, false)
		infraErr := errors.New("auth_code insert failed")
		codes.createFn = func(ctx context.Context, c *model.AuthCode) error {
			return infraErr
		}

		// Act
		challengeID, _, err := svc.BeginAuthentication(ctx, validPKCEChallenge)
		if err != nil {
			t.Fatalf("BeginAuthentication: %v", err)
		}
		_, err = svc.FinishAuthentication(ctx, []byte("body"), challengeID)

		// Assert
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, infraErr) {
			t.Errorf("expected wrapped %v, got %v", infraErr, err)
		}
	})
}
