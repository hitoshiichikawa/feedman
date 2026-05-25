package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// DBTX は *sql.DB と *sql.Tx が共通して満たすクエリ実行インターフェース。
// リポジトリのデータ操作をトランザクション対応（transaction-aware）にするために用いる。
// 非トランザクション時は *sql.DB を、トランザクション時は *sql.Tx を渡す。
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// 静的検査: *sql.DB と *sql.Tx がともに DBTX を満たすこと。
var (
	_ DBTX = (*sql.DB)(nil)
	_ DBTX = (*sql.Tx)(nil)
)

// SQLTx は *sql.Tx をラップし、共有トランザクションのクエリ実行ハンドルと
// 確定／取り消し操作を提供する。
type SQLTx struct {
	tx *sql.Tx
}

// Querier はこのトランザクション上でクエリを実行するための DBTX を返す。
func (t *SQLTx) Querier() DBTX {
	return t.tx
}

// Commit はトランザクションを確定する。
func (t *SQLTx) Commit() error {
	return t.tx.Commit()
}

// Rollback はトランザクションを取り消す。
// すでに確定済みの場合、database/sql は sql.ErrTxDone を返す。
func (t *SQLTx) Rollback() error {
	return t.tx.Rollback()
}

// SQLTxBeginner は *sql.DB をラップし、トランザクションを開始する。
type SQLTxBeginner struct {
	db *sql.DB
}

// NewSQLTxBeginner は *sql.DB から SQLTxBeginner を生成する。
func NewSQLTxBeginner(db *sql.DB) *SQLTxBeginner {
	return &SQLTxBeginner{db: db}
}

// BeginTx は新しいトランザクションを開始し、SQLTx でラップして返す。
func (b *SQLTxBeginner) BeginTx(ctx context.Context) (*SQLTx, error) {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &SQLTx{tx: tx}, nil
}
