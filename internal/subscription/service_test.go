package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- モック ---

type mockSubRepo struct {
	findByIDFn             func(ctx context.Context, id string) (*model.Subscription, error)
	listByUserIDWithFeedFn func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error)
	updateFetchIntervalFn  func(ctx context.Context, id string, minutes int) error
	deleteFn               func(ctx context.Context, id string) error
}

func (m *mockSubRepo) FindByID(ctx context.Context, id string) (*model.Subscription, error) {
	return m.findByIDFn(ctx, id)
}
func (m *mockSubRepo) FindByUserAndFeed(ctx context.Context, userID, feedID string) (*model.Subscription, error) {
	return nil, nil
}
func (m *mockSubRepo) CountByUserID(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *mockSubRepo) Create(ctx context.Context, sub *model.Subscription) error {
	return nil
}
func (m *mockSubRepo) ListByUserID(ctx context.Context, userID string) ([]*model.Subscription, error) {
	return nil, nil
}
func (m *mockSubRepo) MinFetchIntervalByFeedID(ctx context.Context, feedID string) (int, error) {
	return 60, nil
}
func (m *mockSubRepo) UpdateFetchInterval(ctx context.Context, id string, minutes int) error {
	if m.updateFetchIntervalFn != nil {
		return m.updateFetchIntervalFn(ctx, id, minutes)
	}
	return nil
}
func (m *mockSubRepo) Delete(ctx context.Context, id string) error {
	return m.deleteFn(ctx, id)
}
func (m *mockSubRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return nil
}
func (m *mockSubRepo) ListByUserIDWithFeedInfo(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
	return m.listByUserIDWithFeedFn(ctx, userID)
}

type mockItemStateRepo struct {
	deleteByUserAndFeedFn func(ctx context.Context, userID, feedID string) error
}

func (m *mockItemStateRepo) FindByUserAndItem(ctx context.Context, userID, itemID string) (*model.ItemState, error) {
	return nil, nil
}
func (m *mockItemStateRepo) Upsert(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
	return nil, nil
}
func (m *mockItemStateRepo) DeleteByUserAndFeed(ctx context.Context, userID, feedID string) error {
	return m.deleteByUserAndFeedFn(ctx, userID, feedID)
}
func (m *mockItemStateRepo) DeleteByUserID(ctx context.Context, userID string) error {
	return nil
}

type mockFeedRepo struct {
	findByIDFn         func(ctx context.Context, id string) (*model.Feed, error)
	updateFetchStateFn func(ctx context.Context, feed *model.Feed) error
}

func (m *mockFeedRepo) FindByID(ctx context.Context, id string) (*model.Feed, error) {
	return m.findByIDFn(ctx, id)
}
func (m *mockFeedRepo) FindByFeedURL(ctx context.Context, feedURL string) (*model.Feed, error) {
	return nil, nil
}
func (m *mockFeedRepo) Create(ctx context.Context, feed *model.Feed) error {
	return nil
}
func (m *mockFeedRepo) Update(ctx context.Context, feed *model.Feed) error {
	return nil
}
func (m *mockFeedRepo) UpdateFavicon(ctx context.Context, feedID string, data []byte, mime string) error {
	return nil
}
func (m *mockFeedRepo) ListDueForFetch(ctx context.Context) ([]*model.Feed, error) {
	return nil, nil
}
func (m *mockFeedRepo) UpdateFetchState(ctx context.Context, feed *model.Feed) error {
	if m.updateFetchStateFn != nil {
		return m.updateFetchStateFn(ctx, feed)
	}
	return nil
}

// --- テスト ---

