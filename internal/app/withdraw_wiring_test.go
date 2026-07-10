package app

import (
	"testing"

	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/user"
)

// fakeUserTx は *repository.SQLTx 以外の user.Tx 実装。
// querierFromTx が予期しない型を拒否することを検証するために用いる。
type fakeUserTx struct{}

func (fakeUserTx) Commit() error   { return nil }
func (fakeUserTx) Rollback() error { return nil }

// TestQuerierFromTx_RejectsUnexpectedType は user.Tx の実体が
// *repository.SQLTx でない場合にエラーを返すことを検証する。
func TestQuerierFromTx_RejectsUnexpectedType(t *testing.T) {
	// Arrange
	var tx user.Tx = fakeUserTx{}

	// Act
	_, err := querierFromTx(tx)

	// Assert
	if err == nil {
		t.Fatal("expected error for unexpected tx type, got nil")
	}
}

// TestNewTxUserService_Constructs はトランザクション対応の退会サービスが
// 配線できることを検証する（nil リポジトリでも構築自体は成功する）。
// Issue #170: native auth deleter（auth_code / refresh_token）の repository を
// 末尾に追加した呼び出し signature を検証する。
func TestNewTxUserService_Constructs(t *testing.T) {
	beginner := repository.NewSQLTxBeginner(nil)
	svc := newTxUserService(
		beginner,
		repository.NewPostgresUserRepo(nil),
		repository.NewPostgresSessionRepo(nil),
		repository.NewPostgresSubscriptionRepo(nil),
		repository.NewPostgresItemStateRepo(nil),
		repository.NewPostgresAuthCodeRepo(nil),
		repository.NewPostgresRefreshTokenRepo(nil),
	)
	if svc == nil {
		t.Fatal("expected non-nil user.Service")
	}
}

// TestWithdrawWiringAdapters_SatisfyInterfaces は各アダプタが
// user パッケージのトランザクション対応インターフェースを満たすことを検証する。
// Issue #170: txAuthCodeDeleterAdapter / txRefreshTokenDeleterAdapter も対象。
func TestWithdrawWiringAdapters_SatisfyInterfaces(t *testing.T) {
	var _ user.TxBeginner = (*txBeginnerAdapter)(nil)
	var _ user.TxItemStateDeleter = (*txItemStateDeleterAdapter)(nil)
	var _ user.TxSubscriptionDeleter = (*txSubscriptionDeleterAdapter)(nil)
	var _ user.TxSessionDeleter = (*txSessionDeleterAdapter)(nil)
	var _ user.TxUserDeleter = (*txUserDeleterAdapter)(nil)
	var _ user.TxAuthCodeDeleter = (*txAuthCodeDeleterAdapter)(nil)
	var _ user.TxRefreshTokenDeleter = (*txRefreshTokenDeleterAdapter)(nil)
}
