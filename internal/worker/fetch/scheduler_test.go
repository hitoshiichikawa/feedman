package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockFeedRepo はFeedRepositoryのテスト用モック。
type mockFeedRepo struct {
	listDueForFetchFunc   func(ctx context.Context) ([]*model.Feed, error)
	updateFetchStateFunc  func(ctx context.Context, feed *model.Feed) error
	findByIDFunc          func(ctx context.Context, id string) (*model.Feed, error)
	findByFeedURLFunc     func(ctx context.Context, feedURL string) (*model.Feed, error)
	createFunc            func(ctx context.Context, feed *model.Feed) error
	updateFunc            func(ctx context.Context, feed *model.Feed) error
	updateFaviconFunc     func(ctx context.Context, feedID string, faviconData []byte, faviconMime string) error
}

func (m *mockFeedRepo) FindByID(ctx context.Context, id string) (*model.Feed, error) {
	if m.findByIDFunc != nil {
		return m.findByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockFeedRepo) FindByFeedURL(ctx context.Context, feedURL string) (*model.Feed, error) {
	if m.findByFeedURLFunc != nil {
		return m.findByFeedURLFunc(ctx, feedURL)
	}
	return nil, nil
}

func (m *mockFeedRepo) Create(ctx context.Context, feed *model.Feed) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, feed)
	}
	return nil
}

func (m *mockFeedRepo) Update(ctx context.Context, feed *model.Feed) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, feed)
	}
	return nil
}

func (m *mockFeedRepo) UpdateFavicon(ctx context.Context, feedID string, faviconData []byte, faviconMime string) error {
	if m.updateFaviconFunc != nil {
		return m.updateFaviconFunc(ctx, feedID, faviconData, faviconMime)
	}
	return nil
}

func (m *mockFeedRepo) ListDueForFetch(ctx context.Context) ([]*model.Feed, error) {
	if m.listDueForFetchFunc != nil {
		return m.listDueForFetchFunc(ctx)
	}
	return nil, nil
}

func (m *mockFeedRepo) UpdateFetchState(ctx context.Context, feed *model.Feed) error {
	if m.updateFetchStateFunc != nil {
		return m.updateFetchStateFunc(ctx, feed)
	}
	return nil
}

// mockFetcher はFeedFetcherのテスト用モック。
type mockFetcher struct {
	fetchFunc func(ctx context.Context, feed *model.Feed) error
}

func (m *mockFetcher) Fetch(ctx context.Context, feed *model.Feed) error {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, feed)
	}
	return nil
}

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// --- Task 6.1: スケジューラのテスト ---

func TestNewScheduler_ReturnsNonNil(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	s := NewScheduler(&mockFeedRepo{}, &mockFetcher{}, logger, 10)
	if s == nil {
		t.Fatal("NewScheduler は nil を返してはならない")
	}
}

func TestNewScheduler_SetsMaxConcurrency(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	s := NewScheduler(&mockFeedRepo{}, &mockFetcher{}, logger, 5)
	if s.maxConcurrency != 5 {
		t.Errorf("maxConcurrency = %d, want 5", s.maxConcurrency)
	}
}

func TestNewScheduler_DefaultConcurrency(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// 0以下の場合はデフォルトの10を使用する
	s := NewScheduler(&mockFeedRepo{}, &mockFetcher{}, logger, 0)
	if s.maxConcurrency != 10 {
		t.Errorf("maxConcurrency = %d, want 10 (default)", s.maxConcurrency)
	}
}

func TestScheduler_RunOnce_FetchesDueFeeds(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feeds := []*model.Feed{
		{ID: "feed-1", FeedURL: "https://example.com/feed1.xml", FetchStatus: model.FetchStatusActive},
		{ID: "feed-2", FeedURL: "https://example.com/feed2.xml", FetchStatus: model.FetchStatusActive},
	}

	var fetchedIDs []string
	var mu sync.Mutex

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return feeds, nil
		},
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, feed *model.Feed) error {
			mu.Lock()
			fetchedIDs = append(fetchedIDs, feed.ID)
			mu.Unlock()
			return nil
		},
	}

	s := NewScheduler(repo, fetcher, logger, 10)
	err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() がエラーを返した: %v", err)
	}

	if len(fetchedIDs) != 2 {
		t.Errorf("フェッチされたフィード数 = %d, want 2", len(fetchedIDs))
	}
}

func TestScheduler_RunOnce_NoDueFeeds(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return nil, nil
		},
	}

	fetcher := &mockFetcher{}

	s := NewScheduler(repo, fetcher, logger, 10)
	err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() がエラーを返した: %v", err)
	}
}

