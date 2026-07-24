package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/database"
	"github.com/hitoshi/feedman/internal/model"
)

// 本ファイルは PostgresRefreshTokenRepo の結合テスト（design.md §Testing Strategy の
// RefreshTokenRepo 6 ケース）を実装する。
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。
// DB へ接続できない環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。
// 慣習は postgres_auth_code_repo_db_test.go と統一している（Req NFR 3.1）。

// refreshTokenTestDatabaseURL はテスト用のデータベース URL を返す。
func refreshTokenTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupRefreshTokenTestDB はテスト用 DB を準備しマイグレーション適用済みの *sql.DB を返す。
// DB に接続できない場合は t.Skip でテストをスキップする（外部ネットワーク依存なし、NFR 3.1）。
func setupRefreshTokenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := refreshTokenTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	// クリーンアップ: 既存テーブルとマイグレーション履歴をリセットしてクリーンな状態にする。
	// 新規 native auth テーブル（auth_codes / refresh_token_families / refresh_tokens）と
	// passkey テーブル（Issue #216）も明示 DROP しておく
	// （同一 DB を繰り返し利用するローカル開発機でも fresh up を保証）。
	cleanupSQL := `
		DROP TABLE IF EXISTS passkey_challenges CASCADE;
		DROP TABLE IF EXISTS passkey_credentials CASCADE;
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

// insertTestUserForRefreshToken はテスト用ユーザーを作成し、その ID を返す。
func insertTestUserForRefreshToken(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		email, "Refresh Token Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}
	return userID
}

// newTestRefreshTokenFamily は RefreshTokenFamily を組み立てる。
// ID / CreatedAt は repo 側のデフォルトで決まることを想定し空のままにする。
func newTestRefreshTokenFamily(userID string) *model.RefreshTokenFamily {
	return &model.RefreshTokenFamily{
		UserID: userID,
	}
}

// newTestRefreshToken は RefreshToken を組み立てる。
// ID / CreatedAt は repo 側のデフォルトで決まることを想定し空のままにする。
func newTestRefreshToken(familyID, userID, tokenHash string, expiresAt time.Time) *model.RefreshToken {
	return &model.RefreshToken{
		FamilyID:  familyID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}
}

// TestPostgresRefreshTokenRepo_DB は design.md §Testing Strategy の RefreshTokenRepo
// 6 ケースを 1 つのテーブルライク構成で実装する（個別の t.Run でケースを分離）。
// 各サブテストは個別のユーザー・family・token を生成して相互に干渉しない設計。
func TestPostgresRefreshTokenRepo_DB(t *testing.T) {
	db := setupRefreshTokenTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresRefreshTokenRepo(db)

	// Case 1 (Req 3.1, 3.5): CreateFamily + CreateToken + FindByHash で同値復元
	t.Run("Create_FindByHashで保存値が同値復元される", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "create-find@test.com")
		family := newTestRefreshTokenFamily(userID)
		if err := repo.CreateFamily(ctx, family); err != nil {
			t.Fatalf("CreateFamily に失敗: %v", err)
		}
		if family.ID == "" {
			t.Fatal("CreateFamily 後の family.ID が空: DB デフォルトが反映されていない")
		}
		if family.CreatedAt.IsZero() {
			t.Fatal("CreateFamily 後の family.CreatedAt が zero: DB デフォルトが反映されていない")
		}

		expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
		token := newTestRefreshToken(family.ID, userID, "hash-create-find-0123456789abcdef", expiresAt)
		if err := repo.CreateToken(ctx, token); err != nil {
			t.Fatalf("CreateToken に失敗: %v", err)
		}
		if token.ID == "" {
			t.Fatal("CreateToken 後の token.ID が空: DB デフォルトが反映されていない")
		}
		if token.CreatedAt.IsZero() {
			t.Fatal("CreateToken 後の token.CreatedAt が zero: DB デフォルトが反映されていない")
		}

		got, err := repo.FindByHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByHash が nil を返した（保存済み hash がヒットしない）")
		}
		if got.ID != token.ID {
			t.Errorf("ID 不一致: got %q, want %q", got.ID, token.ID)
		}
		if got.FamilyID != family.ID {
			t.Errorf("FamilyID 不一致: got %q, want %q", got.FamilyID, family.ID)
		}
		if got.UserID != userID {
			t.Errorf("UserID 不一致: got %q, want %q", got.UserID, userID)
		}
		if got.TokenHash != token.TokenHash {
			t.Errorf("TokenHash 不一致: got %q, want %q", got.TokenHash, token.TokenHash)
		}
		if !got.ExpiresAt.Equal(expiresAt) {
			t.Errorf("ExpiresAt 不一致: got %v, want %v", got.ExpiresAt, expiresAt)
		}
		if got.RotatedAt != nil {
			t.Errorf("作成直後の RotatedAt が non-nil: got %v", got.RotatedAt)
		}
		if got.RevokedAt != nil {
			t.Errorf("作成直後の RevokedAt が non-nil: got %v", got.RevokedAt)
		}
	})

	// Case 2 (Req 3.3 境界): MarkRotated 成功 → 二度目で ErrRefreshTokenAlreadyRotated
	t.Run("MarkRotated_rotation二度目はErrRefreshTokenAlreadyRotated", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "mark-rotated@test.com")
		family := newTestRefreshTokenFamily(userID)
		if err := repo.CreateFamily(ctx, family); err != nil {
			t.Fatalf("CreateFamily に失敗: %v", err)
		}
		token := newTestRefreshToken(family.ID, userID, "hash-mark-rotated-0123456789abcd", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, token); err != nil {
			t.Fatalf("CreateToken に失敗: %v", err)
		}

		// 1 回目: 成功
		rotatedAt := time.Now().UTC().Truncate(time.Microsecond)
		if err := repo.MarkRotated(ctx, token.ID, rotatedAt); err != nil {
			t.Fatalf("MarkRotated 1 回目で失敗: %v", err)
		}

		// 状態確認: rotated_at が set されている
		got, err := repo.FindByHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("MarkRotated 後の FindByHash が nil")
		}
		if got.RotatedAt == nil {
			t.Fatal("MarkRotated 後の RotatedAt が nil（set されていない）")
		}
		if !got.RotatedAt.Equal(rotatedAt) {
			t.Errorf("RotatedAt 不一致: got %v, want %v", *got.RotatedAt, rotatedAt)
		}

		// 2 回目: ErrRefreshTokenAlreadyRotated
		secondRotatedAt := rotatedAt.Add(time.Second)
		err = repo.MarkRotated(ctx, token.ID, secondRotatedAt)
		if !errors.Is(err, ErrRefreshTokenAlreadyRotated) {
			t.Errorf("MarkRotated 2 回目で ErrRefreshTokenAlreadyRotated 以外を返した: %v", err)
		}

		// 状態確認: 既存 rotated_at が保持されている（永続化状態を変更していない）
		got2, err := repo.FindByHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got2.RotatedAt == nil || !got2.RotatedAt.Equal(rotatedAt) {
			t.Errorf("MarkRotated 失敗後の RotatedAt が変更されている: got %v, want %v", got2.RotatedAt, rotatedAt)
		}
	})

	// Case 3 (Req 3.4): RevokeFamily 後、配下の全 token の revoked_at が non-NULL
	t.Run("RevokeFamily_配下の全tokenにrevoked_atがset", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "revoke-family@test.com")
		family := newTestRefreshTokenFamily(userID)
		if err := repo.CreateFamily(ctx, family); err != nil {
			t.Fatalf("CreateFamily に失敗: %v", err)
		}
		// family に 2 つの token を作成
		tokenA := newTestRefreshToken(family.ID, userID, "hash-revoke-family-a-0123456789", time.Now().Add(24*time.Hour).UTC())
		tokenB := newTestRefreshToken(family.ID, userID, "hash-revoke-family-b-0123456789", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, tokenA); err != nil {
			t.Fatalf("CreateToken A に失敗: %v", err)
		}
		if err := repo.CreateToken(ctx, tokenB); err != nil {
			t.Fatalf("CreateToken B に失敗: %v", err)
		}

		revokedAt := time.Now().UTC().Truncate(time.Microsecond)
		if err := repo.RevokeFamily(ctx, family.ID, revokedAt); err != nil {
			t.Fatalf("RevokeFamily に失敗: %v", err)
		}

		// 配下の全 token の revoked_at が non-NULL
		gotA, err := repo.FindByHash(ctx, tokenA.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash A に失敗: %v", err)
		}
		if gotA == nil || gotA.RevokedAt == nil {
			t.Errorf("token A の RevokedAt が nil（set されていない）: %+v", gotA)
		}
		gotB, err := repo.FindByHash(ctx, tokenB.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash B に失敗: %v", err)
		}
		if gotB == nil || gotB.RevokedAt == nil {
			t.Errorf("token B の RevokedAt が nil（set されていない）: %+v", gotB)
		}

		// family の revoked_at も non-NULL
		var familyRevokedAt sql.NullTime
		if err := db.QueryRow(
			`SELECT revoked_at FROM refresh_token_families WHERE id = $1`, family.ID,
		).Scan(&familyRevokedAt); err != nil {
			t.Fatalf("family revoked_at 取得に失敗: %v", err)
		}
		if !familyRevokedAt.Valid {
			t.Error("family の revoked_at が NULL（set されていない）")
		}
	})

	// Case 4 (境界): RevokeFamily 二重呼び出しが冪等に成功（既存 revoked_at が保持される）
	t.Run("RevokeFamily_二重呼び出しが冪等に成功し既存revoked_atを保持する", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "revoke-twice@test.com")
		family := newTestRefreshTokenFamily(userID)
		if err := repo.CreateFamily(ctx, family); err != nil {
			t.Fatalf("CreateFamily に失敗: %v", err)
		}
		token := newTestRefreshToken(family.ID, userID, "hash-revoke-twice-0123456789abcd", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, token); err != nil {
			t.Fatalf("CreateToken に失敗: %v", err)
		}

		firstRevokedAt := time.Now().UTC().Truncate(time.Microsecond)
		if err := repo.RevokeFamily(ctx, family.ID, firstRevokedAt); err != nil {
			t.Fatalf("RevokeFamily 1 回目で失敗: %v", err)
		}

		// 2 回目: 別 revoked_at で呼び出しても冪等成功（既存値が保持される）
		secondRevokedAt := firstRevokedAt.Add(1 * time.Hour)
		if err := repo.RevokeFamily(ctx, family.ID, secondRevokedAt); err != nil {
			t.Fatalf("RevokeFamily 2 回目で失敗（冪等性違反）: %v", err)
		}

		// 既存 token.revoked_at が 1 回目の値のまま保持されている
		got, err := repo.FindByHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil || got.RevokedAt == nil {
			t.Fatal("token の RevokedAt が nil（set されていない）")
		}
		if !got.RevokedAt.Equal(firstRevokedAt) {
			t.Errorf("token の RevokedAt が 2 回目の値で上書きされている: got %v, want %v (1 回目の値)",
				*got.RevokedAt, firstRevokedAt)
		}

		// family の revoked_at も 1 回目の値のまま保持されている
		var familyRevokedAt sql.NullTime
		if err := db.QueryRow(
			`SELECT revoked_at FROM refresh_token_families WHERE id = $1`, family.ID,
		).Scan(&familyRevokedAt); err != nil {
			t.Fatalf("family revoked_at 取得に失敗: %v", err)
		}
		if !familyRevokedAt.Valid {
			t.Fatal("family の revoked_at が NULL（set されていない）")
		}
		if !familyRevokedAt.Time.UTC().Equal(firstRevokedAt) {
			t.Errorf("family の revoked_at が 2 回目の値で上書きされている: got %v, want %v (1 回目の値)",
				familyRevokedAt.Time.UTC(), firstRevokedAt)
		}
	})

	// Case 5 (Req 3.6): DeleteByUserID 後、当該ユーザーの family/token が 0 件、他ユーザーには影響なし
	t.Run("DeleteByUserID_当該userのみ削除し他userに影響しない", func(t *testing.T) {
		userA := insertTestUserForRefreshToken(t, db, "delete-target@test.com")
		userB := insertTestUserForRefreshToken(t, db, "delete-bystander@test.com")

		// user A: family + token を 1 件作成
		familyA := newTestRefreshTokenFamily(userA)
		if err := repo.CreateFamily(ctx, familyA); err != nil {
			t.Fatalf("CreateFamily A に失敗: %v", err)
		}
		tokenA := newTestRefreshToken(familyA.ID, userA, "hash-delete-target-0123456789abcd", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, tokenA); err != nil {
			t.Fatalf("CreateToken A に失敗: %v", err)
		}

		// user B: family + token を 1 件作成（削除対象外）
		familyB := newTestRefreshTokenFamily(userB)
		if err := repo.CreateFamily(ctx, familyB); err != nil {
			t.Fatalf("CreateFamily B に失敗: %v", err)
		}
		tokenB := newTestRefreshToken(familyB.ID, userB, "hash-delete-bystander-0123456789a", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, tokenB); err != nil {
			t.Fatalf("CreateToken B に失敗: %v", err)
		}

		// user A を DeleteByUserID
		if err := repo.DeleteByUserID(ctx, userA); err != nil {
			t.Fatalf("DeleteByUserID に失敗: %v", err)
		}

		// user A の family / token が 0 件（refresh_tokens は FK CASCADE で削除される）
		var familyCountA int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_token_families WHERE user_id = $1`, userA,
		).Scan(&familyCountA); err != nil {
			t.Fatalf("user A family COUNT 取得に失敗: %v", err)
		}
		if familyCountA != 0 {
			t.Errorf("user A の family が削除されていない: got %d, want 0", familyCountA)
		}
		var tokenCountA int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, userA,
		).Scan(&tokenCountA); err != nil {
			t.Fatalf("user A token COUNT 取得に失敗: %v", err)
		}
		if tokenCountA != 0 {
			t.Errorf("user A の token が FK CASCADE で削除されていない: got %d, want 0", tokenCountA)
		}

		// user B の family / token は影響を受けず残っている
		gotB, err := repo.FindByHash(ctx, tokenB.TokenHash)
		if err != nil {
			t.Fatalf("FindByHash B に失敗: %v", err)
		}
		if gotB == nil {
			t.Error("user B の token が DeleteByUserID(A) で削除された（他 user 影響）")
		}
		var familyCountB int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_token_families WHERE user_id = $1`, userB,
		).Scan(&familyCountB); err != nil {
			t.Fatalf("user B family COUNT 取得に失敗: %v", err)
		}
		if familyCountB != 1 {
			t.Errorf("user B の family が DeleteByUserID(A) で削除された: got %d, want 1", familyCountB)
		}
	})

	// Case 8 (Issue #170 Req 1.2, 2.1): DeleteByUserIDExec が共有 tx に参加し、
	// 配下 tokens も FK CASCADE で削除される。他 user は影響を受けない
	t.Run("DeleteByUserIDExec_共有txでCommitすると当該userのfamily+tokensが消えて他userは残る", func(t *testing.T) {
		userA := insertTestUserForRefreshToken(t, db, "issue170-exec-target@test.com")
		userB := insertTestUserForRefreshToken(t, db, "issue170-exec-bystander@test.com")

		// user A: family + token を 1 件
		familyA := newTestRefreshTokenFamily(userA)
		if err := repo.CreateFamily(ctx, familyA); err != nil {
			t.Fatalf("CreateFamily A に失敗: %v", err)
		}
		tokenA := newTestRefreshToken(familyA.ID, userA, "hash-issue170-exec-a-0123456789abcd", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, tokenA); err != nil {
			t.Fatalf("CreateToken A に失敗: %v", err)
		}

		// user B: family + token を 1 件（削除対象外）
		familyB := newTestRefreshTokenFamily(userB)
		if err := repo.CreateFamily(ctx, familyB); err != nil {
			t.Fatalf("CreateFamily B に失敗: %v", err)
		}
		tokenB := newTestRefreshToken(familyB.ID, userB, "hash-issue170-exec-b-0123456789abcd", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, tokenB); err != nil {
			t.Fatalf("CreateToken B に失敗: %v", err)
		}

		// Act: 共有 tx を開いて user A の DeleteByUserIDExec を呼び、Commit する
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx に失敗: %v", err)
		}
		if err := repo.DeleteByUserIDExec(ctx, tx, userA); err != nil {
			_ = tx.Rollback()
			t.Fatalf("DeleteByUserIDExec に失敗: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit に失敗: %v", err)
		}

		// Assert: user A の family / token は 0 件
		var familyCountA, tokenCountA int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_token_families WHERE user_id = $1`, userA,
		).Scan(&familyCountA); err != nil {
			t.Fatalf("user A family COUNT に失敗: %v", err)
		}
		if familyCountA != 0 {
			t.Errorf("user A の family が削除されていない: got %d, want 0", familyCountA)
		}
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, userA,
		).Scan(&tokenCountA); err != nil {
			t.Fatalf("user A token COUNT に失敗: %v", err)
		}
		if tokenCountA != 0 {
			t.Errorf("user A の token が FK CASCADE で削除されていない: got %d, want 0", tokenCountA)
		}

		// Assert: user B の family / token は影響を受けず残っている
		var familyCountB, tokenCountB int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_token_families WHERE user_id = $1`, userB,
		).Scan(&familyCountB); err != nil {
			t.Fatalf("user B family COUNT に失敗: %v", err)
		}
		if familyCountB != 1 {
			t.Errorf("user B の family が DeleteByUserIDExec(A) で削除された: got %d, want 1", familyCountB)
		}
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, userB,
		).Scan(&tokenCountB); err != nil {
			t.Fatalf("user B token COUNT に失敗: %v", err)
		}
		if tokenCountB != 1 {
			t.Errorf("user B の token が DeleteByUserIDExec(A) で削除された: got %d, want 1", tokenCountB)
		}
	})

	// Case 9 (Issue #170 Req 2.2): DeleteByUserIDExec を共有 tx 上で Rollback すると削除が取り消される
	t.Run("DeleteByUserIDExec_Rollbackすると削除が取り消される", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "issue170-exec-rollback@test.com")
		family := newTestRefreshTokenFamily(userID)
		if err := repo.CreateFamily(ctx, family); err != nil {
			t.Fatalf("CreateFamily に失敗: %v", err)
		}
		token := newTestRefreshToken(family.ID, userID, "hash-issue170-exec-rollback-01234", time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, token); err != nil {
			t.Fatalf("CreateToken に失敗: %v", err)
		}

		// Act: 共有 tx 上で DeleteByUserIDExec を呼び Rollback する
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx に失敗: %v", err)
		}
		if err := repo.DeleteByUserIDExec(ctx, tx, userID); err != nil {
			_ = tx.Rollback()
			t.Fatalf("DeleteByUserIDExec に失敗: %v", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback に失敗: %v", err)
		}

		// Assert: Rollback により削除が取り消され、family / token は残っている
		var familyCount, tokenCount int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_token_families WHERE user_id = $1`, userID,
		).Scan(&familyCount); err != nil {
			t.Fatalf("family COUNT に失敗: %v", err)
		}
		if familyCount != 1 {
			t.Errorf("Rollback 後の family が残存していない: got %d, want 1", familyCount)
		}
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, userID,
		).Scan(&tokenCount); err != nil {
			t.Fatalf("token COUNT に失敗: %v", err)
		}
		if tokenCount != 1 {
			t.Errorf("Rollback 後の token が残存していない: got %d, want 1", tokenCount)
		}
	})

	// Case 10 (Issue #170 Req 1.2 冪等): DeleteByUserIDExec は対象 0 件でも成功する
	t.Run("DeleteByUserIDExec_対象0件でも成功する", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "issue170-exec-empty@test.com")
		// family / token を一切作成しない状態
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx に失敗: %v", err)
		}
		if err := repo.DeleteByUserIDExec(ctx, tx, userID); err != nil {
			_ = tx.Rollback()
			t.Errorf("0 件削除でエラーを返した（冪等性違反）: %v", err)
		}
		_ = tx.Commit()
	})

	// Case 7 (NFR 1.1 セキュリティ回帰): 平文 token 文字列で逆引き SELECT しても 0 件
	// 永続化領域に「平文 token を書いていない」ことを自動検出するための回帰テスト。
	// Create は token_hash を保存するため、平文の "plain-token-xxx" 文字列を直接
	// token_hash カラムへ SELECT に投げてもヒットしてはならない。
	t.Run("平文tokenでSELECTしても0件を返す_NFR1.1回帰", func(t *testing.T) {
		userID := insertTestUserForRefreshToken(t, db, "plaintext-regression@test.com")
		family := newTestRefreshTokenFamily(userID)
		if err := repo.CreateFamily(ctx, family); err != nil {
			t.Fatalf("CreateFamily に失敗: %v", err)
		}
		// 平文相当の文字列を仮定し、その hash 表現を別途保存する。
		plainToken := "plain-token-xxx-regression-12345"
		// 実装上、hash は平文と異なる文字列であることを表現するため "hash::" prefix を付ける。
		tokenHash := "hash::" + plainToken
		token := newTestRefreshToken(family.ID, userID, tokenHash, time.Now().Add(24*time.Hour).UTC())
		if err := repo.CreateToken(ctx, token); err != nil {
			t.Fatalf("CreateToken に失敗: %v", err)
		}

		// 平文文字列を token_hash カラムへ直接 SELECT すると 0 件
		var countByPlain int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = $1`, plainToken,
		).Scan(&countByPlain); err != nil {
			t.Fatalf("平文 SELECT (token_hash) に失敗: %v", err)
		}
		if countByPlain != 0 {
			t.Errorf("平文 token が token_hash として書かれている: got %d, want 0 (NFR 1.1 違反)", countByPlain)
		}

		// hash 値で SELECT すれば 1 件ヒット（保存自体は成立している）
		var countByHash int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = $1`, tokenHash,
		).Scan(&countByHash); err != nil {
			t.Fatalf("hash SELECT に失敗: %v", err)
		}
		if countByHash != 1 {
			t.Errorf("hash 値での SELECT 件数が不正: got %d, want 1（保存自体が失敗している可能性）", countByHash)
		}
	})

	// Case 6 (Req 3.5 異常系 not-found): 存在しない hash で FindByHash が (nil, nil) を返す
	t.Run("FindByHash_存在しないhashで(nil,nil)を返す", func(t *testing.T) {
		got, err := repo.FindByHash(ctx, "hash-does-not-exist-deadbeef-ref")
		if err != nil {
			t.Fatalf("FindByHash がエラーを返した: %v", err)
		}
		if got != nil {
			t.Errorf("FindByHash が non-nil を返した（nil 期待）: %+v", got)
		}
	})
}
