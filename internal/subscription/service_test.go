package subscription

import (
	"context"
	"database/sql"
	"errors"
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
	findByIDFn                  func(ctx context.Context, id string) (*model.Feed, error)
	updateFetchStateFn          func(ctx context.Context, feed *model.Feed) error
	lockFeedFn                  func(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error)
	updateLastSuccessfulFetchFn func(ctx context.Context, feedID string, at time.Time) error
	updateLastSuccessfulFetchAt []time.Time
	updateLastSuccessfulFeedIDs []string
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
func (m *mockFeedRepo) LockFeedForUpdateNowait(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error) {
	if m.lockFeedFn != nil {
		return m.lockFeedFn(ctx, tx, feedID)
	}
	return nil, nil
}
func (m *mockFeedRepo) UpdateLastSuccessfulFetchAt(ctx context.Context, feedID string, at time.Time) error {
	m.updateLastSuccessfulFetchAt = append(m.updateLastSuccessfulFetchAt, at)
	m.updateLastSuccessfulFeedIDs = append(m.updateLastSuccessfulFeedIDs, feedID)
	if m.updateLastSuccessfulFetchFn != nil {
		return m.updateLastSuccessfulFetchFn(ctx, feedID, at)
	}
	return nil
}

type mockFeedFetcher struct {
	fetchFn func(ctx context.Context, feed *model.Feed) error
}

func (m *mockFeedFetcher) Fetch(ctx context.Context, feed *model.Feed) error {
	if m.fetchFn != nil {
		return m.fetchFn(ctx, feed)
	}
	return nil
}

type mockManualFetchTx struct {
	tx         *sql.Tx
	commitFn   func() error
	rollbackFn func() error
	committed  bool
	rolledBack bool
}

func (m *mockManualFetchTx) Tx() *sql.Tx { return m.tx }
func (m *mockManualFetchTx) Commit() error {
	m.committed = true
	if m.commitFn != nil {
		return m.commitFn()
	}
	return nil
}
func (m *mockManualFetchTx) Rollback() error {
	m.rolledBack = true
	if m.rollbackFn != nil {
		return m.rollbackFn()
	}
	return nil
}

type mockManualFetchTxBeginner struct {
	beginFn func(ctx context.Context) (ManualFetchTx, error)
}

func (m *mockManualFetchTxBeginner) BeginManualFetchTx(ctx context.Context) (ManualFetchTx, error) {
	if m.beginFn != nil {
		return m.beginFn(ctx)
	}
	return &mockManualFetchTx{}, nil
}

// mockManualFetchMetricsRecorder は metrics.MetricsCollector のテスト用モック。
// ManualFetch が利用する 4 メソッドの呼び出し回数とラベル値（reason）を観測する。
// 自動フェッチ系の 6 メソッドは ManualFetch から呼ばれないため no-op で interface を充足する。
type mockManualFetchMetricsRecorder struct {
	successCount      int
	failureCount      int
	failureReasons    []string
	cooldownCount     int
	lockConflictCount int
}

func (m *mockManualFetchMetricsRecorder) RecordManualFetchSuccess() {
	m.successCount++
}
func (m *mockManualFetchMetricsRecorder) RecordManualFetchFailure(reason string) {
	m.failureCount++
	m.failureReasons = append(m.failureReasons, reason)
}
func (m *mockManualFetchMetricsRecorder) RecordManualFetchCooldownRejected() {
	m.cooldownCount++
}
func (m *mockManualFetchMetricsRecorder) RecordManualFetchLockConflict() {
	m.lockConflictCount++
}

// MetricsCollector の自動フェッチ系メソッドは ManualFetch のコードパスから呼ばれない。
// interface 充足のための no-op 実装。
func (m *mockManualFetchMetricsRecorder) RecordFetchSuccess(_ string)        {}
func (m *mockManualFetchMetricsRecorder) RecordFetchFailure(_, _ string)     {}
func (m *mockManualFetchMetricsRecorder) RecordParseFailure(_ string)        {}
func (m *mockManualFetchMetricsRecorder) RecordHTTPStatus(_ int)             {}
func (m *mockManualFetchMetricsRecorder) RecordFetchLatency(_ time.Duration) {}
func (m *mockManualFetchMetricsRecorder) RecordItemsUpserted(_ int)          {}

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

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

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

// TestService_UpdateSettings_BoundaryValues はフェッチ間隔の境界値バリデーションを検証する。
// 要件 1.1-1.10 / 2.1 / 2.4 / 3.1 / NFR 1.1 / NFR 2.1 に対応する。
func TestService_UpdateSettings_BoundaryValues(t *testing.T) {
	tests := []struct {
		name       string
		minutes    int
		wantReject bool
	}{
		{"下限未満(29)のとき拒否", 29, true},
		{"下限(30)のとき受理", 30, false},
		{"刻み違反(31)のとき拒否", 31, true},
		{"刻み違反(45)のとき拒否", 45, true},
		{"中間値(60)のとき受理", 60, false},
		{"中間値(90)のとき受理", 90, false},
		{"上限(720)のとき受理", 720, false},
		{"上限超過(721)のとき拒否", 721, true},
		{"ゼロ(0)のとき拒否", 0, true},
		{"負値(-30)のとき拒否", -30, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			now := time.Now()
			updateCalled := false
			subRepo := &mockSubRepo{
				findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
					return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
				},
				updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
					updateCalled = true
					return nil
				},
				listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
					return []repository.SubscriptionWithFeedInfo{
						{
							Subscription: model.Subscription{
								ID:                   "sub-1",
								UserID:               userID,
								FeedID:               "feed-1",
								FetchIntervalMinutes: tt.minutes,
								CreatedAt:            now,
							},
							FeedTitle:   "Test Feed",
							FeedURL:     "https://example.com/feed.xml",
							FetchStatus: model.FetchStatusActive,
							UnreadCount: 2,
						},
					}, nil
				},
			}

			svc := NewService(subRepo, nil, nil, nil, nil, nil)

			// Act
			result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", tt.minutes)

			// Assert
			if tt.wantReject {
				if err == nil {
					t.Fatalf("minutes=%d: expected error, got nil", tt.minutes)
				}
				apiErr, ok := err.(*model.APIError)
				if !ok {
					t.Fatalf("minutes=%d: error type = %T, want *model.APIError", tt.minutes, err)
				}
				if apiErr.Code != model.ErrCodeInvalidFetchInterval {
					t.Errorf("minutes=%d: error code = %q, want %q", tt.minutes, apiErr.Code, model.ErrCodeInvalidFetchInterval)
				}
				if updateCalled {
					t.Errorf("minutes=%d: UpdateFetchInterval should not be called on rejection", tt.minutes)
				}
				if result != nil {
					t.Errorf("minutes=%d: expected nil result on rejection, got %+v", tt.minutes, result)
				}
				return
			}

			if err != nil {
				t.Fatalf("minutes=%d: unexpected error: %v", tt.minutes, err)
			}
			if !updateCalled {
				t.Errorf("minutes=%d: expected UpdateFetchInterval to be called", tt.minutes)
			}
			if result == nil {
				t.Fatalf("minutes=%d: expected non-nil result", tt.minutes)
			}
			if result.FetchIntervalMinutes != tt.minutes {
				t.Errorf("minutes=%d: FetchIntervalMinutes = %d, want %d", tt.minutes, result.FetchIntervalMinutes, tt.minutes)
			}
			if result.FeedTitle != "Test Feed" {
				t.Errorf("minutes=%d: FeedTitle = %q, want %q", tt.minutes, result.FeedTitle, "Test Feed")
			}
		})
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

	svc := NewService(subRepo, itemStateRepo, nil, nil, nil, nil)

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

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

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

	svc := NewService(subRepo, nil, feedRepo, nil, nil, nil)

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

	svc := NewService(subRepo, nil, feedRepo, nil, nil, nil)

	_, err := svc.ResumeFetch(context.Background(), "user-1", "sub-1")
	if err == nil {
		t.Fatal("expected error for non-stopped feed, got nil")
	}
}

