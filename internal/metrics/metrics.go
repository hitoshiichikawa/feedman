// Package metrics はPrometheusメトリクスの収集と公開を提供する。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsCollector はメトリクス収集のインターフェース。
// ワーカーやサービス層から利用する。
type MetricsCollector interface {
	RecordFetchSuccess(feedID string)
	RecordFetchFailure(feedID string, reason string)
	RecordParseFailure(feedID string)
	RecordHTTPStatus(statusCode int)
	RecordFetchLatency(duration time.Duration)
	RecordItemsUpserted(count int)
}

// Collector はPrometheusメトリクスを収集する実装。
type Collector struct {
	fetchSuccess  prometheus.Counter
	fetchFail     prometheus.Counter
	parseFail     prometheus.Counter
	httpStatus    *prometheus.CounterVec
	fetchLatency  prometheus.Histogram
	itemsUpserted prometheus.Counter
}

// NewCollector は新しいCollectorを生成し、指定されたレジストリにメトリクスを登録する。
func NewCollector(reg prometheus.Registerer) *Collector {
	c := &Collector{
		fetchSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "feedman_fetch_success_total",
			Help: "フィードフェッチ成功の合計数",
		}),
		fetchFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "feedman_fetch_fail_total",
			Help: "フィードフェッチ失敗の合計数",
		}),
		parseFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "feedman_parse_fail_total",
			Help: "フィードパース失敗の合計数",
		}),
		httpStatus: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "feedman_http_status_total",
			Help: "HTTPステータスコード別のレスポンス数",
		}, []string{"status_code"}),
		fetchLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "feedman_fetch_latency_seconds",
			Help:    "フィードフェッチのレイテンシ（秒）",
			Buckets: prometheus.DefBuckets,
		}),
		itemsUpserted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "feedman_items_upserted_total",
			Help: "アップサートされた記事の合計数",
		}),
	}

	reg.MustRegister(
		c.fetchSuccess,
		c.fetchFail,
		c.parseFail,
		c.httpStatus,
		c.fetchLatency,
		c.itemsUpserted,
	)

	return c
}

// RecordFetchSuccess はフェッチ成功を記録する。
func (c *Collector) RecordFetchSuccess(feedID string) {
	c.fetchSuccess.Inc()
}

// RecordFetchFailure はフェッチ失敗を記録する。
func (c *Collector) RecordFetchFailure(feedID string, reason string) {
	c.fetchFail.Inc()
}

// RecordParseFailure はパース失敗を記録する。
func (c *Collector) RecordParseFailure(feedID string) {
	c.parseFail.Inc()
}

// RecordHTTPStatus はHTTPステータスコードを記録する。
func (c *Collector) RecordHTTPStatus(statusCode int) {
	c.httpStatus.WithLabelValues(strconv.Itoa(statusCode)).Inc()
}

// RecordFetchLatency はフェッチのレイテンシを記録する。
func (c *Collector) RecordFetchLatency(duration time.Duration) {
	c.fetchLatency.Observe(duration.Seconds())
}

// RecordItemsUpserted はアップサートされた記事数を記録する。
func (c *Collector) RecordItemsUpserted(count int) {
	c.itemsUpserted.Add(float64(count))
}

// Handler はPrometheusスクレイプ用のHTTPハンドラーを返す。
func Handler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

// SetupMetricsRoute は/metricsエンドポイントを提供するHTTPハンドラーを返す。
// Prometheusスクレイプに対応する。
func SetupMetricsRoute(gatherer prometheus.Gatherer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler(gatherer))
	return mux
}
