package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/model"
	_ "github.com/lib/pq"
)

// PostgresFeedRepoはFeedRepositoryインターフェースを満たすことを検証
func TestPostgresFeedRepo_ImplementsInterface(t *testing.T) {
	var _ FeedRepository = (*PostgresFeedRepo)(nil)
}

// NewPostgresFeedRepoが正しく初期化されることを検証
func TestNewPostgresFeedRepo_Initializes(t *testing.T) {
	repo := NewPostgresFeedRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// Feedモデルのフィールドが正しく構築されることを検証
func TestPostgresFeedRepo_FeedModel_Fields(t *testing.T) {
	now := time.Now()
	feed := &model.Feed{
		ID:          "feed-id-1",
		FeedURL:     "https://example.com/feed.xml",
		SiteURL:     "https://example.com",
		Title:       "テストフィード",
		FetchStatus: model.FetchStatusActive,
		NextFetchAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if feed.ID != "feed-id-1" {
		t.Errorf("feed.ID = %q, want %q", feed.ID, "feed-id-1")
	}
	if feed.FeedURL != "https://example.com/feed.xml" {
		t.Errorf("feed.FeedURL = %q, want %q", feed.FeedURL, "https://example.com/feed.xml")
	}
	if feed.FetchStatus != model.FetchStatusActive {
		t.Errorf("feed.FetchStatus = %q, want %q", feed.FetchStatus, model.FetchStatusActive)
	}
}

// Feedのfaviconフィールドがnil許容であることを検証
func TestPostgresFeedRepo_FeedModel_NilFavicon(t *testing.T) {
	feed := &model.Feed{
		ID:      "feed-id-2",
		FeedURL: "https://example.com/feed.xml",
		Title:   "テストフィード",
	}

	if feed.FaviconData != nil {
		t.Error("favicon_data should be nil by default")
	}
	if feed.FaviconMime != "" {
		t.Error("favicon_mime should be empty by default")
	}
}

// ============================================================
// ListDueForFetch 回帰テスト（Issue #98: DISTINCT + FOR UPDATE バグ）
// テスト用PostgreSQLを使用する。接続できない場合はスキップする。
// ============================================================

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

// setupListDueTestDB はListDueForFetch回帰テスト用のクリーンなデータベースを準備する。
// 既存テーブルをドロップしてマイグレーションを適用し、テスト用PostgreSQLに
// 接続できない場合はテストをスキップする。
func setupListDueTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := testDatabaseURL(t)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

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

	t.Cleanup(func() { db.Close() })
	return db
}

// insertTestUser はテスト用ユーザーを挿入し、生成されたIDを返す。
func insertTestUser(t *testing.T, db *sql.DB, email string) string {
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

// insertTestFeed はテスト用フィードを挿入し、生成されたIDを返す。
// next_fetch_at / fetch_status を明示指定することでフェッチ対象選別条件を制御できる。
func insertTestFeed(t *testing.T, db *sql.DB, feedURL string, nextFetchAt time.Time, status model.FetchStatus) string {
	t.Helper()
	var feedID string
	err := db.QueryRow(
		`INSERT INTO feeds (feed_url, title, fetch_status, next_fetch_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		feedURL, "Test Feed", string(status), nextFetchAt,
	).Scan(&feedID)
	if err != nil {
		t.Fatalf("フィード挿入に失敗: %v", err)
	}
	return feedID
}

// insertTestSubscription はテスト用購読を挿入する。
func insertTestSubscription(t *testing.T, db *sql.DB, userID, feedID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO subscriptions (user_id, feed_id) VALUES ($1, $2)`,
		userID, feedID,
	)
	if err != nil {
		t.Fatalf("購読挿入に失敗: %v", err)
	}
}

// containsFeedID は返却されたフィードスライスに指定IDが何回出現するかを数える。
func countFeedID(feeds []*model.Feed, feedID string) int {
	count := 0
	for _, f := range feeds {
		if f.ID == feedID {
			count++
		}
	}
	return count
}

// TestPostgresFeedRepo_ListDueForFetch は ListDueForFetch がフェッチ対象を
// 重複なく・選別条件どおりに・エラーなく取得することを検証する回帰テスト。
// Issue #98: SELECT DISTINCT + FOR UPDATE の併用で 0A000 エラーが発生し
// フェッチ対象取得が全失敗していた不具合の再発防止。
func TestPostgresFeedRepo_ListDueForFetch(t *testing.T) {
	ctx := context.Background()
	due := time.Now().Add(-1 * time.Minute) // 取得期限到来済み

	// Requirement 1.1 / 2.1: 購読者が複数いるフィードが結果に1回だけ返る（重複しない）
	t.Run("購読者が複数存在するフィードのとき結果に1回だけ含まれる", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresFeedRepo(db)

		feedID := insertTestFeed(t, db, "https://example.com/multi.xml", due, model.FetchStatusActive)
		u1 := insertTestUser(t, db, "multi1@example.com")
		u2 := insertTestUser(t, db, "multi2@example.com")
		u3 := insertTestUser(t, db, "multi3@example.com")
		insertTestSubscription(t, db, u1, feedID)
		insertTestSubscription(t, db, u2, feedID)
		insertTestSubscription(t, db, u3, feedID)

		feeds, err := repo.ListDueForFetch(ctx)
		if err != nil {
			t.Fatalf("ListDueForFetch returned error: %v", err)
		}

		if got := countFeedID(feeds, feedID); got != 1 {
			t.Errorf("購読者3人のフィードの出現回数 = %d, want 1（重複してはならない）", got)
		}
	})

	// Requirement 2.4 / 2.5 / 2.6 / 2.7: 選別条件（境界・異常系）
	t.Run("選別条件のとき期限到来済みかつactiveなフィードのみ返る", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresFeedRepo(db)

		user := insertTestUser(t, db, "select@example.com")

		cases := []struct {
			name        string
			feedURL     string
			nextFetchAt time.Time
			status      model.FetchStatus
			wantInclude bool
		}{
			{
				name:        "期限到来済み_active",
				feedURL:     "https://example.com/due-active.xml",
				nextFetchAt: due,
				status:      model.FetchStatusActive,
				wantInclude: true,
			},
			{
				name:        "境界_期限ちょうど現在時刻以下_active",
				feedURL:     "https://example.com/boundary.xml",
				nextFetchAt: time.Now().Add(-1 * time.Second),
				status:      model.FetchStatusActive,
				wantInclude: true,
			},
			{
				name:        "期限未到来_active",
				feedURL:     "https://example.com/future.xml",
				nextFetchAt: time.Now().Add(1 * time.Hour),
				status:      model.FetchStatusActive,
				wantInclude: false,
			},
			{
				name:        "期限到来済み_stopped",
				feedURL:     "https://example.com/stopped.xml",
				nextFetchAt: due,
				status:      model.FetchStatusStopped,
				wantInclude: false,
			},
			{
				name:        "期限到来済み_error",
				feedURL:     "https://example.com/error.xml",
				nextFetchAt: due,
				status:      model.FetchStatusError,
				wantInclude: false,
			},
		}

		feedIDs := make(map[string]string, len(cases))
		for _, c := range cases {
			id := insertTestFeed(t, db, c.feedURL, c.nextFetchAt, c.status)
			feedIDs[c.name] = id
			insertTestSubscription(t, db, user, id)
		}

		feeds, err := repo.ListDueForFetch(ctx)
		if err != nil {
			t.Fatalf("ListDueForFetch returned error: %v", err)
		}

		for _, c := range cases {
			got := countFeedID(feeds, feedIDs[c.name]) > 0
			if got != c.wantInclude {
				t.Errorf("%s: 結果への包含 = %v, want %v", c.name, got, c.wantInclude)
			}
		}
	})

	// Requirement 2.2 / 2.3: 購読者0人のフィードは返らない
	t.Run("購読者が0人のフィードのとき結果から除外される", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresFeedRepo(db)

		// 購読者ゼロのフィード（subscriptionsを挿入しない）
		noSubFeedID := insertTestFeed(t, db, "https://example.com/no-sub.xml", due, model.FetchStatusActive)

		// 比較用: 購読者ありのフィード
		user := insertTestUser(t, db, "withsub@example.com")
		withSubFeedID := insertTestFeed(t, db, "https://example.com/with-sub.xml", due, model.FetchStatusActive)
		insertTestSubscription(t, db, user, withSubFeedID)

		feeds, err := repo.ListDueForFetch(ctx)
		if err != nil {
			t.Fatalf("ListDueForFetch returned error: %v", err)
		}

		if got := countFeedID(feeds, noSubFeedID); got != 0 {
			t.Errorf("購読者0人のフィードの出現回数 = %d, want 0（除外されるべき）", got)
		}
		if got := countFeedID(feeds, withSubFeedID); got != 1 {
			t.Errorf("購読者ありのフィードの出現回数 = %d, want 1", got)
		}
	})

	// Requirement 1.3: 空のデータ状態でエラーなく空の結果が返る
	t.Run("購読フィードが存在しない空のデータ状態のときエラーなく空の結果が返る", func(t *testing.T) {
		db := setupListDueTestDB(t)
		repo := NewPostgresFeedRepo(db)

		feeds, err := repo.ListDueForFetch(ctx)
		if err != nil {
			t.Fatalf("空データ状態でListDueForFetchがエラーを返した: %v", err)
		}
		if len(feeds) != 0 {
			t.Errorf("空データ状態の結果件数 = %d, want 0", len(feeds))
		}
	})
}
