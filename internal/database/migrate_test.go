package database

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// testDatabaseURL はテスト用のデータベースURLを返す。
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、
// 未設定の場合はdocker-compose上のPostgreSQLを想定したデフォルト値を返す。
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupTestDB はテスト用データベースを準備する。
// テスト実行前に全テーブルをドロップしてクリーンな状態にする。
func setupTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()

	dbURL := testDatabaseURL(t)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	// クリーンアップ: 既存のテーブルとマイグレーション履歴を削除
	cleanupSQL := `
		DROP TABLE IF EXISTS sessions CASCADE;
		DROP TABLE IF EXISTS user_settings CASCADE;
		DROP TABLE IF EXISTS item_states CASCADE;
		DROP TABLE IF EXISTS subscriptions CASCADE;
		DROP TABLE IF EXISTS items CASCADE;
		DROP TABLE IF EXISTS feeds CASCADE;
		DROP TABLE IF EXISTS identities CASCADE;
		DROP TABLE IF EXISTS users CASCADE;
		DROP TABLE IF EXISTS schema_migrations CASCADE;
	`
	if _, err := db.Exec(cleanupSQL); err != nil {
		t.Fatalf("クリーンアップに失敗: %v", err)
	}

	return db, dbURL
}

func TestRunMigrations_Up(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	// マイグレーション実行
	err := RunMigrations(dbURL)
	if err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	// すべてのテーブルが作成されたことを確認
	expectedTables := []string{
		"users",
		"identities",
		"feeds",
		"items",
		"subscriptions",
		"item_states",
		"user_settings",
		"sessions",
	}

	for _, table := range expectedTables {
		t.Run("テーブル存在確認_"+table, func(t *testing.T) {
			var exists bool
			err := db.QueryRow(
				"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
				table,
			).Scan(&exists)
			if err != nil {
				t.Fatalf("テーブル存在確認クエリに失敗: %v", err)
			}
			if !exists {
				t.Errorf("テーブル %q が存在しません", table)
			}
		})
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	// 1回目のマイグレーション
	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("1回目のマイグレーション実行に失敗: %v", err)
	}

	// 2回目のマイグレーション（冪等性確認）
	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("2回目のマイグレーション実行に失敗（冪等性の問題）: %v", err)
	}
}

func TestMigrations_UpAndDown(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	m, err := NewMigrator(dbURL)
	if err != nil {
		t.Fatalf("Migrator生成に失敗: %v", err)
	}
	defer m.Close()

	// Up
	if err := m.Up(); err != nil {
		t.Fatalf("Up マイグレーション実行に失敗: %v", err)
	}

	// テーブルが存在することを確認
	var count int
	err = db.QueryRow(
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('users','identities','feeds','items','subscriptions','item_states','user_settings','sessions')",
	).Scan(&count)
	if err != nil {
		t.Fatalf("テーブルカウント取得に失敗: %v", err)
	}
	if count != 8 {
		t.Errorf("Up後のテーブル数が不正: got %d, want 8", count)
	}

	// Down
	if err := m.Down(); err != nil {
		t.Fatalf("Down マイグレーション実行に失敗: %v", err)
	}

	// テーブルが全て削除されたことを確認
	err = db.QueryRow(
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('users','identities','feeds','items','subscriptions','item_states','user_settings','sessions')",
	).Scan(&count)
	if err != nil {
		t.Fatalf("テーブルカウント取得に失敗: %v", err)
	}
	if count != 0 {
		t.Errorf("Down後のテーブル数が不正: got %d, want 0", count)
	}
}

// TestUsersTable はusersテーブルのカラム構成を検証する。
func TestUsersTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	// カラム定義の検証
	expectedColumns := map[string]string{
		"id":         "uuid",
		"email":      "character varying",
		"name":       "character varying",
		"created_at": "timestamp with time zone",
		"updated_at": "timestamp with time zone",
	}
	assertTableColumns(t, db, "users", expectedColumns)

	// NOT NULL制約の検証
	assertNotNull(t, db, "users", []string{"id", "email", "name", "created_at", "updated_at"})

	// PKの検証
	assertPrimaryKey(t, db, "users", "id")
}

