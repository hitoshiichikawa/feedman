package fetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// ItemUpserter は記事のUPSERT処理のインターフェース。
type ItemUpserter interface {
	UpsertItems(ctx context.Context, feedID string, items []model.ParsedItem) (int, int, error)
}

// SSRFValidator はSSRF検証のインターフェース。
type SSRFValidator interface {
	ValidateURL(rawURL string) error
	NewSafeClient(timeout time.Duration, maxResponseSize int64) *http.Client
}

// Fetcher は個別フィードのHTTPフェッチとパースを行う。
// ETag/Last-Modifiedを使用した条件付きGET、SSRF検証、
// gofeedによるパース、ItemUpsertServiceによる記事保存を実行する。
type Fetcher struct {
	feedRepo    repository.FeedRepository
	subRepo     repository.SubscriptionRepository
	upsertSvc   ItemUpserter
	ssrfGuard   SSRFValidator
	logger      *slog.Logger
	timeout     time.Duration
	maxBodySize int64
}

// NewFetcher はFetcherの新しいインスタンスを生成する。
func NewFetcher(
	feedRepo repository.FeedRepository,
	subRepo repository.SubscriptionRepository,
	upsertSvc ItemUpserter,
	ssrfGuard SSRFValidator,
	logger *slog.Logger,
	timeout time.Duration,
	maxBodySize int64,
) *Fetcher {
	return &Fetcher{
		feedRepo:    feedRepo,
		subRepo:     subRepo,
		upsertSvc:   upsertSvc,
		ssrfGuard:   ssrfGuard,
		logger:      logger,
		timeout:     timeout,
		maxBodySize: maxBodySize,
	}
}

