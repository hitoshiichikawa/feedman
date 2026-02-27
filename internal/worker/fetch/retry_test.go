package fetch

import (
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- Task 6.3: リトライ・停止・バックオフ戦略のテスト ---

func TestShouldStopFetch_404(t *testing.T) {
	result := ClassifyHTTPStatus(404)
	if result != FetchResultStop {
		t.Errorf("404 は FetchResultStop を返すべき, got %v", result)
	}
}

func TestShouldStopFetch_410(t *testing.T) {
	result := ClassifyHTTPStatus(410)
	if result != FetchResultStop {
		t.Errorf("410 は FetchResultStop を返すべき, got %v", result)
	}
}

func TestShouldStopFetch_401(t *testing.T) {
	result := ClassifyHTTPStatus(401)
	if result != FetchResultStop {
		t.Errorf("401 は FetchResultStop を返すべき, got %v", result)
	}
}

func TestShouldStopFetch_403(t *testing.T) {
	result := ClassifyHTTPStatus(403)
	if result != FetchResultStop {
		t.Errorf("403 は FetchResultStop を返すべき, got %v", result)
	}
}

func TestShouldBackoff_429(t *testing.T) {
	result := ClassifyHTTPStatus(429)
	if result != FetchResultBackoff {
		t.Errorf("429 は FetchResultBackoff を返すべき, got %v", result)
	}
}

func TestShouldBackoff_500(t *testing.T) {
	result := ClassifyHTTPStatus(500)
	if result != FetchResultBackoff {
		t.Errorf("500 は FetchResultBackoff を返すべき, got %v", result)
	}
}

func TestShouldBackoff_502(t *testing.T) {
	result := ClassifyHTTPStatus(502)
	if result != FetchResultBackoff {
		t.Errorf("502 は FetchResultBackoff を返すべき, got %v", result)
	}
}

func TestShouldBackoff_503(t *testing.T) {
	result := ClassifyHTTPStatus(503)
	if result != FetchResultBackoff {
		t.Errorf("503 は FetchResultBackoff を返すべき, got %v", result)
	}
}

func TestSuccessStatus_200(t *testing.T) {
	result := ClassifyHTTPStatus(200)
	if result != FetchResultOK {
		t.Errorf("200 は FetchResultOK を返すべき, got %v", result)
	}
}

func TestSuccessStatus_304(t *testing.T) {
	result := ClassifyHTTPStatus(304)
	if result != FetchResultNotModified {
		t.Errorf("304 は FetchResultNotModified を返すべき, got %v", result)
	}
}

func TestCalculateBackoff_InitialDelay(t *testing.T) {
	// 初回バックオフ: 30分
	delay := CalculateBackoff(0)
	if delay != 30*time.Minute {
		t.Errorf("初回バックオフ = %v, want 30m", delay)
	}
}

func TestCalculateBackoff_SecondDelay(t *testing.T) {
	// 2回目: 60分
	delay := CalculateBackoff(1)
	if delay != 60*time.Minute {
		t.Errorf("2回目バックオフ = %v, want 60m", delay)
	}
}

func TestCalculateBackoff_ThirdDelay(t *testing.T) {
	// 3回目: 120分
	delay := CalculateBackoff(2)
	if delay != 120*time.Minute {
		t.Errorf("3回目バックオフ = %v, want 120m", delay)
	}
}

func TestCalculateBackoff_MaxDelay(t *testing.T) {
	// 最大12時間を超えない
	delay := CalculateBackoff(100)
	maxDelay := 12 * time.Hour
	if delay > maxDelay {
		t.Errorf("バックオフ = %v, 最大 %v を超えてはならない", delay, maxDelay)
	}
	if delay != maxDelay {
		t.Errorf("高い連続エラー数では最大値 %v を返すべき, got %v", maxDelay, delay)
	}
}

func TestApplyStopFeed(t *testing.T) {
	feed := &model.Feed{
		ID:          "feed-1",
		FetchStatus: model.FetchStatusActive,
	}

	ApplyStopFeed(feed, "404 Not Found")

	if feed.FetchStatus != model.FetchStatusStopped {
		t.Errorf("FetchStatus = %q, want %q", feed.FetchStatus, model.FetchStatusStopped)
	}
	if feed.ErrorMessage == "" {
		t.Error("ErrorMessage は設定されるべき")
	}
}

func TestApplyBackoff(t *testing.T) {
	now := time.Now()
	feed := &model.Feed{
		ID:                "feed-1",
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 0,
	}

	ApplyBackoff(feed, "429 Too Many Requests")

	if feed.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", feed.ConsecutiveErrors)
	}
	if feed.ErrorMessage == "" {
		t.Error("ErrorMessage は設定されるべき")
	}
	// NextFetchAtが現在時刻より後であること
	if !feed.NextFetchAt.After(now) {
		t.Errorf("NextFetchAt は現在時刻より後であるべき: %v", feed.NextFetchAt)
	}
}

