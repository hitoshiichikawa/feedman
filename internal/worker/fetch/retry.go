package fetch

import (
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// FetchResult はHTTPステータスコードに基づくフェッチ結果の分類。
type FetchResult int

const (
	// FetchResultOK はフェッチ成功（200）。
	FetchResultOK FetchResult = iota
	// FetchResultNotModified はコンテンツ未変更（304）。
	FetchResultNotModified
	// FetchResultStop はフェッチ停止が必要なステータス（404/410/401/403）。
	FetchResultStop
	// FetchResultBackoff はバックオフが必要なステータス（429/5xx）。
	FetchResultBackoff
	// FetchResultUnknown は未知のステータスコード。
	FetchResultUnknown
)

const (
	// initialBackoff は指数バックオフの初回遅延（30分）。
	initialBackoff = 30 * time.Minute
	// maxBackoff は指数バックオフの最大遅延（12時間）。
	maxBackoff = 12 * time.Hour
	// parseFailureThreshold はパース失敗によるフェッチ停止の閾値。
	parseFailureThreshold = 10
)

// ClassifyHTTPStatus はHTTPステータスコードをフェッチ結果に分類する。
func ClassifyHTTPStatus(statusCode int) FetchResult {
	switch {
	case statusCode == 200:
		return FetchResultOK
	case statusCode == 304:
		return FetchResultNotModified
	case statusCode == 404 || statusCode == 410:
		return FetchResultStop
	case statusCode == 401 || statusCode == 403:
		return FetchResultStop
	case statusCode == 429:
		return FetchResultBackoff
	case statusCode >= 500:
		return FetchResultBackoff
	default:
		return FetchResultUnknown
	}
}

// CalculateBackoff は連続エラー回数に基づいて指数バックオフ遅延を計算する。
// 初回30分、2倍ずつ増加、最大12時間。
func CalculateBackoff(consecutiveErrors int) time.Duration {
	delay := initialBackoff
	for i := 0; i < consecutiveErrors; i++ {
		delay *= 2
		if delay > maxBackoff {
			return maxBackoff
		}
	}
	return delay
}

// ApplyStopFeed はフィードのフェッチを停止する。
// fetch_statusをstoppedに設定し、エラーメッセージを記録する。
func ApplyStopFeed(feed *model.Feed, reason string) {
	feed.FetchStatus = model.FetchStatusStopped
	feed.ErrorMessage = reason
	feed.UpdatedAt = time.Now()
}

// ApplyBackoff はフィードにバックオフ戦略を適用する。
// 連続エラー回数をインクリメントし、指数バックオフでnext_fetch_atを設定する。
func ApplyBackoff(feed *model.Feed, reason string) {
	feed.ConsecutiveErrors++
	feed.ErrorMessage = reason
	delay := CalculateBackoff(feed.ConsecutiveErrors - 1)
	feed.NextFetchAt = time.Now().Add(delay)
	feed.UpdatedAt = time.Now()
}

// ApplySuccess はフェッチ成功時にフィードの状態をリセットする。
// 連続エラー回数を0にリセットし、エラーメッセージをクリアする。
// intervalMinutesに基づいてnext_fetch_atを設定する。
func ApplySuccess(feed *model.Feed, intervalMinutes int) {
	feed.ConsecutiveErrors = 0
	feed.ErrorMessage = ""
	feed.NextFetchAt = time.Now().Add(time.Duration(intervalMinutes) * time.Minute)
	feed.UpdatedAt = time.Now()
}

// CheckParseFailureThreshold はパース失敗回数が閾値に達しているかを確認する。
func CheckParseFailureThreshold(feed *model.Feed) bool {
	return feed.ConsecutiveErrors >= parseFailureThreshold
}

// ApplyParseFailure はパース失敗時にフィードの連続エラー回数をインクリメントする。
// 閾値に達した場合はフェッチを停止する。
func ApplyParseFailure(feed *model.Feed, reason string) {
	feed.ConsecutiveErrors++
	feed.ErrorMessage = fmt.Sprintf("パース失敗 (%d回連続): %s", feed.ConsecutiveErrors, reason)
	feed.UpdatedAt = time.Now()

	if CheckParseFailureThreshold(feed) {
		feed.FetchStatus = model.FetchStatusStopped
		feed.ErrorMessage = fmt.Sprintf("パース失敗が%d回連続したためフェッチを停止しました: %s", feed.ConsecutiveErrors, reason)
	}
}
