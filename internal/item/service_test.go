package item

import (
	"context"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- テスト用モック（サービス層用） ---

// mockItemRepoForService はサービステスト用のItemRepositoryモック。
type mockItemRepoForService struct {
	*mockItemRepo
	listByFeedFn        func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error)
	listStarredByUserFn func(ctx context.Context, userID string, cursor time.Time, limit int) ([]repository.StarredItemRow, error)
	findByIDFn          func(ctx context.Context, id string) (*model.Item, error)
}

func newMockItemRepoForService() *mockItemRepoForService {
	return &mockItemRepoForService{
		mockItemRepo: newMockItemRepo(),
	}
}

func (m *mockItemRepoForService) ListByFeed(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
	if m.listByFeedFn != nil {
		return m.listByFeedFn(ctx, feedID, userID, filter, cursor, limit)
	}
	return nil, nil
}

func (m *mockItemRepoForService) ListStarredByUser(ctx context.Context, userID string, cursor time.Time, limit int) ([]repository.StarredItemRow, error) {
	if m.listStarredByUserFn != nil {
		return m.listStarredByUserFn(ctx, userID, cursor, limit)
	}
	return nil, nil
}

func (m *mockItemRepoForService) FindByID(ctx context.Context, id string) (*model.Item, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return m.mockItemRepo.FindByID(ctx, id)
}

// mockItemStateRepoForService はサービステスト用のItemStateRepositoryモック。
type mockItemStateRepoForService struct {
	states   map[string]*model.ItemState // userID+itemID -> state
	upsertFn func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error)
}

func newMockItemStateRepoForService() *mockItemStateRepoForService {
	return &mockItemStateRepoForService{
		states: make(map[string]*model.ItemState),
	}
}

func (m *mockItemStateRepoForService) FindByUserAndItem(_ context.Context, userID, itemID string) (*model.ItemState, error) {
	key := userID + "|" + itemID
	state, ok := m.states[key]
	if !ok {
		return nil, nil
	}
	return state, nil
}

func (m *mockItemStateRepoForService) Upsert(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, userID, itemID, isRead, isStarred)
	}
	return nil, nil
}

func (m *mockItemStateRepoForService) DeleteByUserAndFeed(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockItemStateRepoForService) DeleteByUserID(_ context.Context, _ string) error {
	return nil
}

// --- ItemService ListItems テスト ---

// TestItemService_ListItems_ReturnsItems はフィードの記事一覧がpublished_at降順で返されることをテストする。
func TestItemService_ListItems_ReturnsItems(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		if feedID != "feed-1" {
			t.Errorf("feedID = %q, want %q", feedID, "feed-1")
		}
		if userID != "user-123" {
			t.Errorf("userID = %q, want %q", userID, "user-123")
		}
		if filter != model.ItemFilterAll {
			t.Errorf("filter = %q, want %q", filter, model.ItemFilterAll)
		}
		if limit != 51 {
			// limit+1で取得して、HasMoreを判定する
			t.Errorf("limit = %d, want 51 (limit+1)", limit)
		}
		return []model.ItemWithState{
			{
				Item: model.Item{
					ID:          "item-1",
					FeedID:      "feed-1",
					Title:       "記事1",
					Link:        "https://example.com/1",
					PublishedAt: &now,
					HatebuCount: 5,
				},
				IsRead:    false,
				IsStarred: true,
			},
		}, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	result, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Errorf("items count = %d, want 1", len(result.Items))
	}
	if result.HasMore {
		t.Error("expected HasMore to be false (1 item returned, limit was 50)")
	}

	item := result.Items[0]
	if item.ID != "item-1" {
		t.Errorf("item.ID = %q, want %q", item.ID, "item-1")
	}
	if item.IsStarred != true {
		t.Error("expected item to be starred")
	}
}

