package hatebu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockItemRepo はバッチジョブ用のItemRepositoryモック。
// HatebuItemRepository インターフェースのみ実装する。
type mockItemRepo struct {
	listNeedingHatebuFetchFunc func(ctx context.Context, limit int) ([]*model.Item, error)
	updateHatebuCountFunc      func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error
}

func (m *mockItemRepo) ListNeedingHatebuFetch(ctx context.Context, limit int) ([]*model.Item, error) {
	if m.listNeedingHatebuFetchFunc != nil {
		return m.listNeedingHatebuFetchFunc(ctx, limit)
	}
	return nil, nil
}

func (m *mockItemRepo) UpdateHatebuCount(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
	if m.updateHatebuCountFunc != nil {
		return m.updateHatebuCountFunc(ctx, itemID, count, fetchedAt)
	}
	return nil
}

// mockHatebuClient ははてなブックマークAPIクライアントのモック。
type mockHatebuClient struct {
	getBookmarkCountsFunc func(ctx context.Context, urls []string) (map[string]int, error)
}

func (m *mockHatebuClient) GetBookmarkCounts(ctx context.Context, urls []string) (map[string]int, error) {
	if m.getBookmarkCountsFunc != nil {
		return m.getBookmarkCountsFunc(ctx, urls)
	}
	return make(map[string]int), nil
}

// --- Task 7.2: BatchJob のテスト ---

func TestNewBatchJob_ReturnsNonNil(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	job := NewBatchJob(
		&mockItemRepo{},
		&mockHatebuClient{},
		logger,
		DefaultBatchConfig(),
	)
	if job == nil {
		t.Fatal("NewBatchJob は nil を返してはならない")
	}
}

func TestDefaultBatchConfig(t *testing.T) {
	cfg := DefaultBatchConfig()

	if cfg.BatchInterval != 10*time.Minute {
		t.Errorf("BatchInterval = %v, want 10m", cfg.BatchInterval)
	}
	if cfg.APIInterval != 5*time.Second {
		t.Errorf("APIInterval = %v, want 5s", cfg.APIInterval)
	}
	if cfg.MaxCallsPerCycle != 100 {
		t.Errorf("MaxCallsPerCycle = %d, want 100", cfg.MaxCallsPerCycle)
	}
	if cfg.HatebuTTL != 24*time.Hour {
		t.Errorf("HatebuTTL = %v, want 24h", cfg.HatebuTTL)
	}
}

func TestBatchJob_RunOnce_NoItems(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return nil, nil
		},
	}

	job := NewBatchJob(repo, &mockHatebuClient{}, logger, DefaultBatchConfig())
	err := job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce がエラーを返した: %v", err)
	}
}

func TestBatchJob_RunOnce_FetchesAndUpdates(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
		{ID: "item-2", Link: "https://example.com/a2"},
	}

	var updatedItems []string
	var updatedCounts []int
	var mu sync.Mutex

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			mu.Lock()
			defer mu.Unlock()
			updatedItems = append(updatedItems, itemID)
			updatedCounts = append(updatedCounts, count)
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return map[string]int{
				"https://example.com/a1": 10,
				"https://example.com/a2": 20,
			}, nil
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	err := job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce がエラーを返した: %v", err)
	}

	if len(updatedItems) != 2 {
		t.Fatalf("更新された記事数 = %d, want 2", len(updatedItems))
	}
}

func TestBatchJob_RunOnce_ChunksURLsBy50(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// 120個の記事を用意（3バッチ: 50 + 50 + 20）
	items := make([]*model.Item, 120)
	for i := range items {
		items[i] = &model.Item{
			ID:   fmt.Sprintf("item-%d", i),
			Link: fmt.Sprintf("https://example.com/article-%d", i),
		}
	}

	var apiCallCount int32

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			atomic.AddInt32(&apiCallCount, 1)
			if len(urls) > 50 {
				t.Errorf("1バッチのURL数が50を超えている: %d", len(urls))
			}
			result := make(map[string]int)
			for _, u := range urls {
				result[u] = 1
			}
			return result, nil
		},
	}

	cfg := DefaultBatchConfig()
	cfg.APIInterval = 1 * time.Millisecond // テスト用に短い間隔

	job := NewBatchJob(repo, client, logger, cfg)
	err := job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce がエラーを返した: %v", err)
	}

	if atomic.LoadInt32(&apiCallCount) != 3 {
		t.Errorf("API呼び出し回数 = %d, want 3 (120 / 50 = 3 chunks)", atomic.LoadInt32(&apiCallCount))
	}
}

