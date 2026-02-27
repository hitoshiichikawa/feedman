package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestSetupMetricsRoute_ReturnsHandler はメトリクスルートのハンドラーが正常に返ることを検証する。
func TestSetupMetricsRoute_ReturnsHandler(t *testing.T) {
	reg := prometheus.NewRegistry()
	_ = NewCollector(reg)

	handler := SetupMetricsRoute(reg)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

// TestSetupMetricsRoute_ServesMetrics は/metricsパスでメトリクスが返ることを検証する。
func TestSetupMetricsRoute_ServesMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)
	c.RecordFetchSuccess("test-feed")

	handler := SetupMetricsRoute(reg)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "feedman_fetch_success_total") {
		t.Error("response should contain feedman_fetch_success_total metric")
	}
}