// TestIdentitiesTable はidentitiesテーブルのカラム構成と制約を検証する。
func TestIdentitiesTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":               "uuid",
		"user_id":          "uuid",
		"provider":         "character varying",
		"provider_user_id": "character varying",
		"created_at":       "timestamp with time zone",
	}
	assertTableColumns(t, db, "identities", expectedColumns)

	assertNotNull(t, db, "identities", []string{"id", "user_id", "provider", "provider_user_id", "created_at"})
	assertPrimaryKey(t, db, "identities", "id")
	assertUniqueConstraint(t, db, "identities", []string{"provider", "provider_user_id"})
	assertForeignKey(t, db, "identities", "user_id", "users", "id", "CASCADE")
	assertIndexExists(t, db, "identities", "user_id")
}

// TestFeedsTable はfeedsテーブルのカラム構成と制約を検証する。
func TestFeedsTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":                 "uuid",
		"feed_url":           "text",
		"site_url":           "text",
		"title":              "character varying",
		"favicon_data":       "bytea",
		"favicon_mime":       "character varying",
		"etag":               "character varying",
		"last_modified":      "character varying",
		"fetch_status":       "character varying",
		"consecutive_errors": "integer",
		"error_message":      "text",
		"next_fetch_at":      "timestamp with time zone",
		"created_at":         "timestamp with time zone",
		"updated_at":         "timestamp with time zone",
	}
	assertTableColumns(t, db, "feeds", expectedColumns)

	assertNotNull(t, db, "feeds", []string{"id", "feed_url", "title", "fetch_status", "consecutive_errors", "next_fetch_at", "created_at", "updated_at"})
	assertPrimaryKey(t, db, "feeds", "id")
	assertUniqueConstraint(t, db, "feeds", []string{"feed_url"})

	// 部分インデックスの確認: fetch_status = 'active' の next_fetch_at
	assertPartialIndexExists(t, db, "feeds", "next_fetch_at", "fetch_status")
}

// TestItemsTable はitemsテーブルのカラム構成と制約を検証する。
func TestItemsTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":                "uuid",
		"feed_id":           "uuid",
		"guid_or_id":        "character varying",
		"link":              "text",
		"title":             "character varying",
		"content":           "text",
		"summary":           "text",
		"author":            "character varying",
		"published_at":      "timestamp with time zone",
		"is_date_estimated": "boolean",
		"fetched_at":        "timestamp with time zone",
		"content_hash":      "character varying",
		"hatebu_count":      "integer",
		"hatebu_fetched_at": "timestamp with time zone",
		"created_at":        "timestamp with time zone",
		"updated_at":        "timestamp with time zone",
	}
	assertTableColumns(t, db, "items", expectedColumns)

	assertNotNull(t, db, "items", []string{"id", "feed_id", "title", "is_date_estimated", "fetched_at", "hatebu_count", "created_at", "updated_at"})
	assertPrimaryKey(t, db, "items", "id")
	assertForeignKey(t, db, "items", "feed_id", "feeds", "id", "CASCADE")

	// 部分ユニーク制約: (feed_id, guid_or_id) WHERE guid_or_id IS NOT NULL
	assertPartialUniqueIndex(t, db, "items", []string{"feed_id", "guid_or_id"}, "guid_or_id")

	// 複合インデックス
	assertIndexExists(t, db, "items", "feed_id")
	assertIndexExists(t, db, "items", "published_at")
}

// TestSubscriptionsTable はsubscriptionsテーブルのカラム構成と制約を検証する。
func TestSubscriptionsTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":                     "uuid",
		"user_id":                "uuid",
		"feed_id":                "uuid",
		"fetch_interval_minutes": "integer",
		"created_at":             "timestamp with time zone",
		"updated_at":             "timestamp with time zone",
	}
	assertTableColumns(t, db, "subscriptions", expectedColumns)

	assertNotNull(t, db, "subscriptions", []string{"id", "user_id", "feed_id", "fetch_interval_minutes", "created_at", "updated_at"})
	assertPrimaryKey(t, db, "subscriptions", "id")
	assertUniqueConstraint(t, db, "subscriptions", []string{"user_id", "feed_id"})
	assertForeignKey(t, db, "subscriptions", "user_id", "users", "id", "CASCADE")
	assertForeignKey(t, db, "subscriptions", "feed_id", "feeds", "id", "CASCADE")
	assertIndexExists(t, db, "subscriptions", "user_id")
	assertIndexExists(t, db, "subscriptions", "feed_id")
}

