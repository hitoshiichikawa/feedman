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

// mockMetricsCollector は metrics.MetricsCollector のテスト用モック。
// 各記録メソッドの呼び出し回数と最後に渡された値を保持する。
type mockMetricsCollector struct {
	fetchSuccess  int
	fetchFailure  int
	parseFailure  int
	httpStatus    int
	fetchLatency  int
	itemsUpserted int

	lastStatusCode    int
	lastItemsUpserted int
	lastLatency       time.Duration
}

func (m *mockMetricsCollector) RecordFetchSuccess(_ string) { m.fetchSuccess++ }

func (m *mockMetricsCollector) RecordFetchFailure(_, _ string) { m.fetchFailure++ }

func (m *mockMetricsCollector) RecordParseFailure(_ string) { m.parseFailure++ }

func (m *mockMetricsCollector) RecordHTTPStatus(statusCode int) {
	m.httpStatus++
	m.lastStatusCode = statusCode
}

func (m *mockMetricsCollector) RecordFetchLatency(duration time.Duration) {
	m.fetchLatency++
	m.lastLatency = duration
}

func (m *mockMetricsCollector) RecordItemsUpserted(count int) {
	m.itemsUpserted++
	m.lastItemsUpserted = count
}

// 手動フェッチ系（Issue #115）は worker fetcher から呼ばれないが、
// MetricsCollector interface 充足のため no-op 実装する。
func (m *mockMetricsCollector) RecordManualFetchSuccess()          {}
func (m *mockMetricsCollector) RecordManualFetchFailure(_ string)  {}
func (m *mockMetricsCollector) RecordManualFetchCooldownRejected() {}
func (m *mockMetricsCollector) RecordManualFetchLockConflict()     {}

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

// TestFetcher_Fetch_PersistsTitleAndSiteURL は、フェッチ成功時に
// パース済みタイトル・サイト URL が UpdateFetchState（永続化処理）へ
// 渡されることを検証する（Requirement 1.1 / 1.2 / 1.3 / 2.3）。
// mock repo が in-memory の feed をそのまま受け取るため、永続化処理に
// 正しい値が引き渡されることを直接捕捉できる。
func TestFetcher_Fetch_PersistsTitleAndSiteURL(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Parsed Site Title</title>
    <link>https://site.example.com</link>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	var persistedTitle, persistedSiteURL string
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, feed *model.Feed) error {
			persistedTitle = feed.Title
			persistedSiteURL = feed.SiteURL
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:      "feed-1",
		FeedURL: server.URL,
		// 初期タイトルが URL のまま（不具合 #113 の状態）
		Title: server.URL,
	}

	// Act
	if err := f.Fetch(context.Background(), feed); err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// Assert: 永続化処理にパース済みタイトル・サイト URL が渡される
	if persistedTitle != "Parsed Site Title" {
		t.Errorf("UpdateFetchState に渡された Title = %q, want %q", persistedTitle, "Parsed Site Title")
	}
	if persistedSiteURL != "https://site.example.com" {
		t.Errorf("UpdateFetchState に渡された SiteURL = %q, want %q", persistedSiteURL, "https://site.example.com")
	}
}

// TestFetcher_Fetch_EmptyParsedTitleDoesNotOverwrite は、パース済みタイトル・
// サイト URL が空のとき、既存のタイトル・サイト URL が空値で上書きされず、
// 永続化処理にも既存値が引き渡されることを検証する（Requirement 2.1 / 2.2）。
func TestFetcher_Fetch_EmptyParsedTitleDoesNotOverwrite(t *testing.T) {
	// Arrange: title / link を持たないフィードを返すサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <description>タイトル・リンクなしのフィード</description>
    <item><title>記事</title><guid>g1</guid></item>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	var persistedTitle, persistedSiteURL string
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, feed *model.Feed) error {
			persistedTitle = feed.Title
			persistedSiteURL = feed.SiteURL
			return nil
		},
	}

	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{insertCount: 1},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)

	feed := &model.Feed{
		ID:      "feed-1",
		FeedURL: server.URL,
		Title:   "既存タイトル",
		SiteURL: "https://existing.example.com",
	}

	// Act
	if err := f.Fetch(context.Background(), feed); err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// Assert: 既存値が空で上書きされない & 永続化処理にも既存値が渡される
	if feed.Title != "既存タイトル" {
		t.Errorf("Feed.Title = %q, want %q（空で上書きしてはならない）", feed.Title, "既存タイトル")
	}
	if feed.SiteURL != "https://existing.example.com" {
		t.Errorf("Feed.SiteURL = %q, want %q（空で上書きしてはならない）", feed.SiteURL, "https://existing.example.com")
	}
	if persistedTitle != "既存タイトル" {
		t.Errorf("UpdateFetchState に渡された Title = %q, want %q", persistedTitle, "既存タイトル")
	}
	if persistedSiteURL != "https://existing.example.com" {
		t.Errorf("UpdateFetchState に渡された SiteURL = %q, want %q", persistedSiteURL, "https://existing.example.com")
	}
}

