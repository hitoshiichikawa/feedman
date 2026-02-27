package repository

import (
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// TestPostgresItemRepo_ImplementsInterface はPostgresItemRepoがItemRepositoryを実装することを検証する。
func TestPostgresItemRepo_ImplementsInterface(t *testing.T) {
	// コンパイル時チェック：PostgresItemRepoがItemRepositoryを満たすことを検証
	var _ ItemRepository = (*PostgresItemRepo)(nil)
}

// TestPostgresItemStateRepo_ImplementsInterface はPostgresItemStateRepoがItemStateRepositoryを実装することを検証する。
func TestPostgresItemStateRepo_ImplementsInterface(t *testing.T) {
	// コンパイル時チェック：PostgresItemStateRepoがItemStateRepositoryを満たすことを検証
	var _ ItemStateRepository = (*PostgresItemStateRepo)(nil)
}

// TestItemFilterValues はItemFilterの定数値が正しいことを検証する。
func TestItemFilterValues(t *testing.T) {
	if model.ItemFilterAll != "all" {
		t.Errorf("ItemFilterAll = %q, want %q", model.ItemFilterAll, "all")
	}
	if model.ItemFilterUnread != "unread" {
		t.Errorf("ItemFilterUnread = %q, want %q", model.ItemFilterUnread, "unread")
	}
	if model.ItemFilterStarred != "starred" {
		t.Errorf("ItemFilterStarred = %q, want %q", model.ItemFilterStarred, "starred")
	}
}
