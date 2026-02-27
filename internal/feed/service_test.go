package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- FeedService テスト用モック ---

// mockFeedRepo はテスト用のFeedRepositoryモック。
type mockFeedRepo struct {
	feeds       map[string]*model.Feed
	feedByURL   map[string]*model.Feed
	createCalls int
	updateCalls int
	faviconCall struct {
		feedID      string
		faviconData []byte
		faviconMime string
	}
}

func newMockFeedRepo() *mockFeedRepo {
	return &mockFeedRepo{
		feeds:     make(map[string]*model.Feed),
		feedByURL: make(map[string]*model.Feed),
	}
}

func (m *mockFeedRepo) FindByID(_ context.Context, id string) (*model.Feed, error) {
	f, ok := m.feeds[id]
	if !ok {
		return nil, nil
	}
	return f, nil
}

func (m *mockFeedRepo) FindByFeedURL(_ context.Context, feedURL string) (*model.Feed, error) {
	f, ok := m.feedByURL[feedURL]
	if !ok {
		return nil, nil
	}
	return f, nil
}

func (m *mockFeedRepo) Create(_ context.Context, feed *model.Feed) error {
	m.createCalls++
	m.feeds[feed.ID] = feed
	m.feedByURL[feed.FeedURL] = feed
	return nil
}

func (m *mockFeedRepo) Update(_ context.Context, feed *model.Feed) error {
	m.updateCalls++
	m.feeds[feed.ID] = feed
	m.feedByURL[feed.FeedURL] = feed
	return nil
}

func (m *mockFeedRepo) UpdateFavicon(_ context.Context, feedID string, data []byte, mime string) error {
	m.faviconCall.feedID = feedID
	m.faviconCall.faviconData = data
	m.faviconCall.faviconMime = mime
	if f, ok := m.feeds[feedID]; ok {
		f.FaviconData = data
		f.FaviconMime = mime
	}
	return nil
}

func (m *mockFeedRepo) ListDueForFetch(_ context.Context) ([]*model.Feed, error) {
	return nil, nil
}

func (m *mockFeedRepo) UpdateFetchState(_ context.Context, _ *model.Feed) error {
	return nil
}

// mockSubRepo はテスト用のSubscriptionRepositoryモック。
type mockSubRepo struct {
	subs        map[string]*model.Subscription
	countByUser map[string]int
	createCalls int
}

func newMockSubRepo() *mockSubRepo {
	return &mockSubRepo{
		subs:        make(map[string]*model.Subscription),
		countByUser: make(map[string]int),
	}
}

