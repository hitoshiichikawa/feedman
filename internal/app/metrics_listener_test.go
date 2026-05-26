package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/hitoshi/feedman/internal/metrics"
)

// freePort は OS に空きポートを割り当てさせてアドレス文字列（127.0.0.1:NNNN）を返す。
// 取得後すぐにリスナーを閉じるため、僅かな TOCTOU はあるが flaky になりにくい。
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("空きポートの確保に失敗: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// getMetrics は addr の /metrics へ範囲内 IP（loopback）からアクセスし、本文とステータスを返す。
// 起動直後で listener がまだ準備中の場合に備えて短時間ポーリングする。
func getMetrics(t *testing.T, addr string) (int, string) {
	t.Helper()
	url := "http://" + addr + "/metrics"
	client := &http.Client{Timeout: 2 * time.Second}

	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return resp.StatusCode, string(body)
		}
		if time.Now().After(deadline) {
			t.Fatalf("/metrics への接続がタイムアウト: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// startupSeriesNames は記録なしの起動直後でも /metrics に出力される 5 系列名。
// feedman_http_status_total はラベル付き CounterVec のため、ラベル値（ステータスコード）が
// 1 度も記録されるまで Prometheus テキスト出力に現れない。当該系列は
// TestStartWorkerMetricsListener_HTTPStatusSeriesAppearsAfterRecord で別途検証する。
var startupSeriesNames = []string{
	"feedman_fetch_success_total",
	"feedman_fetch_fail_total",
	"feedman_parse_fail_total",
	"feedman_fetch_latency_seconds",
	"feedman_items_upserted_total",
}

// TestStartWorkerMetricsListener_StartupExposesSeries は起動直後（未記録）でも
// /metrics が 200 を返し、ラベルなし 5 系列名を含むことを検証する（Requirement 1.3, 5.3, NFR 1.1）。
func TestStartWorkerMetricsListener_StartupExposesSeries(t *testing.T) {
	// Arrange
	reg := prometheus.NewRegistry()
	_ = metrics.NewCollector(reg)
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Act
	startWorkerMetricsListener(ctx, addr, reg, []string{"127.0.0.0/8"})
	status, body := getMetrics(t, addr)

	// Assert
	if status != http.StatusOK {
		t.Fatalf("起動直後の /metrics status = %d, want 200", status)
	}
	for _, name := range startupSeriesNames {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics 応答に系列名 %q が含まれていない", name)
		}
	}
}

// TestStartWorkerMetricsListener_HTTPStatusSeriesAppearsAfterRecord は
// HTTP ステータスを 1 度記録すると feedman_http_status_total 系列が /metrics に現れることを
// 検証する（Requirement 1.3 の 6 系列目。CounterVec はラベル値記録後に出力されるため）。
func TestStartWorkerMetricsListener_HTTPStatusSeriesAppearsAfterRecord(t *testing.T) {
	// Arrange
	reg := prometheus.NewRegistry()
	collector := metrics.NewCollector(reg)
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWorkerMetricsListener(ctx, addr, reg, []string{"127.0.0.0/8"})

	// Act: HTTP ステータス 200 を記録する
	collector.RecordHTTPStatus(http.StatusOK)
	status, body := getMetrics(t, addr)

	// Assert
	if status != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", status)
	}
	if !strings.Contains(body, "feedman_http_status_total") {
		t.Errorf("ステータス記録後の /metrics 応答に feedman_http_status_total が含まれていない。本文:\n%s", body)
	}
}

// TestStartWorkerMetricsListener_FetchSuccessReflected はフェッチ成功記録後に
// /metrics 応答へ feedman_fetch_success_total の増加が反映されることを検証する（Requirement 3.1, 3.2）。
func TestStartWorkerMetricsListener_FetchSuccessReflected(t *testing.T) {
	// Arrange
	reg := prometheus.NewRegistry()
	collector := metrics.NewCollector(reg)
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWorkerMetricsListener(ctx, addr, reg, []string{"127.0.0.0/8"})

	// Act: フェッチ成功を 1 件記録する
	collector.RecordFetchSuccess("feed-1")
	status, body := getMetrics(t, addr)

	// Assert
	if status != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", status)
	}
	if !strings.Contains(body, "feedman_fetch_success_total 1") {
		t.Errorf("/metrics 応答に feedman_fetch_success_total 1 が反映されていない。本文:\n%s", body)
	}
}

// TestStartWorkerMetricsListener_OutOfRangeForbidden は CIDR 範囲外の listener では
// loopback からのアクセスが 403 になることを検証する（Requirement 4.1, NFR 2.2）。
func TestStartWorkerMetricsListener_OutOfRangeForbidden(t *testing.T) {
	// Arrange: loopback を含まない CIDR を設定する
	reg := prometheus.NewRegistry()
	_ = metrics.NewCollector(reg)
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWorkerMetricsListener(ctx, addr, reg, []string{"10.0.0.0/8"})

	// Act
	status, body := getMetrics(t, addr)

	// Assert
	if status != http.StatusForbidden {
		t.Errorf("範囲外 listener の /metrics status = %d, want 403", status)
	}
	if strings.Contains(body, "feedman_fetch_success_total") {
		t.Errorf("403 応答にメトリクス本文が含まれている: %q", body)
	}
}

// TestStartWorkerMetricsListener_CtxCancelStopsListener は ctx キャンセルで listener が
// graceful stop し、以降の接続が拒否されることを検証する（goroutine リーク防止）。
func TestStartWorkerMetricsListener_CtxCancelStopsListener(t *testing.T) {
	// Arrange
	reg := prometheus.NewRegistry()
	_ = metrics.NewCollector(reg)
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	startWorkerMetricsListener(ctx, addr, reg, []string{"127.0.0.0/8"})

	// 起動を確認してからキャンセルする
	if status, _ := getMetrics(t, addr); status != http.StatusOK {
		t.Fatalf("キャンセル前の /metrics status = %d, want 200", status)
	}

	// Act
	cancel()

	// Assert: shutdown 後は接続が確立できなくなる（短時間ポーリングで確認）
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	stopped := false
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/metrics")
		if err != nil {
			stopped = true
			break
		}
		_ = resp.Body.Close()
		time.Sleep(20 * time.Millisecond)
	}
	if !stopped {
		t.Error("ctx キャンセル後も listener が応答し続けている（graceful stop されていない）")
	}
}
