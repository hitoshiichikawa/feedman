package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- Task 6.2: フィードフェッチャーのテスト ---

// mockSubRepo はSubscriptionRepositoryのテスト用モック。
type mockSubRepo struct {
	minInterval int
	minErr      error
}

func (m *mockSubRepo) FindByID(_ context.Context, _ string) (*model.Subscription, error) {
	return nil, nil
}

func (m *mockSubRepo) FindByUserAndFeed(_ context.Context, _, _ string) (*model.Subscription, error) {
	return nil, nil
}

func (m *mockSubRepo) CountByUserID(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockSubRepo) Create(_ context.Context, _ *model.Subscription) error {
	return nil
}

func (m *mockSubRepo) ListByUserID(_ context.Context, _ string) ([]*model.Subscription, error) {
	return nil, nil
}

func (m *mockSubRepo) MinFetchIntervalByFeedID(_ context.Context, _ string) (int, error) {
	return m.minInterval, m.minErr
}

func (m *mockSubRepo) UpdateFetchInterval(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockSubRepo) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockSubRepo) DeleteByUserID(_ context.Context, _ string) error {
	return nil
}

func (m *mockSubRepo) ListByUserIDWithFeedInfo(_ context.Context, _ string) ([]repository.SubscriptionWithFeedInfo, error) {
	return nil, nil
}

// mockUpsertService はItemUpsertServiceのテスト用モック。
type mockUpsertService struct {
	insertCount int
	updateCount int
	err         error
	calledWith  []model.ParsedItem
}

func (m *mockUpsertService) UpsertItems(_ context.Context, _ string, items []model.ParsedItem) (int, int, error) {
	m.calledWith = items
	return m.insertCount, m.updateCount, m.err
}

// mockSSRFGuard はSSRFGuardServiceのテスト用モック。
type mockSSRFGuard struct {
	validateErr error
}

func (m *mockSSRFGuard) NewSafeClient(timeout time.Duration, _ int64) *http.Client {
	return &http.Client{Timeout: timeout}
}

func (m *mockSSRFGuard) ValidateURL(_ string) error {
	return m.validateErr
}

// --- フェッチャーのテスト ---

func TestNewFetcher_ReturnsNonNil(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	f := NewFetcher(
		&mockFeedRepo{},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)
	if f == nil {
		t.Fatal("NewFetcher は nil を返してはならない")
	}
}

func TestFetcher_Fetch_Success200(t *testing.T) {
	// テストサーバー: 正常なRSSフィードを返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Wed, 01 Jan 2025 00:00:00 GMT")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>Article 1</title>
      <link>https://example.com/article1</link>
      <guid>guid-1</guid>
      <description>Summary 1</description>
    </item>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	updateCalled := false
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			updateCalled = true
			return nil
		},
	}

	upsertSvc := &mockUpsertService{insertCount: 1, updateCount: 0}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		upsertSvc,
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     server.URL,
		FetchStatus: model.FetchStatusActive,
	}

	err := f.Fetch(context.Background(), feed)
	if err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// ETag/Last-Modifiedが保存されること
	if feed.ETag != `"abc123"` {
		t.Errorf("ETag = %q, want %q", feed.ETag, `"abc123"`)
	}
	if feed.LastModified != "Wed, 01 Jan 2025 00:00:00 GMT" {
		t.Errorf("LastModified = %q, want %q", feed.LastModified, "Wed, 01 Jan 2025 00:00:00 GMT")
	}

	// UpsertItemsが呼ばれること
	if len(upsertSvc.calledWith) != 1 {
		t.Errorf("UpsertItems に渡された記事数 = %d, want 1", len(upsertSvc.calledWith))
	}

	// UpdateFetchStateが呼ばれること
	if !updateCalled {
		t.Error("UpdateFetchState が呼ばれるべき")
	}

	// ConsecutiveErrorsがリセットされること
	if feed.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", feed.ConsecutiveErrors)
	}
}

