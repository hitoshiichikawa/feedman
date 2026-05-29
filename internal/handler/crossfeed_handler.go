// Package handler の crossfeed_handler.go は、フィード横断新着一覧（Cross-Feed Timeline）の
// HTTP エンドポイントを提供する。
//
// 提供エンドポイント:
//   - GET  /api/items/cross-feed              : 横断新着記事一覧（cursor / limit / since）
//   - PUT  /api/users/me/cross-feed-last-seen : 最終閲覧時刻の更新（204 No Content）
//
// 認証必須グループ配下に登録される。設計詳細は docs/specs/121-issue/design.md
// 「CrossFeedHandler」節を参照。Issue #121 / Req 1.2, 2.1, 4.3, 4.7, NFR 1.3, NFR 2.1。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// maxCrossFeedLimit は GET /api/items/cross-feed の limit クエリパラメータの上限値
// （NFR 1.3 / design.md「API Contract」節）。これを超える指定はクランプする。
const maxCrossFeedLimit = 200

// CrossFeedServiceInterface は横断新着ハンドラが必要とするサービスインターフェース。
//
// 戻り値は handler 内部レスポンス型（*crossFeedListResult）にすることで、サービス層と
// アダプタ層の責務を明確に分離する。実装は CrossFeedServiceAdapter（service_adapter.go）が
// 担当し、domain 型（crossfeed.NewItemsResult）を crossFeedListResult に変換する。
type CrossFeedServiceInterface interface {
	// ListNewItems は overrideSince（非 nil の場合）または stored last_seen_at（nil なら
	// 24h fallback）を基準とした新着記事を published_at 降順で取得する。
	// cursorStr は前回レスポンスの NextCursor を渡す（空文字なら先頭ページ）。
	ListNewItems(
		ctx context.Context,
		userID string,
		cursorStr string,
		limit int,
		overrideSince *time.Time,
	) (*crossFeedListResult, error)

	// TouchLastSeen は当該ユーザーの user_cross_feed_views.last_seen_at を now() で UPSERT する。
	TouchLastSeen(ctx context.Context, userID string) error
}

// CrossFeedHandler は横断新着一覧の HTTP ハンドラ。
type CrossFeedHandler struct {
	service CrossFeedServiceInterface
}

// NewCrossFeedHandler は CrossFeedHandler を生成する。
func NewCrossFeedHandler(service CrossFeedServiceInterface) *CrossFeedHandler {
	return &CrossFeedHandler{service: service}
}

// --- レスポンス型 ---

// crossFeedItemResponse は横断新着一覧の記事 1 件のレスポンス。
//
// 既存 itemSummaryResponse の全フィールドに加え、発信元フィードのメタ情報
// （feed_title / feed_favicon_url）を併記する（Req 3.1, 3.2）。
// feed_favicon_url は data URL 形式（`data:<mime>;base64,...`）。サービス層から渡された
// data URL 文字列をそのまま転送する。未設定時は nil を入れ JSON では明示的に `null` を返す。
type crossFeedItemResponse struct {
	ID              string    `json:"id"`
	FeedID          string    `json:"feed_id"`
	FeedTitle       string    `json:"feed_title"`
	FeedFaviconURL  *string   `json:"feed_favicon_url"`
	Title           string    `json:"title"`
	Link            string    `json:"link"`
	Summary         string    `json:"summary"`
	PublishedAt     time.Time `json:"published_at"`
	IsDateEstimated bool      `json:"is_date_estimated"`
	IsRead          bool      `json:"is_read"`
	IsStarred       bool      `json:"is_starred"`
	HatebuCount     int       `json:"hatebu_count"`
}

// crossFeedListResult は GET /api/items/cross-feed のレスポンス。
//
// next_cursor は次ページ取得用のカーソル文字列（`<RFC3339Nano>:<uuid>` 形式）。
// 末尾ページ・空結果のときは空文字となる（omitempty で省略）。
// since_time は当該レスポンスで採用した新着判定基準時刻であり、クライアントが
// session-level baseline として保持する（Req 4.7）。
type crossFeedListResult struct {
	Items      []crossFeedItemResponse `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	HasMore    bool                    `json:"has_more"`
	SinceTime  time.Time               `json:"since_time"`
}

// ListItems は GET /api/items/cross-feed のハンドラ。
//
// クエリパラメータ:
//   - cursor : ページネーション用カーソル（任意、`<RFC3339Nano>:<uuid>` 形式）。
//     形式不正は service 層が model.NewInvalidFilterError を返し 400 にマップ
//   - limit  : 1 ページあたり件数（任意、既定 50、上限 200 でクランプ）。形式不正は 400
//   - since  : 新着判定基準時刻の override（任意、RFC3339 形式）。指定時はサーバ側
//     user_cross_feed_views.last_seen_at を参照せず、当該値を基準に新着抽出する
//     （Req 4.7 / session-level baseline）。形式不正は 400 INVALID_REQUEST
//
// エラーレスポンス:
//   - 401 UNAUTHORIZED   : セッションなし（middleware が早期返却）
//   - 400 INVALID_REQUEST: limit または since の形式不正
//   - 400 INVALID_FILTER : cursor 形式不正（service 層から）
//   - 500 INTERNAL_ERROR : DB エラー等
func (h *CrossFeedHandler) ListItems(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	cursor := q.Get("cursor")
	limitStr := q.Get("limit")
	sinceStr := q.Get("since")

	// limit のパース（未指定は既定値 / 形式不正・非正値は 400 / 上限を超える指定はクランプ）
	limit := defaultItemsPerPage
	if limitStr != "" {
		n, parseErr := strconv.Atoi(limitStr)
		if parseErr != nil || n <= 0 {
			middleware.WriteErrorResponse(w, http.StatusBadRequest, &model.APIError{
				Code:     "INVALID_REQUEST",
				Message:  "limit の形式が不正です。",
				Category: "validation",
				Action:   "1 以上の整数を指定してください。",
			})
			return
		}
		if n > maxCrossFeedLimit {
			n = maxCrossFeedLimit
		}
		limit = n
	}

	// since のパース（Req 4.7）。指定時のみ overrideSince に渡し、形式不正は 400 を返す。
	// time.Parse(time.RFC3339, ...) は RFC3339Nano（小数点秒）も受け付ける後方互換挙動。
	var overrideSince *time.Time
	if sinceStr != "" {
		t, parseErr := time.Parse(time.RFC3339, sinceStr)
		if parseErr != nil {
			middleware.WriteErrorResponse(w, http.StatusBadRequest, &model.APIError{
				Code:     "INVALID_REQUEST",
				Message:  "since の形式が不正です。",
				Category: "validation",
				Action:   "RFC3339 形式の日時を指定してください（例: 2026-05-27T12:34:56Z）。",
			})
			return
		}
		overrideSince = &t
	}

	result, err := h.service.ListNewItems(r.Context(), userID, cursor, limit, overrideSince)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Items が nil の場合でも JSON で `"items": []` を返す（NFR 3.1 / 既存 starred と同方針）。
	if result.Items == nil {
		result.Items = []crossFeedItemResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// TouchLastSeen は PUT /api/users/me/cross-feed-last-seen のハンドラ。
// リクエストボディは不要。成功時は 204 No Content を返す（Req 4.3）。
func (h *CrossFeedHandler) TouchLastSeen(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.TouchLastSeen(r.Context(), userID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
