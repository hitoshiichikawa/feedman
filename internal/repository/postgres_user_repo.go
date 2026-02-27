package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresUserRepo はPostgreSQLを使用したユーザーリポジトリ。
type PostgresUserRepo struct {
	db *sql.DB
}

// NewPostgresUserRepo はPostgresUserRepoを生成する。
func NewPostgresUserRepo(db *sql.DB) *PostgresUserRepo {
	return &PostgresUserRepo{db: db}
}

// FindByID は指定IDのユーザーを取得する。見つからない場合はnilを返す。
func (r *PostgresUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	user := &model.User{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, created_at, updated_at FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by ID: %w", err)
	}

	return user, nil
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
	result, err := r.db.ExecContext(ctx,
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