func TestBatchJob_RunOnce_RespectsMaxCallsPerCycle(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// 大量の記事を用意
	items := make([]*model.Item, 300)
	for i := range items {
		items[i] = &model.Item{
			ID:   fmt.Sprintf("item-%d", i),
			Link: fmt.Sprintf("https://example.com/article-%d", i),
		}
	}

	var apiCallCount int32

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			atomic.AddInt32(&apiCallCount, 1)
			result := make(map[string]int)
			for _, u := range urls {
				result[u] = 1
			}
			return result, nil
		},
	}

	// MaxCallsPerCycle を 3 に制限
	cfg := DefaultBatchConfig()
	cfg.MaxCallsPerCycle = 3
	cfg.APIInterval = 1 * time.Millisecond // テスト用に短い間隔

	job := NewBatchJob(repo, client, logger, cfg)
	err := job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce がエラーを返した: %v", err)
	}

	// 300記事 / 50URL per call = 6 chunks だが、MaxCallsPerCycle=3 で打ち切り
	if atomic.LoadInt32(&apiCallCount) > 3 {
		t.Errorf("API呼び出し回数 = %d, MaxCallsPerCycle=3 を超えている", atomic.LoadInt32(&apiCallCount))
	}
}

func TestBatchJob_RunOnce_APIErrorMaintainsPreviousValue(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1", HatebuCount: 5},
	}

	var updateCalled bool

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			updateCalled = true
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return nil, errors.New("API error")
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	// API取得失敗時もRunOnce自体はエラーを返さない（ログのみ）
	err := job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce はAPI取得失敗でもエラーを返さないべき: %v", err)
	}

	// 取得失敗時は更新しない（前回値を維持）
	if updateCalled {
		t.Error("API取得失敗時はUpdateHatebuCountを呼ばないべき（前回値維持）")
	}
}

func TestBatchJob_RunOnce_APIErrorLogsOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
	}

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return nil, errors.New("API error")
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	_ = job.RunOnce(context.Background())

	// エラーがログに記録されること
	logOutput := buf.String()
	if !strings.Contains(logOutput, "ERROR") {
		t.Errorf("API取得失敗時にERRORログが記録されるべき: %s", logOutput)
	}
}

func TestBatchJob_RunOnce_RepoListError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return nil, errors.New("db error")
		},
	}

	job := NewBatchJob(repo, &mockHatebuClient{}, logger, DefaultBatchConfig())
	err := job.RunOnce(context.Background())
	if err == nil {
		t.Fatal("リポジトリエラー時にエラーが返されるべき")
	}
}

func TestBatchJob_RunOnce_UpdateErrorLogsOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
		{ID: "item-2", Link: "https://example.com/a2"},
	}

	var updateCallCount int32

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			atomic.AddInt32(&updateCallCount, 1)
			if itemID == "item-1" {
				return errors.New("update failed")
			}
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return map[string]int{
				"https://example.com/a1": 10,
				"https://example.com/a2": 20,
			}, nil
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	err := job.RunOnce(context.Background())
	// 個別更新エラーはRunOnce全体のエラーとはしない
	if err != nil {
		t.Fatalf("個別の更新エラーでRunOnce全体がエラーになるべきではない: %v", err)
	}

	// 両方のアイテムの更新が試行されること
	if atomic.LoadInt32(&updateCallCount) != 2 {
		t.Errorf("更新試行回数 = %d, want 2", atomic.LoadInt32(&updateCallCount))
	}
}

