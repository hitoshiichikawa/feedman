package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/lib/pq"
)

// pgErrCodeUniqueViolation は PostgreSQL の unique_violation エラーコード。
// UNIQUE 制約違反（passkey 用の username_normalized 部分 UNIQUE INDEX 等）を
// pq.Error.Code で識別するために用いる（Issue #216 / Req 1.4）。
const pgErrCodeUniqueViolation = "23505"

// PostgresUserRepo はPostgreSQLを使用したユーザーリポジトリ。
type PostgresUserRepo struct {
	db *sql.DB
}

// NewPostgresUserRepo はPostgresUserRepoを生成する。
func NewPostgresUserRepo(db *sql.DB) *PostgresUserRepo {
	return &PostgresUserRepo{db: db}
}

// FindByID は指定IDのユーザーを取得する。見つからない場合はnilを返す。
//
// Issue #216 で追加された username / username_normalized カラムも scan する。
// 未設定（Google 由来ユーザー）の場合は NULL であるため sql.NullString で受け、
// nil の場合は空文字にマップして既存挙動と互換性を保つ（NFR 2.1 / 2.2）。
func (r *PostgresUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	user := &model.User{}
	var username, usernameNormalized sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, username, username_normalized, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(
		&user.ID, &user.Email, &user.Name,
		&username, &usernameNormalized,
		&user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by ID: %w", err)
	}

	if username.Valid {
		user.Username = username.String
	}
	if usernameNormalized.Valid {
		user.UsernameNormalized = usernameNormalized.String
	}

	return user, nil
}

