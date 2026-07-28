package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresSessionRepo はPostgreSQLを使用したセッションリポジトリ。
type PostgresSessionRepo struct {
	db *sql.DB
}

// NewPostgresSessionRepo はPostgresSessionRepoを生成する。
func NewPostgresSessionRepo(db *sql.DB) *PostgresSessionRepo {
	return &PostgresSessionRepo{db: db}
}

// Create はセッションを作成する。
//
// 実体は CreateExec への委譲であり、通常の *sql.DB 上で INSERT する（差分等価）。
// 共有トランザクション上で INSERT する場合は CreateExec に DBTX を直接渡す
// （Issue #231 / Web 直接登録 session の 3 行 atomic 化 / DeleteByUserIDExec と同 idiom）。
func (r *PostgresSessionRepo) Create(ctx context.Context, session *model.Session) error {
	return r.CreateExec(ctx, r.db, session)
}

// CreateExec は指定の DBTX（*sql.DB または共有トランザクション）上でセッションを作成する。
//
// Web パスキー登録 finish（Issue #231）では、user・credential・session の 3 行を単一 DB
// トランザクションで INSERT するため本メソッドに tx（repository.DBTX）を渡す。既存
// DeleteByUserIDExec の DBTX 変種パターンに準拠する。
func (r *PostgresSessionRepo) CreateExec(ctx context.Context, q DBTX, session *model.Session) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, data, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		session.ID, session.UserID, []byte("{}"), session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// FindByID は指定IDのセッションを取得する。期限切れの場合はnilを返す。
func (r *PostgresSessionRepo) FindByID(ctx context.Context, id string) (*model.Session, error) {
	session := &model.Session{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, created_at
		 FROM sessions
		 WHERE id = $1 AND expires_at > now()`,
		id,
	).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}

	return session, nil
}

// DeleteByID は指定IDのセッションを削除する。
func (r *PostgresSessionRepo) DeleteByID(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeleteByUserID は指定ユーザーの全セッションを削除する。
func (r *PostgresSessionRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return r.DeleteByUserIDExec(ctx, r.db, userID)
}

// DeleteByUserIDExec は指定の DBTX（*sql.DB または共有トランザクション）上で
// 指定ユーザーの全セッションを削除する。
func (r *PostgresSessionRepo) DeleteByUserIDExec(ctx context.Context, q DBTX, userID string) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}
	return nil
}

// compile-time interface check
var _ SessionRepository = (*PostgresSessionRepo)(nil)
