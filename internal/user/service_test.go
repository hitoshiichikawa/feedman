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

// newTxService はトランザクション対応の Service を組み立てるテストヘルパ。
func newTxService(
	beginner *fakeTxBeginner,
	user *txUserDeleter,
	session *txSessionDeleter,
	sub *txSubDeleter,
	state *txItemStateDeleter,
) *Service {
	return NewServiceWithTx(beginner, user, session, sub, state)
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
// 子→親の順序で削除されることを検証する（AC 1.1 / 1.4 / NFR 2.1）。
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
	want := []string{"item_states", "subscriptions", "sessions", "user"}
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