// --- Task 3.1: WithMetrics によるメトリクス記録のテスト ---

func TestFetcher_Fetch_Metrics_Success200(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item><title>A</title><guid>g1</guid></item>
  </channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	mc := &mockMetricsCollector{}
	f := NewFetcher(
		&mockFeedRepo{updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil }},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{insertCount: 1},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
		WithMetrics(mc),
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	if err := f.Fetch(context.Background(), feed); err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// Assert: 200 成功時は成功数・HTTP ステータス・レイテンシが各 1 回記録される
	if mc.fetchSuccess != 1 {
		t.Errorf("RecordFetchSuccess 呼び出し回数 = %d, want 1", mc.fetchSuccess)
	}
	if mc.fetchFailure != 0 {
		t.Errorf("RecordFetchFailure 呼び出し回数 = %d, want 0", mc.fetchFailure)
	}
	if mc.httpStatus != 1 || mc.lastStatusCode != http.StatusOK {
		t.Errorf("RecordHTTPStatus = (count %d, code %d), want (1, 200)", mc.httpStatus, mc.lastStatusCode)
	}
	if mc.fetchLatency != 1 {
		t.Errorf("RecordFetchLatency 呼び出し回数 = %d, want 1", mc.fetchLatency)
	}
}

func TestFetcher_Fetch_Metrics_304Success(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	var buf bytes.Buffer
	mc := &mockMetricsCollector{}
	f := NewFetcher(
		&mockFeedRepo{updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil }},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
		WithMetrics(mc),
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, ETag: `"abc"`}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: 304 は成功（変更なし）として成功数に計上される
	if mc.fetchSuccess != 1 {
		t.Errorf("304 時の RecordFetchSuccess 呼び出し回数 = %d, want 1", mc.fetchSuccess)
	}
	if mc.lastStatusCode != http.StatusNotModified {
		t.Errorf("RecordHTTPStatus のステータス = %d, want 304", mc.lastStatusCode)
	}
}

func TestFetcher_Fetch_Metrics_HTTPFailure(t *testing.T) {
	// Arrange: SSRFGuard が ValidateURL を成功させた上で、到達不能なアドレスへ client.Do を失敗させる
	var buf bytes.Buffer
	mc := &mockMetricsCollector{}
	f := NewFetcher(
		&mockFeedRepo{updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil }},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		1*time.Second,
		5*1024*1024,
		WithMetrics(mc),
	)
	// 予約済みドキュメント用ドメインで名前解決に失敗させる
	feed := &model.Feed{ID: "feed-1", FeedURL: "http://nonexistent.invalid/feed.xml", FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: HTTP リクエスト失敗で失敗数が記録され、成功数は 0
	if mc.fetchFailure != 1 {
		t.Errorf("RecordFetchFailure 呼び出し回数 = %d, want 1", mc.fetchFailure)
	}
	if mc.fetchSuccess != 0 {
		t.Errorf("RecordFetchSuccess 呼び出し回数 = %d, want 0", mc.fetchSuccess)
	}
	// レイテンシは defer で常に記録される
	if mc.fetchLatency != 1 {
		t.Errorf("RecordFetchLatency 呼び出し回数 = %d, want 1", mc.fetchLatency)
	}
}

