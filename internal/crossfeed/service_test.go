package crossfeed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- テスト用モック ---

// mockItemRepo は ItemRepository のうち本テストで使う ListNewAcrossFeeds のみを
// 関数差し替え可能にしたモック。他メソッドは interface 適合のための no-op スタブ。
type mockItemRepo struct {
	listNewAcrossFeedsFn func(ctx context.Context, userID string, sinceTime, cursorPublishedAt time.Time, cursorItemID string, limit int) ([]repository.CrossFeedItem, error)

	// 呼び出し記録
	lastUserID            string
	lastSinceTime         time.Time
	lastCursorPublishedAt time.Time
	lastCursorItemID      string
	lastLimit             int
	callCount             int
}

func (m *mockItemRepo) ListNewAcrossFeeds(
	ctx context.Context,
	userID string,
	sinceTime time.Time,
	cursorPublishedAt time.Time,
	cursorItemID string,
	limit int,
) ([]repository.CrossFeedItem, error) {
	m.lastUserID = userID
	m.lastSinceTime = sinceTime
	m.lastCursorPublishedAt = cursorPublishedAt
	m.lastCursorItemID = cursorItemID
	m.lastLimit = limit
	m.callCount++
	if m.listNewAcrossFeedsFn != nil {
		return m.listNewAcrossFeedsFn(ctx, userID, sinceTime, cursorPublishedAt, cursorItemID, limit)
	}
	return nil, nil
}

// --- ItemRepository interface の no-op スタブ群 ---

func (m *mockItemRepo) FindByID(_ context.Context, _ string) (*model.Item, error) {
	return nil, nil
}
func (m *mockItemRepo) FindByFeedAndGUID(_ context.Context, _, _ string) (*model.Item, error) {
	return nil, nil
}
func (m *mockItemRepo) FindByFeedAndLink(_ context.Context, _, _ string) (*model.Item, error) {
	return nil, nil
}
func (m *mockItemRepo) FindByContentHash(_ context.Context, _, _ string) (*model.Item, error) {
	return nil, nil
}
func (m *mockItemRepo) ListByFeed(_ context.Context, _, _ string, _ model.ItemFilter, _ time.Time, _ int) ([]model.ItemWithState, error) {
	return nil, nil
}
func (m *mockItemRepo) ListStarredByUser(_ context.Context, _ string, _ time.Time, _ int) ([]repository.StarredItemRow, error) {
	return nil, nil
}
func (m *mockItemRepo) Create(_ context.Context, _ *model.Item) error                  { return nil }
func (m *mockItemRepo) Update(_ context.Context, _ *model.Item) error                  { return nil }
func (m *mockItemRepo) FindExistingForUpsert(_ context.Context, _ string, _, _, _ []string) (*repository.ExistingItems, error) {
	return nil, nil
}
func (m *mockItemRepo) BulkUpsert(_ context.Context, _, _ []*model.Item) error { return nil }

// mockUserCrossFeedViewRepo は UserCrossFeedViewRepository のモック。
type mockUserCrossFeedViewRepo struct {
	getFn    func(ctx context.Context, userID string) (*model.UserCrossFeedView, error)
	upsertFn func(ctx context.Context, userID string, lastSeenAt time.Time) error

	// 呼び出し記録
	upsertCalledWithUserID     string
	upsertCalledWithLastSeenAt time.Time
	upsertCallCount            int
}

