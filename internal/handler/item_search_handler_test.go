package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// recordedSearchCall は handler から service に渡された引数を記録する。
type recordedSearchCall struct {
	userID   string
	rawQuery string
	feedID   *string
	cursor   string
	limit    int
}

// mockItemSearchService は ItemSearchServiceInterface のモック実装。
type mockItemSearchService struct {
	searchFn  func(ctx context.Context, userID, rawQuery string, feedID *string, cursorStr string, limit int) (*itemSearchResponse, error)
	calls     []recordedSearchCall
	callCount int
}

func (m *mockItemSearchService) Search(
	ctx context.Context,
	userID, rawQuery string,
	feedID *string,
	cursorStr string,
	limit int,
) (*itemSearchResponse, error) {
	m.callCount++
	// feedID のスナップショットを取って後続テストアサート用に保持
	var feedIDCopy *string
	if feedID != nil {
		v := *feedID
		feedIDCopy = &v
	}
	m.calls = append(m.calls, recordedSearchCall{
		userID:   userID,
		rawQuery: rawQuery,
		feedID:   feedIDCopy,
		cursor:   cursorStr,
		limit:    limit,
	})
	if m.searchFn != nil {
		return m.searchFn(ctx, userID, rawQuery, feedID, cursorStr, limit)
	}
	return &itemSearchResponse{Items: []itemSearchHitResponse{}}, nil
}

// 有効な UUID フォーマットの定数（テスト用）。
const validFeedUUID = "11111111-2222-3333-4444-555555555555"

// --- 200 OK: 横断検索成功 ---

// TestItemSearchHandler_Search_Global_Success は q=foo の横断検索で service が呼ばれ
// レスポンスが正常に返ることを検証する（Req 1.3, 4.2）。
func TestItemSearchHandler_Search_Global_Success(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	favicon := "data:image/png;base64,Y2FmZQ=="
	svc := &mockItemSearchService{
		searchFn: func(_ context.Context, userID, rawQuery string, feedID *string, cursor string, limit int) (*itemSearchResponse, error) {
			if userID != "user-1" {
				t.Errorf("userID = %q, want %q", userID, "user-1")
			}
			if rawQuery != "foo" {
				t.Errorf("rawQuery = %q, want %q", rawQuery, "foo")
			}
			if feedID != nil {
				t.Errorf("feedID = %v, want nil (global search)", *feedID)
			}
			if limit != defaultItemsPerPage {
				t.Errorf("limit = %d, want %d", limit, defaultItemsPerPage)
			}
			return &itemSearchResponse{
				Items: []itemSearchHitResponse{
					{
						ID:          "item-1",
						FeedID:      "feed-a",
						FeedTitle:   "Feed A",
						FaviconURL:  &favicon,
						Title:       "Foo Bar",
						Link:        "https://example.com/1",
						Summary:     "概要",
						PublishedAt: now,
					},
				},
				NextCursor: now.Format(time.RFC3339Nano) + "|item-1",
				HasMore:    true,
			}, nil
		},
	}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo", nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	items, ok := body["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items invalid: %v", body["items"])
	}
	first := items[0].(map[string]interface{})
	if first["feed_title"] != "Feed A" {
		t.Errorf("feed_title = %v, want Feed A", first["feed_title"])
	}
	if first["favicon_url"] != favicon {
		t.Errorf("favicon_url = %v, want %q", first["favicon_url"], favicon)
	}
	if body["has_more"] != true {
		t.Errorf("has_more = %v, want true", body["has_more"])
	}
}

// --- 200 OK: フィード内検索成功 ---

// TestItemSearchHandler_Search_Feed_Success は q=foo&feed_id=<uuid> のフィード内検索で
// feedID *string が service に伝搬することを検証する（Req 1.4, 2.2）。
func TestItemSearchHandler_Search_Feed_Success(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{
		searchFn: func(_ context.Context, _, _ string, feedID *string, _ string, _ int) (*itemSearchResponse, error) {
			if feedID == nil {
				t.Fatal("expected feedID non-nil")
			}
			if *feedID != validFeedUUID {
				t.Errorf("feedID = %q, want %q", *feedID, validFeedUUID)
			}
			return &itemSearchResponse{Items: []itemSearchHitResponse{}}, nil
		},
	}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo&feed_id="+validFeedUUID, nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if svc.callCount != 1 {
		t.Errorf("service called %d times, want 1", svc.callCount)
	}
}

