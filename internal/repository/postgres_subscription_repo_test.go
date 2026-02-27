package repository

import (
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresSubscriptionRepoはSubscriptionRepositoryインターフェースを満たすことを検証
func TestPostgresSubscriptionRepo_ImplementsInterface(t *testing.T) {
	var _ SubscriptionRepository = (*PostgresSubscriptionRepo)(nil)
}

// NewPostgresSubscriptionRepoが正しく初期化されることを検証
func TestNewPostgresSubscriptionRepo_Initializes(t *testing.T) {
	repo := NewPostgresSubscriptionRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// Subscriptionモデルのフィールドが正しく構築されることを検証
func TestPostgresSubscriptionRepo_SubscriptionModel_Fields(t *testing.T) {
	now := time.Now()
	sub := &model.Subscription{
		ID:                   "sub-id-1",
		UserID:               "user-id-1",
		FeedID:               "feed-id-1",
		FetchIntervalMinutes: 60,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if sub.UserID != "user-id-1" {
		t.Errorf("sub.UserID = %q, want %q", sub.UserID, "user-id-1")
	}
	if sub.FeedID != "feed-id-1" {
		t.Errorf("sub.FeedID = %q, want %q", sub.FeedID, "feed-id-1")
	}
	if sub.FetchIntervalMinutes != 60 {
		t.Errorf("sub.FetchIntervalMinutes = %d, want %d", sub.FetchIntervalMinutes, 60)
	}
}
