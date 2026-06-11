package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/model"
)

// 本ファイルは Issue #170 の design.md §Testing Strategy ケース 6（退会フロー相当の
// 統合検証）を実装する。
//
// 共有トランザクション上で sessions → auth_codes → refresh_token_families → users
// を削除して Commit した後、当該ユーザーの native auth 3 テーブル（auth_codes /
// refresh_token_families / refresh_tokens）が 0 件・他ユーザーの行が残存することを
// 検証する。
//
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。DB へ接続できない
// 環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。

// withdrawTestDatabaseURL はテスト用のデータベース URL を返す。
func withdrawTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupWithdrawTestDB はテスト用 DB を準備しマイグレーション適用済みの *sql.DB を返す。
// DB に接続できない場合は t.Skip でテストをスキップする（NFR 3.1）。
func setupWithdrawTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := withdrawTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	cleanupSQL := `
		DROP TABLE IF EXISTS refresh_tokens CASCADE;
		DROP TABLE IF EXISTS refresh_token_families CASCADE;
		DROP TABLE IF EXISTS auth_codes CASCADE;
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

	return db
}

// insertTestUserForWithdraw はテスト用ユーザーを作成し、その ID を返す。
func insertTestUserForWithdraw(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		email, "Withdraw Integration Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}
	return userID
}

// seedNativeAuthDataForWithdraw は当該ユーザーの auth_codes / refresh_token_families
// / refresh_tokens にそれぞれテスト用レコードを 1 件ずつ作成する。戻り値は
// (familyID, tokenHash) で、検証側で件数確認に使う。
func seedNativeAuthDataForWithdraw(t *testing.T, db *sql.DB, userID, suffix string) {
	t.Helper()
	ctx := context.Background()
	authCodeRepo := NewPostgresAuthCodeRepo(db)
	refreshTokenRepo := NewPostgresRefreshTokenRepo(db)

	// auth_code
	authCode := &model.AuthCode{
		CodeHash:      "hash-withdraw-integration-code-" + suffix,
		UserID:        userID,
		PKCEChallenge: "test-pkce-challenge-withdraw-int",
		ExpiresAt:     time.Now().Add(60 * time.Second).UTC(),
		Used:          false,
	}
	if err := authCodeRepo.Create(ctx, authCode); err != nil {
		t.Fatalf("auth_code Create に失敗 (%s): %v", suffix, err)
	}

	// refresh_token_family + refresh_token
	family := &model.RefreshTokenFamily{UserID: userID}
	if err := refreshTokenRepo.CreateFamily(ctx, family); err != nil {
		t.Fatalf("CreateFamily に失敗 (%s): %v", suffix, err)
	}
	token := &model.RefreshToken{
		FamilyID:  family.ID,
		UserID:    userID,
		TokenHash: "hash-withdraw-integration-token-" + suffix,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := refreshTokenRepo.CreateToken(ctx, token); err != nil {
		t.Fatalf("CreateToken に失敗 (%s): %v", suffix, err)
	}
}

// countByUserID は指定テーブルで当該 user_id の行数を返す（検証ヘルパ）。
func countByUserID(t *testing.T, db *sql.DB, table, userID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM `+table+` WHERE user_id = $1`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("%s COUNT 取得に失敗: %v", table, err)
	}
	return count
}

