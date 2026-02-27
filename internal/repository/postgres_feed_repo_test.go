package repository

import (
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// PostgresFeedRepoはFeedRepositoryインターフェースを満たすことを検証
func TestPostgresFeedRepo_ImplementsInterface(t *testing.T) {
	var _ FeedRepository = (*PostgresFeedRepo)(nil)
}

// NewPostgresFeedRepoが正しく初期化されることを検証
func TestNewPostgresFeedRepo_Initializes(t *testing.T) {
	repo := NewPostgresFeedRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

// Feedモデルのフィールドが正しく構築されることを検証
func TestPostgresFeedRepo_FeedModel_Fields(t *testing.T) {
	now := time.Now()
	feed := &model.Feed{
		ID:          "feed-id-1",
		FeedURL:     "https://example.com/feed.xml",
		SiteURL:     "https://example.com",
		Title:       "テストフィード",
		FetchStatus: model.FetchStatusActive,
		NextFetchAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if feed.ID != "feed-id-1" {
		t.Errorf("feed.ID = %q, want %q", feed.ID, "feed-id-1")
	}
	if feed.FeedURL != "https://example.com/feed.xml" {
		t.Errorf("feed.FeedURL = %q, want %q", feed.FeedURL, "https://example.com/feed.xml")
	}
	if feed.FetchStatus != model.FetchStatusActive {
		t.Errorf("feed.FetchStatus = %q, want %q", feed.FetchStatus, model.FetchStatusActive)
	}
}

// Feedのfaviconフィールドがnil許容であることを検証
func TestPostgresFeedRepo_FeedModel_NilFavicon(t *testing.T) {
	feed := &model.Feed{
		ID:      "feed-id-2",
		FeedURL: "https://example.com/feed.xml",
		Title:   "テストフィード",
	}

	if feed.FaviconData != nil {
		t.Error("favicon_data should be nil by default")
	}
	if feed.FaviconMime != "" {
		t.Error("favicon_mime should be empty by default")
	}
}
