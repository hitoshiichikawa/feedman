package user

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック（非トランザクションのレガシーパス用） ---

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

// FindByNormalizedUsername / CreateUserOnly は Issue #216 で UserRepository に
// 追加されたメソッド。本ファイルの既存テストは退会 tx フローのみを扱い
// これらを呼ばないため、interface 充足のための no-op stub として実装する。
func (m *mockUserRepo) FindByNormalizedUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}

func (m *mockUserRepo) CreateUserOnly(_ context.Context, _ *model.User) error {
	return nil
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

// --- トランザクション対応 fake ---

// fakeTx は *sql.Tx の代わりに渡す不透明なトランザクションハンドル。
type fakeTx struct{}

// fakeTxBeginner は TxBeginner を満たし、コミット／ロールバックの呼び出しを記録する。
type fakeTxBeginner struct {
	beginErr     error
	committed    bool
	rolledBack   bool
	commitErr    error
	beginCalled  bool
	commitCalled bool
}

func (b *fakeTxBeginner) BeginTx(ctx context.Context) (Tx, error) {
	b.beginCalled = true
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return &recordingTx{owner: b}, nil
}

// recordingTx は Tx を満たし、Commit / Rollback を所有者へ記録する。
type recordingTx struct {
	owner *fakeTxBeginner
}

func (t *recordingTx) Commit() error {
	t.owner.commitCalled = true
	if t.owner.commitErr != nil {
		return t.owner.commitErr
	}
	t.owner.committed = true
	return nil
}

func (t *recordingTx) Rollback() error {
	// 既にコミット済みなら no-op（database/sql の sql.ErrTxDone 相当）。
	if t.owner.committed {
		return sql.ErrTxDone
	}
	t.owner.rolledBack = true
	return nil
}

// txCall は各 deleter が受け取った tx と呼び出し順序を記録する。
type txRecorder struct {
	order []string
}

// txItemStateDeleter は TxItemStateDeleter を満たす fake。
type txItemStateDeleter struct {
	rec *txRecorder
	err error
}

func (d *txItemStateDeleter) DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error {
	d.rec.order = append(d.rec.order, "item_states")
	return d.err
}

type txSubDeleter struct {
	rec *txRecorder
	err error
}

func (d *txSubDeleter) DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error {
	d.rec.order = append(d.rec.order, "subscriptions")
	return d.err
}

type txSessionDeleter struct {
	rec *txRecorder
	err error
}

func (d *txSessionDeleter) DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error {
	d.rec.order = append(d.rec.order, "sessions")
	return d.err
}

type txUserDeleter struct {
	rec          *txRecorder
	findByIDFn   func(ctx context.Context, id string) (*model.User, error)
	deleteErr    error
	deleteCalled bool
}

func (d *txUserDeleter) FindByID(ctx context.Context, id string) (*model.User, error) {
	return d.findByIDFn(ctx, id)
}

func (d *txUserDeleter) DeleteByIDTx(ctx context.Context, tx Tx, id string) error {
	d.deleteCalled = true
	d.rec.order = append(d.rec.order, "user")
	return d.deleteErr
}

// txAuthCodeDeleter は TxAuthCodeDeleter を満たす fake（Issue #170）。
type txAuthCodeDeleter struct {
	rec *txRecorder
	err error
}

func (d *txAuthCodeDeleter) DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error {
	d.rec.order = append(d.rec.order, "auth_codes")
	return d.err
}

// txRefreshTokenDeleter は TxRefreshTokenDeleter を満たす fake（Issue #170）。
type txRefreshTokenDeleter struct {
	rec *txRecorder
	err error
}

func (d *txRefreshTokenDeleter) DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error {
	d.rec.order = append(d.rec.order, "refresh_token_families")
	return d.err
}

// txPasskeyCredentialDeleter は TxPasskeyCredentialDeleter を満たす fake（Issue #216）。
type txPasskeyCredentialDeleter struct {
	rec *txRecorder
	err error
}

func (d *txPasskeyCredentialDeleter) DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error {
	d.rec.order = append(d.rec.order, "passkey_credentials")
	return d.err
}

// newTxService はトランザクション対応の Service を組み立てるテストヘルパ。
// Issue #170: native auth deleter（auth code / refresh token）を末尾に追加。
// Issue #216: passkey credential deleter を最末尾にさらに追加。
// nil を渡すと当該段はスキップされる（既存テストとの後方互換）。
func newTxService(
	beginner *fakeTxBeginner,
	user *txUserDeleter,
	session *txSessionDeleter,
	sub *txSubDeleter,
	state *txItemStateDeleter,
	authCode *txAuthCodeDeleter,
	refreshToken *txRefreshTokenDeleter,
	passkeyCred *txPasskeyCredentialDeleter,
) *Service {
	// nil 互換のため、対応するフィールドが nil なら interface も nil を渡す。
	var authCodeDeleter TxAuthCodeDeleter
	if authCode != nil {
		authCodeDeleter = authCode
	}
	var refreshTokenDeleter TxRefreshTokenDeleter
	if refreshToken != nil {
		refreshTokenDeleter = refreshToken
	}
	var passkeyCredentialDeleter TxPasskeyCredentialDeleter
	if passkeyCred != nil {
		passkeyCredentialDeleter = passkeyCred
	}
	return NewServiceWithTx(
		beginner, user, session, sub, state,
		authCodeDeleter, refreshTokenDeleter, passkeyCredentialDeleter,
	)
}

// --- レガシー（非トランザクション）パスのテスト ---

// TestService_Withdraw は退会処理が全関連データを削除することを検証する（AC 1.1）。
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

// TestService_Withdraw_UserNotFound は存在しないユーザーの退会がエラーになることを検証する（AC 3.1）。
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

// --- トランザクション対応パスのテスト ---

// TestService_Withdraw_Tx_CommitsOnSuccess は全削除成功時にコミットされ、
// item_states → subscriptions → sessions → passkey_credentials → auth_codes →
// refresh_token_families → user の順で削除されることを検証する（AC 1.1 / 1.4 /
// Issue #170 Req 1.1, 1.2, 2.1 / Issue #216 Req 7.1, 7.2, 7.4 / NFR 2.1）。
// Issue #170 で sessions の直後・user の前に native auth 2 段（auth_codes /
// refresh_token_families）が挿入され、Issue #216 で sessions の直後・auth_codes
// の直前に passkey_credentials 削除段が追加された。
func TestService_Withdraw_Tx_CommitsOnSuccess(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Email: "test@example.com"}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		&txAuthCodeDeleter{rec: rec},
		&txRefreshTokenDeleter{rec: rec},
		&txPasskeyCredentialDeleter{rec: rec},
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}
	if !beginner.committed {
		t.Error("expected transaction to be committed")
	}
	if beginner.rolledBack {
		t.Error("expected no rollback on success")
	}
	want := []string{
		"item_states", "subscriptions", "sessions",
		"passkey_credentials", "auth_codes", "refresh_token_families", "user",
	}
	if len(rec.order) != len(want) {
		t.Fatalf("delete order = %v, want %v", rec.order, want)
	}
	for i := range want {
		if rec.order[i] != want[i] {
			t.Errorf("delete order[%d] = %q, want %q", i, rec.order[i], want[i])
		}
	}
}