func (m *mockSubRepo) FindByID(_ context.Context, id string) (*model.Subscription, error) {
	s, ok := m.subs[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockSubRepo) FindByUserAndFeed(_ context.Context, userID, feedID string) (*model.Subscription, error) {
	for _, s := range m.subs {
		if s.UserID == userID && s.FeedID == feedID {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockSubRepo) CountByUserID(_ context.Context, userID string) (int, error) {
	return m.countByUser[userID], nil
}

func (m *mockSubRepo) Create(_ context.Context, sub *model.Subscription) error {
	m.createCalls++
	m.subs[sub.ID] = sub
	m.countByUser[sub.UserID]++
	return nil
}

func (m *mockSubRepo) ListByUserID(_ context.Context, userID string) ([]*model.Subscription, error) {
	var result []*model.Subscription
	for _, s := range m.subs {
		if s.UserID == userID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSubRepo) MinFetchIntervalByFeedID(_ context.Context, _ string) (int, error) {
	return 60, nil
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

// mockFaviconFetcher はテスト用のFaviconFetcherモック。
type mockFaviconFetcher struct {
	faviconData []byte
	faviconMime string
	err         error
}

func (m *mockFaviconFetcher) FetchFavicon(_ context.Context, _ string) ([]byte, string, error) {
	return m.faviconData, m.faviconMime, m.err
}

func (m *mockFaviconFetcher) FetchFaviconForSite(_ context.Context, _ string) ([]byte, string, error) {
	return m.faviconData, m.faviconMime, m.err
}

// mockDetector はテスト用のFeedDetectorモック。
type mockDetector struct {
	feedURL string
	err     error
}

func (m *mockDetector) DetectFeedURL(_ context.Context, _ string) (string, error) {
	return m.feedURL, m.err
}

// --- FeedService テスト ---

// TestNewFeedService はFeedServiceが正しく初期化されることを検証する。
func TestNewFeedService_Initializes(t *testing.T) {
	svc := NewFeedService(
		newMockFeedRepo(),
		newMockSubRepo(),
		&mockDetector{},
		&mockFaviconFetcher{},
	)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// TestFeedService_RegisterFeed_NewFeed は新規フィード登録が正常に動作することをテストする。
func TestFeedService_RegisterFeed_NewFeed(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{
		faviconData: []byte{0x89, 0x50, 0x4E, 0x47},
		faviconMime: "image/png",
	}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	feed, sub, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}
	if feed == nil {
		t.Fatal("expected non-nil feed")
	}
	if sub == nil {
		t.Fatal("expected non-nil subscription")
	}
	if feed.FeedURL != "https://example.com/feed.xml" {
		t.Errorf("feed.FeedURL = %q, want %q", feed.FeedURL, "https://example.com/feed.xml")
	}
	if sub.UserID != "user-1" {
		t.Errorf("sub.UserID = %q, want %q", sub.UserID, "user-1")
	}
	if sub.FeedID != feed.ID {
		t.Errorf("sub.FeedID = %q, want %q", sub.FeedID, feed.ID)
	}
	if feedRepo.createCalls != 1 {
		t.Errorf("feedRepo.Create should be called 1 time, got %d", feedRepo.createCalls)
	}
	if subRepo.createCalls != 1 {
		t.Errorf("subRepo.Create should be called 1 time, got %d", subRepo.createCalls)
	}
}

// TestFeedService_RegisterFeed_ExistingFeed は既存フィードへの購読が正常に動作することをテストする。
func TestFeedService_RegisterFeed_ExistingFeed(t *testing.T) {
	feedRepo := newMockFeedRepo()
	existingFeed := &model.Feed{
		ID:          "existing-feed-id",
		FeedURL:     "https://example.com/feed.xml",
		Title:       "既存フィード",
		FetchStatus: model.FetchStatusActive,
	}
	feedRepo.feeds[existingFeed.ID] = existingFeed
	feedRepo.feedByURL[existingFeed.FeedURL] = existingFeed

	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	feed, sub, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}
	if feed.ID != "existing-feed-id" {
		t.Errorf("既存フィードのIDが使用されるべき。feed.ID = %q, want %q", feed.ID, "existing-feed-id")
	}
	if feedRepo.createCalls != 0 {
		t.Errorf("既存フィードの場合、feedRepo.Createは呼ばれるべきでない。got %d", feedRepo.createCalls)
	}
	if sub.FeedID != "existing-feed-id" {
		t.Errorf("sub.FeedID = %q, want %q", sub.FeedID, "existing-feed-id")
	}
}

// TestFeedService_RegisterFeed_DuplicateSubscription は同じユーザーが同じフィードを重複登録しようとした場合のテスト。
func TestFeedService_RegisterFeed_DuplicateSubscription(t *testing.T) {
	feedRepo := newMockFeedRepo()
	existingFeed := &model.Feed{
		ID:      "existing-feed-id",
		FeedURL: "https://example.com/feed.xml",
		Title:   "既存フィード",
	}
	feedRepo.feeds[existingFeed.ID] = existingFeed
	feedRepo.feedByURL[existingFeed.FeedURL] = existingFeed

	subRepo := newMockSubRepo()
	subRepo.subs["existing-sub-id"] = &model.Subscription{
		ID:     "existing-sub-id",
		UserID: "user-1",
		FeedID: "existing-feed-id",
	}

	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	_, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err == nil {
		t.Fatal("重複購読はエラーを返すべき")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("APIError型が期待されるが、%T が返された", err)
	}
	if apiErr.Code != "DUPLICATE_SUBSCRIPTION" {
		t.Errorf("エラーコード = %q, want %q", apiErr.Code, "DUPLICATE_SUBSCRIPTION")
	}
}

// TestFeedService_RegisterFeed_SubscriptionLimit は購読上限(100件)に達した場合のエラーをテストする。
func TestFeedService_RegisterFeed_SubscriptionLimit(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	subRepo.countByUser["user-1"] = 100 // 上限到達

	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	_, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err == nil {
		t.Fatal("購読上限到達時はエラーを返すべき")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("APIError型が期待されるが、%T が返された", err)
	}
	if apiErr.Code != model.ErrCodeSubscriptionLimit {
		t.Errorf("エラーコード = %q, want %q", apiErr.Code, model.ErrCodeSubscriptionLimit)
	}
}

// TestFeedService_RegisterFeed_DetectionFails はフィード検出に失敗した場合のエラーをテストする。
func TestFeedService_RegisterFeed_DetectionFails(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{
		feedURL: "",
		err:     model.NewFeedNotDetectedError("https://example.com"),
	}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	_, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err == nil {
		t.Fatal("フィード検出失敗時はエラーを返すべき")
	}
}

// TestFeedService_RegisterFeed_FaviconFetchFailure はfavicon取得失敗時にnullとして保存されることをテストする。
func TestFeedService_RegisterFeed_FaviconFetchFailure(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	// favicon取得は失敗するがnilデータを返す
	faviconFetcher := &mockFaviconFetcher{
		faviconData: nil,
		faviconMime: "",
	}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	feed, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("favicon取得失敗でもRegisterFeedは成功すべき: %v", err)
	}
	if feed == nil {
		t.Fatal("expected non-nil feed")
	}
	// favicon取得失敗時はnullとして保存される
	if feed.FaviconData != nil {
		t.Error("favicon取得失敗時はfavicon_dataがnilであるべき")
	}
}

// TestFeedService_RegisterFeed_FaviconSavedOnSuccess はfavicon取得成功時にバイナリ保存されることをテストする。
func TestFeedService_RegisterFeed_FaviconSavedOnSuccess(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{
		faviconData: []byte{0x89, 0x50, 0x4E, 0x47},
		faviconMime: "image/png",
	}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	feed, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}

	// faviconデータが保存されていることを確認
	if feedRepo.faviconCall.feedID != feed.ID {
		t.Errorf("UpdateFaviconのfeedID = %q, want %q", feedRepo.faviconCall.feedID, feed.ID)
	}
	if feedRepo.faviconCall.faviconMime != "image/png" {
		t.Errorf("faviconMime = %q, want %q", feedRepo.faviconCall.faviconMime, "image/png")
	}
}

// TestFeedService_GetFeed はフィード取得が正常に動作することをテストする。
func TestFeedService_GetFeed(t *testing.T) {
	feedRepo := newMockFeedRepo()
	feedRepo.feeds["feed-1"] = &model.Feed{
		ID:      "feed-1",
		FeedURL: "https://example.com/feed.xml",
		Title:   "テストフィード",
	}

	svc := NewFeedService(feedRepo, newMockSubRepo(), &mockDetector{}, &mockFaviconFetcher{})

	feed, err := svc.GetFeed(context.Background(), "feed-1")
	if err != nil {
		t.Fatalf("GetFeed returned error: %v", err)
	}
	if feed == nil {
		t.Fatal("expected non-nil feed")
	}
	if feed.Title != "テストフィード" {
		t.Errorf("feed.Title = %q, want %q", feed.Title, "テストフィード")
	}
}

// TestFeedService_GetFeed_NotFound は存在しないフィードの取得でnilを返すことをテストする。
func TestFeedService_GetFeed_NotFound(t *testing.T) {
	feedRepo := newMockFeedRepo()
	svc := NewFeedService(feedRepo, newMockSubRepo(), &mockDetector{}, &mockFaviconFetcher{})

	feed, err := svc.GetFeed(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetFeed returned error: %v", err)
	}
	if feed != nil {
		t.Error("存在しないフィードはnilを返すべき")
	}
}

// TestFeedService_UpdateFeedURL はフィードURL更新が正常に動作することをテストする。
func TestFeedService_UpdateFeedURL(t *testing.T) {
	feedRepo := newMockFeedRepo()
	feedRepo.feeds["feed-1"] = &model.Feed{
		ID:          "feed-1",
		FeedURL:     "https://example.com/old-feed.xml",
		Title:       "テストフィード",
		FetchStatus: model.FetchStatusActive,
	}

	svc := NewFeedService(feedRepo, newMockSubRepo(), &mockDetector{}, &mockFaviconFetcher{})

	feed, err := svc.UpdateFeedURL(context.Background(), "feed-1", "https://example.com/new-feed.xml")
	if err != nil {
		t.Fatalf("UpdateFeedURL returned error: %v", err)
	}
	if feed.FeedURL != "https://example.com/new-feed.xml" {
		t.Errorf("feed.FeedURL = %q, want %q", feed.FeedURL, "https://example.com/new-feed.xml")
	}
	if feedRepo.updateCalls != 1 {
		t.Errorf("feedRepo.Update should be called 1 time, got %d", feedRepo.updateCalls)
	}
}

// TestFeedService_UpdateFeedURL_NotFound は存在しないフィードのURL更新がエラーを返すことをテストする。
func TestFeedService_UpdateFeedURL_NotFound(t *testing.T) {
	feedRepo := newMockFeedRepo()
	svc := NewFeedService(feedRepo, newMockSubRepo(), &mockDetector{}, &mockFaviconFetcher{})

	_, err := svc.UpdateFeedURL(context.Background(), "nonexistent", "https://example.com/new-feed.xml")
	if err == nil {
		t.Fatal("存在しないフィードの更新はエラーを返すべき")
	}
}

// TestFeedService_RegisterFeed_SubscriptionLimitBoundary は購読数が99の場合に登録可能であることをテストする。
func TestFeedService_RegisterFeed_SubscriptionLimitBoundary(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	subRepo.countByUser["user-1"] = 99 // 上限-1

	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	_, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("購読数99の場合はまだ登録可能であるべき: %v", err)
	}
}

// TestFeedService_RegisterFeed_DefaultFetchInterval は新規購読のデフォルトフェッチ間隔が60分であることをテストする。
func TestFeedService_RegisterFeed_DefaultFetchInterval(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	_, sub, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}
	if sub.FetchIntervalMinutes != 60 {
		t.Errorf("sub.FetchIntervalMinutes = %d, want %d", sub.FetchIntervalMinutes, 60)
	}
}

