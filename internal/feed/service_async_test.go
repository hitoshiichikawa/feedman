package feed

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// --- 非同期 favicon 取得テスト用の制御可能なモック ---

// controllableFaviconFetcher はテストから取得の遅延・成否・呼び出し時の context を
// 観測できる FaviconFetcherService モック。
type controllableFaviconFetcher struct {
	mu sync.Mutex

	// block が non-nil の場合、FetchFaviconForSite はこのチャネルが閉じられるまでブロックする。
	block chan struct{}

	// 返却値
	data []byte
	mime string
	err  error

	// 観測用
	called      bool
	ctxCanceled bool // 呼び出し時点で渡された ctx が既にキャンセル済みだったか
}

func (c *controllableFaviconFetcher) FetchFavicon(ctx context.Context, _ string) ([]byte, string, error) {
	return c.FetchFaviconForSite(ctx, "")
}

func (c *controllableFaviconFetcher) FetchFaviconForSite(ctx context.Context, _ string) ([]byte, string, error) {
	c.mu.Lock()
	c.called = true
	c.ctxCanceled = ctx.Err() != nil
	block := c.block
	c.mu.Unlock()

	if block != nil {
		<-block
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data, c.mime, c.err
}

func (c *controllableFaviconFetcher) wasCalled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.called
}

func (c *controllableFaviconFetcher) ctxWasCanceledOnCall() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctxCanceled
}

// newRegisterTestService は新規フィード登録テスト用の FeedService を生成する。
func newRegisterTestService(fetcher FaviconFetcherService) (*FeedService, *mockFeedRepo) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	return NewFeedService(feedRepo, subRepo, detector, fetcher), feedRepo
}

// --- Requirement 1: 登録レスポンスのタイムアウト安全性 ---

// TestRegisterFeed_ReturnsBeforeFaviconCompletes は favicon 取得が遅延・未完了でも
// RegisterFeed が完了を待たずに即座に返ることを検証する（AC 1.1, 1.2, 1.3）。
func TestRegisterFeed_ReturnsBeforeFaviconCompletes(t *testing.T) {
	// Arrange: favicon 取得を明示的にブロックする
	fetcher := &controllableFaviconFetcher{
		block: make(chan struct{}),
		data:  []byte{0x89, 0x50, 0x4E, 0x47},
		mime:  "image/png",
	}
	svc, _ := newRegisterTestService(fetcher)

	// Act: favicon 取得がブロック中でも RegisterFeed は短時間で返るはず
	done := make(chan struct{})
	var feed interface{}
	var regErr error
	start := time.Now()
	go func() {
		f, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
		feed, regErr = f, err
		close(done)
	}()

	// Assert: favicon 取得をブロックしたまま、RegisterFeed が完了することを確認
	select {
	case <-done:
		// 期待どおり即時に返った
	case <-time.After(2 * time.Second):
		close(fetcher.block) // goroutine を解放してリークを防ぐ
		svc.waitFaviconFetch()
		t.Fatal("RegisterFeed が favicon 取得の完了を待ってブロックしている")
	}

	if regErr != nil {
		close(fetcher.block)
		svc.waitFaviconFetch()
		t.Fatalf("RegisterFeed returned error: %v", regErr)
	}
	if feed == nil {
		close(fetcher.block)
		svc.waitFaviconFetch()
		t.Fatal("expected non-nil feed")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("RegisterFeed の応答時間が favicon 取得に依存している: %v", elapsed)
	}

	// クリーンアップ: ブロックを解放しバックグラウンド goroutine を回収する
	close(fetcher.block)
	svc.waitFaviconFetch()
}

// --- Requirement 3: favicon 取得成功時の保存 ---

