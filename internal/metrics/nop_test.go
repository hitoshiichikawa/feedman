package metrics

import (
	"testing"
	"time"
)

// TestNopCollector_ImplementsMetricsCollector は NopCollector が
// MetricsCollector インターフェースを満たすことをコンパイル時に検証する。
// Requirement 5.1（既存挙動の後方互換維持）/ NFR 1.2 に対応。
func TestNopCollector_ImplementsMetricsCollector(t *testing.T) {
	// Arrange / Act
	var c MetricsCollector = NopCollector{}

	// Assert
	if c == nil {
		t.Fatal("NopCollector should satisfy MetricsCollector interface")
	}
}

// TestNopCollector_MethodsDoNotPanic は NopCollector の全 10 メソッドが
// panic せず副作用なく呼べることを検証する。
// Collector 未注入時の既定値として nil 安全に振る舞うことを担保する。
// Requirement 5.1 / NFR 1.2 / Issue #115 Req 8.1〜8.4 に対応。
func TestNopCollector_MethodsDoNotPanic(t *testing.T) {
	cases := []struct {
		name string
		call func(c NopCollector)
	}{
		{
			name: "RecordFetchSuccessを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordFetchSuccess("feed-1") },
		},
		{
			name: "RecordFetchFailureを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordFetchFailure("feed-1", "timeout") },
		},
		{
			name: "RecordParseFailureを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordParseFailure("feed-1") },
		},
		{
			name: "RecordHTTPStatusを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordHTTPStatus(200) },
		},
		{
			name: "RecordFetchLatencyを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordFetchLatency(150 * time.Millisecond) },
		},
		{
			name: "RecordItemsUpsertedを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordItemsUpserted(3) },
		},
		{
			name: "RecordManualFetchSuccessを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordManualFetchSuccess() },
		},
		{
			name: "RecordManualFetchFailureを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordManualFetchFailure("fetch_error") },
		},
		{
			name: "RecordManualFetchCooldownRejectedを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordManualFetchCooldownRejected() },
		},
		{
			name: "RecordManualFetchLockConflictを呼んでもpanicしない",
			call: func(c NopCollector) { c.RecordManualFetchLockConflict() },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			c := NopCollector{}

			// Act & Assert: panic しないことを検証する
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("expected no panic, got %v", r)
				}
			}()
			tc.call(c)
		})
	}
}

// TestNopCollector_ZeroValueIsUsable は NopCollector のゼロ値が
// そのまま利用可能（境界値: 空文字 feedID / 0 件 / ステータス 0）であることを検証する。
// Requirement 5.1 / NFR 1.2 に対応。
func TestNopCollector_ZeroValueIsUsable(t *testing.T) {
	// Arrange
	var c NopCollector

	// Act & Assert
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("expected no panic with boundary inputs, got %v", r)
		}
	}()
	c.RecordFetchSuccess("")
	c.RecordFetchFailure("", "")
	c.RecordParseFailure("")
	c.RecordHTTPStatus(0)
	c.RecordFetchLatency(0)
	c.RecordItemsUpserted(0)
	c.RecordManualFetchSuccess()
	c.RecordManualFetchFailure("")
	c.RecordManualFetchCooldownRejected()
	c.RecordManualFetchLockConflict()
}