// --- FeedService + FeedDetector 結合テスト ---

// TestFeedService_RegisterFeed_Integration_WithHTTPServer はHTTPサーバーを使った結合テスト。
func TestFeedService_RegisterFeed_Integration_WithHTTPServer(t *testing.T) {
	var serverURL string

	// RSSフィードを返すテストサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><head>
				<link rel="alternate" type="application/rss+xml" href="%s/feed.xml">
			</head><body></body></html>`, serverURL)
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte{0x00, 0x00, 0x01, 0x00})
		}
	}))
	defer server.Close()
	serverURL = server.URL

	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()

	// 実際のFeedDetectorを使用
	guard := &mockSSRFGuard{}
	realDetector := NewFeedDetector(guard)
	detectorAdapter := &feedDetectorAdapter{detector: realDetector}

	faviconFetcher := &mockFaviconFetcher{
		faviconData: []byte{0x00, 0x00, 0x01, 0x00},
		faviconMime: "image/x-icon",
	}

	svc := NewFeedService(feedRepo, subRepo, detectorAdapter, faviconFetcher)

	feed, sub, err := svc.RegisterFeed(context.Background(), "user-1", server.URL+"/")
	if err != nil {
		t.Fatalf("RegisterFeed returned error: %v", err)
	}
	if feed == nil {
		t.Fatal("expected non-nil feed")
	}
	if sub == nil {
		t.Fatal("expected non-nil subscription")
	}
	if feed.FeedURL != server.URL+"/feed.xml" {
		t.Errorf("feed.FeedURL = %q, want %q", feed.FeedURL, server.URL+"/feed.xml")
	}
}

// feedDetectorAdapter はFeedDetectorをDetectorインターフェースに適合させるアダプター。
type feedDetectorAdapter struct {
	detector *FeedDetector
}

func (a *feedDetectorAdapter) DetectFeedURL(ctx context.Context, inputURL string) (string, error) {
	return a.detector.DetectFeedURL(ctx, inputURL)
}

// --- SubscriptionLimitのエラーメッセージテスト ---

// TestSubscriptionLimitError はSubscriptionLimitErrorの内容をテストする。
func TestSubscriptionLimitError(t *testing.T) {
	err := model.NewSubscriptionLimitError()

	if err.Code != model.ErrCodeSubscriptionLimit {
		t.Errorf("Code = %q, want %q", err.Code, model.ErrCodeSubscriptionLimit)
	}
	if err.Category != "feed" {
		t.Errorf("Category = %q, want %q", err.Category, "feed")
	}
	if err.Action == "" {
		t.Error("Action should not be empty")
	}
	if err.Message == "" {
		t.Error("Message should not be empty")
	}
}

// TestDuplicateSubscriptionError はDuplicateSubscriptionErrorの内容をテストする。
func TestDuplicateSubscriptionError(t *testing.T) {
	err := model.NewDuplicateSubscriptionError()

	if err.Code != "DUPLICATE_SUBSCRIPTION" {
		t.Errorf("Code = %q, want %q", err.Code, "DUPLICATE_SUBSCRIPTION")
	}
	if err.Category != "feed" {
		t.Errorf("Category = %q, want %q", err.Category, "feed")
	}
}

// --- FeedService_RegisterFeed 追加境界値テスト ---

// TestFeedService_RegisterFeed_SubscriptionLimitExact100 は購読数がちょうど100の場合に拒否されることをテストする。
func TestFeedService_RegisterFeed_SubscriptionLimitExact100(t *testing.T) {
	feedRepo := newMockFeedRepo()
	subRepo := newMockSubRepo()
	subRepo.countByUser["user-1"] = 100

	detector := &mockDetector{feedURL: "https://example.com/feed.xml"}
	faviconFetcher := &mockFaviconFetcher{}

	svc := NewFeedService(feedRepo, subRepo, detector, faviconFetcher)

	_, _, err := svc.RegisterFeed(context.Background(), "user-1", "https://example.com")
	if err == nil {
		t.Fatal("購読数100の場合は拒否されるべき")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("APIError型が期待されるが、%T が返された", err)
	}
	if apiErr.Code != model.ErrCodeSubscriptionLimit {
		t.Errorf("エラーコード = %q, want %q", apiErr.Code, model.ErrCodeSubscriptionLimit)
	}
}

