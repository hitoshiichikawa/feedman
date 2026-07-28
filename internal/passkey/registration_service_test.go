package passkey

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/hitoshi/feedman/internal/auth"
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
) ([]byte, []byte, uint32, bool, error) {
	// Issue #234 task 3 compile glue: WebAuthnAdapter interface に updatedBackupState を
	// 追加したため、本 stub もシグネチャを合わせる必要がある（no-op 実装のまま）。
	// 挙動は不変。
	return nil, nil, 0, false, errors.New("FinishLogin not used in registration tests")
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
	findByNormalizedFn   func(ctx context.Context, normalized string) (*model.User, error)
	createUserOnlyFn     func(ctx context.Context, u *model.User) error
	createUserOnlyExecFn func(ctx context.Context, q repository.DBTX, u *model.User) error
	findByIDFn           func(ctx context.Context, id string) (*model.User, error)

	createUserOnlyCalled     int
	createUserOnlyExecCalled int
	lastCreated              *model.User
	// lastCreateExecQuerier は Issue #230 のテストで CreateUserOnlyExec が
	// 期待する共有トランザクション上の querier で呼ばれたかを検証するために保持する。
	lastCreateExecQuerier repository.DBTX
	callOrder             *[]string
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

func (u *stubUserWriter) CreateUserOnlyExec(ctx context.Context, q repository.DBTX, user *model.User) error {
	u.createUserOnlyExecCalled++
	u.lastCreated = user
	u.lastCreateExecQuerier = q
	if u.callOrder != nil {
		*u.callOrder = append(*u.callOrder, "user")
	}
	if u.createUserOnlyExecFn != nil {
		return u.createUserOnlyExecFn(ctx, q, user)
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
	createExecFn         func(ctx context.Context, q repository.DBTX, c *model.PasskeyCredential) error

	createCalled     int
	createExecCalled int
	lastCreatedCred  *model.PasskeyCredential
	listByUserCalled int
	// lastCreateExecQuerier は Issue #230 のテストで CreateExec が
	// 期待する共有トランザクション上の querier で呼ばれたかを検証するために保持する。
	lastCreateExecQuerier repository.DBTX
	callOrder             *[]string
}

type stubSessionWriter struct {
	createExecFn func(ctx context.Context, q repository.DBTX, session *model.Session) error

	createExecCalled      int
	lastCreatedSession    *model.Session
	lastCreateExecQuerier repository.DBTX
	callOrder             *[]string
}

func (s *stubSessionWriter) CreateExec(
	ctx context.Context,
	q repository.DBTX,
	session *model.Session,
) error {
	s.createExecCalled++
	s.lastCreatedSession = session
	s.lastCreateExecQuerier = q
	if s.callOrder != nil {
		*s.callOrder = append(*s.callOrder, "session")
	}
	if s.createExecFn != nil {
		return s.createExecFn(ctx, q, session)
	}
	return nil
}

type stubRegistrationSessionFactory struct {
	newSessionFn func(userID string) (*model.Session, error)

	newSessionCalled int
	lastUserID       string
	session          *model.Session
}

func (f *stubRegistrationSessionFactory) NewSession(userID string) (*model.Session, error) {
	f.newSessionCalled++
	f.lastUserID = userID
	if f.newSessionFn != nil {
		return f.newSessionFn(userID)
	}
	session := f.session
	if session == nil {
		session = &model.Session{
			ID:        "web-session-id",
			UserID:    userID,
			CreatedAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
			ExpiresAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		}
	}
	return session, nil
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

func (c *stubCredentialWriter) CreateExec(ctx context.Context, q repository.DBTX, cred *model.PasskeyCredential) error {
	c.createExecCalled++
	c.lastCreatedCred = cred
	c.lastCreateExecQuerier = q
	if c.callOrder != nil {
		*c.callOrder = append(*c.callOrder, "credential")
	}
	if c.createExecFn != nil {
		return c.createExecFn(ctx, q, cred)
	}
	return nil
}

// stubRegistrationTx / stubRegistrationTxBeginner は RegistrationTx /
// RegistrationTxBeginner を差し替えるスタブ（Issue #230 / Req 1.1〜1.6 / NFR 4.1）。
//
// stubRegistrationTx は Querier / Commit / Rollback の呼び出し回数を記録し、
// FinishRegistrationNew が「両成功時のみ Commit」「失敗時は Rollback」の契約を
// 満たしているかを検証する。
type stubRegistrationTx struct {
	querier        repository.DBTX
	commitCalled   int
	rollbackCalled int
	commitErr      error
}

func (t *stubRegistrationTx) Querier() repository.DBTX { return t.querier }
func (t *stubRegistrationTx) Commit() error {
	t.commitCalled++
	return t.commitErr
}
func (t *stubRegistrationTx) Rollback() error {
	t.rollbackCalled++
	return nil
}

type stubRegistrationTxBeginner struct {
	beginErr    error
	commitErr   error
	beginCalled int
	// 発行済み tx を保持し、テスト側から Commit/Rollback の呼び出し回数を検証できるようにする。
	lastTx *stubRegistrationTx
	// querier は Querier() が返す値。stub なので任意の senyinel を渡せる（nil でも可）。
	querier repository.DBTX
}

func (b *stubRegistrationTxBeginner) BeginTx(ctx context.Context) (RegistrationTx, error) {
	b.beginCalled++
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	tx := &stubRegistrationTx{querier: b.querier, commitErr: b.commitErr}
	b.lastTx = tx
	return tx, nil
}

// stubDBTX は repository.DBTX を充足する sentinel。実際にクエリを実行することは
// なく、tx.Querier() が返した値と *Exec に渡された値が同一であることの検証にのみ使う。
// stub 内の *Exec 実装で呼ばれない前提のため、全メソッドは panic する。
type stubDBTX struct {
	label string
}

func (s *stubDBTX) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	panic("stubDBTX.ExecContext should not be called in unit tests")
}
func (s *stubDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	panic("stubDBTX.QueryContext should not be called in unit tests")
}
func (s *stubDBTX) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	panic("stubDBTX.QueryRowContext should not be called in unit tests")
}

// validPKCEChallenge は auth.ValidatePKCES256 の形式（43 文字 base64url）を通過する
// テスト用固定値。
const validPKCEChallenge = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

// newRegistrationServiceFixture は RegistrationService の全依存を stub 化した
// 標準セットを返すヘルパー。テストごとに必要な stub の挙動だけを差し替える。
//
// Issue #230 / Req 1.1〜1.6: 新規登録 finish の 1 tx 化に伴い RegistrationTxBeginner を
// 追加で注入する。tx stub の Querier は sentinel *stubDBTX を返し、CreateUserOnlyExec /
// CreateExec に同一 querier が渡ることをテスト側で検証できるようにする。
func newRegistrationServiceFixture(t *testing.T) (
	*RegistrationService,
	*stubWebAuthnAdapter,
	*stubChallengeStoreForRegistration,
	*stubUserWriter,
	*stubCredentialWriter,
	*stubRegistrationTxBeginner,
) {
	t.Helper()
	adapter := &stubWebAuthnAdapter{}
	challenges := &stubChallengeStoreForRegistration{}
	users := &stubUserWriter{}
	creds := &stubCredentialWriter{}
	txBeginner := &stubRegistrationTxBeginner{querier: &stubDBTX{label: "tx-querier"}}
	sessions := &stubSessionWriter{}
	sessionFactory := &stubRegistrationSessionFactory{}
	now := func() time.Time { return time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC) }
	svc := NewRegistrationService(
		adapter,
		challenges,
		users,
		creds,
		txBeginner,
		sessions,
		auth.SessionFactoryFunc(sessionFactory),
		now,
	)
	return svc, adapter, challenges, users, creds, txBeginner
}

