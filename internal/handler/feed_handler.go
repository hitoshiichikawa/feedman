package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// FeedServiceInterface はフィードハンドラーが必要とするサービスインターフェース。
type FeedServiceInterface interface {
	// RegisterFeed はURLからフィードを検出し登録する。
	RegisterFeed(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error)
	// GetFeed はフィード情報を取得する。
	GetFeed(ctx context.Context, feedID string) (*model.Feed, error)
	// UpdateFeedURL はフィードURLを更新する。
	UpdateFeedURL(ctx context.Context, feedID, newURL string) (*model.Feed, error)
}

// SubscriptionDeleter は購読削除のためのインターフェース。
// 購読解除（フィード削除操作）で使用する。
// repository.SubscriptionRepositoryを直接変更せず、最小限のインターフェースとして定義する。
type SubscriptionDeleter interface {
	// DeleteByUserAndFeed はユーザーIDとフィードIDで購読を削除する。
	DeleteByUserAndFeed(ctx context.Context, userID, feedID string) error
}

// FeedHandler はフィード管理のHTTPハンドラー。
type FeedHandler struct {
	service FeedServiceInterface
	deleter SubscriptionDeleter
}

// NewFeedHandler はFeedHandlerを生成する。
func NewFeedHandler(service FeedServiceInterface, deleter SubscriptionDeleter) *FeedHandler {
	return &FeedHandler{
		service: service,
		deleter: deleter,
	}
}

// registerFeedRequest はフィード登録リクエストのボディ。
type registerFeedRequest struct {
	URL string `json:"url"`
}

// updateFeedURLRequest はフィードURL更新リクエストのボディ。
type updateFeedURLRequest struct {
	FeedURL string `json:"feed_url"`
}

// feedResponse はフィード情報のAPIレスポンス。
type feedResponse struct {
	ID          string `json:"id"`
	FeedURL     string `json:"feed_url"`
	SiteURL     string `json:"site_url"`
	Title       string `json:"title"`
	FetchStatus string `json:"fetch_status"`
}

// apiErrorResponse は統一エラーフォーマットのレスポンス。
type apiErrorResponse struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Category string `json:"category"`
	Action   string `json:"action"`
}

// RegisterFeed はフィード登録を処理する。
// POST /api/feeds
func (h *FeedHandler) RegisterFeed(w http.ResponseWriter, r *http.Request) {
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

	var req registerFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIErrorResponse(w, http.StatusBadRequest, &model.APIError{
			Code:     "INVALID_REQUEST",
			Message:  "リクエストボディの解析に失敗しました。",
			Category: "validation",
			Action:   "正しいJSON形式でリクエストしてください。",
		})
		return
	}

	if req.URL == "" {
		writeAPIErrorResponse(w, http.StatusBadRequest, model.NewInvalidURLError("URLが空です"))
		return
	}

	feed, _, err := h.service.RegisterFeed(r.Context(), userID, req.URL)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toFeedResponse(feed))
}

