package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// JWTVerifier は BearerOrSession middleware が access token の検証に必要とする
// 最小インターフェース（Issue #169）。auth.JWTVerifier が構造的に充足する
// （SessionFinder と同じ interface segregation パターン）。
type JWTVerifier interface {
	// VerifyAccessToken は tokenString を検証し、成立時に userID を返す。
	VerifyAccessToken(tokenString string) (string, error)
}

// NewBearerOrSessionMiddleware は Bearer 優先・Session 委譲の複合認証 middleware を返す
// （Issue #169）。
//
// 判定フロー（design.md「判定フロー」参照）:
//  0. jwtVerifier == nil（NATIVE_AUTH_JWT_SECRET 未設定）
//     → NewSessionMiddleware(sessionFinder) をそのまま返す縮退（Req 4.2 / 4.3:
//     全要求が従来どおり Cookie 評価。Authorization ヘッダは一切読まない）
//  1. Authorization ヘッダ無し → 既存 SessionMiddleware に委譲（Req 3.1: 従来パス完全維持）
//  2. ヘッダの scheme が Bearer 以外（Basic 等）→ 1 と同じく委譲（Req 3.2。
//     従来も Authorization を無視していたため互換）
//  3. scheme が Bearer（大文字小文字不区別 / RFC 9110 §11.1）:
//     a. token 部（trim 後）が空 → 401（Req 2.5。委譲しない）
//     b. VerifyAccessToken 失敗 → 401（Req 2.4: Cookie へ fallback しない）
//     c. 成功 → ContextWithUserID で user context 注入 → next（Req 1.1 / 1.3:
//     sessionFinder は呼ばない）
//
// 401 応答は既存 SessionMiddleware の未認証応答と同一形式
// （http.Error(w, "unauthorized", http.StatusUnauthorized) / Req 2.6）であり、
// 署名不正・期限切れ・用途不一致・形式不正の別を応答から区別できない（Req 2.7）。
// 検証失敗は slog.Warn にエラー理由のみ記録し、token 文字列・claims 値は渡さない
// （NFR 1.1）。
func NewBearerOrSessionMiddleware(jwtVerifier JWTVerifier, sessionFinder SessionFinder) func(next http.Handler) http.Handler {
	// 0. verifier 未設定環境では既存 SessionMiddleware をそのまま返す
	//    （構成が本機能導入前と文字どおり同一になる fail-safe 縮退）。
	if jwtVerifier == nil {
		return NewSessionMiddleware(sessionFinder)
	}

	sessionMW := NewSessionMiddleware(sessionFinder)
	return func(next http.Handler) http.Handler {
		delegate := sessionMW(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Authorization ヘッダ無し → Cookie セッション認証へ委譲
			authz := r.Header.Get("Authorization")
			if authz == "" {
				delegate.ServeHTTP(w, r)
				return
			}

			// 2. scheme 分離。Bearer 以外（Basic 等）は従来どおり無視して委譲
			scheme, token, _ := strings.Cut(authz, " ")
			if !strings.EqualFold(scheme, "Bearer") {
				delegate.ServeHTTP(w, r)
				return
			}

			// 3a. Bearer scheme で token 部が空（"Bearer" 単独を含む）→ 401
			token = strings.TrimSpace(token)
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 3b. 検証失敗 → 401（有効な Cookie が併送されていても fallback しない）
			userID, err := jwtVerifier.VerifyAccessToken(token)
			if err != nil {
				// token 文字列・claims 値はログに渡さない（NFR 1.1）。
				slog.Warn("bearer token rejected", slog.String("error", err.Error()))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 3c. 成功 → Cookie セッション認証と同一の context key に userID を注入
			next.ServeHTTP(w, r.WithContext(ContextWithUserID(r.Context(), userID)))
		})
	}
}
