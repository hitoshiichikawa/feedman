package handler

import (
	"context"
	"encoding/json"
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
	// GetCurrent は指定 userID の current user 情報をレスポンス用 DTO で返す。
	// userID は middleware（BearerOrSession）が解決したものを呼び出し側が渡す。
	// userID が DB 上に存在しない場合は model.NewUserNotFoundError を返す。
	GetCurrent(ctx context.Context, userID string) (*currentUserResponse, error)
}

// currentUserResponse は GET /api/users/me の応答 JSON 表現。
// avatar_url は v1 では常に nil（DB 未拡張）であり omitempty で省略される
// （Req 2.3 / 2.4 / design.md「設計判断: avatar_url を当面 nil 固定で返す」節）。
// session_id / refresh_token などの secret は絶対に含めない
// （CLAUDE.md「機能追加チェックリスト」/「機密情報の扱い」）。
type currentUserResponse struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url,omitempty"`
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
		middleware.WriteErrorResponse(w, http.StatusUnauthorized, &model.APIError{
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

// GetCurrent は現在ログイン中のユーザー情報を JSON で返す。
// GET /api/users/me
//
// BearerOrSession middleware を通過した後の経路（Authorization: Bearer / Cookie
// session_id のいずれも）で同じ handler を呼ぶため、handler 自体は経路を区別しない
// （Req 2.1 / 2.2）。middleware が解決した userID を context から取得して service に渡す。
//
// 応答キー集合は {id, email, name} ⊆ X ⊆ {id, email, name, avatar_url}（Req 2.3 / 2.4）。
func (h *UserHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		middleware.WriteErrorResponse(w, http.StatusUnauthorized, &model.APIError{
			Code:     "UNAUTHORIZED",
			Message:  "認証が必要です。",
			Category: "auth",
			Action:   "ログインしてください。",
		})
		return
	}

	resp, err := h.service.GetCurrent(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// SetupUserRoutes はユーザー管理関連のルーティングを設定したchi.Routerを返す。
func SetupUserRoutes(service UserServiceInterface) http.Handler {
	r := chi.NewRouter()
	h := NewUserHandler(service)

	r.Route("/api/users", func(r chi.Router) {
		r.Get("/me", h.GetCurrent)
		r.Delete("/me", h.Withdraw)
	})

	return r
}