// TestItemService_ListItems_IncludesSummary は記事一覧のサマリーに概要(Summary)が
// 含まれること、および空概要が空文字列として保持されることを検証する。Req 1.1 / 1.3 に対応。
func TestItemService_ListItems_IncludesSummary(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	cases := []struct {
		name        string
		summary     string
		wantSummary string
	}{
		{name: "概要が存在するとき概要を保持する", summary: "記事の概要テキスト", wantSummary: "記事の概要テキスト"},
		{name: "概要が空のとき空文字列を保持する", summary: "", wantSummary: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			repo := newMockItemRepoForService()
			repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
				return []model.ItemWithState{
					{
						Item: model.Item{
							ID:          "item-1",
							FeedID:      "feed-1",
							Title:       "記事1",
							Link:        "https://example.com/1",
							Summary:     tc.summary,
							PublishedAt: &now,
						},
					},
				}, nil
			}
			svc := NewItemService(repo, newMockItemStateRepoForService())

			// Act
			result, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)

			// Assert
			if err != nil {
				t.Fatalf("ListItems returned error: %v", err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("items count = %d, want 1", len(result.Items))
			}
			if result.Items[0].Summary != tc.wantSummary {
				t.Errorf("Summary = %q, want %q", result.Items[0].Summary, tc.wantSummary)
			}
		})
	}
}

// TestItemService_SummaryConsistentBetweenListAndDetail は同一記事の概要が
// 一覧(ListItems)と詳細(GetItem)で同一の文字列値になることを検証する。Req 1.2 に対応。
func TestItemService_SummaryConsistentBetweenListAndDetail(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	const wantSummary = "一覧と詳細で一致する概要"

	srcItem := model.Item{
		ID:          "item-1",
		FeedID:      "feed-1",
		Title:       "記事1",
		Link:        "https://example.com/1",
		Summary:     wantSummary,
		Content:     "<p>本文</p>",
		PublishedAt: &now,
	}

	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		return []model.ItemWithState{{Item: srcItem}}, nil
	}
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		itemCopy := srcItem
		return &itemCopy, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())

	listResult, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	detail, err := svc.GetItem(context.Background(), "user-123", "item-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}

	if len(listResult.Items) != 1 {
		t.Fatalf("items count = %d, want 1", len(listResult.Items))
	}
	if listResult.Items[0].Summary != wantSummary {
		t.Errorf("list Summary = %q, want %q", listResult.Items[0].Summary, wantSummary)
	}
	if detail.Summary != wantSummary {
		t.Errorf("detail Summary = %q, want %q", detail.Summary, wantSummary)
	}
	if listResult.Items[0].Summary != detail.Summary {
		t.Errorf("list Summary %q != detail Summary %q", listResult.Items[0].Summary, detail.Summary)
	}
}

// TestItemService_ListItems_HasMore は50件超の結果でHasMore=trueが返されることをテストする。
func TestItemService_ListItems_HasMore(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		// limit+1件（51件）を返してHasMoreを検証
		items := make([]model.ItemWithState, limit)
		for i := 0; i < limit; i++ {
			pubTime := now.Add(-time.Duration(i) * time.Hour)
			items[i] = model.ItemWithState{
				Item: model.Item{
					ID:          "item-" + string(rune('A'+i)),
					FeedID:      "feed-1",
					Title:       "記事",
					PublishedAt: &pubTime,
				},
			}
		}
		return items, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	result, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if !result.HasMore {
		t.Error("expected HasMore to be true (51 items returned)")
	}

	// 実際に返される記事数はlimit（50件）
	if len(result.Items) != 50 {
		t.Errorf("items count = %d, want 50", len(result.Items))
	}

	if result.NextCursor == "" {
		t.Error("expected NextCursor to be set when HasMore is true")
	}
}

// TestItemService_ListItems_InvalidFilter は無効なフィルタでエラーが返されることをテストする。
func TestItemService_ListItems_InvalidFilter(t *testing.T) {
	repo := newMockItemRepoForService()
	svc := NewItemService(repo, newMockItemStateRepoForService())

	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilter("invalid"), "", 50)
	if err == nil {
		t.Fatal("expected error for invalid filter")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeInvalidFilter {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeInvalidFilter)
	}
}

