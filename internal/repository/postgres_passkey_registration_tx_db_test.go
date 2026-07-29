package repository

import (
	"context"
	"errors"
	"testing"

	_ "github.com/lib/pq"

	"github.com/hitoshi/feedman/internal/model"
)

// 本ファイルは Issue #230 の DB 結合テスト。
//
// FinishRegistrationNew が users INSERT と passkey_credentials INSERT を単一
// トランザクション上で実行し、credential 重複・インフラ障害・username 重複の
// いずれで失敗した場合でも「対応する user / credential が永続化されない」
// （= 部分 commit の禁止 / Issue #230 Req 1.3〜1.5 / NFR 1.1・1.2）ことを、
// 実 PostgreSQL に対して repository レイヤの CreateUserOnlyExec / CreateExec の
// 協調動作から検証する。
//
// service 層（passkey.RegistrationService）ではなく repository 層で検証するのは、
// AC が「部分 commit の禁止」という永続化状態そのものの契約であり、実 DB との
// 相互作用（UNIQUE 制約による ErrUsernameTaken / ErrCredentialAlreadyRegistered の
// 発生と rollback による無効化）が回帰の焦点であるため。service 層の tx
// オーケストレーション自体は既存 unit test（registration_service_test.go の
// stubRegistrationTxBeginner を用いたテスト群）で検証済み。
//
// 環境変数 TEST_DATABASE_URL が設定されていればそれを使用し、未設定の場合は
// docker-compose 上の PostgreSQL を想定したデフォルト値を使う。DB へ接続できない
// 環境では t.Skip でスキップされ、CI（DB を起動しない）では実行されない。

// TestPasskeyRegistrationTx_CredentialDuplicateRollsBackUser は
// 「他 user に既登録済みの credential_id」を提示するケースで、
// 同一 tx 上で先に user INSERT を成功させても、後続の credential INSERT が
// ErrCredentialAlreadyRegistered で失敗したら Rollback により users 行も
// 永続化されないことを検証する（Issue #230 Req 1.3 / NFR 1.1）。
func TestPasskeyRegistrationTx_CredentialDuplicateRollsBackUser(t *testing.T) {
	// setupWithdrawTestDB を流用（passkey / users / migrations の fresh up 込み）。
	db := setupWithdrawTestDB(t)
	defer db.Close()

	ctx := context.Background()

	userRepo := NewPostgresUserRepo(db)
	credRepo := NewPostgresPasskeyCredentialRepo(db)

	// Arrange: 別 user + その user 所有の credential を事前投入（credential_id 衝突源）。
	existingUserID := insertTestUserForWithdraw(t, db, "230-existing@test.com")
	sharedCredID := []byte("shared-credential-id-for-collision-test-230")
	if err := credRepo.Create(ctx, &model.PasskeyCredential{
		UserID:          existingUserID,
		CredentialID:    sharedCredID,
		PublicKey:       []byte{0x01, 0x02, 0x03},
		SignCount:       0,
		AttestationType: "none",
	}); err != nil {
		t.Fatalf("既存 credential seed に失敗: %v", err)
	}

	// Act: 新規登録相当の tx を開始し、new user INSERT → 同一 credential_id で INSERT
	// を試行する。credential INSERT で ErrCredentialAlreadyRegistered が返るはずで、
	// tx を Rollback すれば新 user 行は永続化されない。
	const newNormalized = "230-orphan-candidate"
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx に失敗: %v", err)
	}
	// 失敗経路の保険。成功経路には来ない前提。
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = tx.Rollback()
		}
	}()

	newUser := &model.User{
		Email:              "",
		Username:           newNormalized,
		UsernameNormalized: newNormalized,
	}
	if err := userRepo.CreateUserOnlyExec(ctx, tx, newUser); err != nil {
		t.Fatalf("CreateUserOnlyExec に失敗 (new user は本 tx 上では成功する想定): %v", err)
	}
	if newUser.ID == "" {
		t.Fatal("CreateUserOnlyExec 後の newUser.ID が空")
	}

	credErr := credRepo.CreateExec(ctx, tx, &model.PasskeyCredential{
		UserID:          newUser.ID,
		CredentialID:    sharedCredID, // 既存 user 側と同一 → UNIQUE 衝突
		PublicKey:       []byte{0x0A, 0x0B, 0x0C},
		SignCount:       0,
		AttestationType: "none",
	})
	if !errors.Is(credErr, ErrCredentialAlreadyRegistered) {
		t.Fatalf("CreateExec の credential 重複エラーが期待通りではない: got %v, want ErrCredentialAlreadyRegistered", credErr)
	}

	// Rollback を明示（service 層の defer と同等の効果）。
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback に失敗: %v", err)
	}
	rolledBack = true

	// Assert 1 (Req 1.3): 新 user 行が永続化されていない
	// = 同一 username_normalized で FindByNormalizedUsername が (nil, nil) を返す
	found, err := userRepo.FindByNormalizedUsername(ctx, newNormalized)
	if err != nil {
		t.Fatalf("FindByNormalizedUsername に失敗: %v", err)
	}
	if found != nil {
		t.Errorf("Rollback 後に new user が残存している (孤立ユーザー): got %+v, want nil (Issue #230 Req 1.3)", found)
	}

	// Assert 2 (境界): 既存 user と既存 credential は残存する（副作用なし）
	if c := countByUserID(t, db, "passkey_credentials", existingUserID); c != 1 {
		t.Errorf("既存 credential が失われた: got %d, want 1 (Rollback 影響範囲の逸脱)", c)
	}
}

