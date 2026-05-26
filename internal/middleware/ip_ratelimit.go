package middleware

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// unknownIPKey は接続元アドレスからクライアント IP を判定できなかったリクエストに
// 割り当てる固定キー。IP 判定不能なリクエストを無制限に通過させず、この単一バケットで
// まとめてレート制限することで安全側に倒す（Requirement 3.3）。
const unknownIPKey = "__unknown_ip__"

// IPRateLimiterConfig は IP 単位レート制限の設定を保持する。
type IPRateLimiterConfig struct {
	Rate            rate.Limit    // IP 単位のレート（req/sec）
	Burst           int           // IP 単位のバーストサイズ
	CleanupInterval time.Duration // 期限切れエントリのクリーンアップ間隔
}

// DefaultIPRateLimiterConfig は requestsPerMin（req/min/IP）から IP 単位レート制限の
// デフォルト設定を構築する。
//
// requestsPerMin が 0 以下の場合は最低 1 req/min にフォールバックする
// （rate.Limit が 0 や負だと全リクエストが恒常的に拒否されるのを防ぐ安全側）。
func DefaultIPRateLimiterConfig(requestsPerMin int) IPRateLimiterConfig {
	if requestsPerMin < 1 {
		requestsPerMin = 1
	}
	return IPRateLimiterConfig{
		Rate:            rate.Limit(float64(requestsPerMin) / 60.0),
		Burst:           requestsPerMin,
		CleanupInterval: 5 * time.Minute,
	}
}

// IPRateLimiter はクライアント IP ごとのレート制限を管理する。
//
// 未認証エンドポイント（/auth/google/login・/auth/google/callback・/health）に適用し、
// セッションを持たないリクエストを接続元 IP 単位で制限する。userID 単位の RateLimiter
// とは独立した型として実装し、既存の userID ベース挙動には一切影響しない（Requirement 4）。
type IPRateLimiter struct {
	config IPRateLimiterConfig

	mu       sync.RWMutex
	limiters map[string]*userLimiter

	stopCh chan struct{}
}

// NewIPRateLimiter は新しい IPRateLimiter を生成する。
// バックグラウンドで期限切れエントリのクリーンアップを開始する。
func NewIPRateLimiter(config IPRateLimiterConfig) *IPRateLimiter {
	rl := &IPRateLimiter{
		config:   config,
		limiters: make(map[string]*userLimiter),
		stopCh:   make(chan struct{}),
	}

	go rl.cleanupLoop()

	return rl
}

// Stop はクリーンアップのバックグラウンドゴルーチンを停止する。
func (rl *IPRateLimiter) Stop() {
	close(rl.stopCh)
}

// Middleware は IP 単位レート制限ミドルウェアを返す。
//
// クライアント IP は接続元アドレス（r.RemoteAddr）から判定し、X-Forwarded-For
// ヘッダーは信頼しない（Requirement 3.1, 3.2）。IP を判定できない場合は固定キーで
// まとめて制限し、無制限通過を許さない（Requirement 3.3）。
//
// 閾値超過時は 429 を返し、Retry-After ヘッダーで再試行可能になるまでの待機時間を
// 通知する（Requirement 1.6）。拒否ログにはセッション情報やトークンを含めず、
// レート種別のみを記録する（NFR 1.1, 1.2）。
func (rl *IPRateLimiter) Middleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientIPFromRemoteAddr(r.RemoteAddr)
			if key == "" {
				// IP 判定不能なリクエストは安全側で扱い、固定キーでまとめて制限する。
				key = unknownIPKey
			}

			limiter := rl.getOrCreateLimiter(key)

			if !limiter.Allow() {
				writeRateLimitResponse(w, rl.config.Rate)
				slog.Warn("rate limit exceeded",
					slog.String("limit_type", "unauth_ip"),
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LimiterCount は現在管理されている IP リミッターのエントリ数を返す。
// テストおよびメトリクス用。
func (rl *IPRateLimiter) LimiterCount() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.limiters)
}

// getOrCreateLimiter は IP（または固定キー）のリミッターを取得または作成する。
func (rl *IPRateLimiter) getOrCreateLimiter(key string) *rate.Limiter {
	rl.mu.RLock()
	ul, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if exists {
		rl.mu.Lock()
		ul.lastAccess = time.Now()
		rl.mu.Unlock()
		return ul.limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// ダブルチェック
	if ul, exists := rl.limiters[key]; exists {
		ul.lastAccess = time.Now()
		return ul.limiter
	}

	limiter := rate.NewLimiter(rl.config.Rate, rl.config.Burst)
	rl.limiters[key] = &userLimiter{
		limiter:    limiter,
		lastAccess: time.Now(),
	}

	return limiter
}

// cleanupLoop はバックグラウンドで期限切れエントリを定期的にクリーンアップする。
func (rl *IPRateLimiter) cleanupLoop() {
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

// cleanup は最終アクセス時刻が CleanupInterval の 2 倍を超えたエントリを削除する。
// 一定期間アクセスのない IP の内部状態を解放し、無制限なメモリ増加を防ぐ（NFR 3.1）。
func (rl *IPRateLimiter) cleanup() {
	ttl := rl.config.CleanupInterval * 2
	now := time.Now()

	rl.mu.Lock()
	for key, ul := range rl.limiters {
		if now.Sub(ul.lastAccess) > ttl {
			delete(rl.limiters, key)
		}
	}
	rl.mu.Unlock()
}
