package middleware

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiterConfig はレート制限の設定を保持する。
type RateLimiterConfig struct {
	GeneralRate     rate.Limit    // API全般のレート（req/sec）。120/60 = 2 req/sec
	GeneralBurst    int           // API全般のバーストサイズ
	FeedRegRate     rate.Limit    // フィード登録のレート（req/sec）。10/60
	FeedRegBurst    int           // フィード登録のバーストサイズ
	CleanupInterval time.Duration // 期限切れエントリのクリーンアップ間隔
}

// DefaultRateLimiterConfig はデフォルトのレート制限設定を返す。
// 要件: API全般 120 req/min/user、フィード登録 10 req/min/user
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		GeneralRate:     rate.Limit(120.0 / 60.0), // 2 req/sec
		GeneralBurst:    120,
		FeedRegRate:     rate.Limit(10.0 / 60.0), // ~0.167 req/sec
		FeedRegBurst:    10,
		CleanupInterval: 5 * time.Minute,
	}
}

// userLimiter はユーザーごとのレートリミッターとアクセス時刻を保持する。
type userLimiter struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// RateLimiter はユーザーごとのレート制限を管理する。
// API全般のレート制限とフィード登録のレート制限の2種類を提供する。
type RateLimiter struct {
	config RateLimiterConfig

	generalMu       sync.RWMutex
	generalLimiters map[string]*userLimiter

	feedRegMu       sync.RWMutex
	feedRegLimiters map[string]*userLimiter

	stopCh chan struct{}
}

// NewRateLimiter は新しいRateLimiterを生成する。
// バックグラウンドで期限切れエントリのクリーンアップを開始する。
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	rl := &RateLimiter{
		config:          config,
		generalLimiters: make(map[string]*userLimiter),
		feedRegLimiters: make(map[string]*userLimiter),
		stopCh:          make(chan struct{}),
	}

	go rl.cleanupLoop()

	return rl
}

// Stop はクリーンアップのバックグラウンドゴルーチンを停止する。
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// GeneralMiddleware はAPI全般のレート制限ミドルウェアを返す。
// リクエストコンテキストにユーザーIDが含まれている必要がある（SessionMiddlewareの後に配置）。
func (rl *RateLimiter) GeneralMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := UserIDFromContext(r.Context())
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			limiter := rl.getOrCreateGeneralLimiter(userID)

			if !limiter.Allow() {
				writeRateLimitResponse(w, rl.config.GeneralRate)
				slog.Warn("rate limit exceeded",
					slog.String("user_id", userID),
					slog.String("limit_type", "general"),
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// FeedRegistrationMiddleware はフィード登録専用のレート制限ミドルウェアを返す。
// API全般のレート制限とは独立に動作する。
func (rl *RateLimiter) FeedRegistrationMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := UserIDFromContext(r.Context())
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			limiter := rl.getOrCreateFeedRegLimiter(userID)

			if !limiter.Allow() {
				writeRateLimitResponse(w, rl.config.FeedRegRate)
				slog.Warn("rate limit exceeded",
					slog.String("user_id", userID),
					slog.String("limit_type", "feed_registration"),
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GeneralLimiterCount は現在管理されているAPI全般リミッターのエントリ数を返す。
// テストおよびメトリクス用。
func (rl *RateLimiter) GeneralLimiterCount() int {
	rl.generalMu.RLock()
	defer rl.generalMu.RUnlock()
	return len(rl.generalLimiters)
}

// FeedRegLimiterCount は現在管理されているフィード登録リミッターのエントリ数を返す。
// テストおよびメトリクス用。
func (rl *RateLimiter) FeedRegLimiterCount() int {
	rl.feedRegMu.RLock()
	defer rl.feedRegMu.RUnlock()
	return len(rl.feedRegLimiters)
}

// getOrCreateGeneralLimiter はユーザーのAPI全般リミッターを取得または作成する。
func (rl *RateLimiter) getOrCreateGeneralLimiter(userID string) *rate.Limiter {
	rl.generalMu.RLock()
	ul, exists := rl.generalLimiters[userID]
	rl.generalMu.RUnlock()

	if exists {
		rl.generalMu.Lock()
		ul.lastAccess = time.Now()
		rl.generalMu.Unlock()
		return ul.limiter
	}

	rl.generalMu.Lock()
	defer rl.generalMu.Unlock()

	// ダブルチェック
	if ul, exists := rl.generalLimiters[userID]; exists {
		ul.lastAccess = time.Now()
		return ul.limiter
	}

	limiter := rate.NewLimiter(rl.config.GeneralRate, rl.config.GeneralBurst)
	rl.generalLimiters[userID] = &userLimiter{
		limiter:    limiter,
		lastAccess: time.Now(),
	}

	return limiter
}

// getOrCreateFeedRegLimiter はユーザーのフィード登録リミッターを取得または作成する。
func (rl *RateLimiter) getOrCreateFeedRegLimiter(userID string) *rate.Limiter {
	rl.feedRegMu.RLock()
	ul, exists := rl.feedRegLimiters[userID]
	rl.feedRegMu.RUnlock()

	if exists {
		rl.feedRegMu.Lock()
		ul.lastAccess = time.Now()
		rl.feedRegMu.Unlock()
		return ul.limiter
	}

	rl.feedRegMu.Lock()
	defer rl.feedRegMu.Unlock()

	// ダブルチェック
	if ul, exists := rl.feedRegLimiters[userID]; exists {
		ul.lastAccess = time.Now()
		return ul.limiter
	}

	limiter := rate.NewLimiter(rl.config.FeedRegRate, rl.config.FeedRegBurst)
	rl.feedRegLimiters[userID] = &userLimiter{
		limiter:    limiter,
		lastAccess: time.Now(),
	}

	return limiter
}

// cleanupLoop はバックグラウンドで期限切れエントリを定期的にクリーンアップする。
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanup は最終アクセス時刻がCleanupIntervalの2倍を超えたエントリを削除する。
func (rl *RateLimiter) cleanup() {
	ttl := rl.config.CleanupInterval * 2

	now := time.Now()

	rl.generalMu.Lock()
	for userID, ul := range rl.generalLimiters {
		if now.Sub(ul.lastAccess) > ttl {
			delete(rl.generalLimiters, userID)
		}
	}
	rl.generalMu.Unlock()

	rl.feedRegMu.Lock()
	for userID, ul := range rl.feedRegLimiters {
		if now.Sub(ul.lastAccess) > ttl {
			delete(rl.feedRegLimiters, userID)
		}
	}
	rl.feedRegMu.Unlock()
}

// writeRateLimitResponse は429 Too Many Requestsレスポンスを書き込む。
// Retry-Afterヘッダーにはトークンが補充されるまでの推定秒数を設定する。
func writeRateLimitResponse(w http.ResponseWriter, r rate.Limit) {
	// Retry-Afterの算出: 1トークンが補充されるまでの秒数
	retryAfterSec := int(math.Ceil(1.0 / float64(r)))
	if retryAfterSec < 1 {
		retryAfterSec = 1
	}

	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	json.NewEncoder(w).Encode(map[string]string{
		"code":     "rate_limit_exceeded",
		"message":  "Too many requests. Please try again later.",
		"category": "system",
		"action":   "Please wait and retry after the specified time.",
	})
}