func TestBatchJob_RunOnce_SkipsItemsWithoutLink(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
		{ID: "item-2", Link: ""}, // リンクなし
		{ID: "item-3", Link: "https://example.com/a3"},
	}

	var apiURLs []string

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			apiURLs = append(apiURLs, urls...)
			result := make(map[string]int)
			for _, u := range urls {
				result[u] = 1
			}
			return result, nil
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	_ = job.RunOnce(context.Background())

	// リンクなしの記事はAPI呼び出しに含まれないこと
	for _, u := range apiURLs {
		if u == "" {
			t.Error("空リンクの記事がAPIに渡されるべきではない")
		}
	}

	if len(apiURLs) != 2 {
		t.Errorf("API呼び出しURL数 = %d, want 2", len(apiURLs))
	}
}

func TestBatchJob_RunOnce_LimitPassedToRepo(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	var receivedLimit int

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			receivedLimit = limit
			return nil, nil
		},
	}

	// MaxCallsPerCycle=5、1回のAPIで50URLなので最大250件
	cfg := DefaultBatchConfig()
	cfg.MaxCallsPerCycle = 5

	job := NewBatchJob(repo, &mockHatebuClient{}, logger, cfg)
	_ = job.RunOnce(context.Background())

	expectedLimit := 5 * maxURLsPerRequest // 250
	if receivedLimit != expectedLimit {
		t.Errorf("リポジトリに渡されたlimit = %d, want %d", receivedLimit, expectedLimit)
	}
}

func TestBatchJob_ConsecutiveErrorBackoff(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// 連続エラーのバックオフテスト
	job := NewBatchJob(&mockItemRepo{}, &mockHatebuClient{}, logger, DefaultBatchConfig())

	// 3回連続エラー → 30分待機
	backoff := job.calculateErrorBackoff(3)
	if backoff != 30*time.Minute {
		t.Errorf("3回連続エラーのバックオフ = %v, want 30m", backoff)
	}

	// 5回連続エラー → 1時間待機
	backoff = job.calculateErrorBackoff(5)
	if backoff != 1*time.Hour {
		t.Errorf("5回連続エラーのバックオフ = %v, want 1h", backoff)
	}

	// 10回連続エラー → 6時間待機
	backoff = job.calculateErrorBackoff(10)
	if backoff != 6*time.Hour {
		t.Errorf("10回連続エラーのバックオフ = %v, want 6h", backoff)
	}

	// 1回連続エラー → バックオフなし
	backoff = job.calculateErrorBackoff(1)
	if backoff != 0 {
		t.Errorf("1回連続エラーのバックオフ = %v, want 0", backoff)
	}

	// 2回連続エラー → バックオフなし
	backoff = job.calculateErrorBackoff(2)
	if backoff != 0 {
		t.Errorf("2回連続エラーのバックオフ = %v, want 0", backoff)
	}
}

func TestBatchJob_RunOnce_TracksConsecutiveErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
	}

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return nil, errors.New("API error")
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())

	// 1回目のエラー
	_ = job.RunOnce(context.Background())
	if job.consecutiveErrors != 1 {
		t.Errorf("連続エラー回数 = %d, want 1", job.consecutiveErrors)
	}

	// 2回目のエラー
	_ = job.RunOnce(context.Background())
	if job.consecutiveErrors != 2 {
		t.Errorf("連続エラー回数 = %d, want 2", job.consecutiveErrors)
	}
}

func TestBatchJob_RunOnce_ResetsConsecutiveErrorsOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
	}

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			return nil
		},
	}

	callCount := 0
	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			callCount++
			if callCount <= 2 {
				return nil, errors.New("API error")
			}
			return map[string]int{"https://example.com/a1": 5}, nil
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())

	// 2回連続エラー
	_ = job.RunOnce(context.Background())
	_ = job.RunOnce(context.Background())
	if job.consecutiveErrors != 2 {
		t.Errorf("連続エラー回数 = %d, want 2", job.consecutiveErrors)
	}

	// 成功するとリセット
	_ = job.RunOnce(context.Background())
	if job.consecutiveErrors != 0 {
		t.Errorf("成功後の連続エラー回数 = %d, want 0", job.consecutiveErrors)
	}
}