// TestService_Unsubscribe_NilItemStateRepo_SkipsItemStateDelete は
// 記事状態リポジトリが未設定（nil）のとき、記事状態削除をスキップしたうえで
// 購読削除が正常に完了することを検証する（要件 3.1）。
func TestService_Unsubscribe_NilItemStateRepo_SkipsItemStateDelete(t *testing.T) {
	// Arrange: itemStateRepo を nil で生成する。
	deleteCalled := false
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			deleteCalled = true
			return nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	err := svc.Unsubscribe(context.Background(), "user-1", "sub-1")

	// Assert: nil 分岐でも購読削除が呼ばれ、エラーなく完了する。
	if err != nil {
		t.Fatalf("Unsubscribe returned error: %v", err)
	}
	if !deleteCalled {
		t.Error("expected subscription Delete to be called even when itemStateRepo is nil")
	}
}

// TestService_Unsubscribe_ItemStateDeleteError_PropagatesError は
// 記事状態リポジトリの削除（DeleteByUserAndFeed）がエラーを返したとき、
// 購読解除がそのエラーを呼び出し元へ伝播し、購読削除を行わないことを検証する（要件 3.2）。
func TestService_Unsubscribe_ItemStateDeleteError_PropagatesError(t *testing.T) {
	// Arrange: DeleteByUserAndFeed がエラーを返す。
	sentinelErr := errors.New("delete item states failed")
	deleteCalled := false
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
			return sentinelErr
		},
	}

	svc := NewService(subRepo, itemStateRepo, nil, nil, nil, nil)

	// Act
	err := svc.Unsubscribe(context.Background(), "user-1", "sub-1")

	// Assert: エラーが wrap されて伝播し、購読削除は実行されない。
	if err == nil {
		t.Fatal("expected error from item state delete, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected wrapped error to match sentinel, got %v", err)
	}
	if deleteCalled {
		t.Error("subscription Delete should not be called when item state delete fails")
	}
}

