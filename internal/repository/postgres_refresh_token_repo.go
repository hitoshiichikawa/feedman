package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresRefreshTokenRepo は RefreshTokenRepository の PostgreSQL 実装。
//
// 本実装は native auth の refresh token / family（Issue #163 / #164）の
// 保存・hash 一致参照・rotation 状態確定・family 単位 revoke・user 単位削除を提供する。
// 平文 token は引数にも戻り値にも一切含めず、SHA-256 由来の hash 文字列（token_hash）
// のみを保持する（NFR 1.1 / 1.2）。エラーメッセージにも token_hash や family_id /
// user_id / id の値を含めない。
//
// 1 メソッド 1 SQL を基本とし、複数操作の atomic 性が必要な orchestration
// （rotation = 旧 token rotate + 新 token create）は呼び出し側に委ねる。
// RevokeFamily だけは 2 UPDATE を 1 メソッド内で発行するが、外側で tx 化は呼び出し側に
// 委ねるベストエフォート冪等として設計する（design.md §PostgresRefreshTokenRepo）。
type PostgresRefreshTokenRepo struct {
	db *sql.DB
}

// NewPostgresRefreshTokenRepo は PostgresRefreshTokenRepo を生成する。
func NewPostgresRefreshTokenRepo(db *sql.DB) *PostgresRefreshTokenRepo {
	return &PostgresRefreshTokenRepo{db: db}
}

// CreateFamily は新規 refresh token family を保存する（Req 3.2）。
// family.UserID は呼び出し側で確定済みであること。
// family.ID が空文字 / family.CreatedAt が zero-value の場合はそれぞれ DB の
// デフォルト（gen_random_uuid() / now()）を採用し、確定値を呼び出し元の struct に反映する。
func (r *PostgresRefreshTokenRepo) CreateFamily(ctx context.Context, family *model.RefreshTokenFamily) error {
	if family == nil {
		return fmt.Errorf("failed to create refresh_token_family: family is nil")
	}
	var idArg interface{}
	if family.ID != "" {
		idArg = family.ID
	} else {
		idArg = nil
	}
	var createdAtArg interface{}
	if !family.CreatedAt.IsZero() {
		createdAtArg = family.CreatedAt
	} else {
		createdAtArg = nil
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO refresh_token_families (id, user_id, created_at, revoked_at)
		 VALUES (
		     COALESCE($1::uuid, gen_random_uuid()),
		     $2,
		     COALESCE($3::timestamptz, now()),
		     $4
		 )
		 RETURNING id, created_at`,
		idArg, family.UserID, createdAtArg, family.RevokedAt,
	).Scan(&family.ID, &family.CreatedAt)
	if err != nil {
		// NFR 1.2: user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to create refresh_token_family: %w", err)
	}
	return nil
}

// CreateToken は family に属する refresh token を保存する（Req 3.1, 3.2）。
// token.TokenHash / FamilyID / UserID / ExpiresAt は呼び出し側で確定済みであること。
// token.ID が空文字 / token.CreatedAt が zero-value の場合は DB デフォルトを採用し、
// 確定値を呼び出し元の struct に反映する。
func (r *PostgresRefreshTokenRepo) CreateToken(ctx context.Context, token *model.RefreshToken) error {
	if token == nil {
		return fmt.Errorf("failed to create refresh_token: token is nil")
	}
	var idArg interface{}
	if token.ID != "" {
		idArg = token.ID
	} else {
		idArg = nil
	}
	var createdAtArg interface{}
	if !token.CreatedAt.IsZero() {
		createdAtArg = token.CreatedAt
	} else {
		createdAtArg = nil
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO refresh_tokens (id, family_id, user_id, token_hash, expires_at, rotated_at, revoked_at, created_at)
		 VALUES (
		     COALESCE($1::uuid, gen_random_uuid()),
		     $2, $3, $4, $5, $6, $7,
		     COALESCE($8::timestamptz, now())
		 )
		 RETURNING id, created_at`,
		idArg, token.FamilyID, token.UserID, token.TokenHash, token.ExpiresAt,
		token.RotatedAt, token.RevokedAt, createdAtArg,
	).Scan(&token.ID, &token.CreatedAt)
	if err != nil {
		// NFR 1.2: token_hash / family_id / user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to create refresh_token: %w", err)
	}
	return nil
}