// TestRegisterFeed_FaviconSavedAsynchronously は favicon 取得がバックグラウンドで成功したとき、
// 最終的に UpdateFavicon が呼ばれ favicon が保存されることを検証する（AC 3.1, 3.2）。
func TestRegisterFeed_FaviconSavedAsynchronously(t *testing.T) {
	// Arrange
	fetcher := &controllableFaviconFetcher{
		data: []byte{0x89, 0x50, 0x4E, 0x47},
		mime: "image/png",
	}
	svc, feedRepo := newRegisterTestService(fetcher)

	// Act
	feed, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}
	svc.waitFaviconFetch()

	// Assert: 非同期完了後に favicon が保存されている
	gotFeedID, gotData, gotMime := feedRepo.getFaviconCall()
	if gotFeedID != feed.ID {
		t.Errorf("UpdateFavicon の feedID = %q, want %q", gotFeedID, feed.ID)
	}
	if gotMime != "image/png" {
		t.Errorf("favicon mime = %q, want %q", gotMime, "image/png")
	}
	if len(gotData) == 0 {
		t.Error("favicon データが保存されていない")
	}
}

// --- Requirement 2: favicon 取得失敗・遅延時の登録成功維持 ---

// TestRegisterFeed_SucceedsWhenFaviconFetchErrors は favicon 取得がエラーを返しても
// RegisterFeed が成功し、favicon が保存されない（null 保持）ことを検証する（AC 2.1, 2.4）。
func TestRegisterFeed_SucceedsWhenFaviconFetchErrors(t *testing.T) {
	// Arrange: favicon 取得がエラーを返す
	fetcher := &controllableFaviconFetcher{
		err: errors.New("favicon 取得失敗"),
	}
	svc, feedRepo := newRegisterTestService(fetcher)

	// Act
	feed, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")

	// Assert: 登録は成功
	if err != nil {
		t.Fatalf("favicon 取得エラーでも RegisterFeed は成功すべき: %v", err)
	}
	if feed == nil {
		t.Fatal("expected non-nil feed")
	}
	svc.waitFaviconFetch()

	// favicon は保存されない（null 保持）
	if gotFeedID, _, _ := feedRepo.getFaviconCall(); gotFeedID != "" {
		t.Errorf("favicon 取得失敗時は UpdateFavicon が呼ばれるべきでない。feedID = %q", gotFeedID)
	}
}

// TestRegisterFeed_SucceedsWhenFaviconNotFound は favicon が見つからない（data == nil）場合でも
// 登録が成功し、favicon が null 保持されることを検証する（AC 2.3, 境界値: 空入力）。
func TestRegisterFeed_SucceedsWhenFaviconNotFound(t *testing.T) {
	// Arrange: favicon 未検出（data == nil, err == nil）
	fetcher := &controllableFaviconFetcher{
		data: nil,
		mime: "",
		err:  nil,
	}
	svc, feedRepo := newRegisterTestService(fetcher)

	// Act
	feed, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("favicon 未検出でも RegisterFeed は成功すべき: %v", err)
	}
	if feed == nil {
		t.Fatal("expected non-nil feed")
	}
	svc.waitFaviconFetch()

	// Assert: favicon は保存されない（null 保持）
	if gotFeedID, _, _ := feedRepo.getFaviconCall(); gotFeedID != "" {
		t.Errorf("favicon 未検出時は UpdateFavicon が呼ばれるべきでない。feedID = %q", gotFeedID)
	}
}

// --- Requirement 3.3 / 境界値: リクエスト ctx キャンセル時も独立 context で継続 ---

