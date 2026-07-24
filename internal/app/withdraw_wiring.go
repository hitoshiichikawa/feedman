package app

import (
	"context"
	"fmt"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
	"github.com/hitoshi/feedman/internal/user"
)

// 本ファイルは退会処理（user.Service）をトランザクション対応で組み立てるための
// 配線（wiring）アダプタを提供する。repository パッケージは user パッケージを
// import できない（user → repository の依存があるため）ことから、両者を結ぶ
// アダプタを app パッケージ側に置く。

// txBeginnerAdapter は *repository.SQLTxBeginner を user.TxBeginner に適合させる。
type txBeginnerAdapter struct {
	beginner *repository.SQLTxBeginner
}

// BeginTx はトランザクションを開始し、user.Tx として返す。
func (a *txBeginnerAdapter) BeginTx(ctx context.Context) (user.Tx, error) {
	tx, err := a.beginner.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// querierFromTx は user.Tx から共有トランザクションの DBTX を取り出す。
// 本配線では user.Tx の実体は必ず *repository.SQLTx であり、その Querier() を用いる。
func querierFromTx(tx user.Tx) (repository.DBTX, error) {
	sqlTx, ok := tx.(*repository.SQLTx)
	if !ok {
		return nil, fmt.Errorf("予期しないトランザクション型です: %T", tx)
	}
	return sqlTx.Querier(), nil
}

// txItemStateDeleterAdapter は記事状態リポジトリを user.TxItemStateDeleter に適合させる。
type txItemStateDeleterAdapter struct {
	repo *repository.PostgresItemStateRepo
}

func (a *txItemStateDeleterAdapter) DeleteByUserIDTx(ctx context.Context, tx user.Tx, userID string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByUserIDExec(ctx, q, userID)
}

// txSubscriptionDeleterAdapter は購読リポジトリを user.TxSubscriptionDeleter に適合させる。
type txSubscriptionDeleterAdapter struct {
	repo *repository.PostgresSubscriptionRepo
}

func (a *txSubscriptionDeleterAdapter) DeleteByUserIDTx(ctx context.Context, tx user.Tx, userID string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByUserIDExec(ctx, q, userID)
}

// txSessionDeleterAdapter はセッションリポジトリを user.TxSessionDeleter に適合させる。
type txSessionDeleterAdapter struct {
	repo *repository.PostgresSessionRepo
}

func (a *txSessionDeleterAdapter) DeleteByUserIDTx(ctx context.Context, tx user.Tx, userID string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByUserIDExec(ctx, q, userID)
}

// txAuthCodeDeleterAdapter は認可コードリポジトリを user.TxAuthCodeDeleter に
// 適合させる（Issue #170 Req 1.1, 2.1）。
type txAuthCodeDeleterAdapter struct {
	repo *repository.PostgresAuthCodeRepo
}

func (a *txAuthCodeDeleterAdapter) DeleteByUserIDTx(ctx context.Context, tx user.Tx, userID string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByUserIDExec(ctx, q, userID)
}

// txRefreshTokenDeleterAdapter は refresh token リポジトリを
// user.TxRefreshTokenDeleter に適合させる（Issue #170 Req 1.2, 2.1）。
// family の DELETE 1 文で配下 tokens は FK CASCADE により連動削除される。
type txRefreshTokenDeleterAdapter struct {
	repo *repository.PostgresRefreshTokenRepo
}

func (a *txRefreshTokenDeleterAdapter) DeleteByUserIDTx(ctx context.Context, tx user.Tx, userID string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByUserIDExec(ctx, q, userID)
}

// txPasskeyCredentialDeleterAdapter は passkey credential リポジトリを
// user.TxPasskeyCredentialDeleter に適合させる（Issue #216 Req 7.1, 7.2, 7.3, 7.4）。
// 退会 tx 内で当該ユーザーに紐付く全 passkey_credentials を DELETE する。
type txPasskeyCredentialDeleterAdapter struct {
	repo *repository.PostgresPasskeyCredentialRepo
}

func (a *txPasskeyCredentialDeleterAdapter) DeleteByUserIDTx(ctx context.Context, tx user.Tx, userID string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByUserIDExec(ctx, q, userID)
}

// txUserDeleterAdapter はユーザーリポジトリを user.TxUserDeleter に適合させる。
type txUserDeleterAdapter struct {
	repo *repository.PostgresUserRepo
}

func (a *txUserDeleterAdapter) FindByID(ctx context.Context, id string) (*model.User, error) {
	return a.repo.FindByID(ctx, id)
}

func (a *txUserDeleterAdapter) DeleteByIDTx(ctx context.Context, tx user.Tx, id string) error {
	q, err := querierFromTx(tx)
	if err != nil {
		return err
	}
	return a.repo.DeleteByIDExec(ctx, q, id)
}

// newTxUserService はトランザクション対応の退会サービスを組み立てる。
//
// Issue #170: native auth deleter（auth_code / refresh_token）を末尾に追加し、
// 退会トランザクションへ auth_codes / refresh_token_families の明示削除を
// 統合する。
// Issue #216: passkey credential deleter をさらに末尾に追加し、退会 tx へ
// passkey_credentials の明示削除を統合する（Req 7.1〜7.4）。
// passkeyCredentialRepo が nil の場合は passkey deleter に nil を渡し、
// 当該段はスキップされる（NFR 2.2 の env 未設定環境互換）。
func newTxUserService(
	beginner *repository.SQLTxBeginner,
	userRepo *repository.PostgresUserRepo,
	sessionRepo *repository.PostgresSessionRepo,
	subRepo *repository.PostgresSubscriptionRepo,
	itemStateRepo *repository.PostgresItemStateRepo,
	authCodeRepo *repository.PostgresAuthCodeRepo,
	refreshTokenRepo *repository.PostgresRefreshTokenRepo,
	passkeyCredentialRepo *repository.PostgresPasskeyCredentialRepo,
) *user.Service {
	// passkey credential deleter は nil-safe: repo が nil なら interface も nil で渡す
	// （typed-nil を作らない）ことで、user.Service の nil ガード経路を素直に踏ませる。
	var passkeyDeleter user.TxPasskeyCredentialDeleter
	if passkeyCredentialRepo != nil {
		passkeyDeleter = &txPasskeyCredentialDeleterAdapter{repo: passkeyCredentialRepo}
	}
	return user.NewServiceWithTx(
		&txBeginnerAdapter{beginner: beginner},
		&txUserDeleterAdapter{repo: userRepo},
		&txSessionDeleterAdapter{repo: sessionRepo},
		&txSubscriptionDeleterAdapter{repo: subRepo},
		&txItemStateDeleterAdapter{repo: itemStateRepo},
		&txAuthCodeDeleterAdapter{repo: authCodeRepo},
		&txRefreshTokenDeleterAdapter{repo: refreshTokenRepo},
		passkeyDeleter,
	)
}

// compile-time interface checks
var (
	_ user.TxBeginner                 = (*txBeginnerAdapter)(nil)
	_ user.TxItemStateDeleter         = (*txItemStateDeleterAdapter)(nil)
	_ user.TxSubscriptionDeleter      = (*txSubscriptionDeleterAdapter)(nil)
	_ user.TxSessionDeleter           = (*txSessionDeleterAdapter)(nil)
	_ user.TxUserDeleter              = (*txUserDeleterAdapter)(nil)
	_ user.TxAuthCodeDeleter          = (*txAuthCodeDeleterAdapter)(nil)
	_ user.TxRefreshTokenDeleter      = (*txRefreshTokenDeleterAdapter)(nil)
	_ user.TxPasskeyCredentialDeleter = (*txPasskeyCredentialDeleterAdapter)(nil)
)
