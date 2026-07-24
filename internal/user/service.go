// Package user はユーザー管理のドメインロジックを提供する。
package user

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// SubscriptionDeleter は購読の一括削除インターフェース（レガシー・非トランザクション）。
type SubscriptionDeleter interface {
	DeleteByUserID(ctx context.Context, userID string) error
}

// ItemStateDeleter は記事状態の一括削除インターフェース（レガシー・非トランザクション）。
type ItemStateDeleter interface {
	DeleteByUserID(ctx context.Context, userID string) error
}

// Tx は退会処理で共有するトランザクションハンドル。
// database/sql の *sql.Tx が満たす Commit / Rollback の最小集合であり、
// テストでは fake 実装に差し替えられるよう抽象化している。
type Tx interface {
	// Commit はトランザクションを確定する。
	Commit() error
	// Rollback はトランザクションを取り消す。確定済みの場合は sql.ErrTxDone を返す。
	Rollback() error
}

// TxBeginner はトランザクションを開始するインターフェース。
type TxBeginner interface {
	// BeginTx は新しいトランザクションを開始する。
	BeginTx(ctx context.Context) (Tx, error)
}

// TxUserDeleter はユーザーの存在確認と、共有トランザクション上での削除を行うインターフェース。
type TxUserDeleter interface {
	// FindByID は指定IDのユーザーを取得する。見つからない場合はnilを返す。
	FindByID(ctx context.Context, id string) (*model.User, error)
	// DeleteByIDTx は共有トランザクション上でユーザーを削除する。
	// 関連する identities / user_settings は CASCADE 削除される。
	DeleteByIDTx(ctx context.Context, tx Tx, id string) error
}

// TxSessionDeleter は共有トランザクション上でセッションを一括削除するインターフェース。
type TxSessionDeleter interface {
	DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error
}

// TxSubscriptionDeleter は共有トランザクション上で購読を一括削除するインターフェース。
type TxSubscriptionDeleter interface {
	DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error
}

// TxItemStateDeleter は共有トランザクション上で記事状態を一括削除するインターフェース。
type TxItemStateDeleter interface {
	DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error
}

// TxAuthCodeDeleter は共有トランザクション上で native auth 一時認可コードを
// 一括削除するインターフェース（Issue #170 Req 1.1, 2.1）。
type TxAuthCodeDeleter interface {
	DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error
}

// TxRefreshTokenDeleter は共有トランザクション上で refresh token（family ごと）を
// 一括削除するインターフェース（Issue #170 Req 1.2, 2.1）。
type TxRefreshTokenDeleter interface {
	DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error
}

// TxPasskeyCredentialDeleter は共有トランザクション上でパスキー credential を
// 一括削除するインターフェース（Issue #216 Req 7.1, 7.2, 7.3, 7.4）。
// 既存 TxAuthCodeDeleter / TxRefreshTokenDeleter と同型で、退会 tx への
// passkey_credentials 削除段の統合に用いる。
type TxPasskeyCredentialDeleter interface {
	DeleteByUserIDTx(ctx context.Context, tx Tx, userID string) error
}

// Service はユーザー管理のサービス層。
// 退会処理のビジネスロジックを提供する。
//
// txBeginner が設定されている場合、退会処理は単一トランザクションで原子的に実行され、
// 途中失敗時は全削除がロールバックされる（推奨パス）。txBeginner が nil の場合は、
// 後方互換のためレガシーの逐次削除パス（非原子）にフォールバックする。
type Service struct {
	// レガシー（非トランザクション）パス用のフィールド。
	userRepo     repository.UserRepository
	sessionRepo  repository.SessionRepository
	subDeleter   SubscriptionDeleter
	stateDeleter ItemStateDeleter

	// トランザクションパス用のフィールド（txBeginner != nil のとき使用）。
	txBeginner                 TxBeginner
	txUserDeleter              TxUserDeleter
	txSessionDeleter           TxSessionDeleter
	txSubDeleter               TxSubscriptionDeleter
	txStateDeleter             TxItemStateDeleter
	txAuthCodeDeleter          TxAuthCodeDeleter          // Issue #170: native auth 認可コード削除
	txRefreshTokenDeleter      TxRefreshTokenDeleter      // Issue #170: native auth refresh token 削除
	txPasskeyCredentialDeleter TxPasskeyCredentialDeleter // Issue #216: passkey credential 削除
}

