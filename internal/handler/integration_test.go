package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// --- 統合テスト用のステートフルモック ---

// integrationState は統合テスト用の共有状態を保持する。
type integrationState struct {
	sessions      map[string]*model.Session
	feeds         map[string]*model.Feed
	subscriptions map[string]*model.Subscription
	items         map[string]*model.Item
	itemStates    map[string]*model.ItemState
	users         map[string]*model.User
	subsByUser    map[string][]string // userID -> []subscriptionID
}

func newIntegrationState() *integrationState {
	return &integrationState{
		sessions:      make(map[string]*model.Session),
		feeds:         make(map[string]*model.Feed),
		subscriptions: make(map[string]*model.Subscription),
		items:         make(map[string]*model.Item),
		itemStates:    make(map[string]*model.ItemState),
		users:         make(map[string]*model.User),
		subsByUser:    make(map[string][]string),
	}
}

// --- 統合テスト用ルーター構築ヘルパー ---

func createIntegrationRouter(state *integrationState) http.Handler {
	sessionFinder := &mockSessionFinderForRouter{
		sessions: state.sessions,
	}

	deps := &RouterDeps{
		SessionFinder:     sessionFinder,
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService: &mockAuthService{
			getLoginURLFn: func(s string) string {
				return "https://accounts.google.com/o/oauth2/auth?state=" + s
			},
			handleCallbackFn: func(ctx context.Context, code string) (*model.Session, error) {
				session := &model.Session{
					ID:        "session-integration-1",
					UserID:    "user-integration-1",
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				state.sessions[session.ID] = session
				state.users["user-integration-1"] = &model.User{
					ID:    "user-integration-1",
					Email: "integration@example.com",
					Name:  "Integration User",
				}
				return session, nil
			},
			logoutFn: func(ctx context.Context, sessionID string) error {
				delete(state.sessions, sessionID)
				return nil
			},
			getCurrentUserFn: func(ctx context.Context, sessionID string) (*model.User, error) {
				sess, ok := state.sessions[sessionID]
				if !ok {
					return nil, fmt.Errorf("session not found")
				}
				user, ok := state.users[sess.UserID]
				if !ok {
					return nil, fmt.Errorf("user not found")
				}
				return user, nil
			},
		},
		AuthConfig: AuthHandlerConfig{BaseURL: "http://localhost:3000", SessionMaxAge: 86400},
		FeedService: &mockFeedService{
			registerFeedFn: func(ctx context.Context, userID, inputURL string) (*model.Feed, *model.Subscription, error) {
				f := &model.Feed{
					ID:          "feed-integration-1",
					FeedURL:     inputURL,
					SiteURL:     "https://example.com",
					Title:       "Integration Feed",
					FetchStatus: model.FetchStatusActive,
				}
				sub := &model.Subscription{
					ID:     "sub-integration-1",
					UserID: userID,
					FeedID: f.ID,
				}
				state.feeds[f.ID] = f
				state.subscriptions[sub.ID] = sub
				state.subsByUser[userID] = append(state.subsByUser[userID], sub.ID)
				return f, sub, nil
			},
			getFeedFn: func(ctx context.Context, feedID string) (*model.Feed, error) {
				f, ok := state.feeds[feedID]
				if !ok {
					return nil, nil
				}
				return f, nil
			},
			updateFeedURLFn: func(ctx context.Context, feedID, newURL string) (*model.Feed, error) {
				f, ok := state.feeds[feedID]
				if !ok {
					return nil, &model.APIError{
						Code:     "FEED_NOT_FOUND",
						Message:  "フィードが見つかりません。",
						Category: "feed",
						Action:   "フィードIDを確認してください。",
					}
				}
				f.FeedURL = newURL
				return f, nil
			},
		},
		SubscriptionDeleter: &mockSubscriptionDeleter{
			deleteByUserAndFeedFn: func(ctx context.Context, userID, feedID string) error {
				for id, sub := range state.subscriptions {
					if sub.UserID == userID && sub.FeedID == feedID {
						delete(state.subscriptions, id)
						break
					}
				}
				return nil
			},
		},
		ItemService: &mockItemService{
			listItemsFn: func(ctx context.Context, userID, feedID string, filter model.ItemFilter, cursor string, limit int) (*itemListResult, error) {
				return &itemListResult{
					Items: []itemSummaryResponse{
						{
							ID:     "item-integration-1",
							FeedID: feedID,
							Title:  "Integration Item",
							Link:   "https://example.com/article/1",
						},
					},
					HasMore: false,
				}, nil
			},
			getItemFn: func(ctx context.Context, userID, itemID string) (*itemDetailResponse, error) {
				return &itemDetailResponse{
					itemSummaryResponse: itemSummaryResponse{
						ID:    itemID,
						Title: "Integration Item Detail",
						Link:  "https://example.com/article/1",
					},
					Content: "<p>Integration test content</p>",
				}, nil
			},
		},
		ItemStateService: &mockItemStateService{
			updateStateFn: func(ctx context.Context, userID, itemID string, isRead *bool, isStarred *bool) (*model.ItemState, error) {
				key := userID + ":" + itemID
				is := &model.ItemState{UserID: userID, ItemID: itemID}
				if isRead != nil {
					is.IsRead = *isRead
				}
				if isStarred != nil {
					is.IsStarred = *isStarred
				}
				state.itemStates[key] = is
				return is, nil
			},
		},
		SubscriptionService: &mockSubscriptionService{
			listSubscriptionsFn: func(ctx context.Context, userID string) ([]subscriptionResponse, error) {
				var results []subscriptionResponse
				for _, subID := range state.subsByUser[userID] {
					sub := state.subscriptions[subID]
					if sub == nil {
						continue
					}
					f := state.feeds[sub.FeedID]
					title := ""
					if f != nil {
						title = f.Title
					}
					results = append(results, subscriptionResponse{
						ID:        sub.ID,
						UserID:    sub.UserID,
						FeedID:    sub.FeedID,
						FeedTitle: title,
					})
				}
				return results, nil
			},
		},
		UserService: &mockUserService{
			withdrawFn: func(ctx context.Context, userID string) error {
				// ユーザー関連データを全削除
				delete(state.users, userID)
				for id, sub := range state.subscriptions {
					if sub.UserID == userID {
						delete(state.subscriptions, id)
					}
				}
				delete(state.subsByUser, userID)
				for id, sess := range state.sessions {
					if sess.UserID == userID {
						delete(state.sessions, id)
					}
				}
				for key, is := range state.itemStates {
					if is.UserID == userID {
						delete(state.itemStates, key)
					}
				}
				return nil
			},
		},
	}

	return NewRouter(deps)
}

// --- エンドツーエンド統合テスト ---

// TestIntegration_AuthFlow_LoginCallbackMeLogout はOAuth認証フロー全体を検証する。
// ログイン → コールバック → セッション発行 → /auth/me で認証確認 → ログアウト → セッション破棄
func TestIntegration_AuthFlow_LoginCallbackMeLogout(t *testing.T) {
	state := newIntegrationState()
	router := createIntegrationRouter(state)

	// 1. ログイン: OAuthリダイレクトURLが返ること
	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("step1: GET /auth/google/login status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "accounts.google.com") {
		t.Fatalf("step1: redirect location = %q, should contain accounts.google.com", location)
	}

	// OAuthステートクッキーを取得
	var oauthStateCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "oauth_state" {
			oauthStateCookie = c
			break
		}
	}
	if oauthStateCookie == nil {
		t.Fatal("step1: expected oauth_state cookie")
	}

	// 2. コールバック: セッションが発行されること
	callbackURL := "/auth/google/callback?code=test-auth-code&state=" + oauthStateCookie.Value
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(oauthStateCookie)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("step2: callback status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	// セッションクッキーを取得
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("step2: expected session_id cookie")
	}
	if sessionCookie.Value == "" {
		t.Fatal("step2: expected non-empty session_id")
	}

	// 3. /auth/me: セッション付きでユーザー情報が取得できること
	req = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step3: GET /auth/me status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var meBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&meBody)
	if meBody["email"] != "integration@example.com" {
		t.Errorf("step3: email = %q, want %q", meBody["email"], "integration@example.com")
	}

	// 4. ログアウト: セッションが破棄されること
	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("step4: POST /auth/logout status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	// 5. ログアウト後に /auth/me にアクセスすると401が返ること
	req = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(sessionCookie) // 古いセッションを使用
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("step5: GET /auth/me after logout status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestIntegration_FeedRegistrationFlow はフィード登録フロー全体を検証する。
// CSRFトークン取得 → セッション付きでフィード登録 → 登録されたフィードを取得
func TestIntegration_FeedRegistrationFlow(t *testing.T) {
	state := newIntegrationState()
	// セッションを事前に設定
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{
		ID:    "user-test",
		Email: "test@example.com",
		Name:  "Test User",
	}

	router := createIntegrationRouter(state)

	// 1. フィード登録（POST /api/feeds）
	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("step1: POST /api/feeds status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var feedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&feedResp)
	if feedResp["id"] == nil || feedResp["id"] == "" {
		t.Fatal("step1: expected non-empty feed id")
	}
	feedID := feedResp["id"].(string)

	// 3. 登録されたフィードの詳細を取得（GET /api/feeds/{id}）
	req = httptest.NewRequest(http.MethodGet, "/api/feeds/"+feedID, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step3: GET /api/feeds/%s status = %d, want %d", feedID, resp.StatusCode, http.StatusOK)
	}

	var getFeedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&getFeedResp)
	if getFeedResp["title"] != "Integration Feed" {
		t.Errorf("step3: feed title = %q, want %q", getFeedResp["title"], "Integration Feed")
	}

	// 4. 購読一覧にフィードが含まれること（GET /api/subscriptions）
	req = httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step4: GET /api/subscriptions status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var subsResp []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&subsResp)
	if len(subsResp) != 1 {
		t.Fatalf("step4: expected 1 subscription, got %d", len(subsResp))
	}
	if subsResp[0]["feed_title"] != "Integration Feed" {
		t.Errorf("step4: subscription feed_title = %q, want %q", subsResp[0]["feed_title"], "Integration Feed")
	}

	// 5. 記事一覧を取得（GET /api/feeds/{id}/items）
	req = httptest.NewRequest(http.MethodGet, "/api/feeds/"+feedID+"/items", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step5: GET /api/feeds/%s/items status = %d, want %d", feedID, resp.StatusCode, http.StatusOK)
	}

	var itemsResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&itemsResp)
	items := itemsResp["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("step5: expected 1 item, got %d", len(items))
	}
}