func TestApplyBackoff_IncrementErrors(t *testing.T) {
	feed := &model.Feed{
		ID:                "feed-1",
		ConsecutiveErrors: 3,
	}

	ApplyBackoff(feed, "500 Internal Server Error")

	if feed.ConsecutiveErrors != 4 {
		t.Errorf("ConsecutiveErrors = %d, want 4", feed.ConsecutiveErrors)
	}
}

func TestApplySuccess(t *testing.T) {
	feed := &model.Feed{
		ID:                "feed-1",
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 5,
		ErrorMessage:      "previous error",
	}

	interval := 60 // 60分
	ApplySuccess(feed, interval)

	if feed.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", feed.ConsecutiveErrors)
	}
	if feed.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", feed.ErrorMessage)
	}
	// NextFetchAtが約60分後であること
	expectedTime := time.Now().Add(time.Duration(interval) * time.Minute)
	diff := feed.NextFetchAt.Sub(expectedTime)
	if diff > time.Second || diff < -time.Second {
		t.Errorf("NextFetchAt が期待値から大幅にずれている: %v (期待: %v)", feed.NextFetchAt, expectedTime)
	}
}

func TestCheckParseFailures_UnderThreshold(t *testing.T) {
	feed := &model.Feed{
		ConsecutiveErrors: 8,
	}

	shouldStop := CheckParseFailureThreshold(feed)
	if shouldStop {
		t.Error("連続エラー8回ではまだ停止すべきでない")
	}
}

func TestCheckParseFailures_AtThreshold(t *testing.T) {
	feed := &model.Feed{
		ConsecutiveErrors: 10,
	}

	shouldStop := CheckParseFailureThreshold(feed)
	if !shouldStop {
		t.Error("連続エラー10回で停止すべき")
	}
}

func TestCheckParseFailures_OverThreshold(t *testing.T) {
	feed := &model.Feed{
		ConsecutiveErrors: 15,
	}

	shouldStop := CheckParseFailureThreshold(feed)
	if !shouldStop {
		t.Error("連続エラー15回で停止すべき")
	}
}

func TestApplyParseFailure(t *testing.T) {
	feed := &model.Feed{
		ID:                "feed-1",
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 0,
	}

	ApplyParseFailure(feed, "invalid XML")

	if feed.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", feed.ConsecutiveErrors)
	}
	if feed.FetchStatus != model.FetchStatusActive {
		t.Error("1回目のパース失敗ではまだアクティブであるべき")
	}
}

func TestApplyParseFailure_StopsAt10(t *testing.T) {
	feed := &model.Feed{
		ID:                "feed-1",
		FetchStatus:       model.FetchStatusActive,
		ConsecutiveErrors: 9,
	}

	ApplyParseFailure(feed, "invalid XML")

	if feed.ConsecutiveErrors != 10 {
		t.Errorf("ConsecutiveErrors = %d, want 10", feed.ConsecutiveErrors)
	}
	if feed.FetchStatus != model.FetchStatusStopped {
		t.Errorf("10回連続パース失敗で停止されるべき: FetchStatus = %q", feed.FetchStatus)
	}
	if feed.ErrorMessage == "" {
		t.Error("ErrorMessage は設定されるべき")
	}
}