// TestService_Withdraw_Tx_RollsBackOnDeleteError は途中の削除失敗時に
// ロールバックされコミットされないことを検証する（AC 2.1 / 2.2 / 2.3）。
func TestService_Withdraw_Tx_RollsBackOnDeleteError(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	deleteErr := errors.New("subscription delete failed")
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec, err: deleteErr},
		&txItemStateDeleter{rec: rec},
		nil, // Issue #170: 本テストは subscriptions 失敗のみを検証するため auth/refresh は注入しない
		nil,
		nil, // Issue #216: passkey deleter も未注入（subscriptions 失敗より後段の全 deleter は呼ばれない）
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err == nil {
		t.Fatal("expected error from failing delete, got nil")
	}
	if !errors.Is(err, deleteErr) {
		t.Errorf("expected error to wrap %v, got %v", deleteErr, err)
	}
	if beginner.committed {
		t.Error("expected no commit when a delete fails")
	}
	if !beginner.rolledBack {
		t.Error("expected rollback when a delete fails")
	}
	// 失敗した subscriptions より後の sessions / user は実行されないこと。
	if user.deleteCalled {
		t.Error("expected user delete NOT to be called after earlier failure")
	}
}

// TestService_Withdraw_Tx_UserNotFound は存在しないユーザーでは
// トランザクションを開始も確定もしないことを検証する（AC 3.1 / 3.2）。
func TestService_Withdraw_Tx_UserNotFound(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return nil, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		nil, // Issue #170: 存在しないユーザーは tx 開始すらしないため deleter は不要
		nil,
		nil, // Issue #216: passkey deleter も同様に不要
	)

	// Act
	err := svc.Withdraw(context.Background(), "nonexistent-user")

	// Assert
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != model.ErrCodeUserNotFound {
		t.Fatalf("expected UserNotFound error, got %v", err)
	}
	if beginner.beginCalled {
		t.Error("expected no transaction to be started for nonexistent user")
	}
	if beginner.committed {
		t.Error("expected no commit for nonexistent user")
	}
	if len(rec.order) != 0 {
		t.Errorf("expected no deletes, got %v", rec.order)
	}
}