// TestIntegration_ItemStateManagement は記事状態管理フローを検証する。
// 記事詳細取得 → 既読にする → スターを付ける
func TestIntegration_ItemStateManagement(t *testing.T) {
	state := newIntegrationState()
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	router := createIntegrationRouter(state)

	// 1. 記事詳細を取得（GET /api/items/{id}）
	req := httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step1: GET /api/items/item-1 status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var itemDetail map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&itemDetail)
	if itemDetail["title"] != "Integration Item Detail" {
		t.Errorf("step1: title = %q, want %q", itemDetail["title"], "Integration Item Detail")
	}
	if itemDetail["content"] != "<p>Integration test content</p>" {
		t.Errorf("step1: content = %q, want sanitized HTML", itemDetail["content"])
	}

	// 2. 既読にする（PUT /api/items/{id}/state）
	body := `{"is_read": true}`
	req = httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step2: PUT /api/items/item-1/state status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var stateResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stateResp)
	if stateResp["is_read"] != true {
		t.Errorf("step2: is_read = %v, want true", stateResp["is_read"])
	}

	// 3. スターを付ける（PUT /api/items/{id}/state）
	body = `{"is_starred": true}`
	req = httptest.NewRequest(http.MethodPut, "/api/items/item-1/state", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step3: PUT /api/items/item-1/state status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestIntegration_UnsubscribeAndWithdrawFlow は購読解除・退会フローを検証する。
// フィード登録 → フィード削除（購読解除） → 退会 → 全データ削除確認
func TestIntegration_UnsubscribeAndWithdrawFlow(t *testing.T) {
	state := newIntegrationState()
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{
		ID:    "user-test",
		Email: "test@example.com",
		Name:  "Test User",
	}

	router := createIntegrationRouter(state)

	// 1. フィード登録
	body := `{"url": "https://example.com/feed.xml"}`
	req := httptest.NewRequest(http.MethodPost, "/api/feeds", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("step1: POST /api/feeds status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var feedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&feedResp)
	feedID := feedResp["id"].(string)

	// 購読が作成されたことを確認
	if len(state.subscriptions) != 1 {
		t.Fatalf("step1: expected 1 subscription, got %d", len(state.subscriptions))
	}

	// 2. フィード削除（購読解除）（DELETE /api/feeds/{id}）
	req = httptest.NewRequest(http.MethodDelete, "/api/feeds/"+feedID, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("step2: DELETE /api/feeds/%s status = %d, want %d", feedID, resp.StatusCode, http.StatusNoContent)
	}

	// 購読が削除されたことを確認
	if len(state.subscriptions) != 0 {
		t.Errorf("step2: expected 0 subscriptions after delete, got %d", len(state.subscriptions))
	}

	// 3. 退会（DELETE /api/users/me）
	req = httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("step3: DELETE /api/users/me status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// 全データが削除されたことを確認
	if len(state.users) != 0 {
		t.Errorf("step3: expected 0 users after withdraw, got %d", len(state.users))
	}
	if len(state.sessions) != 0 {
		t.Errorf("step3: expected 0 sessions after withdraw, got %d", len(state.sessions))
	}
	if len(state.subscriptions) != 0 {
		t.Errorf("step3: expected 0 subscriptions after withdraw, got %d", len(state.subscriptions))
	}
}

// TestIntegration_ProtectedEndpoints_RequireAuth は全保護エンドポイントが認証を要求することを検証する。
func TestIntegration_ProtectedEndpoints_RequireAuth(t *testing.T) {
	state := newIntegrationState()
	router := createIntegrationRouter(state)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/subscriptions", ""},
		{http.MethodGet, "/api/feeds/feed-1", ""},
		{http.MethodPost, "/api/feeds", `{"url": "https://example.com"}`},
		{http.MethodGet, "/api/feeds/feed-1/items", ""},
		{http.MethodGet, "/api/items/item-1", ""},
		{http.MethodPut, "/api/items/item-1/state", `{"is_read": true}`},
		{http.MethodDelete, "/api/feeds/feed-1", ""},
		{http.MethodDelete, "/api/users/me", ""},
		{http.MethodDelete, "/api/subscriptions/sub-1", ""},
		{http.MethodPut, "/api/subscriptions/sub-1/settings", `{"fetch_interval_minutes": 60}`},
		{http.MethodPost, "/api/subscriptions/sub-1/resume", ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Result().StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s (no auth) status = %d, want %d",
					ep.method, ep.path, w.Result().StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