func TestFetcher_Fetch_304NotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ETagが一致する場合は304を返す
		if r.Header.Get("If-None-Match") == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	updateCalled := false
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			updateCalled = true
			return nil
		},
	}

	upsertSvc := &mockUpsertService{}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		upsertSvc,
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     server.URL,
		FetchStatus: model.FetchStatusActive,
		ETag:        `"abc123"`,
	}

	err := f.Fetch(context.Background(), feed)
	if err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// 304の場合、UpsertItemsは呼ばれない
	if upsertSvc.calledWith != nil {
		t.Error("304の場合、UpsertItemsは呼ばれないべき")
	}

	// UpdateFetchStateは呼ばれる（next_fetch_at更新のため）
	if !updateCalled {
		t.Error("304でもUpdateFetchStateが呼ばれるべき")
	}
}

func TestFetcher_Fetch_ConditionalGET_ETag(t *testing.T) {
	var receivedIfNoneMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedIfNoneMatch = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	f := NewFetcher(
		&mockFeedRepo{
			updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
				return nil
			},
		},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:      "feed-1",
		FeedURL: server.URL,
		ETag:    `"etag-value"`,
	}

	_ = f.Fetch(context.Background(), feed)

	if receivedIfNoneMatch != `"etag-value"` {
		t.Errorf("If-None-Match = %q, want %q", receivedIfNoneMatch, `"etag-value"`)
	}
}

func TestFetcher_Fetch_ConditionalGET_LastModified(t *testing.T) {
	var receivedIfModifiedSince string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedIfModifiedSince = r.Header.Get("If-Modified-Since")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	f := NewFetcher(
		&mockFeedRepo{
			updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
				return nil
			},
		},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:           "feed-1",
		FeedURL:      server.URL,
		LastModified: "Wed, 01 Jan 2025 00:00:00 GMT",
	}

	_ = f.Fetch(context.Background(), feed)

	if receivedIfModifiedSince != "Wed, 01 Jan 2025 00:00:00 GMT" {
		t.Errorf("If-Modified-Since = %q, want %q", receivedIfModifiedSince, "Wed, 01 Jan 2025 00:00:00 GMT")
	}
}

func TestFetcher_Fetch_SSRFValidation(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ssrfGuard := &mockSSRFGuard{
		validateErr: fmt.Errorf("blocked IP address"),
	}

	f := NewFetcher(
		&mockFeedRepo{
			updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
				return nil
			},
		},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		ssrfGuard,
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     "http://192.168.1.1/feed.xml",
		FetchStatus: model.FetchStatusActive,
	}

	err := f.Fetch(context.Background(), feed)
	if err == nil {
		t.Fatal("SSRF検証失敗時はエラーを返すべき")
	}

	// フィードが停止されること
	if feed.FetchStatus != model.FetchStatusStopped {
		t.Errorf("SSRF検証失敗時はfetch_statusがstoppedになるべき: %q", feed.FetchStatus)
	}
}

func TestFetcher_Fetch_404StopsFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     server.URL,
		FetchStatus: model.FetchStatusActive,
	}

	err := f.Fetch(context.Background(), feed)
	// フェッチ自体はエラーではなく、フィードの停止として処理
	if err != nil {
		t.Fatalf("404はフェッチエラーではなく停止処理: %v", err)
	}

	if feed.FetchStatus != model.FetchStatusStopped {
		t.Errorf("404時にfetch_status = %q, want %q", feed.FetchStatus, model.FetchStatusStopped)
	}
}

func TestFetcher_Fetch_429Backoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:                "feed-1",
		FeedURL:           server.URL,
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 0,
	}

	err := f.Fetch(context.Background(), feed)
	if err != nil {
		t.Fatalf("429はフェッチエラーではなくバックオフ処理: %v", err)
	}

	// バックオフが適用されること
	if feed.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", feed.ConsecutiveErrors)
	}
	if feed.FetchStatus != model.FetchStatusActive {
		t.Errorf("429時はアクティブのまま: fetch_status = %q", feed.FetchStatus)
	}
}

func TestFetcher_Fetch_500Backoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     server.URL,
		FetchStatus: model.FetchStatusActive,
	}

	_ = f.Fetch(context.Background(), feed)

	if feed.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", feed.ConsecutiveErrors)
	}
}

