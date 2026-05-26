package app

import (
	"context"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
)

// waitGoroutineCount は goroutine 数が target 以下に収束するのを最大 timeout 待つ。
// goroutine の停止は非同期に行われるため、即時比較ではなくポーリングで収束を待つ。
func waitGoroutineCount(target int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		runtime.Gosched()
		n := runtime.NumGoroutine()
		if n <= target || time.Now().After(deadline) {
			return n
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newTestRateLimiter() *middleware.RateLimiter {
	return middleware.NewRateLimiter(middleware.RateLimiterConfig{
		GeneralRate:     2,
		GeneralBurst:    5,
		FeedRegRate:     1,
		FeedRegBurst:    10,
		CleanupInterval: 1 * time.Minute,
	})
}

// TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine は、シャットダウン経路で
// RateLimiter のクリーンアップ goroutine が停止し、リークしないことを検証する（NFR 1.1, AC 1.3）。
func TestShutdownCoordinator_StopsRateLimiterCleanupGoroutine(t *testing.T) {
	// Arrange: クリーンアップ goroutine を起動した RateLimiter と即時 Shutdown 可能なサーバー
	before := runtime.NumGoroutine()

	rl := newTestRateLimiter()

	// goroutine が起動したことを確認
	afterStart := runtime.NumGoroutine()
	if afterStart <= before {
		t.Fatalf("expected goroutine count to increase after NewRateLimiter, before=%d after=%d", before, afterStart)
	}

	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl)

	// Act: シャットダウン手続きを実行
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}

	// Assert: goroutine 数が起動前の水準に戻る（クリーンアップ goroutine が残存しない）
	after := waitGoroutineCount(before, 2*time.Second)
	if after > before {
		t.Errorf("goroutine leaked: before=%d after=%d (cleanup goroutine not stopped)", before, after)
	}
}

// TestShutdownCoordinator_DoesNotPanic は、シャットダウン経路で RateLimiter を停止しても
// panic しないことを検証する（AC 2.1）。
func TestShutdownCoordinator_DoesNotPanic(t *testing.T) {
	// Arrange
	rl := newTestRateLimiter()
	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl)

	// Act & Assert: panic が起きないこと（panic すれば test が異常終了する）
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

// TestShutdownCoordinator_DoubleInvocationDoesNotPanic は、シャットダウン経路が
// 重複して起動され得る状況でも panic しないことを検証する（AC 2.2, NFR 1.2）。
// shutdownCoordinator は RateLimiter.Stop() を 1 回だけ実行する idempotent 設計のため、
// 2 回呼んでも二重 close panic を起こさない。
func TestShutdownCoordinator_DoubleInvocationDoesNotPanic(t *testing.T) {
	// Arrange
	before := runtime.NumGoroutine()
	rl := newTestRateLimiter()
	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl)

	// Act: shutdown を 2 回呼ぶ（重複起動シナリオ）
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown returned error: %v", err)
	}
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown returned error: %v", err)
	}

	// Assert: panic せず、goroutine も収束する
	after := waitGoroutineCount(before, 2*time.Second)
	if after > before {
		t.Errorf("goroutine leaked: before=%d after=%d", before, after)
	}
}