func (m *mockUserCrossFeedViewRepo) Get(ctx context.Context, userID string) (*model.UserCrossFeedView, error) {
	if m.getFn != nil {
		return m.getFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockUserCrossFeedViewRepo) Upsert(ctx context.Context, userID string, lastSeenAt time.Time) error {
	m.upsertCalledWithUserID = userID
	m.upsertCalledWithLastSeenAt = lastSeenAt
	m.upsertCallCount++
	if m.upsertFn != nil {
		return m.upsertFn(ctx, userID, lastSeenAt)
	}
	return nil
}

// --- ヘルパ ---

// newRowAt は指定の published_at / id / feed_id を持つ最小限の CrossFeedItem を生成する。
func newRowAt(id, feedID, feedTitle string, publishedAt time.Time) repository.CrossFeedItem {
	pa := publishedAt
	return repository.CrossFeedItem{
		ItemWithState: model.ItemWithState{
			Item: model.Item{
				ID:          id,
				FeedID:      feedID,
				Title:       "title-" + id,
				Link:        "https://example.com/" + id,
				Summary:     "summary-" + id,
				PublishedAt: &pa,
			},
		},
		FeedTitle: feedTitle,
	}
}

// --- ListNewItems テスト ---

func TestListNewItems(t *testing.T) {
	ctx := context.Background()
	userID := "user-1"

	t.Run("overrideSince=nil + lastSeen ありで sinceTime=lastSeen を採用すること（Req 4.7 / 4.5）", func(t *testing.T) {
		// Arrange
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
		itemRepo := &mockItemRepo{}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{
					UserID:     userID,
					LastSeenAt: lastSeen,
				}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 50, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !itemRepo.lastSinceTime.Equal(lastSeen) {
			t.Errorf("sinceTime mismatch: got %v, want %v", itemRepo.lastSinceTime, lastSeen)
		}
		if !result.SinceTime.Equal(lastSeen) {
			t.Errorf("result.SinceTime mismatch: got %v, want %v", result.SinceTime, lastSeen)
		}
	})

	t.Run("overrideSince=nil + lastSeen なしで fallback 24h を採用すること（Req 4.4）", func(t *testing.T) {
		// Arrange
		fixedNow := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
		want := fixedNow.Add(-24 * time.Hour)
		itemRepo := &mockItemRepo{}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return nil, nil // 未登録
			},
		}
		s := NewService(itemRepo, viewRepo)
		s.nowFn = func() time.Time { return fixedNow }

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 50, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !itemRepo.lastSinceTime.Equal(want) {
			t.Errorf("fallback sinceTime mismatch: got %v, want %v", itemRepo.lastSinceTime, want)
		}
		if !result.SinceTime.Equal(want) {
			t.Errorf("result.SinceTime mismatch: got %v, want %v", result.SinceTime, want)
		}
	})

	t.Run("overrideSince=非nil で lastSeen を無視し *overrideSince を採用すること（Req 4.7 優先順位）", func(t *testing.T) {
		// Arrange
		override := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC) // 異なる時刻
		itemRepo := &mockItemRepo{}
		getCalled := false
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				getCalled = true
				return &model.UserCrossFeedView{
					UserID:     userID,
					LastSeenAt: lastSeen,
				}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 50, &override)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if getCalled {
			t.Errorf("userCrossFeedViewRepo.Get should NOT be called when overrideSince is non-nil (Req 4.7)")
		}
		if !itemRepo.lastSinceTime.Equal(override) {
			t.Errorf("sinceTime mismatch: got %v, want %v (override)", itemRepo.lastSinceTime, override)
		}
		if !result.SinceTime.Equal(override) {
			t.Errorf("result.SinceTime mismatch: got %v, want %v", result.SinceTime, override)
		}
	})

	t.Run("limit+1 件取得で HasMore=true / NextCursor が <RFC3339Nano>:<itemID> 形式で組み立てられること", func(t *testing.T) {
		// Arrange
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
		// limit=2 を要求 → repo は limit+1=3 件を返し HasMore=true / 3 件目は cursor 算出にのみ使い表示から除外
		pa1 := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
		pa2 := time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)
		pa3 := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
		rows := []repository.CrossFeedItem{
			newRowAt("item-1", "feed-A", "Feed A", pa1),
			newRowAt("item-2", "feed-A", "Feed A", pa2),
			newRowAt("item-3", "feed-B", "Feed B", pa3),
		}
		itemRepo := &mockItemRepo{
			listNewAcrossFeedsFn: func(_ context.Context, _ string, _, _ time.Time, _ string, _ int) ([]repository.CrossFeedItem, error) {
				return rows, nil
			},
		}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{LastSeenAt: lastSeen}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 2, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if itemRepo.lastLimit != 3 {
			t.Errorf("repo は limit+1=3 件で呼ばれるべき: got %d", itemRepo.lastLimit)
		}
		if !result.HasMore {
			t.Errorf("HasMore should be true when repo returns limit+1 rows")
		}
		if len(result.Items) != 2 {
			t.Fatalf("Items count should be limit=2 (HasMore 判定用の 1 件は切り詰める): got %d", len(result.Items))
		}
		// NextCursor は表示最後尾（item-2 / pa2）の (RFC3339Nano, id)
		wantCursor := pa2.Format(time.RFC3339Nano) + ":item-2"
		if result.NextCursor != wantCursor {
			t.Errorf("NextCursor mismatch: got %q, want %q", result.NextCursor, wantCursor)
		}
	})

	t.Run("HasMore=false のとき NextCursor は空文字列", func(t *testing.T) {
		// Arrange
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
		pa1 := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
		rows := []repository.CrossFeedItem{
			newRowAt("item-1", "feed-A", "Feed A", pa1),
		}
		itemRepo := &mockItemRepo{
			listNewAcrossFeedsFn: func(_ context.Context, _ string, _, _ time.Time, _ string, _ int) ([]repository.CrossFeedItem, error) {
				return rows, nil
			},
		}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{LastSeenAt: lastSeen}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 50, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.HasMore {
			t.Errorf("HasMore should be false when repo returns <= limit rows")
		}
		if result.NextCursor != "" {
			t.Errorf("NextCursor should be empty when HasMore=false: got %q", result.NextCursor)
		}
		if len(result.Items) != 1 {
			t.Fatalf("Items count mismatch: got %d, want 1", len(result.Items))
		}
	})

	t.Run("favicon 非空のとき FeedFaviconURL に data URL が設定されること", func(t *testing.T) {
		// Arrange
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
		pa1 := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
		row := newRowAt("item-1", "feed-A", "Feed A", pa1)
		row.FaviconData = []byte{0x89, 0x50, 0x4e, 0x47}
		row.FaviconMime = "image/png"
		itemRepo := &mockItemRepo{
			listNewAcrossFeedsFn: func(_ context.Context, _ string, _, _ time.Time, _ string, _ int) ([]repository.CrossFeedItem, error) {
				return []repository.CrossFeedItem{row}, nil
			},
		}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{LastSeenAt: lastSeen}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 50, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Items) != 1 {
			t.Fatalf("Items count mismatch: got %d", len(result.Items))
		}
		got := result.Items[0].FeedFaviconURL
		if got == nil {
			t.Fatalf("FeedFaviconURL should be non-nil when favicon data is present")
		}
		// data:image/png;base64,iVBORw== の形式（4 byte = 6 char base64 + 2 padding）
		want := "data:image/png;base64,iVBORw=="
		if *got != want {
			t.Errorf("FeedFaviconURL mismatch: got %q, want %q", *got, want)
		}
	})

	t.Run("favicon 未設定（空 data または空 mime）のとき FeedFaviconURL が nil であること", func(t *testing.T) {
		// Arrange
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
		pa1 := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
		row := newRowAt("item-1", "feed-A", "Feed A", pa1)
		// 意図的に favicon を空のまま
		itemRepo := &mockItemRepo{
			listNewAcrossFeedsFn: func(_ context.Context, _ string, _, _ time.Time, _ string, _ int) ([]repository.CrossFeedItem, error) {
				return []repository.CrossFeedItem{row}, nil
			},
		}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{LastSeenAt: lastSeen}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		result, err := s.ListNewItems(ctx, userID, "", 50, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Items[0].FeedFaviconURL != nil {
			t.Errorf("FeedFaviconURL should be nil when favicon data/mime is empty")
		}
	})

	t.Run("有効な cursor が <RFC3339Nano>:<itemID> 形式で repo に渡されること", func(t *testing.T) {
		// Arrange
		lastSeen := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
		cursorPA := time.Date(2026, 5, 28, 11, 0, 0, 123456789, time.UTC)
		cursorID := "550e8400-e29b-41d4-a716-446655440000"
		cursorStr := cursorPA.Format(time.RFC3339Nano) + ":" + cursorID

		itemRepo := &mockItemRepo{}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{LastSeenAt: lastSeen}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		_, err := s.ListNewItems(ctx, userID, cursorStr, 50, nil)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !itemRepo.lastCursorPublishedAt.Equal(cursorPA) {
			t.Errorf("cursorPublishedAt mismatch: got %v, want %v", itemRepo.lastCursorPublishedAt, cursorPA)
		}
		if itemRepo.lastCursorItemID != cursorID {
			t.Errorf("cursorItemID mismatch: got %q, want %q", itemRepo.lastCursorItemID, cursorID)
		}
	})

	t.Run("不正形式の cursor は model.NewInvalidFilterError を返すこと", func(t *testing.T) {
		// Arrange
		itemRepo := &mockItemRepo{}
		viewRepo := &mockUserCrossFeedViewRepo{
			getFn: func(_ context.Context, _ string) (*model.UserCrossFeedView, error) {
				return &model.UserCrossFeedView{LastSeenAt: time.Now()}, nil
			},
		}
		s := NewService(itemRepo, viewRepo)

		// 不正形式の cursor パターンを複数検証
		invalidCursors := []string{
			"not-a-cursor",                // ":" を含まない
			":item-id-only",               // 先頭が ":"
			"2026-05-28T12:00:00Z:",       // 末尾が ":" で itemID が空
			"invalid-time:item-id",        // published_at が parse 不能
		}

		for _, c := range invalidCursors {
			// Act
			_, err := s.ListNewItems(ctx, userID, c, 50, nil)

			// Assert
			if err == nil {
				t.Errorf("invalid cursor %q should return error", c)
				continue
			}
			var apiErr *model.APIError
			if !errors.As(err, &apiErr) {
				t.Errorf("invalid cursor %q: expected *model.APIError, got %T (%v)", c, err, err)
				continue
			}
			if apiErr.Code != model.ErrCodeInvalidFilter {
				t.Errorf("invalid cursor %q: expected code %q, got %q", c, model.ErrCodeInvalidFilter, apiErr.Code)
			}
		}
	})
}

