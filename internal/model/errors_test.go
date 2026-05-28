package model

import (
	"strings"
	"testing"
)

// TestNewFeedFetchInProgressError は行ロック競合エラーが
// 期待する Code / Category と Action 文字列を含むことを検証する（Req 3.2, 3.3）。
func TestNewFeedFetchInProgressError(t *testing.T) {
	// Arrange & Act
	err := NewFeedFetchInProgressError()

	// Assert
	if err == nil {
		t.Fatal("expected non-nil APIError, got nil")
	}
	if err.Code != ErrCodeFeedFetchInProgress {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeFeedFetchInProgress)
	}
	if err.Category != "feed" {
		t.Errorf("Category = %q, want %q", err.Category, "feed")
	}
	if !strings.Contains(err.Action, "再試行") {
		t.Errorf("Action = %q, expected to contain 再試行", err.Action)
	}
	if err.Details != nil {
		t.Errorf("Details = %+v, want nil for lock conflict", err.Details)
	}
}

// TestNewFeedCooldownError はクールダウンエラーが
// retry_after_seconds を Details に int 型で載せることを検証する（Req 2.2）。
func TestNewFeedCooldownError(t *testing.T) {
	tests := []struct {
		name              string
		retryAfterSeconds int
	}{
		{"残り0秒のとき", 0},
		{"残り1秒のとき", 1},
		{"残り300秒のとき", 300},
		{"残り599秒のとき", 599},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange & Act
			err := NewFeedCooldownError(tt.retryAfterSeconds)

			// Assert
			if err == nil {
				t.Fatal("expected non-nil APIError, got nil")
			}
			if err.Code != ErrCodeFeedCooldown {
				t.Errorf("Code = %q, want %q", err.Code, ErrCodeFeedCooldown)
			}
			if err.Category != "feed" {
				t.Errorf("Category = %q, want %q", err.Category, "feed")
			}
			if err.Details == nil {
				t.Fatal("Details must be non-nil for cooldown error")
			}
			retry, ok := err.Details["retry_after_seconds"]
			if !ok {
				t.Fatal("Details[\"retry_after_seconds\"] not set")
			}
			gotInt, ok := retry.(int)
			if !ok {
				t.Fatalf("Details[\"retry_after_seconds\"] type = %T, want int", retry)
			}
			if gotInt != tt.retryAfterSeconds {
				t.Errorf("Details[\"retry_after_seconds\"] = %d, want %d", gotInt, tt.retryAfterSeconds)
			}
		})
	}
}

// TestAPIError_Error_PreservesFormat は既存テストが Details を参照していないため、
// Error() メソッドの出力フォーマットが Details 追加で変化していないことを検証する。
func TestAPIError_Error_PreservesFormat(t *testing.T) {
	// Arrange
	err := &APIError{
		Code:    "TEST_CODE",
		Message: "テストメッセージ",
	}

	// Act
	got := err.Error()

	// Assert
	want := "[TEST_CODE] テストメッセージ"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestAPIError_Details_OptionalDefault は Details フィールドの既定値が nil である
// ことを検証する（既存 APIError 生成関数が Details を設定していない箇所での後方互換性）。
func TestAPIError_Details_OptionalDefault(t *testing.T) {
	// Arrange
	err := NewSubscriptionNotFoundError("sub-1")

	// Assert
	if err.Details != nil {
		t.Errorf("Details should be nil for existing constructors, got %+v", err.Details)
	}
}
