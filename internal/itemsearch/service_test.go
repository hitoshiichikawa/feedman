package itemsearch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- テスト用モック ---

// recordedSearchCall は SearchByUserAndKeyword に渡された引数を記録する。
type recordedSearchCall struct {
	userID            string
	pattern           string
	feedID            *string
	cursorID          string
	cursorPublishedAt time.Time
	limit             int
}

// mockItemSearchRepo は ItemSearchRepository のテスト用モック。
type mockItemSearchRepo struct {
	// searchFn が非 nil の場合はそれを呼び出す。nil の場合は returnHits を返す。
	searchFn    func(ctx context.Context, userID, pattern string, feedID *string, cursorID string, cursorPublishedAt time.Time, limit int) ([]model.ItemSearchHit, error)
	returnHits  []model.ItemSearchHit
	returnErr   error
	calls       []recordedSearchCall
	callCount   int
}

func (m *mockItemSearchRepo) SearchByUserAndKeyword(
	ctx context.Context,
	userID, pattern string,
	feedID *string,
	cursorID string,
	cursorPublishedAt time.Time,
	limit int,
) ([]model.ItemSearchHit, error) {
	m.callCount++
	// feedID のスナップショットを取る（呼び出し元での書き換えから守る）
	var feedIDCopy *string
	if feedID != nil {
		v := *feedID
		feedIDCopy = &v
	}
	m.calls = append(m.calls, recordedSearchCall{
		userID:            userID,
		pattern:           pattern,
		feedID:            feedIDCopy,
		cursorID:          cursorID,
		cursorPublishedAt: cursorPublishedAt,
		limit:             limit,
	})
	if m.searchFn != nil {
		return m.searchFn(ctx, userID, pattern, feedID, cursorID, cursorPublishedAt, limit)
	}
	return m.returnHits, m.returnErr
}

// mockSubRepo は SubscriptionRepository のテスト用モック。
// 本テストでは FindByUserAndFeed のみを実装し、他メソッドは呼ばれない前提でパニックする。
type mockSubRepo struct {
	findFn        func(ctx context.Context, userID, feedID string) (*model.Subscription, error)
	findCallCount int
}

func (m *mockSubRepo) FindByID(_ context.Context, _ string) (*model.Subscription, error) {
	panic("mockSubRepo.FindByID: not implemented")
}

func (m *mockSubRepo) FindByUserAndFeed(ctx context.Context, userID, feedID string) (*model.Subscription, error) {
	m.findCallCount++
	if m.findFn != nil {
		return m.findFn(ctx, userID, feedID)
	}
	return nil, nil
}

func (m *mockSubRepo) CountByUserID(_ context.Context, _ string) (int, error) {
	panic("mockSubRepo.CountByUserID: not implemented")
}

func (m *mockSubRepo) Create(_ context.Context, _ *model.Subscription) error {
	panic("mockSubRepo.Create: not implemented")
}

func (m *mockSubRepo) ListByUserID(_ context.Context, _ string) ([]*model.Subscription, error) {
	panic("mockSubRepo.ListByUserID: not implemented")
}

func (m *mockSubRepo) MinFetchIntervalByFeedID(_ context.Context, _ string) (int, error) {
	panic("mockSubRepo.MinFetchIntervalByFeedID: not implemented")
}

func (m *mockSubRepo) UpdateFetchInterval(_ context.Context, _ string, _ int) error {
	panic("mockSubRepo.UpdateFetchInterval: not implemented")
}

func (m *mockSubRepo) Delete(_ context.Context, _ string) error {
	panic("mockSubRepo.Delete: not implemented")
}

func (m *mockSubRepo) DeleteByUserID(_ context.Context, _ string) error {
	panic("mockSubRepo.DeleteByUserID: not implemented")
}

func (m *mockSubRepo) ListByUserIDWithFeedInfo(_ context.Context, _ string) ([]repository.SubscriptionWithFeedInfo, error) {
	panic("mockSubRepo.ListByUserIDWithFeedInfo: not implemented")
}

