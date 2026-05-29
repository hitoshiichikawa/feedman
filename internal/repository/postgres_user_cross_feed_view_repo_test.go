package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/database"
)

// PostgresUserCrossFeedViewRepoの interface 適合性を compile-time に確認するための包括テスト。
func TestPostgresUserCrossFeedViewRepo_ImplementsInterface(t *testing.T) {
	var _ UserCrossFeedViewRepository = (*PostgresUserCrossFeedViewRepo)(nil)
}

// NewPostgresUserCrossFeedViewRepoが正しく初期化されることを検証する。
func TestNewPostgresUserCrossFeedViewRepo_Initializes(t *testing.T) {
	repo := NewPostgresUserCrossFeedViewRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// このファイルは PostgresUserCrossFeedViewRepo の DB 結合テスト。
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。
// DB へ接続できない環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。
// この Skip ガードの慣習は他の Postgres 系結合テストと統一している。

// crossFeedViewTestDatabaseURL はテスト用のデータベース URL を返す。
func crossFeedViewTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupCrossFeedViewTestDB はテスト用データベースを準備し、マイグレーション適用済みの
// *sql.DB を返す。DB へ接続できない場合は t.Skip でテストをスキップする。
func setupCrossFeedViewTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := crossFeedViewTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	cleanupSQL := `
		DROP TABLE IF EXISTS user_cross_feed_views CASCADE;
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

// insertTestUserForCrossFeedView はテスト用ユーザーを作成し、その ID を返す。
func insertTestUserForCrossFeedView(t *testing.T, db *sql.DB, email string) string {
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

// TestPostgresUserCrossFeedViewRepo_GetWhenNotRegistered は、未登録ユーザーに対して
// Get が (nil, nil) を返すことを検証する（Req 4.4 / 初回利用時の判定の前提）。
func TestPostgresUserCrossFeedViewRepo_GetWhenNotRegistered(t *testing.T) {
	// Arrange
	db := setupCrossFeedViewTestDB(t)
	repo := NewPostgresUserCrossFeedViewRepo(db)
	userID := insertTestUserForCrossFeedView(t, db, "not-registered@example.com")

	// Act
	got, err := repo.Get(context.Background(), userID)

	// Assert
	if err != nil {
		t.Fatalf("Get に失敗: %v", err)
	}
	if got != nil {
		t.Fatalf("未登録ユーザーには nil が返るべき。got=%+v", got)
	}
}

// TestPostgresUserCrossFeedViewRepo_UpsertThenGet は、Upsert で挿入した値を Get で取得
// できること、および last_seen_at が DB に正しく永続化されることを検証する（Req 4.1, 4.5）。
func TestPostgresUserCrossFeedViewRepo_UpsertThenGet(t *testing.T) {
	// Arrange
	db := setupCrossFeedViewTestDB(t)
	repo := NewPostgresUserCrossFeedViewRepo(db)
	ctx := context.Background()
	userID := insertTestUserForCrossFeedView(t, db, "upsert-get@example.com")
	want := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	// Act
	if err := repo.Upsert(ctx, userID, want); err != nil {
		t.Fatalf("Upsert に失敗: %v", err)
	}
	got, err := repo.Get(ctx, userID)

	// Assert
	if err != nil {
		t.Fatalf("Get に失敗: %v", err)
	}
	if got == nil {
		t.Fatalf("Upsert 後の Get で nil が返った")
	}
	if got.UserID != userID {
		t.Errorf("UserID = %q, want %q", got.UserID, userID)
	}
	if !got.LastSeenAt.Equal(want) {
		t.Errorf("LastSeenAt = %v, want %v", got.LastSeenAt, want)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt はゼロ値であってはならない（DB の now() default が機能していない可能性）")
	}
}

// TestPostgresUserCrossFeedViewRepo_UpsertOverwritesExisting は、再 Upsert で
// last_seen_at が更新され updated_at も新しい値に進むこと（冪等な上書き）を検証する。
// Req 4.3「同一セッション内で初めて表示完了時にサーバ側 last_seen_at を更新」の DB 側保証。
func TestPostgresUserCrossFeedViewRepo_UpsertOverwritesExisting(t *testing.T) {
	// Arrange
	db := setupCrossFeedViewTestDB(t)
	repo := NewPostgresUserCrossFeedViewRepo(db)
	ctx := context.Background()
	userID := insertTestUserForCrossFeedView(t, db, "upsert-overwrite@example.com")

	first := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 28, 18, 30, 0, 0, time.UTC)

	if err := repo.Upsert(ctx, userID, first); err != nil {
		t.Fatalf("1 回目の Upsert に失敗: %v", err)
	}
	got1, err := repo.Get(ctx, userID)
	if err != nil || got1 == nil {
		t.Fatalf("1 回目の Get に失敗: err=%v got=%+v", err, got1)
	}
	firstUpdatedAt := got1.UpdatedAt

	// updated_at の進行を検出可能にするため微小スリープ（DB の clock 分解能を超える）
	time.Sleep(10 * time.Millisecond)

	// Act
	if err := repo.Upsert(ctx, userID, second); err != nil {
		t.Fatalf("2 回目の Upsert に失敗: %v", err)
	}
	got2, err := repo.Get(ctx, userID)

	// Assert
	if err != nil {
		t.Fatalf("2 回目の Get に失敗: %v", err)
	}
	if got2 == nil {
		t.Fatalf("2 回目の Upsert 後の Get で nil が返った")
	}
	if !got2.LastSeenAt.Equal(second) {
		t.Errorf("LastSeenAt = %v, want %v（上書きされていない）", got2.LastSeenAt, second)
	}
	if !got2.UpdatedAt.After(firstUpdatedAt) {
		t.Errorf("UpdatedAt は進行すべき。before=%v after=%v", firstUpdatedAt, got2.UpdatedAt)
	}

	// 行数が 1 件のままであることを確認（PK 制約により高々 1 行 / Req 4.1 不変条件）
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM user_cross_feed_views WHERE user_id = $1`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("count クエリに失敗: %v", err)
	}
	if count != 1 {
		t.Errorf("user_cross_feed_views の行数 = %d, want 1（PK 制約違反）", count)
	}
}