// TestItemStatesTable はitem_statesテーブルのカラム構成と制約を検証する。
func TestItemStatesTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":         "uuid",
		"user_id":    "uuid",
		"item_id":    "uuid",
		"is_read":    "boolean",
		"is_starred": "boolean",
		"updated_at": "timestamp with time zone",
	}
	assertTableColumns(t, db, "item_states", expectedColumns)

	assertNotNull(t, db, "item_states", []string{"id", "user_id", "item_id", "is_read", "is_starred", "updated_at"})
	assertPrimaryKey(t, db, "item_states", "id")
	assertUniqueConstraint(t, db, "item_states", []string{"user_id", "item_id"})
	assertForeignKey(t, db, "item_states", "user_id", "users", "id", "CASCADE")
	assertForeignKey(t, db, "item_states", "item_id", "items", "id", "CASCADE")

	// 部分インデックス: is_read = false
	assertPartialIndexOnBool(t, db, "item_states", "is_read", "false")
	// 部分インデックス: is_starred = true
	assertPartialIndexOnBool(t, db, "item_states", "is_starred", "true")
}

// TestUserSettingsTable はuser_settingsテーブルのカラム構成と制約を検証する。
func TestUserSettingsTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":         "uuid",
		"user_id":    "uuid",
		"theme":      "character varying",
		"updated_at": "timestamp with time zone",
	}
	assertTableColumns(t, db, "user_settings", expectedColumns)

	assertNotNull(t, db, "user_settings", []string{"id", "user_id", "theme", "updated_at"})
	assertPrimaryKey(t, db, "user_settings", "id")
	assertUniqueConstraint(t, db, "user_settings", []string{"user_id"})
	assertForeignKey(t, db, "user_settings", "user_id", "users", "id", "CASCADE")
}

// TestSessionsTable はsessionsテーブルのカラム構成と制約を検証する。
func TestSessionsTable(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	expectedColumns := map[string]string{
		"id":         "character varying",
		"user_id":    "uuid",
		"data":       "bytea",
		"expires_at": "timestamp with time zone",
		"created_at": "timestamp with time zone",
	}
	assertTableColumns(t, db, "sessions", expectedColumns)

	assertNotNull(t, db, "sessions", []string{"id", "user_id", "data", "expires_at", "created_at"})
	assertPrimaryKey(t, db, "sessions", "id")
	assertForeignKey(t, db, "sessions", "user_id", "users", "id", "CASCADE")
	assertIndexExists(t, db, "sessions", "expires_at")
	assertIndexExists(t, db, "sessions", "user_id")
}