// TestService_Withdraw_Tx_NoRelatedData は関連データが 0 件でも退会が
// 成功しコミットされることを検証する（AC 4.1 / 4.2）。
func TestService_Withdraw_Tx_NoRelatedData(t *testing.T) {
	// Arrange: 各 deleter は 0 件削除でもエラーを返さない（DELETE ... WHERE は 0 行でも成功）。
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		nil, // Issue #170: nil ガード経路の検証は別テスト (TestService_Withdraw_Tx_NilNativeAuthDeletersSkip) が担う
		nil,
		nil, // Issue #216: passkey nil ガード経路の検証は別テスト (TestService_Withdraw_Tx_NilPasskeyDeleterSkip) が担う
	)

	// Act
	err := svc.Withdraw(context.Background(), "lonely-user")

	// Assert
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}
	if !beginner.committed {
		t.Error("expected commit even with no related data")
	}
	if !user.deleteCalled {
		t.Error("expected user (and CASCADE targets) to be deleted")
	}
}

// TestService_Withdraw_Tx_RollsBackOnCommitError はコミット失敗時に
// エラーを返すことを検証する（AC 2.3）。
func TestService_Withdraw_Tx_CommitError(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	commitErr := errors.New("commit failed")
	beginner := &fakeTxBeginner{commitErr: commitErr}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		nil, // Issue #170: コミットエラーの検証なので native auth deleter は不要
		nil,
		nil, // Issue #216: passkey deleter も同様に不要
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err == nil {
		t.Fatal("expected error on commit failure, got nil")
	}
	if !errors.Is(err, commitErr) {
		t.Errorf("expected error to wrap %v, got %v", commitErr, err)
	}
	if beginner.committed {
		t.Error("expected committed flag to remain false on commit error")
	}
}

// TestService_Withdraw_Tx_BeginError はトランザクション開始失敗時に
// エラーを返し削除を実行しないことを検証する（AC 2.1 / 2.3）。
func TestService_Withdraw_Tx_BeginError(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginErr := errors.New("begin failed")
	beginner := &fakeTxBeginner{beginErr: beginErr}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		nil, // Issue #170: tx 開始失敗のため deleter は呼ばれない
		nil,
		nil, // Issue #216: passkey deleter も同様に呼ばれない
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err == nil {
		t.Fatal("expected error on begin failure, got nil")
	}
	if !errors.Is(err, beginErr) {
		t.Errorf("expected error to wrap %v, got %v", beginErr, err)
	}
	if len(rec.order) != 0 {
		t.Errorf("expected no deletes when begin fails, got %v", rec.order)
	}
}

// --- Issue #170: 退会時の native auth 認証状態削除のテスト ---

// TestService_Withdraw_Tx_RollsBackOnAuthCodeDeleteError は auth_codes 削除が
// 失敗したとき、退会全体が Rollback され user / refresh_token_families 削除に
// 到達しないことを検証する（Issue #170 Req 2.2）。
func TestService_Withdraw_Tx_RollsBackOnAuthCodeDeleteError(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	deleteErr := errors.New("auth_code delete failed")
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	refreshTokenDel := &txRefreshTokenDeleter{rec: rec}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		&txAuthCodeDeleter{rec: rec, err: deleteErr},
		refreshTokenDel,
		&txPasskeyCredentialDeleter{rec: rec}, // Issue #216: passkey は auth_codes より前段で成功する
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err == nil {
		t.Fatal("expected error from failing auth_code delete, got nil")
	}
	if !errors.Is(err, deleteErr) {
		t.Errorf("expected error to wrap %v, got %v", deleteErr, err)
	}
	if beginner.committed {
		t.Error("expected no commit when auth_code delete fails")
	}
	if !beginner.rolledBack {
		t.Error("expected rollback when auth_code delete fails")
	}
	// auth_codes 失敗後の refresh_token_families / user は呼ばれない（fail-fast）
	for _, step := range rec.order {
		if step == "refresh_token_families" {
			t.Error("expected refresh_token_families delete NOT to be called after auth_codes failure")
		}
		if step == "user" {
			t.Error("expected user delete NOT to be called after auth_codes failure")
		}
	}
	if user.deleteCalled {
		t.Error("expected user delete NOT to be called after auth_codes failure")
	}
}