// TestItemService_ListItems_CursorParsing はカーソル文字列が正しくパースされることをテストする。
func TestItemService_ListItems_CursorParsing(t *testing.T) {
	var receivedCursor time.Time
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedCursor = cursor
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	cursorStr := "2026-02-27T10:00:00Z"
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, cursorStr, 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	expectedCursor, _ := time.Parse(time.RFC3339, cursorStr)
	if !receivedCursor.Equal(expectedCursor) {
		t.Errorf("cursor = %v, want %v", receivedCursor, expectedCursor)
	}
}

// TestItemService_ListItems_EmptyCursor は空カーソルでゼロ値が渡されることをテストする。
func TestItemService_ListItems_EmptyCursor(t *testing.T) {
	var receivedCursor time.Time
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedCursor = cursor
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterAll, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if !receivedCursor.IsZero() {
		t.Errorf("expected zero cursor, got %v", receivedCursor)
	}
}

// TestItemService_ListItems_UnreadFilter は未読フィルタがリポジトリに渡されることをテストする。
func TestItemService_ListItems_UnreadFilter(t *testing.T) {
	var receivedFilter model.ItemFilter
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedFilter = filter
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterUnread, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if receivedFilter != model.ItemFilterUnread {
		t.Errorf("filter = %q, want %q", receivedFilter, model.ItemFilterUnread)
	}
}

// TestItemService_ListItems_StarredFilter はスターフィルタがリポジトリに渡されることをテストする。
func TestItemService_ListItems_StarredFilter(t *testing.T) {
	var receivedFilter model.ItemFilter
	repo := newMockItemRepoForService()
	repo.listByFeedFn = func(ctx context.Context, feedID, userID string, filter model.ItemFilter, cursor time.Time, limit int) ([]model.ItemWithState, error) {
		receivedFilter = filter
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.ListItems(context.Background(), "user-123", "feed-1", model.ItemFilterStarred, "", 50)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}

	if receivedFilter != model.ItemFilterStarred {
		t.Errorf("filter = %q, want %q", receivedFilter, model.ItemFilterStarred)
	}
}

// --- ItemService ListStarredItems テスト ---

// makeStarredRow はテスト用の StarredItemRow を組み立てるヘルパ。
func makeStarredRow(id, feedID, feedTitle string, pubAt time.Time) repository.StarredItemRow {
	pubCopy := pubAt
	return repository.StarredItemRow{
		ItemWithState: model.ItemWithState{
			Item: model.Item{
				ID:          id,
				FeedID:      feedID,
				Title:       "starred-" + id,
				Link:        "https://example.com/" + id,
				PublishedAt: &pubCopy,
			},
			IsRead:    false,
			IsStarred: true,
		},
		FeedTitle: feedTitle,
	}
}

// TestItemService_ListStarredItems_EmptyCursor は空カーソルで先頭ページが取得され、
// fetchLimit が limit+1 で repository に渡ることを検証する。
// 対応 AC: Req 4.4（cursor なしで先頭ページ）
func TestItemService_ListStarredItems_EmptyCursor(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	var receivedCursor time.Time
	var receivedLimit int
	var receivedUserID string
	repo := newMockItemRepoForService()
	repo.listStarredByUserFn = func(_ context.Context, userID string, cursor time.Time, limit int) ([]repository.StarredItemRow, error) {
		receivedCursor = cursor
		receivedLimit = limit
		receivedUserID = userID
		return []repository.StarredItemRow{
			makeStarredRow("item-1", "feed-1", "Feed A", now),
		}, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())

	// Act
	result, err := svc.ListStarredItems(context.Background(), "user-123", "", 50)

	// Assert
	if err != nil {
		t.Fatalf("ListStarredItems returned error: %v", err)
	}
	if !receivedCursor.IsZero() {
		t.Errorf("expected zero cursor for empty cursorStr, got %v", receivedCursor)
	}
	if receivedLimit != 51 {
		t.Errorf("limit = %d, want 51 (limit+1)", receivedLimit)
	}
	if receivedUserID != "user-123" {
		t.Errorf("userID = %q, want %q", receivedUserID, "user-123")
	}
	if len(result.Items) != 1 {
		t.Fatalf("items count = %d, want 1", len(result.Items))
	}
	if result.HasMore {
		t.Error("expected HasMore=false when only 1 item returned")
	}
	if result.NextCursor != "" {
		t.Errorf("expected empty NextCursor when HasMore=false, got %q", result.NextCursor)
	}
	if result.Items[0].FeedTitle != "Feed A" {
		t.Errorf("FeedTitle = %q, want %q", result.Items[0].FeedTitle, "Feed A")
	}
	if result.Items[0].ID != "item-1" {
		t.Errorf("ID = %q, want %q", result.Items[0].ID, "item-1")
	}
}

