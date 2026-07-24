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

// 本ファイルは PostgresAuthCodeRepo の結合テスト（design.md §Testing Strategy の
// AuthCodeRepo 5 ケース）を実装する。
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。
// DB へ接続できない環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。
// 慣習は postgres_subscription_repo_db_test.go と統一している（Req NFR 3.1）。

// authCodeTestDatabaseURL はテスト用のデータベース URL を返す。
func authCodeTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupAuthCodeTestDB はテスト用 DB を準備しマイグレーション適用済みの *sql.DB を返す。
// DB に接続できない場合は t.Skip でテストをスキップする（外部ネットワーク依存なし、NFR 3.1）。
func setupAuthCodeTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := authCodeTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	// クリーンアップ: 既存テーブルとマイグレーション履歴をリセットしてクリーンな状態にする。
	// 新規 native auth テーブル（auth_codes / refresh_token_families / refresh_tokens）も
	// 明示 DROP しておく（同一 DB を繰り返し利用するローカル開発機でも fresh up を保証）。
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

// insertTestUserForAuthCode はテスト用ユーザーを作成し、その ID を返す。
func insertTestUserForAuthCode(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		email, "Auth Code Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}
	return userID
}

// newTestAuthCode は AuthCode の各フィールドを設定したテスト用 struct を組み立てる。
// ID / CreatedAt は repo 側のデフォルトで決まることを想定し空のままにする。
func newTestAuthCode(userID, codeHash string, expiresAt time.Time) *model.AuthCode {
	return &model.AuthCode{
		CodeHash:      codeHash,
		UserID:        userID,
		PKCEChallenge: "test-pkce-challenge-s256",
		ExpiresAt:     expiresAt,
		Used:          false,
	}
}

