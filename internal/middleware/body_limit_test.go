package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewMaxBodyBytesMiddleware は上限以下/超過/素通しの各ケースを検証する。
func TestNewMaxBodyBytesMiddleware(t *testing.T) {
	// ハンドラは Body を全量読み取り、エラー有無を記録する。
	readBody := func(readErr *error) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			*readErr = err
			w.WriteHeader(http.StatusOK)
		})
	}

	t.Run("上限以下のボディは読み取りに成功すること", func(t *testing.T) {
		// Arrange
		var readErr error
		mw := NewMaxBodyBytesMiddleware(1024)
		h := mw(readBody(&readErr))
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 512)))
		rec := httptest.NewRecorder()

		// Act
		h.ServeHTTP(rec, req)

		// Assert
		if readErr != nil {
			t.Errorf("上限以下なのに読み取りエラー: %v", readErr)
		}
	})

	t.Run("上限を超えるボディは読み取りでエラーになること", func(t *testing.T) {
		// Arrange
		var readErr error
		mw := NewMaxBodyBytesMiddleware(16)
		h := mw(readBody(&readErr))
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 1024)))
		rec := httptest.NewRecorder()

		// Act
		h.ServeHTTP(rec, req)

		// Assert
		if readErr == nil {
			t.Error("上限超過なのに読み取りエラーが発生しなかった")
		}
	})

	t.Run("limit が 0 のとき素通しすること", func(t *testing.T) {
		// Arrange
		var readErr error
		mw := NewMaxBodyBytesMiddleware(0)
		h := mw(readBody(&readErr))
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 100_000)))
		rec := httptest.NewRecorder()

		// Act
		h.ServeHTTP(rec, req)

		// Assert: 上限なしなので全量読み取れる
		if readErr != nil {
			t.Errorf("limit=0（素通し）なのに読み取りエラー: %v", readErr)
		}
	})
}