func TestFetcher_Fetch_Metrics_ParseFailure(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `not valid XML at all!!!`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	mc := &mockMetricsCollector{}
	f := NewFetcher(
		&mockFeedRepo{updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil }},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
		WithMetrics(mc),
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: パース失敗はパース失敗数とフェッチ失敗数の両方を記録する
	if mc.parseFailure != 1 {
		t.Errorf("RecordParseFailure 呼び出し回数 = %d, want 1", mc.parseFailure)
	}
	if mc.fetchFailure != 1 {
		t.Errorf("RecordFetchFailure 呼び出し回数 = %d, want 1", mc.fetchFailure)
	}
	if mc.fetchSuccess != 0 {
		t.Errorf("RecordFetchSuccess 呼び出し回数 = %d, want 0", mc.fetchSuccess)
	}
}

func TestFetcher_Fetch_Metrics_HTTPStatusRecordedOnBackoff(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	mc := &mockMetricsCollector{}
	f := NewFetcher(
		&mockFeedRepo{updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil }},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
		WithMetrics(mc),
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: 500 はバックオフ失敗として失敗数を記録し、HTTP ステータスも記録される
	if mc.lastStatusCode != http.StatusInternalServerError {
		t.Errorf("RecordHTTPStatus のステータス = %d, want 500", mc.lastStatusCode)
	}
	if mc.fetchFailure != 1 {
		t.Errorf("RecordFetchFailure 呼び出し回数 = %d, want 1", mc.fetchFailure)
	}
}

func TestNewFetcher_DefaultMetricsIsNopAndNilSafe(t *testing.T) {
	// Arrange: WithMetrics を指定しない（既存 7 引数 call site 相当）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title></channel></rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	f := NewFetcher(
		&mockFeedRepo{updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil }},
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act + Assert: option 未指定でも nil 参照で panic せず正常完了する
	if err := f.Fetch(context.Background(), feed); err != nil {
		t.Fatalf("option 未指定の Fetch() がエラーを返した: %v", err)
	}
}

// --- Task 3.1: ApplySuccess 直後の UpdateLastSuccessfulFetchAt 反映テスト ---

// TestFetcher_Fetch_RecordsLastSuccessfulFetchAt_200 は 200 OK 成功時に
// UpdateLastSuccessfulFetchAt が feed.ID 付きで 1 回呼ばれることを検証する
// （Issue #115 Req 2.4 / 自動経路の成功時刻記録）。
func TestFetcher_Fetch_RecordsLastSuccessfulFetchAt_200(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel><title>T</title><item><title>A</title><guid>g1</guid></item></channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
	}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{insertCount: 1},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	if err := f.Fetch(context.Background(), feed); err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// Assert: 200 成功で UpdateLastSuccessfulFetchAt が 1 回 / feed.ID 引数で呼ばれる
	if feedRepo.lastSuccessfulFetchAtCalls != 1 {
		t.Errorf("UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 1", feedRepo.lastSuccessfulFetchAtCalls)
	}
	if len(feedRepo.lastSuccessfulFetchAtFeedIDs) != 1 || feedRepo.lastSuccessfulFetchAtFeedIDs[0] != "feed-1" {
		t.Errorf("UpdateLastSuccessfulFetchAt に渡された feedID = %v, want [feed-1]", feedRepo.lastSuccessfulFetchAtFeedIDs)
	}
}

// TestFetcher_Fetch_RecordsLastSuccessfulFetchAt_304 は 304 Not Modified 成功時に
// UpdateLastSuccessfulFetchAt が 1 回呼ばれることを検証する（Issue #115 Req 2.4）。
func TestFetcher_Fetch_RecordsLastSuccessfulFetchAt_304(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
	}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, ETag: `"abc"`}

	// Act
	if err := f.Fetch(context.Background(), feed); err != nil {
		t.Fatalf("Fetch() がエラーを返した: %v", err)
	}

	// Assert: 304 成功で UpdateLastSuccessfulFetchAt が 1 回呼ばれる
	if feedRepo.lastSuccessfulFetchAtCalls != 1 {
		t.Errorf("304 時の UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 1", feedRepo.lastSuccessfulFetchAtCalls)
	}
}

// TestFetcher_Fetch_DoesNotRecordOnBackoff は 429 / 5xx バックオフ経路で
// UpdateLastSuccessfulFetchAt が呼ばれないことを検証する（Issue #115 Req 2.4: 成功時刻は ApplySuccess 経由のみ）。
func TestFetcher_Fetch_DoesNotRecordOnBackoff(t *testing.T) {
	// Arrange: 500 を返してバックオフ経路に入れる
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
	}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: バックオフ経路では成功時刻を記録しない
	if feedRepo.lastSuccessfulFetchAtCalls != 0 {
		t.Errorf("バックオフ時の UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 0", feedRepo.lastSuccessfulFetchAtCalls)
	}
}