func TestBatchJob_RunOnce_ContextCancellation(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return nil, ctx.Err()
		},
	}

	job := NewBatchJob(repo, &mockHatebuClient{}, logger, DefaultBatchConfig())
	err := job.RunOnce(ctx)
	if err == nil {
		t.Fatal("キャンセル済みコンテキストでエラーが返されるべき")
	}
}

func TestBatchJob_RunOnce_APIIntervalRespected(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// 100個の記事を用意（2バッチ: 50 + 50）
	items := make([]*model.Item, 100)
	for i := range items {
		items[i] = &model.Item{
			ID:   fmt.Sprintf("item-%d", i),
			Link: fmt.Sprintf("https://example.com/article-%d", i),
		}
	}

	var callTimes []time.Time
	var mu sync.Mutex

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			mu.Lock()
			callTimes = append(callTimes, time.Now())
			mu.Unlock()
			result := make(map[string]int)
			for _, u := range urls {
				result[u] = 1
			}
			return result, nil
		},
	}

	// APIInterval を短くしてテストを高速化
	cfg := DefaultBatchConfig()
	cfg.APIInterval = 100 * time.Millisecond

	job := NewBatchJob(repo, client, logger, cfg)
	err := job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce がエラーを返した: %v", err)
	}

	if len(callTimes) < 2 {
		t.Fatalf("API呼び出し回数 = %d, 2回以上必要", len(callTimes))
	}

	// 2回目の呼び出しが最低APIInterval後であること
	interval := callTimes[1].Sub(callTimes[0])
	if interval < 80*time.Millisecond { // 少し余裕を持たせる
		t.Errorf("API呼び出し間隔 = %v, 100ms以上であるべき", interval)
	}
}

func TestBatchJob_Start_StopsOnContextCancel(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return nil, nil
		},
	}

	cfg := DefaultBatchConfig()
	cfg.BatchInterval = 50 * time.Millisecond // テスト用に短い間隔

	job := NewBatchJob(repo, &mockHatebuClient{}, logger, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		job.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// 正常に停止した
	case <-time.After(5 * time.Second):
		t.Fatal("Start がコンテキストキャンセル後に停止しなかった")
	}
}

func TestBatchJob_RunOnce_LogsCycleInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
	}

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			return nil
		},
	}

	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return map[string]int{"https://example.com/a1": 5}, nil
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	_ = job.RunOnce(context.Background())

	logOutput := buf.String()
	// ログにサイクル情報が含まれること
	var found bool
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	for _, line := range lines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if msg, ok := entry["msg"]; ok {
			if s, ok := msg.(string); ok && strings.Contains(s, "はてなブックマーク") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("ログにはてなブックマーク関連のメッセージが含まれるべき: %s", logOutput)
	}
}

func TestBatchJob_RunOnce_ZeroBookmarkItemsAlsoUpdated(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	items := []*model.Item{
		{ID: "item-1", Link: "https://example.com/a1"},
	}

	var updatedCount int

	repo := &mockItemRepo{
		listNeedingHatebuFetchFunc: func(ctx context.Context, limit int) ([]*model.Item, error) {
			return items, nil
		},
		updateHatebuCountFunc: func(ctx context.Context, itemID string, count int, fetchedAt time.Time) error {
			updatedCount = count
			return nil
		},
	}

	// ブックマーク0件のレスポンス
	client := &mockHatebuClient{
		getBookmarkCountsFunc: func(ctx context.Context, urls []string) (map[string]int, error) {
			return map[string]int{"https://example.com/a1": 0}, nil
		},
	}

	job := NewBatchJob(repo, client, logger, DefaultBatchConfig())
	_ = job.RunOnce(context.Background())

	// 0件でも更新される（hatebu_fetched_atを記録するため）
	if updatedCount != 0 {
		t.Errorf("0件ブックマーク更新時のcount = %d, want 0", updatedCount)
	}
}
