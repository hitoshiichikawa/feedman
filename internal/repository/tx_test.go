package repository

import (
	"database/sql"
	"testing"
)

// TestDBTX_SatisfiedBySQLTypes は *sql.DB と *sql.Tx がともに DBTX を満たすことを検証する。
func TestDBTX_SatisfiedBySQLTypes(t *testing.T) {
	var _ DBTX = (*sql.DB)(nil)
	var _ DBTX = (*sql.Tx)(nil)
}

// TestNewSQLTxBeginner_Initializes は SQLTxBeginner が初期化できることを検証する。
func TestNewSQLTxBeginner_Initializes(t *testing.T) {
	b := NewSQLTxBeginner(nil)
	if b == nil {
		t.Fatal("expected non-nil SQLTxBeginner")
	}
}

// TestSQLTx_QuerierReturnsUnderlyingTx は SQLTx.Querier が
// ラップしている *sql.Tx を DBTX として返すことを検証する。
func TestSQLTx_QuerierReturnsUnderlyingTx(t *testing.T) {
	tx := &sql.Tx{}
	wrapped := &SQLTx{tx: tx}

	q := wrapped.Querier()
	if q == nil {
		t.Fatal("expected non-nil querier")
	}
	// Querier が返す DBTX はラップ元の *sql.Tx と同一であること。
	if got, ok := q.(*sql.Tx); !ok || got != tx {
		t.Errorf("Querier() = %v, want underlying *sql.Tx %v", q, tx)
	}
}

// TestPostgresAuthCodeRepo_ImplementsInterface は PostgresAuthCodeRepo が
// AuthCodeRepository を満たすことを compile-time に検証する（Req 4.3）。
// 各実装ファイル内の `var _ AuthCodeRepository = (*PostgresAuthCodeRepo)(nil)` と
// 同等の保証を集約箇所（tx_test.go）にも追加することで、interface drift を 2 箇所で
// 検出できるようにする。
func TestPostgresAuthCodeRepo_ImplementsInterface(t *testing.T) {
	var _ AuthCodeRepository = (*PostgresAuthCodeRepo)(nil)
}

// TestPostgresRefreshTokenRepo_ImplementsInterface は PostgresRefreshTokenRepo が
// RefreshTokenRepository を満たすことを compile-time に検証する（Req 4.3）。
func TestPostgresRefreshTokenRepo_ImplementsInterface(t *testing.T) {
	var _ RefreshTokenRepository = (*PostgresRefreshTokenRepo)(nil)
}