// TestService_ResumeFetch_NotStopped_ReturnsFeedNotStoppedAndDoesNotUpdate は
// 停止中ではない（active）フィードに対してフェッチ再開が呼ばれたとき、
// 状態前提違反として FEED_NOT_STOPPED 専用エラーを返し、フィード状態を更新しないことを
// 検証する（要件 4.1）。
func TestService_ResumeFetch_NotStopped_ReturnsFeedNotStoppedAndDoesNotUpdate(t *testing.T) {
	// Arrange: フィードは active 状態（停止中ではない）。
	updateCalled := false
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
	}
	feedRepo := &mockFeedRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Feed, error) {
			return &model.Feed{ID: "feed-1", FetchStatus: model.FetchStatusActive}, nil
		},
		updateFetchStateFn: func(ctx context.Context, feed *model.Feed) error {
			updateCalled = true
			return nil
		},
	}

	svc := NewService(subRepo, nil, feedRepo, nil, nil, nil)

	// Act
	result, err := svc.ResumeFetch(context.Background(), "user-1", "sub-1")

	// Assert: 専用エラー（FEED_NOT_STOPPED）が返り、状態更新は呼ばれない。
	if err == nil {
		t.Fatal("expected error for non-stopped feed, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeFeedNotStopped {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeFeedNotStopped)
	}
	if updateCalled {
		t.Error("UpdateFetchState should not be called when feed is not stopped")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_UpdateSettings_Success は有効間隔・所有者一致・全依存成功時に
// 更新後の購読情報を返し UpdateFetchInterval が呼ばれることを検証する（要件 1.1）。
func TestService_UpdateSettings_Success(t *testing.T) {
	// Arrange
	now := time.Now()
	updateCalled := false
	const wantMinutes = 60
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			updateCalled = true
			return nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return []repository.SubscriptionWithFeedInfo{
				{
					Subscription: model.Subscription{
						ID:                   "sub-1",
						UserID:               userID,
						FeedID:               "feed-1",
						FetchIntervalMinutes: wantMinutes,
						CreatedAt:            now,
					},
					FeedTitle:   "Test Feed",
					FeedURL:     "https://example.com/feed.xml",
					FetchStatus: model.FetchStatusActive,
					UnreadCount: 3,
				},
			}, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", wantMinutes)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("expected UpdateFetchInterval to be called")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "sub-1" {
		t.Errorf("ID = %q, want %q", result.ID, "sub-1")
	}
	if result.FetchIntervalMinutes != wantMinutes {
		t.Errorf("FetchIntervalMinutes = %d, want %d", result.FetchIntervalMinutes, wantMinutes)
	}
	if result.FeedTitle != "Test Feed" {
		t.Errorf("FeedTitle = %q, want %q", result.FeedTitle, "Test Feed")
	}
	if result.UnreadCount != 3 {
		t.Errorf("UnreadCount = %d, want %d", result.UnreadCount, 3)
	}
}

// TestService_UpdateSettings_WrongUser_ReturnsSubscriptionNotFound は
// 他ユーザー所有の購読 ID 指定時に SUBSCRIPTION_NOT_FOUND を返し、
// フェッチ間隔更新が呼ばれないことを検証する（要件 1.2 / 2.1 / 2.2）。
func TestService_UpdateSettings_WrongUser_ReturnsSubscriptionNotFound(t *testing.T) {
	// Arrange
	updateCalled := false
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-other", FeedID: "feed-1"}, nil
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			updateCalled = true
			return nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return nil, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", 60)

	// Assert
	if err == nil {
		t.Fatal("expected error for wrong user, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeSubscriptionNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeSubscriptionNotFound)
	}
	if updateCalled {
		t.Error("UpdateFetchInterval should not be called on authorization failure")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_UpdateSettings_SubscriptionNotFound は
// 購読が存在しない（FindByID が nil を返す）場合に
// SUBSCRIPTION_NOT_FOUND を返すことを検証する（要件 1.3）。
func TestService_UpdateSettings_SubscriptionNotFound(t *testing.T) {
	// Arrange
	updateCalled := false
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return nil, nil
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			updateCalled = true
			return nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return nil, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", 60)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing subscription, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeSubscriptionNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeSubscriptionNotFound)
	}
	if updateCalled {
		t.Error("UpdateFetchInterval should not be called when subscription is missing")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_UpdateSettings_FindByIDError は
// 購読フェッチ（FindByID）が永続層エラーを返す場合に
// 当該エラーが wrap されて伝播することを検証する（要件 1.4）。
func TestService_UpdateSettings_FindByIDError(t *testing.T) {
	// Arrange
	sentinelErr := errors.New("find by id failed")
	updateCalled := false
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return nil, sentinelErr
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			updateCalled = true
			return nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return nil, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", 60)

	// Assert
	if err == nil {
		t.Fatal("expected error from FindByID, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected wrapped error to match sentinel, got %v", err)
	}
	if updateCalled {
		t.Error("UpdateFetchInterval should not be called when FindByID fails")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_UpdateSettings_UpdateFetchIntervalError は
// フェッチ間隔更新（UpdateFetchInterval）がエラーを返す場合に
// 当該エラーが wrap されて伝播することを検証する（要件 1.5）。
func TestService_UpdateSettings_UpdateFetchIntervalError(t *testing.T) {
	// Arrange
	sentinelErr := errors.New("update fetch interval failed")
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			return sentinelErr
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return nil, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", 60)

	// Assert
	if err == nil {
		t.Fatal("expected error from UpdateFetchInterval, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected wrapped error to match sentinel, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_UpdateSettings_ListByUserIDWithFeedInfoError は
// 更新後の購読一覧再取得（ListByUserIDWithFeedInfo）がエラーを返す場合に
// 当該エラーが wrap されて伝播することを検証する（要件 1.6）。
func TestService_UpdateSettings_ListByUserIDWithFeedInfoError(t *testing.T) {
	// Arrange
	sentinelErr := errors.New("list by user id with feed info failed")
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			return nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return nil, sentinelErr
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", 60)

	// Assert
	if err == nil {
		t.Fatal("expected error from ListByUserIDWithFeedInfo, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected wrapped error to match sentinel, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_UpdateSettings_NotFoundAfterUpdate は
// 更新は成功するが再取得結果に対象購読 ID が含まれない場合に
// SUBSCRIPTION_NOT_FOUND を返すことを検証する（要件 1.7）。
func TestService_UpdateSettings_NotFoundAfterUpdate(t *testing.T) {
	// Arrange
	now := time.Now()
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}, nil
		},
		updateFetchIntervalFn: func(ctx context.Context, id string, minutes int) error {
			return nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			// 対象 ID（sub-1）を含まない別購読のみを返す
			return []repository.SubscriptionWithFeedInfo{
				{
					Subscription: model.Subscription{
						ID:                   "sub-other",
						UserID:               userID,
						FeedID:               "feed-other",
						FetchIntervalMinutes: 60,
						CreatedAt:            now,
					},
					FeedTitle:   "Other Feed",
					FeedURL:     "https://example.com/other.xml",
					FetchStatus: model.FetchStatusActive,
				},
			}, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	result, err := svc.UpdateSettings(context.Background(), "user-1", "sub-1", 60)

	// Assert
	if err == nil {
		t.Fatal("expected error when target subscription is absent after update, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeSubscriptionNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeSubscriptionNotFound)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// TestService_ManualFetch_Success は手動フェッチが正常に成功し、
// 最新の購読情報を返すこと、クールダウンが更新されることを検証する。
func TestService_ManualFetch_Success(t *testing.T) {
	sub := &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}
	feed := &model.Feed{
		ID:          "feed-1",
		FetchStatus: model.FetchStatusActive,
	}

	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return sub, nil
		},
		listByUserIDWithFeedFn: func(ctx context.Context, userID string) ([]repository.SubscriptionWithFeedInfo, error) {
			return []repository.SubscriptionWithFeedInfo{
				{
					Subscription: *sub,
					FeedTitle:    "Test Feed",
					FeedURL:      "https://example.com/feed.xml",
					FetchStatus:  model.FetchStatusActive,
				},
			}, nil
		},
	}

	feedRepo := &mockFeedRepo{
		lockFeedFn: func(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error) {
			return feed, nil
		},
	}

	fetcher := &mockFeedFetcher{
		fetchFn: func(ctx context.Context, f *model.Feed) error {
			// フェッチャーの中で成功したと仮定して状態を更新
			f.ConsecutiveErrors = 0
			f.FetchStatus = model.FetchStatusActive
			return nil
		},
	}

	txBeginner := &mockManualFetchTxBeginner{}
	metrics := &mockManualFetchMetricsRecorder{}

	svc := NewService(subRepo, nil, feedRepo, fetcher, txBeginner, metrics)

	// Act
	result, err := svc.ManualFetch(context.Background(), "user-1", "sub-1")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "sub-1" {
		t.Errorf("expected subscription ID sub-1, got %q", result.ID)
	}
	if metrics.successCount != 1 {
		t.Errorf("expected successCount to be 1, got %d", metrics.successCount)
	}
	if len(feedRepo.updateLastSuccessfulFetchAt) != 1 {
		t.Errorf("expected UpdateLastSuccessfulFetchAt to be called once, got %d", len(feedRepo.updateLastSuccessfulFetchAt))
	}
}

// TestService_ManualFetch_SubscriptionNotFound は購読情報が存在しないか、
// 他ユーザーのものである場合に SUBSCRIPTION_NOT_FOUND を返すことを検証する。
func TestService_ManualFetch_SubscriptionNotFound(t *testing.T) {
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			// 他ユーザーの購読
			return &model.Subscription{ID: "sub-1", UserID: "user-other", FeedID: "feed-1"}, nil
		},
	}

	svc := NewService(subRepo, nil, nil, nil, nil, nil)

	// Act
	_, err := svc.ManualFetch(context.Background(), "user-1", "sub-1")

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeSubscriptionNotFound {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeSubscriptionNotFound)
	}
}

// TestService_ManualFetch_LockConflict は行ロック競合時に
// FEED_FETCH_IN_PROGRESS を返すことを検証する。
func TestService_ManualFetch_LockConflict(t *testing.T) {
	sub := &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}
	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return sub, nil
		},
	}

	feedRepo := &mockFeedRepo{
		lockFeedFn: func(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error) {
			return nil, repository.ErrFeedLocked
		},
	}

	txBeginner := &mockManualFetchTxBeginner{}
	metrics := &mockManualFetchMetricsRecorder{}

	svc := NewService(subRepo, nil, feedRepo, nil, txBeginner, metrics)

	// Act
	_, err := svc.ManualFetch(context.Background(), "user-1", "sub-1")

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeFeedFetchInProgress {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeFeedFetchInProgress)
	}
	if metrics.lockConflictCount != 1 {
		t.Errorf("expected lockConflictCount to be 1, got %d", metrics.lockConflictCount)
	}
}

// TestService_ManualFetch_Cooldown はクールダウン判定（10分以内）により、
// FEED_COOLDOWN を返すことを検証する。
func TestService_ManualFetch_Cooldown(t *testing.T) {
	sub := &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}
	// 5分前に成功した履歴がある
	lastFetch := time.Now().Add(-5 * time.Minute)
	feed := &model.Feed{
		ID:                    "feed-1",
		FetchStatus:           model.FetchStatusActive,
		LastSuccessfulFetchAt: &lastFetch,
	}

	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return sub, nil
		},
	}

	feedRepo := &mockFeedRepo{
		lockFeedFn: func(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error) {
			return feed, nil
		},
	}

	txBeginner := &mockManualFetchTxBeginner{}
	metrics := &mockManualFetchMetricsRecorder{}

	svc := NewService(subRepo, nil, feedRepo, nil, txBeginner, metrics)

	// Act
	_, err := svc.ManualFetch(context.Background(), "user-1", "sub-1")

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *model.APIError", err)
	}
	if apiErr.Code != model.ErrCodeFeedCooldown {
		t.Errorf("error code = %q, want %q", apiErr.Code, model.ErrCodeFeedCooldown)
	}
	if metrics.cooldownCount != 1 {
		t.Errorf("expected cooldownCount to be 1, got %d", metrics.cooldownCount)
	}
}

// TestService_ManualFetch_FetchError はフェッチ失敗時に、
// 適切な API エラーを返すこと、メトリクスが記録されることを検証する。
func TestService_ManualFetch_FetchError(t *testing.T) {
	sub := &model.Subscription{ID: "sub-1", UserID: "user-1", FeedID: "feed-1"}
	feed := &model.Feed{
		ID:          "feed-1",
		FetchStatus: model.FetchStatusActive,
	}

	subRepo := &mockSubRepo{
		findByIDFn: func(ctx context.Context, id string) (*model.Subscription, error) {
			return sub, nil
		},
	}

	feedRepo := &mockFeedRepo{
		lockFeedFn: func(ctx context.Context, tx *sql.Tx, feedID string) (*model.Feed, error) {
			return feed, nil
		},
	}

	tests := []struct {
		name       string
		fetchErr   error
		feedErrMsg string
		feedStatus model.FetchStatus
		wantCode   string
		wantReason string
	}{
		{
			name:       "SSRF error",
			fetchErr:   errors.New("SSRF detected"),
			wantCode:   model.ErrCodeFetchFailed,
			wantReason: "ssrf_blocked",
		},
		{
			name:       "Parse error in fetch",
			fetchErr:   errors.New("parse failed"),
			wantCode:   model.ErrCodeFetchFailed,
			wantReason: "parse_error",
		},
		{
			name:       "Normal fetch error",
			fetchErr:   errors.New("http 500"),
			wantCode:   model.ErrCodeFetchFailed,
			wantReason: "fetch_error",
		},
		{
			name:       "Parse failed in feed message status",
			fetchErr:   nil,
			feedErrMsg: "パース失敗しました",
			feedStatus: model.FetchStatusError,
			wantCode:   model.ErrCodeParseFailed,
			wantReason: "parse_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed.ErrorMessage = tt.feedErrMsg
			feed.FetchStatus = tt.feedStatus
			if tt.feedStatus == "" {
				feed.FetchStatus = model.FetchStatusActive
			}

			fetcher := &mockFeedFetcher{
				fetchFn: func(ctx context.Context, f *model.Feed) error {
					return tt.fetchErr
				},
			}

			txBeginner := &mockManualFetchTxBeginner{}
			metrics := &mockManualFetchMetricsRecorder{}

			svc := NewService(subRepo, nil, feedRepo, fetcher, txBeginner, metrics)

			// Act
			_, err := svc.ManualFetch(context.Background(), "user-1", "sub-1")

			// Assert
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			apiErr, ok := err.(*model.APIError)
			if !ok {
				t.Fatalf("error type = %T, want *model.APIError", err)
			}
			if apiErr.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", apiErr.Code, tt.wantCode)
			}
			if metrics.failureCount != 1 {
				t.Errorf("expected failureCount to be 1, got %d", metrics.failureCount)
			}
			if metrics.failureReasons[0] != tt.wantReason {
				t.Errorf("expected failure reason %q, got %q", tt.wantReason, metrics.failureReasons[0])
			}
		})
	}
}