// TestPostgresAuthCodeRepo_DB は design.md §Testing Strategy の AuthCodeRepo 5 ケースを
// 1 つのテーブルライク構成で実装する（個別の t.Run でケースを分離）。
// 各サブテストは個別のユーザー・auth_code を生成して相互に干渉しない設計。
func TestPostgresAuthCodeRepo_DB(t *testing.T) {
	db := setupAuthCodeTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresAuthCodeRepo(db)

	// Case 1 (Req 2.1, 2.4): Create → FindByHash で同値が返り、code_hash でしかヒットしない
	t.Run("Create_FindByHashで保存値が同値復元される", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "create-find@test.com")
		expiresAt := time.Now().Add(60 * time.Second).UTC().Truncate(time.Microsecond)
		code := newTestAuthCode(userID, "hash-create-find-0123456789abcdef", expiresAt)

		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}
		if code.ID == "" {
			t.Fatal("Create 後の code.ID が空: DB デフォルトが反映されていない")
		}
		if code.CreatedAt.IsZero() {
			t.Fatal("Create 後の code.CreatedAt が zero: DB デフォルトが反映されていない")
		}

		got, err := repo.FindByHash(ctx, code.CodeHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByHash が nil を返した（保存済み hash がヒットしない）")
		}
		if got.ID != code.ID {
			t.Errorf("ID 不一致: got %q, want %q", got.ID, code.ID)
		}
		if got.CodeHash != code.CodeHash {
			t.Errorf("CodeHash 不一致: got %q, want %q", got.CodeHash, code.CodeHash)
		}
		if got.UserID != userID {
			t.Errorf("UserID 不一致: got %q, want %q", got.UserID, userID)
		}
		if got.PKCEChallenge != code.PKCEChallenge {
			t.Errorf("PKCEChallenge 不一致: got %q, want %q", got.PKCEChallenge, code.PKCEChallenge)
		}
		if got.Used {
			t.Error("作成直後の Used が true になっている（false 期待）")
		}
		// expires_at は TIMESTAMPTZ で micro 秒精度。Truncate 後の値と一致するはず。
		if !got.ExpiresAt.Equal(expiresAt) {
			t.Errorf("ExpiresAt 不一致: got %v, want %v", got.ExpiresAt, expiresAt)
		}
	})

	// Case 2 (Req 2.4 異常系 not-found): 存在しない hash で FindByHash が (nil, nil) を返す
	t.Run("FindByHash_存在しないhashで(nil,nil)を返す", func(t *testing.T) {
		got, err := repo.FindByHash(ctx, "hash-does-not-exist-deadbeef")
		if err != nil {
			t.Fatalf("FindByHash がエラーを返した: %v", err)
		}
		if got != nil {
			t.Errorf("FindByHash が non-nil を返した（nil 期待）: %+v", got)
		}
	})

	// Case 3 (Req 2.5, 2.6 境界): MarkUsed が一度成功し、二度目で ErrAuthCodeNotUsable を返す
	t.Run("MarkUsed_単回利用境界_二度目はErrAuthCodeNotUsable", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "mark-used@test.com")
		expiresAt := time.Now().Add(60 * time.Second).UTC()
		code := newTestAuthCode(userID, "hash-mark-used-0123456789abcdef0", expiresAt)
		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// 1 回目: 成功
		if err := repo.MarkUsed(ctx, code.ID); err != nil {
			t.Fatalf("MarkUsed 1 回目で失敗: %v", err)
		}

		// 状態確認: used = true に遷移している
		got, err := repo.FindByHash(ctx, code.CodeHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("MarkUsed 後の FindByHash が nil")
		}
		if !got.Used {
			t.Error("MarkUsed 後の Used が false（true 期待）")
		}

		// 2 回目: ErrAuthCodeNotUsable
		err = repo.MarkUsed(ctx, code.ID)
		if !errors.Is(err, ErrAuthCodeNotUsable) {
			t.Errorf("MarkUsed 2 回目で ErrAuthCodeNotUsable 以外を返した: %v", err)
		}
	})

	// Case 4 (Req 2.6 境界 期限切れ): expires_at を過去にした auth_code に MarkUsed すると ErrAuthCodeNotUsable
	t.Run("MarkUsed_期限切れ境界_ErrAuthCodeNotUsable", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "expired@test.com")
		// 1 分前を expires_at に設定して即座に期限切れ
		expiresAt := time.Now().Add(-1 * time.Minute).UTC()
		code := newTestAuthCode(userID, "hash-expired-0123456789abcdef00ff", expiresAt)
		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		err := repo.MarkUsed(ctx, code.ID)
		if !errors.Is(err, ErrAuthCodeNotUsable) {
			t.Errorf("期限切れ MarkUsed で ErrAuthCodeNotUsable 以外を返した: %v", err)
		}

		// 状態確認: used は false のまま（永続化状態を変更していない、Req 2.6）
		got, err := repo.FindByHash(ctx, code.CodeHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByHash が nil（レコードが消えている）")
		}
		if got.Used {
			t.Error("期限切れ MarkUsed 失敗後の Used が true（変更されている）")
		}
	})

	// Case 6 (NFR 1.1 セキュリティ回帰): 平文 code 文字列で逆引き SELECT しても 0 件
	// 永続化領域に「平文 code を書いていない」ことを自動検出するための回帰テスト。
	// Create は code_hash を保存するため、平文の "plain-code-xxx" 文字列を直接
	// code_hash カラムへ SELECT に投げてもヒットしてはならない。
	t.Run("平文codeでSELECTしても0件を返す_NFR1.1回帰", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "plaintext-regression@test.com")
		// 平文相当の文字列を仮定し、その hash 表現を別途保存する。
		plainCode := "plain-code-xxx-regression-1234567"
		// 実装上、hash は平文と異なる文字列であることを表現するため "hash::" prefix を付ける。
		codeHash := "hash::" + plainCode
		code := newTestAuthCode(userID, codeHash, time.Now().Add(60*time.Second).UTC())
		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// 平文文字列を code_hash カラムへ直接 SELECT すると 0 件
		var countByPlain int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM auth_codes WHERE code_hash = $1`, plainCode,
		).Scan(&countByPlain); err != nil {
			t.Fatalf("平文 SELECT (code_hash) に失敗: %v", err)
		}
		if countByPlain != 0 {
			t.Errorf("平文 code が code_hash として書かれている: got %d, want 0 (NFR 1.1 違反)", countByPlain)
		}

		// pkce_challenge カラムへの平文混入も無いことを念のため確認（防御的回帰）
		var countByPKCE int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM auth_codes WHERE pkce_challenge = $1`, plainCode,
		).Scan(&countByPKCE); err != nil {
			t.Fatalf("平文 SELECT (pkce_challenge) に失敗: %v", err)
		}
		if countByPKCE != 0 {
			t.Errorf("平文 code が pkce_challenge として書かれている: got %d, want 0 (NFR 1.1 違反)", countByPKCE)
		}

		// 一方、hash 値で SELECT すれば 1 件ヒット（保存自体は成立している）
		var countByHash int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM auth_codes WHERE code_hash = $1`, codeHash,
		).Scan(&countByHash); err != nil {
			t.Fatalf("hash SELECT に失敗: %v", err)
		}
		if countByHash != 1 {
			t.Errorf("hash 値での SELECT 件数が不正: got %d, want 1（保存自体が失敗している可能性）", countByHash)
		}
	})

	// Case 7 (Issue #170 Req 1.1, 3.1): DeleteByUserID は当該 user のみ削除し、他 user に影響しない
	t.Run("DeleteByUserID_当該userのみ削除し他userに影響しない", func(t *testing.T) {
		userA := insertTestUserForAuthCode(t, db, "issue170-delete-target@test.com")
		userB := insertTestUserForAuthCode(t, db, "issue170-delete-bystander@test.com")

		// user A に 2 件、user B に 1 件の auth_code を作成
		codeA1 := newTestAuthCode(userA, "hash-issue170-a-1-0123456789abcd00", time.Now().Add(60*time.Second).UTC())
		codeA2 := newTestAuthCode(userA, "hash-issue170-a-2-0123456789abcd00", time.Now().Add(60*time.Second).UTC())
		codeB := newTestAuthCode(userB, "hash-issue170-b-0123456789abcd0000", time.Now().Add(60*time.Second).UTC())
		for _, c := range []*model.AuthCode{codeA1, codeA2, codeB} {
			if err := repo.Create(ctx, c); err != nil {
				t.Fatalf("Create に失敗: %v", err)
			}
		}

		// Act: user A の auth_code を一括削除
		if err := repo.DeleteByUserID(ctx, userA); err != nil {
			t.Fatalf("DeleteByUserID に失敗: %v", err)
		}

		// Assert: user A の auth_codes は 0 件
		var countA int
		if err := db.QueryRow(`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1`, userA).Scan(&countA); err != nil {
			t.Fatalf("user A COUNT 取得に失敗: %v", err)
		}
		if countA != 0 {
			t.Errorf("user A の auth_codes が残存している: got %d, want 0", countA)
		}

		// user B の auth_codes は 1 件残っている
		var countB int
		if err := db.QueryRow(`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1`, userB).Scan(&countB); err != nil {
			t.Fatalf("user B COUNT 取得に失敗: %v", err)
		}
		if countB != 1 {
			t.Errorf("user B の auth_codes が DeleteByUserID(A) で削除された: got %d, want 1", countB)
		}
	})

	// Case 8 (Issue #170 Req 1.1 冪等): 対象 0 件でも DeleteByUserID は成功する
	t.Run("DeleteByUserID_対象0件でも成功する", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "issue170-empty@test.com")
		// auth_code を一切作成しない状態で DeleteByUserID
		if err := repo.DeleteByUserID(ctx, userID); err != nil {
			t.Errorf("0 件削除でエラーを返した（冪等性違反）: %v", err)
		}
	})

	// Case 9 (Issue #170 Req 2.1): DeleteByUserIDExec が共有 tx に参加し、Rollback で取り消される
	t.Run("DeleteByUserIDExec_共有txに参加しRollbackで取り消される", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "issue170-tx-rollback@test.com")
		code := newTestAuthCode(userID, "hash-issue170-tx-rollback-01234567", time.Now().Add(60*time.Second).UTC())
		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// Act: 共有 tx を開いて DeleteByUserIDExec を呼び、Rollback する
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

		// Assert: Rollback により削除が取り消され、auth_code は依然存在する
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1`, userID,
		).Scan(&count); err != nil {
			t.Fatalf("COUNT 取得に失敗: %v", err)
		}
		if count != 1 {
			t.Errorf("Rollback 後の auth_codes が残存していない: got %d, want 1", count)
		}
	})

	// Case 10 (Issue #170 Req 2.1): DeleteByUserIDExec を共有 tx 上で Commit すると削除が確定する
	t.Run("DeleteByUserIDExec_共有txでCommitすると削除が確定する", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "issue170-tx-commit@test.com")
		code := newTestAuthCode(userID, "hash-issue170-tx-commit-0123456789", time.Now().Add(60*time.Second).UTC())
		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// Act: 共有 tx を開いて DeleteByUserIDExec を呼び、Commit する
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx に失敗: %v", err)
		}
		if err := repo.DeleteByUserIDExec(ctx, tx, userID); err != nil {
			_ = tx.Rollback()
			t.Fatalf("DeleteByUserIDExec に失敗: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit に失敗: %v", err)
		}

		// Assert: Commit により削除が確定し、auth_code は 0 件
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1`, userID,
		).Scan(&count); err != nil {
			t.Fatalf("COUNT 取得に失敗: %v", err)
		}
		if count != 0 {
			t.Errorf("Commit 後の auth_codes が削除されていない: got %d, want 0", count)
		}
	})

	// Case 5 (Req 1.5 / 3.6 同系統 cascade): users 削除時に auth_codes が cascade 削除される
	t.Run("users削除時にauth_codesがCASCADE削除される", func(t *testing.T) {
		userID := insertTestUserForAuthCode(t, db, "cascade@test.com")
		code := newTestAuthCode(userID, "hash-cascade-0123456789abcdef00aa", time.Now().Add(60*time.Second).UTC())
		if err := repo.Create(ctx, code); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// 削除前にレコードが存在することを確認
		var beforeCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1`, userID).Scan(&beforeCount); err != nil {
			t.Fatalf("削除前 COUNT 取得に失敗: %v", err)
		}
		if beforeCount != 1 {
			t.Fatalf("削除前 auth_codes 件数が不正: got %d, want 1", beforeCount)
		}

		// users 削除
		if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Fatalf("users 削除に失敗: %v", err)
		}

		// CASCADE で auth_codes も削除されているはず
		var afterCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM auth_codes WHERE user_id = $1`, userID).Scan(&afterCount); err != nil {
			t.Fatalf("削除後 COUNT 取得に失敗: %v", err)
		}
		if afterCount != 0 {
			t.Errorf("CASCADE 削除が機能していない: got %d, want 0", afterCount)
		}
	})
}