// TestRegisterFeed_FaviconContinuesAfterRequestCtxCanceled はリクエストスコープの ctx が
// キャンセル/タイムアウトされても、独立 context で favicon 取得が中断されず継続することを
// 検証する（AC 3.3、境界値）。
func TestRegisterFeed_FaviconContinuesAfterRequestCtxCanceled(t *testing.T) {
	// Arrange: favicon 取得をブロックし、その間にリクエスト ctx をキャンセルする
	fetcher := &controllableFaviconFetcher{
		block: make(chan struct{}),
		data:  []byte{0x89, 0x50, 0x4E, 0x47},
		mime:  "image/png",
	}
	svc, feedRepo := newRegisterTestService(fetcher)

	reqCtx, cancel := context.WithCancel(context.Background())

	// Act: 登録（favicon は非同期で起動される）
	feed, _, err := svc.RegisterFeed(reqCtx, "user-1", "https://example.com")
	if err != nil {
		close(fetcher.block)
		svc.waitFaviconFetch()
		t.Fatalf("RegisterFeed returned error: %v", err)
	}

	// favicon 取得 goroutine が起動して呼び出しに入るまで待つ
	deadline := time.After(2 * time.Second)
	for !fetcher.wasCalled() {
		select {
		case <-deadline:
			close(fetcher.block)
			svc.waitFaviconFetch()
			t.Fatal("favicon 取得が起動されていない")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// リクエストスコープの ctx をキャンセル（レスポンス送出後のリクエスト完了を模す）
	cancel()

	// 取得処理を完了させる
	close(fetcher.block)
	svc.waitFaviconFetch()

	// Assert: リクエスト ctx のキャンセルに引きずられず、独立 context で取得が完了し保存された
	if fetcher.ctxWasCanceledOnCall() {
		t.Error("favicon 取得に渡された context が呼び出し時点で既にキャンセルされている（独立 context になっていない）")
	}
	gotFeedID, _, _ := feedRepo.getFaviconCall()
	if gotFeedID != feed.ID {
		t.Errorf("リクエスト ctx キャンセル後も favicon は保存されるべき。UpdateFavicon feedID = %q, want %q", gotFeedID, feed.ID)
	}
}

// --- Requirement 4: バックグラウンド処理の有界性 ---

// TestBackgroundFaviconTimeout_IsBounded はバックグラウンド favicon 取得に上限時間
// （30 秒以内）が設定されていることを検証する（AC 4.1）。
func TestBackgroundFaviconTimeout_IsBounded(t *testing.T) {
	if backgroundFaviconTimeout <= 0 {
		t.Fatalf("backgroundFaviconTimeout は正の上限時間であるべき: %v", backgroundFaviconTimeout)
	}
	if backgroundFaviconTimeout > 30*time.Second {
		t.Errorf("backgroundFaviconTimeout は 30 秒以内であるべき: %v", backgroundFaviconTimeout)
	}
}

// TestStartFaviconFetch_AppliesTimeoutDeadline は startFaviconFetch が favicon fetcher へ
// 渡す context にデッドライン（上限時間）が設定されていることを検証する（AC 4.1, 4.2）。
func TestStartFaviconFetch_AppliesTimeoutDeadline(t *testing.T) {
	// Arrange: 呼び出し時の context のデッドラインを観測するフェッチャー
	observed := &deadlineObservingFetcher{}
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	svc := NewFeedService(feedRepo, subRepo, detector, observed)

	// Act
	_, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}
	svc.waitFaviconFetch()

	// Assert: 渡された context にデッドラインが設定されている（無制限ではない）
	deadline, hasDeadline := observed.deadline()
	if !hasDeadline {
		t.Fatal("favicon 取得の context に上限時間（デッドライン）が設定されていない")
	}
	if remaining := time.Until(deadline); remaining > backgroundFaviconTimeout+time.Second {
		t.Errorf("デッドラインが上限時間より大幅に先: 残り %v", remaining)
	}
}

// deadlineObservingFetcher は呼び出し時の context のデッドラインを観測する FaviconFetcherService モック。
type deadlineObservingFetcher struct {
	mu          sync.Mutex
	dl          time.Time
	hasDeadline bool
}

func (d *deadlineObservingFetcher) FetchFavicon(ctx context.Context, _ string) ([]byte, string, error) {
	return d.FetchFaviconForSite(ctx, "")
}

func (d *deadlineObservingFetcher) FetchFaviconForSite(ctx context.Context, _ string) ([]byte, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dl, d.hasDeadline = ctx.Deadline()
	return nil, "", nil
}

func (d *deadlineObservingFetcher) deadline() (time.Time, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dl, d.hasDeadline
}