// FindByNormalizedUsername は正規化済みユーザー名（lowercase）でユーザーを検索する
// （Issue #216 / Req 1.4）。見つからない場合は (nil, nil) を返す。
//
// 呼び出し側は非空 normalized のみを渡す前提。DB 側の部分 UNIQUE INDEX は
// username_normalized IS NOT NULL を対象にしており、NULL 同士は衝突しないため、
// 空文字を渡した場合の挙動は未定義（呼び出し側の validator で防ぐ）。
// NFR 1.2: 入力 normalized の値をエラーメッセージに含めない。
func (r *PostgresUserRepo) FindByNormalizedUsername(ctx context.Context, normalized string) (*model.User, error) {
	user := &model.User{}
	var username, usernameNormalized sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, username, username_normalized, created_at, updated_at
		 FROM users WHERE username_normalized = $1`,
		normalized,
	).Scan(
		&user.ID, &user.Email, &user.Name,
		&username, &usernameNormalized,
		&user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by normalized username: %w", err)
	}

	if username.Valid {
		user.Username = username.String
	}
	if usernameNormalized.Valid {
		user.UsernameNormalized = usernameNormalized.String
	}

	return user, nil
}

// CreateUserOnly は identity を持たないユーザー（パスキー新規登録ユーザー）の
// users 行のみを INSERT する（Issue #216 / Req 1.2 / 1.6）。
//
// u.ID が空文字 / u.CreatedAt / u.UpdatedAt が zero-value の場合は DB 側デフォルト
// （gen_random_uuid() / now()）を採用し、確定値を u に反映する。
// username_normalized の UNIQUE 制約違反時は ErrUsernameTaken に変換する
// （Req 1.4）。email 空文字を許容し、リカバリ用メールなしのユーザー作成に対応する
// （Req 1.6）。NFR 1.2: エラーメッセージには username / email の値を含めない。
//
// 実体は CreateUserOnlyExec に委譲し、非トランザクション時は *sql.DB を渡す。
// 共有トランザクション上で INSERT したい場合は CreateUserOnlyExec を直接呼ぶ
// （Issue #230 / Req 1.1〜1.6: 新規パスキー登録 finish の 1 tx 化に用いる）。
func (r *PostgresUserRepo) CreateUserOnly(ctx context.Context, u *model.User) error {
	return r.CreateUserOnlyExec(ctx, r.db, u)
}

// CreateUserOnlyExec は指定の DBTX（*sql.DB または共有トランザクション）上で
// identity を持たないユーザー行を INSERT する（Issue #216 / Req 1.2 / 1.6、
// Issue #230 / Req 1.1〜1.6 / NFR 1.1 の 1 tx 化サポート）。
//
// パラメータ変換と ErrUsernameTaken マッピングは CreateUserOnly と同一。
// pq.Error による 23505 判定は、当該 INSERT で発生し得る UNIQUE 制約が
// `idx_users_username_normalized`（部分 UNIQUE）のみである前提で、23505 を
// ErrUsernameTaken に対応させる。将来 users テーブルに他 UNIQUE 制約が追加された
// 場合は pq.Error.Constraint 名で分岐する必要があるため、その場合は本 doc comment を
// 更新すること。
func (r *PostgresUserRepo) CreateUserOnlyExec(ctx context.Context, q DBTX, u *model.User) error {
	if u == nil {
		return fmt.Errorf("failed to create user: user is nil")
	}
	// username / username_normalized は空文字なら NULL として挿入する
	// （部分 UNIQUE INDEX は NULL 同士を衝突扱いにしない）。
	var usernameArg, usernameNormalizedArg interface{}
	if u.Username != "" {
		usernameArg = u.Username
	}
	if u.UsernameNormalized != "" {
		usernameNormalizedArg = u.UsernameNormalized
	}
	// id / created_at / updated_at が未設定なら DB デフォルトに委ねる。
	var idArg interface{}
	if u.ID != "" {
		idArg = u.ID
	}
	var createdAtArg, updatedAtArg interface{}
	if !u.CreatedAt.IsZero() {
		createdAtArg = u.CreatedAt
	}
	if !u.UpdatedAt.IsZero() {
		updatedAtArg = u.UpdatedAt
	}
	err := q.QueryRowContext(ctx,
		`INSERT INTO users (id, email, name, username, username_normalized, created_at, updated_at)
		 VALUES (
		     COALESCE($1::uuid, gen_random_uuid()),
		     $2, $3, $4, $5,
		     COALESCE($6::timestamptz, now()),
		     COALESCE($7::timestamptz, now())
		 )
		 RETURNING id, created_at, updated_at`,
		idArg, u.Email, u.Name, usernameArg, usernameNormalizedArg,
		createdAtArg, updatedAtArg,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == pgErrCodeUniqueViolation {
			return ErrUsernameTaken
		}
		// NFR 1.2: username / email の値はメッセージに含めない。
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// CreateWithIdentity はユーザーとidentityを同一トランザクションで作成する。
func (r *PostgresUserRepo) CreateWithIdentity(ctx context.Context, user *model.User, identity *model.Identity) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// ユーザーを作成
	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, email, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		user.ID, user.Email, user.Name, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}

	// identityを作成
	_, err = tx.ExecContext(ctx,
		`INSERT INTO identities (id, user_id, provider, provider_user_id, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		identity.ID, identity.UserID, identity.Provider, identity.ProviderUserID, identity.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteByID は指定IDのユーザーを削除する。
// 関連するidentities、user_settingsはCASCADE削除される。
func (r *PostgresUserRepo) DeleteByID(ctx context.Context, id string) error {
	return r.DeleteByIDExec(ctx, r.db, id)
}

// DeleteByIDExec は指定の DBTX（*sql.DB または共有トランザクション）上で
// 指定IDのユーザーを削除する。関連する identities / user_settings は CASCADE 削除される。
// 対象が存在しない場合はエラーを返す（既存の DeleteByID と同一挙動）。
func (r *PostgresUserRepo) DeleteByIDExec(ctx context.Context, q DBTX, id string) error {
	result, err := q.ExecContext(ctx,
		`DELETE FROM users WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found: %s", id)
	}
	return nil
}

// compile-time interface check
var _ UserRepository = (*PostgresUserRepo)(nil)
