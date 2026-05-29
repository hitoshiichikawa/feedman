package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// --- モック定義 ---

// mockCrossFeedService は CrossFeedServiceInterface のモック実装。
// 各テストで listNewItemsFn / touchLastSeenFn を差し替えることで、認証 / クエリ
// パラメータ / 異常系の振る舞いを個別に検証する。
type mockCrossFeedService struct {
	listNewItemsFn  func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error)
	touchLastSeenFn func(ctx context.Context, userID string) error
}

func (m *mockCrossFeedService) ListNewItems(
	ctx context.Context,
	userID string,
	cursorStr string,
	limit int,
	overrideSince *time.Time,
) (*crossFeedListResult, error) {
	if m.listNewItemsFn != nil {
		return m.listNewItemsFn(ctx, userID, cursorStr, limit, overrideSince)
	}
	return &crossFeedListResult{}, nil
}

func (m *mockCrossFeedService) TouchLastSeen(ctx context.Context, userID string) error {
	if m.touchLastSeenFn != nil {
		return m.touchLastSeenFn(ctx, userID)
	}
	return nil
}

// --- GET /api/items/cross-feed テスト ---

// TestCrossFeedHandler_ListItems_NoUserID_ReturnsUnauthorized は未認証リクエストに対して
// 401 を返し、応答ボディに記事データを含めないことを検証する（Req 1.2 / 認証必須グループ配下）。
func TestCrossFeedHandler_ListItems_NoUserID_ReturnsUnauthorized(t *testing.T) {
	// Arrange
	h := NewCrossFeedHandler(&mockCrossFeedService{})

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed", nil)
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if _, ok := result["items"]; ok {
		t.Error("expected no items field in 401 response")
	}
}

// TestCrossFeedHandler_ListItems_Success は認証ありで service が返した items / next_cursor /
// has_more / since_time が正しく JSON で返ることを検証する（Req 2.1 / NFR 1.3）。
func TestCrossFeedHandler_ListItems_Success(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-24 * time.Hour)
	favicon := "data:image/png;base64,iVBORw0KGgo="
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			if limit != defaultItemsPerPage {
				t.Errorf("limit = %d, want %d (default)", limit, defaultItemsPerPage)
			}
			if overrideSince != nil {
				t.Errorf("overrideSince = %v, want nil (no since query)", overrideSince)
			}
			return &crossFeedListResult{
				Items: []crossFeedItemResponse{
					{
						ID:             "item-1",
						FeedID:         "feed-A",
						FeedTitle:      "Feed Alpha",
						FeedFaviconURL: &favicon,
						Title:          "記事タイトル1",
						Link:           "https://example.com/1",
						Summary:        "概要1",
						PublishedAt:    now,
						IsRead:         false,
						IsStarred:      false,
						HatebuCount:    5,
					},
					{
						ID:             "item-2",
						FeedID:         "feed-B",
						FeedTitle:      "Feed Beta",
						FeedFaviconURL: nil, // favicon 未設定
						Title:          "記事タイトル2",
						Link:           "https://example.com/2",
						PublishedAt:    now.Add(-time.Hour),
					},
				},
				NextCursor: now.Add(-time.Hour).Format(time.RFC3339Nano) + ":item-2",
				HasMore:    true,
				SinceTime:  since,
			}, nil
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array in response")
	}
	if len(items) != 2 {
		t.Fatalf("items length = %d, want 2", len(items))
	}

	hasMore, ok := result["has_more"].(bool)
	if !ok || !hasMore {
		t.Errorf("has_more = %v, want true", result["has_more"])
	}

	if _, ok := result["next_cursor"]; !ok {
		t.Error("expected next_cursor in response")
	}
	if _, ok := result["since_time"]; !ok {
		t.Error("expected since_time in response")
	}

	// 1 件目: feed_title / feed_favicon_url / feed_id が含まれる（Req 3.1, 3.2）
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected items[0] to be an object")
	}
	if first["feed_id"] != "feed-A" {
		t.Errorf("items[0].feed_id = %v, want %q", first["feed_id"], "feed-A")
	}
	if first["feed_title"] != "Feed Alpha" {
		t.Errorf("items[0].feed_title = %v, want %q", first["feed_title"], "Feed Alpha")
	}
	if first["feed_favicon_url"] != favicon {
		t.Errorf("items[0].feed_favicon_url = %v, want %q", first["feed_favicon_url"], favicon)
	}

	// 2 件目: feed_favicon_url は明示的に null（omitempty ではない / Req 3.3 のクライアント側
	// fallback 判定のため field 自体は存在し null が入る）
	second, ok := items[1].(map[string]interface{})
	if !ok {
		t.Fatal("expected items[1] to be an object")
	}
	favVal, exists := second["feed_favicon_url"]
	if !exists {
		t.Error("expected items[1].feed_favicon_url field to exist (must not be omitted)")
	}
	if favVal != nil {
		t.Errorf("items[1].feed_favicon_url = %v, want nil", favVal)
	}
}