// TestItemService_ListStarredItems_InvalidCursor は不正なカーソル文字列で
// INVALID_FILTER エラーが返されることを検証する。
// 対応 AC: Req 4.8（不正カーソルで既存と同等の 400 相当）
func TestItemService_ListStarredItems_InvalidCursor(t *testing.T) {
	// Arrange
	repo := newMockItemRepoForService()
	repoCalled := false
	repo.listStarredByUserFn = func(_ context.Context, _ string, _ time.Time, _ int) ([]repository.StarredItemRow, error) {
		repoCalled = true
		return nil, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())

	// Act
	_, err := svc.ListStarredItems(context.Background(), "user-123", "not-a-timestamp", 50)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeInvalidFilter {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeInvalidFilter)
	}
	if repoCalled {
		t.Error("repository should not be called when cursor parse fails")
	}
}

// TestItemService_ListStarredItems_HasMoreTrue は limit+1 件を取得したとき
// HasMore=true となり末尾の余分な 1 件が切り詰められ、NextCursor が
// RFC3339Nano フォーマットで設定されることを検証する。
// 対応 AC: Req 4.3（has_more / next_cursor を含む）、Req 4.5（next_cursor で継続）、NFR 3.1
func TestItemService_ListStarredItems_HasMoreTrue(t *testing.T) {
	// Arrange
	base := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newMockItemRepoForService()
	repo.listStarredByUserFn = func(_ context.Context, _ string, _ time.Time, limit int) ([]repository.StarredItemRow, error) {
		// limit+1 件（51 件）返却して HasMore を発火させる
		rows := make([]repository.StarredItemRow, limit)
		for i := 0; i < limit; i++ {
			pubAt := base.Add(-time.Duration(i) * time.Hour)
			rows[i] = makeStarredRow(
				"item-"+string(rune('A'+i%26)),
				"feed-1",
				"Feed A",
				pubAt,
			)
		}
		return rows, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())

	// Act
	result, err := svc.ListStarredItems(context.Background(), "user-123", "", 50)

	// Assert
	if err != nil {
		t.Fatalf("ListStarredItems returned error: %v", err)
	}
	if !result.HasMore {
		t.Error("expected HasMore=true when limit+1 rows returned")
	}
	if len(result.Items) != 50 {
		t.Errorf("items count = %d, want 50 (truncated from limit+1)", len(result.Items))
	}
	if result.NextCursor == "" {
		t.Fatal("expected NextCursor to be set when HasMore=true")
	}
	// NextCursor は最後尾（index 49）の PublishedAt を RFC3339Nano でフォーマットしたもの
	wantCursor := base.Add(-49 * time.Hour).Format(time.RFC3339Nano)
	if result.NextCursor != wantCursor {
		t.Errorf("NextCursor = %q, want %q", result.NextCursor, wantCursor)
	}
	// RFC3339Nano としてパースできること
	if _, perr := time.Parse(time.RFC3339Nano, result.NextCursor); perr != nil {
		t.Errorf("NextCursor %q is not valid RFC3339Nano: %v", result.NextCursor, perr)
	}
}