// TestService_Withdraw_Tx_RollsBackOnRefreshTokenDeleteError は
// refresh_token_families 削除が失敗したとき、退会全体が Rollback され user 削除に
// 到達しないことを検証する（Issue #170 Req 2.2 / 2.3）。
func TestService_Withdraw_Tx_RollsBackOnRefreshTokenDeleteError(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	deleteErr := errors.New("refresh_token delete failed")
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		&txAuthCodeDeleter{rec: rec},
		&txRefreshTokenDeleter{rec: rec, err: deleteErr},
		&txPasskeyCredentialDeleter{rec: rec}, // Issue #216: passkey は refresh_token より前段で成功する
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err == nil {
		t.Fatal("expected error from failing refresh_token delete, got nil")
	}
	if !errors.Is(err, deleteErr) {
		t.Errorf("expected error to wrap %v, got %v", deleteErr, err)
	}
	if beginner.committed {
		t.Error("expected no commit when refresh_token delete fails")
	}
	if !beginner.rolledBack {
		t.Error("expected rollback when refresh_token delete fails")
	}
	if user.deleteCalled {
		t.Error("expected user delete NOT to be called after refresh_token failure")
	}
}

// --- Issue #207: user.Service.GetByID のテスト ---

// TestService_GetByID_Success は GetByID が repository から取得した user を
// そのまま返すことを検証する（Req 2.1 / 2.2）。レガシーパスと txBeginner パスの
// 両方で同一挙動になることを subtest で確認する。
func TestService_GetByID_Success(t *testing.T) {
	want := &model.User{ID: "user-1", Email: "test@example.com", Name: "Test User"}

	t.Run("レガシーパスのとき repository が返した user をそのまま返す", func(t *testing.T) {
		// Arrange
		userRepo := &mockUserRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
				if id != want.ID {
					t.Errorf("FindByID id = %q, want %q", id, want.ID)
				}
				return want, nil
			},
		}
		svc := NewService(userRepo, nil, nil, nil)

		// Act
		got, err := svc.GetByID(context.Background(), want.ID)

		// Assert
		if err != nil {
			t.Fatalf("GetByID returned error: %v", err)
		}
		if got != want {
			t.Errorf("GetByID returned %+v, want %+v", got, want)
		}
	})

	t.Run("txBeginner パスのとき txUserDeleter が返した user をそのまま返す", func(t *testing.T) {
		// Arrange
		rec := &txRecorder{}
		beginner := &fakeTxBeginner{}
		user := &txUserDeleter{
			rec: rec,
			findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
				if id != want.ID {
					t.Errorf("FindByID id = %q, want %q", id, want.ID)
				}
				return want, nil
			},
		}
		svc := newTxService(beginner, user,
			&txSessionDeleter{rec: rec},
			&txSubDeleter{rec: rec},
			&txItemStateDeleter{rec: rec},
			nil, nil, nil,
		)

		// Act
		got, err := svc.GetByID(context.Background(), want.ID)

		// Assert
		if err != nil {
			t.Fatalf("GetByID returned error: %v", err)
		}
		if got != want {
			t.Errorf("GetByID returned %+v, want %+v", got, want)
		}
		if beginner.beginCalled {
			t.Error("expected no transaction to be started for read-only lookup")
		}
	})
}

// TestService_GetByID_NotFound_ReturnsUserNotFoundError は repository が nil を
// 返したとき model.NewUserNotFoundError が返ることを検証する（Req 2.3）。
// レガシーパスと txBeginner パスの両方を確認する。
func TestService_GetByID_NotFound_ReturnsUserNotFoundError(t *testing.T) {
	t.Run("レガシーパスのとき repository が nil なら UserNotFound を返す", func(t *testing.T) {
		// Arrange
		userRepo := &mockUserRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
				return nil, nil
			},
		}
		svc := NewService(userRepo, nil, nil, nil)

		// Act
		got, err := svc.GetByID(context.Background(), "missing-user")

		// Assert
		if got != nil {
			t.Errorf("expected nil user, got %+v", got)
		}
		var apiErr *model.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != model.ErrCodeUserNotFound {
			t.Fatalf("expected UserNotFound error, got %v", err)
		}
	})

	t.Run("txBeginner パスのとき txUserDeleter が nil なら UserNotFound を返す", func(t *testing.T) {
		// Arrange
		rec := &txRecorder{}
		beginner := &fakeTxBeginner{}
		user := &txUserDeleter{
			rec: rec,
			findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
				return nil, nil
			},
		}
		svc := newTxService(beginner, user,
			&txSessionDeleter{rec: rec},
			&txSubDeleter{rec: rec},
			&txItemStateDeleter{rec: rec},
			nil, nil, nil,
		)

		// Act
		got, err := svc.GetByID(context.Background(), "missing-user")

		// Assert
		if got != nil {
			t.Errorf("expected nil user, got %+v", got)
		}
		var apiErr *model.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != model.ErrCodeUserNotFound {
			t.Fatalf("expected UserNotFound error, got %v", err)
		}
	})
}

