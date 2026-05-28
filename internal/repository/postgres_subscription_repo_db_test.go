package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/database"
)

// このファイルはテスト用 PostgreSQL を介した結合テスト（Issue #100 回帰テスト）。
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。
// DB へ接続できない環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。
// この Skip ガードの慣習は internal/database/migrate_test.go と統一している。

// subTestDatabaseURL はテスト用のデータベース URL を返す。
func subTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupSubscriptionTestDB はテスト用データベースを準備し、マイグレーション適用済みの
// *sql.DB を返す。DB へ接続できない場合は t.Skip でテストをスキップする。
func setupSubscriptionTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := subTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	// クリーンアップ: 既存のテーブルとマイグレーション履歴を削除してクリーンな状態にする
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
		db.Close()
		t.Fatalf("クリーンアップに失敗: %v", err)
	}

	if err := database.RunMigrations(dbURL); err != nil {
		db.Close()
		t.Fatalf("マイグレーション実行に失敗: %v", err)
	}

	return db
}

// insertTestUserForSub はテスト用ユーザーを作成し、その ID を返す。
func insertTestUserForSub(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		email, "Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}
	return userID
}

// insertTestFeedForSub はテスト用フィードを作成し、その ID を返す。
// faviconMime が nil の場合は favicon_mime を NULL のまま（未設定）で挿入する。
func insertTestFeedForSub(t *testing.T, db *sql.DB, feedURL, title string, faviconMime *string) string {
	t.Helper()
	var feedID string
	err := db.QueryRow(
		`INSERT INTO feeds (feed_url, title, favicon_mime) VALUES ($1, $2, $3) RETURNING id`,
		feedURL, title, faviconMime,
	).Scan(&feedID)
	if err != nil {
		t.Fatalf("フィード挿入に失敗: %v", err)
	}
	return feedID
}

// insertTestSubscriptionForSub はテスト用購読を作成する。
func insertTestSubscriptionForSub(t *testing.T, db *sql.DB, userID, feedID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2)`,
		userID, feedID,
	)
	if err != nil {
		t.Fatalf("購読挿入に失敗: %v", err)
	}
}

// TestListByUserIDWithFeedInfo_FaviconMimeNull は Issue #100 の回帰テスト。
// favicon_mime が NULL のフィードを購読していても ListByUserIDWithFeedInfo が
// 成功し、当該フィードの FaviconMime が空文字で返ることを検証する。
// 修正前のクエリ（COALESCE 無し）では NULL を非 NULL string へ Scan しようとして
// "converting NULL to string is unsupported" で失敗する（Red）。
func TestListByUserIDWithFeedInfo_FaviconMimeNull(t *testing.T) {
	db := setupSubscriptionTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresSubscriptionRepo(db)

	// Req 1.1 / Req 1.3 / Req 3.1: favicon_mime が NULL のフィードのみを購読
	t.Run("favicon_mimeがNULLのフィードのみを購読しているとき成功し空文字を返す", func(t *testing.T) {
		userID := insertTestUserForSub(t, db, "null-only@test.com")
		feedID := insertTestFeedForSub(t, db, "https://example.com/null-only.xml", "No Favicon Feed", nil)
		insertTestSubscriptionForSub(t, db, userID, feedID)

		results, err := repo.ListByUserIDWithFeedInfo(ctx, userID)
		if err != nil {
			t.Fatalf("ListByUserIDWithFeedInfo がエラーを返した（NULL favicon_mime で失敗）: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("購読件数が不正: got %d, want 1", len(results))
		}
		if results[0].FeedID != feedID {
			t.Errorf("FeedID が不正: got %q, want %q", results[0].FeedID, feedID)
		}
		if results[0].FaviconMime != "" {
			t.Errorf("FaviconMime が空文字ではない: got %q, want %q", results[0].FaviconMime, "")
		}
	})

	// Req 1.2 / Req 2.1 / Req 2.2 / Req 4.2: favicon あり/なし混在で全件返り、
	// favicon ありのものは実際の mime 値が返る
	t.Run("faviconあり/なし混在のとき全件返り favicon ありは実際のmime値を返す", func(t *testing.T) {
		userID := insertTestUserForSub(t, db, "mixed@test.com")

		mime := "image/png"
		feedWithFaviconID := insertTestFeedForSub(t, db, "https://example.com/with-favicon.xml", "With Favicon", &mime)
		feedWithoutFaviconID := insertTestFeedForSub(t, db, "https://example.com/without-favicon.xml", "Without Favicon", nil)

		insertTestSubscriptionForSub(t, db, userID, feedWithFaviconID)
		insertTestSubscriptionForSub(t, db, userID, feedWithoutFaviconID)

		results, err := repo.ListByUserIDWithFeedInfo(ctx, userID)
		if err != nil {
			t.Fatalf("ListByUserIDWithFeedInfo がエラーを返した: %v", err)
		}

		if len(results) != 2 {
			t.Fatalf("購読件数が不正（全件返却されていない）: got %d, want 2", len(results))
		}

		byFeedID := make(map[string]SubscriptionWithFeedInfo, len(results))
		for _, r := range results {
			byFeedID[r.FeedID] = r
		}

		withFavicon, ok := byFeedID[feedWithFaviconID]
		if !ok {
			t.Fatalf("favicon ありフィードの購読が返却されていない: feedID=%q", feedWithFaviconID)
		}
		if withFavicon.FaviconMime != mime {
			t.Errorf("favicon ありフィードの FaviconMime が不正: got %q, want %q", withFavicon.FaviconMime, mime)
		}

		withoutFavicon, ok := byFeedID[feedWithoutFaviconID]
		if !ok {
			t.Fatalf("favicon なしフィードの購読が返却されていない: feedID=%q", feedWithoutFaviconID)
		}
		if withoutFavicon.FaviconMime != "" {
			t.Errorf("favicon なしフィードの FaviconMime が空文字ではない: got %q, want %q", withoutFavicon.FaviconMime, "")
		}
	})

	// Req 1.4: 購読を 1 件も持たないとき空の一覧を返す
	t.Run("購読が1件もないとき空の一覧を返す", func(t *testing.T) {
		userID := insertTestUserForSub(t, db, "empty@test.com")

		results, err := repo.ListByUserIDWithFeedInfo(ctx, userID)
		if err != nil {
			t.Fatalf("ListByUserIDWithFeedInfo がエラーを返した: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("購読件数が不正: got %d, want 0", len(results))
		}
	})
}
