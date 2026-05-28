package metrics

import "time"

// NopCollector は MetricsCollector の no-op 実装。
// Collector が未注入のフェッチャー／サービス層で、nil チェックを書かずに
// 既定値として安全に利用するために使う（メトリクスを一切記録しない）。
type NopCollector struct{}

// RecordFetchSuccess は何も記録しない。
func (NopCollector) RecordFetchSuccess(feedID string) {}

// RecordFetchFailure は何も記録しない。
func (NopCollector) RecordFetchFailure(feedID, reason string) {}

// RecordParseFailure は何も記録しない。
func (NopCollector) RecordParseFailure(feedID string) {}

// RecordHTTPStatus は何も記録しない。
func (NopCollector) RecordHTTPStatus(statusCode int) {}

// RecordFetchLatency は何も記録しない。
func (NopCollector) RecordFetchLatency(duration time.Duration) {}

// RecordItemsUpserted は何も記録しない。
func (NopCollector) RecordItemsUpserted(count int) {}