// NewService は Service の新しいインスタンスを生成する（レガシー・非トランザクションパス）。
//
// 後方互換のためシグネチャを維持している。原子的な退会処理を行う場合は
// NewServiceWithTx を使用すること。
func NewService(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	subDeleter SubscriptionDeleter,
	stateDeleter ItemStateDeleter,
) *Service {
	return &Service{
		userRepo:     userRepo,
		sessionRepo:  sessionRepo,
		subDeleter:   subDeleter,
		stateDeleter: stateDeleter,
	}
}

// NewServiceWithTx はトランザクション対応の Service を生成する。
//
// 退会処理は txBeginner が開始する単一トランザクション上で
// item_states → subscriptions → sessions → passkey_credentials → auth_codes →
// refresh_token_families → user の順に削除し、全成功時のみコミット、途中失敗時は
// 全ロールバックする（Issue #170 で auth_codes / refresh_token_families の削除
// 2 段を sessions の直後・user の前に挿入し、Issue #216 で passkey_credentials
// 削除段を sessions の直後・auth_codes の直前に追加）。
//
// authCodeDeleter / refreshTokenDeleter / passkeyCredentialDeleter は nil でも
// 構築でき、その場合は当該段はスキップされる（既存 deleter 群と同じ nil ガード
// 方針。本番 wiring では常に非 nil を注入する）。
func NewServiceWithTx(
	txBeginner TxBeginner,
	userDeleter TxUserDeleter,
	sessionDeleter TxSessionDeleter,
	subDeleter TxSubscriptionDeleter,
	stateDeleter TxItemStateDeleter,
	authCodeDeleter TxAuthCodeDeleter,
	refreshTokenDeleter TxRefreshTokenDeleter,
	passkeyCredentialDeleter TxPasskeyCredentialDeleter,
) *Service {
	return &Service{
		txBeginner:                 txBeginner,
		txUserDeleter:              userDeleter,
		txSessionDeleter:           sessionDeleter,
		txSubDeleter:               subDeleter,
		txStateDeleter:             stateDeleter,
		txAuthCodeDeleter:          authCodeDeleter,
		txRefreshTokenDeleter:      refreshTokenDeleter,
		txPasskeyCredentialDeleter: passkeyCredentialDeleter,
	}
}

