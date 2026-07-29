package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/lib/pq"
)

// PostgresPasskeyCredentialRepo は PasskeyCredentialRepository の PostgreSQL 実装
// （Issue #216 / Req 1.2 / 3.2 / 3.4 / 3.6 / 7.1〜7.5 / NFR 1.1 / NFR 1.2）。
//
// 保存対象は WebAuthn credential の検証情報（公開鍵・credential_id・sign counter・
// attestation type・aaguid・transports）のみで、パスキー本体の秘密情報は保持しない
// （NFR 1.1）。エラーメッセージには credential_id / public_key / user_id 等の
// 機密値を含めない（NFR 1.2）。
type PostgresPasskeyCredentialRepo struct {
	db *sql.DB
}

// NewPostgresPasskeyCredentialRepo は PostgresPasskeyCredentialRepo を生成する。
func NewPostgresPasskeyCredentialRepo(db *sql.DB) *PostgresPasskeyCredentialRepo {
	return &PostgresPasskeyCredentialRepo{db: db}
}

// transportsSeparator は Transports スライスを DB カラム（VARCHAR(255)）へ
// 決定論的にシリアライズする際の区切り文字。WebAuthn の transports 値は固定 enum
// （"usb" / "nfc" / "ble" / "internal" / "hybrid"）でカンマを含まないため、
// カンマ区切りでラウンドトリップ可能。空スライス → 空文字。
const transportsSeparator = ","

// serializeTransports は Transports スライスを DB カラム値にシリアライズする。
// 空スライス / nil は空文字に変換する。
func serializeTransports(t []string) string {
	if len(t) == 0 {
		return ""
	}
	return strings.Join(t, transportsSeparator)
}

// deserializeTransports は DB カラム値を Transports スライスに戻す。
// 空文字は nil を返し、空スライスと区別せず「未設定」として扱う。
func deserializeTransports(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, transportsSeparator)
}

// Create は credential を新規保存する（Req 1.2 / 3.2）。
//
// credential_id の UNIQUE 制約違反時は ErrCredentialAlreadyRegistered に変換する
// （Req 3.6 の防衛線 / 1.7）。実体は CreateExec に委譲し、非トランザクション時は
// *sql.DB を渡す。共有トランザクション上で INSERT したい場合は CreateExec を直接
// 呼ぶ（Issue #230 / Req 1.1〜1.6: 新規パスキー登録 finish の 1 tx 化に用いる）。
func (r *PostgresPasskeyCredentialRepo) Create(ctx context.Context, c *model.PasskeyCredential) error {
	return r.CreateExec(ctx, r.db, c)
}

// CreateExec は指定の DBTX（*sql.DB または共有トランザクション）上で credential
// を新規保存する（Req 1.2 / 3.2、Issue #230 / Req 1.1〜1.6 / NFR 1.1 の 1 tx 化サポート）。
//
// credential_id の UNIQUE 制約違反時は ErrCredentialAlreadyRegistered に変換する。
// 当該 INSERT で発生し得る UNIQUE 制約は `passkey_credentials_credential_id_key`
// （credential_id UNIQUE）のみのため、23505 を素直に ErrCredentialAlreadyRegistered
// に対応させる。c.ID が空文字 / c.CreatedAt が zero-value の場合は DB 側デフォルトを
// 採用する。
func (r *PostgresPasskeyCredentialRepo) CreateExec(
	ctx context.Context, q DBTX, c *model.PasskeyCredential,
) error {
	if c == nil {
		return fmt.Errorf("failed to create passkey credential: credential is nil")
	}
	var idArg interface{}
	if c.ID != "" {
		idArg = c.ID
	}
	var createdAtArg interface{}
	if !c.CreatedAt.IsZero() {
		createdAtArg = c.CreatedAt
	}
	// last_used_at は nullable。nil のとき DB へ NULL を渡す。
	var lastUsedAtArg interface{}
	if c.LastUsedAt != nil {
		lastUsedAtArg = *c.LastUsedAt
	}
	// aaguid は nullable BYTEA。空スライスと NULL を区別せず、len==0 で NULL 扱いにする。
	var aaguidArg interface{}
	if len(c.AAGUID) > 0 {
		aaguidArg = c.AAGUID
	}
	err := q.QueryRowContext(ctx,
		`INSERT INTO passkey_credentials (
		     id, user_id, credential_id, public_key, sign_count,
		     attestation_type, aaguid, transports, created_at, last_used_at,
		     backup_eligible, backup_state
		 )
		 VALUES (
		     COALESCE($1::uuid, gen_random_uuid()),
		     $2, $3, $4, $5, $6, $7, $8,
		     COALESCE($9::timestamptz, now()),
		     $10,
		     $11, $12
		 )
		 RETURNING id, created_at`,
		idArg, c.UserID, c.CredentialID, c.PublicKey, int64(c.SignCount),
		c.AttestationType, aaguidArg, serializeTransports(c.Transports),
		createdAtArg, lastUsedAtArg,
		c.BackupEligible, c.BackupState,
	).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == pgErrCodeUniqueViolation {
			return ErrCredentialAlreadyRegistered
		}
		// NFR 1.2: credential_id / public_key / user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to create passkey credential: %w", err)
	}
	return nil
}

