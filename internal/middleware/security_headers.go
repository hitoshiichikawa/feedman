package middleware

import "net/http"

const (
	// contentSecurityPolicyValue は JSON 専用 API に適した厳格な CSP の値。
	// default-src 'none' で全リソース読み込みを禁止し、frame-ancestors 'none' で
	// クリックジャッキングを防止する。HTTP / HTTPS いずれの配信でも同一値を付与する。
	contentSecurityPolicyValue = "default-src 'none'; frame-ancestors 'none'"

	// strictTransportSecurityValue は HTTPS 配信時に付与する HSTS の値。
	// max-age=1 年・サブドメイン込み。preload は付与しない（運用判断が必要なため）。
	strictTransportSecurityValue = "max-age=31536000; includeSubDomains"
)

// NewSecurityHeadersMiddleware はセキュリティ関連のHTTPレスポンスヘッダーを付与するミドルウェアを返す。
//
// 全レスポンスに以下を常時付与する:
//   - X-Content-Type-Options / X-Frame-Options / Referrer-Policy / Permissions-Policy（既存4ヘッダー）
//   - Content-Security-Policy（JSON 専用 API 向けの厳格な値、HTTP / HTTPS 共通）
//
// hstsEnabled が true かつ HTTPS 配信と判定される場合に限り Strict-Transport-Security を付与する。
// HTTPS 配信判定は、リバースプロキシ配下を想定して X-Forwarded-Proto: https を信頼するほか、
// Go ランタイムが直接 TLS 接続を終端している場合（r.TLS != nil）もフォールバックで HTTPS とみなす。
func NewSecurityHeadersMiddleware(hstsEnabled bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Content-Security-Policy", contentSecurityPolicyValue)

			if hstsEnabled && isHTTPS(r) {
				h.Set("Strict-Transport-Security", strictTransportSecurityValue)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isHTTPS はリクエストが HTTPS 配信であるかを判定する。
//
// 本リポジトリは Cloudflare / リバースプロキシ配下で TLS 終端の内側（平文 HTTP）で
// 動作するため、X-Forwarded-Proto の値が "https" の場合に HTTPS 配信と判定する。
// X-Forwarded-Proto が "https" 以外（"http" または欠落）の場合は HTTP 配信とみなす。
// Go ランタイムが直接 TLS を終端している構成（r.TLS != nil）はフォールバックで HTTPS と判定する。
func isHTTPS(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	return r.TLS != nil
}
