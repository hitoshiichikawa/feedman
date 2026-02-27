package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- RateLimitMiddleware (API全般) のテスト ---

func TestRateLimitMiddleware_AllowsRequestsWithinLimit(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  2,  // 2 req/sec
		GeneralBurst: 5,  // バースト5
		FeedRegRate:  1,  // 未使用
		FeedRegBurst: 10, // 未使用
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handlerCallCount := 0
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCallCount++
		w.WriteHeader(http.StatusOK)
	}))

	// バースト内の5リクエストは全て通る
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		ctx := context.WithValue(req.Context(), userIDContextKey, "user-1")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i, w.Result().StatusCode, http.StatusOK)
		}
	}

	if handlerCallCount != 5 {
		t.Errorf("handler call count = %d, want 5", handlerCallCount)
	}
}

func TestRateLimitMiddleware_Returns429WhenLimitExceeded(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  1,  // 1 req/sec
		GeneralBurst: 2,  // バースト2
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// バースト分（2回）は通る
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		ctx := context.WithValue(req.Context(), userIDContextKey, "user-rate-limit")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i, w.Result().StatusCode, http.StatusOK)
		}
	}

	// 3回目はレート制限に引っかかる
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := context.WithValue(req.Context(), userIDContextKey, "user-rate-limit")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddleware_Returns429WithRetryAfterHeader(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  1,  // 1 req/sec
		GeneralBurst: 1,  // バースト1
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1回目は通る
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := context.WithValue(req.Context(), userIDContextKey, "user-retry-after")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// 2回目は429になる
	req2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx2 := context.WithValue(req2.Context(), userIDContextKey, "user-retry-after")
	req2 = req2.WithContext(ctx2)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w2.Result().StatusCode, http.StatusTooManyRequests)
	}

	retryAfter := w2.Result().Header.Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header to be present")
	}

	// Retry-Afterは数値（秒）であること
	retrySeconds, err := strconv.Atoi(retryAfter)
	if err != nil {
		t.Errorf("Retry-After header should be a number, got %q", retryAfter)
	}
	if retrySeconds < 1 {
		t.Errorf("Retry-After = %d, should be at least 1", retrySeconds)
	}
}

func TestRateLimitMiddleware_IsolatesUserRateLimits(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  1,  // 1 req/sec
		GeneralBurst: 1,  // バースト1
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// ユーザーAの1回目は通る
	reqA := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctxA := context.WithValue(reqA.Context(), userIDContextKey, "user-A")
	reqA = reqA.WithContext(ctxA)
	wA := httptest.NewRecorder()
	handler.ServeHTTP(wA, reqA)

	if wA.Result().StatusCode != http.StatusOK {
		t.Errorf("user-A first request: status = %d, want %d", wA.Result().StatusCode, http.StatusOK)
	}

	// ユーザーAの2回目は429
	reqA2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctxA2 := context.WithValue(reqA2.Context(), userIDContextKey, "user-A")
	reqA2 = reqA2.WithContext(ctxA2)
	wA2 := httptest.NewRecorder()
	handler.ServeHTTP(wA2, reqA2)

	if wA2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("user-A second request: status = %d, want %d", wA2.Result().StatusCode, http.StatusTooManyRequests)
	}

	// ユーザーBの1回目は通る（ユーザーAのレートに影響されない）
	reqB := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctxB := context.WithValue(reqB.Context(), userIDContextKey, "user-B")
	reqB = reqB.WithContext(ctxB)
	wB := httptest.NewRecorder()
	handler.ServeHTTP(wB, reqB)

	if wB.Result().StatusCode != http.StatusOK {
		t.Errorf("user-B first request: status = %d, want %d", wB.Result().StatusCode, http.StatusOK)
	}
}

func TestRateLimitMiddleware_NoUserID_Returns401(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  2,
		GeneralBurst: 5,
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without user ID")
	}))

	// コンテキストにユーザーIDがない場合は401
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Result().StatusCode, http.StatusUnauthorized)
	}
}

// --- FeedRegistrationRateLimit のテスト ---

func TestFeedRegistrationRateLimit_AllowsRequestsWithinLimit(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  100, // 高い値（制限に引っかからないように）
		GeneralBurst: 200,
		FeedRegRate:  1,  // 1 req/sec
		FeedRegBurst: 3,  // バースト3
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.FeedRegistrationMiddleware()

	handlerCallCount := 0
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCallCount++
		w.WriteHeader(http.StatusOK)
	}))

	// バースト内の3リクエストは全て通る
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/feeds", nil)
		ctx := context.WithValue(req.Context(), userIDContextKey, "user-feed-reg")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i, w.Result().StatusCode, http.StatusOK)
		}
	}

	if handlerCallCount != 3 {
		t.Errorf("handler call count = %d, want 3", handlerCallCount)
	}
}

func TestFeedRegistrationRateLimit_Returns429WhenLimitExceeded(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  100,
		GeneralBurst: 200,
		FeedRegRate:  1,  // 1 req/sec
		FeedRegBurst: 1,  // バースト1
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.FeedRegistrationMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1回目は通る
	req1 := httptest.NewRequest(http.MethodPost, "/api/feeds", nil)
	ctx1 := context.WithValue(req1.Context(), userIDContextKey, "user-feed-429")
	req1 = req1.WithContext(ctx1)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Result().StatusCode != http.StatusOK {
		t.Errorf("request 1: status = %d, want %d", w1.Result().StatusCode, http.StatusOK)
	}

	// 2回目は429
	req2 := httptest.NewRequest(http.MethodPost, "/api/feeds", nil)
	ctx2 := context.WithValue(req2.Context(), userIDContextKey, "user-feed-429")
	req2 = req2.WithContext(ctx2)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("request 2: status = %d, want %d", w2.Result().StatusCode, http.StatusTooManyRequests)
	}

	retryAfter := w2.Result().Header.Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header to be present")
	}
}

