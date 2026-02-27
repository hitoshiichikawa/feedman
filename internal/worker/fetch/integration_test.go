package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// TestIntegration_WorkerFetchFlow はワーカーフェッチフロー全体を検証する。
// スケジューラ → フェッチ対象取得 → HTTP GET → gofeedパース → UPSERT → next_fetch_at更新
func TestIntegration_WorkerFetchFlow(t *testing.T) {
	// テスト用RSSフィードサーバーを起動
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Integration Test Feed</title>
    <link>https://example.com</link>
    <description>A test feed</description>
    <item>
      <title>Article 1</title>
      <link>https://example.com/article/1</link>
      <guid>guid-1</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>
      <description>First article content</description>
    </item>
    <item>
      <title>Article 2</title>
      <link>https://example.com/article/2</link>
      <guid>guid-2</guid>
      <pubDate>Tue, 02 Jan 2024 00:00:00 +0000</pubDate>
      <description>Second article content</description>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", `"test-etag-123"`)
		fmt.Fprint(w, rssContent)
	}))
	defer server.Close()

	// テスト用フィードデータ
	testFeed := &model.Feed{
		ID:          "feed-integration-1",
		FeedURL:     server.URL + "/feed.xml",
		SiteURL:     "https://example.com",
		Title:       "Old Title",
		FetchStatus: model.FetchStatusActive,
		NextFetchAt: time.Now().Add(-1 * time.Minute),
	}

	var fetchStateUpdated bool
	var updatedFeed *model.Feed
	var mu sync.Mutex

	feedRepo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			mu.Lock()
			defer mu.Unlock()
			if fetchStateUpdated {
				return nil, nil
			}
			return []*model.Feed{testFeed}, nil
		},
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			mu.Lock()
			defer mu.Unlock()
			fetchStateUpdated = true
			updatedFeed = &model.Feed{}
			*updatedFeed = *feed
			return nil
		},
	}

	subRepo := &mockSubRepo{
		minInterval: 60,
	}

	upsertSvc := &mockUpsertService{
		insertCount: 2,
	}

	ssrfGuard := &mockSSRFGuard{}

	// フェッチャーとスケジューラの初期化
	fetcher := NewFetcher(
		feedRepo, subRepo, upsertSvc, ssrfGuard,
		slog.Default(), 10*time.Second, 5*1024*1024,
	)

	scheduler := NewScheduler(feedRepo, fetcher, slog.Default(), 2)

	// RunOnceで1サイクル実行
	err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// 検証: フェッチ状態が更新されたこと
	if !fetchStateUpdated {
		t.Fatal("expected fetch state to be updated")
	}

	// 検証: フィードのタイトルが更新されたこと
	if updatedFeed.Title != "Integration Test Feed" {
		t.Errorf("feed title = %q, want %q", updatedFeed.Title, "Integration Test Feed")
	}

	// 検証: ETagが保存されたこと
	if updatedFeed.ETag != `"test-etag-123"` {
		t.Errorf("feed etag = %q, want %q", updatedFeed.ETag, `"test-etag-123"`)
	}

	// 検証: next_fetch_atが未来に設定されたこと
	if !updatedFeed.NextFetchAt.After(time.Now()) {
		t.Error("expected next_fetch_at to be in the future")
	}

	// 検証: フェッチ状態がactiveのままであること
	if updatedFeed.FetchStatus != model.FetchStatusActive {
		t.Errorf("fetch_status = %s, want active", updatedFeed.FetchStatus)
	}

	// 検証: 記事がUPSERTされたこと（upsertSvcにitemsが渡されたこと）
	if len(upsertSvc.calledWith) != 2 {
		t.Errorf("upserted items = %d, want 2", len(upsertSvc.calledWith))
	}

	// 検証: consecutive_errorsがリセットされたこと
	if updatedFeed.ConsecutiveErrors != 0 {
		t.Errorf("consecutive_errors = %d, want 0", updatedFeed.ConsecutiveErrors)
	}
}

// TestIntegration_WorkerFetchFlow_404StopsFeed は404レスポンス時にフェッチが停止されることを検証する。
func TestIntegration_WorkerFetchFlow_404StopsFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	testFeed := &model.Feed{
		ID:          "feed-404",
		FeedURL:     server.URL + "/gone.xml",
		FetchStatus: model.FetchStatusActive,
		NextFetchAt: time.Now().Add(-1 * time.Minute),
	}

	var updatedFeed *model.Feed

	feedRepo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return []*model.Feed{testFeed}, nil
		},
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			updatedFeed = &model.Feed{}
			*updatedFeed = *feed
			return nil
		},
	}

	subRepo := &mockSubRepo{minInterval: 60}
	ssrfGuard := &mockSSRFGuard{}

	fetcher := NewFetcher(
		feedRepo, subRepo, nil, ssrfGuard,
		slog.Default(), 10*time.Second, 5*1024*1024,
	)

	scheduler := NewScheduler(feedRepo, fetcher, slog.Default(), 2)
	err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	if updatedFeed == nil {
		t.Fatal("expected fetch state to be updated")
	}

	// 404の場合、フェッチが停止されること
	if updatedFeed.FetchStatus != model.FetchStatusStopped {
		t.Errorf("fetch_status = %s, want stopped", updatedFeed.FetchStatus)
	}

	if updatedFeed.ErrorMessage == "" {
		t.Error("expected non-empty error_message for stopped feed")
	}
}

// TestIntegration_WorkerFetchFlow_ConditionalGET_304 は304応答時のフロー（記事UPSERTなし）を検証する。
func TestIntegration_WorkerFetchFlow_ConditionalGET_304(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"existing-etag"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<rss></rss>")
	}))
	defer server.Close()

	testFeed := &model.Feed{
		ID:          "feed-304",
		FeedURL:     server.URL + "/feed.xml",
		FetchStatus: model.FetchStatusActive,
		ETag:        `"existing-etag"`,
		NextFetchAt: time.Now().Add(-1 * time.Minute),
	}

	var updatedFeed *model.Feed

	feedRepo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return []*model.Feed{testFeed}, nil
		},
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			updatedFeed = &model.Feed{}
			*updatedFeed = *feed
			return nil
		},
	}

	subRepo := &mockSubRepo{minInterval: 30}
	upsertSvc := &mockUpsertService{}
	ssrfGuard := &mockSSRFGuard{}

	fetcher := NewFetcher(
		feedRepo, subRepo, upsertSvc, ssrfGuard,
		slog.Default(), 10*time.Second, 5*1024*1024,
	)

	scheduler := NewScheduler(feedRepo, fetcher, slog.Default(), 2)
	err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	// 304の場合、UPSERTが呼ばれないこと
	if upsertSvc.calledWith != nil {
		t.Error("expected UpsertItems NOT to be called for 304 response")
	}

	if updatedFeed == nil {
		t.Fatal("expected fetch state to be updated")
	}

	// フィードのステータスはactiveのままであること
	if updatedFeed.FetchStatus != model.FetchStatusActive {
		t.Errorf("fetch_status = %s, want active", updatedFeed.FetchStatus)
	}

	// next_fetch_atが未来に更新されたこと
	if !updatedFeed.NextFetchAt.After(time.Now()) {
		t.Error("expected next_fetch_at to be in the future")
	}
}
