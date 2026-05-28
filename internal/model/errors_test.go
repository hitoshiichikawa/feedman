package model

import (
	"strings"
	"testing"
)

// TestNewInvalidSearchQueryError は記事検索クエリ不正エラーが
// Code / Category / Message の各フィールドを期待どおりに設定することを検証する
// （Issue #120 Task 2.2 / Req 3.3）。
func TestNewInvalidSearchQueryError(t *testing.T) {
	t.Run("reason が message に埋め込まれ Category が validation のとき", func(t *testing.T) {
		// Arrange
		reason := "cursor のフォーマットが不正です"

		// Act
		err := NewInvalidSearchQueryError(reason)

		// Assert
		if err == nil {
			t.Fatal("NewInvalidSearchQueryError が nil を返した")
		}
		if err.Code != ErrCodeInvalidSearchQuery {
			t.Errorf("Code = %q, want %q", err.Code, ErrCodeInvalidSearchQuery)
		}
		if err.Code != "INVALID_SEARCH_QUERY" {
			t.Errorf("Code リテラル = %q, want %q", err.Code, "INVALID_SEARCH_QUERY")
		}
		if err.Category != "validation" {
			t.Errorf("Category = %q, want %q", err.Category, "validation")
		}
		if !strings.Contains(err.Message, reason) {
			t.Errorf("Message に reason が埋め込まれていない: %q (reason=%q)", err.Message, reason)
		}
		if err.Action == "" {
			t.Error("Action が空である（ユーザー向け対処方法が必要）")
		}
	})
}

// TestNewFeedNotSubscribedError はフィード未購読エラーが
// Code / Category / Message の各フィールドを期待どおりに設定することを検証する
// （Issue #120 Task 2.2 / Req 3.5）。
func TestNewFeedNotSubscribedError(t *testing.T) {
	t.Run("feedID が message に埋め込まれ Category が authorization のとき", func(t *testing.T) {
		// Arrange
		feedID := "00000000-0000-0000-0000-000000000001"

		// Act
		err := NewFeedNotSubscribedError(feedID)

		// Assert
		if err == nil {
			t.Fatal("NewFeedNotSubscribedError が nil を返した")
		}
		if err.Code != ErrCodeFeedNotSubscribed {
			t.Errorf("Code = %q, want %q", err.Code, ErrCodeFeedNotSubscribed)
		}
		if err.Code != "FEED_NOT_SUBSCRIBED" {
			t.Errorf("Code リテラル = %q, want %q", err.Code, "FEED_NOT_SUBSCRIBED")
		}
		if err.Category != "authorization" {
			t.Errorf("Category = %q, want %q", err.Category, "authorization")
		}
		if !strings.Contains(err.Message, feedID) {
			t.Errorf("Message に feedID が埋め込まれていない: %q (feedID=%q)", err.Message, feedID)
		}
		if err.Action == "" {
			t.Error("Action が空である（ユーザー向け対処方法が必要）")
		}
	})
}
