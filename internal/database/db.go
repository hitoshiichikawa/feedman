package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// 接続プール設定の名前付き定数。
// api / worker の 2 プロセスが同一の Open を呼び出してそれぞれプールを保持するため、
// 各プロセスの同時接続数上限を合算しても PostgreSQL の max_connections（既定 100）を
// 超えない値とする。
const (
	// maxOpenConns は1プロセスあたりの同時接続数の上限。
	// api 25 + worker 25 = 50 ≤ max_connections(100) とし、
	// マイグレーションや管理接続の余裕も確保する。
	maxOpenConns = 25

	// maxIdleConns はアイドル状態で保持する接続数の上限。
	// 利用されない接続を際限なく保持しないよう maxOpenConns 以下に抑える。
	maxIdleConns = 10

	// connMaxLifetime は接続の最大寿命。
	// 長寿命接続をこの時間で再確立し、ネットワーク機器や DB 側のタイムアウトによる
	// 断線が顕在化しないようにする。
	connMaxLifetime = 5 * time.Minute
)

// Open はPostgreSQLデータベース接続を開く。
// databaseURLはPostgreSQLの接続URLを指定する（例: "postgres://user:pass@host:5432/dbname?sslmode=disable"）。
// sql.Openは接続を試行しないため、実際の接続確認にはdb.Ping()を使用すること。
// 返される*sql.DBには接続プールの上限（MaxOpenConns / MaxIdleConns / ConnMaxLifetime）が設定済み。
func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	return db, nil
}
