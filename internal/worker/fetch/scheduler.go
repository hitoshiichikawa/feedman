// Package fetch はフィードのバックグラウンドフェッチ処理を提供する。
// スケジューラ、フェッチャー、リトライ/バックオフ戦略を含む。
package fetch

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// FeedFetcherService はフィードフェッチの実行インターフェース。
type FeedFetcherService interface {
	// Fetch は指定フィードをフェッチし、結果に応じてフィード状態を更新する。
	Fetch(ctx context.Context, feed *model.Feed) error
}

// Scheduler はフィードフェッチのスケジューリングと並列制御を行う。
// 5分間隔のティッカーでフェッチ対象フィードを取得し、
// semaphoreパターンで最大並列数を制御しながらフェッチを実行する。
type Scheduler struct {
	feedRepo       repository.FeedRepository
	fetcher        FeedFetcherService
	logger         *slog.Logger
	maxConcurrency int
}

// NewScheduler はSchedulerの新しいインスタンスを生成する。
// maxConcurrencyが0以下の場合はデフォルト値10を使用する。
func NewScheduler(
	feedRepo repository.FeedRepository,
	fetcher FeedFetcherService,
	logger *slog.Logger,
	maxConcurrency int,
) *Scheduler {
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}
	return &Scheduler{
		feedRepo:       feedRepo,
		fetcher:        fetcher,
		logger:         logger,
		maxConcurrency: maxConcurrency,
	}
}

// Start は5分間隔のティッカーでスケジューラを起動する。
// コンテキストがキャンセルされるまで実行を継続する。
func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("フェッチスケジューラを開始しました",
		slog.Duration("interval", interval),
		slog.Int("max_concurrency", s.maxConcurrency),
	)

	// 起動直後に1回実行
	if err := s.RunOnce(ctx); err != nil {
		s.logger.Error("フェッチサイクルの実行に失敗しました",
			slog.String("error", err.Error()),
		)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("フェッチスケジューラを停止しました")
			return
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				s.logger.Error("フェッチサイクルの実行に失敗しました",
					slog.String("error", err.Error()),
				)
			}
		}
	}
}

// RunOnce はフェッチ対象フィードを1回取得し、並列でフェッチを実行する。
// semaphoreパターンで最大並列数を制御する。
func (s *Scheduler) RunOnce(ctx context.Context) error {
	start := time.Now()

	// フェッチ対象フィードを取得（FOR UPDATE SKIP LOCKED）
	feeds, err := s.feedRepo.ListDueForFetch(ctx)
	if err != nil {
		return err
	}

	if len(feeds) == 0 {
		s.logger.Info("フェッチ対象のフィードはありません")
		return nil
	}

	s.logger.Info("フェッチサイクルを開始します",
		slog.Int("feed_count", len(feeds)),
	)

	// semaphoreパターンで並列数を制御
	sem := make(chan struct{}, s.maxConcurrency)
	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		sem <- struct{}{} // semaphore取得（ブロック）

		go func(f *model.Feed) {
			defer wg.Done()
			defer func() { <-sem }() // semaphore解放

			if err := s.fetcher.Fetch(ctx, f); err != nil {
				s.logger.Error("フィードフェッチに失敗しました",
					slog.String("feed_id", f.ID),
					slog.String("feed_url", f.FeedURL),
					slog.String("error", err.Error()),
				)
			}
		}(feed)
	}

	wg.Wait()

	duration := time.Since(start)
	s.logger.Info("フェッチサイクルが完了しました",
		slog.Int("feed_count", len(feeds)),
		slog.Float64("duration_ms", float64(duration.Milliseconds())),
	)

	return nil
}
