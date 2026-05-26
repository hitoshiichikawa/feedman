// Package feed はフィード登録・管理のドメインロジックを提供する。
package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// maxSubscriptionsPerUser はユーザーあたりの購読上限。
const maxSubscriptionsPerUser = 100

// defaultFetchIntervalMinutes は新規購読のデフォルトフェッチ間隔（分）。
const defaultFetchIntervalMinutes = 60

// backgroundFaviconTimeout はバックグラウンドでの favicon 取得処理に課す上限時間。
// リクエストスコープから切り離した独立 context にこのタイムアウトを付与することで、
// goroutine が無制限に滞留するのを防ぐ（要件 4: バックグラウンド処理の有界性）。
const backgroundFaviconTimeout = 30 * time.Second

// Detector はフィード検出のインターフェース。
// テスタビリティのためFeedDetectorを抽象化する。
type Detector interface {
	DetectFeedURL(ctx context.Context, inputURL string) (string, error)
}

// FeedService はフィード登録・管理のサービス層。
// 検出 → フィード保存 → 購読作成 → favicon取得のフローを統括する。
// favicon 取得は購読作成完了後に独立した goroutine で非同期実行され、
// 登録レスポンスの応答時間に影響しない（要件 1: タイムアウト安全性）。
type FeedService struct {
	feedRepo       repository.FeedRepository
	subRepo        repository.SubscriptionRepository
	detector       Detector
	faviconFetcher FaviconFetcherService

	// faviconWG はバックグラウンドの favicon 取得 goroutine の完了を追跡する。
	// テストから非同期完了を待つために用いる（本番では Wait を呼ばないため挙動に影響しない）。
	faviconWG sync.WaitGroup
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

	// 5. favicon取得（非同期）。
	// リクエストスコープの ctx から切り離した独立 context で実行し、
	// 取得完了を待たずに登録レスポンスを返す（要件 1）。
	// ctx のキャンセル/タイムアウトに引きずられないこと（要件 3.3）、
	// かつ上限時間を付与して goroutine リークを防ぐこと（要件 4）。
	s.startFaviconFetch(ctx, feed.ID, faviconTargetURL(feed))

	return feed, sub, nil
}

// startFaviconFetch はリクエストスコープから切り離した独立 context で
// favicon 取得を非同期実行する goroutine を起動する。
// 独立 context には backgroundFaviconTimeout の上限時間を付与し、
// goroutine の完了を faviconWG で追跡する。
func (s *FeedService) startFaviconFetch(ctx context.Context, feedID, siteURL string) {
	if s.faviconFetcher == nil {
		return
	}

	// リクエスト ctx の値（トレース情報等）は引き継ぎつつ、
	// キャンセル/デッドラインの伝播を断ち切る独立 context を生成する。
	bgCtx := context.WithoutCancel(ctx)

	s.faviconWG.Add(1)
	go func() {
		defer s.faviconWG.Done()

		// 上限時間を付与し、処理完了または打ち切り時に必ずリソースを解放する（要件 4.3）。
		timeoutCtx, cancel := context.WithTimeout(bgCtx, backgroundFaviconTimeout)
		defer cancel()

		s.fetchAndSaveFavicon(timeoutCtx, feedID, siteURL)
	}()
}

// waitFaviconFetch は進行中のバックグラウンド favicon 取得 goroutine の完了を待つ。
// 本番フローでは呼ばれず、非同期完了を決定論的に検証したいテストからのみ利用する
// （テスト容易性のための補助であり、本番挙動には影響しない）。
func (s *FeedService) waitFaviconFetch() {
	s.faviconWG.Wait()
}

// faviconTargetURL は favicon 取得の対象となるサイト URL を決定する。
// SiteURL が空の場合は FeedURL をフォールバックとして用いる。
func faviconTargetURL(feed *model.Feed) string {
	if feed.SiteURL != "" {
		return feed.SiteURL
	}
	return feed.FeedURL
}

// GetFeed はフィード情報を取得する。
// 認可: リクエストユーザーが当該フィードを購読している場合のみ取得可能。
// 購読していない場合は IDOR を避けるため nil, nil を返す（ハンドラーで 404 として扱う）。
func (s *FeedService) GetFeed(ctx context.Context, userID, feedID string) (*model.Feed, error) {
	sub, err := s.subRepo.FindByUserAndFeed(ctx, userID, feedID)
	if err != nil {
		return nil, fmt.Errorf("購読の確認に失敗しました: %w", err)
	}
	if sub == nil {
		return nil, nil
	}
	return s.feedRepo.FindByID(ctx, feedID)
}

// UpdateFeedURL はフィードURLを更新する。
// 認可: リクエストユーザーが当該フィードを購読している場合のみ更新可能。
// 購読していない場合は IDOR を避けるため FEED_NOT_FOUND を返す。
func (s *FeedService) UpdateFeedURL(ctx context.Context, userID, feedID, newURL string) (*model.Feed, error) {
	sub, err := s.subRepo.FindByUserAndFeed(ctx, userID, feedID)
	if err != nil {
		return nil, fmt.Errorf("購読の確認に失敗しました: %w", err)
	}
	if sub == nil {
		return nil, &model.APIError{
			Code:     "FEED_NOT_FOUND",
			Message:  "指定されたフィードが見つかりません。",
			Category: "feed",
			Action:   "フィードIDを確認してください。",
		}
	}

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
// 取得失敗・未検出・タイムアウト時はログ出力のみで、エラーを返さず favicon を null のまま保持する。
// 返却済みの feed ポインタへの並行書き込みを避けるため、引数は feedID / siteURL のみとし、
// 取得結果はローカル変数経由で DB（UpdateFavicon）にのみ反映する。
func (s *FeedService) fetchAndSaveFavicon(ctx context.Context, feedID, siteURL string) {
	if s.faviconFetcher == nil {
		return
	}

	data, mimeType, err := s.faviconFetcher.FetchFaviconForSite(ctx, siteURL)
	if err != nil {
		slog.Warn("favicon取得エラー", "feedID", feedID, "siteURL", siteURL, "error", err)
		return
	}

	if data == nil {
		slog.Info("favicon未検出（nullとして保存）", "feedID", feedID, "siteURL", siteURL)
		return
	}

	// faviconをDB保存
	if err := s.feedRepo.UpdateFavicon(ctx, feedID, data, mimeType); err != nil {
		slog.Warn("favicon保存エラー", "feedID", feedID, "error", err)
		return
	}

	slog.Info("favicon保存完了", "feedID", feedID, "mimeType", mimeType, "size", len(data))
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