// compile-time check: mockSubRepo は repository.SubscriptionRepository を満たす
var _ repository.SubscriptionRepository = (*mockSubRepo)(nil)

// --- escapeLikePattern のユニットテスト ---

func TestEscapeLikePattern(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no meta chars", "hello", "hello"},
		{"percent only", "50%off", `50\%off`},
		{"underscore only", "a_b", `a\_b`},
		{"backslash only", `a\b`, `a\\b`},
		{"all three", `100%_off\back`, `100\%\_off\\back`},
		{"backslash before percent (order)", `\%`, `\\\%`},
		{"empty", "", ""},
		{"only meta chars", `%_\`, `\%\_\\`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: tc.in
			// Act
			got := escapeLikePattern(tc.in)
			// Assert
			if got != tc.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// --- clampLimit のユニットテスト ---

func TestClampLimit(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero -> default", 0, defaultSearchLimit},
		{"negative -> default", -10, defaultSearchLimit},
		{"in range -> unchanged 1", 1, 1},
		{"in range -> unchanged 50", 50, 50},
		{"in range -> unchanged 200", 200, 200},
		{"over max -> clamped to max", 201, maxSearchLimit},
		{"way over max -> clamped to max", 10000, maxSearchLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clampLimit(tc.in)
			if got != tc.want {
				t.Errorf("clampLimit(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// --- parseCursor のユニットテスト ---

func TestParseCursor(t *testing.T) {
	validTs := "2026-05-28T12:34:56.123456789Z"
	validUUID := "11111111-2222-3333-4444-555555555555"

	t.Run("empty -> zero values, no error", func(t *testing.T) {
		gotTs, gotID, err := parseCursor("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !gotTs.IsZero() {
			t.Errorf("expected zero time, got %v", gotTs)
		}
		if gotID != "" {
			t.Errorf("expected empty id, got %q", gotID)
		}
	})

	t.Run("valid -> parsed tuple", func(t *testing.T) {
		gotTs, gotID, err := parseCursor(validTs + "|" + validUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedTs, _ := time.Parse(time.RFC3339Nano, validTs)
		if !gotTs.Equal(expectedTs) {
			t.Errorf("ts = %v, want %v", gotTs, expectedTs)
		}
		if gotID != validUUID {
			t.Errorf("id = %q, want %q", gotID, validUUID)
		}
	})

	invalidCases := []struct {
		name   string
		cursor string
	}{
		{"no pipe", "invalid"},
		{"three parts", "a|b|c"},
		{"invalid timestamp", "not-a-time|" + validUUID},
		{"empty timestamp", "|" + validUUID},
		{"empty id", validTs + "|"},
		{"only whitespace id", validTs + "|   "},
		{"completely bogus", "garbage"},
	}
	for _, tc := range invalidCases {
		t.Run("invalid_"+tc.name, func(t *testing.T) {
			_, _, err := parseCursor(tc.cursor)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.cursor)
			}
			var apiErr *model.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *model.APIError, got %T: %v", err, err)
			}
			if apiErr.Code != model.ErrCodeInvalidSearchQuery {
				t.Errorf("Code = %q, want %q", apiErr.Code, model.ErrCodeInvalidSearchQuery)
			}
		})
	}
}

// --- Search のテーブル駆動テスト ---

// makeHits は連続した PublishedAt を持つ ItemSearchHit を n 件生成するヘルパー。
// i 番目のヒットの PublishedAt は base - i*1s（降順並びの想定）。
func makeHits(base time.Time, n int) []model.ItemSearchHit {
	hits := make([]model.ItemSearchHit, n)
	for i := 0; i < n; i++ {
		hits[i] = model.ItemSearchHit{
			ID:          "item-" + string(rune('a'+i)),
			FeedID:      "feed-1",
			FeedTitle:   "Feed One",
			FaviconData: []byte{0xCA, 0xFE},
			FaviconMime: "image/png",
			Title:       "title-" + string(rune('a'+i)),
			Link:        "https://example.com/" + string(rune('a'+i)),
			Summary:     "summary",
			PublishedAt: base.Add(-time.Duration(i) * time.Second),
		}
	}
	return hits
}

// TestSearch_EmptyQuery: rawQuery が空文字または全空白なら、
// repository を呼ばずに空結果を返す（Req 1.5）。
func TestSearch_EmptyQuery(t *testing.T) {
	cases := []struct {
		name     string
		rawQuery string
	}{
		{"empty string", ""},
		{"single space", " "},
		{"multiple spaces", "    "},
		{"tabs and newlines", "\t\n  \t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			repo := &mockItemSearchRepo{}
			subRepo := &mockSubRepo{}
			svc := NewSearchService(repo, subRepo)
			// Act
			got, err := svc.Search(context.Background(), "user-1", tc.rawQuery, nil, "", 10)
			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("got nil result, want empty SearchResult")
			}
			if len(got.Items) != 0 {
				t.Errorf("expected empty Items, got %d", len(got.Items))
			}
			if got.HasMore {
				t.Errorf("HasMore = true, want false")
			}
			if got.NextCursor != "" {
				t.Errorf("NextCursor = %q, want empty", got.NextCursor)
			}
			if repo.callCount != 0 {
				t.Errorf("repository called %d times, want 0", repo.callCount)
			}
			if subRepo.findCallCount != 0 {
				t.Errorf("sub repository called %d times, want 0", subRepo.findCallCount)
			}
		})
	}
}

// TestSearch_LikeEscape: LIKE メタ文字（%, _, \）がエスケープされて repository に
// 渡されることを検証する（Req 2.4）。
func TestSearch_LikeEscape(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantPattern string
	}{
		{"plain", "hello", "%hello%"},
		{"percent", "50%off", `%50\%off%`},
		{"underscore", "a_b", `%a\_b%`},
		{"backslash", `back\slash`, `%back\\slash%`},
		{"mixed", `100%_off\x`, `%100\%\_off\\x%`},
		{"trim", "  hello  ", "%hello%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			repo := &mockItemSearchRepo{returnHits: nil}
			subRepo := &mockSubRepo{}
			svc := NewSearchService(repo, subRepo)
			// Act
			_, err := svc.Search(context.Background(), "user-1", tc.query, nil, "", 10)
			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.callCount != 1 {
				t.Fatalf("expected 1 repo call, got %d", repo.callCount)
			}
			if repo.calls[0].pattern != tc.wantPattern {
				t.Errorf("pattern = %q, want %q", repo.calls[0].pattern, tc.wantPattern)
			}
		})
	}
}

// TestSearch_InvalidCursor: cursor 形式不正で INVALID_SEARCH_QUERY APIError を返す。
func TestSearch_InvalidCursor(t *testing.T) {
	cases := []struct {
		name   string
		cursor string
	}{
		{"no pipe", "invalid-cursor"},
		{"too many pipes", "2026-05-28T12:00:00Z|uuid|extra"},
		{"bad timestamp", "not-a-time|11111111-2222-3333-4444-555555555555"},
		{"empty id", "2026-05-28T12:00:00Z|"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			repo := &mockItemSearchRepo{}
			subRepo := &mockSubRepo{}
			svc := NewSearchService(repo, subRepo)
			// Act
			_, err := svc.Search(context.Background(), "user-1", "hello", nil, tc.cursor, 10)
			// Assert
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var apiErr *model.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *model.APIError, got %T: %v", err, err)
			}
			if apiErr.Code != model.ErrCodeInvalidSearchQuery {
				t.Errorf("Code = %q, want %q", apiErr.Code, model.ErrCodeInvalidSearchQuery)
			}
			if repo.callCount != 0 {
				t.Errorf("repository called %d times, want 0", repo.callCount)
			}
		})
	}
}

// TestSearch_ValidCursor: 正常な cursor が repository に正しく渡される。
func TestSearch_ValidCursor(t *testing.T) {
	// Arrange
	repo := &mockItemSearchRepo{returnHits: nil}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	tsStr := "2026-05-28T12:34:56.123456789Z"
	uuidStr := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	// Act
	_, err := svc.Search(context.Background(), "user-1", "hello", nil, tsStr+"|"+uuidStr, 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.callCount != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.callCount)
	}
	call := repo.calls[0]
	wantTs, _ := time.Parse(time.RFC3339Nano, tsStr)
	if !call.cursorPublishedAt.Equal(wantTs) {
		t.Errorf("cursorPublishedAt = %v, want %v", call.cursorPublishedAt, wantTs)
	}
	if call.cursorID != uuidStr {
		t.Errorf("cursorID = %q, want %q", call.cursorID, uuidStr)
	}
}

// TestSearch_FeedIDNotSubscribed: feedID 非 nil かつ未購読で FEED_NOT_SUBSCRIBED が返り、
// item repository は呼ばれない (Req 3.5)。
func TestSearch_FeedIDNotSubscribed(t *testing.T) {
	// Arrange
	repo := &mockItemSearchRepo{}
	subRepo := &mockSubRepo{
		findFn: func(_ context.Context, _, _ string) (*model.Subscription, error) {
			return nil, nil // 未購読
		},
	}
	svc := NewSearchService(repo, subRepo)
	feedID := "feed-1"
	// Act
	_, err := svc.Search(context.Background(), "user-1", "hello", &feedID, "", 10)
	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *model.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != model.ErrCodeFeedNotSubscribed {
		t.Errorf("Code = %q, want %q", apiErr.Code, model.ErrCodeFeedNotSubscribed)
	}
	if repo.callCount != 0 {
		t.Errorf("item repository called %d times, want 0", repo.callCount)
	}
	if subRepo.findCallCount != 1 {
		t.Errorf("sub repository called %d times, want 1", subRepo.findCallCount)
	}
}

// TestSearch_FeedIDSubscribed: feedID 非 nil かつ購読あり で item repository が呼ばれ、
// feedID が引数に伝搬する。
func TestSearch_FeedIDSubscribed(t *testing.T) {
	// Arrange
	repo := &mockItemSearchRepo{returnHits: nil}
	subRepo := &mockSubRepo{
		findFn: func(_ context.Context, userID, feedID string) (*model.Subscription, error) {
			if userID != "user-1" {
				t.Errorf("userID = %q, want %q", userID, "user-1")
			}
			if feedID != "feed-1" {
				t.Errorf("feedID = %q, want %q", feedID, "feed-1")
			}
			return &model.Subscription{ID: "sub-1", UserID: userID, FeedID: feedID}, nil
		},
	}
	svc := NewSearchService(repo, subRepo)
	feedID := "feed-1"
	// Act
	_, err := svc.Search(context.Background(), "user-1", "hello", &feedID, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.callCount != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.callCount)
	}
	call := repo.calls[0]
	if call.feedID == nil {
		t.Fatal("expected feedID to be non-nil in repo call")
	}
	if *call.feedID != feedID {
		t.Errorf("feedID = %q, want %q", *call.feedID, feedID)
	}
}

// TestSearch_FeedIDNil_SubRepoNotCalled: feedID == nil のとき subscription repository は
// 呼ばれず、item repository に nil が伝搬する。
func TestSearch_FeedIDNil_SubRepoNotCalled(t *testing.T) {
	// Arrange
	repo := &mockItemSearchRepo{returnHits: nil}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	_, err := svc.Search(context.Background(), "user-1", "hello", nil, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subRepo.findCallCount != 0 {
		t.Errorf("sub repository called %d times, want 0", subRepo.findCallCount)
	}
	if repo.callCount != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.callCount)
	}
	if repo.calls[0].feedID != nil {
		t.Errorf("expected feedID nil in repo call, got %v", *repo.calls[0].feedID)
	}
}

// TestSearch_SubRepoError: SubscriptionRepository がエラーを返したら wrap されて返る。
func TestSearch_SubRepoError(t *testing.T) {
	// Arrange
	repo := &mockItemSearchRepo{}
	sentinel := errors.New("db down")
	subRepo := &mockSubRepo{
		findFn: func(_ context.Context, _, _ string) (*model.Subscription, error) {
			return nil, sentinel
		},
	}
	svc := NewSearchService(repo, subRepo)
	feedID := "feed-1"
	// Act
	_, err := svc.Search(context.Background(), "user-1", "hello", &feedID, "", 10)
	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "check subscription") {
		t.Errorf("expected wrap message containing 'check subscription', got %q", err.Error())
	}
	if repo.callCount != 0 {
		t.Errorf("item repository called %d times, want 0", repo.callCount)
	}
}

// TestSearch_LimitClamp: limit のクランプ動作。
//
// 0 以下なら defaultSearchLimit、上限超なら maxSearchLimit。さらに repository には
// (effectiveLimit + 1) が渡される（HasMore 判定のため）。
func TestSearch_LimitClamp(t *testing.T) {
	cases := []struct {
		name              string
		limit             int
		wantRepoLimit     int
	}{
		{"zero -> default+1", 0, defaultSearchLimit + 1},
		{"negative -> default+1", -5, defaultSearchLimit + 1},
		{"in range -> +1", 25, 26},
		{"at max -> +1", 200, 201},
		{"over max -> max+1", 500, maxSearchLimit + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			repo := &mockItemSearchRepo{returnHits: nil}
			subRepo := &mockSubRepo{}
			svc := NewSearchService(repo, subRepo)
			// Act
			_, err := svc.Search(context.Background(), "user-1", "hello", nil, "", tc.limit)
			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.callCount != 1 {
				t.Fatalf("expected 1 repo call, got %d", repo.callCount)
			}
			if repo.calls[0].limit != tc.wantRepoLimit {
				t.Errorf("repo.limit = %d, want %d", repo.calls[0].limit, tc.wantRepoLimit)
			}
		})
	}
}

// TestSearch_HasMoreTrue: repository が limit+1 件を返したら HasMore=true、
// 結果が limit 件に切り詰められ、NextCursor が組み立てられる。
func TestSearch_HasMoreTrue(t *testing.T) {
	// Arrange
	base := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	repo := &mockItemSearchRepo{returnHits: makeHits(base, 11)} // limit=10 + 1
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	got, err := svc.Search(context.Background(), "user-1", "hello", nil, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 10 {
		t.Errorf("len(Items) = %d, want 10", len(got.Items))
	}
	if !got.HasMore {
		t.Errorf("HasMore = false, want true")
	}
	if got.NextCursor == "" {
		t.Errorf("NextCursor is empty, want non-empty")
	}
	// NextCursor の形式: <RFC3339Nano>|<id>
	parts := strings.SplitN(got.NextCursor, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("NextCursor format invalid: %q", got.NextCursor)
	}
	wantTs := base.Add(-9 * time.Second).UTC().Format(time.RFC3339Nano)
	if parts[0] != wantTs {
		t.Errorf("NextCursor ts = %q, want %q", parts[0], wantTs)
	}
	wantID := "item-" + string(rune('a'+9))
	if parts[1] != wantID {
		t.Errorf("NextCursor id = %q, want %q", parts[1], wantID)
	}
}

// TestSearch_HasMoreFalse: repository が limit 以下を返したら HasMore=false、NextCursor=""。
func TestSearch_HasMoreFalse(t *testing.T) {
	// Arrange
	base := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	repo := &mockItemSearchRepo{returnHits: makeHits(base, 3)}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	got, err := svc.Search(context.Background(), "user-1", "hello", nil, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(got.Items))
	}
	if got.HasMore {
		t.Errorf("HasMore = true, want false")
	}
	if got.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty", got.NextCursor)
	}
}

// TestSearch_HasMoreFalse_ExactlyLimit: repository が limit 件ジャストを返したら
// HasMore=false（次ページなし）。
func TestSearch_HasMoreFalse_ExactlyLimit(t *testing.T) {
	// Arrange
	base := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	repo := &mockItemSearchRepo{returnHits: makeHits(base, 10)} // limit=10 にぴったり
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	got, err := svc.Search(context.Background(), "user-1", "hello", nil, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 10 {
		t.Errorf("len(Items) = %d, want 10", len(got.Items))
	}
	if got.HasMore {
		t.Errorf("HasMore = true, want false")
	}
	if got.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty", got.NextCursor)
	}
}

// TestSearch_HasMoreTrue_ZeroPublishedAt: HasMore=true でも末尾項目の PublishedAt が
// ゼロ値なら NextCursor は空文字（並びの安定性が保てないため）。
func TestSearch_HasMoreTrue_ZeroPublishedAt(t *testing.T) {
	// Arrange
	hits := []model.ItemSearchHit{
		{ID: "item-1", PublishedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)},
		{ID: "item-2", PublishedAt: time.Time{}}, // 末尾がゼロ
		{ID: "item-3", PublishedAt: time.Time{}}, // 余分（HasMore=true 誘発）
	}
	repo := &mockItemSearchRepo{returnHits: hits}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	got, err := svc.Search(context.Background(), "user-1", "hello", nil, "", 2)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.HasMore {
		t.Errorf("HasMore = false, want true")
	}
	if got.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty (zero PublishedAt at tail)", got.NextCursor)
	}
	if len(got.Items) != 2 {
		t.Errorf("len(Items) = %d, want 2", len(got.Items))
	}
}

// TestSearch_RepoError: repository がエラーを返したらそのまま返る（wrap しない）。
func TestSearch_RepoError(t *testing.T) {
	// Arrange
	sentinel := errors.New("db lookup failed")
	repo := &mockItemSearchRepo{returnErr: sentinel}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	_, err := svc.Search(context.Background(), "user-1", "hello", nil, "", 10)
	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel, got %v", err)
	}
}

// TestSearch_HitsConvertedToSummaries: ItemSearchHit の各フィールドが ItemSearchSummary に
// 正しくマッピングされる。FaviconURL は nil（Adapter 層の責務）であり、FaviconData /
// FaviconMime が pass-through される。
func TestSearch_HitsConvertedToSummaries(t *testing.T) {
	// Arrange
	publishedAt := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	hit := model.ItemSearchHit{
		ID:              "item-x",
		FeedID:          "feed-x",
		FeedTitle:       "Feed X",
		FaviconData:     []byte{0x01, 0x02, 0x03},
		FaviconMime:     "image/png",
		Title:           "Sample Title",
		Link:            "https://example.com/x",
		Summary:         "Sample Summary",
		PublishedAt:     publishedAt,
		IsDateEstimated: true,
		IsRead:          true,
		IsStarred:       true,
		HatebuCount:     42,
	}
	repo := &mockItemSearchRepo{returnHits: []model.ItemSearchHit{hit}}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	got, err := svc.Search(context.Background(), "user-1", "sample", nil, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(got.Items))
	}
	want := ItemSearchSummary{
		ID:              "item-x",
		FeedID:          "feed-x",
		FeedTitle:       "Feed X",
		FaviconURL:      nil,
		FaviconData:     []byte{0x01, 0x02, 0x03},
		FaviconMime:     "image/png",
		Title:           "Sample Title",
		Link:            "https://example.com/x",
		Summary:         "Sample Summary",
		PublishedAt:     publishedAt,
		IsDateEstimated: true,
		IsRead:          true,
		IsStarred:       true,
		HatebuCount:     42,
	}
	if !reflect.DeepEqual(got.Items[0], want) {
		t.Errorf("Items[0] = %+v, want %+v", got.Items[0], want)
	}
}

// TestSearch_UserIDPropagated: userID が repository に伝搬する。
func TestSearch_UserIDPropagated(t *testing.T) {
	// Arrange
	repo := &mockItemSearchRepo{returnHits: nil}
	subRepo := &mockSubRepo{}
	svc := NewSearchService(repo, subRepo)
	// Act
	_, err := svc.Search(context.Background(), "user-xyz", "hello", nil, "", 10)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.calls[0].userID != "user-xyz" {
		t.Errorf("userID = %q, want %q", repo.calls[0].userID, "user-xyz")
	}
}
