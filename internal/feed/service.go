// Package feed はフィード登録・管理のドメインロジックを提供する。
package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// maxSubscriptionsPerUser はユーザーあたりの購読上限。
const maxSubscriptionsPerUser = 100

// defaultFetchIntervalMinutes は新規購読のデフォルトフェッチ間隔（分）。
const defaultFetchIntervalMinutes = 60

// Detector はフィード検出のインターフェース。
// テスタビリティのためFeedDetectorを抽象化する。
type Detector interface {
	DetectFeedURL(ctx context.Context, inputURL string) (string, error)
}

// FeedService はフィード登録・管理のサービス層。
// 検出 → フィード保存 → 購読作成 → favicon取得のフローを統括する。
type FeedService struct {
	feedRepo       repository.FeedRepository
	subRepo        repository.SubscriptionRepository
	detector       Detector
	faviconFetcher FaviconFetcherService
}

// NewFeedService はFeedServiceの新しいインスタンスを生成する。
func NewFeedService(
	feedRepo repository.FeedRepository,
	subRepo repository.SubscriptionRepository,
	detector Detector,
	faviconFetcher FaviconFetcherService,
) *FeedService {
	return &FeedService{
		feedRepo:       feedRepo,
		subRepo:        subRepo,
		detector:       detector,
		faviconFetcher: faviconFetcher,
	}
}

// RegisterFeed はURLからフィードを検出し登録する。
// フロー: 購読上限チェック → フィード検出 → フィード保存（重複チェック） → 購読作成 → favicon取得
func (s *FeedService) RegisterFeed(ctx context.Context, userID string, inputURL string) (*model.Feed, *model.Subscription, error) {
	// 1. 購読上限チェック
	count, err := s.subRepo.CountByUserID(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("購読数の確認に失敗しました: %w", err)
	}
	if count >= maxSubscriptionsPerUser {
		return nil, nil, model.NewSubscriptionLimitError()
	}

	// 2. フィードURL検出
	feedURL, err := s.detector.DetectFeedURL(ctx, inputURL)
	if err != nil {
		return nil, nil, err
	}

	// 3. 既存フィードの重複チェック（feed_urlで検索）
	existingFeed, err := s.feedRepo.FindByFeedURL(ctx, feedURL)
	if err != nil {
		return nil, nil, fmt.Errorf("フィードの検索に失敗しました: %w", err)
	}

	var feed *model.Feed

	if existingFeed != nil {
		// 既存フィードが見つかった場合
		feed = existingFeed

		// 同じユーザーが同じフィードを既に購読していないかチェック
		existingSub, err := s.subRepo.FindByUserAndFeed(ctx, userID, feed.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("購読の確認に失敗しました: %w", err)
		}
		if existingSub != nil {
			return nil, nil, model.NewDuplicateSubscriptionError()
		}
	} else {
		// 新規フィードの作成
		now := time.Now()
		feed = &model.Feed{
			ID:          uuid.New().String(),
			FeedURL:     feedURL,
			SiteURL:     extractSiteURL(inputURL),
			Title:       feedURL, // 初期タイトルはフィードURL（パース時に更新される）
			FetchStatus: model.FetchStatusActive,
			NextFetchAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := s.feedRepo.Create(ctx, feed); err != nil {
			return nil, nil, fmt.Errorf("フィードの保存に失敗しました: %w", err)
		}
	}

	// 4. 購読レコードの作成
	now := time.Now()
	sub := &model.Subscription{
		ID:                   uuid.New().String(),
		UserID:               userID,
		FeedID:               feed.ID,
		FetchIntervalMinutes: defaultFetchIntervalMinutes,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := s.subRepo.Create(ctx, sub); err != nil {
		return nil, nil, fmt.Errorf("購読の作成に失敗しました: %w", err)
	}

	// 5. favicon取得（非同期ではなく同期で実行。取得失敗時はnullとして保存）
	s.fetchAndSaveFavicon(ctx, feed)

	return feed, sub, nil
}

// GetFeed はフィード情報を取得する。
func (s *FeedService) GetFeed(ctx context.Context, feedID string) (*model.Feed, error) {
	return s.feedRepo.FindByID(ctx, feedID)
}

// UpdateFeedURL はフィードURLを更新する。
func (s *FeedService) UpdateFeedURL(ctx context.Context, feedID string, newURL string) (*model.Feed, error) {
	feed, err := s.feedRepo.FindByID(ctx, feedID)
	if err != nil {
		return nil, fmt.Errorf("フィードの取得に失敗しました: %w", err)
	}
	if feed == nil {
		return nil, &model.APIError{
			Code:     "FEED_NOT_FOUND",
			Message:  "指定されたフィードが見つかりません。",
			Category: "feed",
			Action:   "フィードIDを確認してください。",
		}
	}

	feed.FeedURL = newURL
	feed.UpdatedAt = time.Now()

	if err := s.feedRepo.Update(ctx, feed); err != nil {
		return nil, fmt.Errorf("フィードURLの更新に失敗しました: %w", err)
	}

	return feed, nil
}

// fetchAndSaveFavicon はフィードのfaviconを取得して保存する。
// 取得失敗時はログ出力のみで、エラーを返さない。
func (s *FeedService) fetchAndSaveFavicon(ctx context.Context, feed *model.Feed) {
	if s.faviconFetcher == nil {
		return
	}

	// サイトURLからfaviconを取得
	siteURL := feed.SiteURL
	if siteURL == "" {
		siteURL = feed.FeedURL
	}

	data, mimeType, err := s.faviconFetcher.FetchFaviconForSite(ctx, siteURL)
	if err != nil {
		slog.Warn("favicon取得エラー", "feedID", feed.ID, "siteURL", siteURL, "error", err)
		return
	}

	if data == nil {
		slog.Info("favicon未検出（nullとして保存）", "feedID", feed.ID, "siteURL", siteURL)
		return
	}

	// faviconをDB保存
	if err := s.feedRepo.UpdateFavicon(ctx, feed.ID, data, mimeType); err != nil {
		slog.Warn("favicon保存エラー", "feedID", feed.ID, "error", err)
		return
	}

	feed.FaviconData = data
	feed.FaviconMime = mimeType
	slog.Info("favicon保存完了", "feedID", feed.ID, "mimeType", mimeType, "size", len(data))
}

// extractSiteURL はフィードURLまたは入力URLからサイトURLを抽出する。
func extractSiteURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