// TestService_ListSubscriptions は購読一覧取得を検証する。
func TestService_ListSubscriptions(t *testing.T) {
	now := time.Now()
	subRepo := &mockSubRepo{
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return []repository.SubscriptionWithFeedInfo{
				{
					Subscription: model.Subscription{
						ID:                   "sub-1",
						UserID:               userID,
						FeedID:               "feed-1",
						FetchIntervalMinutes: 60,
						CreatedAt:            now,
					},
					FeedTitle:   "Test Feed",
					FeedURL:     "https://example.com/feed.xml",
					FetchStatus: model.FetchStatusActive,
					UnreadCount: 5,
				},
			}, nil
		},
	}

	svc := NewService(subRepo, nil, nil)

	results, err := svc.ListSubscriptions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListSubscriptions returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(results))
	}
	if results[0].FeedTitle != "Test Feed" {
		t.Errorf("FeedTitle = %q, want %q", results[0].FeedTitle, "Test Feed")
	}
	if results[0].UnreadCount != 5 {
		t.Errorf("UnreadCount = %d, want %d", results[0].UnreadCount, 5)
	}
}

// TestService_Unsubscribe は購読解除を検証する。
func TestService_Unsubscribe(t *testing.T) {
	deleteCalled := false
	itemStateDeleteCalled := false
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			deleteCalled = true
			return nil
		},
	}
	itemStateRepo := &mockItemStateRepo{
		deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
			itemStateDeleteCalled = true
			return nil
		},
	}

	svc := NewService(subRepo, itemStateRepo, nil)

	err := svc.Unsubscribe(context.Background(), "user-1", "sub-1")
	if err != nil {
		t.Fatalf("Unsubscribe returned error: %v", err)
	}
	if !deleteCalled {
		t.Error("expected subscription Delete to be called")
	}
	if !itemStateDeleteCalled {
		t.Error("expected item_states DeleteByUserAndFeed to be called")
	}
}

// TestService_Unsubscribe_WrongUser_ReturnsError は他ユーザーの購読解除が拒否されることを検証する。
func TestService_Unsubscribe_WrongUser_ReturnsError(t *testing.T) {
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-other", FeedID: "feed-1"}, nil
		},
	}

	svc := NewService(subRepo, nil, nil)

	err := svc.Unsubscribe(context.Background(), "user-1", "sub-1")
	if err == nil {
		t.Fatal("expected error for wrong user, got nil")
	}
}

// TestService_ResumeFetch は停止中フィードの再開を検証する。
func TestService_ResumeFetch(t *testing.T) {
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return []repository.SubscriptionWithFeedInfo{
				{
					Subscription: model.Subscription{ID: "sub-1", UserID: userID, FeedID: "feed-1", FetchIntervalMinutes: 60},
					FeedTitle:    "Test Feed",
					FeedURL:      "https://example.com/feed.xml",
					FetchStatus:  model.FetchStatusActive,
				},
			}, nil
		},
	}
	feedRepo := &mockFeedRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Feed, error) {
			return &model.Feed{
				ID:          "feed-1",
				FetchStatus: model.FetchStatusStopped,
			}, nil
		},
		updateFetchStateFn: func(ctx context.Context, feed *model.Feed) error {
			if feed.FetchStatus != model.FetchStatusActive {
				t.Errorf("expected FetchStatus = active, got %s", feed.FetchStatus)
			}
			return nil
		},
	}

	svc := NewService(subRepo, nil, feedRepo)

	result, err := svc.ResumeFetch(context.Background(), "user-1", "sub-1")
	if err != nil {
		t.Fatalf("ResumeFetch returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.FeedTitle != "Test Feed" {
		t.Errorf("FeedTitle = %q, want %q", result.FeedTitle, "Test Feed")
	}
}

// TestService_ResumeFetch_NotStopped_ReturnsError はアクティブなフィードの再開がエラーになることを検証する。
func TestService_ResumeFetch_NotStopped_ReturnsError(t *testing.T) {
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
	}
	feedRepo := &mockFeedRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Feed, error) {
			return &model.Feed{
				ID:          "feed-1",
				FetchStatus: model.FetchStatusActive,
			}, nil
		},
	}

	svc := NewService(subRepo, nil, feedRepo)

	_, err := svc.ResumeFetch(context.Background(), "user-1", "sub-1")
	if err == nil {
		t.Fatal("expected error for non-stopped feed, got nil")
	}
}
