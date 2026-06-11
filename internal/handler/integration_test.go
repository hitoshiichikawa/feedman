package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/auth"
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

	// 横断スター記事一覧（/api/feeds/starred/items）取得時に
	// service 層に渡された cursor / userID を記録するためのトレーシング用フィールド。
	// router 経由でハンドラに正しくディスパッチされたことを検証する用途に使う。
	lastStarredUserID string
	lastStarredCursor string
	lastStarredLimit  int
	starredCallCount  int
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

// --- Native Auth 用 stateful mock service（Issue #166） ---

// mockNativeTokenExchangeService は integration_test 内で auth_code → token 交換の
// 単回利用と「同一 code の再交換 400」、および refresh token rotation の通しを
// 検証するためのステートフルモック。実際の repository/service 層を介さず、
// issuedCode との一致 + 単回フラグで判定し、refresh は払い出した平文と family ID を
// 内部 map で追跡する（Issue #167）。
type mockNativeTokenExchangeService struct {
	mu           sync.Mutex
	issuedCode   string
	used         bool
	callCount    int
	lastCode     string
	lastVerifier string

	// refresh tokens の rotation 用ステート（Issue #167）。
	// activeRefresh[plain]=familyID で「現役の refresh token 平文 → family」を保持。
	// rotation 成功で旧 plain を活性集合から外し（rotated 扱い）新 plain を追加する。
	// 旧 plain は rotatedRefresh に move し、再提示時の「rotation 済み拒否」を判定する。
	activeRefresh    map[string]string // plain → familyID
	rotatedRefresh   map[string]struct{}
	rotateCallCount  int
	lastRefreshToken string
}

// IssueCode は事前に「払い出した auth_code 平文」を mock に登録する。
// callback フローを通さずに token 交換だけを検証したいテストで使う。
func (m *mockNativeTokenExchangeService) IssueCode(plainCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issuedCode = plainCode
	m.used = false
}

func (m *mockNativeTokenExchangeService) ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	m.lastCode = authCode
	m.lastVerifier = codeVerifier
	// 不明 / 既使用 / verifier 空 を一律 ErrInvalidGrant に正規化（Req 2.6）
	if m.issuedCode == "" || authCode != m.issuedCode {
		return nil, auth.ErrInvalidGrant
	}
	if m.used {
		return nil, auth.ErrInvalidGrant
	}
	if codeVerifier == "" {
		return nil, auth.ErrInvalidGrant
	}
	m.used = true // 単回消費
	// rotation 通しテスト用に、払い出した refresh token を family 付きで活性化する。
	if m.activeRefresh == nil {
		m.activeRefresh = make(map[string]string)
	}
	if m.rotatedRefresh == nil {
		m.rotatedRefresh = make(map[string]struct{})
	}
	const plain = "integration-refresh-token"
	const familyID = "family-integration"
	m.activeRefresh[plain] = familyID
	return &auth.TokenPair{
		AccessToken:  "integration-access-token",
		RefreshToken: plain,
		ExpiresIn:    900,
	}, nil
}

// RotateRefreshToken は Issue #167 の rotation を simulate する。
//   - 活性集合に存在する plain → 旧 plain を rotated 集合へ move、新 plain を活性化して
//     新 TokenPair を返す（同一 family に紐付け / Req 1.3, 1.4）
//   - rotated 集合に存在する plain → ErrInvalidRefreshToken（Req 2.4: rotation 済み再利用）
//   - 活性集合にも rotated 集合にも存在しない plain → ErrInvalidRefreshToken（Req 2.1: 不明）
//
// 連続 rotation の通しテストでは「N 回目の rotation で N 番目の plain が返る」決定論を
// 担保するために、内部カウンタで新 plain を生成する。
func (m *mockNativeTokenExchangeService) RotateRefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rotateCallCount++
	m.lastRefreshToken = refreshToken

	if m.activeRefresh == nil {
		return nil, auth.ErrInvalidRefreshToken
	}
	if _, isRotated := m.rotatedRefresh[refreshToken]; isRotated {
		// rotation 済み再利用は単純拒否（#168 が family 失効へ昇格するまでは ErrInvalidRefreshToken）
		return nil, auth.ErrInvalidRefreshToken
	}
	familyID, isActive := m.activeRefresh[refreshToken]
	if !isActive {
		// 不明
		return nil, auth.ErrInvalidRefreshToken
	}

	// 旧 plain を rotated に move、新 plain を活性化（同一 family / Req 1.3）。
	delete(m.activeRefresh, refreshToken)
	if m.rotatedRefresh == nil {
		m.rotatedRefresh = make(map[string]struct{})
	}
	m.rotatedRefresh[refreshToken] = struct{}{}
	newPlain := fmt.Sprintf("integration-refresh-token-rot-%d", m.rotateCallCount)
	m.activeRefresh[newPlain] = familyID

	return &auth.TokenPair{
		AccessToken:  fmt.Sprintf("integration-access-token-rot-%d", m.rotateCallCount),
		RefreshToken: newPlain,
		ExpiresIn:    900,
	}, nil
}

// RevokeRefreshToken は TokenExchangeService の interface 追従（Issue #168 task 2）。
// revoke の意味論（family 失効）の simulate と通しシナリオは task 3 で実装する。
func (m *mockNativeTokenExchangeService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	return nil
}