// ------------------------------------------------------------
// 新規登録 (BeginRegistrationNew / FinishRegistrationNew)
// ------------------------------------------------------------

func TestRegistrationService_BeginRegistrationNew(t *testing.T) {
	ctx := context.Background()

	t.Run("有効な username と PKCE で challenge_id と options を返す", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _, _ := newRegistrationServiceFixture(t)
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
		if challenges.lastIssueUser != nil {
			t.Errorf("Issue userID must be nil for registration_new (user_id は users FK), got %v",
				*challenges.lastIssueUser)
		}
		if challenges.lastIssuePend == nil || *challenges.lastIssuePend != "alice" {
			t.Errorf("Issue pendingUsername = %v, want &alice", challenges.lastIssuePend)
		}
		// 仮 UUID（WebAuthn user handle）が発行されていることを検証
		// （sessionData 経由で Finish に引き継がれる / task 5 credentialLookup 整合性の担保）。
		if len(adapter.lastUserID) == 0 {
			t.Errorf("WebAuthnID must be a non-empty pending UUID, got empty")
		}
	})

	t.Run("username 形式不正 (空) は ErrInvalidUsername を返し ceremony を起動しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _, _ := newRegistrationServiceFixture(t)

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
		svc, _, _, _, _, _ := newRegistrationServiceFixture(t)

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "invalid space", "", validPKCEChallenge)

		// Assert
		if !errors.Is(err, ErrInvalidUsername) {
			t.Fatalf("expected ErrInvalidUsername, got %v", err)
		}
	})

	t.Run("username 重複は ErrUsernameTaken を返し WebAuthn を起動しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, _, _ := newRegistrationServiceFixture(t)
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
		svc, adapter, challenges, _, _, _ := newRegistrationServiceFixture(t)

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
		svc, _, _, _, _, _ := newRegistrationServiceFixture(t)

		// Act
		_, _, err := svc.BeginRegistrationNew(ctx, "alice", "", validPKCEChallenge)

		// Assert
		if err != nil {
			t.Fatalf("expected no error for empty email, got %v", err)
		}
	})

	t.Run("FindByNormalizedUsername infra エラーは wrap して返す", func(t *testing.T) {
		// Arrange
		svc, _, _, users, _, _ := newRegistrationServiceFixture(t)
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

	// begin 時の仮 UUID は challenge.user_id 列ではなく sessionData
	// （webauthn.SessionData.UserID）に保存される契約（BeginRegistrationNew 参照）。
	sessionJSON, err := json.Marshal(webauthn.SessionData{UserID: []byte(pendingUserID)})
	if err != nil {
		t.Fatalf("marshal session fixture: %v", err)
	}

	newConsumedFn := func() func(ctx context.Context, challengeID string,
		expectedKind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error) {
		return func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			pend := pendingUsername
			return &model.PasskeyChallenge{
				ID:              challengeID,
				Kind:            expectedKind,
				PendingUsername: &pend,
				SessionData:     sessionJSON,
			}, nil
		}
	}

	t.Run("成功: user 行と credential 行を単一 tx で作成し Commit / userID を返す (Req 1.1 / Req 1.6 / Issue #230 Req 1.1・1.2)", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()

		// Act
		userID, webSession, err := svc.FinishRegistrationNew(
			ctx, "challenge-id", []byte("attestation-body"), false,
		)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if userID != pendingUserID {
			t.Errorf("userID = %q, want %q (pending UUID promoted to users.id)", userID, pendingUserID)
		}
		if webSession != nil {
			t.Errorf("webSession = %+v, want nil in iOS/native mode", webSession)
		}
		sessions := svc.sessions.(*stubSessionWriter)
		factory := svc.sessionFactory.(*stubRegistrationSessionFactory)
		if sessions.createExecCalled != 0 || factory.newSessionCalled != 0 {
			t.Errorf("iOS/native mode must not create a session: writer=%d factory=%d",
				sessions.createExecCalled, factory.newSessionCalled)
		}
		// tx 契約: BeginTx が 1 回、Commit が 1 回、Rollback は defer no-op（既 commit 済み）
		if txBeginner.beginCalled != 1 {
			t.Errorf("BeginTx called %d times, want 1", txBeginner.beginCalled)
		}
		if txBeginner.lastTx == nil {
			t.Fatal("txBeginner.lastTx is nil")
		}
		if txBeginner.lastTx.commitCalled != 1 {
			t.Errorf("Commit called %d times, want 1 (成功時のみ Commit)", txBeginner.lastTx.commitCalled)
		}
		// defer は committed=true なので Rollback を呼ばない
		if txBeginner.lastTx.rollbackCalled != 0 {
			t.Errorf("Rollback called %d times, want 0 (Commit 済みは Rollback しない)", txBeginner.lastTx.rollbackCalled)
		}
		// CreateUserOnlyExec / CreateExec の呼び出しと同一 tx querier での実行を検証
		if users.createUserOnlyExecCalled != 1 {
			t.Errorf("CreateUserOnlyExec called %d times, want 1", users.createUserOnlyExecCalled)
		}
		if users.createUserOnlyCalled != 0 {
			t.Errorf("CreateUserOnly (non-Exec) must not be called; got %d (finish は Exec 経由が正)", users.createUserOnlyCalled)
		}
		if creds.createExecCalled != 1 {
			t.Errorf("credential.CreateExec called %d times, want 1", creds.createExecCalled)
		}
		if creds.createCalled != 0 {
			t.Errorf("credential.Create (non-Exec) must not be called; got %d", creds.createCalled)
		}
		// tx.Querier() と CreateUserOnlyExec / CreateExec に渡された DBTX が同一
		wantQuerier := txBeginner.querier
		if users.lastCreateExecQuerier != wantQuerier {
			t.Errorf("CreateUserOnlyExec に渡された querier が tx.Querier() と一致しない (users)")
		}
		if creds.lastCreateExecQuerier != wantQuerier {
			t.Errorf("CreateExec に渡された querier が tx.Querier() と一致しない (credentials)")
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
		if creds.lastCreatedCred == nil || creds.lastCreatedCred.UserID != pendingUserID {
			t.Errorf("credential.UserID mismatch: %+v", creds.lastCreatedCred)
		}
		if string(adapter.lastUserID) != pendingUserID {
			t.Errorf("FinishRegistration user WebAuthnID = %q, want %q",
				string(adapter.lastUserID), pendingUserID)
		}
	})

	t.Run("Web mode: user・credential・session を同一 tx へ順番に作成して Commit する", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		sessions := svc.sessions.(*stubSessionWriter)
		factory := svc.sessionFactory.(*stubRegistrationSessionFactory)
		callOrder := []string{}
		users.callOrder = &callOrder
		creds.callOrder = &callOrder
		sessions.callOrder = &callOrder
		wantSession := &model.Session{
			ID:        "web-session-id",
			UserID:    pendingUserID,
			CreatedAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
			ExpiresAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		}
		factory.newSessionFn = func(userID string) (*model.Session, error) {
			if userID != pendingUserID {
				t.Errorf("factory userID = %q, want %q", userID, pendingUserID)
			}
			return wantSession, nil
		}

		// Act
		userID, webSession, err := svc.FinishRegistrationNew(
			ctx, "challenge-id", []byte("attestation-body"), true,
		)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if userID != pendingUserID {
			t.Errorf("userID = %q, want %q", userID, pendingUserID)
		}
		if webSession != wantSession {
			t.Errorf("webSession = %+v, want factory session %+v", webSession, wantSession)
		}
		if factory.newSessionCalled != 1 || factory.lastUserID != pendingUserID {
			t.Errorf("factory calls/user = %d/%q, want 1/%q",
				factory.newSessionCalled, factory.lastUserID, pendingUserID)
		}
		if sessions.createExecCalled != 1 || sessions.lastCreatedSession != wantSession {
			t.Errorf("session CreateExec calls/session = %d/%+v, want 1/%+v",
				sessions.createExecCalled, sessions.lastCreatedSession, wantSession)
		}
		if sessions.lastCreateExecQuerier != txBeginner.querier {
			t.Error("session CreateExec did not receive tx.Querier()")
		}
		wantOrder := []string{"user", "credential", "session"}
		if len(callOrder) != len(wantOrder) {
			t.Fatalf("call order = %v, want %v", callOrder, wantOrder)
		}
		for i := range wantOrder {
			if callOrder[i] != wantOrder[i] {
				t.Errorf("call order = %v, want %v", callOrder, wantOrder)
				break
			}
		}
		if txBeginner.lastTx.commitCalled != 1 || txBeginner.lastTx.rollbackCalled != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0",
				txBeginner.lastTx.commitCalled, txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("Web mode: session factory 失敗は全 rollback し webSession を返さない", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		sessions := svc.sessions.(*stubSessionWriter)
		factory := svc.sessionFactory.(*stubRegistrationSessionFactory)
		wantErr := errors.New("session ID generation failed")
		factory.newSessionFn = func(string) (*model.Session, error) {
			return nil, wantErr
		}

		// Act
		userID, webSession, err := svc.FinishRegistrationNew(
			ctx, "challenge-id", []byte("attestation-body"), true,
		)

		// Assert
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapped %v", err, wantErr)
		}
		if userID != "" || webSession != nil {
			t.Errorf("userID/webSession = %q/%+v, want empty/nil", userID, webSession)
		}
		if users.createUserOnlyExecCalled != 1 || creds.createExecCalled != 1 {
			t.Errorf("user/credential calls = %d/%d, want 1/1 before factory failure",
				users.createUserOnlyExecCalled, creds.createExecCalled)
		}
		if sessions.createExecCalled != 0 {
			t.Errorf("session CreateExec calls = %d, want 0", sessions.createExecCalled)
		}
		if txBeginner.lastTx.commitCalled != 0 || txBeginner.lastTx.rollbackCalled != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1",
				txBeginner.lastTx.commitCalled, txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("Web mode: session INSERT 失敗は全 rollback し webSession を返さない", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, _, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		sessions := svc.sessions.(*stubSessionWriter)
		wantErr := errors.New("session insert failed")
		sessions.createExecFn = func(context.Context, repository.DBTX, *model.Session) error {
			return wantErr
		}

		// Act
		userID, webSession, err := svc.FinishRegistrationNew(
			ctx, "challenge-id", []byte("attestation-body"), true,
		)

		// Assert
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapped %v", err, wantErr)
		}
		if userID != "" || webSession != nil {
			t.Errorf("userID/webSession = %q/%+v, want empty/nil", userID, webSession)
		}
		if sessions.createExecCalled != 1 {
			t.Errorf("session CreateExec calls = %d, want 1", sessions.createExecCalled)
		}
		if txBeginner.lastTx.commitCalled != 0 || txBeginner.lastTx.rollbackCalled != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1",
				txBeginner.lastTx.commitCalled, txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("Web mode: Commit 失敗は rollback し構築済み webSession を返さない", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, _, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		wantErr := errors.New("commit failed")
		txBeginner.commitErr = wantErr

		// Act
		userID, webSession, err := svc.FinishRegistrationNew(
			ctx, "challenge-id", []byte("attestation-body"), true,
		)

		// Assert
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want wrapped %v", err, wantErr)
		}
		if userID != "" || webSession != nil {
			t.Errorf("userID/webSession = %q/%+v, want empty/nil", userID, webSession)
		}
		if txBeginner.lastTx.commitCalled != 1 || txBeginner.lastTx.rollbackCalled != 1 {
			t.Errorf("commit/rollback = %d/%d, want 1/1",
				txBeginner.lastTx.commitCalled, txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("challenge 期限切れ (Consume が ErrChallengeNotUsable) は ErrRegistrationFailed に正規化 / tx 未開始", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			return nil, ErrChallengeNotUsable
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "expired-id", []byte("body"), false)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if users.createUserOnlyExecCalled != 0 || creds.createExecCalled != 0 {
			t.Errorf("no writes should occur on expired challenge")
		}
		if txBeginner.beginCalled != 0 {
			t.Errorf("BeginTx must not be called on expired challenge (early return before tx)")
		}
	})

	t.Run("attestation 検証失敗 (adapter が ErrRegistrationFailed) は そのまま返し tx を開始しない", func(t *testing.T) {
		// Arrange
		svc, adapter, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		adapter.finishRegistrationFn = func(user WebAuthnUser, sessionData []byte, requestBody []byte) (
			*ParsedCredential, error,
		) {
			return nil, ErrRegistrationFailed
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("bad-body"), false)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		if users.createUserOnlyExecCalled != 0 {
			t.Errorf("CreateUserOnlyExec must not be called after attestation failure")
		}
		if creds.createExecCalled != 0 {
			t.Errorf("credential.CreateExec must not be called after attestation failure")
		}
		if txBeginner.beginCalled != 0 {
			t.Errorf("BeginTx must not be called after attestation failure (attestation は tx 開始より前)")
		}
	})

	t.Run("CreateUserOnlyExec の UNIQUE 衝突 (username race) は tx rollback + ErrRegistrationFailed / credential は永続化されない (Issue #230 Req 1.5)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		users.createUserOnlyExecFn = func(ctx context.Context, q repository.DBTX, u *model.User) error {
			return repository.ErrUsernameTaken
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"), false)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed (uniform), got %v", err)
		}
		if creds.createExecCalled != 0 {
			t.Errorf("credential.CreateExec must not be called after user race conflict (順序: user → credential)")
		}
		if txBeginner.beginCalled != 1 {
			t.Errorf("BeginTx called %d times, want 1", txBeginner.beginCalled)
		}
		if txBeginner.lastTx == nil {
			t.Fatal("txBeginner.lastTx is nil")
		}
		if txBeginner.lastTx.commitCalled != 0 {
			t.Errorf("Commit must not be called on user race conflict; got %d", txBeginner.lastTx.commitCalled)
		}
		if txBeginner.lastTx.rollbackCalled != 1 {
			t.Errorf("Rollback must be called exactly once on user race conflict; got %d (Req 1.5)", txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("credential 重複 (ErrCredentialAlreadyRegistered) は tx rollback + ErrRegistrationFailed / user 行は永続化されない (Issue #230 Req 1.3)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		creds.createExecFn = func(ctx context.Context, q repository.DBTX, c *model.PasskeyCredential) error {
			return repository.ErrCredentialAlreadyRegistered
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"), false)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed, got %v", err)
		}
		// user は 1 回 tx 上で CreateUserOnlyExec が呼ばれる（先行）
		if users.createUserOnlyExecCalled != 1 {
			t.Errorf("CreateUserOnlyExec should be called before credential.CreateExec; got %d", users.createUserOnlyExecCalled)
		}
		// credential も 1 回 CreateExec が呼ばれ、そこで重複が返る
		if creds.createExecCalled != 1 {
			t.Errorf("credential.CreateExec should be called; got %d", creds.createExecCalled)
		}
		// tx 契約: Commit されず Rollback が呼ばれる（Req 1.3 の孤立ユーザー禁止）
		if txBeginner.lastTx == nil {
			t.Fatal("txBeginner.lastTx is nil")
		}
		if txBeginner.lastTx.commitCalled != 0 {
			t.Errorf("Commit must not be called on credential duplicate; got %d (Req 1.3)", txBeginner.lastTx.commitCalled)
		}
		if txBeginner.lastTx.rollbackCalled != 1 {
			t.Errorf("Rollback must be called on credential duplicate; got %d", txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("credential インフラ障害 (任意の error) は tx rollback + wrap して返す / user 行は永続化されない (Issue #230 Req 1.4)", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		infraErr := errors.New("simulated postgres down")
		creds.createExecFn = func(ctx context.Context, q repository.DBTX, c *model.PasskeyCredential) error {
			return infraErr
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"), false)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// infra error は uniform sentinel ではなく wrap で返す（handler で 500 相当）。
		if !errors.Is(err, infraErr) {
			t.Errorf("expected wrapped %v, got %v", infraErr, err)
		}
		if errors.Is(err, ErrRegistrationFailed) {
			t.Errorf("infra error must not be re-mapped to ErrRegistrationFailed uniform sentinel: %v", err)
		}
		if users.createUserOnlyExecCalled != 1 {
			t.Errorf("CreateUserOnlyExec should be called before credential.CreateExec; got %d", users.createUserOnlyExecCalled)
		}
		if creds.createExecCalled != 1 {
			t.Errorf("credential.CreateExec should be called; got %d", creds.createExecCalled)
		}
		// Req 1.4: 孤立ユーザーを残さない = Rollback される
		if txBeginner.lastTx == nil {
			t.Fatal("txBeginner.lastTx is nil")
		}
		if txBeginner.lastTx.commitCalled != 0 {
			t.Errorf("Commit must not be called on credential infra error; got %d (Req 1.4)", txBeginner.lastTx.commitCalled)
		}
		if txBeginner.lastTx.rollbackCalled != 1 {
			t.Errorf("Rollback must be called on credential infra error; got %d", txBeginner.lastTx.rollbackCalled)
		}
	})

	t.Run("BeginTx 自体の失敗は wrap して返す / 各 Exec は呼ばれない", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = newConsumedFn()
		beginErr := errors.New("simulated begin tx failure")
		txBeginner.beginErr = beginErr

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"), false)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, beginErr) {
			t.Errorf("expected wrapped %v, got %v", beginErr, err)
		}
		if errors.Is(err, ErrRegistrationFailed) {
			t.Errorf("tx begin failure must not be re-mapped to ErrRegistrationFailed")
		}
		if users.createUserOnlyExecCalled != 0 || creds.createExecCalled != 0 {
			t.Errorf("no writes should occur when BeginTx fails")
		}
	})

	t.Run("challenge の PendingUsername が nil の場合は ErrRegistrationFailed / tx 未開始", func(t *testing.T) {
		// Arrange
		svc, _, challenges, _, _, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			return &model.PasskeyChallenge{
				ID:          challengeID,
				Kind:        expectedKind,
				SessionData: sessionJSON,
				// PendingUsername = nil (契約違反シミュレート)
			}, nil
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"), false)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed on malformed challenge, got %v", err)
		}
		if txBeginner.beginCalled != 0 {
			t.Errorf("BeginTx must not be called on malformed challenge (early return before tx)")
		}
	})

	t.Run("sessionData に user handle (仮 UUID) が無い場合は ErrRegistrationFailed / tx 未開始", func(t *testing.T) {
		// Arrange
		svc, _, challenges, users, creds, txBeginner := newRegistrationServiceFixture(t)
		challenges.consumeFn = func(ctx context.Context, challengeID string,
			expectedKind model.PasskeyChallengeKind,
		) (*model.PasskeyChallenge, error) {
			pend := pendingUsername
			return &model.PasskeyChallenge{
				ID:              challengeID,
				Kind:            expectedKind,
				PendingUsername: &pend,
				SessionData:     []byte(`{}`), // user_id 欠落 (契約違反シミュレート)
			}, nil
		}

		// Act
		_, _, err := svc.FinishRegistrationNew(ctx, "id", []byte("body"), false)

		// Assert
		if !errors.Is(err, ErrRegistrationFailed) {
			t.Fatalf("expected ErrRegistrationFailed on malformed session, got %v", err)
		}
		if users.createUserOnlyExecCalled != 0 || creds.createExecCalled != 0 {
			t.Errorf("no writes should occur on malformed session")
		}
		if txBeginner.beginCalled != 0 {
			t.Errorf("BeginTx must not be called on malformed session")
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
		svc, adapter, challenges, users, creds, _ := newRegistrationServiceFixture(t)
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
		svc, adapter, challenges, users, _, _ := newRegistrationServiceFixture(t)
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
		svc, _, challenges, users, creds, _ := newRegistrationServiceFixture(t)
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
		svc, adapter, challenges, users, creds, _ := newRegistrationServiceFixture(t)
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
		svc, _, challenges, users, creds, _ := newRegistrationServiceFixture(t)
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
		svc, adapter, challenges, users, creds, _ := newRegistrationServiceFixture(t)
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
		svc, _, challenges, users, creds, _ := newRegistrationServiceFixture(t)
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
