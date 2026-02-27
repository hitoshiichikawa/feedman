package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// SubscriptionServiceInterface は購読ハンドラーが必要とするサービスインターフェース。
type SubscriptionServiceInterface interface {
	// ListSubscriptions はユーザーの購読一覧を返す。
	ListSubscriptions(ctx context.Context, userID string) ([]subscriptionResponse, error)
	// UpdateSettings は購読のフェッチ間隔を更新する。
	UpdateSettings(ctx context.Context, userID, subscriptionID string, minutes int) (*subscriptionResponse, error)
	// Unsubscribe は購読を解除する（subscription + 関連item_statesを削除）。
	Unsubscribe(ctx context.Context, userID, subscriptionID string) error
	// ResumeFetch は停止中フィードのフェッチを再開する。
	ResumeFetch(ctx context.Context, userID, subscriptionID string) (*subscriptionResponse, error)
}

// SubscriptionHandler は購読管理のHTTPハンドラー。
type SubscriptionHandler struct {
	service SubscriptionServiceInterface
}

// NewSubscriptionHandler はSubscriptionHandlerを生成する。
func NewSubscriptionHandler(service SubscriptionServiceInterface) *SubscriptionHandler {
	return &SubscriptionHandler{
		service: service,
	}
}

// subscriptionResponse は購読情報のAPIレスポンス。
type subscriptionResponse struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	FeedID               string    `json:"feed_id"`
	FeedTitle            string    `json:"feed_title"`
	FeedURL              string    `json:"feed_url"`
	FaviconURL           *string   `json:"favicon_url,omitempty"`
	FetchIntervalMinutes int       `json:"fetch_interval_minutes"`
	FeedStatus           string    `json:"feed_status"`
	ErrorMessage         *string   `json:"error_message,omitempty"`
	UnreadCount          int       `json:"unread_count"`
	CreatedAt            time.Time `json:"created_at"`
}

// subscriptionSettingsRequest はフェッチ間隔設定更新リクエストのボディ。
type subscriptionSettingsRequest struct {
	FetchIntervalMinutes int `json:"fetch_interval_minutes"`
}

// ListSubscriptions はユーザーの購読一覧を取得する。
// GET /api/subscriptions
func (h *SubscriptionHandler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
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

	subs, err := h.service.ListSubscriptions(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subs)
}

// UpdateSettings は購読のフェッチ間隔設定を更新する。
// PUT /api/subscriptions/:id/settings
func (h *SubscriptionHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
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

	subscriptionID := chi.URLParam(r, "id")

	var req subscriptionSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIErrorResponse(w, http.StatusBadRequest, &model.APIError{
			Code:     "INVALID_REQUEST",
			Message:  "リクエストボディの解析に失敗しました。",
			Category: "validation",
			Action:   "正しいJSON形式でリクエストしてください。",
		})
		return
	}

	// フェッチ間隔のバリデーション: 30分-720分（12時間）、30分刻み
	if !isValidFetchInterval(req.FetchIntervalMinutes) {
		writeAPIErrorResponse(w, http.StatusBadRequest, model.NewInvalidFetchIntervalError(req.FetchIntervalMinutes))
		return
	}

	sub, err := h.service.UpdateSettings(r.Context(), userID, subscriptionID, req.FetchIntervalMinutes)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

// Unsubscribe は購読を解除する。
// DELETE /api/subscriptions/:id
func (h *SubscriptionHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
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

	subscriptionID := chi.URLParam(r, "id")

	if err := h.service.Unsubscribe(r.Context(), userID, subscriptionID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ResumeFetch は停止中フィードのフェッチを再開する。
// POST /api/subscriptions/:id/resume
func (h *SubscriptionHandler) ResumeFetch(w http.ResponseWriter, r *http.Request) {
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

	subscriptionID := chi.URLParam(r, "id")

	sub, err := h.service.ResumeFetch(r.Context(), userID, subscriptionID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

// SetupSubscriptionRoutes は購読管理関連のルーティングを設定したchi.Routerを返す。
func SetupSubscriptionRoutes(service SubscriptionServiceInterface) http.Handler {
	r := chi.NewRouter()
	h := NewSubscriptionHandler(service)

	r.Route("/api/subscriptions", func(r chi.Router) {
		r.Get("/", h.ListSubscriptions)

		r.Route("/{id}", func(r chi.Router) {
			r.Delete("/", h.Unsubscribe)
			r.Put("/settings", h.UpdateSettings)
			r.Post("/resume", h.ResumeFetch)
		})
	})

	return r
}

// isValidFetchInterval はフェッチ間隔のバリデーションを行う。
// 30分-720分（12時間）、30分刻みであることを検証する。
func isValidFetchInterval(minutes int) bool {
	return minutes >= 30 && minutes <= 720 && minutes%30 == 0
}
