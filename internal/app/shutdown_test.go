package app

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
)

// cleanup goroutine の関数名（goroutine スタックダンプ上の表記）。
// `middleware.(*` まで含めることで RateLimiter / IPRateLimiter が substring 衝突しない。
const (
	rateLimiterCleanupFn   = "middleware.(*RateLimiter).cleanupLoop"
	ipRateLimiterCleanupFn = "middleware.(*IPRateLimiter).cleanupLoop"
)

// countGoroutinesContaining は全 goroutine のスタックダンプから、関数名 fn を含む
// goroutine 数を数える。
//
// runtime.NumGoroutine() のグローバル総数比較は、同一テストバイナリ内の先行テストが
// 残した goroutine の起動・終了とレースして flaky になる（CI で before=after の偽陰性を
// 2 度観測）。対象 goroutine の関数名を直接数えることで、無関係な goroutine の増減に
// 影響されず決定論的に判定する。
func countGoroutinesContaining(fn string) int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), fn)
}

// waitGoroutineRunning は fn を含む goroutine が現れるのを最大 timeout 待ち、最終観測数を返す。
// go 文によるgoroutine 起動は非同期のため、起動直後の即時観測ではなくポーリングで待つ。
func waitGoroutineRunning(fn string, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		runtime.Gosched()
		c := countGoroutinesContaining(fn)
		if c > 0 || time.Now().After(deadline) {
			return c
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitGoroutineGone は fn を含む goroutine が消えるのを最大 timeout 待ち、最終観測数を返す。
// goroutine の停止は非同期に行われるため、ポーリングで収束を待つ。
func waitGoroutineGone(fn string, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		runtime.Gosched()
		c := countGoroutinesContaining(fn)
		if c == 0 || time.Now().After(deadline) {
			return c
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
	rl := newTestRateLimiter()

	// goroutine が起動したことを確認（スケジュール完了をポーリングで待つ）
	if c := waitGoroutineRunning(rateLimiterCleanupFn, 2*time.Second); c == 0 {
		t.Fatal("expected RateLimiter cleanup goroutine to start after NewRateLimiter")
	}

	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl, nil)

	// Act: シャットダウン手続きを実行
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}

	// Assert: クリーンアップ goroutine が残存しない
	if c := waitGoroutineGone(rateLimiterCleanupFn, 2*time.Second); c > 0 {
		t.Errorf("cleanup goroutine leaked: %d goroutine(s) still running after shutdown", c)
	}
}

// TestShutdownCoordinator_StopsIPRateLimiterCleanupGoroutine は、シャットダウン経路で
// IPRateLimiter のクリーンアップ goroutine も停止し、リークしないことを検証する（NFR 3.1）。
func TestShutdownCoordinator_StopsIPRateLimiterCleanupGoroutine(t *testing.T) {
	// Arrange: クリーンアップ goroutine を起動した IPRateLimiter（userID 側は nil）。
	ipRL := middleware.NewIPRateLimiter(middleware.DefaultIPRateLimiterConfig(30))

	// goroutine が起動したことを確認（スケジュール完了をポーリングで待つ）
	if c := waitGoroutineRunning(ipRateLimiterCleanupFn, 2*time.Second); c == 0 {
		t.Fatal("expected IPRateLimiter cleanup goroutine to start after NewIPRateLimiter")
	}

	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, nil, ipRL)

	// Act
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}

	// Assert: クリーンアップ goroutine が残存しない
	if c := waitGoroutineGone(ipRateLimiterCleanupFn, 2*time.Second); c > 0 {
		t.Errorf("IP cleanup goroutine leaked: %d goroutine(s) still running after shutdown", c)
	}
}

// TestShutdownCoordinator_StopsBothLimiters は userID/IP 両リミッターを同時に停止しても
// 二重 close panic が起きず goroutine が収束することを検証する。
func TestShutdownCoordinator_StopsBothLimiters(t *testing.T) {
	// Arrange
	rl := newTestRateLimiter()
	ipRL := middleware.NewIPRateLimiter(middleware.DefaultIPRateLimiterConfig(30))
	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl, ipRL)

	// Act: 重複起動でも panic しないこと。
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown returned error: %v", err)
	}
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown returned error: %v", err)
	}

	// Assert: 両リミッターのクリーンアップ goroutine が残存しない
	if c := waitGoroutineGone(rateLimiterCleanupFn, 2*time.Second); c > 0 {
		t.Errorf("RateLimiter cleanup goroutine leaked: %d goroutine(s) still running", c)
	}
	if c := waitGoroutineGone(ipRateLimiterCleanupFn, 2*time.Second); c > 0 {
		t.Errorf("IPRateLimiter cleanup goroutine leaked: %d goroutine(s) still running", c)
	}
}

// TestShutdownCoordinator_DoesNotPanic は、シャットダウン経路で RateLimiter を停止しても
// panic しないことを検証する（AC 2.1）。
func TestShutdownCoordinator_DoesNotPanic(t *testing.T) {
	// Arrange
	rl := newTestRateLimiter()
	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl, nil)

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
	rl := newTestRateLimiter()
	sc := newShutdownCoordinator(&http.Server{Addr: ":0"}, rl, nil)

	// Act: shutdown を 2 回呼ぶ（重複起動シナリオ）
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown returned error: %v", err)
	}
	if err := sc.shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown returned error: %v", err)
	}

	// Assert: panic せず、クリーンアップ goroutine も収束する
	if c := waitGoroutineGone(rateLimiterCleanupFn, 2*time.Second); c > 0 {
		t.Errorf("cleanup goroutine leaked: %d goroutine(s) still running", c)
	}
}
