package database

import (
	"testing"
)

// TestOpen_SetsMaxOpenConns はOpenが返す*sql.DBに同時接続数の上限が設定されていることを検証する。
// db.Stats().MaxOpenConnectionsは公開取得可能なため、実物に最も近い形で
// MaxOpenConnsが設定値どおりであることを確認できる。
// 対応AC: Requirement 1.1 / 1.2（有限の正の上限値を設定する）。
func TestOpen_SetsMaxOpenConns(t *testing.T) {
	t.Run("Openが返すDBのMaxOpenConnectionsがmaxOpenConnsと一致するとき設定が適用されている", func(t *testing.T) {
		// Arrange / Act
		db, err := Open("postgres://user:pass@localhost:5432/feedman?sslmode=disable")
		if err != nil {
			t.Fatalf("Open returned unexpected error: %v", err)
		}
		defer db.Close()

		// Assert
		if got := db.Stats().MaxOpenConnections; got != maxOpenConns {
			t.Errorf("MaxOpenConnections = %d, want %d", got, maxOpenConns)
		}
	})
}

// TestPoolConstants_AreFinitePositive はプール設定定数の不変条件を検証する。
// MaxIdleConns / ConnMaxLifetimeには公開getterが無いため、
// パッケージ内テストから定数の不変条件として検証する。
func TestPoolConstants_AreFinitePositive(t *testing.T) {
	// 対応AC: Requirement 1.2（同時接続数の上限は無制限0以外の正の有限値）。
	t.Run("maxOpenConnsが正の有限値のとき接続枯渇耐性の前提を満たす", func(t *testing.T) {
		if maxOpenConns <= 0 {
			t.Errorf("maxOpenConns = %d, want > 0", maxOpenConns)
		}
	})

	// 対応AC: Requirement 2.1 / 2.2（アイドル接続数は有限かつ同時接続数の上限以下）。
	t.Run("maxIdleConnsが正かつmaxOpenConns以下のときアイドル上限の不変条件を満たす", func(t *testing.T) {
		if maxIdleConns <= 0 {
			t.Errorf("maxIdleConns = %d, want > 0", maxIdleConns)
		}
		if maxIdleConns > maxOpenConns {
			t.Errorf("maxIdleConns = %d, want <= maxOpenConns(%d)", maxIdleConns, maxOpenConns)
		}
	})

	// 対応AC: Requirement 3.1 / 3.2（接続の最大寿命は正の有限の時間値）。
	t.Run("connMaxLifetimeが正の時間値のとき接続寿命上限の不変条件を満たす", func(t *testing.T) {
		if connMaxLifetime <= 0 {
			t.Errorf("connMaxLifetime = %v, want > 0", connMaxLifetime)
		}
	})
}

// TestPoolConstants_TwoProcessSumWithinMaxConnections は
// api / worker 2プロセス合算の同時接続数上限がPostgreSQL max_connections(既定100)を
// 超えないことを定数不変条件として検証する。
// 対応AC: Requirement 1.3 / NFR 1.1。
func TestPoolConstants_TwoProcessSumWithinMaxConnections(t *testing.T) {
	t.Run("2プロセス合算のMaxOpenConnsが100以下のとき接続枯渇を引き起こさない", func(t *testing.T) {
		const postgresMaxConnections = 100
		if 2*maxOpenConns > postgresMaxConnections {
			t.Errorf("2 * maxOpenConns = %d, want <= %d", 2*maxOpenConns, postgresMaxConnections)
		}
	})
}

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
