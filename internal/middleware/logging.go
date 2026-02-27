package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder はhttp.ResponseWriterをラップし、ステータスコードを記録する。
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader はステータスコードを記録してから委譲する。
func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.written {
		sr.statusCode = code
		sr.written = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

// Write はデータを書き込む。WriteHeaderが未呼び出しの場合は200を記録する。
func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.written {
		sr.statusCode = http.StatusOK
		sr.written = true
	}
	return sr.ResponseWriter.Write(b)
}

// NewLoggingMiddleware はリクエストのJSON構造化ログを出力するミドルウェアを返す。
// ログにはmethod、path、status、duration_ms、user_id（認証済みの場合）を含む。
func NewLoggingMiddleware(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rec := &statusRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(rec, r)

			duration := time.Since(start)
			durationMs := float64(duration.Nanoseconds()) / float64(time.Millisecond)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.statusCode),
				slog.Float64("duration_ms", durationMs),
			}

			// ユーザーIDがコンテキストにある場合は追加
			if userID, err := UserIDFromContext(r.Context()); err == nil && userID != "" {
				attrs = append(attrs, slog.String("user_id", userID))
			}

			// slogのログレベルをステータスコードに応じて変更
			level := slog.LevelInfo
			if rec.statusCode >= 500 {
				level = slog.LevelError
			} else if rec.statusCode >= 400 {
				level = slog.LevelWarn
			}

			// slog.Attr をany スライスに変換
			args := make([]any, len(attrs))
			for i, attr := range attrs {
				args[i] = attr
			}

			logger.Log(r.Context(), level, "http_request", args...)
		})
	}
}