// createNativeAuthIntegrationRouter は native auth 系統（login / callback / token 交換）の
// 通し検証用に、token 交換 service を stateful mock として注入した router を返す。
// 既存 createIntegrationRouter は固定 mock の listItems などに依存しているため、ここでは
// 必要最小の依存（auth + native handler のみ）に絞った別ヘルパーを用意する。
func createNativeAuthIntegrationRouter(state *integrationState, tokenSvc *mockNativeTokenExchangeService) http.Handler {
	sessionFinder := &mockSessionFinderForRouter{sessions: state.sessions}

	authService := &mockAuthService{
		getLoginURLFn: func(s string) string {
			return "https://accounts.google.com/o/oauth2/auth?state=" + s
		},
		handleNativeCallbackFn: func(ctx context.Context, code, pkceChallenge string) (string, error) {
			// callback が払い出す auth_code（平文）。token 交換 mock にも登録して
			// 単回消費の正しさを検証する。
			const plain = "integration-native-auth-code"
			tokenSvc.IssueCode(plain)
			return plain, nil
		},
	}

	nativeHandler := NewNativeAuthHandler(tokenSvc)
	deps := &RouterDeps{
		SessionFinder:     sessionFinder,
		CORSAllowedOrigin: "http://localhost:3000",
		RateLimiter:       middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig()),
		AuthService:       authService,
		AuthConfig:        AuthHandlerConfig{BaseURL: "http://localhost:3000"},
		NativeAuthHandler: nativeHandler,
	}
	return NewRouter(deps)
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
			handleNativeCallbackFn: func(ctx context.Context, code, pkceChallenge string) (string, error) {
				// native flow（#165）はセッションを作成せず auth_code のみを返す。
				return "integration-native-auth-code", nil
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
			getFeedFn: func(ctx context.Context, userID, feedID string) (*model.Feed, error) {
				f, ok := state.feeds[feedID]
				if !ok {
					return nil, nil
				}
				return f, nil
			},
			updateFeedURLFn: func(ctx context.Context, userID, feedID, newURL string) (*model.Feed, error) {
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
			// listStarredItemsFn は state.itemStates / state.items / state.feeds をスキャンして
			// 当該ユーザーがスター付与した記事を published_at 降順で返す。これにより
			// router 経由で /api/feeds/starred/items が ListStarredItems ハンドラに
			// 到達すること、および user_id によるフィルタが期待通り行われることを
			// 統合的に検証できる。state.items が空の既存テストでは空一覧が返るため、
			// 既存テストの挙動は変化しない（後方互換）。
			listStarredItemsFn: func(ctx context.Context, userID, cursor string, limit int) (*starredItemListResult, error) {
				state.lastStarredUserID = userID
				state.lastStarredCursor = cursor
				state.lastStarredLimit = limit
				state.starredCallCount++

				// 不正カーソルのパース: 既存単一フィード API と同等の RFC3339Nano → RFC3339
				// フォールバックパース（不正なら model.NewInvalidFilterError を返す）。
				var cursorTime time.Time
				if cursor != "" {
					t, err := time.Parse(time.RFC3339Nano, cursor)
					if err != nil {
						t, err = time.Parse(time.RFC3339, cursor)
						if err != nil {
							return nil, model.NewInvalidFilterError("無効なカーソル値: " + cursor)
						}
					}
					cursorTime = t
				}

				// state.itemStates から userID の is_starred=true な行を集める。
				type starredEntry struct {
					item      *model.Item
					feedTitle string
				}
				var entries []starredEntry
				for _, is := range state.itemStates {
					if is.UserID != userID || !is.IsStarred {
						continue
					}
					item, ok := state.items[is.ItemID]
					if !ok {
						continue
					}
					if !cursorTime.IsZero() {
						if item.PublishedAt == nil || !item.PublishedAt.Before(cursorTime) {
							continue
						}
					}
					feedTitle := ""
					if f, ok := state.feeds[item.FeedID]; ok && f != nil {
						feedTitle = f.Title
					}
					entries = append(entries, starredEntry{item: item, feedTitle: feedTitle})
				}

				// published_at 降順にソート。
				sort.Slice(entries, func(i, j int) bool {
					var ti, tj time.Time
					if entries[i].item.PublishedAt != nil {
						ti = *entries[i].item.PublishedAt
					}
					if entries[j].item.PublishedAt != nil {
						tj = *entries[j].item.PublishedAt
					}
					return ti.After(tj)
				})

				// limit+1 取得相当の has_more 判定。
				hasMore := false
				if limit > 0 && len(entries) > limit {
					entries = entries[:limit]
					hasMore = true
				}

				items := make([]starredItemSummaryResponse, 0, len(entries))
				for _, e := range entries {
					pubAt := time.Time{}
					if e.item.PublishedAt != nil {
						pubAt = *e.item.PublishedAt
					}
					isRead := false
					key := userID + ":" + e.item.ID
					if is, ok := state.itemStates[key]; ok && is != nil {
						isRead = is.IsRead
					}
					items = append(items, starredItemSummaryResponse{
						itemSummaryResponse: itemSummaryResponse{
							ID:          e.item.ID,
							FeedID:      e.item.FeedID,
							Title:       e.item.Title,
							Link:        e.item.Link,
							Summary:     e.item.Summary,
							PublishedAt: pubAt,
							IsRead:      isRead,
							IsStarred:   true,
							HatebuCount: e.item.HatebuCount,
						},
						FeedTitle: e.feedTitle,
					})
				}

				nextCursor := ""
				if hasMore && len(items) > 0 {
					nextCursor = items[len(items)-1].PublishedAt.Format(time.RFC3339Nano)
				}

				return &starredItemListResult{
					Items:      items,
					NextCursor: nextCursor,
					HasMore:    hasMore,
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

// TestIntegration_NativeAuthFlow_LoginCallbackReturnsAuthCode は flow=native の OAuth フローを
// 実 router で通しで検証する（#165）。
// login(native) → state / challenge cookie 発行 → callback → アプリスキームへ auth_code 付き
// 303 リダイレクト + セッション Cookie 不在。
func TestIntegration_NativeAuthFlow_LoginCallbackReturnsAuthCode(t *testing.T) {
	state := newIntegrationState()
	router := createIntegrationRouter(state)

	// 1. native ログイン: OAuth リダイレクトと両 Cookie が発行されること
	loginURL := "/auth/google/login?flow=native&code_challenge=" + nativeTestChallenge + "&code_challenge_method=S256"
	req := httptest.NewRequest(http.MethodGet, loginURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("step1: GET %s status = %d, want %d", loginURL, resp.StatusCode, http.StatusTemporaryRedirect)
	}

	var oauthStateC, nativeChallengeC *http.Cookie
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "oauth_state":
			oauthStateC = c
		case "oauth_native_challenge":
			nativeChallengeC = c
		}
	}
	if oauthStateC == nil {
		t.Fatal("step1: expected oauth_state cookie")
	}
	if nativeChallengeC == nil {
		t.Fatal("step1: expected oauth_native_challenge cookie")
	}

	// 2. コールバック: アプリスキームへ auth_code 付きでリダイレクトされること
	callbackURL := "/auth/google/callback?code=test-auth-code&state=" + oauthStateC.Value
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(oauthStateC)
	req.AddCookie(nativeChallengeC)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("step2: native callback status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	location := resp.Header.Get("Location")
	want := "feedman://auth/callback?auth_code=integration-native-auth-code"
	if location != want {
		t.Fatalf("step2: Location = %q, want %q", location, want)
	}

	// セッション Cookie が発行されないこと（Req 2.3）
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			t.Fatal("step2: session_id cookie must not be issued for native flow")
		}
	}
}

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
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("step2: callback status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
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
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("step4: POST /auth/logout status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
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
		// /api/feeds/starred/items も認証必須であることを検証する（Requirement 4.6）。
		{http.MethodGet, "/api/feeds/starred/items", ""},
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

// --- 横断スター記事一覧（Issue #117 / Task 4）の結合テスト ---

// seedStarredFixture は横断スター記事一覧の結合テスト用フィクスチャを state に積む。
// user-test（メインユーザー）と other-user（クロスユーザー汚染検証用）を作成し、
// 複数フィードに記事と is_starred=true の state 行を配置する。
// 返り値は user-test 用のセッション ID。
func seedStarredFixture(state *integrationState) string {
	now := time.Now().UTC().Truncate(time.Second)

	// セッションとユーザー（user-test = 検証対象、other-user = クロスユーザー）。
	state.sessions["session-user-test"] = &model.Session{
		ID:        "session-user-test",
		UserID:    "user-test",
		ExpiresAt: now.Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{
		ID:    "user-test",
		Email: "user-test@example.com",
		Name:  "User Test",
	}
	state.users["other-user"] = &model.User{
		ID:    "other-user",
		Email: "other-user@example.com",
		Name:  "Other User",
	}

	// 2 つのフィードを配置（複数フィードにまたがる検証用）。
	state.feeds["feed-A"] = &model.Feed{
		ID:    "feed-A",
		Title: "Feed Alpha",
	}
	state.feeds["feed-B"] = &model.Feed{
		ID:    "feed-B",
		Title: "Feed Beta",
	}

	// 記事を 3 件配置（feed-A に 2 件、feed-B に 1 件）。
	pub1 := now.Add(-1 * time.Hour)
	pub2 := now.Add(-2 * time.Hour)
	pub3 := now.Add(-3 * time.Hour)
	state.items["item-1"] = &model.Item{
		ID:          "item-1",
		FeedID:      "feed-A",
		Title:       "Article 1 (newest)",
		Link:        "https://example.com/1",
		PublishedAt: &pub1,
	}
	state.items["item-2"] = &model.Item{
		ID:          "item-2",
		FeedID:      "feed-B",
		Title:       "Article 2 (middle)",
		Link:        "https://example.com/2",
		PublishedAt: &pub2,
	}
	state.items["item-3"] = &model.Item{
		ID:          "item-3",
		FeedID:      "feed-A",
		Title:       "Article 3 (oldest)",
		Link:        "https://example.com/3",
		PublishedAt: &pub3,
	}

	// user-test は item-1 / item-3 にスターを付ける（feed-A）+ item-2（feed-B）にスター。
	// 結果として 3 件すべてが横断スター一覧に出る想定。
	state.itemStates["user-test:item-1"] = &model.ItemState{
		UserID:    "user-test",
		ItemID:    "item-1",
		IsStarred: true,
	}
	state.itemStates["user-test:item-2"] = &model.ItemState{
		UserID:    "user-test",
		ItemID:    "item-2",
		IsStarred: true,
	}
	state.itemStates["user-test:item-3"] = &model.ItemState{
		UserID:    "user-test",
		ItemID:    "item-3",
		IsStarred: true,
	}

	// other-user は item-1 にスター（クロスユーザー汚染検証用 / NFR 2.1）。
	state.itemStates["other-user:item-1"] = &model.ItemState{
		UserID:    "other-user",
		ItemID:    "item-1",
		IsStarred: true,
	}
	// other-user は item-2 をスター解除した状態（is_starred=false）。
	// 別ユーザーの is_starred=false 行が混入しないことを副次的に確認。
	state.itemStates["other-user:item-2"] = &model.ItemState{
		UserID:    "other-user",
		ItemID:    "item-2",
		IsStarred: false,
	}

	return "session-user-test"
}

// TestIntegration_ListStarredItems_OnlyOwnStarredItems は認証クッキー付きで
// /api/feeds/starred/items を呼んだとき、自ユーザーのスター記事のみが
// published_at 降順で含まれ、他ユーザーのスター記事が一切混入しないこと、
// 各 item に feed_id / feed_title / is_starred=true が含まれることを検証する。
// Requirement 4.1 / 4.2 / 4.9 / 4.10 / 要件 2.4 / NFR 2.1 / Task 4 (a)(b) に対応。
func TestIntegration_ListStarredItems_OnlyOwnStarredItems(t *testing.T) {
	// Arrange
	state := newIntegrationState()
	sessionID := seedStarredFixture(state)
	router := createIntegrationRouter(state)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: status 200 / Content-Type
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %T", body["items"])
	}

	// user-test のスター記事は 3 件のみ（item-1 / item-2 / item-3）。
	// other-user のスター記事（item-1 への other-user 行）は混入しない。
	if len(items) != 3 {
		t.Fatalf("items length = %d, want 3 (user-test のみのスター記事数)", len(items))
	}

	// published_at 降順: item-1 (最新) → item-2 → item-3 (最古) の順。
	wantIDs := []string{"item-1", "item-2", "item-3"}
	wantFeedIDs := []string{"feed-A", "feed-B", "feed-A"}
	wantFeedTitles := []string{"Feed Alpha", "Feed Beta", "Feed Alpha"}
	for i, raw := range items {
		row, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("items[%d] is not an object", i)
		}
		if row["id"] != wantIDs[i] {
			t.Errorf("items[%d].id = %v, want %q", i, row["id"], wantIDs[i])
		}
		if row["feed_id"] != wantFeedIDs[i] {
			t.Errorf("items[%d].feed_id = %v, want %q", i, row["feed_id"], wantFeedIDs[i])
		}
		if row["feed_title"] != wantFeedTitles[i] {
			t.Errorf("items[%d].feed_title = %v, want %q (要件 2.4)", i, row["feed_title"], wantFeedTitles[i])
		}
		if row["is_starred"] != true {
			t.Errorf("items[%d].is_starred = %v, want true", i, row["is_starred"])
		}
	}

	// NFR 2.1（クロスユーザー漏洩なし）: other-user の記事は応答に含まれない。
	// other-user は item-1 にしかスターを付けていないが、item-1 自体は user-test も
	// スターを付けているため、応答に item-1 が含まれること自体は問題ではない。
	// 件数が 3 件で固定されていることが「other-user 単独の項目が混入していない」
	// 直接的な担保となる（他ユーザーが付けたスター記事だけを別 item として持たせる
	// fixture でも漏洩しないことを次のテストで補強する）。

	// router 経由のルーティング到達性: state.lastStarredUserID に user-test が記録されること。
	if state.lastStarredUserID != "user-test" {
		t.Errorf("listStarredItems was called with userID = %q, want %q (ListStarredItems ハンドラに到達していない可能性)",
			state.lastStarredUserID, "user-test")
	}
	if state.starredCallCount != 1 {
		t.Errorf("listStarredItems call count = %d, want 1", state.starredCallCount)
	}
}

// TestIntegration_ListStarredItems_NoOtherUsersItemsLeaked は、user-test 自身が
// 一切スターしていない記事を他ユーザーがスターしているケースで、
// 当該記事が user-test の横断スター一覧に絶対に混入しないことを検証する。
// NFR 2.1（クロスユーザー漏洩防止）の strict invariant 担保。
func TestIntegration_ListStarredItems_NoOtherUsersItemsLeaked(t *testing.T) {
	// Arrange
	state := newIntegrationState()
	now := time.Now().UTC().Truncate(time.Second)
	state.sessions["session-user-test"] = &model.Session{
		ID:        "session-user-test",
		UserID:    "user-test",
		ExpiresAt: now.Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{ID: "user-test", Email: "u@e.com", Name: "U"}
	state.users["other-user"] = &model.User{ID: "other-user", Email: "o@e.com", Name: "O"}

	state.feeds["feed-X"] = &model.Feed{ID: "feed-X", Title: "Feed X"}

	// other-user 専用記事: other-user のみスター、user-test は触っていない。
	pub := now.Add(-1 * time.Hour)
	state.items["item-other-only"] = &model.Item{
		ID:          "item-other-only",
		FeedID:      "feed-X",
		Title:       "Other User Only Article",
		Link:        "https://example.com/other",
		PublishedAt: &pub,
	}
	state.itemStates["other-user:item-other-only"] = &model.ItemState{
		UserID:    "other-user",
		ItemID:    "item-other-only",
		IsStarred: true,
	}

	router := createIntegrationRouter(state)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-user-test"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: user-test の応答は空であること（他ユーザーの記事が混入しない）。
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	items, _ := body["items"].([]interface{})
	if len(items) != 0 {
		t.Errorf("items length = %d, want 0 (他ユーザーのスター記事が混入している / NFR 2.1 違反)", len(items))
	}
	if hasMore, _ := body["has_more"].(bool); hasMore {
		t.Errorf("has_more = true, want false (スター 0 件)")
	}
}

// TestIntegration_ListStarredItems_EmptyResult はスター 0 件のユーザーで
// 200 / items=[] / has_more=false が返ることを検証する（Requirement 4.7 / Task 4 (c)）。
func TestIntegration_ListStarredItems_EmptyResult(t *testing.T) {
	// Arrange: ユーザーは存在するがスター記事を 1 件も持たない。
	state := newIntegrationState()
	state.sessions["session-zero"] = &model.Session{
		ID:        "session-zero",
		UserID:    "user-zero",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-zero"] = &model.User{ID: "user-zero", Email: "z@e.com", Name: "Z"}

	router := createIntegrationRouter(state)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-zero"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	bodyBytes := w.Body.Bytes()
	// items は null ではなく [] で返る（NFR 3.1: 既存応答スキーマと区別不能 = 配列フィールドは
	// 常に配列であり null にならない）。
	if !strings.Contains(string(bodyBytes), `"items":[]`) {
		t.Errorf("expected items=[] in JSON, got %s", string(bodyBytes))
	}

	var body map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(string(bodyBytes))).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if hasMore, _ := body["has_more"].(bool); hasMore {
		t.Errorf("has_more = true, want false")
	}
	if nc, ok := body["next_cursor"]; ok && nc != nil && nc != "" {
		t.Errorf("next_cursor = %v, want absent or empty", nc)
	}
}

// TestIntegration_ListStarredItems_InvalidCursor_Returns400 は不正な cursor 文字列に対して
// 既存単一フィード API と同等のクライアントエラー（400 / INVALID_FILTER）が返ることを検証する
// （Requirement 4.8 / Task 4 (d)）。
func TestIntegration_ListStarredItems_InvalidCursor_Returns400(t *testing.T) {
	// Arrange
	state := newIntegrationState()
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{ID: "user-test", Email: "t@e.com", Name: "T"}

	router := createIntegrationRouter(state)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items?cursor=not-a-valid-time", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (不正カーソルで 400)", resp.StatusCode, http.StatusBadRequest)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if code, _ := body["code"].(string); code != model.ErrCodeInvalidFilter {
		t.Errorf("code = %q, want %q", code, model.ErrCodeInvalidFilter)
	}
	// 応答ボディに items は含まれない（エラー応答は APIError 形式）。
	if _, ok := body["items"]; ok {
		t.Error("expected no items field in error response")
	}
}

// TestIntegration_ListStarredItems_Unauthorized_Returns401 はセッションクッキーなしの
// リクエストが 401 を返すことを直接検証する（Requirement 4.6 / Task 4 (e)）。
// 同様の挙動は TestIntegration_ProtectedEndpoints_RequireAuth でも担保されているが、
// 横断スター固有の追加検証として「session middleware 段階で 401 を返し、service 層が
// 呼ばれない（= 記事データを応答に含めない）」ことを確認する目的で個別テストを置く。
func TestIntegration_ListStarredItems_Unauthorized_Returns401(t *testing.T) {
	// Arrange
	state := newIntegrationState()
	router := createIntegrationRouter(state)

	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items", nil)
	// セッションクッキー無し。
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// 応答ボディに items を含めない（Requirement 4.6）。session middleware の 401 は
	// plain text "unauthorized" を返す実装のため、ここでは JSON パースを期待せず、
	// 応答に "items" 文字列が含まれないことで担保する。
	bodyBytes := w.Body.Bytes()
	if bytes.Contains(bodyBytes, []byte(`"items"`)) {
		t.Errorf("expected no items field in 401 response, got body: %s", string(bodyBytes))
	}

	// service 層が呼ばれていないこと（middleware 段階で 401 を返している）も確認。
	if state.starredCallCount != 0 {
		t.Errorf("listStarredItems was called %d times, want 0 (401 should not reach service layer)",
			state.starredCallCount)
	}
}

// TestIntegration_ListItems_ByFeedID_StillWorksAfterStarredRouteAdded は、
// /api/feeds/starred/items ルート追加後も既存 /api/feeds/{id}/items が変化せず動作すること、
// および chi のトライ木で `starred` が動的パラメータ `{id}` より優先されることに依存せず、
// /api/feeds/{id}/items が ListItems ハンドラに到達することを検証する
// （Requirement 5.1 / 5.3 / Task 4 既存挙動の非干渉確認）。
func TestIntegration_ListItems_ByFeedID_StillWorksAfterStarredRouteAdded(t *testing.T) {
	// Arrange: 既存 createIntegrationRouter の listItemsFn は固定で
	// item-integration-1 / FeedID=path の Title=Integration Item を返す。
	state := newIntegrationState()
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{ID: "user-test", Email: "t@e.com", Name: "T"}

	router := createIntegrationRouter(state)

	// feed-1 と starred 以外の通常の feed_id でリクエスト。
	const feedID = "feed-some-id"
	req := httptest.NewRequest(http.MethodGet, "/api/feeds/"+feedID+"/items", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: 既存 ListItems ハンドラが到達し、その固定モック応答が返ること。
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := body["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected exactly 1 item from ListItems mock, got %v", body["items"])
	}
	first, _ := items[0].(map[string]interface{})
	if first["id"] != "item-integration-1" {
		t.Errorf("items[0].id = %v, want %q (ListItems ハンドラに到達していない可能性)",
			first["id"], "item-integration-1")
	}
	if first["title"] != "Integration Item" {
		t.Errorf("items[0].title = %v, want %q", first["title"], "Integration Item")
	}
	if first["feed_id"] != feedID {
		t.Errorf("items[0].feed_id = %v, want %q", first["feed_id"], feedID)
	}
	// has_more が false（ListItems モックの固定挙動）。
	if hm, _ := body["has_more"].(bool); hm {
		t.Errorf("has_more = true, want false (既存 ListItems モックの固定挙動)")
	}

	// /starred/items 側のハンドラは呼ばれていないこと（誤ディスパッチがないこと）。
	if state.starredCallCount != 0 {
		t.Errorf("listStarredItems was called %d times, want 0 (誤ディスパッチの疑い)",
			state.starredCallCount)
	}
}

// TestIntegration_ListStarredItems_CursorPropagation は cursor クエリパラメータが
// router 経由で service 層まで伝搬することを検証する（Requirement 4.5）。
func TestIntegration_ListStarredItems_CursorPropagation(t *testing.T) {
	// Arrange
	state := newIntegrationState()
	state.sessions["session-test"] = &model.Session{
		ID:        "session-test",
		UserID:    "user-test",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	state.users["user-test"] = &model.User{ID: "user-test", Email: "t@e.com", Name: "T"}

	router := createIntegrationRouter(state)

	const cursorValue = "2026-02-27T10:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/feeds/starred/items?cursor="+cursorValue, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "session-test"})
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert: cursor が service 層へ伝搬している。
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if state.lastStarredCursor != cursorValue {
		t.Errorf("cursor propagated to service = %q, want %q", state.lastStarredCursor, cursorValue)
	}
	if state.lastStarredLimit != defaultItemsPerPage {
		t.Errorf("limit propagated to service = %d, want %d (default)",
			state.lastStarredLimit, defaultItemsPerPage)
	}
}

// --- Native Auth Token 交換の通し統合テスト（Issue #166 / task 6） ---

// TestIntegration_NativeAuthFlow_CallbackThenExchangeSucceeds は flow=native の OAuth
// callback で発行された auth_code を POST /api/auth/token で交換し、200 で 4 フィールドが
// 返ることを通しで検証する（Req 1.1, 1.2, 1.5, 1.6）。
func TestIntegration_NativeAuthFlow_CallbackThenExchangeSucceeds(t *testing.T) {
	// Arrange
	state := newIntegrationState()
	tokenSvc := &mockNativeTokenExchangeService{}
	router := createNativeAuthIntegrationRouter(state, tokenSvc)

	// 1. native login: state / challenge Cookie を取得
	loginURL := "/auth/google/login?flow=native&code_challenge=" + nativeTestChallenge + "&code_challenge_method=S256"
	req := httptest.NewRequest(http.MethodGet, loginURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("step1: login status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	var oauthStateC, nativeChallengeC *http.Cookie
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "oauth_state":
			oauthStateC = c
		case "oauth_native_challenge":
			nativeChallengeC = c
		}
	}
	if oauthStateC == nil || nativeChallengeC == nil {
		t.Fatal("step1: expected both oauth_state and oauth_native_challenge cookies")
	}

	// 2. callback: アプリスキームへ auth_code が返る
	callbackURL := "/auth/google/callback?code=test-auth-code&state=" + oauthStateC.Value
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(oauthStateC)
	req.AddCookie(nativeChallengeC)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("step2: callback status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	location := resp.Header.Get("Location")
	const wantAuthCode = "integration-native-auth-code"
	wantLocation := "feedman://auth/callback?auth_code=" + wantAuthCode
	if location != wantLocation {
		t.Fatalf("step2: Location = %q, want %q", location, wantLocation)
	}

	// 3. token 交換: 当該 auth_code を POST /api/auth/token に送ると 200 で 4 フィールド
	body := fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, wantAuthCode, "plain-verifier")
	req = httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("step3: token exchange status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("step3: Content-Type = %q, want %q", ct, "application/json")
	}

	var tokenBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&tokenBody); err != nil {
		t.Fatalf("step3: decode token body: %v", err)
	}
	if tokenBody["access_token"] != "integration-access-token" {
		t.Errorf("step3: access_token = %v, want %q", tokenBody["access_token"], "integration-access-token")
	}
	if tokenBody["refresh_token"] != "integration-refresh-token" {
		t.Errorf("step3: refresh_token = %v, want %q", tokenBody["refresh_token"], "integration-refresh-token")
	}
	if tokenBody["token_type"] != "Bearer" {
		t.Errorf("step3: token_type = %v, want %q (Req 1.1)", tokenBody["token_type"], "Bearer")
	}
	if v, _ := tokenBody["expires_in"].(float64); v != 900 {
		t.Errorf("step3: expires_in = %v, want 900 (Req 1.6)", v)
	}
	if tokenSvc.lastVerifier != "plain-verifier" {
		t.Errorf("step3: service.lastVerifier = %q, want %q", tokenSvc.lastVerifier, "plain-verifier")
	}
}

// TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400 は同一 auth_code の 2 度目の
// 交換が 400 INVALID_GRANT で拒否されることを検証する（Req 1.2: 単回消費 / Req 2.6: uniform）。
func TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400(t *testing.T) {
	// Arrange: native login → callback で auth_code を払い出した状態にする
	state := newIntegrationState()
	tokenSvc := &mockNativeTokenExchangeService{}
	router := createNativeAuthIntegrationRouter(state, tokenSvc)

	loginURL := "/auth/google/login?flow=native&code_challenge=" + nativeTestChallenge + "&code_challenge_method=S256"
	req := httptest.NewRequest(http.MethodGet, loginURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	var oauthStateC, nativeChallengeC *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "oauth_state" {
			oauthStateC = c
		}
		if c.Name == "oauth_native_challenge" {
			nativeChallengeC = c
		}
	}
	callbackURL := "/auth/google/callback?code=test-auth-code&state=" + oauthStateC.Value
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(oauthStateC)
	req.AddCookie(nativeChallengeC)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	const wantAuthCode = "integration-native-auth-code"

	// 1 回目の交換: 成功
	body := fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, wantAuthCode, "plain-verifier")
	req = httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("1 回目の交換が成功しない: status = %d", w.Result().StatusCode)
	}

	// Act: 同一 auth_code で 2 回目の交換
	req = httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert: 400 INVALID_GRANT
	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("2 回目の交換 status = %d, want %d (Req 1.2: 単回消費)", resp.StatusCode, http.StatusBadRequest)
	}
	var body2 map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body2)
	if body2["code"] != "INVALID_GRANT" {
		t.Errorf("2 回目の code = %v, want %q (Req 2.6 uniform 拒否)", body2["code"], "INVALID_GRANT")
	}
	if body2["category"] != "auth" {
		t.Errorf("2 回目の category = %v, want %q", body2["category"], "auth")
	}
	// 詳細な拒否理由を反射しない（Req 2.6）
	if msg, _ := body2["message"].(string); strings.Contains(msg, "used") || strings.Contains(msg, "expired") {
		t.Errorf("message %q must not differentiate rejection reasons", msg)
	}
}