// --- 401: 未認証 ---

// TestItemSearchHandler_Search_NoUserID_ReturnsUnauthorized は withUserID なしで
// 401 UNAUTHORIZED が返ることを検証する（Req 3.3）。
func TestItemSearchHandler_Search_NoUserID_ReturnsUnauthorized(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo", nil)
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
	}
	if svc.callCount != 0 {
		t.Errorf("service must not be called when unauthenticated, got %d calls", svc.callCount)
	}
}

// --- 400: cursor 不正（service から INVALID_SEARCH_QUERY） ---

// TestItemSearchHandler_Search_InvalidCursor_ReturnsBadRequest は service が
// NewInvalidSearchQueryError を返したときに 400 にマッピングされることを検証する。
func TestItemSearchHandler_Search_InvalidCursor_ReturnsBadRequest(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{
		searchFn: func(_ context.Context, _, _ string, _ *string, _ string, _ int) (*itemSearchResponse, error) {
			return nil, model.NewInvalidSearchQueryError("cursor の形式が不正です")
		},
	}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo&cursor=bogus", nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusBadRequest)
	}
	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeInvalidSearchQuery {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeInvalidSearchQuery)
	}
}

// --- 400: feed_id UUID パース失敗（handler 層で 400） ---

// TestItemSearchHandler_Search_InvalidFeedID_ReturnsBadRequest は feed_id が UUID
// 形式でないとき handler 層で 400 INVALID_SEARCH_QUERY を返すことを検証する。
// service は呼ばれないこと（fail-fast）も検証する。
func TestItemSearchHandler_Search_InvalidFeedID_ReturnsBadRequest(t *testing.T) {
	// google/uuid の Parse は dashes 無しの 32 hex 形式（urn:uuid:... を除いた中身）も
	// 妥当として受け付けるため、純粋に非 UUID と判定されるケースのみを並べる。
	cases := []struct {
		name   string
		feedID string
	}{
		{"random string", "not-a-uuid"},
		{"too short", "1234"},
		{"invalid hex chars", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockItemSearchService{}
			h := NewItemSearchHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo&feed_id="+tc.feedID, nil)
			req = withUserID(req, "user-1")
			w := httptest.NewRecorder()

			// Act
			h.Search(w, req)

			// Assert
			if w.Result().StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusBadRequest)
			}
			if svc.callCount != 0 {
				t.Errorf("service must not be called when feed_id is invalid, got %d calls", svc.callCount)
			}
			errResp := parseAPIErrorResponse(t, w)
			if errResp["code"] != model.ErrCodeInvalidSearchQuery {
				t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeInvalidSearchQuery)
			}
		})
	}
}

// --- 403: 未購読 feed_id ---

// TestItemSearchHandler_Search_FeedNotSubscribed_ReturnsForbidden は service が
// NewFeedNotSubscribedError を返したときに 403 にマッピングされることを検証する（Req 3.5）。
func TestItemSearchHandler_Search_FeedNotSubscribed_ReturnsForbidden(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{
		searchFn: func(_ context.Context, _, _ string, _ *string, _ string, _ int) (*itemSearchResponse, error) {
			return nil, model.NewFeedNotSubscribedError(validFeedUUID)
		},
	}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo&feed_id="+validFeedUUID, nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusForbidden)
	}
	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeFeedNotSubscribed {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeFeedNotSubscribed)
	}
}

// --- 200: 空クエリ → 空配列（Req 1.5） ---

