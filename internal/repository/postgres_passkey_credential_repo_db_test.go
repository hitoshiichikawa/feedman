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

// 本ファイルは PostgresPasskeyCredentialRepo の結合テスト（Issue #216 task 2）を実装する。
// 慣習は postgres_auth_code_repo_db_test.go と統一している（Req NFR 4.1）。
// DB へ接続できない環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。

// passkeyTestDatabaseURL はテスト用のデータベース URL を返す。
func passkeyTestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://feedman:feedman@localhost:5432/feedman_test?sslmode=disable"
}

// setupPasskeyTestDB はテスト用 DB を準備しマイグレーション適用済みの *sql.DB を返す。
// DB に接続できない場合は t.Skip でテストをスキップする（NFR 4.1）。
// passkey_credentials / passkey_challenges を明示 DROP するクリーンアップを含む。
func setupPasskeyTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := passkeyTestDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("データベースへの接続に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("テスト用データベースに接続できません（スキップ）: %v", err)
	}

	// クリーンアップ: passkey 2 テーブルを含む全テーブルと migration 履歴をリセットする。
	// 同一 DB を繰り返し利用するローカル開発機でも fresh up を保証する。
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

// insertPasskeyTestUser はテスト用ユーザーを作成し、その ID を返す。
func insertPasskeyTestUser(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		email, "Passkey Credential Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("ユーザー挿入に失敗: %v", err)
	}
	return userID
}

// newTestPasskeyCredential は PasskeyCredential のテスト用 struct を組み立てる。
// ID / CreatedAt は repo 側のデフォルトで決まるため空のままにする。
func newTestPasskeyCredential(userID string, credentialID, publicKey []byte) *model.PasskeyCredential {
	return &model.PasskeyCredential{
		UserID:          userID,
		CredentialID:    credentialID,
		PublicKey:       publicKey,
		SignCount:       0,
		AttestationType: "none",
		AAGUID:          nil,
		Transports:      []string{"internal"},
	}
}