// TestWithdrawIntegration_NativeAuthCleanup は退会フロー相当の統合検証として、
// 共有トランザクション上で sessions → auth_codes → refresh_token_families →
// users を削除して Commit した後、当該ユーザーの native auth 3 テーブルが 0 件・
// 他ユーザーの行が残存することを検証する（Issue #170 design.md §Testing Strategy
// ケース 6 / Req 1.1, 1.2, 2.1, 3.1 / NFR 2.1）。
//
// 本テストは PostgresAuthCodeRepo / PostgresRefreshTokenRepo / PostgresSessionRepo /
// PostgresUserRepo の Exec 変種が共有トランザクション上で協調動作することを
// 検証する（user.Service.withdrawTx と同じ削除順序を repository レイヤから直接
// 再現するため、handler / service 層には依存しない）。
func TestWithdrawIntegration_NativeAuthCleanup(t *testing.T) {
	db := setupWithdrawTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Arrange: target user と bystander user に native auth + session を seed する
	targetUserID := insertTestUserForWithdraw(t, db, "withdraw-target@test.com")
	bystanderUserID := insertTestUserForWithdraw(t, db, "withdraw-bystander@test.com")

	seedNativeAuthDataForWithdraw(t, db, targetUserID, "target")
	seedNativeAuthDataForWithdraw(t, db, bystanderUserID, "bystander")

	// 各 user に session を 1 件追加（sessions の削除も確認するため）
	for _, uid := range []string{targetUserID, bystanderUserID} {
		if _, err := db.Exec(
			`INSERT INTO sessions (id, user_id, data, expires_at, created_at)
			 VALUES (gen_random_uuid(), $1, '{}'::jsonb, now() + interval '1 hour', now())`,
			uid,
		); err != nil {
			t.Fatalf("session 挿入に失敗 (%s): %v", uid, err)
		}
	}

	// Sanity check: 削除前は両 user に 3 テーブル + sessions が 1 件ずつ存在する
	for _, table := range []string{"auth_codes", "refresh_token_families", "refresh_tokens", "sessions"} {
		if c := countByUserID(t, db, table, targetUserID); c != 1 {
			t.Fatalf("seed sanity: target %s = %d, want 1", table, c)
		}
		if c := countByUserID(t, db, table, bystanderUserID); c != 1 {
			t.Fatalf("seed sanity: bystander %s = %d, want 1", table, c)
		}
	}

	// Act: 共有 tx 上で sessions → auth_codes → refresh_token_families → users を
	// 削除して Commit する（user.Service.withdrawTx と同じ順序）。
	authCodeRepo := NewPostgresAuthCodeRepo(db)
	refreshTokenRepo := NewPostgresRefreshTokenRepo(db)
	sessionRepo := NewPostgresSessionRepo(db)
	userRepo := NewPostgresUserRepo(db)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx に失敗: %v", err)
	}
	defer func() {
		// 失敗時の保険。成功経路では Commit 済みなので no-op。
		_ = tx.Rollback()
	}()

	if err := sessionRepo.DeleteByUserIDExec(ctx, tx, targetUserID); err != nil {
		t.Fatalf("sessions 削除に失敗: %v", err)
	}
	if err := authCodeRepo.DeleteByUserIDExec(ctx, tx, targetUserID); err != nil {
		t.Fatalf("auth_codes 削除に失敗: %v", err)
	}
	if err := refreshTokenRepo.DeleteByUserIDExec(ctx, tx, targetUserID); err != nil {
		t.Fatalf("refresh_token_families 削除に失敗: %v", err)
	}
	if err := userRepo.DeleteByIDExec(ctx, tx, targetUserID); err != nil {
		t.Fatalf("users 削除に失敗: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit に失敗: %v", err)
	}

	// Assert 1: target user の native auth 3 テーブルが 0 件
	for _, table := range []string{"auth_codes", "refresh_token_families", "refresh_tokens"} {
		if c := countByUserID(t, db, table, targetUserID); c != 0 {
			t.Errorf("退会後の target %s が残存している: got %d, want 0", table, c)
		}
	}
	// sessions も削除されている
	if c := countByUserID(t, db, "sessions", targetUserID); c != 0 {
		t.Errorf("退会後の target sessions が残存している: got %d, want 0", c)
	}
	// users 自体も削除されている
	var userExists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = $1`, targetUserID).Scan(&userExists); err != nil {
		t.Fatalf("users COUNT 取得に失敗: %v", err)
	}
	if userExists != 0 {
		t.Errorf("退会後の target users が残存している: got %d, want 0", userExists)
	}

	// Assert 2: bystander user の native auth 3 テーブル + sessions は影響を受けず残存
	for _, table := range []string{"auth_codes", "refresh_token_families", "refresh_tokens", "sessions"} {
		if c := countByUserID(t, db, table, bystanderUserID); c != 1 {
			t.Errorf("bystander %s が target 退会で削除された: got %d, want 1 (他ユーザー非影響)", table, c)
		}
	}
}

// TestWithdrawIntegration_NativeAuthRollback は退会フロー相当のトランザクション内
// 削除を Rollback した場合、target user の 3 テーブルすべてが残存することを検証する
// （Issue #170 Req 2.2 / 2.3 の「途中失敗時に部分的な削除状態を確定しない」を
// repository レイヤ単体で再現する統合テスト）。
func TestWithdrawIntegration_NativeAuthRollback(t *testing.T) {
	db := setupWithdrawTestDB(t)
	defer db.Close()

	ctx := context.Background()

	targetUserID := insertTestUserForWithdraw(t, db, "withdraw-rollback-target@test.com")
	seedNativeAuthDataForWithdraw(t, db, targetUserID, "rollback")

	authCodeRepo := NewPostgresAuthCodeRepo(db)
	refreshTokenRepo := NewPostgresRefreshTokenRepo(db)
	sessionRepo := NewPostgresSessionRepo(db)

	// Act: 共有 tx で sessions → auth_codes → refresh_token_families を削除し Rollback する
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx に失敗: %v", err)
	}
	if err := sessionRepo.DeleteByUserIDExec(ctx, tx, targetUserID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("sessions 削除に失敗: %v", err)
	}
	if err := authCodeRepo.DeleteByUserIDExec(ctx, tx, targetUserID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("auth_codes 削除に失敗: %v", err)
	}
	if err := refreshTokenRepo.DeleteByUserIDExec(ctx, tx, targetUserID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("refresh_token_families 削除に失敗: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback に失敗: %v", err)
	}

	// Assert: Rollback により target user の native auth 3 テーブルすべてが残存する
	for _, table := range []string{"auth_codes", "refresh_token_families", "refresh_tokens"} {
		if c := countByUserID(t, db, table, targetUserID); c != 1 {
			t.Errorf("Rollback 後の target %s が残存していない: got %d, want 1 (部分削除確定の禁止)", table, c)
		}
	}
}