// TestItemService_ListStarredItems_HasMoreFalse は limit 以下の件数で
// HasMore=false かつ NextCursor が空文字列となることを検証する。
// 対応 AC: Req 4.7（スター 0 件で空 + has_more=false）の境界補完、Req 4.3 の has_more=false 形
func TestItemService_ListStarredItems_HasMoreFalse(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.listStarredByUserFn = func(_ context.Context, _ string, _ time.Time, _ int) ([]repository.StarredItemRow, error) {
		return []repository.StarredItemRow{
			makeStarredRow("item-1", "feed-1", "Feed A", now),
			makeStarredRow("item-2", "feed-2", "Feed B", now.Add(-time.Hour)),
		}, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())

	// Act
	result, err := svc.ListStarredItems(context.Background(), "user-123", "", 50)

	// Assert
	if err != nil {
		t.Fatalf("ListStarredItems returned error: %v", err)
	}
	if result.HasMore {
		t.Error("expected HasMore=false when fewer than limit+1 rows returned")
	}
	if result.NextCursor != "" {
		t.Errorf("expected empty NextCursor when HasMore=false, got %q", result.NextCursor)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items count = %d, want 2", len(result.Items))
	}
	// 複数フィードにまたがる FeedTitle が保持されている
	if result.Items[0].FeedTitle != "Feed A" {
		t.Errorf("Items[0].FeedTitle = %q, want %q", result.Items[0].FeedTitle, "Feed A")
	}
	if result.Items[1].FeedTitle != "Feed B" {
		t.Errorf("Items[1].FeedTitle = %q, want %q", result.Items[1].FeedTitle, "Feed B")
	}
}

// TestItemService_ListStarredItems_NextCursorRFC3339NanoFormat は NextCursor が
// RFC3339Nano フォーマットで返されることを精密に検証する（nanosecond 精度を含む）。
// 対応 AC: Req 4.5 の cursor 送り規約一貫性、NFR 3.1（既存 API と区別不能）
func TestItemService_ListStarredItems_NextCursorRFC3339NanoFormat(t *testing.T) {
	// Arrange: nanosecond 精度を含む時刻を、保持される末尾（外部 limit=50 → index 49）に置く
	const outerLimit = 50
	tailTime := time.Date(2026, 5, 29, 12, 34, 56, 123456789, time.UTC)
	repo := newMockItemRepoForService()
	repo.listStarredByUserFn = func(_ context.Context, _ string, _ time.Time, limit int) ([]repository.StarredItemRow, error) {
		// fetchLimit (=outerLimit+1) 件返却して HasMore=true を発火させる。
		// インデックス outerLimit-1 (=49) が truncate 後の末尾になり、ここに tailTime を置く。
		// それ以前のインデックスは tailTime より後の時刻（公開日時降順を維持）。
		// インデックス outerLimit (=50) は HasMore で切り捨てられる余分行。
		rows := make([]repository.StarredItemRow, limit)
		for i := 0; i < outerLimit-1; i++ {
			pubAt := tailTime.Add(time.Duration(outerLimit-i) * time.Hour)
			rows[i] = makeStarredRow("item-"+string(rune('A'+i%26)), "feed-1", "Feed A", pubAt)
		}
		// 保持される末尾（HasMore truncate 後の最終要素）
		rows[outerLimit-1] = makeStarredRow("item-tail", "feed-1", "Feed A", tailTime)
		// 切り捨てられる余分行（tailTime より過去の時刻）
		rows[outerLimit] = makeStarredRow("item-overflow", "feed-1", "Feed A", tailTime.Add(-time.Hour))
		return rows, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())

	// Act
	result, err := svc.ListStarredItems(context.Background(), "user-123", "", outerLimit)

	// Assert
	if err != nil {
		t.Fatalf("ListStarredItems returned error: %v", err)
	}
	if !result.HasMore {
		t.Fatal("expected HasMore=true")
	}
	wantCursor := tailTime.Format(time.RFC3339Nano)
	if result.NextCursor != wantCursor {
		t.Errorf("NextCursor = %q, want %q (RFC3339Nano)", result.NextCursor, wantCursor)
	}
	// 既存 ListItems と完全同一フォーマットで生成されることを再確認（NFR 3.1）
	parsed, perr := time.Parse(time.RFC3339Nano, result.NextCursor)
	if perr != nil {
		t.Fatalf("NextCursor %q failed RFC3339Nano parse: %v", result.NextCursor, perr)
	}
	if !parsed.Equal(tailTime) {
		t.Errorf("parsed cursor = %v, want %v", parsed, tailTime)
	}
}