// TestCrossFeedHandler_ListItems_WithSince_PassesOverrideSinceToService は
// since=<valid RFC3339> 指定時に Service の overrideSince に当該値が渡ることを検証する
// （Req 4.7: クライアント主導 session-level baseline）。
func TestCrossFeedHandler_ListItems_WithSince_PassesOverrideSinceToService(t *testing.T) {
	// Arrange
	wantSince := time.Date(2026, 5, 27, 12, 34, 56, 0, time.UTC)
	var receivedOverride *time.Time
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			receivedOverride = overrideSince
			return &crossFeedListResult{Items: []crossFeedItemResponse{}, SinceTime: wantSince}, nil
		},
	}

	h := NewCrossFeedHandler(svc)

	url := "/api/items/cross-feed?since=" + wantSince.Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if receivedOverride == nil {
		t.Fatal("expected overrideSince to be non-nil when since query is provided")
	}
	if !receivedOverride.Equal(wantSince) {
		t.Errorf("overrideSince = %v, want %v", receivedOverride, wantSince)
	}
}

// TestCrossFeedHandler_ListItems_InvalidSince_ReturnsBadRequest は since=<invalid> 指定時に
// 400 INVALID_REQUEST を返し、Service が呼ばれないことを検証する（Req 4.7）。
func TestCrossFeedHandler_ListItems_InvalidSince_ReturnsBadRequest(t *testing.T) {
	// Arrange
	serviceCalled := false
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			serviceCalled = true
			return &crossFeedListResult{}, nil
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed?since=not-a-date", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if serviceCalled {
		t.Error("expected service NOT to be called for invalid since")
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != "INVALID_REQUEST" {
		t.Errorf("code = %q, want %q", errResp["code"], "INVALID_REQUEST")
	}
}

// TestCrossFeedHandler_ListItems_WithLimit はクエリパラメータ limit が Service に伝搬し、
// 上限値（200）を超える指定がクランプされることを検証する（NFR 1.3）。
func TestCrossFeedHandler_ListItems_WithLimit(t *testing.T) {
	cases := []struct {
		name      string
		limitStr  string
		wantLimit int
	}{
		{name: "未指定時は既定値 50", limitStr: "", wantLimit: defaultItemsPerPage},
		{name: "100 を指定すると 100", limitStr: "100", wantLimit: 100},
		{name: "200 を超える指定は 200 にクランプ", limitStr: "500", wantLimit: maxCrossFeedLimit},
		{name: "200 ちょうどはそのまま", limitStr: "200", wantLimit: 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var receivedLimit int
			svc := &mockCrossFeedService{
				listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
					receivedLimit = limit
					return &crossFeedListResult{Items: []crossFeedItemResponse{}}, nil
				},
			}
			h := NewCrossFeedHandler(svc)

			url := "/api/items/cross-feed"
			if tc.limitStr != "" {
				url += "?limit=" + tc.limitStr
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = withUserID(req, "user-123")
			w := httptest.NewRecorder()

			// Act
			h.ListItems(w, req)

			// Assert
			if w.Result().StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
			}
			if receivedLimit != tc.wantLimit {
				t.Errorf("limit propagated = %d, want %d", receivedLimit, tc.wantLimit)
			}
		})
	}
}