func TestFeedRegistrationRateLimit_IndependentFromGeneralLimit(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  1,
		GeneralBurst: 1,
		FeedRegRate:  1,
		FeedRegBurst: 1,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	generalMW := rl.GeneralMiddleware()
	feedRegMW := rl.FeedRegistrationMiddleware()

	// General MWでリクエスト（バーストを消費）
	generalHandler := generalMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx1 := context.WithValue(req1.Context(), userIDContextKey, "user-indep")
	req1 = req1.WithContext(ctx1)
	w1 := httptest.NewRecorder()
	generalHandler.ServeHTTP(w1, req1)

	// General limitは使い果たした。Feed Registration limitはまだ使える
	feedRegHandler := feedRegMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req2 := httptest.NewRequest(http.MethodPost, "/api/feeds", nil)
	ctx2 := context.WithValue(req2.Context(), userIDContextKey, "user-indep")
	req2 = req2.WithContext(ctx2)
	w2 := httptest.NewRecorder()
	feedRegHandler.ServeHTTP(w2, req2)

	if w2.Result().StatusCode != http.StatusOK {
		t.Errorf("feed registration should still be allowed: status = %d, want %d",
			w2.Result().StatusCode, http.StatusOK)
	}
}

// --- 429レスポンスフォーマットのテスト ---

func TestRateLimitMiddleware_429ResponseIsJSON(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  1,
		GeneralBurst: 1,
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// バースト消費
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := context.WithValue(req.Context(), userIDContextKey, "user-json-test")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// 429レスポンス
	req2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx2 := context.WithValue(req2.Context(), userIDContextKey, "user-json-test")
	req2 = req2.WithContext(ctx2)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	resp := w2.Result()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["code"] == "" {
		t.Error("expected 'code' field in error response")
	}
	if body["message"] == "" {
		t.Error("expected 'message' field in error response")
	}
	if body["category"] == "" {
		t.Error("expected 'category' field in error response")
	}
}

// --- クリーンアップのテスト ---

func TestRateLimiter_CleanupRemovesExpiredEntries(t *testing.T) {
	cfg := RateLimiterConfig{
		GeneralRate:  2,
		GeneralBurst: 5,
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 50 * time.Millisecond, // テスト用に短く
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	mw := rl.GeneralMiddleware()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// ユーザーのリクエストを発行してエントリを作成
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := context.WithValue(req.Context(), userIDContextKey, "user-cleanup")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// エントリが存在することを確認
	if rl.GeneralLimiterCount() == 0 {
		t.Fatal("expected at least one limiter entry")
	}

	// クリーンアップが実行されるのを待つ
	// クリーンアップ間隔の3倍待つことで確実に実行される
	time.Sleep(200 * time.Millisecond)

	// クリーンアップ後にエントリが削除されていること
	// （最後のアクセスからcleanupIntervalを超えたエントリが削除される）
	// 注意: ここではクリーンアップが正常に動作する仕組みの存在を確認
	// エントリのTTLはcleanupIntervalの2倍
	// 50ms * 2 = 100ms がTTL、200ms待てば削除されるはず
	if count := rl.GeneralLimiterCount(); count != 0 {
		t.Errorf("expected 0 limiter entries after cleanup, got %d", count)
	}
}

// --- ミドルウェアチェーンとの統合テスト ---

func TestRateLimitMiddleware_InChainWithSessionAndCORS(t *testing.T) {
	repo := &mockSessionRepository{
		findByIDFn: func(ctx context.Context, id string) (*model.Session, error) {
			if id == "rate-limit-session" {
				return &model.Session{
					ID:        "rate-limit-session",
					UserID:    "user-rate-chain",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return nil, nil
		},
	}

	cfg := RateLimiterConfig{
		GeneralRate:  1,
		GeneralBurst: 2,
		FeedRegRate:  1,
		FeedRegBurst: 10,
		CleanupInterval: 1 * time.Minute,
	}

	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	sessionMW := NewSessionMiddleware(repo)
	corsMW := NewCORSMiddleware("http://localhost:3000")
	rateMW := rl.GeneralMiddleware()

	// CORS -> Session -> RateLimit -> Handler
	handler := corsMW(sessionMW(rateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := UserIDFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
	}))))

	// GETリクエスト：2回通る
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "rate-limit-session"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i, w.Result().StatusCode, http.StatusOK)
		}
	}

	// 3回目は429
	req3 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req3.AddCookie(&http.Cookie{Name: "session_id", Value: "rate-limit-session"})
	w3 := httptest.NewRecorder()

	handler.ServeHTTP(w3, req3)

	if w3.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("request 3: status = %d, want %d", w3.Result().StatusCode, http.StatusTooManyRequests)
	}
}

// --- デフォルト設定値のテスト ---

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := DefaultRateLimiterConfig()

	if cfg.GeneralRate != 2.0 { // 120/60 = 2
		t.Errorf("GeneralRate = %f, want 2.0", cfg.GeneralRate)
	}
	if cfg.GeneralBurst != 120 {
		t.Errorf("GeneralBurst = %d, want 120", cfg.GeneralBurst)
	}
	if cfg.FeedRegRate == 0 {
		t.Error("FeedRegRate should not be 0")
	}
	if cfg.FeedRegBurst != 10 {
		t.Errorf("FeedRegBurst = %d, want 10", cfg.FeedRegBurst)
	}
}