// TestItemService_ListStarredItems_CursorPassedToRepo は受け取った cursorStr が
// time.Time にパースされて repository 層にそのまま伝搬することを検証する。
// 対応 AC: Req 4.5（next_cursor を渡したとき後続ページを返す）
func TestItemService_ListStarredItems_CursorPassedToRepo(t *testing.T) {
	// Arrange
	var receivedCursor time.Time
	repo := newMockItemRepoForService()
	repo.listStarredByUserFn = func(_ context.Context, _ string, cursor time.Time, _ int) ([]repository.StarredItemRow, error) {
		receivedCursor = cursor
		return nil, nil
	}
	svc := NewItemService(repo, newMockItemStateRepoForService())
	cursorStr := "2026-02-27T10:00:00Z"

	// Act
	_, err := svc.ListStarredItems(context.Background(), "user-123", cursorStr, 50)

	// Assert
	if err != nil {
		t.Fatalf("ListStarredItems returned error: %v", err)
	}
	expected, _ := time.Parse(time.RFC3339, cursorStr)
	if !receivedCursor.Equal(expected) {
		t.Errorf("cursor passed to repo = %v, want %v", receivedCursor, expected)
	}
}

// --- ItemService GetItem テスト ---

// TestItemService_GetItem_ReturnsDetail は記事詳細が正しく取得されることをテストする。
func TestItemService_GetItem_ReturnsDetail(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		if id != "item-1" {
			t.Errorf("id = %q, want %q", id, "item-1")
		}
		return &model.Item{
			ID:              "item-1",
			FeedID:          "feed-1",
			Title:           "テスト記事",
			Link:            "https://example.com/article",
			Content:         "<p>サニタイズ済み</p>",
			Summary:         "サマリー",
			Author:          "著者",
			PublishedAt:     &now,
			IsDateEstimated: false,
			HatebuCount:     5,
		}, nil
	}

	stateRepo := newMockItemStateRepoForService()
	stateRepo.states["user-123|item-1"] = &model.ItemState{
		UserID:    "user-123",
		ItemID:    "item-1",
		IsRead:    true,
		IsStarred: true,
	}

	svc := NewItemService(repo, stateRepo)
	detail, err := svc.GetItem(context.Background(), "user-123", "item-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}

	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if detail.ID != "item-1" {
		t.Errorf("detail.ID = %q, want %q", detail.ID, "item-1")
	}
	if detail.Content != "<p>サニタイズ済み</p>" {
		t.Errorf("detail.Content = %q, want sanitized content", detail.Content)
	}
	if detail.Link != "https://example.com/article" {
		t.Errorf("detail.Link = %q, want %q", detail.Link, "https://example.com/article")
	}
	if detail.Author != "著者" {
		t.Errorf("detail.Author = %q, want %q", detail.Author, "著者")
	}
	if !detail.IsRead {
		t.Error("expected detail.IsRead to be true")
	}
	if !detail.IsStarred {
		t.Error("expected detail.IsStarred to be true")
	}
}

// TestItemService_GetItem_NotFound は存在しない記事でエラーが返されることをテストする。
func TestItemService_GetItem_NotFound(t *testing.T) {
	repo := newMockItemRepoForService()
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return nil, nil
	}

	svc := NewItemService(repo, newMockItemStateRepoForService())
	_, err := svc.GetItem(context.Background(), "user-123", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent item")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeItemNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeItemNotFound)
	}
}

// TestItemService_GetItem_NoState は記事状態が未設定の場合にデフォルト値が返されることをテストする。
func TestItemService_GetItem_NoState(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockItemRepoForService()
	repo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{
			ID:          "item-1",
			FeedID:      "feed-1",
			Title:       "記事",
			PublishedAt: &now,
		}, nil
	}

	// item_statesにレコードなし
	svc := NewItemService(repo, newMockItemStateRepoForService())
	detail, err := svc.GetItem(context.Background(), "user-123", "item-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}

	// 状態未設定の場合、デフォルトで未読・未スター
	if detail.IsRead {
		t.Error("expected detail.IsRead to be false (no state)")
	}
	if detail.IsStarred {
		t.Error("expected detail.IsStarred to be false (no state)")
	}
}

