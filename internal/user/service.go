// Package user はユーザー管理のドメインロジックを提供する。
package user

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// SubscriptionDeleter は購読の一括削除インターフェース。
type SubscriptionDeleter interface {
	DeleteByUserID(ctx context.Context, userID string) error
}

// ItemStateDeleter は記事状態の一括削除インターフェース。
type ItemStateDeleter interface {
	DeleteByUserID(ctx context.Context, userID string) error
}

// Service はユーザー管理のサービス層。
// 退会処理のビジネスロジックを提供する。
type Service struct {
	userRepo      repository.UserRepository
	sessionRepo   repository.SessionRepository
	subDeleter    SubscriptionDeleter
	stateDeleter  ItemStateDeleter
}

// NewService はServiceの新しいインスタンスを生成する。
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

// Withdraw はユーザーの退会処理を実行する。
// 削除順序: item_states → subscriptions → sessions → user（+ CASCADE: identities, user_settings）
// feeds と items は共有キャッシュとして残す。
func (s *Service) Withdraw(ctx context.Context, userID string) error {
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