// TestItemSearchHandler_Search_EmptyQuery_ReturnsEmptyArray は q が空文字でも
// 200 OK で items: [] が返ることを検証する。service には rawQuery="" が伝わるが
// サービス層側で空判定して空結果を返す前提（Req 1.5）。
func TestItemSearchHandler_Search_EmptyQuery_ReturnsEmptyArray(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{
		searchFn: func(_ context.Context, _, rawQuery string, _ *string, _ string, _ int) (*itemSearchResponse, error) {
			if rawQuery != "" {
				t.Errorf("rawQuery = %q, want empty", rawQuery)
			}
			// service 層が空クエリで Items nil を返す挙動を模倣
			return &itemSearchResponse{Items: nil, HasMore: false}, nil
		},
	}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=", nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	// items が JSON で空配列として返ること（nil ではなく []）
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, `"items":[]`) {
		t.Errorf("expected items to be empty JSON array, got %q", bodyStr)
	}
}

// --- 500: service の generic error ---

// TestItemSearchHandler_Search_ServiceError_ReturnsInternalServerError は service が
// APIError 以外のエラーを返したとき 500 にマッピングされることを検証する。
func TestItemSearchHandler_Search_ServiceError_ReturnsInternalServerError(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{
		searchFn: func(_ context.Context, _, _ string, _ *string, _ string, _ int) (*itemSearchResponse, error) {
			return nil, errors.New("db connection lost")
		},
	}
	h := NewItemSearchHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo", nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusInternalServerError)
	}
}

// --- limit のクランプ確認 ---

// TestItemSearchHandler_Search_LimitClamp は limit クエリパラメータが上限値を超えた
// ときに maxSearchLimit にクランプされることを検証する。
func TestItemSearchHandler_Search_LimitClamp(t *testing.T) {
	cases := []struct {
		name      string
		limitStr  string
		wantLimit int
	}{
		{"unspecified -> default", "", defaultItemsPerPage},
		{"valid -> as-is", "25", 25},
		{"at max -> as-is", "200", maxSearchLimit},
		{"over max -> clamped", "500", maxSearchLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockItemSearchService{}
			h := NewItemSearchHandler(svc)

			u := "/api/items/search?q=foo"
			if tc.limitStr != "" {
				u += "&limit=" + tc.limitStr
			}
			req := httptest.NewRequest(http.MethodGet, u, nil)
			req = withUserID(req, "user-1")
			w := httptest.NewRecorder()

			// Act
			h.Search(w, req)

			// Assert
			if w.Result().StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
			}
			if svc.callCount != 1 {
				t.Fatalf("service callCount = %d, want 1", svc.callCount)
			}
			if svc.calls[0].limit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", svc.calls[0].limit, tc.wantLimit)
			}
		})
	}
}

// TestItemSearchHandler_Search_InvalidLimit_ReturnsBadRequest は limit クエリが
// 非数値 / 0 以下の場合に handler 層で 400 を返すことを検証する。
func TestItemSearchHandler_Search_InvalidLimit_ReturnsBadRequest(t *testing.T) {
	cases := []struct {
		name     string
		limitStr string
	}{
		{"non-numeric", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockItemSearchService{}
			h := NewItemSearchHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/items/search?q=foo&limit="+tc.limitStr, nil)
			req = withUserID(req, "user-1")
			w := httptest.NewRecorder()

			// Act
			h.Search(w, req)

			// Assert
			if w.Result().StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusBadRequest)
			}
			if svc.callCount != 0 {
				t.Errorf("service must not be called when limit is invalid, got %d calls", svc.callCount)
			}
		})
	}
}

// --- cursor がそのまま service に伝搬する ---

// TestItemSearchHandler_Search_CursorPropagates は cursor クエリパラメータが
// service にそのまま伝搬することを検証する（パースは service 層の責務）。
func TestItemSearchHandler_Search_CursorPropagates(t *testing.T) {
	// Arrange
	svc := &mockItemSearchService{}
	h := NewItemSearchHandler(svc)

	cursorValue := "2026-05-28T12:34:56.123456789Z|" + validFeedUUID
	req := httptest.NewRequest(http.MethodGet,
		"/api/items/search?q=foo&cursor="+cursorValue, nil)
	req = withUserID(req, "user-1")
	w := httptest.NewRecorder()

	// Act
	h.Search(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if svc.calls[0].cursor != cursorValue {
		t.Errorf("cursor = %q, want %q", svc.calls[0].cursor, cursorValue)
	}
}