// FindByHash は token_hash に一致するレコードを 1 件返す（Req 3.5）。
// 見つからない場合は (nil, nil) を返す（既存 FindByID / FindByHash パターンに整合）。
// 戻り値の token が RotatedAt / RevokedAt を持つかは呼び出し側で判定する。
func (r *PostgresRefreshTokenRepo) FindByHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	token := &model.RefreshToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, family_id, user_id, token_hash, expires_at, rotated_at, revoked_at, created_at
		 FROM refresh_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(
		&token.ID,
		&token.FamilyID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.RotatedAt,
		&token.RevokedAt,
		&token.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		// NFR 1.2: token_hash の値はメッセージに含めない。
		return nil, fmt.Errorf("failed to find refresh_token: %w", err)
	}
	return token, nil
}

// MarkRotated は当該 ID の refresh_token の rotated_at を set する（Req 3.3）。
//
// 既に rotated_at が set 済みの場合（rotation 二度目）は ErrRefreshTokenAlreadyRotated を
// 返し、永続化状態は変更しない。WHERE 句で rotated_at IS NULL を判定することで、
// 並行アクセス下でも race を起こさず単一遷移を保証する（race 安全）。
func (r *PostgresRefreshTokenRepo) MarkRotated(ctx context.Context, id string, rotatedAt time.Time) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens
		 SET rotated_at = $1
		 WHERE id = $2 AND rotated_at IS NULL`,
		rotatedAt, id,
	)
	if err != nil {
		// NFR 1.2: id の値はメッセージに含めない。
		return fmt.Errorf("failed to mark refresh_token rotated: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark refresh_token rotated: %w", err)
	}
	if affected == 0 {
		return ErrRefreshTokenAlreadyRotated
	}
	return nil
}

// RevokeFamily は当該 family を revoked にし、family 配下の全 token の revoked_at を
// 一括で set する（Req 3.4）。
//
// 実装は同一 method 内で 2 UPDATE を実行する:
//  1. refresh_token_families.revoked_at を set
//  2. refresh_tokens.revoked_at を family_id 一致行に対して set
//
// 既に revoked 済みの family / token に対しても冪等に成功させるため、
// COALESCE(revoked_at, $1) で既存値を保持する（二重 revoke 安全）。
// 外側 tx 化は呼び出し側に委ねる（本実装はベストエフォート冪等、design.md 方針）。
func (r *PostgresRefreshTokenRepo) RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE refresh_token_families
		 SET revoked_at = COALESCE(revoked_at, $1)
		 WHERE id = $2`,
		revokedAt, familyID,
	); err != nil {
		// NFR 1.2: family_id の値はメッセージに含めない。
		return fmt.Errorf("failed to revoke refresh_token_family: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = COALESCE(revoked_at, $1)
		 WHERE family_id = $2`,
		revokedAt, familyID,
	); err != nil {
		return fmt.Errorf("failed to revoke refresh_tokens by family: %w", err)
	}
	return nil
}

// DeleteByUserID は当該ユーザーに属する全ての refresh_token と family を削除する（Req 3.6）。
//
// refresh_token_families を DELETE すると、refresh_tokens.family_id への FK
// ON DELETE CASCADE により配下の token も自動削除される。本実装は family の DELETE
// 1 文だけを発行する（refresh_tokens は明示 DELETE しない）。
//
// Issue #170: 共有トランザクション上で実行する場合は DeleteByUserIDExec を直接呼ぶ。
// 本メソッドは r.db を渡して同 Exec 変種に委譲する（SQL・エラーメッセージは挙動等価）。
func (r *PostgresRefreshTokenRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return r.DeleteByUserIDExec(ctx, r.db, userID)
}

// DeleteByUserIDExec は指定の DBTX（*sql.DB または共有トランザクション）上で
// 当該ユーザーに属する全ての refresh_token と family を削除する（Issue #170 Req 1.2, 2.1）。
//
// refresh_token_families を DELETE すると、refresh_tokens.family_id への FK
// ON DELETE CASCADE により配下の token も自動削除される（本メソッドは family の
// DELETE 1 文だけを発行する）。退会トランザクション（user.Service.withdrawTx）への
// 統合経路として提供する（PostgresSessionRepo / PostgresAuthCodeRepo と同型）。
func (r *PostgresRefreshTokenRepo) DeleteByUserIDExec(ctx context.Context, q DBTX, userID string) error {
	if _, err := q.ExecContext(ctx,
		`DELETE FROM refresh_token_families WHERE user_id = $1`,
		userID,
	); err != nil {
		// NFR 1.2: user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to delete refresh_token_families by user: %w", err)
	}
	return nil
}

// compile-time interface check
var _ RefreshTokenRepository = (*PostgresRefreshTokenRepo)(nil)