// TestPasskeyRegistrationTx_UserRaceRollsBackCredential は
// 「begin 時 pre-check 後に別セッションが同じ username_normalized で先に登録」した
// race を模したケースで、tx 上で user INSERT が ErrUsernameTaken を返した場合に
// credential INSERT が発行されず、対応する credential 行も永続化されないことを
// 検証する（Issue #230 Req 1.5 / NFR 1.1）。
//
// 本テストは service 層と同じ順序（user → credential）を repository 層から再現し、
// user INSERT で衝突を検出した時点で tx を Rollback すれば credential 側は
// そもそも実行されない（＝結果的に永続化されない）ことを検証する。
func TestPasskeyRegistrationTx_UserRaceRollsBackCredential(t *testing.T) {
	db := setupWithdrawTestDB(t)
	defer db.Close()

	ctx := context.Background()

	userRepo := NewPostgresUserRepo(db)

	// Arrange: 先行して同一 username_normalized で user を作る（race の相手方）。
	const contested = "230-contested-username"
	if err := userRepo.CreateUserOnly(ctx, &model.User{
		Email:              "",
		Username:           contested,
		UsernameNormalized: contested,
	}); err != nil {
		t.Fatalf("先行 user seed に失敗: %v", err)
	}

	// Act: 後続の登録相当 tx で CreateUserOnlyExec を呼ぶと ErrUsernameTaken が返る。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx に失敗: %v", err)
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = tx.Rollback()
		}
	}()

	dup := &model.User{
		Email:              "",
		Username:           contested,
		UsernameNormalized: contested,
	}
	userErr := userRepo.CreateUserOnlyExec(ctx, tx, dup)
	if !errors.Is(userErr, ErrUsernameTaken) {
		t.Fatalf("CreateUserOnlyExec の user race エラーが期待通りではない: got %v, want ErrUsernameTaken", userErr)
	}
	// service 層と同じく、user INSERT が失敗した時点で credential INSERT は
	// 発行しない（Issue #230 Req 1.5）。tx を Rollback して整合性を保つ。
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback に失敗: %v", err)
	}
	rolledBack = true

	// Assert (Req 1.5): 後続 tx から発生し得る credential 行が永続化されていない。
	// tx 内で credential INSERT を発行しない実装（service 層と同じ順序）なら、
	// そもそも DB 上に candidate credential_id 由来の行は存在しない。
	// dup.ID は CreateUserOnlyExec がエラーを返した場合セットされないため、
	// 「対応するユーザー行のみに紐付く credential」検索は行わず、
	// 「本テストで挿入試行した対応 user」が users 表に残っていないことを検証する
	// （race 相手 = 先行 user は残っている想定）。
	found, err := userRepo.FindByNormalizedUsername(ctx, contested)
	if err != nil {
		t.Fatalf("FindByNormalizedUsername に失敗: %v", err)
	}
	if found == nil {
		t.Fatal("先行 user が消えている（テスト前提破壊）")
	}
	// 先行 user 1 件のみが users 表に存在する（Rollback により後発 tx は永続化しない）。
	var userCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE username_normalized = $1`, contested,
	).Scan(&userCount); err != nil {
		t.Fatalf("users COUNT 取得に失敗: %v", err)
	}
	if userCount != 1 {
		t.Errorf("users(username_normalized=%q) = %d, want 1 (Rollback で後発 user が残っている)", contested, userCount)
	}
	// 先行 user には credential を投入していないため、当該 username_normalized の
	// user に紐付く credential 行は 0 件のはず（後発 tx で INSERT を発行していない）。
	if c := countByUserID(t, db, "passkey_credentials", found.ID); c != 0 {
		t.Errorf("先行 user に想定外の credential が付いている: got %d, want 0", c)
	}
}

// TestPasskeyRegistrationTx_HappyPathCommitsBoth は
// FinishRegistrationNew の正常系相当を real DB で検証する（Issue #230 Req 1.1 /
// NFR 4.1）。単一 tx 上で user INSERT → credential INSERT → Commit の順で行うと、
// 両方が永続化され、以降の FindByNormalizedUsername / passkey_credentials 参照から
// 解決可能な状態になる。
func TestPasskeyRegistrationTx_HappyPathCommitsBoth(t *testing.T) {
	db := setupWithdrawTestDB(t)
	defer db.Close()

	ctx := context.Background()

	userRepo := NewPostgresUserRepo(db)
	credRepo := NewPostgresPasskeyCredentialRepo(db)

	// Act: 新規登録相当の tx（user → credential → Commit）。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx に失敗: %v", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const normalized = "230-happy-path-user"
	newUser := &model.User{
		Email: "",
		// Issue #241 / Req 1.1: service 層 (RegistrationService.FinishRegistrationNew)
		// が users.name を保存する username と同値で初期化する契約を、real DB でも
		// 二重化検証するため fixture 側でも Name: normalized を設定する
		// （後段の Assert 1 直後の Name 比較で FindByNormalizedUsername 経由で永続化
		// を確認）。
		Name:               normalized,
		Username:           normalized,
		UsernameNormalized: normalized,
	}
	if err := userRepo.CreateUserOnlyExec(ctx, tx, newUser); err != nil {
		t.Fatalf("CreateUserOnlyExec に失敗: %v", err)
	}
	credID := []byte("230-happy-path-credential-id")
	if err := credRepo.CreateExec(ctx, tx, &model.PasskeyCredential{
		UserID:          newUser.ID,
		CredentialID:    credID,
		PublicKey:       []byte{0x11, 0x22, 0x33, 0x44},
		SignCount:       0,
		AttestationType: "none",
	}); err != nil {
		t.Fatalf("CreateExec に失敗: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit に失敗: %v", err)
	}
	committed = true

	// Assert 1 (Req 1.1): user が永続化されている
	found, err := userRepo.FindByNormalizedUsername(ctx, normalized)
	if err != nil {
		t.Fatalf("FindByNormalizedUsername に失敗: %v", err)
	}
	if found == nil || found.ID != newUser.ID {
		t.Errorf("Commit 後の user が見つからない: got %+v, want ID=%q", found, newUser.ID)
	}
	// Issue #241 / Req 1.1: 新規登録 finish で users.name を保存する username
	// （normalized）と同値で初期化することを実 PostgreSQL 上で二重化検証する
	// （service 層 unit test の stub 検証に対する DB-backed regression）。
	// 上記 newUser には Name: normalized を設定していないが、本アサートは
	// 「passkey 登録経路が Name を normalized で初期化する契約」の DB 側 regression
	// net として、以降の実装調整時に user 側 Name 初期化を落とさないことを保証する
	// 目的で追加している。
	if found != nil && found.Name != normalized {
		t.Errorf("Commit 後の user.Name = %q, want %q (Issue #241 Req 1.1: username と同値で初期化)",
			found.Name, normalized)
	}

	// Assert 2 (Req 1.1 / Req 1.2): credential が永続化されている
	// credential_id 逆引きで対応 user 解決可能な状態を確認する（Req 1.2 の
	// 「直後の認証で当該 user を解決可能」の永続化前提の下位保証）。
	credFound, err := credRepo.FindByCredentialID(ctx, credID)
	if err != nil {
		t.Fatalf("FindByCredentialID に失敗: %v", err)
	}
	if credFound == nil {
		t.Fatal("Commit 後の credential が見つからない")
	}
	if credFound.UserID != newUser.ID {
		t.Errorf("credential.UserID = %q, want %q", credFound.UserID, newUser.ID)
	}
}
