package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// 本ファイルは PostgresPasskeyChallengeRepo の結合テスト（Issue #216 task 2）を実装する。
// setupPasskeyTestDB / passkeyTestDatabaseURL は
// postgres_passkey_credential_repo_db_test.go と共有する。

// newTestPasskeyChallenge は PasskeyChallenge のテスト用 struct を組み立てる。
// ID / CreatedAt は repo 側のデフォルトで決まるため空のままにする。
func newTestPasskeyChallenge(
	kind model.PasskeyChallengeKind,
	challengeHash string,
	expiresAt time.Time,
) *model.PasskeyChallenge {
	return &model.PasskeyChallenge{
		ChallengeHash: challengeHash,
		Kind:          kind,
		SessionData:   []byte(`{"kind":"stub"}`),
		ExpiresAt:     expiresAt,
		Consumed:      false,
	}
}

// TestPostgresPasskeyChallengeRepo_DB は PostgresPasskeyChallengeRepo の主要 6 ケースを
// 1 つのテーブルライク構成で実装する（Issue #216 task 2 の検証項目）。
func TestPostgresPasskeyChallengeRepo_DB(t *testing.T) {
	db := setupPasskeyTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewPostgresPasskeyChallengeRepo(db)

	// Case 1 (Req 4.1 / 4.2): Create → FindByHash で同値復元
	t.Run("Create_FindByHashで保存値が同値復元される", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Microsecond)
		ch := newTestPasskeyChallenge(
			model.PasskeyChallengeKindRegistrationNew,
			"hash-create-find-01234567",
			expiresAt,
		)
		pendingUsername := "alice_new"
		ch.PendingUsername = &pendingUsername

		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}
		if ch.ID == "" {
			t.Fatal("Create 後の ch.ID が空: DB デフォルトが反映されていない")
		}
		if ch.CreatedAt.IsZero() {
			t.Fatal("Create 後の ch.CreatedAt が zero: DB デフォルトが反映されていない")
		}

		got, err := repo.FindByHash(ctx, ch.ChallengeHash)
		if err != nil {
			t.Fatalf("FindByHash に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByHash が nil を返した（保存済み challenge がヒットしない）")
		}
		if got.ID != ch.ID {
			t.Errorf("ID 不一致: got %q, want %q", got.ID, ch.ID)
		}
		if got.ChallengeHash != ch.ChallengeHash {
			t.Errorf("ChallengeHash 不一致: got %q, want %q", got.ChallengeHash, ch.ChallengeHash)
		}
		if got.Kind != model.PasskeyChallengeKindRegistrationNew {
			t.Errorf("Kind 不一致: got %q, want %q", got.Kind, model.PasskeyChallengeKindRegistrationNew)
		}
		if got.PendingUsername == nil || *got.PendingUsername != pendingUsername {
			t.Errorf("PendingUsername 不一致: got %v, want %q", got.PendingUsername, pendingUsername)
		}
		if got.UserID != nil {
			t.Errorf("UserID は nil であるべき: got %v", got.UserID)
		}
		if !got.ExpiresAt.Equal(expiresAt) {
			t.Errorf("ExpiresAt 不一致: got %v, want %v", got.ExpiresAt, expiresAt)
		}
		if got.Consumed {
			t.Error("作成直後の Consumed が true になっている（false 期待）")
		}
	})

	// Case 2 (異常系 not-found): FindByHash が未存在で (nil, nil)
	t.Run("FindByHash_未存在で(nil,nil)を返す", func(t *testing.T) {
		got, err := repo.FindByHash(ctx, "hash-does-not-exist-abcdefg")
		if err != nil {
			t.Fatalf("FindByHash がエラーを返した: %v", err)
		}
		if got != nil {
			t.Errorf("FindByHash が non-nil を返した（nil 期待）: %+v", got)
		}
	})

	// Case 3 (task 3 スコープ調整): FindByID で opaque challenge_id 逆引きが可能
	t.Run("FindByID_保存済みidで同値復元される", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute).UTC()
		ch := newTestPasskeyChallenge(
			model.PasskeyChallengeKindAuthentication,
			"hash-findbyid-01234567",
			expiresAt,
		)
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		got, err := repo.FindByID(ctx, ch.ID)
		if err != nil {
			t.Fatalf("FindByID に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByID が nil を返した（保存済み id がヒットしない）")
		}
		if got.ChallengeHash != ch.ChallengeHash {
			t.Errorf("ChallengeHash 不一致: got %q, want %q", got.ChallengeHash, ch.ChallengeHash)
		}
		if got.Kind != model.PasskeyChallengeKindAuthentication {
			t.Errorf("Kind 不一致: got %q, want %q", got.Kind, model.PasskeyChallengeKindAuthentication)
		}
	})

	// Case 4 (異常系): FindByID が未存在で (nil, nil)
	t.Run("FindByID_未存在で(nil,nil)を返す", func(t *testing.T) {
		got, err := repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("FindByID がエラーを返した: %v", err)
		}
		if got != nil {
			t.Errorf("FindByID が non-nil を返した（nil 期待）: %+v", got)
		}
	})

	// Case 5 (Req 4.3): MarkConsumed 成功で consumed=true に遷移する
	t.Run("MarkConsumed_成功で単回消費される", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute).UTC()
		ch := newTestPasskeyChallenge(
			model.PasskeyChallengeKindRegistrationNew,
			"hash-markconsumed-ok-01",
			expiresAt,
		)
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		if err := repo.MarkConsumed(ctx, ch.ID); err != nil {
			t.Fatalf("MarkConsumed に失敗: %v", err)
		}

		got, err := repo.FindByID(ctx, ch.ID)
		if err != nil {
			t.Fatalf("FindByID に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("MarkConsumed 後の FindByID が nil")
		}
		if !got.Consumed {
			t.Error("MarkConsumed 後の Consumed が false（true 期待）")
		}
	})

	// Case 6 (Req 4.4 二重消費): 一度消費した challenge に MarkConsumed → ErrChallengeNotUsable
	t.Run("MarkConsumed_二重消費でErrChallengeNotUsable", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute).UTC()
		ch := newTestPasskeyChallenge(
			model.PasskeyChallengeKindAuthentication,
			"hash-markconsumed-dup-01",
			expiresAt,
		)
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// 1 回目: 成功
		if err := repo.MarkConsumed(ctx, ch.ID); err != nil {
			t.Fatalf("MarkConsumed 1 回目で失敗: %v", err)
		}

		// 2 回目: ErrChallengeNotUsable
		err := repo.MarkConsumed(ctx, ch.ID)
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Errorf("2 回目 MarkConsumed で ErrChallengeNotUsable 以外を返した: %v", err)
		}
	})

	// Case 7 (Req 4.4 期限切れ境界): expires_at を過去にした challenge に MarkConsumed → ErrChallengeNotUsable
	//                                永続化状態は変わらない（consumed のまま false）
	t.Run("MarkConsumed_期限切れ境界_ErrChallengeNotUsable_状態不変", func(t *testing.T) {
		// 期限切れ challenge を作成
		expiresAt := time.Now().Add(-1 * time.Minute).UTC()
		ch := newTestPasskeyChallenge(
			model.PasskeyChallengeKindAuthentication,
			"hash-markconsumed-exp-01",
			expiresAt,
		)
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		err := repo.MarkConsumed(ctx, ch.ID)
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Errorf("期限切れ MarkConsumed で ErrChallengeNotUsable 以外を返した: %v", err)
		}

		// 状態確認: consumed は false のまま（永続化状態を変更していない / Req 4.4）
		got, err := repo.FindByID(ctx, ch.ID)
		if err != nil {
			t.Fatalf("FindByID に失敗: %v", err)
		}
		if got == nil {
			t.Fatal("FindByID が nil（レコードが消えている）")
		}
		if got.Consumed {
			t.Error("期限切れ MarkConsumed 失敗後の Consumed が true（永続化状態を変更している / Req 4.4 違反）")
		}
	})

	// Case 8 (Req 4.4 未存在): 存在しない id に MarkConsumed → ErrChallengeNotUsable
	t.Run("MarkConsumed_未存在idでErrChallengeNotUsable", func(t *testing.T) {
		err := repo.MarkConsumed(ctx, "00000000-0000-0000-0000-000000000000")
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Errorf("未存在 id MarkConsumed で ErrChallengeNotUsable 以外を返した: %v", err)
		}
	})

	// Case 9 (NFR 1.2 回帰): 生 challenge を保存しない（challenge_hash 列に平文が
	// 直接書かれていないことを、平文 SELECT が 0 件になることで検証）
	t.Run("生challengeが保存されない_NFR1.2回帰", func(t *testing.T) {
		plainChallenge := "plain-challenge-should-not-be-stored"
		challengeHash := "hash::" + plainChallenge // 実装上、hash は平文と異なる文字列
		ch := newTestPasskeyChallenge(
			model.PasskeyChallengeKindAuthentication,
			challengeHash,
			time.Now().Add(5*time.Minute).UTC(),
		)
		if err := repo.Create(ctx, ch); err != nil {
			t.Fatalf("Create に失敗: %v", err)
		}

		// challenge_hash カラムを平文で SELECT すると 0 件
		var countByPlain int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM passkey_challenges WHERE challenge_hash = $1`, plainChallenge,
		).Scan(&countByPlain); err != nil {
			t.Fatalf("平文 SELECT に失敗: %v", err)
		}
		if countByPlain != 0 {
			t.Errorf("平文 challenge が challenge_hash として書かれている: got %d, want 0 (NFR 1.2 違反)", countByPlain)
		}

		// hash 値で SELECT すれば 1 件ヒット（保存自体は成立している）
		var countByHash int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM passkey_challenges WHERE challenge_hash = $1`, challengeHash,
		).Scan(&countByHash); err != nil {
			t.Fatalf("hash SELECT に失敗: %v", err)
		}
		if countByHash != 1 {
			t.Errorf("hash 値での SELECT 件数が不正: got %d, want 1", countByHash)
		}
	})
}

// TestPostgresPasskeyChallengeRepo_ImplementsInterface は DB 非接続でも実行可能な
// compile-time interface check。
func TestPostgresPasskeyChallengeRepo_ImplementsInterface(t *testing.T) {
	var _ PasskeyChallengeRepository = (*PostgresPasskeyChallengeRepo)(nil)
}
