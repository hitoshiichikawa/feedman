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
