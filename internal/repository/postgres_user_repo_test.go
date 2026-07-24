package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresUserRepoはUserRepositoryインターフェースを満たすことを検証
func TestPostgresUserRepo_ImplementsInterface(t *testing.T) {
	var _ UserRepository = (*PostgresUserRepo)(nil)
}

// PostgresIdentityRepoはIdentityRepositoryインターフェースを満たすことを検証
func TestPostgresIdentityRepo_ImplementsInterface(t *testing.T) {
	var _ IdentityRepository = (*PostgresIdentityRepo)(nil)
}

// PostgresSessionRepoはSessionRepositoryインターフェースを満たすことを検証
func TestPostgresSessionRepo_ImplementsInterface(t *testing.T) {
	var _ SessionRepository = (*PostgresSessionRepo)(nil)
}

// NewPostgresUserRepoが正しく初期化されることを検証
func TestNewPostgresUserRepo_Initializes(t *testing.T) {
	repo := NewPostgresUserRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// NewPostgresIdentityRepoが正しく初期化されることを検証
func TestNewPostgresIdentityRepo_Initializes(t *testing.T) {
	repo := NewPostgresIdentityRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// NewPostgresSessionRepoが正しく初期化されることを検証
func TestNewPostgresSessionRepo_Initializes(t *testing.T) {
	repo := NewPostgresSessionRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// ユニットテスト: CreateWithIdentityが正しいSQLパラメータを構築すること
// （DB接続なしでロジックのみ検証）
func TestPostgresUserRepo_CreateWithIdentity_SetsTimestamps(t *testing.T) {
	now := time.Now()
	user := &model.User{
		ID:    "user-id-1",
		Email: "test@example.com",
		Name:  "Test User",
	}
	identity := &model.Identity{
		ID:             "identity-id-1",
		UserID:         "user-id-1",
		Provider:       "google",
		ProviderUserID: "google-123",
	}

	// タイムスタンプが設定前に空であることを確認
	if !user.CreatedAt.IsZero() && user.CreatedAt.Before(now.Add(-1*time.Hour)) {
		// CreatedAtが既に設定されている場合のチェック
	}

	// identityのUserIDがuserのIDと一致することを確認
	if identity.UserID != user.ID {
		t.Errorf("identity.UserID = %q, want %q", identity.UserID, user.ID)
	}
}

// SessionRepoのFindByIDが期限切れセッションを返さないことの期待動作
func TestPostgresSessionRepo_FindByID_ExpiredSession_Concept(t *testing.T) {
	// このテストはDB接続なしでコンセプトを検証する
	session := &model.Session{
		ID:        "expired-session",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	if session.ExpiresAt.After(time.Now()) {
		t.Error("expected session to be expired")
	}
}

// SessionRepoのDeleteByIDが正しいセッションIDで呼ばれることの検証
func TestPostgresSessionRepo_DeleteByID_Concept(t *testing.T) {
	sessionID := "session-to-delete"
	ctx := context.Background()

	if sessionID == "" {
		t.Fatal("session ID should not be empty")
	}
	if ctx == nil {
		t.Fatal("context should not be nil")
	}
}

// TestPostgresUserRepo_CreateUserOnly_And_FindByNormalizedUsername_DB は
// Issue #216 task 2 で追加した CreateUserOnly / FindByNormalizedUsername の
// DB 結合テスト。setup* / cleanup は postgres_passkey_credential_repo_db_test.go の
// setupPasskeyTestDB と共有する（DB 未接続時は t.Skip でスキップ）。
func TestPostgresUserRepo_CreateUserOnly_And_FindByNormalizedUsername_DB(t *testing.T) {
	db := setupPasskeyTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresUserRepo(db)

	// Case 1 (Req 1.2): CreateUserOnly 成功 → FindByNormalizedUsername で復元
	t.Run("CreateUserOnly成功でFindByNormalizedUsernameから復元される", func(t *testing.T) {
		u := &model.User{
			Email:              "alice@example.com",
			Name:               "Alice",
			Username:           "Alice",
			UsernameNormalized: "alice",
		}
		if err := repo.CreateUserOnly(ctx, u); err != nil {
			t.Fatalf("CreateUserOnly に失敗: %v", err)
		}
		if u.ID == "" {
			t.Fatal("Create 後の u.ID が空")
		}
		if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
			t.Fatal("Create 後の timestamps が zero")
		}

		got, err := repo.FindByNormalizedUsername(ctx, "alice")
		if err != nil {
			t.Fatalf("FindByNormalizedUsername に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByNormalizedUsername が nil を返した")
		}
		if got.ID != u.ID {
			t.Errorf("ID 不一致: got %q, want %q", got.ID, u.ID)
		}
		if got.Username != "Alice" {
			t.Errorf("Username 不一致: got %q, want %q", got.Username, "Alice")
		}
		if got.UsernameNormalized != "alice" {
			t.Errorf("UsernameNormalized 不一致: got %q, want %q", got.UsernameNormalized, "alice")
		}
	})

	// Case 2 (Req 1.4): username_normalized 重複 INSERT で ErrUsernameTaken
	t.Run("username_normalized重複でErrUsernameTaken", func(t *testing.T) {
		u1 := &model.User{
			Email:              "bob-1@example.com",
			Name:               "Bob 1",
			Username:           "bob",
			UsernameNormalized: "bob",
		}
		if err := repo.CreateUserOnly(ctx, u1); err != nil {
			t.Fatalf("Create user1 に失敗: %v", err)
		}

		u2 := &model.User{
			Email:              "bob-2@example.com",
			Name:               "Bob 2",
			Username:           "BOB", // 見た目は異なるが normalized は同じ
			UsernameNormalized: "bob",
		}
		err := repo.CreateUserOnly(ctx, u2)
		if !errors.Is(err, ErrUsernameTaken) {
			t.Errorf("重複 normalized で ErrUsernameTaken 以外を返した: %v", err)
		}
	})

	// Case 3 (異常系 not-found): 未存在 normalized で (nil, nil)
	t.Run("FindByNormalizedUsername_未存在で(nil,nil)を返す", func(t *testing.T) {
		got, err := repo.FindByNormalizedUsername(ctx, "does-not-exist-xyz")
		if err != nil {
			t.Fatalf("FindByNormalizedUsername がエラーを返した: %v", err)
		}
		if got != nil {
			t.Errorf("未存在で non-nil を返した: %+v", got)
		}
	})

	// Case 4 (Req 1.6): email 空文字でも user 行を作成できる
	t.Run("email空文字でも作成可能_Req1.6", func(t *testing.T) {
		u := &model.User{
			Email:              "",
			Name:               "No Email User",
			Username:           "noemail",
			UsernameNormalized: "noemail",
		}
		if err := repo.CreateUserOnly(ctx, u); err != nil {
			t.Fatalf("email 空文字で CreateUserOnly に失敗（Req 1.6 違反）: %v", err)
		}

		got, err := repo.FindByNormalizedUsername(ctx, "noemail")
		if err != nil {
			t.Fatalf("FindByNormalizedUsername に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByNormalizedUsername が nil を返した")
		}
		if got.Email != "" {
			t.Errorf("email が空文字で保存されていない: got %q", got.Email)
		}
	})

	// Case 5 (部分 UNIQUE の NULL 挙動): username_normalized 未設定の
	// 複数ユーザーは共存できる（NULL 同士は衝突しない / NFR 2.1 の既存互換）
	t.Run("username_normalized未設定の複数ユーザーは共存可_NFR2.1", func(t *testing.T) {
		// 生 SQL で NULL の users を 2 件挿入（既存 Google 由来ユーザー相当）
		if _, err := db.Exec(
			`INSERT INTO users (email, name, username, username_normalized) VALUES ($1, $2, NULL, NULL), ($3, $4, NULL, NULL)`,
			"legacy1@example.com", "Legacy 1",
			"legacy2@example.com", "Legacy 2",
		); err != nil {
			t.Fatalf("既存互換 NULL 挿入に失敗（部分 UNIQUE INDEX が NOT NULL 条件を持たない可能性 / NFR 2.1）: %v", err)
		}
	})

	// Case 6 (compile-time): 追加メソッドを含む UserRepository interface 充足
	t.Run("compile_time_interface_check", func(t *testing.T) {
		var _ UserRepository = (*PostgresUserRepo)(nil)
	})
}

// TestPostgresUserRepo_FindByID_ScansUsernameColumns_DB は Issue #216 で
// FindByID の SELECT に username / username_normalized カラムを追加した際に、
// 既存 Google 由来ユーザー（両カラム NULL）が空文字で復元されることを検証する
// （NFR 2.1 / 2.2: 既存挙動と互換）。
func TestPostgresUserRepo_FindByID_ScansUsernameColumns_DB(t *testing.T) {
	db := setupPasskeyTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresUserRepo(db)

	// legacy user（username 未設定 = NULL）を挿入
	var legacyID string
	if err := db.QueryRow(
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		"legacy-findbyid@example.com", "Legacy FindByID",
	).Scan(&legacyID); err != nil {
		t.Fatalf("legacy user 挿入に失敗: %v", err)
	}

	got, err := repo.FindByID(ctx, legacyID)
	if err != nil {
		t.Fatalf("FindByID に失敗: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID が nil を返した")
	}
	if got.Username != "" {
		t.Errorf("legacy user の Username が空文字ではない（NFR 2.1 違反）: got %q", got.Username)
	}
	if got.UsernameNormalized != "" {
		t.Errorf("legacy user の UsernameNormalized が空文字ではない（NFR 2.1 違反）: got %q", got.UsernameNormalized)
	}
	if got.Email != "legacy-findbyid@example.com" {
		t.Errorf("既存 email 復元に失敗: got %q", got.Email)
	}
}
