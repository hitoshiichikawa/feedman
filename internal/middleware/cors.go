package middleware

import "net/http"

// NewCORSMiddleware は指定されたオリジンに対するCORSミドルウェアを返す。
// credentials送信と共存するため、ワイルドカード(*)は使用しない。
// OPTIONSプリフライトリクエストには204で応答する。
func NewCORSMiddleware(allowedOrigin string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// OPTIONSプリフライトリクエストには204で応答
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