// GetByID は指定 userID の current user を取得する。
//
// 認可は呼び出し側（BearerOrSession middleware）が担保しており、本メソッドは
// ビジネス認可を行わない（caller userID = lookup userID 前提）。userID が DB 上に
// 存在しない場合は model.NewUserNotFoundError を返す（既存 Withdraw と同パターン）。
//
// txBeginner が設定されている場合は txUserDeleter.FindByID を、設定されていない
// 場合は userRepo.FindByID を呼ぶ（既存 Withdraw の lookup と同一の選択ロジック）。
func (s *Service) GetByID(ctx context.Context, userID string) (*model.User, error) {
	var (
		user *model.User
		err  error
	)
	if s.txBeginner != nil {
		user, err = s.txUserDeleter.FindByID(ctx, userID)
	} else {
		user, err = s.userRepo.FindByID(ctx, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("ユーザーの取得に失敗しました: %w", err)
	}
	if user == nil {
		return nil, model.NewUserNotFoundError()
	}
	return user, nil
}

// Withdraw はユーザーの退会処理を実行する。
// 削除順序（トランザクションパス）: item_states → subscriptions → sessions →
// passkey_credentials → auth_codes → refresh_token_families → user
// （+ CASCADE: identities, user_settings, refresh_tokens）
// feeds と items は共有キャッシュとして残す。
//
// Issue #170: native auth の認証状態（auth_codes / refresh_token_families）を
// sessions の直後・user の前に挿入する。refresh_tokens は
// refresh_token_families の DELETE に伴い FK ON DELETE CASCADE で削除される。
// Issue #216: passkey_credentials 削除段を sessions の直後・auth_codes の直前に
// 追加する（Req 7.1, 7.2, 7.3, 7.4）。passkey_credentials は user_id FK ON DELETE
// CASCADE も持っているが、明示削除により tx 内の順序と失敗時 rollback 契約を確定させる。
//
// txBeginner が設定されている場合は単一トランザクションで原子的に削除し、
// 途中失敗時は全ロールバックする。設定されていない場合はレガシーの逐次削除を行う
// （レガシーパスでは native auth / passkey の明示削除を行わず、user 削除時の
// FK CASCADE による DB 防衛線で行残存を防ぐ）。
func (s *Service) Withdraw(ctx context.Context, userID string) error {
	if s.txBeginner != nil {
		return s.withdrawTx(ctx, userID)
	}
	return s.withdrawLegacy(ctx, userID)
}

// withdrawTx は単一トランザクション上で原子的に退会処理を実行する。
func (s *Service) withdrawTx(ctx context.Context, userID string) error {
	// ユーザー存在確認（トランザクション外で実施。存在しなければ何も削除しない）。
	user, err := s.txUserDeleter.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("ユーザーの取得に失敗しました: %w", err)
	}
	if user == nil {
		return model.NewUserNotFoundError()
	}

	slog.Info("退会処理を開始します",
		slog.String("user_id", userID),
	)

	tx, err := s.txBeginner.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	// 確定前に関数を抜けた場合は必ずロールバックする。コミット済みなら no-op。
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1. 記事状態を削除
	if s.txStateDeleter != nil {
		if err := s.txStateDeleter.DeleteByUserIDTx(ctx, tx, userID); err != nil {
			return fmt.Errorf("記事状態の削除に失敗しました: %w", err)
		}
	}

	// 2. 購読を削除
	if s.txSubDeleter != nil {
		if err := s.txSubDeleter.DeleteByUserIDTx(ctx, tx, userID); err != nil {
			return fmt.Errorf("購読の削除に失敗しました: %w", err)
		}
	}

	// 3. セッションを削除
	if s.txSessionDeleter != nil {
		if err := s.txSessionDeleter.DeleteByUserIDTx(ctx, tx, userID); err != nil {
			return fmt.Errorf("セッションの削除に失敗しました: %w", err)
		}
	}

	// 4. passkey credential を削除（Issue #216 Req 7.1, 7.2, 7.3, 7.4）
	//    削除段は sessions の直後・auth_codes の直前に挿入する。deleter が nil の
	//    場合はスキップし、user 削除時の FK ON DELETE CASCADE を防衛線として残す。
	if s.txPasskeyCredentialDeleter != nil {
		if err := s.txPasskeyCredentialDeleter.DeleteByUserIDTx(ctx, tx, userID); err != nil {
			return fmt.Errorf("passkey credential の削除に失敗しました: %w", err)
		}
	}

	// 5. native auth 認可コードを削除（Issue #170 Req 1.1, 2.1）
	if s.txAuthCodeDeleter != nil {
		if err := s.txAuthCodeDeleter.DeleteByUserIDTx(ctx, tx, userID); err != nil {
			return fmt.Errorf("認可コードの削除に失敗しました: %w", err)
		}
	}

	// 6. native auth refresh token（family ごと、配下 tokens は FK CASCADE）を削除
	//    （Issue #170 Req 1.2, 2.1）
	if s.txRefreshTokenDeleter != nil {
		if err := s.txRefreshTokenDeleter.DeleteByUserIDTx(ctx, tx, userID); err != nil {
			return fmt.Errorf("refresh token の削除に失敗しました: %w", err)
		}
	}

	// 7. ユーザーを削除（identities, user_settings は CASCADE 削除）
	if err := s.txUserDeleter.DeleteByIDTx(ctx, tx, userID); err != nil {
		return fmt.Errorf("ユーザーの削除に失敗しました: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("トランザクションの確定に失敗しました: %w", err)
	}
	committed = true

	slog.Info("退会処理が完了しました",
		slog.String("user_id", userID),
	)

	return nil
}

// withdrawLegacy は後方互換のための非トランザクション逐次削除パス。
// txBeginner が設定されていない場合に使用する。
func (s *Service) withdrawLegacy(ctx context.Context, userID string) error {
	// ユーザー存在確認
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("ユーザーの取得に失敗しました: %w", err)
	}
	if user == nil {
		return model.NewUserNotFoundError()
	}

	slog.Info("退会処理を開始します",
		slog.String("user_id", userID),
	)

	// 1. 記事状態を削除
	if s.stateDeleter != nil {
		if err := s.stateDeleter.DeleteByUserID(ctx, userID); err != nil {
			return fmt.Errorf("記事状態の削除に失敗しました: %w", err)
		}
	}

	// 2. 購読を削除
	if s.subDeleter != nil {
		if err := s.subDeleter.DeleteByUserID(ctx, userID); err != nil {
			return fmt.Errorf("購読の削除に失敗しました: %w", err)
		}
	}

	// 3. セッションを削除
	if s.sessionRepo != nil {
		if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
			return fmt.Errorf("セッションの削除に失敗しました: %w", err)
		}
	}

	// 4. ユーザーを削除（identities, user_settingsはCASCADE削除）
	if err := s.userRepo.DeleteByID(ctx, userID); err != nil {
		return fmt.Errorf("ユーザーの削除に失敗しました: %w", err)
	}

	slog.Info("退会処理が完了しました",
		slog.String("user_id", userID),
	)

	return nil
}