func TestFetcher_Fetch_NextFetchAtUsesMinInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel><title>Test</title></channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	// 最小フェッチ間隔は30分
	subRepo := &mockSubRepo{minInterval: 30}

	f := NewFetcher(
		feedRepo,
		subRepo,
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	now := time.Now()
	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     server.URL,
		FetchStatus: model.FetchStatusActive,
	}

	_ = f.Fetch(context.Background(), feed)

	// NextFetchAtが約30分後であること
	expectedTime := now.Add(30 * time.Minute)
	diff := feed.NextFetchAt.Sub(expectedTime)
	if diff > 5*time.Second || diff < -5*time.Second {
		t.Errorf("NextFetchAt が期待値から大幅にずれている: %v (期待: ~%v)", feed.NextFetchAt, expectedTime)
	}
}

func TestFetcher_Fetch_ParsesGofeedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>Article 1</title>
      <link>https://example.com/1</link>
      <guid>guid-1</guid>
      <description>Summary 1</description>
    </item>
    <item>
      <title>Article 2</title>
      <link>https://example.com/2</link>
      <guid>guid-2</guid>
      <description>Summary 2</description>
    </item>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	upsertSvc := &mockUpsertService{insertCount: 2}

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		upsertSvc,
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:          "feed-1",
		FeedURL:     server.URL,
		FetchStatus: model.FetchStatusActive,
	}

	_ = f.Fetch(context.Background(), feed)

	if len(upsertSvc.calledWith) != 2 {
		t.Fatalf("UpsertItemsに渡された記事数 = %d, want 2", len(upsertSvc.calledWith))
	}

	// 各記事のフィールドが正しくマッピングされること
	if upsertSvc.calledWith[0].GuidOrID != "guid-1" {
		t.Errorf("記事1のGuidOrID = %q, want %q", upsertSvc.calledWith[0].GuidOrID, "guid-1")
	}
	if upsertSvc.calledWith[1].Link != "https://example.com/2" {
		t.Errorf("記事2のLink = %q, want %q", upsertSvc.calledWith[1].Link, "https://example.com/2")
	}
}

func TestFetcher_Fetch_ParseFailureIncrements(t *testing.T) {
	// 不正なXMLを返すサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `not valid XML at all!!!`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:                "feed-1",
		FeedURL:           server.URL,
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 0,
	}

	err := f.Fetch(context.Background(), feed)
	if err != nil {
		t.Fatalf("パース失敗はフェッチエラーではなくエラーカウント更新: %v", err)
	}

	if feed.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", feed.ConsecutiveErrors)
	}
}

func TestFetcher_Fetch_ParseFailure10ConsecutiveStops(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `not valid XML!!!`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:                "feed-1",
		FeedURL:           server.URL,
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 9, // 9回目の失敗後
	}

	_ = f.Fetch(context.Background(), feed)

	if feed.FetchStatus != model.FetchStatusStopped {
		t.Errorf("10回連続パース失敗でfetch_status = %q, want %q", feed.FetchStatus, model.FetchStatusStopped)
	}
}

func TestFetcher_Fetch_LogsStructuredInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel><title>Test</title>
    <item><title>A</title><guid>g1</guid></item>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{insertCount: 1},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:      "feed-1",
		FeedURL: server.URL,
	}

	_ = f.Fetch(context.Background(), feed)

	// 構造化ログにfeed_id、http_status、処理時間が含まれること
	var entry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	foundFeedID := false
	for _, line := range lines {
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if fid, ok := entry["feed_id"]; ok && fid == "feed-1" {
			foundFeedID = true
		}
	}
	if !foundFeedID {
		t.Errorf("ログに feed_id が記録されていない。ログ出力: %s", buf.String())
	}
}

func TestFetcher_Fetch_UpdatesFeedTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Updated Feed Title</title>
    <link>https://example.com</link>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(ctx context.Context, feed *model.Feed) error {
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		logger,
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:      "feed-1",
		FeedURL: server.URL,
		Title:   "Old Title",
	}

	_ = f.Fetch(context.Background(), feed)

	if feed.Title != "Updated Feed Title" {
		t.Errorf("Feed.Title = %q, want %q", feed.Title, "Updated Feed Title")
	}
}
