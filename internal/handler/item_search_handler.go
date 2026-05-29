package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// 検索リクエストの limit パラメータの既定値と上限値。
//
// design.md「API Contract」節に従い、limit クエリ未指定時は defaultItemsPerPage を使い、
// maxSearchLimit を超える指定はクランプする。サービス層（`itemsearch.SearchService`）も
// 同じクランプを防御的に行うため、handler 層の値はそのまま下層に伝搬しても安全である。
const maxSearchLimit = 200

// ItemSearchServiceInterface は記事検索ハンドラが必要とするサービスインターフェース。
//
// 戻り値は handler 内部レスポンス型（`*itemSearchResponse`）にすることで、サービス層と
// アダプタ層の責務を明確に分離する。実装は `ItemSearchServiceAdapter`
// （`service_adapter.go`）が担当し、Adapter 層がドメイン型（`itemsearch.SearchResult`）を
// `itemSearchResponse` に変換する（favicon の data URL 化を含む）。
type ItemSearchServiceInterface interface {
	// Search は当該ユーザーが購読中のフィードに属する記事から、キーワードに部分一致する
	// ものを published_at 降順で返す。feedID 非 nil 時はフィード内検索モードに切り替わる。
	// cursorStr は前回レスポンスの NextCursor を渡す（空文字なら先頭ページ）。
	// limit は実取得件数の上限（HasMore 判定はサービス層が limit+1 取得で行う）。
	Search(ctx context.Context, userID, rawQuery string, feedID *string, cursorStr string, limit int) (*itemSearchResponse, error)
}

// ItemSearchHandler は記事検索の HTTP ハンドラ。
//
// クエリパラメータ（`q` / `feed_id` / `cursor` / `limit`）のパース、認証チェック、
// `feed_id` の UUID 形式バリデーション、検索サービス呼び出し、レスポンス整形を担う。
// ビジネスロジック（クエリ正規化・購読確認・DB クエリ）はサービス層に委譲する。
type ItemSearchHandler struct {
	service ItemSearchServiceInterface
}

// NewItemSearchHandler は ItemSearchHandler を生成する。
func NewItemSearchHandler(service ItemSearchServiceInterface) *ItemSearchHandler {
	return &ItemSearchHandler{service: service}
}

// --- レスポンス型 ---

// itemSearchHitResponse は検索結果 1 件の API レスポンス。
//
// favicon_url は data URL 形式（`data:<mime>;base64,...`）。Service 層から渡された
// 生バイト + MIME を Adapter 層で組み立てた結果が入る。欠落時は nil を入れ、JSON では
// `omitempty` でフィールドごと省略する（既存 subscription レスポンスと同じ流儀）。
type itemSearchHitResponse struct {
	ID              string    `json:"id"`
	FeedID          string    `json:"feed_id"`
	FeedTitle       string    `json:"feed_title"`
	FaviconURL      *string   `json:"favicon_url,omitempty"`
	Title           string    `json:"title"`
	Link            string    `json:"link"`
	Summary         string    `json:"summary"`
	PublishedAt     time.Time `json:"published_at"`
	IsDateEstimated bool      `json:"is_date_estimated"`
	IsRead          bool      `json:"is_read"`
	IsStarred       bool      `json:"is_starred"`
	HatebuCount     int       `json:"hatebu_count"`
}

// itemSearchResponse は GET /api/items/search のレスポンス。
//
// next_cursor は次ページ取得用のカーソル文字列（`<RFC3339Nano>|<id>` 形式）。
// 末尾ページ・空結果のときは空文字となる（`omitempty` で省略）。has_more は次ページの
// 存在を示し、cursor を発行できない場合（末尾項目の PublishedAt がゼロ値等）でも
// true を返しうるため、UI 側は next_cursor の空判定だけでなく has_more も参照する。
type itemSearchResponse struct {
	Items      []itemSearchHitResponse `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	HasMore    bool                    `json:"has_more"`
}

// Search は GET /api/items/search のハンドラ。
//
// クエリパラメータ:
//   - q      : 検索キーワード（必須に近いが、空クエリは 200 OK で空配列を返す。Req 1.5）
//   - feed_id: フィード内検索のスコープ指定（任意、UUID 形式）。
//     形式不正は 400 INVALID_SEARCH_QUERY、未購読は 403 FEED_NOT_SUBSCRIBED。
//   - cursor : ページネーションのカーソル（任意、`<RFC3339Nano>|<uuid>` 形式）。
//     形式不正は 400 INVALID_SEARCH_QUERY（サービス層で判定）。
//   - limit  : 1 ページあたり件数（任意、既定 50、上限 200 でクランプ）。
//
// エラーレスポンス:
//   - 401 UNAUTHORIZED        : セッションなし
//   - 400 INVALID_SEARCH_QUERY: cursor 形式不正 / feed_id UUID パース失敗 / limit 形式不正
//   - 403 FEED_NOT_SUBSCRIBED : feed_id 指定だが当該ユーザーが未購読
//   - 500 INTERNAL_ERROR      : DB エラー等
//
// NFR 3.1 に従い、リクエスト入口で slog.Info の構造化ログを発行する。
// PII / ログ汚染を避けるためクエリ本文は出力せず、長さ（query_len）のみ記録する。
func (h *ItemSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
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
	rawQuery := q.Get("q")
	feedIDStr := q.Get("feed_id")
	cursor := q.Get("cursor")
	limitStr := q.Get("limit")

	// feed_id の UUID パース（空文字 = 横断検索）
	var feedIDPtr *string
	feedIDForLog := ""
	searchType := "global"
	if feedIDStr != "" {
		if _, parseErr := uuid.Parse(feedIDStr); parseErr != nil {
			middleware.WriteErrorResponse(
				w, http.StatusBadRequest,
				model.NewInvalidSearchQueryError("feed_id の形式が不正です"),
			)
			return
		}
		feedIDPtr = &feedIDStr
		feedIDForLog = feedIDStr
		searchType = "feed"
	}

	// limit のパース（未指定は既定値 / 形式不正は 400 / 上限を超える指定はクランプ）
	limit := defaultItemsPerPage
	if limitStr != "" {
		n, parseErr := strconv.Atoi(limitStr)
		if parseErr != nil || n <= 0 {
			middleware.WriteErrorResponse(
				w, http.StatusBadRequest,
				model.NewInvalidSearchQueryError("limit の形式が不正です"),
			)
			return
		}
		if n > maxSearchLimit {
			n = maxSearchLimit
		}
		limit = n
	}

	// NFR 3.1: 検索リクエストの認証主体・検索種別・検索範囲・クエリ長・スコープ feed_id を
	// 構造化ログに記録する（クエリ本文は PII / ログ汚染回避のため記録しない）。
	slog.Info("item search request",
		slog.String("user_id", userID),
		slog.String("search_type", searchType),
		slog.String("scope", "subscribed"),
		slog.Int("query_len", len(rawQuery)),
		slog.String("feed_id", feedIDForLog),
	)

	result, err := h.service.Search(r.Context(), userID, rawQuery, feedIDPtr, cursor, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Items が nil でも JSON では空配列を返したい（Req 4.3 の UX 一貫性 / Req 1.5）。
	if result.Items == nil {
		result.Items = []itemSearchHitResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