// GetFeed はフィード詳細を取得する。
// GET /api/feeds/:id
func (h *FeedHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	feedID := chi.URLParam(r, "id")

	feed, err := h.service.GetFeed(r.Context(), feedID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if feed == nil {
		writeAPIErrorResponse(w, http.StatusNotFound, &model.APIError{
			Code:     "FEED_NOT_FOUND",
			Message:  "指定されたフィードが見つかりません。",
			Category: "feed",
			Action:   "フィードIDを確認してください。",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toFeedResponse(feed))
}

// UpdateFeedURL はフィードURLを更新する。
// PATCH /api/feeds/:id
func (h *FeedHandler) UpdateFeedURL(w http.ResponseWriter, r *http.Request) {
	feedID := chi.URLParam(r, "id")

	var req updateFeedURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIErrorResponse(w, http.StatusBadRequest, &model.APIError{
			Code:     "INVALID_REQUEST",
			Message:  "リクエストボディの解析に失敗しました。",
			Category: "validation",
			Action:   "正しいJSON形式でリクエストしてください。",
		})
		return
	}

	if req.FeedURL == "" {
		writeAPIErrorResponse(w, http.StatusBadRequest, model.NewInvalidURLError("フィードURLが空です"))
		return
	}

	feed, err := h.service.UpdateFeedURL(r.Context(), feedID, req.FeedURL)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toFeedResponse(feed))
}

// DeleteFeed はフィードの購読を解除する。
// DELETE /api/feeds/:id
func (h *FeedHandler) DeleteFeed(w http.ResponseWriter, r *http.Request) {
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

	feedID := chi.URLParam(r, "id")

	if err := h.deleter.DeleteByUserAndFeed(r.Context(), userID, feedID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetupFeedRoutes はフィード管理関連のルーティングを設定したchi.Routerを返す。
// feedRegMiddleware が nil でない場合、POST /api/feeds にフィード登録専用レート制限を適用する。
func SetupFeedRoutes(service FeedServiceInterface, deleter SubscriptionDeleter, feedRegMiddleware func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	h := NewFeedHandler(service, deleter)

	r.Route("/api/feeds", func(r chi.Router) {
		// POST /api/feeds - フィード登録（登録専用レート制限を適用）
		if feedRegMiddleware != nil {
			r.With(feedRegMiddleware).Post("/", h.RegisterFeed)
		} else {
			r.Post("/", h.RegisterFeed)
		}

		// /api/feeds/:id 以下のルーティング
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetFeed)
			r.Patch("/", h.UpdateFeedURL)
			r.Delete("/", h.DeleteFeed)
		})
	})

	return r
}

// --- ヘルパー関数 ---

// toFeedResponse はmodel.FeedからAPIレスポンスに変換する。
func toFeedResponse(feed *model.Feed) feedResponse {
	return feedResponse{
		ID:          feed.ID,
		FeedURL:     feed.FeedURL,
		SiteURL:     feed.SiteURL,
		Title:       feed.Title,
		FetchStatus: string(feed.FetchStatus),
	}
}

// writeAPIErrorResponse は統一エラーフォーマットでエラーレスポンスを書き込む。
func writeAPIErrorResponse(w http.ResponseWriter, statusCode int, apiErr *model.APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(apiErrorResponse{
		Code:     apiErr.Code,
		Message:  apiErr.Message,
		Category: apiErr.Category,
		Action:   apiErr.Action,
	})
}

// handleServiceError はサービス層から返されたエラーを適切なHTTPステータスコードに変換する。
func handleServiceError(w http.ResponseWriter, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		statusCode := mapAPIErrorToHTTPStatus(apiErr)
		writeAPIErrorResponse(w, statusCode, apiErr)
		return
	}

	// APIError以外のエラーは内部サーバーエラーとして扱う
	slog.Error("internal server error", slog.String("error", err.Error()))
	writeAPIErrorResponse(w, http.StatusInternalServerError, &model.APIError{
		Code:     "INTERNAL_ERROR",
		Message:  "内部エラーが発生しました。",
		Category: "system",
		Action:   "しばらく待ってから再度お試しください。",
	})
}

// mapAPIErrorToHTTPStatus はAPIErrorコードからHTTPステータスコードにマッピングする。
func mapAPIErrorToHTTPStatus(apiErr *model.APIError) int {
	switch apiErr.Code {
	case model.ErrCodeFeedNotDetected:
		return http.StatusUnprocessableEntity
	case model.ErrCodeInvalidURL:
		return http.StatusBadRequest
	case model.ErrCodeSSRFBlocked:
		return http.StatusForbidden
	case model.ErrCodeFetchFailed:
		return http.StatusBadGateway
	case model.ErrCodeParseFailed:
		return http.StatusUnprocessableEntity
	case model.ErrCodeSubscriptionLimit:
		return http.StatusConflict
	case "DUPLICATE_SUBSCRIPTION":
		return http.StatusConflict
	case "FEED_NOT_FOUND", model.ErrCodeSubscriptionNotFound, model.ErrCodeItemNotFound:
		return http.StatusNotFound
	case model.ErrCodeInvalidFilter, model.ErrCodeInvalidFetchInterval:
		return http.StatusBadRequest
	case model.ErrCodeFeedNotStopped:
		return http.StatusConflict
	case model.ErrCodeUserNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
