package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// NewRecoveryMiddleware はpanic発生時にプロセスクラッシュを防ぎ、
// 500レスポンスを返すミドルウェアを生成する。
func NewRecoveryMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("panic recovered",
						slog.Any("panic", rec),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("stack", string(debug.Stack())),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