// --- TouchLastSeen テスト ---

func TestTouchLastSeen(t *testing.T) {
	ctx := context.Background()
	userID := "user-1"

	t.Run("TouchLastSeen が Upsert を now() で呼ぶこと", func(t *testing.T) {
		// Arrange
		fixedNow := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
		itemRepo := &mockItemRepo{}
		viewRepo := &mockUserCrossFeedViewRepo{}
		s := NewService(itemRepo, viewRepo)
		s.nowFn = func() time.Time { return fixedNow }

		// Act
		err := s.TouchLastSeen(ctx, userID)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if viewRepo.upsertCallCount != 1 {
			t.Errorf("Upsert call count mismatch: got %d, want 1", viewRepo.upsertCallCount)
		}
		if viewRepo.upsertCalledWithUserID != userID {
			t.Errorf("Upsert userID mismatch: got %q, want %q", viewRepo.upsertCalledWithUserID, userID)
		}
		if !viewRepo.upsertCalledWithLastSeenAt.Equal(fixedNow) {
			t.Errorf("Upsert lastSeenAt mismatch: got %v, want %v", viewRepo.upsertCalledWithLastSeenAt, fixedNow)
		}
	})

	t.Run("Upsert がエラーを返したとき TouchLastSeen はエラーを wrap して返すこと", func(t *testing.T) {
		// Arrange
		wantErr := errors.New("db connection lost")
		itemRepo := &mockItemRepo{}
		viewRepo := &mockUserCrossFeedViewRepo{
			upsertFn: func(_ context.Context, _ string, _ time.Time) error {
				return wantErr
			},
		}
		s := NewService(itemRepo, viewRepo)

		// Act
		err := s.TouchLastSeen(ctx, userID)

		// Assert
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("expected wrapped error to contain %v, got %v", wantErr, err)
		}
	})
}
