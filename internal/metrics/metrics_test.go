package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// TestNewCollector_ReturnsNonNil はCollectorが正常に生成されることを検証する。
func TestNewCollector_ReturnsNonNil(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	if c == nil {
		t.Fatal("expected non-nil Collector")
	}
}

// TestRecordFetchSuccess_IncrementsCounter はフェッチ成功カウンタが増加することを検証する。
func TestRecordFetchSuccess_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	c.RecordFetchSuccess("feed-1")
	c.RecordFetchSuccess("feed-1")

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "feedman_fetch_success_total" {
			found = true
			if len(mf.GetMetric()) != 1 {
				t.Fatalf("expected 1 metric, got %d", len(mf.GetMetric()))
			}
			val := mf.GetMetric()[0].GetCounter().GetValue()
			if val != 2 {
				t.Errorf("fetch_success_total = %v, want 2", val)
			}
		}
	}
	if !found {
		t.Error("feedman_fetch_success_total metric not found")
	}
}

// TestRecordFetchFailure_IncrementsCounter はフェッチ失敗カウンタが増加することを検証する。
func TestRecordFetchFailure_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	c.RecordFetchFailure("feed-2", "timeout")

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "feedman_fetch_fail_total" {
			found = true
			val := mf.GetMetric()[0].GetCounter().GetValue()
			if val != 1 {
				t.Errorf("fetch_fail_total = %v, want 1", val)
			}
		}
	}
	if !found {
		t.Error("feedman_fetch_fail_total metric not found")
	}
}

// TestRecordParseFailure_IncrementsCounter はパース失敗カウンタが増加することを検証する。
func TestRecordParseFailure_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	c.RecordParseFailure("feed-3")
	c.RecordParseFailure("feed-3")
	c.RecordParseFailure("feed-3")

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "feedman_parse_fail_total" {
			found = true
			val := mf.GetMetric()[0].GetCounter().GetValue()
			if val != 3 {
				t.Errorf("parse_fail_total = %v, want 3", val)
			}
		}
	}
	if !found {
		t.Error("feedman_parse_fail_total metric not found")
	}
}

// TestRecordHTTPStatus_IncrementsCounterWithLabel はHTTPステータスカウンタがラベル付きで増加することを検証する。
func TestRecordHTTPStatus_IncrementsCounterWithLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	c.RecordHTTPStatus(200)
	c.RecordHTTPStatus(200)
	c.RecordHTTPStatus(404)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "feedman_http_status_total" {
			found = true
			if len(mf.GetMetric()) != 2 {
				t.Fatalf("expected 2 label combinations, got %d", len(mf.GetMetric()))
			}
			for _, m := range mf.GetMetric() {
				label := m.GetLabel()[0].GetValue()
				val := m.GetCounter().GetValue()
				switch label {
				case "200":
					if val != 2 {
						t.Errorf("http_status_total{status_code=200} = %v, want 2", val)
					}
				case "404":
					if val != 1 {
						t.Errorf("http_status_total{status_code=404} = %v, want 1", val)
					}
				default:
					t.Errorf("unexpected label value: %s", label)
				}
			}
		}
	}
	if !found {
		t.Error("feedman_http_status_total metric not found")
	}
}

// TestRecordFetchLatency_ObservesHistogram はフェッチレイテンシのヒストグラムに値が記録されることを検証する。
func TestRecordFetchLatency_ObservesHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	c.RecordFetchLatency(100 * time.Millisecond)
	c.RecordFetchLatency(2 * time.Second)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "feedman_fetch_latency_seconds" {
			found = true
			h := mf.GetMetric()[0].GetHistogram()
			if h.GetSampleCount() != 2 {
				t.Errorf("sample_count = %d, want 2", h.GetSampleCount())
			}
			// 合計は0.1 + 2.0 = 2.1秒
			if h.GetSampleSum() < 2.0 || h.GetSampleSum() > 2.2 {
				t.Errorf("sample_sum = %v, want ~2.1", h.GetSampleSum())
			}
		}
	}
	if !found {
		t.Error("feedman_fetch_latency_seconds metric not found")
	}
}

// TestRecordItemsUpserted_IncrementsCounter は記事アップサートカウンタが増加することを検証する。
func TestRecordItemsUpserted_IncrementsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	c.RecordItemsUpserted(10)
	c.RecordItemsUpserted(5)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "feedman_items_upserted_total" {
			found = true
			val := mf.GetMetric()[0].GetCounter().GetValue()
			if val != 15 {
				t.Errorf("items_upserted_total = %v, want 15", val)
			}
		}
	}
	if !found {
		t.Error("feedman_items_upserted_total metric not found")
	}
}

// TestMetricsHandler_ReturnsPrometheusFormat は/metricsエンドポイントがPrometheus形式で返すことを検証する。
func TestMetricsHandler_ReturnsPrometheusFormat(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCollector(reg)

	// いくつかのメトリクスを記録
	c.RecordFetchSuccess("feed-test")
	c.RecordFetchFailure("feed-test", "error")
	c.RecordHTTPStatus(200)
	c.RecordFetchLatency(500 * time.Millisecond)
	c.RecordItemsUpserted(3)

	handler := Handler(reg)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Prometheus形式のメトリクスが含まれていることを確認
	expectedMetrics := []string{
		"feedman_fetch_success_total",
		"feedman_fetch_fail_total",
		"feedman_http_status_total",
		"feedman_fetch_latency_seconds",
		"feedman_items_upserted_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("response body does not contain %q", metric)
		}
	}
}

// TestCollector_ImplementsMetricsCollectorInterface はCollectorがMetricsCollectorインターフェースを実装することを検証する。
func TestCollector_ImplementsMetricsCollectorInterface(t *testing.T) {
	reg := prometheus.NewRegistry()
	var _ MetricsCollector = NewCollector(reg)
}

// TestMultipleCollectors_IndependentRegistries は異なるレジストリで独立に動作することを検証する。
func TestMultipleCollectors_IndependentRegistries(t *testing.T) {
	reg1 := prometheus.NewRegistry()
	reg2 := prometheus.NewRegistry()
	c1 := NewCollector(reg1)
	c2 := NewCollector(reg2)

	c1.RecordFetchSuccess("feed-a")
	c2.RecordFetchSuccess("feed-b")
	c2.RecordFetchSuccess("feed-b")

	metrics1, _ := reg1.Gather()
	metrics2, _ := reg2.Gather()

	var val1, val2 float64
	for _, mf := range metrics1 {
		if mf.GetName() == "feedman_fetch_success_total" {
			val1 = mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	for _, mf := range metrics2 {
		if mf.GetName() == "feedman_fetch_success_total" {
			val2 = mf.GetMetric()[0].GetCounter().GetValue()
		}
	}

	if val1 != 1 {
		t.Errorf("reg1 fetch_success = %v, want 1", val1)
	}
	if val2 != 2 {
		t.Errorf("reg2 fetch_success = %v, want 2", val2)
	}
}