// TestCascadeDelete は外部キーのCASCADE削除が正しく動作するか検証する。
func TestCascadeDelete(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	// テストデータ挿入
	var userID string
	err := db.QueryRow(`INSERT INTO users (email, name) VALUES ('test@example.com', 'Test User') RETURNING id`).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}

	// identity作成
	_, err = db.Exec(`INSERT INTO identities (user_id, provider, provider_user_id) VALUES ($1, 'google', 'google-123')`, userID)
	if err != nil {
		t.Fatalf("identity挿入に失敗: %v", err)
	}

	// feed作成
	var feedID string
	err = db.QueryRow(`INSERT INTO feeds (feed_url, title) VALUES ('https://example.com/feed.xml', 'Test Feed') RETURNING id`).Scan(&feedID)
	if err != nil {
		t.Fatalf("フィード挿入に失敗: %v", err)
	}

	// item作成
	var itemID string
	err = db.QueryRow(`INSERT INTO items (feed_id, title) VALUES ($1, 'Test Item') RETURNING id`, feedID).Scan(&itemID)
	if err != nil {
		t.Fatalf("記事挿入に失敗: %v", err)
	}

	// subscription作成
	_, err = db.Exec(`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2)`, userID, feedID)
	if err != nil {
		t.Fatalf("購読挿入に失敗: %v", err)
	}

	// item_state作成
	_, err = db.Exec(`INSERT INTO item_states (user_id, item_id) VALUES ($1, $2)`, userID, itemID)
	if err != nil {
		t.Fatalf("記事状態挿入に失敗: %v", err)
	}

	// user_settings作成
	_, err = db.Exec(`INSERT INTO user_settings (user_id) VALUES ($1)`, userID)
	if err != nil {
		t.Fatalf("ユーザー設定挿入に失敗: %v", err)
	}

	// session作成
	_, err = db.Exec(`INSERT INTO sessions (id, user_id, data, expires_at) VALUES ('session-1', $1, '\x00', now() + interval '1 day')`, userID)
	if err != nil {
		t.Fatalf("セッション挿入に失敗: %v", err)
	}

	t.Run("ユーザー削除でidentities,subscriptions,item_states,user_settings,sessionsがCASCADE削除される", func(t *testing.T) {
		_, err := db.Exec(`DELETE FROM users WHERE id = $1`, userID)
		if err != nil {
			t.Fatalf("ユーザー削除に失敗: %v", err)
		}

		// CASCADE削除の確認
		cascadeTargets := []struct {
			table string
			col   string
		}{
			{"identities", "user_id"},
			{"subscriptions", "user_id"},
			{"item_states", "user_id"},
			{"user_settings", "user_id"},
			{"sessions", "user_id"},
		}

		for _, target := range cascadeTargets {
			var count int
			err := db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1", target.table, target.col), userID).Scan(&count)
			if err != nil {
				t.Fatalf("%s テーブルのカウント取得に失敗: %v", target.table, err)
			}
			if count != 0 {
				t.Errorf("%s テーブルにレコードが残存: count=%d", target.table, count)
			}
		}
	})

	t.Run("フィード削除でitems,subscriptionsがCASCADE削除される", func(t *testing.T) {
		// 残存確認（feedはまだ存在する）
		var feedCount int
		db.QueryRow("SELECT count(*) FROM feeds WHERE id = $1", feedID).Scan(&feedCount)
		if feedCount == 0 {
			t.Skip("フィードが既に削除されています")
		}

		_, err := db.Exec(`DELETE FROM feeds WHERE id = $1`, feedID)
		if err != nil {
			t.Fatalf("フィード削除に失敗: %v", err)
		}

		var itemCount int
		db.QueryRow("SELECT count(*) FROM items WHERE feed_id = $1", feedID).Scan(&itemCount)
		if itemCount != 0 {
			t.Errorf("items テーブルにレコードが残存: count=%d", itemCount)
		}
	})
}

