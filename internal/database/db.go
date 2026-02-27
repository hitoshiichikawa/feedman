package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Open はPostgreSQLデータベース接続を開く。
// databaseURLはPostgreSQLの接続URLを指定する（例: "postgres://user:pass@host:5432/dbname?sslmode=disable"）。
// sql.Openは接続を試行しないため、実際の接続確認にはdb.Ping()を使用すること。
func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return db, nil
}