// TestCrossFeedHandler_ListItems_InvalidLimit_ReturnsBadRequest は limit が非数値 / 0 以下の
// とき 400 INVALID_REQUEST を返し、Service が呼ばれないことを検証する（境界値）。
func TestCrossFeedHandler_ListItems_InvalidLimit_ReturnsBadRequest(t *testing.T) {
	cases := []string{"abc", "0", "-1"}
	for _, lim := range cases {
		t.Run("limit="+lim, func(t *testing.T) {
			// Arrange
			serviceCalled := false
			svc := &mockCrossFeedService{
				listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
					serviceCalled = true
					return &crossFeedListResult{}, nil
				},
			}
			h := NewCrossFeedHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed?limit="+lim, nil)
			req = withUserID(req, "user-123")
			w := httptest.NewRecorder()

			// Act
			h.ListItems(w, req)

			// Assert
			if w.Result().StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusBadRequest)
			}
			if serviceCalled {
				t.Error("expected service NOT to be called for invalid limit")
			}
		})
	}
}

// TestCrossFeedHandler_ListItems_WithCursor は cursor クエリパラメータが Service に
// 伝搬することを検証する（NFR 1.3 / ページング）。
func TestCrossFeedHandler_ListItems_WithCursor(t *testing.T) {
	// Arrange
	wantCursor := "2026-05-27T12:34:56.789Z:550e8400-e29b-41d4-a716-446655440000"
	var receivedCursor string
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			receivedCursor = cursorStr
			return &crossFeedListResult{Items: []crossFeedItemResponse{}}, nil
		},
	}
	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed?cursor="+wantCursor, nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if receivedCursor != wantCursor {
		t.Errorf("cursor = %q, want %q", receivedCursor, wantCursor)
	}
}

// TestCrossFeedHandler_ListItems_InvalidCursor_ReturnsBadRequest は service 層が
// model.NewInvalidFilterError を返したときに 400 INVALID_FILTER にマップされることを検証する。
func TestCrossFeedHandler_ListItems_InvalidCursor_ReturnsBadRequest(t *testing.T) {
	// Arrange
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			return nil, model.NewInvalidFilterError("invalid cursor: " + cursorStr)
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed?cursor=broken", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	errResp := parseAPIErrorResponse(t, w)
	if errResp["code"] != model.ErrCodeInvalidFilter {
		t.Errorf("code = %q, want %q", errResp["code"], model.ErrCodeInvalidFilter)
	}
}

// TestCrossFeedHandler_ListItems_EmptyResult_ReturnsItemsArray は 0 件返却時に
// JSON 上 items=[] で返ること（null ではないこと）を検証する（NFR 3.1 / Req 4.6）。
func TestCrossFeedHandler_ListItems_EmptyResult_ReturnsItemsArray(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			return &crossFeedListResult{
				Items:     nil,
				HasMore:   false,
				SinceTime: now,
			}, nil
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}

	bodyBytes := w.Body.Bytes()
	// items は null ではなく [] で返る（NFR 3.1）
	if !bytes.Contains(bodyBytes, []byte(`"items":[]`)) {
		t.Errorf("expected items=[] in JSON, got %s", string(bodyBytes))
	}
}

// TestCrossFeedHandler_ListItems_ServiceError_ReturnsInternalServerError は service 層が
// 汎用エラーを返したときに 500 にマップされることを検証する（異常系）。
func TestCrossFeedHandler_ListItems_ServiceError_ReturnsInternalServerError(t *testing.T) {
	// Arrange
	svc := &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			return nil, errors.New("database error")
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.ListItems(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusInternalServerError)
	}
}

// --- PUT /api/users/me/cross-feed-last-seen テスト ---