// Fetch はフィードをフェッチし、結果に応じてフィード状態を更新する。
// FeedFetcherServiceインターフェースを実装する。
func (f *Fetcher) Fetch(ctx context.Context, feed *model.Feed) error {
	start := time.Now()

	// SSRF検証
	if err := f.ssrfGuard.ValidateURL(feed.FeedURL); err != nil {
		f.logger.Error("SSRF検証に失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("feed_url", feed.FeedURL),
			slog.String("error", err.Error()),
		)
		ApplyStopFeed(feed, fmt.Sprintf("SSRF検証失敗: %s", err.Error()))
		if updateErr := f.feedRepo.UpdateFetchState(ctx, feed); updateErr != nil {
			f.logger.Error("フィード状態の更新に失敗しました",
				slog.String("feed_id", feed.ID),
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("SSRF検証に失敗: %w", err)
	}

	// HTTPリクエスト構築
	client := f.ssrfGuard.NewSafeClient(f.timeout, f.maxBodySize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.FeedURL, nil)
	if err != nil {
		return fmt.Errorf("リクエスト作成に失敗: %w", err)
	}

	req.Header.Set("User-Agent", "Feedman/1.0 RSS Reader")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")

	// 条件付きGET: ETag
	if feed.ETag != "" {
		req.Header.Set("If-None-Match", feed.ETag)
	}
	// 条件付きGET: Last-Modified
	if feed.LastModified != "" {
		req.Header.Set("If-Modified-Since", feed.LastModified)
	}

	// HTTPリクエスト実行
	resp, err := client.Do(req)
	if err != nil {
		f.logger.Error("HTTPリクエストに失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("feed_url", feed.FeedURL),
			slog.String("error", err.Error()),
		)
		ApplyBackoff(feed, fmt.Sprintf("HTTPリクエスト失敗: %s", err.Error()))
		if updateErr := f.feedRepo.UpdateFetchState(ctx, feed); updateErr != nil {
			f.logger.Error("フィード状態の更新に失敗しました",
				slog.String("feed_id", feed.ID),
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("HTTPリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// HTTPステータスに基づく処理分岐
	result := ClassifyHTTPStatus(resp.StatusCode)

	switch result {
	case FetchResultNotModified:
		// 304: コンテンツ未変更 - next_fetch_atのみ更新
		f.logger.Info("フィードは未変更です（304）",
			slog.String("feed_id", feed.ID),
			slog.String("feed_url", feed.FeedURL),
			slog.Int("http_status", resp.StatusCode),
			slog.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		interval, err := f.getMinFetchInterval(ctx, feed.ID)
		if err != nil {
			f.logger.Error("最小フェッチ間隔の取得に失敗しました",
				slog.String("feed_id", feed.ID),
				slog.String("error", err.Error()),
			)
			interval = 60 // デフォルト60分
		}
		ApplySuccess(feed, interval)
		return f.feedRepo.UpdateFetchState(ctx, feed)

	case FetchResultStop:
		// 404/410/401/403: フェッチ停止
		reason := fmt.Sprintf("HTTPステータス %d によりフェッチを停止しました", resp.StatusCode)
		f.logger.Warn("フィードフェッチを停止します",
			slog.String("feed_id", feed.ID),
			slog.String("feed_url", feed.FeedURL),
			slog.Int("http_status", resp.StatusCode),
			slog.String("reason", reason),
		)
		ApplyStopFeed(feed, reason)
		return f.feedRepo.UpdateFetchState(ctx, feed)

	case FetchResultBackoff:
		// 429/5xx: バックオフ
		reason := fmt.Sprintf("HTTPステータス %d によりバックオフを適用しました", resp.StatusCode)
		f.logger.Warn("フィードフェッチにバックオフを適用します",
			slog.String("feed_id", feed.ID),
			slog.String("feed_url", feed.FeedURL),
			slog.Int("http_status", resp.StatusCode),
			slog.Int("consecutive_errors", feed.ConsecutiveErrors+1),
		)
		ApplyBackoff(feed, reason)
		return f.feedRepo.UpdateFetchState(ctx, feed)

	case FetchResultOK:
		// 200: 正常フェッチ - 以下で処理を続行
	default:
		// その他のステータスコード
		f.logger.Warn("予期しないHTTPステータスコード",
			slog.String("feed_id", feed.ID),
			slog.Int("http_status", resp.StatusCode),
		)
		ApplyBackoff(feed, fmt.Sprintf("予期しないHTTPステータス: %d", resp.StatusCode))
		return f.feedRepo.UpdateFetchState(ctx, feed)
	}

	// レスポンスボディを読み込み（最大サイズ制限付き）
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodySize))
	if err != nil {
		f.logger.Error("レスポンスボディの読み取りに失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("error", err.Error()),
		)
		ApplyBackoff(feed, fmt.Sprintf("レスポンス読み取り失敗: %s", err.Error()))
		return f.feedRepo.UpdateFetchState(ctx, feed)
	}

	// ETag/Last-Modifiedを保存
	if etag := resp.Header.Get("ETag"); etag != "" {
		feed.ETag = etag
	}
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		feed.LastModified = lastMod
	}

	// gofeedでフィードをパース
	parser := gofeed.NewParser()
	parsedFeed, err := parser.ParseString(string(body))
	if err != nil {
		f.logger.Error("フィードのパースに失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("feed_url", feed.FeedURL),
			slog.String("error", err.Error()),
		)
		ApplyParseFailure(feed, err.Error())
		if updateErr := f.feedRepo.UpdateFetchState(ctx, feed); updateErr != nil {
			f.logger.Error("フィード状態の更新に失敗しました",
				slog.String("feed_id", feed.ID),
				slog.String("error", updateErr.Error()),
			)
		}
		return nil // パース失敗はフェッチエラーとしない（カウントして継続）
	}

	// フィードタイトルを更新
	if parsedFeed.Title != "" {
		feed.Title = parsedFeed.Title
	}
	if parsedFeed.Link != "" {
		feed.SiteURL = parsedFeed.Link
	}

	// gofeedの記事をParsedItemに変換
	parsedItems := convertGofeedItems(parsedFeed.Items)

	// ItemUpsertServiceで記事を保存
	inserted, updated, err := f.upsertSvc.UpsertItems(ctx, feed.ID, parsedItems)
	if err != nil {
		f.logger.Error("記事のUPSERTに失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("error", err.Error()),
		)
		ApplyParseFailure(feed, fmt.Sprintf("記事UPSERT失敗: %s", err.Error()))
		if updateErr := f.feedRepo.UpdateFetchState(ctx, feed); updateErr != nil {
			f.logger.Error("フィード状態の更新に失敗しました",
				slog.String("feed_id", feed.ID),
				slog.String("error", updateErr.Error()),
			)
		}
		return nil
	}

	// 最小フェッチ間隔を取得してnext_fetch_atを設定
	interval, err := f.getMinFetchInterval(ctx, feed.ID)
	if err != nil {
		f.logger.Error("最小フェッチ間隔の取得に失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("error", err.Error()),
		)
		interval = 60 // デフォルト60分
	}

	ApplySuccess(feed, interval)

	// フィード状態を更新
	if updateErr := f.feedRepo.UpdateFetchState(ctx, feed); updateErr != nil {
		f.logger.Error("フィード状態の更新に失敗しました",
			slog.String("feed_id", feed.ID),
			slog.String("error", updateErr.Error()),
		)
		return updateErr
	}

	f.logger.Info("フィードフェッチが完了しました",
		slog.String("feed_id", feed.ID),
		slog.String("feed_url", feed.FeedURL),
		slog.Int("http_status", resp.StatusCode),
		slog.Int("items_inserted", inserted),
		slog.Int("items_updated", updated),
		slog.Int("items_total", len(parsedItems)),
		slog.Float64("duration_ms", float64(duration.Milliseconds())),
	)

	return nil
}

// getMinFetchInterval はフィードの全購読者の中で最小のfetch_interval_minutesを取得する。
func (f *Fetcher) getMinFetchInterval(ctx context.Context, feedID string) (int, error) {
	interval, err := f.subRepo.MinFetchIntervalByFeedID(ctx, feedID)
	if err != nil {
		return 0, err
	}
	return interval, nil
}

// convertGofeedItems はgofeedの記事をmodel.ParsedItemに変換する。
func convertGofeedItems(items []*gofeed.Item) []model.ParsedItem {
	parsedItems := make([]model.ParsedItem, 0, len(items))

	for _, item := range items {
		if item == nil {
			continue
		}

		parsed := model.ParsedItem{
			Title:   item.Title,
			Link:    item.Link,
			Content: item.Content,
			Summary: item.Description,
		}

		// GUIDの設定: gofeedはGUIDをitem.GUIDに格納
		if item.GUID != "" {
			parsed.GuidOrID = item.GUID
		}

		// 著者情報
		if item.Author != nil {
			parsed.Author = item.Author.Name
		}
		// Authorsが空でAuthor文字列がある場合
		if parsed.Author == "" && len(item.Authors) > 0 && item.Authors[0] != nil {
			parsed.Author = item.Authors[0].Name
		}

		// 公開日時
		if item.PublishedParsed != nil {
			t := *item.PublishedParsed
			parsed.PublishedAt = &t
		} else if item.UpdatedParsed != nil {
			t := *item.UpdatedParsed
			parsed.PublishedAt = &t
		}

		// Contentが空の場合はDescriptionを使用
		if parsed.Content == "" && item.Description != "" {
			parsed.Content = item.Description
		}

		// LinkがなくGUIDがURL形式の場合はGUIDをLinkとして使用
		if parsed.Link == "" && parsed.GuidOrID != "" &&
			(strings.HasPrefix(parsed.GuidOrID, "http://") || strings.HasPrefix(parsed.GuidOrID, "https://")) {
			parsed.Link = parsed.GuidOrID
		}

		parsedItems = append(parsedItems, parsed)
	}

	return parsedItems
}