// TestPostgresPasskeyCredentialRepo_DB は PostgresPasskeyCredentialRepo の主要 6 ケースを
// 1 つのテーブルライク構成で実装する（Issue #216 task 2 の検証項目）。
// 各サブテストは個別のユーザー・credential を作成して相互に干渉しない設計。
func TestPostgresPasskeyCredentialRepo_DB(t *testing.T) {
	db := setupPasskeyTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresPasskeyCredentialRepo(db)

	// Case 1 (Req 1.2 / 3.2 / Issue #234 Req 2.1): Create → FindByCredentialID で同値復元
	// BE=true / BS=true を含む round-trip も検証し、保存値が復元値と同値であることを確認する。
	t.Run("Create_FindByCredentialIDで保存値が同値復元される", func(t *testing.T) {
		userID := insertPasskeyTestUser(t, db, "cred-create-find@test.com")
		credentialID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
		publicKey := []byte{0xaa, 0xbb, 0xcc, 0xdd}
		c := newTestPasskeyCredential(userID, credentialID, publicKey)
		c.AttestationType = "packed"
		c.AAGUID = []byte{0xde, 0xad, 0xbe, 0xef}
		c.Transports = []string{"internal", "hybrid"}
		c.SignCount = 42
		// Issue #234: BE=true / BS=true の round-trip 検証（Req 2.1〜2.3）。
		c.BackupEligible = true
		c.BackupState = true

		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}
		if c.ID == "" {
			t.Fatal("Create 後の c.ID が空: DB デフォルトが反映されていない")
		}
		if c.CreatedAt.IsZero() {
			t.Fatal("Create 後の c.CreatedAt が zero: DB デフォルトが反映されていない")
		}

		got, err := repo.FindByCredentialID(ctx, credentialID)
		if err != nil {
			t.Fatalf("FindByCredentialID に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByCredentialID が nil を返した（保存済み credential がヒットしない）")
		}
		if got.ID != c.ID {
			t.Errorf("ID 不一致: got %q, want %q", got.ID, c.ID)
		}
		if got.UserID != userID {
			t.Errorf("UserID 不一致: got %q, want %q", got.UserID, userID)
		}
		if string(got.CredentialID) != string(credentialID) {
			t.Errorf("CredentialID 不一致: got %v, want %v", got.CredentialID, credentialID)
		}
		if string(got.PublicKey) != string(publicKey) {
			t.Errorf("PublicKey 不一致: got %v, want %v", got.PublicKey, publicKey)
		}
		if got.SignCount != 42 {
			t.Errorf("SignCount 不一致: got %d, want 42", got.SignCount)
		}
		if got.AttestationType != "packed" {
			t.Errorf("AttestationType 不一致: got %q, want %q", got.AttestationType, "packed")
		}
		if string(got.AAGUID) != string([]byte{0xde, 0xad, 0xbe, 0xef}) {
			t.Errorf("AAGUID 不一致: got %v", got.AAGUID)
		}
		if len(got.Transports) != 2 || got.Transports[0] != "internal" || got.Transports[1] != "hybrid" {
			t.Errorf("Transports 不一致: got %v, want [internal hybrid]", got.Transports)
		}
		if got.LastUsedAt != nil {
			t.Errorf("初期 LastUsedAt が nil ではない: got %v", got.LastUsedAt)
		}
		// Issue #234 Req 2.1〜2.3: BE/BS が保存 → 復元で同値であること。
		if !got.BackupEligible {
			t.Errorf("BackupEligible 復元不一致: got %v, want true", got.BackupEligible)
		}
		if !got.BackupState {
			t.Errorf("BackupState 復元不一致: got %v, want true", got.BackupState)
		}
	})

	// Case 2 (Req 3.6 / 1.7): credential_id UNIQUE 衝突 → ErrCredentialAlreadyRegistered
	t.Run("Create_credential_id重複でErrCredentialAlreadyRegistered", func(t *testing.T) {
		userA := insertPasskeyTestUser(t, db, "cred-uniq-a@test.com")
		userB := insertPasskeyTestUser(t, db, "cred-uniq-b@test.com")
		credentialID := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}

		// user A に登録
		c1 := newTestPasskeyCredential(userA, credentialID, []byte{0xff})
		if err := repo.Create(ctx, c1); err != nil {
			t.Fatalf("Create (user A) に失敗: %v", err)
		}

		// user B に同一 credential_id を登録すると ErrCredentialAlreadyRegistered
		c2 := newTestPasskeyCredential(userB, credentialID, []byte{0xee})
		err := repo.Create(ctx, c2)
		if !errors.Is(err, ErrCredentialAlreadyRegistered) {
			t.Errorf("重複 credential_id で ErrCredentialAlreadyRegistered 以外を返した: %v", err)
		}
	})

	// Case 3 (Req 2.2 異常系 not-found): FindByCredentialID が未存在で (nil, nil)
	t.Run("FindByCredentialID_未存在で(nil,nil)を返す", func(t *testing.T) {
		got, err := repo.FindByCredentialID(ctx, []byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe})
		if err != nil {
			t.Fatalf("FindByCredentialID がエラーを返した: %v", err)
		}
		if got != nil {
			t.Errorf("FindByCredentialID が non-nil を返した（nil 期待）: %+v", got)
		}
	})

	// Case 4 (Req 3.4): ListByUserID で同一 user の複数 credential が返る
	t.Run("ListByUserID_同一userの複数credentialが返る", func(t *testing.T) {
		userID := insertPasskeyTestUser(t, db, "cred-list-multi@test.com")
		cred1 := newTestPasskeyCredential(userID, []byte{0xa0, 0x01}, []byte{0x01})
		cred2 := newTestPasskeyCredential(userID, []byte{0xa0, 0x02}, []byte{0x02})
		cred3 := newTestPasskeyCredential(userID, []byte{0xa0, 0x03}, []byte{0x03})
		for _, c := range []*model.PasskeyCredential{cred1, cred2, cred3} {
			if err := repo.Create(ctx, c); err != nil {
				t.Fatalf("Create に失敗: %v", err)
			}
		}

		list, err := repo.ListByUserID(ctx, userID)
		if err != nil {
			t.Fatalf("ListByUserID に失敗: %v", err)
		}
		if len(list) != 3 {
			t.Errorf("ListByUserID 件数不一致: got %d, want 3", len(list))
		}
	})

	// Case 5 (Issue #234 / Req 4.1, 4.2, 4.3): UpdateAuthenticationState で
	// sign_count / backup_state / last_used_at が反映され、backup_eligible は不変であることを確認する。
	//
	// 登録時 BE=true / BS=false の credential を作り、UpdateAuthenticationState で
	// (signCount=100, backupState=true, lastUsedAt=now) を渡した後に:
	//   - SignCount / BackupState / LastUsedAt は最新値へ反映される（Req 4.1, 4.2）
	//   - BackupEligible は登録時の true のまま不変（Req 4.3 / SQL SET 句に含まれないため）
	// を検証する。
	t.Run("UpdateAuthenticationState_sign_count_backup_state_last_used_atが反映される", func(t *testing.T) {
		userID := insertPasskeyTestUser(t, db, "cred-updateauthnstate@test.com")
		c := newTestPasskeyCredential(userID, []byte{0xb0, 0x01}, []byte{0x11})
		// Issue #234 Req 4.3 検証用: 登録時 BE=true / BS=false にすることで、
		// UpdateAuthenticationState 後に BE が不変で保持されることを assert できる。
		c.BackupEligible = true
		c.BackupState = false
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		newSignCount := uint32(100)
		newLastUsedAt := time.Now().UTC().Truncate(time.Microsecond)
		if err := repo.UpdateAuthenticationState(ctx, c.ID, newSignCount, true, newLastUsedAt); err != nil {
			t.Fatalf("UpdateAuthenticationState に失敗: %v", err)
		}

		got, err := repo.FindByCredentialID(ctx, c.CredentialID)
		if err != nil {
			t.Fatalf("FindByCredentialID に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByCredentialID が nil を返した")
		}
		if got.SignCount != newSignCount {
			t.Errorf("SignCount 更新反映されず: got %d, want %d", got.SignCount, newSignCount)
		}
		if got.LastUsedAt == nil {
			t.Fatal("LastUsedAt が nil のまま（更新反映されず）")
		}
		if !got.LastUsedAt.Equal(newLastUsedAt) {
			t.Errorf("LastUsedAt 不一致: got %v, want %v", got.LastUsedAt, newLastUsedAt)
		}
		// Req 4.2: BackupState は最新観測値（true）へ更新されている。
		if !got.BackupState {
			t.Errorf("BackupState 更新反映されず: got %v, want true", got.BackupState)
		}
		// Req 4.3: BackupEligible は登録時の値（true）のまま不変
		// （SQL SET 句に backup_eligible を含めない構造で担保）。
		if !got.BackupEligible {
			t.Errorf("BackupEligible が変化した（Req 4.3 違反 / 不変であるべき）: got %v, want true",
				got.BackupEligible)
		}
	})

	// Case 5b (NFR 1.3 regression): 本修正前に登録された credential 行を模して、
	// backup_eligible / backup_state を INSERT 対象から省いた行（DB DEFAULT false が入る）を
	// 直接 INSERT する。その後 FindByCredentialID で BE=false / BS=false のまま
	// エラーなく読み出せることを確認する（NFR 1.3: 旧行読み出しでアプリケーションを
	// 異常終了させない / DEFAULT false による安全側 fallback）。
	t.Run("FindByCredentialID_旧行(BE_BS列DEFAULT)を安全に読み出す_NFR1.3", func(t *testing.T) {
		userID := insertPasskeyTestUser(t, db, "cred-legacy-row@test.com")
		credentialID := []byte{0xb0, 0x99}
		publicKey := []byte{0x99}
		// backup_eligible / backup_state を INSERT 列に含めず、DB DEFAULT false に任せる。
		// これは migration 適用前に永続化された行（本修正導入前の credential）と同じ状態。
		if _, err := db.ExecContext(ctx,
			`INSERT INTO passkey_credentials (
			     user_id, credential_id, public_key, sign_count,
			     attestation_type, transports
			 ) VALUES ($1, $2, $3, 0, 'none', 'internal')`,
			userID, credentialID, publicKey,
		); err != nil {
			t.Fatalf("旧行 INSERT に失敗: %v", err)
		}

		got, err := repo.FindByCredentialID(ctx, credentialID)
		if err != nil {
			t.Fatalf("旧行 FindByCredentialID がエラーを返した（NFR 1.3 違反）: %v", err)
		}
		if got == nil {
			t.Fatal("旧行 FindByCredentialID が nil を返した（保存済み credential がヒットしない）")
		}
		if got.BackupEligible {
			t.Errorf("旧行 BackupEligible が DEFAULT false ではない: got %v, want false", got.BackupEligible)
		}
		if got.BackupState {
			t.Errorf("旧行 BackupState が DEFAULT false ではない: got %v, want false", got.BackupState)
		}
	})

	// Case 6 (Req 7.1 / 7.4): DeleteByUserID は当該 user のみ削除し他 user に影響しない
	t.Run("DeleteByUserID_当該userのみ削除し他userに影響しない", func(t *testing.T) {
		userA := insertPasskeyTestUser(t, db, "cred-del-a@test.com")
		userB := insertPasskeyTestUser(t, db, "cred-del-b@test.com")

		credA1 := newTestPasskeyCredential(userA, []byte{0xc0, 0x01}, []byte{0x01})
		credA2 := newTestPasskeyCredential(userA, []byte{0xc0, 0x02}, []byte{0x02})
		credB := newTestPasskeyCredential(userB, []byte{0xc0, 0x03}, []byte{0x03})
		for _, c := range []*model.PasskeyCredential{credA1, credA2, credB} {
			if err := repo.Create(ctx, c); err != nil {
				t.Fatalf("Create に失敗: %v", err)
			}
		}

		// Act: user A の credential を一括削除
		if err := repo.DeleteByUserID(ctx, userA); err != nil {
			t.Fatalf("DeleteByUserID に失敗: %v", err)
		}

		// Assert: user A は 0 件
		listA, err := repo.ListByUserID(ctx, userA)
		if err != nil {
			t.Fatalf("ListByUserID (A) に失敗: %v", err)
		}
		if len(listA) != 0 {
			t.Errorf("user A の credential が残存: got %d, want 0", len(listA))
		}

		// user B は 1 件残っている（他 user に影響しない / Req 7.4）
		listB, err := repo.ListByUserID(ctx, userB)
		if err != nil {
			t.Fatalf("ListByUserID (B) に失敗: %v", err)
		}
		if len(listB) != 1 {
			t.Errorf("user B の credential が影響を受けた: got %d, want 1", len(listB))
		}
	})

	// Case 7 (Req 7.1 冪等): DeleteByUserID は対象 0 件でも成功する
	t.Run("DeleteByUserID_対象0件でも成功する", func(t *testing.T) {
		userID := insertPasskeyTestUser(t, db, "cred-del-empty@test.com")
		if err := repo.DeleteByUserID(ctx, userID); err != nil {
			t.Errorf("0 件削除でエラーを返した（冪等性違反）: %v", err)
		}
	})

	// Case 8 (NFR 1.1 回帰): 秘密情報を持たない = 生 challenge / 平文パスワード等の
	// 列は passkey_credentials スキーマに存在しない。ここでは代表として、保存対象カラムが
	// 検証情報のみに限定されている（設計文書 §Physical Data Model と一致）ことをカラム名 SELECT で確認する。
	t.Run("保存対象が検証情報のみに限定される_NFR1.1回帰", func(t *testing.T) {
		// information_schema からカラム名一覧を取得
		rows, err := db.Query(
			`SELECT column_name FROM information_schema.columns
			 WHERE table_name = 'passkey_credentials'
			 ORDER BY column_name`,
		)
		if err != nil {
			t.Fatalf("information_schema 参照に失敗: %v", err)
		}
		defer rows.Close()

		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatalf("scan に失敗: %v", err)
			}
			cols = append(cols, c)
		}

		// 期待列（design.md §Physical Data Model と 1:1）
		// Issue #234: BackupEligible / BackupState 列を追加（NFR 1.1 の許容範囲 = 既存列不変 +
		// 検証情報のみに限定）。BE/BS は WebAuthn credential 検証情報のため NFR 1.1 に整合する。
		want := map[string]bool{
			"id": true, "user_id": true, "credential_id": true, "public_key": true,
			"sign_count": true, "attestation_type": true, "aaguid": true,
			"transports": true, "created_at": true, "last_used_at": true,
			"backup_eligible": true, "backup_state": true,
		}
		if len(cols) != len(want) {
			t.Errorf("カラム数が期待と異なる（秘密情報用の列が追加されている可能性 / NFR 1.1）: got %d cols %v, want %d",
				len(cols), cols, len(want))
		}
		for _, c := range cols {
			if !want[c] {
				t.Errorf("期待外のカラムが存在（NFR 1.1 違反の可能性）: %q", c)
			}
		}
	})

	// Case 9 (compile-time): PostgresPasskeyCredentialRepo が interface を充足する
	t.Run("compile_time_interface_check", func(t *testing.T) {
		var _ PasskeyCredentialRepository = (*PostgresPasskeyCredentialRepo)(nil)
	})
}

// TestPostgresPasskeyCredentialRepo_ImplementsInterface は DB 非接続でも実行可能な
// compile-time interface check。CI では常に実行される。
func TestPostgresPasskeyCredentialRepo_ImplementsInterface(t *testing.T) {
	var _ PasskeyCredentialRepository = (*PostgresPasskeyCredentialRepo)(nil)
}
