package middleware

import "net/http"

// DefaultMaxBodyBytes は API リクエストボディのデフォルト上限（1 MiB）。
// フィード登録・記事状態更新などの JSON ペイロードは数百バイト程度であり、
// この上限は十分な余裕を持ちつつ、巨大ボディによるメモリ枯渇 DoS を防ぐ。
const DefaultMaxBodyBytes int64 = 1 << 20

// NewMaxBodyBytesMiddleware は各リクエストの Body を http.MaxBytesReader で limit バイトに
// 制限するミドルウェアを返す。
//
// 上限を超えた場合、ハンドラ側の Body 読み取り（json.Decode 等）がエラーを返し、
// 各ハンドラの既存エラーハンドリング（400 応答）に委ねられる。GET など Body を持たない
// リクエストには影響しない。limit <= 0 の場合は素通しする（後方互換）。
func NewMaxBodyBytesMiddleware(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limit <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
