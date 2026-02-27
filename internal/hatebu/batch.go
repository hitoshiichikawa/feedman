package hatebu

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hitoshi/feedman/internal/repository"
)

// BookmarkCounter ははてなブックマーク数取得のインターフェース。
// テスト時にモックに差し替え可能。
type BookmarkCounter interface {
	GetBookmarkCounts(ctx context.Context, urls []string) (map[string]int, error)
}

// BatchConfig はバッチジョブの設定パラメータ。
// 環境変数から設定可能。
type BatchConfig struct {
	// BatchInterval はバッチジョブの実行間隔（デフォルト: 10分）。
	BatchInterval time.Duration
	// APIInterval はAPI呼び出しの最低間隔（デフォルト: 5秒）。
	APIInterval time.Duration
	// MaxCallsPerCycle は1サイクルあたりの最大API呼び出し回数（デフォルト: 100）。
	MaxCallsPerCycle int
	// HatebuTTL はブックマーク数の再取得間隔（デフォルト: 24時間）。
	HatebuTTL time.Duration
}

// DefaultBatchConfig はデフォルトのバッチジョブ設定を返す。
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		BatchInterval:    10 * time.Minute,
		APIInterval:      5 * time.Second,
		MaxCallsPerCycle: 100,
		HatebuTTL:        24 * time.Hour,
	}
}

// BatchJob ははてなブックマーク数のバッチ取得ジョブ。
// 定期的にhatebu_fetched_atがNULLまたは24時間経過した記事を対象に
// はてなブックマークAPIを呼び出してブックマーク数を更新する。
type BatchJob struct {
	itemRepo         repository.HatebuItemRepository
	client           BookmarkCounter
	logger           *slog.Logger
	config           BatchConfig
	consecutiveErrors int
	backoffUntil     time.Time
}

// NewBatchJob はBatchJobの新しいインスタンスを生成する。
func NewBatchJob(
	itemRepo repository.HatebuItemRepository,
	client BookmarkCounter,
	logger *slog.Logger,
	config BatchConfig,
) *BatchJob {
	return &BatchJob{
		itemRepo: itemRepo,
		client:   client,
		logger:   logger,
		config:   config,
	}
}

// Start はバッチジョブをティッカーで定期実行する。
// コンテキストがキャンセルされるまで実行を継続する。
func (b *BatchJob) Start(ctx context.Context) {
	ticker := time.NewTicker(b.config.BatchInterval)
	defer ticker.Stop()

	b.logger.Info("はてなブックマークバッチジョブを開始しました",
		slog.Duration("batch_interval", b.config.BatchInterval),
		slog.Duration("api_interval", b.config.APIInterval),
		slog.Int("max_calls_per_cycle", b.config.MaxCallsPerCycle),
	)

	// 起動直後に1回実行
	if err := b.RunOnce(ctx); err != nil {
		b.logger.Error("はてなブックマークバッチサイクルの実行に失敗しました",
			slog.String("error", err.Error()),
		)
	}

	for {
		select {
		case <-ctx.Done():
			b.logger.Info("はてなブックマークバッチジョブを停止しました")
			return
		case <-ticker.C:
			if err := b.RunOnce(ctx); err != nil {
				b.logger.Error("はてなブックマークバッチサイクルの実行に失敗しました",
					slog.String("error", err.Error()),
				)
			}
		}
	}
}