// TestService_GetByID_RepoError_PropagatesError は repository がエラーを返したとき
// wrap された error が返ることを検証する。レガシーパスと txBeginner パスの両方で
// 同一挙動になることを subtest で確認する。
func TestService_GetByID_RepoError_PropagatesError(t *testing.T) {
	repoErr := errors.New("db connection lost")

	t.Run("レガシーパスのとき repository のエラーを wrap して返す", func(t *testing.T) {
		// Arrange
		userRepo := &mockUserRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
				return nil, repoErr
			},
		}
		svc := NewService(userRepo, nil, nil, nil)

		// Act
		got, err := svc.GetByID(context.Background(), "user-1")

		// Assert
		if got != nil {
			t.Errorf("expected nil user on error, got %+v", got)
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected error to wrap %v, got %v", repoErr, err)
		}
	})

	t.Run("txBeginner パスのとき txUserDeleter のエラーを wrap して返す", func(t *testing.T) {
		// Arrange
		rec := &txRecorder{}
		beginner := &fakeTxBeginner{}
		user := &txUserDeleter{
			rec: rec,
			findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
				return nil, repoErr
			},
		}
		svc := newTxService(beginner, user,
			&txSessionDeleter{rec: rec},
			&txSubDeleter{rec: rec},
			&txItemStateDeleter{rec: rec},
			nil, nil, nil,
		)

		// Act
		got, err := svc.GetByID(context.Background(), "user-1")

		// Assert
		if got != nil {
			t.Errorf("expected nil user on error, got %+v", got)
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected error to wrap %v, got %v", repoErr, err)
		}
	})
}

// TestService_Withdraw_Tx_NilNativeAuthDeletersSkip は native auth deleter が
// 両方 nil のとき、退会が従来どおり成功し（NFR 1.1 後方互換）、
// auth_codes / refresh_token_families の段がスキップされることを検証する
// （Issue #170 nil ガード）。
func TestService_Withdraw_Tx_NilNativeAuthDeletersSkip(t *testing.T) {
	// Arrange: native auth deleter を nil で構築
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		nil, // authCode deleter を注入しない（nil ガード経路を踏む）
		nil, // refreshToken deleter を注入しない
		nil, // Issue #216: passkey deleter も注入しないことで全 nil ガード経路を検証
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}
	if !beginner.committed {
		t.Error("expected commit when native auth deleters are nil")
	}
	// nil ガードにより auth_codes / refresh_token_families / passkey_credentials は order に現れない
	want := []string{"item_states", "subscriptions", "sessions", "user"}
	if len(rec.order) != len(want) {
		t.Fatalf("delete order = %v, want %v (native auth / passkey steps skipped)", rec.order, want)
	}
	for i := range want {
		if rec.order[i] != want[i] {
			t.Errorf("delete order[%d] = %q, want %q", i, rec.order[i], want[i])
		}
	}
}

// --- Issue #216: 退会時の passkey credential 削除のテスト ---

// TestService_Withdraw_Tx_PasskeyCredentialDeleterInvoked は passkey deleter が
// 注入されているとき、withdrawTx の passkey_credentials 段で当該 deleter が
// 呼ばれることを検証する（Req 7.1, 7.2）。
func TestService_Withdraw_Tx_PasskeyCredentialDeleterInvoked(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	passkeyDel := &txPasskeyCredentialDeleter{rec: rec}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		&txAuthCodeDeleter{rec: rec},
		&txRefreshTokenDeleter{rec: rec},
		passkeyDel,
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}
	if !beginner.committed {
		t.Error("expected transaction to be committed")
	}
	// passkey_credentials が order に含まれること（Req 7.1）
	foundPasskey := false
	for _, step := range rec.order {
		if step == "passkey_credentials" {
			foundPasskey = true
			break
		}
	}
	if !foundPasskey {
		t.Errorf("expected passkey_credentials in order, got %v", rec.order)
	}
	// 順序: sessions の直後・auth_codes の直前（Req 7.2 の同一 tx 内挿入位置）
	want := []string{
		"item_states", "subscriptions", "sessions",
		"passkey_credentials", "auth_codes", "refresh_token_families", "user",
	}
	if len(rec.order) != len(want) {
		t.Fatalf("delete order = %v, want %v", rec.order, want)
	}
	for i := range want {
		if rec.order[i] != want[i] {
			t.Errorf("delete order[%d] = %q, want %q", i, rec.order[i], want[i])
		}
	}
}

