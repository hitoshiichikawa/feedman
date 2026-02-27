// Package subscription は購読管理のドメインロジックを提供する。
package subscription

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// SubscriptionInfo は購読情報とフィード情報を結合したドメインオブジェクト。
type SubscriptionInfo struct {
	ID                   string
	UserID               string
	FeedID               string
	FeedTitle            string
	FeedURL              string
	FaviconURL           *string
	FetchIntervalMinutes int
	FeedStatus           string
	ErrorMessage         *string
	UnreadCount          int
	CreatedAt            time.Time
}

// Service は購読管理のサービス層。
// 購読一覧取得、設定更新、購読解除、フェッチ再開のビジネスロジックを提供する。
type Service struct {
	subRepo       repository.SubscriptionRepository
	itemStateRepo repository.ItemStateRepository
	feedRepo      repository.FeedRepository
}

// NewService はServiceの新しいインスタンスを生成する。
func NewService(
	subRepo repository.SubscriptionRepository,
	itemStateRepo repository.ItemStateRepository,
	feedRepo repository.FeedRepository,
) *Service {
	return &Service{
		subRepo:       subRepo,
		itemStateRepo: itemStateRepo,
		feedRepo:      feedRepo,
	}
}

// ListSubscriptions はユーザーの購読一覧をフィード情報付きで返す。
func (s *Service) ListSubscriptions(ctx context.Context, userID string) ([]SubscriptionInfo, error) {
	rows, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読一覧の取得に失敗しました: %w", err)
	}

	results := make([]SubscriptionInfo, len(rows))
	for i, row := range rows {
		info := SubscriptionInfo{
			ID:                   row.ID,
			UserID:               row.UserID,
			FeedID:               row.FeedID,
			FeedTitle:            row.FeedTitle,
			FeedURL:              row.FeedURL,
			FetchIntervalMinutes: row.FetchIntervalMinutes,
			FeedStatus:           string(row.FetchStatus),
			UnreadCount:          row.UnreadCount,
			CreatedAt:            row.CreatedAt,
		}

		// faviconデータがある場合はdata URLに変換
		if len(row.FaviconData) > 0 && row.FaviconMime != "" {
			dataURL := fmt.Sprintf("data:%s;base64,%s", row.FaviconMime, base64.StdEncoding.EncodeToString(row.FaviconData))
			info.FaviconURL = &dataURL
		}

		// エラーメッセージがある場合
		if row.ErrorMessage != "" {
			msg := row.ErrorMessage
			info.ErrorMessage = &msg
		}

		results[i] = info
	}

	return results, nil
}

// UpdateSettings は購読のフェッチ間隔を更新する。
func (s *Service) UpdateSettings(ctx context.Context, userID, subscriptionID string, minutes int) (*SubscriptionInfo, error) {
	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}
	if sub.UserID != userID {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}

	if err := s.subRepo.UpdateFetchInterval(ctx, subscriptionID, minutes); err != nil {
		return nil, fmt.Errorf("フェッチ間隔の更新に失敗しました: %w", err)
	}

	// 更新後の購読情報を取得して返す
	infos, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読情報の再取得に失敗しました: %w", err)
	}

	for _, info := range infos {
		if info.ID == subscriptionID {
			result := &SubscriptionInfo{
				ID:                   info.ID,
				UserID:               info.UserID,
				FeedID:               info.FeedID,
				FeedTitle:            info.FeedTitle,
				FeedURL:              info.FeedURL,
				FetchIntervalMinutes: info.FetchIntervalMinutes,
				FeedStatus:           string(info.FetchStatus),
				UnreadCount:          info.UnreadCount,
				CreatedAt:            info.CreatedAt,
			}
			return result, nil
		}
	}

	return nil, model.NewSubscriptionNotFoundError(subscriptionID)
}

// Unsubscribe は購読を解除する。
// subscription と関連 item_states を削除する。
func (s *Service) Unsubscribe(ctx context.Context, userID, subscriptionID string) error {
	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil {
		return model.NewSubscriptionNotFoundError(subscriptionID)
	}
	if sub.UserID != userID {
		return model.NewSubscriptionNotFoundError(subscriptionID)
	}

	// 関連item_statesを削除
	if s.itemStateRepo != nil {
		if err := s.itemStateRepo.DeleteByUserAndFeed(ctx, userID, sub.FeedID); err != nil {
			return fmt.Errorf("記事状態の削除に失敗しました: %w", err)
		}
	}

	// 購読を削除
	if err := s.subRepo.Delete(ctx, subscriptionID); err != nil {
		return fmt.Errorf("購読の削除に失敗しました: %w", err)
	}

	return nil
}

// ResumeFetch は停止中フィードのフェッチを再開する。
func (s *Service) ResumeFetch(ctx context.Context, userID, subscriptionID string) (*SubscriptionInfo, error) {
	sub, err := s.subRepo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("購読の取得に失敗しました: %w", err)
	}
	if sub == nil {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}
	if sub.UserID != userID {
		return nil, model.NewSubscriptionNotFoundError(subscriptionID)
	}

	// フィード状態を取得
	feed, err := s.feedRepo.FindByID(ctx, sub.FeedID)
	if err != nil {
		return nil, fmt.Errorf("フィードの取得に失敗しました: %w", err)
	}
	if feed == nil {
		return nil, fmt.Errorf("フィードが見つかりません: %s", sub.FeedID)
	}

	if feed.FetchStatus != model.FetchStatusStopped {
		return nil, model.NewFeedNotStoppedError()
	}

	// フェッチ状態をactiveに戻す
	feed.FetchStatus = model.FetchStatusActive
	feed.ErrorMessage = ""
	feed.ConsecutiveErrors = 0
	feed.NextFetchAt = time.Now()

	if err := s.feedRepo.UpdateFetchState(ctx, feed); err != nil {
		return nil, fmt.Errorf("フィード状態の更新に失敗しました: %w", err)
	}

	// 更新後の購読情報を返す
	infos, err := s.subRepo.ListByUserIDWithFeedInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("購読情報の再取得に失敗しました: %w", err)
	}

	for _, info := range infos {
		if info.ID == subscriptionID {
			result := &SubscriptionInfo{
				ID:                   info.ID,
				UserID:               info.UserID,
				FeedID:               info.FeedID,
				FeedTitle:            info.FeedTitle,
				FeedURL:              info.FeedURL,
				FetchIntervalMinutes: info.FetchIntervalMinutes,
				FeedStatus:           string(info.FetchStatus),
				UnreadCount:          info.UnreadCount,
				CreatedAt:            info.CreatedAt,
			}
			return result, nil
		}
	}

	return nil, model.NewSubscriptionNotFoundError(subscriptionID)
}