// TestDefaultValues はデフォルト値が正しく設定されるか検証する。
func TestDefaultValues(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	t.Run("feeds_fetch_status_default_active", func(t *testing.T) {
		var feedID string
		err := db.QueryRow(`INSERT INTO feeds (feed_url, title) VALUES ('https://example.com/feed', 'Test') RETURNING id`).Scan(&feedID)
		if err != nil {
			t.Fatalf("フィード挿入に失敗: %v", err)
		}

		var fetchStatus string
		var consecutiveErrors int
		err = db.QueryRow(`SELECT fetch_status, consecutive_errors FROM feeds WHERE id = $1`, feedID).Scan(&fetchStatus, &consecutiveErrors)
		if err != nil {
			t.Fatalf("フィード取得に失敗: %v", err)
		}
		if fetchStatus != "active" {
			t.Errorf("fetch_statusのデフォルト値が不正: got %q, want %q", fetchStatus, "active")
		}
		if consecutiveErrors != 0 {
			t.Errorf("consecutive_errorsのデフォルト値が不正: got %d, want 0", consecutiveErrors)
		}
	})

	t.Run("items_defaults", func(t *testing.T) {
		// まずfeedが必要
		var feedID string
		db.QueryRow(`SELECT id FROM feeds LIMIT 1`).Scan(&feedID)

		var itemID string
		err := db.QueryRow(`INSERT INTO items (feed_id, title) VALUES ($1, 'Test Item') RETURNING id`, feedID).Scan(&itemID)
		if err != nil {
			t.Fatalf("記事挿入に失敗: %v", err)
		}

		var isDateEstimated bool
		var hatebuCount int
		err = db.QueryRow(`SELECT is_date_estimated, hatebu_count FROM items WHERE id = $1`, itemID).Scan(&isDateEstimated, &hatebuCount)
		if err != nil {
			t.Fatalf("記事取得に失敗: %v", err)
		}
		if isDateEstimated != false {
			t.Errorf("is_date_estimatedのデフォルト値が不正: got %v, want false", isDateEstimated)
		}
		if hatebuCount != 0 {
			t.Errorf("hatebu_countのデフォルト値が不正: got %d, want 0", hatebuCount)
		}
	})

	t.Run("item_states_defaults", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('default@test.com', 'Default') RETURNING id`).Scan(&userID)

		var feedID string
		db.QueryRow(`SELECT id FROM feeds LIMIT 1`).Scan(&feedID)

		var itemID string
		db.QueryRow(`SELECT id FROM items LIMIT 1`).Scan(&itemID)

		var stateID string
		err := db.QueryRow(`INSERT INTO item_states (user_id, item_id) VALUES ($1, $2) RETURNING id`, userID, itemID).Scan(&stateID)
		if err != nil {
			t.Fatalf("記事状態挿入に失敗: %v", err)
		}

		var isRead, isStarred bool
		err = db.QueryRow(`SELECT is_read, is_starred FROM item_states WHERE id = $1`, stateID).Scan(&isRead, &isStarred)
		if err != nil {
			t.Fatalf("記事状態取得に失敗: %v", err)
		}
		if isRead != false {
			t.Errorf("is_readのデフォルト値が不正: got %v, want false", isRead)
		}
		if isStarred != false {
			t.Errorf("is_starredのデフォルト値が不正: got %v, want false", isStarred)
		}
	})

	t.Run("user_settings_theme_default_light", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('theme@test.com', 'Theme') RETURNING id`).Scan(&userID)

		var settingsID string
		err := db.QueryRow(`INSERT INTO user_settings (user_id) VALUES ($1) RETURNING id`, userID).Scan(&settingsID)
		if err != nil {
			t.Fatalf("ユーザー設定挿入に失敗: %v", err)
		}

		var theme string
		err = db.QueryRow(`SELECT theme FROM user_settings WHERE id = $1`, settingsID).Scan(&theme)
		if err != nil {
			t.Fatalf("ユーザー設定取得に失敗: %v", err)
		}
		if theme != "light" {
			t.Errorf("themeのデフォルト値が不正: got %q, want %q", theme, "light")
		}
	})

	t.Run("subscriptions_fetch_interval_minutes_default_60", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('sub@test.com', 'Sub') RETURNING id`).Scan(&userID)

		var feedID string
		db.QueryRow(`SELECT id FROM feeds LIMIT 1`).Scan(&feedID)

		var subID string
		err := db.QueryRow(`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2) RETURNING id`, userID, feedID).Scan(&subID)
		if err != nil {
			t.Fatalf("購読挿入に失敗: %v", err)
		}

		var interval int
		err = db.QueryRow(`SELECT fetch_interval_minutes FROM subscriptions WHERE id = $1`, subID).Scan(&interval)
		if err != nil {
			t.Fatalf("購読取得に失敗: %v", err)
		}
		if interval != 60 {
			t.Errorf("fetch_interval_minutesのデフォルト値が不正: got %d, want 60", interval)
		}
	})
}

// TestUniqueConstraints はユニーク制約が正しく動作するか検証する。
func TestUniqueConstraints(t *testing.T) {
	db, dbURL := setupTestDB(t)
	defer db.Close()

	if err := RunMigrations(dbURL); err != nil {
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	t.Run("identities_provider_provider_user_id_unique", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('unique1@test.com', 'Unique1') RETURNING id`).Scan(&userID)

		_, err := db.Exec(`INSERT INTO identities (user_id, provider, provider_user_id) VALUES ($1, 'google', 'gid-1')`, userID)
		if err != nil {
			t.Fatalf("1件目のidentity挿入に失敗: %v", err)
		}

		// 同じ (provider, provider_user_id) で挿入するとエラーになるべき
		_, err = db.Exec(`INSERT INTO identities (user_id, provider, provider_user_id) VALUES ($1, 'google', 'gid-1')`, userID)
		if err == nil {
			t.Error("重複するidentityの挿入がエラーにならなかった")
		}
	})

	t.Run("feeds_feed_url_unique", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO feeds (feed_url, title) VALUES ('https://unique.example.com/feed', 'Feed1')`)
		if err != nil {
			t.Fatalf("1件目のフィード挿入に失敗: %v", err)
		}

		_, err = db.Exec(`INSERT INTO feeds (feed_url, title) VALUES ('https://unique.example.com/feed', 'Feed2')`)
		if err == nil {
			t.Error("重複するfeed_urlの挿入がエラーにならなかった")
		}
	})

	t.Run("subscriptions_user_feed_unique", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('unique2@test.com', 'Unique2') RETURNING id`).Scan(&userID)

		var feedID string
		db.QueryRow(`INSERT INTO feeds (feed_url, title) VALUES ('https://unique2.example.com/feed', 'Feed') RETURNING id`).Scan(&feedID)

		_, err := db.Exec(`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2)`, userID, feedID)
		if err != nil {
			t.Fatalf("1件目の購読挿入に失敗: %v", err)
		}

		_, err = db.Exec(`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2)`, userID, feedID)
		if err == nil {
			t.Error("重複する購読の挿入がエラーにならなかった")
		}
	})

	t.Run("item_states_user_item_unique", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('unique3@test.com', 'Unique3') RETURNING id`).Scan(&userID)

		var feedID string
		db.QueryRow(`SELECT id FROM feeds LIMIT 1`).Scan(&feedID)

		var itemID string
		db.QueryRow(`INSERT INTO items (feed_id, title) VALUES ($1, 'Unique Item') RETURNING id`, feedID).Scan(&itemID)

		_, err := db.Exec(`INSERT INTO item_states (user_id, item_id) VALUES ($1, $2)`, userID, itemID)
		if err != nil {
			t.Fatalf("1件目の記事状態挿入に失敗: %v", err)
		}

		_, err = db.Exec(`INSERT INTO item_states (user_id, item_id) VALUES ($1, $2)`, userID, itemID)
		if err == nil {
			t.Error("重複する記事状態の挿入がエラーにならなかった")
		}
	})

	t.Run("user_settings_user_id_unique", func(t *testing.T) {
		var userID string
		db.QueryRow(`INSERT INTO users (email, name) VALUES ('unique4@test.com', 'Unique4') RETURNING id`).Scan(&userID)

		_, err := db.Exec(`INSERT INTO user_settings (user_id) VALUES ($1)`, userID)
		if err != nil {
			t.Fatalf("1件目のユーザー設定挿入に失敗: %v", err)
		}

		_, err = db.Exec(`INSERT INTO user_settings (user_id) VALUES ($1)`, userID)
		if err == nil {
			t.Error("重複するuser_settingsの挿入がエラーにならなかった")
		}
	})

	t.Run("items_feed_id_guid_or_id_partial_unique", func(t *testing.T) {
		var feedID string
		db.QueryRow(`INSERT INTO feeds (feed_url, title) VALUES ('https://partial-unique.example.com/feed', 'PU Feed') RETURNING id`).Scan(&feedID)

		// guid_or_idがnon-NULLの場合はユニーク制約が適用される
		_, err := db.Exec(`INSERT INTO items (feed_id, title, guid_or_id) VALUES ($1, 'Item1', 'guid-1')`, feedID)
		if err != nil {
			t.Fatalf("1件目の記事挿入に失敗: %v", err)
		}

		_, err = db.Exec(`INSERT INTO items (feed_id, title, guid_or_id) VALUES ($1, 'Item2', 'guid-1')`, feedID)
		if err == nil {
			t.Error("重複する(feed_id, guid_or_id)の挿入がエラーにならなかった")
		}

		// guid_or_idがNULLの場合は重複が許される
		_, err = db.Exec(`INSERT INTO items (feed_id, title) VALUES ($1, 'Item3')`, feedID)
		if err != nil {
			t.Fatalf("guid_or_id NULLの1件目の挿入に失敗: %v", err)
		}
		_, err = db.Exec(`INSERT INTO items (feed_id, title) VALUES ($1, 'Item4')`, feedID)
		if err != nil {
			t.Fatalf("guid_or_id NULLの2件目の挿入に失敗（NULLの重複は許されるべき）: %v", err)
		}
	})
}

// ============================================================
// ヘルパー関数
// ============================================================

// assertTableColumns はテーブルのカラムとデータ型を検証する。
func assertTableColumns(t *testing.T, db *sql.DB, table string, expected map[string]string) {
	t.Helper()

	rows, err := db.Query(
		"SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1",
		table,
	)
	if err != nil {
		t.Fatalf("%s テーブルのカラム情報取得に失敗: %v", table, err)
	}
	defer rows.Close()

	actual := make(map[string]string)
	for rows.Next() {
		var name, dtype string
		if err := rows.Scan(&name, &dtype); err != nil {
			t.Fatalf("カラム情報のスキャンに失敗: %v", err)
		}
		actual[name] = dtype
	}

	for col, expectedType := range expected {
		actualType, ok := actual[col]
		if !ok {
			t.Errorf("%s.%s カラムが存在しません", table, col)
			continue
		}
		if actualType != expectedType {
			t.Errorf("%s.%s のデータ型が不正: got %q, want %q", table, col, actualType, expectedType)
		}
	}
}

// assertNotNull はカラムのNOT NULL制約を検証する。
func assertNotNull(t *testing.T, db *sql.DB, table string, columns []string) {
	t.Helper()

	for _, col := range columns {
		var isNullable string
		err := db.QueryRow(
			"SELECT is_nullable FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2",
			table, col,
		).Scan(&isNullable)
		if err != nil {
			t.Errorf("%s.%s のNOT NULL制約確認に失敗: %v", table, col, err)
			continue
		}
		if isNullable != "NO" {
			t.Errorf("%s.%s にNOT NULL制約が設定されていません", table, col)
		}
	}
}

// assertPrimaryKey はプライマリキーを検証する。
func assertPrimaryKey(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT count(*) FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_schema = 'public'
			AND tc.table_name = $1
			AND kcu.column_name = $2
	`, table, column).Scan(&count)
	if err != nil {
		t.Fatalf("%s.%s のPK確認に失敗: %v", table, column, err)
	}
	if count == 0 {
		t.Errorf("%s.%s にプライマリキーが設定されていません", table, column)
	}
}