// --- ItemStateService テスト ---

// TestItemStateService_UpdateState_SetRead は既読状態の設定をテストする。
func TestItemStateService_UpdateState_SetRead(t *testing.T) {
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		if userID != "user-123" {
			t.Errorf("userID = %q, want %q", userID, "user-123")
		}
		if itemID != "item-1" {
			t.Errorf("itemID = %q, want %q", itemID, "item-1")
		}
		if isRead == nil || !*isRead {
			t.Error("expected isRead to be true")
		}
		if isStarred != nil {
			t.Error("expected isStarred to be nil")
		}
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    true,
			IsStarred: false,
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isRead := true
	state, err := svc.UpdateState(context.Background(), "user-123", "item-1", &isRead, nil)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	if !state.IsRead {
		t.Error("expected state.IsRead to be true")
	}
}

// TestItemStateService_UpdateState_SetStarred はスター状態の設定をテストする。
func TestItemStateService_UpdateState_SetStarred(t *testing.T) {
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		if isStarred == nil || !*isStarred {
			t.Error("expected isStarred to be true")
		}
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    false,
			IsStarred: true,
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isStarred := true
	state, err := svc.UpdateState(context.Background(), "user-123", "item-1", nil, &isStarred)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	if !state.IsStarred {
		t.Error("expected state.IsStarred to be true")
	}
}

// TestItemStateService_UpdateState_NilFieldsNotChanged はnilフィールドが変更されないことをテストする。
func TestItemStateService_UpdateState_NilFieldsNotChanged(t *testing.T) {
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		// isReadのみ指定されている
		if isRead == nil {
			t.Error("expected isRead to be non-nil")
		}
		// isStarredはnil（変更しない）
		if isStarred != nil {
			t.Error("expected isStarred to be nil (not changed)")
		}
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    *isRead,
			IsStarred: true, // 既存の値を維持
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isRead := false
	state, err := svc.UpdateState(context.Background(), "user-123", "item-1", &isRead, nil)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	if state.IsRead {
		t.Error("expected state.IsRead to be false")
	}
	if !state.IsStarred {
		t.Error("expected state.IsStarred to remain true (unchanged)")
	}
}

// TestItemStateService_UpdateState_ItemNotFound は存在しない記事でエラーが返されることをテストする。
func TestItemStateService_UpdateState_ItemNotFound(t *testing.T) {
	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return nil, nil // 記事が存在しない
	}

	svc := NewItemStateService(itemRepo, newMockItemStateRepoForService())
	isRead := true
	_, err := svc.UpdateState(context.Background(), "user-123", "nonexistent", &isRead, nil)
	if err == nil {
		t.Fatal("expected error for non-existent item")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("expected *model.APIError, got %T", err)
	}
	if apiErr.Code != model.ErrCodeItemNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeItemNotFound)
	}
}

// TestItemStateService_UpdateState_UserDataIsolation はユーザーデータ分離が強制されることをテストする。
func TestItemStateService_UpdateState_UserDataIsolation(t *testing.T) {
	receivedUserID := ""
	stateRepo := newMockItemStateRepoForService()
	stateRepo.upsertFn = func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
		receivedUserID = userID
		return &model.ItemState{
			UserID:    userID,
			ItemID:    itemID,
			IsRead:    true,
			IsStarred: false,
		}, nil
	}

	itemRepo := newMockItemRepoForService()
	itemRepo.findByIDFn = func(ctx context.Context, id string) (*model.Item, error) {
		return &model.Item{ID: "item-1"}, nil
	}

	svc := NewItemStateService(itemRepo, stateRepo)
	isRead := true
	_, err := svc.UpdateState(context.Background(), "user-456", "item-1", &isRead, nil)
	if err != nil {
		t.Fatalf("UpdateState returned error: %v", err)
	}

	// user_idが正しくリポジトリに渡されることを検証
	if receivedUserID != "user-456" {
		t.Errorf("received userID = %q, want %q", receivedUserID, "user-456")
	}
}
