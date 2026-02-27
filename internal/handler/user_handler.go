package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// UserServiceInterface はユーザーハンドラーが必要とするサービスインターフェース。
type UserServiceInterface interface {
	// Withdraw はユーザーの退会処理を実行する。
	// user、identities、subscriptions、item_states、settingsを一括削除する。
	// feeds、itemsは共有キャッシュとして残す。
	Withdraw(ctx context.Context, userID string) error
}

// UserHandler はユーザー管理のHTTPハンドラー。
type UserHandler struct {
	service UserServiceInterface
}

// NewUserHandler はUserHandlerを生成する。
func NewUserHandler(service UserServiceInterface) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

// Withdraw はユーザーの退会処理を実行する。
// DELETE /api/users/me
func (h *UserHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		writeAPIErrorResponse(w, http.StatusUnauthorized, &model.APIError{
			Code:     "UNAUTHORIZED",
			Message:  "認証が必要です。",
			Category: "auth",
			Action:   "ログインしてください。",
		})
		return
	}

	if err := h.service.Withdraw(r.Context(), userID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetupUserRoutes はユーザー管理関連のルーティングを設定したchi.Routerを返す。
func SetupUserRoutes(service UserServiceInterface) http.Handler {
	r := chi.NewRouter()
	h := NewUserHandler(service)

	r.Route("/api/users", func(r chi.Router) {
		r.Delete("/me", h.Withdraw)
	})

	return r
}