func TestScheduler_RunOnce_RepoError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return nil, errors.New("db connection failed")
		},
	}

	s := NewScheduler(repo, &mockFetcher{}, logger, 10)
	err := s.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce() はリポジトリエラー時にエラーを返すべき")
	}
}

func TestScheduler_RunOnce_ConcurrencyLimit(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// 20個のフィードを用意し、最大並列数を3に制限
	feeds := make([]*model.Feed, 20)
	for i := range feeds {
		feeds[i] = &model.Feed{ID: "feed-" + string(rune('a'+i)), FetchStatus: model.FetchStatusActive}
	}

	var maxConcurrent int32
	var currentConcurrent int32
	var fetchCount int32

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return feeds, nil
		},
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, feed *model.Feed) error {
			current := atomic.AddInt32(&currentConcurrent, 1)
			defer atomic.AddInt32(&currentConcurrent, -1)
			atomic.AddInt32(&fetchCount, 1)

			// 最大同時実行数を記録
			for {
				old := atomic.LoadInt32(&maxConcurrent)
				if current <= old {
					break
				}
				if atomic.CompareAndSwapInt32(&maxConcurrent, old, current) {
					break
				}
			}

			// 少し待つことで並列実行を促す
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}

	s := NewScheduler(repo, fetcher, logger, 3)
	err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() がエラーを返した: %v", err)
	}

	if atomic.LoadInt32(&fetchCount) != 20 {
		t.Errorf("フェッチ回数 = %d, want 20", atomic.LoadInt32(&fetchCount))
	}

	if atomic.LoadInt32(&maxConcurrent) > 3 {
		t.Errorf("最大同時実行数 = %d, 3以下であるべき", atomic.LoadInt32(&maxConcurrent))
	}
}

func TestScheduler_RunOnce_FetchErrorDoesNotStopOthers(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feeds := []*model.Feed{
		{ID: "feed-1", FetchStatus: model.FetchStatusActive},
		{ID: "feed-2", FetchStatus: model.FetchStatusActive},
		{ID: "feed-3", FetchStatus: model.FetchStatusActive},
	}

	var fetchCount int32

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return feeds, nil
		},
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, feed *model.Feed) error {
			atomic.AddInt32(&fetchCount, 1)
			if feed.ID == "feed-2" {
				return errors.New("fetch failed")
			}
			return nil
		},
	}

	s := NewScheduler(repo, fetcher, logger, 10)
	// 個別フィードのフェッチエラーはRunOnceのエラーとはならない
	err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() は個別フェッチエラーでもエラーを返さないべき: %v", err)
	}

	if atomic.LoadInt32(&fetchCount) != 3 {
		t.Errorf("全フィードのフェッチが試行されるべき: got %d, want 3", atomic.LoadInt32(&fetchCount))
	}
}

func TestScheduler_RunOnce_LogsFetchError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feeds := []*model.Feed{
		{ID: "feed-1", FetchStatus: model.FetchStatusActive},
	}

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return feeds, nil
		},
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, feed *model.Feed) error {
			return errors.New("timeout")
		},
	}

	s := NewScheduler(repo, fetcher, logger, 10)
	_ = s.RunOnce(context.Background())

	// エラーログが出力されていること
	logOutput := buf.String()
	if !strings.Contains(logOutput, "ERROR") {
		t.Errorf("フェッチエラー時にERRORレベルのログが記録されていない: %s", logOutput)
	}
}

func TestScheduler_RunOnce_LogsFetchCount(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	feeds := []*model.Feed{
		{ID: "feed-1", FetchStatus: model.FetchStatusActive},
		{ID: "feed-2", FetchStatus: model.FetchStatusActive},
	}

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return feeds, nil
		},
	}

	fetcher := &mockFetcher{}

	s := NewScheduler(repo, fetcher, logger, 10)
	_ = s.RunOnce(context.Background())

	// ログにフェッチ対象数が記録されていること
	var entry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	found := false
	for _, line := range lines {
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if count, ok := entry["feed_count"]; ok {
			if count == float64(2) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("ログに feed_count=2 が記録されていない。ログ出力: %s", buf.String())
	}
}

func TestScheduler_RunOnce_RespectsContext(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	repo := &mockFeedRepo{
		listDueForFetchFunc: func(ctx context.Context) ([]*model.Feed, error) {
			return nil, ctx.Err()
		},
	}

	s := NewScheduler(repo, &mockFetcher{}, logger, 10)
	err := s.RunOnce(ctx)

	// キャンセル済みコンテキストではエラーが返る
	if err == nil {
		t.Fatal("キャンセル済みコンテキストではエラーが返るべき")
	}
}
