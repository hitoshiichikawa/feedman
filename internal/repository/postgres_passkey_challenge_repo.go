package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresPasskeyChallengeRepo は PasskeyChallengeRepository の PostgreSQL 実装
// （Issue #216 / Req 4.1 / 4.2 / 4.3 / 4.4 / NFR 1.2）。
//
// 生 challenge 値は保存せず、SHA-256 hex（challenge_hash）のみを保持する（NFR 1.2）。
// MarkConsumed は UPDATE の atomic 判定により単回利用を保証する（Req 4.3 /
// PostgresAuthCodeRepo.MarkUsed と同流儀）。エラーメッセージには challenge_hash /
// session_data / user_id 等の機密値を含めない（NFR 1.2）。
type PostgresPasskeyChallengeRepo struct {
	db *sql.DB
}

// NewPostgresPasskeyChallengeRepo は PostgresPasskeyChallengeRepo を生成する。
func NewPostgresPasskeyChallengeRepo(db *sql.DB) *PostgresPasskeyChallengeRepo {
	return &PostgresPasskeyChallengeRepo{db: db}
}

// Create は challenge を新規保存する（Req 4.1 / 4.2）。
//
// ch.ChallengeHash / ch.Kind / ch.SessionData / ch.ExpiresAt は呼び出し側で
// 確定済みであること。ch.ID が空文字 / ch.CreatedAt が zero-value の場合は
// DB 側デフォルト（gen_random_uuid() / now()）を採用し、確定値を ch に反映する。
// UserID / PendingUsername は nullable（それぞれ nil / *string で受け取る）。
func (r *PostgresPasskeyChallengeRepo) Create(ctx context.Context, ch *model.PasskeyChallenge) error {
	if ch == nil {
		return fmt.Errorf("failed to create passkey challenge: challenge is nil")
	}
	var idArg interface{}
	if ch.ID != "" {
		idArg = ch.ID
	}
	var createdAtArg interface{}
	if !ch.CreatedAt.IsZero() {
		createdAtArg = ch.CreatedAt
	}
	var userIDArg interface{}
	if ch.UserID != nil {
		userIDArg = *ch.UserID
	}
	var pendingUsernameArg interface{}
	if ch.PendingUsername != nil {
		pendingUsernameArg = *ch.PendingUsername
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO passkey_challenges (
		     id, challenge_hash, kind, user_id, pending_username,
		     session_data, expires_at, consumed, created_at
		 )
		 VALUES (
		     COALESCE($1::uuid, gen_random_uuid()),
		     $2, $3, $4, $5, $6, $7, $8,
		     COALESCE($9::timestamptz, now())
		 )
		 RETURNING id, created_at`,
		idArg, ch.ChallengeHash, string(ch.Kind), userIDArg, pendingUsernameArg,
		ch.SessionData, ch.ExpiresAt, ch.Consumed, createdAtArg,
	).Scan(&ch.ID, &ch.CreatedAt)
	if err != nil {
		// NFR 1.2: challenge_hash / session_data の値はメッセージに含めない。
		return fmt.Errorf("failed to create passkey challenge: %w", err)
	}
	return nil
}

// FindByHash は challenge_hash に一致するレコードを 1 件返す。
// 見つからない場合は (nil, nil) を返す（既存 FindByHash パターンに整合）。
func (r *PostgresPasskeyChallengeRepo) FindByHash(
	ctx context.Context, hash string,
) (*model.PasskeyChallenge, error) {
	return r.findOne(ctx, `WHERE challenge_hash = $1`, hash)
}

// FindByID は id（PK / opaque challenge_id）でレコードを 1 件返す。
// 見つからない場合は (nil, nil) を返す。ChallengeStore.Consume が client から
// 受け取る opaque challenge_id で challenge を逆引きするために用いる
// （tasks.md task 3 スコープ調整で本 interface に追加）。
func (r *PostgresPasskeyChallengeRepo) FindByID(
	ctx context.Context, id string,
) (*model.PasskeyChallenge, error) {
	return r.findOne(ctx, `WHERE id = $1`, id)
}

// findOne は WHERE 句と引数を受け取って 1 行取得する内部ヘルパー。
// FindByHash / FindByID の SELECT / Scan 共通部分を集約する。
func (r *PostgresPasskeyChallengeRepo) findOne(
	ctx context.Context, whereClause string, arg interface{},
) (*model.PasskeyChallenge, error) {
	query := `SELECT id, challenge_hash, kind, user_id, pending_username,
	                 session_data, expires_at, consumed, created_at
	          FROM passkey_challenges ` + whereClause

	ch := &model.PasskeyChallenge{}
	var kind string
	var userID sql.NullString
	var pendingUsername sql.NullString
	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&ch.ID, &ch.ChallengeHash, &kind, &userID, &pendingUsername,
		&ch.SessionData, &ch.ExpiresAt, &ch.Consumed, &ch.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		// NFR 1.2: 検索キー値はメッセージに含めない。
		return nil, fmt.Errorf("failed to find passkey challenge: %w", err)
	}
	ch.Kind = model.PasskeyChallengeKind(kind)
	if userID.Valid {
		s := userID.String
		ch.UserID = &s
	}
	if pendingUsername.Valid {
		s := pendingUsername.String
		ch.PendingUsername = &s
	}
	return ch, nil
}

// MarkConsumed は当該 ID の challenge を consumed = true に遷移させる（Req 4.3）。
//
// レコードが以下のいずれかに該当する場合は ErrChallengeNotUsable を返し、
// 永続化状態は変更しない（Req 4.4）:
//   - 既に consumed = true（二重消費）
//   - expires_at <= now()（期限切れ）
//   - id に一致するレコードが存在しない
//
// UPDATE 文の WHERE 句で consumed / expires_at をまとめて判定することで、
// 並行アクセス下でも race を起こさず単回利用を保証する（race 安全 /
// PostgresAuthCodeRepo.MarkUsed と同流儀）。
func (r *PostgresPasskeyChallengeRepo) MarkConsumed(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE passkey_challenges
		 SET consumed = true
		 WHERE id = $1 AND consumed = false AND expires_at > now()`,
		id,
	)
	if err != nil {
		// NFR 1.2: id の値もメッセージに含めない。
		return fmt.Errorf("failed to mark passkey challenge consumed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark passkey challenge consumed: %w", err)
	}
	if affected == 0 {
		return ErrChallengeNotUsable
	}
	return nil
}

// compile-time interface check
var _ PasskeyChallengeRepository = (*PostgresPasskeyChallengeRepo)(nil)