// TestService_Withdraw_Tx_RollsBackOnPasskeyCredentialDeleteError は
// passkey_credentials 削除が失敗したとき、退会全体が Rollback され後続の
// auth_codes / refresh_token_families / user 削除に到達しないことを検証する
// （Req 7.3: 削除失敗時は tx 全体を失敗させ、それまでの削除を確定しない）。
func TestService_Withdraw_Tx_RollsBackOnPasskeyCredentialDeleteError(t *testing.T) {
	// Arrange
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	deleteErr := errors.New("passkey credential delete failed")
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	authCodeDel := &txAuthCodeDeleter{rec: rec}
	refreshTokenDel := &txRefreshTokenDeleter{rec: rec}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		authCodeDel,
		refreshTokenDel,
		&txPasskeyCredentialDeleter{rec: rec, err: deleteErr},
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err == nil {
		t.Fatal("expected error from failing passkey credential delete, got nil")
	}
	if !errors.Is(err, deleteErr) {
		t.Errorf("expected error to wrap %v, got %v", deleteErr, err)
	}
	if beginner.committed {
		t.Error("expected no commit when passkey credential delete fails")
	}
	if !beginner.rolledBack {
		t.Error("expected rollback when passkey credential delete fails")
	}
	// passkey_credentials 失敗後は後続の auth_codes / refresh_token_families / user は呼ばれない
	// （fail-fast / Req 7.3 の「それまでの削除を確定しない」の順序面担保）。
	for _, step := range rec.order {
		if step == "auth_codes" {
			t.Error("expected auth_codes delete NOT to be called after passkey failure")
		}
		if step == "refresh_token_families" {
			t.Error("expected refresh_token_families delete NOT to be called after passkey failure")
		}
		if step == "user" {
			t.Error("expected user delete NOT to be called after passkey failure")
		}
	}
	if user.deleteCalled {
		t.Error("expected user delete NOT to be called after passkey failure")
	}
}

// TestService_Withdraw_Tx_NilPasskeyDeleterSkip は passkey deleter が nil のとき、
// 退会が従来どおり成功し（NFR 2.2 後方互換）、passkey_credentials 段だけが
// スキップされ、既存 native auth 段は引き続き呼ばれることを検証する
// （Issue #216 nil ガード / env 未設定環境の互換維持）。
func TestService_Withdraw_Tx_NilPasskeyDeleterSkip(t *testing.T) {
	// Arrange: passkey deleter だけ nil、他の deleter は全て非 nil
	rec := &txRecorder{}
	beginner := &fakeTxBeginner{}
	user := &txUserDeleter{
		rec: rec,
		findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
	}
	svc := newTxService(beginner,
		user,
		&txSessionDeleter{rec: rec},
		&txSubDeleter{rec: rec},
		&txItemStateDeleter{rec: rec},
		&txAuthCodeDeleter{rec: rec},
		&txRefreshTokenDeleter{rec: rec},
		nil, // Issue #216: passkey deleter を注入しない（nil ガード経路を踏む）
	)

	// Act
	err := svc.Withdraw(context.Background(), "user-1")

	// Assert
	if err != nil {
		t.Fatalf("Withdraw returned error: %v", err)
	}
	if !beginner.committed {
		t.Error("expected commit when passkey deleter is nil")
	}
	// passkey_credentials が order に含まれず、既存 4 段 + native auth 2 段が呼ばれる
	want := []string{"item_states", "subscriptions", "sessions", "auth_codes", "refresh_token_families", "user"}
	if len(rec.order) != len(want) {
		t.Fatalf("delete order = %v, want %v (passkey step skipped)", rec.order, want)
	}
	for i := range want {
		if rec.order[i] != want[i] {
			t.Errorf("delete order[%d] = %q, want %q", i, rec.order[i], want[i])
		}
	}
}
