package database

import (
	"testing"
)

// TestOpen_WithEmptyURL_ReturnsDB はsql.Openは接続を試行しないため、
// 空URLでもDBオブジェクトが返ることを検証する。
// 実際の接続確認にはPingが必要。
func TestOpen_ReturnsDBForAnyURL(t *testing.T) {
	// sql.Openはドライバ名が正しければURLフォーマットに関わらず成功する。
	// 実際の接続検証はdb.Ping()で行う。
	db, err := Open("postgres://invalid")
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
	defer db.Close()
}

// TestOpen_WithValidURL_ReturnsDB は有効なDB URLでDB接続が返ることを検証する。
// 注意: 実際のDB接続は行わず、sql.Open自体がURLフォーマットを受け入れることを確認する。
func TestOpen_WithValidURL_ReturnsDB(t *testing.T) {
	// sql.Open自体は接続を試行しないため、フォーマットが正しければ成功する。
	// 実際のDB接続はPingで検証する必要があるが、ここではOpen関数の基本動作のみをテストする。
	db, err := Open("postgres://user:pass@localhost:5432/feedman?sslmode=disable")
	if err != nil {
		t.Fatalf("Open with valid URL returned error: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
	defer db.Close()
}