// assertUniqueConstraint はユニーク制約を検証する（カラムの組み合わせ）。
func assertUniqueConstraint(t *testing.T, db *sql.DB, table string, columns []string) {
	t.Helper()

	// pg_catalogを使用してユニーク制約またはユニークインデックスの存在を確認
	query := `
		SELECT count(*) FROM (
			SELECT i.relname
			FROM pg_index ix
			JOIN pg_class t ON t.oid = ix.indrelid
			JOIN pg_class i ON i.oid = ix.indexrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			WHERE t.relname = $1
				AND n.nspname = 'public'
				AND ix.indisunique = true
				AND ix.indisprimary = false
				AND (
					SELECT array_agg(a.attname::text ORDER BY array_position(ix.indkey, a.attnum))
					FROM pg_attribute a
					WHERE a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
				) = $2::text[]
		) sub
	`
	var count int
	err := db.QueryRow(query, table, fmt.Sprintf("{%s}", joinStrings(columns))).Scan(&count)
	if err != nil {
		t.Fatalf("%s のユニーク制約確認に失敗: %v", table, err)
	}
	if count == 0 {
		t.Errorf("%s テーブルに %v のユニーク制約が設定されていません", table, columns)
	}
}

// assertForeignKey は外部キー制約を検証する。
func assertForeignKey(t *testing.T, db *sql.DB, table, column, refTable, refColumn, deleteRule string) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT count(*) FROM information_schema.referential_constraints rc
		JOIN information_schema.key_column_usage kcu
			ON rc.constraint_name = kcu.constraint_name
			AND rc.constraint_schema = kcu.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
			ON rc.unique_constraint_name = ccu.constraint_name
			AND rc.unique_constraint_schema = ccu.constraint_schema
		WHERE kcu.table_schema = 'public'
			AND kcu.table_name = $1
			AND kcu.column_name = $2
			AND ccu.table_name = $3
			AND ccu.column_name = $4
			AND rc.delete_rule = $5
	`, table, column, refTable, refColumn, deleteRule).Scan(&count)
	if err != nil {
		t.Fatalf("%s.%s -> %s.%s のFK確認に失敗: %v", table, column, refTable, refColumn, err)
	}
	if count == 0 {
		t.Errorf("%s.%s -> %s.%s の外部キー制約（ON DELETE %s）が設定されていません", table, column, refTable, refColumn, deleteRule)
	}
}

// assertIndexExists はインデックスの存在を検証する（カラム名を含む）。
func assertIndexExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public'
			AND tablename = $1
			AND indexdef LIKE '%' || $2 || '%'
	`, table, column).Scan(&count)
	if err != nil {
		t.Fatalf("%s.%s のインデックス確認に失敗: %v", table, column, err)
	}
	if count == 0 {
		t.Errorf("%s.%s にインデックスが設定されていません", table, column)
	}
}