// TestCrossFeedHandler_TouchLastSeen_Success は認証ありで service の TouchLastSeen が
// 呼ばれ、204 No Content が返ることを検証する（Req 4.3）。
func TestCrossFeedHandler_TouchLastSeen_Success(t *testing.T) {
	// Arrange
	called := false
	svc := &mockCrossFeedService{
		touchLastSeenFn: func(ctx context.Context, userID string) error {
			called = true
			if userID != "user-123" {
				t.Errorf("userID = %q, want %q", userID, "user-123")
			}
			return nil
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/users/me/cross-feed-last-seen", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.TouchLastSeen(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if !called {
		t.Error("expected TouchLastSeen to be called")
	}
	// 204 はボディが空であるべき
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %s", w.Body.String())
	}
}

// TestCrossFeedHandler_TouchLastSeen_NoUserID_ReturnsUnauthorized は未認証リクエストで
// 401 を返し、service が呼ばれないことを検証する。
func TestCrossFeedHandler_TouchLastSeen_NoUserID_ReturnsUnauthorized(t *testing.T) {
	// Arrange
	called := false
	svc := &mockCrossFeedService{
		touchLastSeenFn: func(ctx context.Context, userID string) error {
			called = true
			return nil
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/users/me/cross-feed-last-seen", nil)
	// ユーザーIDを注入しない
	w := httptest.NewRecorder()

	// Act
	h.TouchLastSeen(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
	}
	if called {
		t.Error("expected TouchLastSeen NOT to be called when unauthenticated")
	}
}

// TestCrossFeedHandler_TouchLastSeen_ServiceError_ReturnsInternalServerError は
// service 層が汎用エラーを返したときに 500 にマップされることを検証する。
func TestCrossFeedHandler_TouchLastSeen_ServiceError_ReturnsInternalServerError(t *testing.T) {
	// Arrange
	svc := &mockCrossFeedService{
		touchLastSeenFn: func(ctx context.Context, userID string) error {
			return errors.New("db error")
		},
	}

	h := NewCrossFeedHandler(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/users/me/cross-feed-last-seen", nil)
	req = withUserID(req, "user-123")
	w := httptest.NewRecorder()

	// Act
	h.TouchLastSeen(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusInternalServerError)
	}
}

// --- ルーティングテスト ---

// TestRouter_CrossFeedRoutes_RegisteredAndRouted は NewRouter が CrossFeedService 配線時に
// 以下を正しく設定することを検証する（Req 1.2 / 4.3）:
//   - GET /api/items/cross-feed が ListItems ハンドラに到達する（chi のトライ木で
//     /api/items/{id} に吸われないこと、既存 starred 同様の保護を確認）
//   - PUT /api/users/me/cross-feed-last-seen が TouchLastSeen ハンドラに到達する
func TestRouter_CrossFeedRoutes_RegisteredAndRouted(t *testing.T) {
	state := newIntegrationState()
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{ID: "user-test", Email: "t@e.com", Name: "T"}

	listCalled := false
	touchCalled := false

	// integration router の deps に CrossFeedService を後付けで設定し、
	// 既存 createIntegrationRouter とは別 router を直接組み立てる。
	deps := buildCrossFeedRouterDeps(state, &mockCrossFeedService{
		listNewItemsFn: func(ctx context.Context, userID, cursorStr string, limit int, overrideSince *time.Time) (*crossFeedListResult, error) {
			listCalled = true
			if userID != "user-test" {
				t.Errorf("ListItems userID = %q, want %q", userID, "user-test")
			}
			return &crossFeedListResult{
				Items: []crossFeedItemResponse{},
			}, nil
		},
		touchLastSeenFn: func(ctx context.Context, userID string) error {
			touchCalled = true
			if userID != "user-test" {
				t.Errorf("TouchLastSeen userID = %q, want %q", userID, "user-test")
			}
			return nil
		},
	})

	router := NewRouter(deps)

	// GET /api/items/cross-feed
	req := httptest.NewRequest(http.MethodGet, "/api/items/cross-feed", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /api/items/cross-feed status = %d, want %d", w.Result().StatusCode, http.StatusOK)
	}
	if !listCalled {
		t.Error("expected ListItems to be called via router")
	}

	// PUT /api/users/me/cross-feed-last-seen
	req = httptest.NewRequest(http.MethodPut, "/api/users/me/cross-feed-last-seen", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusNoContent {
		t.Errorf("PUT /api/users/me/cross-feed-last-seen status = %d, want %d",
			w.Result().StatusCode, http.StatusNoContent)
	}
	if !touchCalled {
		t.Error("expected TouchLastSeen to be called via router")
	}
}

// TestRouter_CrossFeedRoutes_Unauthorized_Returns401 は認証クッキー無しで
// GET /api/items/cross-feed / PUT /api/users/me/cross-feed-last-seen が
// session middleware 段階で 401 を返すことを検証する（認証必須グループ配下）。
func TestRouter_CrossFeedRoutes_Unauthorized_Returns401(t *testing.T) {
	state := newIntegrationState()
	deps := buildCrossFeedRouterDeps(state, &mockCrossFeedService{})
	router := NewRouter(deps)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/items/cross-feed"},
		{http.MethodPut, "/api/users/me/cross-feed-last-seen"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Result().StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

// --- テストヘルパー ---

// buildCrossFeedRouterDeps は createIntegrationRouter ベースの RouterDeps を構築し、
// CrossFeedService に与えられた mock を注入する。
//
// 既存 createIntegrationRouter はすべての deps を内部生成するため、CrossFeedService 単独で
// 差し替えできない。本ヘルパは router_test.go 内の mockSessionFinderForRouter / 各種 mock を
// 流用しつつ、CrossFeedService のみテスト指定の mock に差し替える。
func buildCrossFeedRouterDeps(state *integrationState, svc CrossFeedServiceInterface) *RouterDeps {
	// createIntegrationRouter は内部で deps を組み立てて即時 NewRouter まで呼ぶため、
	// 部分的に deps を変更するには直接組み立てる必要がある。本ヘルパは「認証 middleware が
	// 効くこと」と「CrossFeedService が router に伝わること」のみを保証する最小限の deps を
	// 返す。他の handler 群はテストで参照しないため zero 値で安全（router 構築時の参照は
	// 認証必須グループ内のみで、対応するルートに到達しない限り nil でも問題ない）。
	deps := minimalRouterDepsForCrossFeed(state)
	deps.CrossFeedService = svc
	return deps
}

// minimalRouterDepsForCrossFeed は createIntegrationRouter のロジックを CrossFeed 検証に
// 必要な最小範囲だけ複製したヘルパ。createIntegrationRouter は他の service 群もすべて埋める
// 重量級ヘルパだが、本テストは CrossFeed の ListItems / TouchLastSeen 配線のみ検証するため
// 既存 integration_test.go の mock 群を活用しつつ最小構成で済ませる。
func minimalRouterDepsForCrossFeed(state *integrationState) *RouterDeps {
	// createIntegrationRouter を再利用するため、その出力から RouterDeps を取り出す代わりに
	// 同等の deps を最小構成で組み立てる。
	// 既存テストインフラとの整合のため、router_test / integration_test の mock 群を借用する。
	return &RouterDeps{
		SessionFinder:       &mockSessionFinderForRouter{sessions: state.sessions},
		CORSAllowedOrigin:   "http://localhost:3000",
		RateLimiter:         middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthConfig:          AuthHandlerConfig{BaseURL: "http://localhost:3000", SessionMaxAge: 86400},
		AuthService:         &mockAuthService{},
		FeedService:         &mockFeedService{},
		SubscriptionDeleter: &mockSubscriptionDeleter{},
		ItemService:         &mockItemService{},
		ItemStateService:    &mockItemStateService{},
		ItemSearchService:   &mockItemSearchService{},
		SubscriptionService: &mockSubscriptionService{},
		UserService:         &mockUserService{},
	}
}

