package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresAuthCodeRepo は AuthCodeRepository の PostgreSQL 実装。
//
// 本実装は native auth の使い捨て auth_code（Issue #163 / #164）の
// 保存・hash 一致参照・単回利用確定を提供する。平文 code は引数にも戻り値にも一切含めず、
// SHA-256 由来の hash 文字列（code_hash）のみを保持する（NFR 1.1 / 1.2）。
// エラーメッセージにも code_hash の値や平文を含めない。
type PostgresAuthCodeRepo struct {
	db *sql.DB
}

// NewPostgresAuthCodeRepo は PostgresAuthCodeRepo を生成する。
func NewPostgresAuthCodeRepo(db *sql.DB) *PostgresAuthCodeRepo {
	return &PostgresAuthCodeRepo{db: db}
}

// Create は AuthCode を新規保存する（Req 2.1, 2.2, 2.3）。
// code.CodeHash / code.UserID / code.PKCEChallenge / code.ExpiresAt は
// 呼び出し側で確定済みであること。
// code.ID が空文字 / code.CreatedAt が zero-value の場合はそれぞれ DB の
// デフォルト（gen_random_uuid() / now()）を採用し、確定値を呼び出し元の struct に反映する。
func (r *PostgresAuthCodeRepo) Create(ctx context.Context, code *model.AuthCode) error {
	if code == nil {
		return fmt.Errorf("failed to create auth_code: code is nil")
	}
	// id が空文字 / created_at が zero-value のときは sql.NullX として渡し、
	// COALESCE で DB 側デフォルトに委ねる（呼び出し側に UUID 生成義務を負わせない）。
	var idArg interface{}
	if code.ID != "" {
		idArg = code.ID
	} else {
		idArg = nil
	}
	var createdAtArg interface{}
	if !code.CreatedAt.IsZero() {
		createdAtArg = code.CreatedAt
	} else {
		createdAtArg = nil
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO auth_codes (id, code_hash, user_id, pkce_challenge, expires_at, used, created_at)
		 VALUES (
		     COALESCE($1::uuid, gen_random_uuid()),
		     $2, $3, $4, $5, $6,
		     COALESCE($7::timestamptz, now())
		 )
		 RETURNING id, created_at`,
		idArg, code.CodeHash, code.UserID, code.PKCEChallenge, code.ExpiresAt, code.Used, createdAtArg,
	).Scan(&code.ID, &code.CreatedAt)
	if err != nil {
		// NFR 1.2: code_hash や user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to create auth_code: %w", err)
	}
	return nil
}

// FindByHash は code_hash に一致するレコードを 1 件返す（Req 2.4）。
// 見つからない場合は (nil, nil) を返す（既存 FindByID / FindByHash パターンに整合）。
func (r *PostgresAuthCodeRepo) FindByHash(ctx context.Context, codeHash string) (*model.AuthCode, error) {
	code := &model.AuthCode{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code_hash, user_id, pkce_challenge, expires_at, used, created_at
		 FROM auth_codes
		 WHERE code_hash = $1`,
		codeHash,
	).Scan(
		&code.ID,
		&code.CodeHash,
		&code.UserID,
		&code.PKCEChallenge,
		&code.ExpiresAt,
		&code.Used,
		&code.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		// NFR 1.2: code_hash の値はメッセージに含めない。
		return nil, fmt.Errorf("failed to find auth_code: %w", err)
	}
	return code, nil
}

// MarkUsed は当該 ID の auth_code を used = true に遷移させる（Req 2.5）。
//
// レコードが以下のいずれかに該当する場合は ErrAuthCodeNotUsable を返し、
// 永続化状態は変更しない（Req 2.6）:
//   - 既に used = true
//   - expires_at <= now()
//   - id に一致するレコードが存在しない
//
// UPDATE 文の WHERE 句で used / expires_at をまとめて判定することで、
// 並行アクセス下でも race を起こさず単回利用を保証する（race 安全）。
func (r *PostgresAuthCodeRepo) MarkUsed(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE auth_codes
		 SET used = true
		 WHERE id = $1 AND used = false AND expires_at > now()`,
		id,
	)
	if err != nil {
		// NFR 1.2: id の値も詳細メッセージに含めない（"failed to mark auth_code used" のみ）。
		return fmt.Errorf("failed to mark auth_code used: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark auth_code used: %w", err)
	}
	if affected == 0 {
		return ErrAuthCodeNotUsable
	}
	return nil
}

// DeleteByUserID は当該ユーザーに属する全ての auth_code を削除する（Issue #170 Req 1.1）。
//
// 対象 0 件でも成功する（冪等）。退会フロー（user.Service.withdrawTx）からの
// 明示的削除経路として提供する。FK ON DELETE CASCADE による防衛線も維持される
// （RefreshTokenRepository.DeleteByUserID と対）。
func (r *PostgresAuthCodeRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return r.DeleteByUserIDExec(ctx, r.db, userID)
}

// DeleteByUserIDExec は指定の DBTX（*sql.DB または共有トランザクション）上で
// 当該ユーザーに属する全ての auth_code を削除する（Issue #170 Req 1.1, 2.1）。
//
// SQL: DELETE FROM auth_codes WHERE user_id = $1
// PostgresSessionRepo の 2 段パターン（DeleteByUserID + DeleteByUserIDExec）と同型。
func (r *PostgresAuthCodeRepo) DeleteByUserIDExec(ctx context.Context, q DBTX, userID string) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM auth_codes WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		// NFR 1.2: user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to delete auth_codes by user: %w", err)
	}
	return nil
}

// compile-time interface check
var _ AuthCodeRepository = (*PostgresAuthCodeRepo)(nil)