// assertPartialIndexExists は部分インデックスの存在を検証する。
func assertPartialIndexExists(t *testing.T, db *sql.DB, table, indexedCol, whereCol string) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public'
			AND tablename = $1
			AND indexdef LIKE '%' || $2 || '%'
			AND indexdef LIKE '%WHERE%' || $3 || '%'
	`, table, indexedCol, whereCol).Scan(&count)
	if err != nil {
		t.Fatalf("%s の部分インデックス確認に失敗: %v", table, err)
	}
	if count == 0 {
		t.Errorf("%s テーブルに %s の部分インデックス（WHERE %s）が設定されていません", table, indexedCol, whereCol)
	}
}

// assertPartialUniqueIndex は部分ユニークインデックスの存在を検証する。
func assertPartialUniqueIndex(t *testing.T, db *sql.DB, table string, columns []string, whereCol string) {
	t.Helper()

	var count int
	query := `
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public'
			AND tablename = $1
			AND indexdef LIKE '%UNIQUE%'
			AND indexdef LIKE '%WHERE%' || $2 || '%'
	`
	err := db.QueryRow(query, table, whereCol).Scan(&count)
	if err != nil {
		t.Fatalf("%s の部分ユニークインデックス確認に失敗: %v", table, err)
	}
	if count == 0 {
		t.Errorf("%s テーブルに %v の部分ユニークインデックス（WHERE %s IS NOT NULL）が設定されていません", table, columns, whereCol)
	}
}

// assertPartialIndexOnBool はboolean型の部分インデックスの存在を検証する。
func assertPartialIndexOnBool(t *testing.T, db *sql.DB, table, column, value string) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public'
			AND tablename = $1
			AND indexdef LIKE '%' || $2 || '%'
			AND indexdef LIKE '%WHERE%'
	`, table, column).Scan(&count)
	if err != nil {
		t.Fatalf("%s.%s の部分インデックス確認に失敗: %v", table, column, err)
	}
	if count == 0 {
		t.Errorf("%s テーブルに %s = %s の部分インデックスが設定されていません", table, column, value)
	}
}

// joinStrings はスライスをカンマ区切りの文字列に変換する。
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}