// FindByCredentialID は credential_id で credential を検索する（Req 2.2 / 3.6）。
// 見つからない場合は (nil, nil) を返す。
func (r *PostgresPasskeyCredentialRepo) FindByCredentialID(
	ctx context.Context, credentialID []byte,
) (*model.PasskeyCredential, error) {
	c := &model.PasskeyCredential{}
	var signCount int64
	var transports string
	var aaguid []byte
	var lastUsedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, credential_id, public_key, sign_count,
		        attestation_type, aaguid, transports, created_at, last_used_at,
		        backup_eligible, backup_state
		 FROM passkey_credentials
		 WHERE credential_id = $1`,
		credentialID,
	).Scan(
		&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &signCount,
		&c.AttestationType, &aaguid, &transports, &c.CreatedAt, &lastUsedAt,
		&c.BackupEligible, &c.BackupState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		// NFR 1.2: credential_id の値はメッセージに含めない。
		return nil, fmt.Errorf("failed to find passkey credential: %w", err)
	}
	c.SignCount = uint32(signCount)
	c.Transports = deserializeTransports(transports)
	if len(aaguid) > 0 {
		c.AAGUID = aaguid
	}
	c.LastUsedAt = nullTimeValue(lastUsedAt)
	return c, nil
}

// ListByUserID は当該 user に紐付く全 credential を返す（Req 3.4 / 3.1 の excludeCredentials 用途）。
// 存在しない場合は空スライスを返す。
func (r *PostgresPasskeyCredentialRepo) ListByUserID(
	ctx context.Context, userID string,
) ([]*model.PasskeyCredential, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, credential_id, public_key, sign_count,
		        attestation_type, aaguid, transports, created_at, last_used_at,
		        backup_eligible, backup_state
		 FROM passkey_credentials
		 WHERE user_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		// NFR 1.2: user_id の値はメッセージに含めない。
		return nil, fmt.Errorf("failed to list passkey credentials: %w", err)
	}
	defer rows.Close()

	var results []*model.PasskeyCredential
	for rows.Next() {
		c := &model.PasskeyCredential{}
		var signCount int64
		var transports string
		var aaguid []byte
		var lastUsedAt sql.NullTime
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &signCount,
			&c.AttestationType, &aaguid, &transports, &c.CreatedAt, &lastUsedAt,
			&c.BackupEligible, &c.BackupState,
		); err != nil {
			return nil, fmt.Errorf("failed to scan passkey credential: %w", err)
		}
		c.SignCount = uint32(signCount)
		c.Transports = deserializeTransports(transports)
		if len(aaguid) > 0 {
			c.AAGUID = aaguid
		}
		c.LastUsedAt = nullTimeValue(lastUsedAt)
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate passkey credentials: %w", err)
	}
	return results, nil
}

// UpdateAuthenticationState は認証 ceremony 成功時に当該 credential の
// sign_count / backup_state / last_used_at の 3 列を更新する
// （Issue #234 / Req 4.1, 4.2, 4.3）。
//
// backup_eligible は本メソッドで **一切更新しない**（引数にも SQL SET 句にも
// 含めない）。これにより Req 4.3「BE は再認証時に上書き更新しない = 初回登録時の
// 値を不変で保持する」を SQL レベルで構造的に保証する。将来 code 側で誤って BE を
// 渡そうとしても、SQL に到達しない構造で担保している。
//
// sign_count は uint32 だが DB カラムは BIGINT（将来拡張余地）のため int64 に
// 昇格して UPDATE する。対象レコードが存在しない場合はエラーにせず 0 rows で成功する
// （呼び出し側が事前に FindByCredentialID で存在確認する前提）。
// メッセージには credential_id / user_id 等の機密値を含めない（NFR 1.2）。
func (r *PostgresPasskeyCredentialRepo) UpdateAuthenticationState(
	ctx context.Context, id string, signCount uint32, backupState bool, lastUsedAt time.Time,
) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE passkey_credentials
		 SET sign_count = $2, backup_state = $3, last_used_at = $4
		 WHERE id = $1`,
		id, int64(signCount), backupState, lastUsedAt,
	)
	if err != nil {
		// NFR 1.2: id の値もメッセージに含めない。
		return fmt.Errorf("failed to update passkey credential authentication state: %w", err)
	}
	return nil
}

// DeleteByUserID は当該ユーザーに紐付く全 passkey_credentials を削除する
// （Issue #216 / Req 7.1 / 7.4）。対象 0 件でも成功する（冪等）。
func (r *PostgresPasskeyCredentialRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return r.DeleteByUserIDExec(ctx, r.db, userID)
}

// DeleteByUserIDExec は指定の DBTX（*sql.DB または共有トランザクション）上で
// 当該ユーザーに紐付く全 passkey_credentials を削除する（Req 7.2 / 7.3）。
//
// SQL: DELETE FROM passkey_credentials WHERE user_id = $1
// PostgresAuthCodeRepo.DeleteByUserIDExec / PostgresSessionRepo.DeleteByUserIDExec と同型で、
// 退会 tx（user.Service.withdrawTx）に統合するための共有 tx 対応版。
func (r *PostgresPasskeyCredentialRepo) DeleteByUserIDExec(
	ctx context.Context, q DBTX, userID string,
) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM passkey_credentials WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		// NFR 1.2: user_id の値はメッセージに含めない。
		return fmt.Errorf("failed to delete passkey credentials by user: %w", err)
	}
	return nil
}

// compile-time interface check
var _ PasskeyCredentialRepository = (*PostgresPasskeyCredentialRepo)(nil)
