package handler

import "net/http"

// buildSessionCookie は Web 用 session Cookie（`session_id`）の canonical builder である
// （Issue #231 design.md §Delta 4 §Cookie 属性の維持）。
//
// Google OAuth Callback（auth_handler.go）・パスキーログイン session 交換
// （native_auth_handler.go::Session）・パスキー Web 登録 finish（passkey_handler.go）の
// 3 経路が **完全同一属性** の Cookie を発行することを 1 箇所で保証し、属性の重複記述と
// 将来のドリフトを排除する。Name / HttpOnly / SameSite / Path は固定、Domain / Secure /
// Max-Age のみ呼び出し側が wiring 時の設定値（cfg.CookieDomain / cfg.CookieSecure /
// cfg.SessionMaxAge）を渡す。
//
// 引数:
//   - name:   Cookie 名（呼び出し側は sessionCookieName を渡す）
//   - value:  session_id 生値（Set-Cookie ヘッダ経由でのみクライアントへ渡す / ログには残さない）
//   - domain: Cookie の Domain 属性（空文字は同一ドメイン限定を意味する有効な構成）
//   - secure: Secure 属性（本番 https では true）
//   - maxAge: Max-Age（秒）
func buildSessionCookie(name, value, domain string, secure bool, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}