// RunOnce は1回のバッチサイクルを実行する。
// 取得対象の記事を取得し、50URL単位でAPIを呼び出してブックマーク数を更新する。
func (b *BatchJob) RunOnce(ctx context.Context) error {
	start := time.Now()

	// バックオフ中の場合はスキップ
	if !b.backoffUntil.IsZero() && time.Now().Before(b.backoffUntil) {
		b.logger.Info("はてなブックマークバッチジョブはバックオフ中のためスキップします",
			slog.Time("backoff_until", b.backoffUntil),
		)
		return nil
	}

	// 取得対象記事の上限 = MaxCallsPerCycle * maxURLsPerRequest
	fetchLimit := b.config.MaxCallsPerCycle * maxURLsPerRequest

	items, err := b.itemRepo.ListNeedingHatebuFetch(ctx, fetchLimit)
	if err != nil {
		return fmt.Errorf("はてブ取得対象記事の取得に失敗しました: %w", err)
	}

	if len(items) == 0 {
		b.logger.Info("はてなブックマーク取得対象の記事はありません")
		return nil
	}

	// リンクのある記事のみ抽出し、URLからitemIDへのマッピングを構築
	type itemInfo struct {
		id  string
		url string
	}
	var validItems []itemInfo
	for _, item := range items {
		if item.Link != "" {
			validItems = append(validItems, itemInfo{id: item.ID, url: item.Link})
		}
	}

	if len(validItems) == 0 {
		b.logger.Info("リンクを持つ取得対象記事がありません")
		return nil
	}

	b.logger.Info("はてなブックマークバッチサイクルを開始します",
		slog.Int("target_items", len(validItems)),
	)

	// URL → Item ID のマッピング（同じURLの記事が複数ある場合に対応）
	urlToItemIDs := make(map[string][]string)
	for _, vi := range validItems {
		urlToItemIDs[vi.url] = append(urlToItemIDs[vi.url], vi.id)
	}

	// ユニークなURLリストを構築
	var uniqueURLs []string
	seen := make(map[string]bool)
	for _, vi := range validItems {
		if !seen[vi.url] {
			seen[vi.url] = true
			uniqueURLs = append(uniqueURLs, vi.url)
		}
	}

	// 50URL単位でチャンクに分割してAPI呼び出し
	var apiCallCount int
	var updatedCount int
	var hadError bool

	for i := 0; i < len(uniqueURLs); i += maxURLsPerRequest {
		// コンテキストチェック
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// MaxCallsPerCycle チェック
		if apiCallCount >= b.config.MaxCallsPerCycle {
			b.logger.Info("1サイクルあたりの最大API呼び出し回数に達しました",
				slog.Int("api_call_count", apiCallCount),
				slog.Int("max_calls_per_cycle", b.config.MaxCallsPerCycle),
			)
			break
		}

		// API呼び出しインターバル（初回は待たない）
		if apiCallCount > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(b.config.APIInterval):
			}
		}

		// チャンクの範囲を決定
		end := i + maxURLsPerRequest
		if end > len(uniqueURLs) {
			end = len(uniqueURLs)
		}
		chunk := uniqueURLs[i:end]

		apiCallCount++

		// API呼び出し
		counts, err := b.client.GetBookmarkCounts(ctx, chunk)
		if err != nil {
			b.logger.Error("はてなブックマークAPIの呼び出しに失敗しました",
				slog.String("error", err.Error()),
				slog.Int("chunk_size", len(chunk)),
				slog.Int("api_call_count", apiCallCount),
			)
			hadError = true
			b.consecutiveErrors++
			// バックオフ判定
			backoff := b.calculateErrorBackoff(b.consecutiveErrors)
			if backoff > 0 {
				b.backoffUntil = time.Now().Add(backoff)
				b.logger.Warn("連続エラーによりバックオフを適用します",
					slog.Int("consecutive_errors", b.consecutiveErrors),
					slog.Duration("backoff_duration", backoff),
				)
				break
			}
			continue // このチャンクはスキップし次のチャンクへ（前回値維持）
		}

		// 取得成功: 各記事のブックマーク数を更新
		now := time.Now()
		for url, count := range counts {
			itemIDs, ok := urlToItemIDs[url]
			if !ok {
				continue
			}
			for _, itemID := range itemIDs {
				if err := b.itemRepo.UpdateHatebuCount(ctx, itemID, count, now); err != nil {
					b.logger.Error("はてなブックマーク数の更新に失敗しました",
						slog.String("item_id", itemID),
						slog.String("url", url),
						slog.Int("count", count),
						slog.String("error", err.Error()),
					)
				} else {
					updatedCount++
				}
			}
		}

		// レスポンスに含まれないURL（0件）も更新する
		for _, url := range chunk {
			if _, ok := counts[url]; !ok {
				itemIDs, exists := urlToItemIDs[url]
				if !exists {
					continue
				}
				for _, itemID := range itemIDs {
					if err := b.itemRepo.UpdateHatebuCount(ctx, itemID, 0, now); err != nil {
						b.logger.Error("はてなブックマーク数の更新に失敗しました",
							slog.String("item_id", itemID),
							slog.String("url", url),
							slog.Int("count", 0),
							slog.String("error", err.Error()),
						)
					} else {
						updatedCount++
					}
				}
			}
		}
	}

	// エラーがなければ連続エラーカウントをリセット
	if !hadError {
		b.consecutiveErrors = 0
		b.backoffUntil = time.Time{}
	}

	duration := time.Since(start)
	b.logger.Info("はてなブックマークバッチサイクルが完了しました",
		slog.Int("api_call_count", apiCallCount),
		slog.Int("updated_items", updatedCount),
		slog.Int("target_items", len(validItems)),
		slog.Float64("duration_ms", float64(duration.Milliseconds())),
	)

	return nil
}

// calculateErrorBackoff は連続エラー回数に基づくバックオフ時間を計算する。
// 3回連続: 30分、5回連続: 1時間、10回連続: 6時間。
func (b *BatchJob) calculateErrorBackoff(consecutiveErrors int) time.Duration {
	switch {
	case consecutiveErrors >= 10:
		return 6 * time.Hour
	case consecutiveErrors >= 5:
		return 1 * time.Hour
	case consecutiveErrors >= 3:
		return 30 * time.Minute
	default:
		return 0
	}
}