// TestFetcher_Fetch_DoesNotRecordOnStopFeed は 404 等の停止経路で
// UpdateLastSuccessfulFetchAt が呼ばれないことを検証する。
func TestFetcher_Fetch_DoesNotRecordOnStopFeed(t *testing.T) {
	// Arrange: 404 を返して停止経路に入れる
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
	}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: 停止経路では成功時刻を記録しない
	if feedRepo.lastSuccessfulFetchAtCalls != 0 {
		t.Errorf("停止経路時の UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 0", feedRepo.lastSuccessfulFetchAtCalls)
	}
}

// TestFetcher_Fetch_DoesNotRecordOnSSRFFailure は SSRF 検証失敗経路で
// UpdateLastSuccessfulFetchAt が呼ばれないことを検証する。
func TestFetcher_Fetch_DoesNotRecordOnSSRFFailure(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
	}
	ssrfGuard := &mockSSRFGuard{validateErr: fmt.Errorf("blocked IP address")}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		ssrfGuard,
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: "http://192.168.1.1/feed.xml", FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: SSRF 失敗時は成功時刻を記録しない
	if feedRepo.lastSuccessfulFetchAtCalls != 0 {
		t.Errorf("SSRF 失敗時の UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 0", feedRepo.lastSuccessfulFetchAtCalls)
	}
}

// TestFetcher_Fetch_DoesNotRecordOnParseFailure はパース失敗経路で
// UpdateLastSuccessfulFetchAt が呼ばれないことを検証する。
func TestFetcher_Fetch_DoesNotRecordOnParseFailure(t *testing.T) {
	// Arrange: 不正な XML を返す
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `not valid XML!!!`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
	}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	_ = f.Fetch(context.Background(), feed)

	// Assert: パース失敗時は成功時刻を記録しない
	if feedRepo.lastSuccessfulFetchAtCalls != 0 {
		t.Errorf("パース失敗時の UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 0", feedRepo.lastSuccessfulFetchAtCalls)
	}
}

// TestFetcher_Fetch_LastSuccessfulFetchAtErrorDoesNotFailFetch は
// UpdateLastSuccessfulFetchAt がエラーを返してもフェッチ自体は成功扱いを維持することを検証する
// （Issue #115 Req 2.4: 成功時刻の記録失敗は fetch 全体の失敗にしない）。
func TestFetcher_Fetch_LastSuccessfulFetchAtErrorDoesNotFailFetch(t *testing.T) {
	// Arrange: 200 + 成功時刻記録だけ失敗するモック
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<rss version="2.0">
  <channel><title>T</title><item><title>A</title><guid>g1</guid></item></channel>
</rss>`)
	}))
	defer server.Close()

	var buf bytes.Buffer
	feedRepo := &mockFeedRepo{
		updateFetchStateFunc: func(_ context.Context, _ *model.Feed) error { return nil },
		updateLastSuccessfulFetchAtFn: func(_ context.Context, _ string, _ time.Time) error {
			return fmt.Errorf("simulated db error")
		},
	}
	f := NewFetcher(
		feedRepo,
		&mockSubRepo{minInterval: 60},
		&mockUpsertService{insertCount: 1},
		&mockSSRFGuard{},
		newTestLogger(&buf),
		10*time.Second,
		5*1024*1024,
	)
	feed := &model.Feed{ID: "feed-1", FeedURL: server.URL, FetchStatus: model.FetchStatusActive}

	// Act
	err := f.Fetch(context.Background(), feed)

	// Assert: 成功時刻の記録失敗で fetch 自体は失敗扱いしない
	if err != nil {
		t.Fatalf("Fetch() は last_successful_fetch_at 更新失敗時もエラーを返さないべき: %v", err)
	}
	if feedRepo.lastSuccessfulFetchAtCalls != 1 {
		t.Errorf("UpdateLastSuccessfulFetchAt 呼び出し回数 = %d, want 1", feedRepo.lastSuccessfulFetchAtCalls)
	}
}
