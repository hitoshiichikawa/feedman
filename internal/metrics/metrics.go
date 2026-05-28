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
//
// 手動フェッチ系（Record ManualFetch*）は subscription.Service.ManualFetch（Issue #115）
// から呼び出され、自動フェッチ系（RecordFetchSuccess / RecordFetchFailure 等）とは別の
// メトリクス系列（feedman_manual_fetch_total）に集計される（Req 8.1〜8.5）。
type MetricsCollector interface {
	RecordFetchSuccess(feedID string)
	RecordFetchFailure(feedID string, reason string)
	RecordParseFailure(feedID string)
	RecordHTTPStatus(statusCode int)
	RecordFetchLatency(duration time.Duration)
	RecordItemsUpserted(count int)
	RecordManualFetchSuccess()
	RecordManualFetchFailure(reason string)
	RecordManualFetchCooldownRejected()
	RecordManualFetchLockConflict()
}

// Collector はPrometheusメトリクスを収集する実装。
type Collector struct {
	fetchSuccess     prometheus.Counter
	fetchFail        prometheus.Counter
	parseFail        prometheus.Counter
	httpStatus       *prometheus.CounterVec
	fetchLatency     prometheus.Histogram
	itemsUpserted    prometheus.Counter
	manualFetchTotal *prometheus.CounterVec
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
		manualFetchTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "feedman_manual_fetch_total",
			Help: "手動フェッチの実行回数（result ラベルで成功・失敗カテゴリ・拒否を区別）",
		}, []string{"result"}),
	}

	reg.MustRegister(
		c.fetchSuccess,
		c.fetchFail,
		c.parseFail,
		c.httpStatus,
		c.fetchLatency,
		c.itemsUpserted,
		c.manualFetchTotal,
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

// manualFetchResult* は feedman_manual_fetch_total の result ラベル値（Req 8.1〜8.4）。
// 直接 string をハードコードせず定数化することで、誤字混入と将来のラベル追加を局所化する。
const (
	manualFetchResultSuccess          = "success"
	manualFetchResultCooldownRejected = "cooldown_rejected"
	manualFetchResultLockConflict     = "lock_conflict"
)

// RecordManualFetchSuccess は手動フェッチが成功したことを記録する（Req 8.1）。
// result="success" ラベルで feedman_manual_fetch_total を 1 増加させる。
func (c *Collector) RecordManualFetchSuccess() {
	c.manualFetchTotal.WithLabelValues(manualFetchResultSuccess).Inc()
}

// RecordManualFetchFailure は手動フェッチが失敗したことを記録する（Req 8.2）。
// reason は失敗カテゴリ（fetch_error / parse_error / upsert_error / ssrf_blocked）で、
// 呼び出し側（subscription.Service.ManualFetch）が値域を決定する。本メソッドは reason 文字列を
// そのまま result ラベルに通す（値域の whitelist 化はサービス層の責務）。
func (c *Collector) RecordManualFetchFailure(reason string) {
	c.manualFetchTotal.WithLabelValues(reason).Inc()
}

// RecordManualFetchCooldownRejected は手動フェッチがクールダウン中で拒否されたことを記録する（Req 8.3）。
// result="cooldown_rejected" ラベルで feedman_manual_fetch_total を 1 増加させる。
func (c *Collector) RecordManualFetchCooldownRejected() {
	c.manualFetchTotal.WithLabelValues(manualFetchResultCooldownRejected).Inc()
}

// RecordManualFetchLockConflict は手動フェッチが行ロック競合で拒否されたことを記録する（Req 8.4）。
// result="lock_conflict" ラベルで feedman_manual_fetch_total を 1 増加させる。
func (c *Collector) RecordManualFetchLockConflict() {
	c.manualFetchTotal.WithLabelValues(manualFetchResultLockConflict).Inc()
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
