package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresIdentityRepo はPostgreSQLを使用したidentityリポジトリ。
type PostgresIdentityRepo struct {
	db *sql.DB
}

// NewPostgresIdentityRepo はPostgresIdentityRepoを生成する。
func NewPostgresIdentityRepo(db *sql.DB) *PostgresIdentityRepo {
	return &PostgresIdentityRepo{db: db}
}

// FindByProviderAndProviderUserID はproviderとprovider_user_idでidentityを検索する。
// 見つからない場合はnilを返す。
func (r *PostgresIdentityRepo) FindByProviderAndProviderUserID(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
	identity := &model.Identity{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, provider, provider_user_id, created_at
		 FROM identities
		 WHERE provider = $1 AND provider_user_id = $2`,
		provider, providerUserID,
	).Scan(&identity.ID, &identity.UserID, &identity.Provider, &identity.ProviderUserID, &identity.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find identity: %w", err)
	}

	return identity, nil
}

// compile-time interface check
var _ IdentityRepository = (*PostgresIdentityRepo)(nil)