// TestIntegration_NativeAuthFlow_UnknownAuthCodeReturns400 は callback を経由せず未知の
// auth_code を直接送ったとき 400 INVALID_GRANT が返ることを検証する（Req 2.2, 2.6）。
func TestIntegration_NativeAuthFlow_UnknownAuthCodeReturns400(t *testing.T) {
	// Arrange: callback を経由せず token 交換だけ呼ぶ
	state := newIntegrationState()
	tokenSvc := &mockNativeTokenExchangeService{}
	router := createNativeAuthIntegrationRouter(state, tokenSvc)

	body := `{"auth_code":"unknown-code","code_verifier":"plain-verifier"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var resBody map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&resBody)
	if resBody["code"] != "INVALID_GRANT" {
		t.Errorf("code = %v, want %q (Req 2.2 / 2.6)", resBody["code"], "INVALID_GRANT")
	}
}

// --- Refresh ローテーションの通し統合テスト（Issue #167 / task 3） ---

// runNativeLoginCallbackAndExchange は native login → callback → token 交換の通しを
// 実行し、初回 refresh token 平文を返す。本ヘルパーは Refresh 通しテストで
// 「正常な refresh token を払い出した状態」を作るために使う。
func runNativeLoginCallbackAndExchange(t *testing.T, router http.Handler) (refreshToken string) {
	t.Helper()

	// 1. native login: state / challenge Cookie を取得
	loginURL := "/auth/google/login?flow=native&code_challenge=" + nativeTestChallenge + "&code_challenge_method=S256"
	req := httptest.NewRequest(http.MethodGet, loginURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	var oauthStateC, nativeChallengeC *http.Cookie
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "oauth_state":
			oauthStateC = c
		case "oauth_native_challenge":
			nativeChallengeC = c
		}
	}
	if oauthStateC == nil || nativeChallengeC == nil {
		t.Fatal("login: expected both oauth_state and oauth_native_challenge cookies")
	}

	// 2. callback: アプリスキームへ auth_code が返る
	callbackURL := "/auth/google/callback?code=test-auth-code&state=" + oauthStateC.Value
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(oauthStateC)
	req.AddCookie(nativeChallengeC)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp = w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	const wantAuthCode = "integration-native-auth-code"
	wantLocation := "feedman://auth/callback?auth_code=" + wantAuthCode
	if location := resp.Header.Get("Location"); location != wantLocation {
		t.Fatalf("callback Location = %q, want %q", location, wantLocation)
	}

	// 3. token 交換: 200 で 4 フィールドを取得
	body := fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, wantAuthCode, "plain-verifier")
	req = httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token exchange status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var tokenBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&tokenBody); err != nil {
		t.Fatalf("token exchange decode body: %v", err)
	}
	rt, ok := tokenBody["refresh_token"].(string)
	if !ok || rt == "" {
		t.Fatalf("token exchange: refresh_token = %v, want non-empty string", tokenBody["refresh_token"])
	}
	return rt
}

// TestIntegration_RefreshFlow_TokenExchangeRefreshSucceedsThenOldTokenRejected は
// 「token 交換で pair 取得 → 旧 refresh token で refresh 成功（新 pair 取得）→
// 旧 refresh token で再 refresh が 401 INVALID_REFRESH_TOKEN」の通しケースを検証する。
// Issue #167 の AC（Req 1.1, 1.2: 新 pair / 旧 token 拒否）に対応する。
func TestIntegration_RefreshFlow_TokenExchangeRefreshSucceedsThenOldTokenRejected(t *testing.T) {
	// Arrange: native login → callback → token 交換で初回 refresh token を取得
	state := newIntegrationState()
	tokenSvc := &mockNativeTokenExchangeService{}
	router := createNativeAuthIntegrationRouter(state, tokenSvc)
	oldRefreshToken := runNativeLoginCallbackAndExchange(t, router)

	// Act 1: 旧 refresh token で refresh 成功（新 pair 取得 / Req 1.1）
	body := fmt.Sprintf(`{"refresh_token":%q}`, oldRefreshToken)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh 1st status = %d, want %d (Req 1.1)", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("refresh 1st Content-Type = %q, want %q", ct, "application/json")
	}
	var newTokenBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&newTokenBody); err != nil {
		t.Fatalf("refresh 1st decode body: %v", err)
	}
	// 4 フィールドが揃う（Req 1.1, 1.6: token 交換と同形）
	newAccess, _ := newTokenBody["access_token"].(string)
	newRefresh, _ := newTokenBody["refresh_token"].(string)
	if newAccess == "" {
		t.Error("refresh 1st: access_token is empty")
	}
	if newRefresh == "" {
		t.Error("refresh 1st: refresh_token is empty")
	}
	if newTokenBody["token_type"] != "Bearer" {
		t.Errorf("refresh 1st: token_type = %v, want %q", newTokenBody["token_type"], "Bearer")
	}
	if v, _ := newTokenBody["expires_in"].(float64); v != 900 {
		t.Errorf("refresh 1st: expires_in = %v, want 900", v)
	}
	// 新 refresh token は旧と異なる（rotation の本質 / Req 1.2）
	if newRefresh == oldRefreshToken {
		t.Errorf("refresh 1st: new refresh_token equals old (rotation must produce different token)")
	}

	// Act 2: 同じ旧 refresh token で再 refresh → 401 INVALID_REFRESH_TOKEN（Req 1.2 / 2.4）
	req = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh 2nd with old token status = %d, want %d (Req 1.2 / 2.4 / SERVER.md §1.3)",
			resp.StatusCode, http.StatusUnauthorized)
	}
	var rejectBody map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rejectBody)
	if rejectBody["code"] != "INVALID_REFRESH_TOKEN" {
		t.Errorf("refresh 2nd: code = %v, want %q (Req 2.6 uniform 拒否)",
			rejectBody["code"], "INVALID_REFRESH_TOKEN")
	}
	if rejectBody["category"] != "auth" {
		t.Errorf("refresh 2nd: category = %v, want %q", rejectBody["category"], "auth")
	}
	// 詳細な拒否理由を反射しない（Req 2.6 / NFR 1.3）
	if msg, _ := rejectBody["message"].(string); strings.Contains(msg, "rotated") || strings.Contains(msg, "expired") || strings.Contains(msg, "revoked") {
		t.Errorf("refresh 2nd: message %q must not differentiate rejection reasons", msg)
	}

	// 新 refresh token は依然有効（rotation 後に発行された世代）
	body2 := fmt.Sprintf(`{"refresh_token":%q}`, newRefresh)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body2))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("refresh with new token status = %d, want 200 (新 token は有効)",
			w.Result().StatusCode)
	}
}

// TestIntegration_RefreshFlow_UnknownRefreshTokenReturns401 は callback / token 交換を
// 経由せず未知の refresh token を送ったとき 401 INVALID_REFRESH_TOKEN が返ることを
// 検証する（Req 2.1, 2.6）。
func TestIntegration_RefreshFlow_UnknownRefreshTokenReturns401(t *testing.T) {
	// Arrange: 何も払い出していない状態で refresh だけ呼ぶ
	state := newIntegrationState()
	tokenSvc := &mockNativeTokenExchangeService{}
	router := createNativeAuthIntegrationRouter(state, tokenSvc)

	body := `{"refresh_token":"unknown-refresh-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (Req 2.1)", resp.StatusCode, http.StatusUnauthorized)
	}
	var resBody map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&resBody)
	if resBody["code"] != "INVALID_REFRESH_TOKEN" {
		t.Errorf("code = %v, want %q (Req 2.6)", resBody["code"], "INVALID_REFRESH_TOKEN")
	}
}
