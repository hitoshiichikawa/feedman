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

// defaultItemsPerPage は記事一覧の1回の取得件数（デフォルト）。
const defaultItemsPerPage = 50

// ItemServiceInterface は記事ハンドラーが必要とするサービスインターフェース。
type ItemServiceInterface interface {
	// ListItems はフィードの記事一覧をフィルタ・ページネーション付きで返す。
	ListItems(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error)
	// GetItem は記事詳細を返す。
	GetItem(ctx context.Context, userID, itemID string) (*itemDetailResponse, error)
}

// ItemStateServiceInterface は記事状態管理サービスのインターフェース。
type ItemStateServiceInterface interface {
	// UpdateState は記事の既読・スター状態を冪等に更新する。
	// nilフィールドは変更しない部分更新を行う。
	UpdateState(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error)
}

// ItemHandler は記事管理のHTTPハンドラー。
type ItemHandler struct {
	service      ItemServiceInterface
	stateService ItemStateServiceInterface
}

// NewItemHandler はItemHandlerを生成する。
func NewItemHandler(service ItemServiceInterface, stateService ItemStateServiceInterface) *ItemHandler {
	return &ItemHandler{
		service:      service,
		stateService: stateService,
	}
}

// --- レスポンス型 ---

// itemSummaryResponse は記事一覧のサマリーレスポンス。
type itemSummaryResponse struct {
	ID              string    `json:"id"`
	FeedID          string    `json:"feed_id"`
	Title           string    `json:"title"`
	Link            string    `json:"link"`
	PublishedAt     time.Time `json:"published_at"`
	IsDateEstimated bool      `json:"is_date_estimated"`
	IsRead          bool      `json:"is_read"`
	IsStarred       bool      `json:"is_starred"`
	HatebuCount     int       `json:"hatebu_count"`
}

// itemListResult は記事一覧のレスポンス。
type itemListResult struct {
	Items      []itemSummaryResponse `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}

// itemDetailResponse は記事詳細のレスポンス。
type itemDetailResponse struct {
	itemSummaryResponse
	Content string `json:"content"` // サニタイズ済みHTML
	Summary string `json:"summary"`
	Author  string `json:"author"`
}

// itemStateRequest は記事状態更新リクエストのボディ。
type itemStateRequest struct {
	IsRead    *bool `json:"is_read,omitempty"`
	IsStarred *bool `json:"is_starred,omitempty"`
}

// itemStateResponse は記事状態のレスポンス。
type itemStateResponse struct {
	ItemID    string `json:"item_id"`
	IsRead    bool   `json:"is_read"`
	IsStarred bool   `json:"is_starred"`
}

// ListItems はフィードの記事一覧を取得する。
// GET /api/feeds/:id/items?cursor=xxx&filter=all|unread|starred
func (h *ItemHandler) ListItems(w http.ResponseWriter, r *http.Request) {
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
	cursor := r.URL.Query().Get("cursor")
	filterStr := r.URL.Query().Get("filter")

	// デフォルトフィルタは "all"
	filter := model.ItemFilterAll
	if filterStr != "" {
		filter = model.ItemFilter(filterStr)
	}

	result, err := h.service.ListItems(r.Context(), userID, feedID, filter, cursor, defaultItemsPerPage)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetItem は記事詳細を取得する。
// GET /api/items/:id
func (h *ItemHandler) GetItem(w http.ResponseWriter, r *http.Request) {
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

	itemID := chi.URLParam(r, "id")

	detail, err := h.service.GetItem(r.Context(), userID, itemID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if detail == nil {
		writeAPIErrorResponse(w, http.StatusNotFound, model.NewItemNotFoundError(itemID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// UpdateItemState は記事の既読・スター状態を更新する。
// PUT /api/items/:id/state
func (h *ItemHandler) UpdateItemState(w http.ResponseWriter, r *http.Request) {
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

	itemID := chi.URLParam(r, "id")

	var req itemStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIErrorResponse(w, http.StatusBadRequest, &model.APIError{
			Code:     "INVALID_REQUEST",
			Message:  "リクエストボディの解析に失敗しました。",
			Category: "validation",
			Action:   "正しいJSON形式でリクエストしてください。",
		})
		return
	}

	// is_readとis_starredの両方がnilの場合はバリデーションエラー
	if req.IsRead == nil && req.IsStarred == nil {
		writeAPIErrorResponse(w, http.StatusBadRequest, &model.APIError{
			Code:     "INVALID_REQUEST",
			Message:  "is_readまたはis_starredのいずれかを指定してください。",
			Category: "validation",
			Action:   "更新するフィールドを指定してください。",
		})
		return
	}

	state, err := h.stateService.UpdateState(r.Context(), userID, itemID, req.IsRead, req.IsStarred)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(itemStateResponse{
		ItemID:    state.ItemID,
		IsRead:    state.IsRead,
		IsStarred: state.IsStarred,
	})
}

// SetupItemRoutes は記事管理関連のルーティングを設定したchi.Routerを返す。
func SetupItemRoutes(service ItemServiceInterface, stateService ItemStateServiceInterface) http.Handler {
	r := chi.NewRouter()
	h := NewItemHandler(service, stateService)

	// GET /api/feeds/:id/items - 記事一覧（フィードごと）
	r.Route("/api/feeds/{id}/items", func(r chi.Router) {
		r.Get("/", h.ListItems)
	})

	// /api/items/:id 以下のルーティング
	r.Route("/api/items/{id}", func(r chi.Router) {
		r.Get("/", h.GetItem)
		r.Put("/state", h.UpdateItemState)
	})

	return r
}
