// Package middleware はHTTPミドルウェアを提供する。
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hitoshi/feedman/internal/model"
)

const sessionCookieName = "session_id"

// contextKey はコンテキストに値を格納するための型安全なキー。
type contextKey string

// userIDContextKey はリクエストコンテキストにユーザーIDを格納するためのキー。
var userIDContextKey = contextKey("user_id")

// SessionFinder はセッションの検索に必要なインターフェース。
// repository.SessionRepositoryの部分集合として定義する。
type SessionFinder interface {
	FindByID(ctx context.Context, id string) (*model.Session, error)
}

// NewSessionMiddleware はHTTP Only Cookieからセッションを読み取り、
// 有効性を検証するミドルウェアを返す。
// 認証済みユーザーIDをリクエストコンテキストに注入する。
// 未認証リクエストには401 Unauthorizedを返す。
func NewSessionMiddleware(sessionFinder SessionFinder) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. CookieからセッションIDを取得
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 2. セッションの有効性を検証
			session, err := sessionFinder.FindByID(r.Context(), cookie.Value)
			if err != nil {
				slog.Error("failed to find session",
					slog.String("error", err.Error()),
				)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if session == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 3. 認証済みユーザーIDをコンテキストに注入
			ctx := context.WithValue(r.Context(), userIDContextKey, session.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext はリクエストコンテキストからユーザーIDを取得する。
// セッションミドルウェアを通過したリクエストでのみ有効。
func UserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("user ID not found in context")
	}
	return userID, nil
}

// ContextWithUserID はコンテキストにユーザーIDを注入する。
// テストやミドルウェア以外のコンテキスト生成で使用する。
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}
