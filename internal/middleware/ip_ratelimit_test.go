package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// testIPRateLimiterConfig はテスト用の IPRateLimiterConfig を返す（クリーンアップは長めに設定）。
func testIPRateLimiterConfig(r rate.Limit, burst int) IPRateLimiterConfig {
	return IPRateLimiterConfig{
		Rate:            r,
		Burst:           burst,
		CleanupInterval: 1 * time.Minute,
	}
}

// newIPHandler は IPRateLimiter.Middleware を適用したハンドラと呼び出し回数カウンタを返す。
func newIPHandler(rl *IPRateLimiter) (http.Handler, *int) {
	count := 0
	h := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
	}))
	return h, &count
}

// doIPRequest は指定 path・remoteAddr でリクエストを送信し、ステータスコードを返す。
func doIPRequest(h http.Handler, path, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// Req 1.1, 1.2, 1.3: 各未認証エンドポイントで同一 IP の閾値超過時に 429 を返す。
func TestIPRateLimiter_Returns429WhenLimitExceeded_PerEndpoint(t *testing.T) {
	paths := []struct {
		name string
		path string
	}{
		{"auth_google_login", "/auth/google/login"},
		{"auth_google_callback", "/auth/google/callback"},
		{"health", "/health"},
	}

	for _, p := range paths {
		t.Run(p.name+"で閾値超過のとき429を返す", func(t *testing.T) {
			// Arrange: burst 1 のリミッターで 2 回目に必ず超過させる。
			rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(1), 1))
			defer rl.Stop()
			h, _ := newIPHandler(rl)
			const addr = "203.0.113.10:50000"

			// Act: 1 回目は通過、2 回目は超過。
			w1 := doIPRequest(h, p.path, addr)
			w2 := doIPRequest(h, p.path, addr)

			// Assert
			if w1.Result().StatusCode != http.StatusOK {
				t.Errorf("1回目: status = %d, want %d", w1.Result().StatusCode, http.StatusOK)
			}
			if w2.Result().StatusCode != http.StatusTooManyRequests {
				t.Errorf("2回目: status = %d, want %d", w2.Result().StatusCode, http.StatusTooManyRequests)
			}
		})
	}
}

// Req 1.4: 閾値以内のリクエストは後続ハンドラへ通過させる。
func TestIPRateLimiter_AllowsRequestsWithinLimit(t *testing.T) {
	// Arrange: burst 5。
	rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(2), 5))
	defer rl.Stop()
	h, count := newIPHandler(rl)
	const addr = "203.0.113.20:50000"

	// Act: burst 内の 5 リクエスト。
	for i := 0; i < 5; i++ {
		w := doIPRequest(h, "/health", addr)
		// Assert
		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i, w.Result().StatusCode, http.StatusOK)
		}
	}
	if *count != 5 {
		t.Errorf("handler call count = %d, want 5", *count)
	}
}

// Req 1.5: 異なる IP は独立にカウントされる。
func TestIPRateLimiter_IsolatesRateLimitsPerIP(t *testing.T) {
	// Arrange: burst 1。
	rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(1), 1))
	defer rl.Stop()
	h, _ := newIPHandler(rl)

	// Act: IP-A の 1 回目は通過、2 回目は超過。IP-B の 1 回目は IP-A に影響されず通過。
	wA1 := doIPRequest(h, "/health", "198.51.100.1:40000")
	wA2 := doIPRequest(h, "/health", "198.51.100.1:40000")
	wB1 := doIPRequest(h, "/health", "198.51.100.2:40000")

	// Assert
	if wA1.Result().StatusCode != http.StatusOK {
		t.Errorf("IP-A 1回目: status = %d, want %d", wA1.Result().StatusCode, http.StatusOK)
	}
	if wA2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("IP-A 2回目: status = %d, want %d", wA2.Result().StatusCode, http.StatusTooManyRequests)
	}
	if wB1.Result().StatusCode != http.StatusOK {
		t.Errorf("IP-B 1回目: status = %d, want %d", wB1.Result().StatusCode, http.StatusOK)
	}
}

// Req 1.6: 429 応答に Retry-After ヘッダーを付与する。
func TestIPRateLimiter_Returns429WithRetryAfterHeader(t *testing.T) {
	// Arrange: burst 1。
	rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(1), 1))
	defer rl.Stop()
	h, _ := newIPHandler(rl)
	const addr = "203.0.113.30:50000"

	// Act: 2 回目で超過させる。
	doIPRequest(h, "/health", addr)
	w := doIPRequest(h, "/health", addr)

	// Assert
	if w.Result().StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", w.Result().StatusCode, http.StatusTooManyRequests)
	}
	retryAfter := w.Result().Header.Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("Retry-After ヘッダーが存在すべき")
	}
	sec, err := strconv.Atoi(retryAfter)
	if err != nil {
		t.Errorf("Retry-After は数値であるべき, got %q", retryAfter)
	}
	if sec < 1 {
		t.Errorf("Retry-After = %d, 少なくとも 1 であるべき", sec)
	}
}

// Req 3.1, 3.2: クライアント IP は接続元アドレスから判定し、X-Forwarded-For を信頼しない。
func TestIPRateLimiter_IgnoresXForwardedFor(t *testing.T) {
	// Arrange: burst 1。同一 RemoteAddr から、XFF を毎回変えて偽装する。
	rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(1), 1))
	defer rl.Stop()
	h, _ := newIPHandler(rl)
	const addr = "203.0.113.40:50000"

	makeReq := func(xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = addr
		req.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	// Act: XFF を別 IP に偽装しても、RemoteAddr が同一なので 2 回目は超過する。
	w1 := makeReq("10.0.0.1")
	w2 := makeReq("10.0.0.2")

	// Assert
	if w1.Result().StatusCode != http.StatusOK {
		t.Errorf("1回目: status = %d, want %d", w1.Result().StatusCode, http.StatusOK)
	}
	if w2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("2回目（XFF偽装）: status = %d, want %d（RemoteAddrベースで制限されるべき）",
			w2.Result().StatusCode, http.StatusTooManyRequests)
	}
}

// Req 3.3: 接続元アドレスから IP を判定できない場合、無制限通過を許さない。
func TestIPRateLimiter_IndeterminateIP_NotUnlimited(t *testing.T) {
	// Arrange: burst 1。RemoteAddr が IP として解釈不能。
	rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(1), 1))
	defer rl.Stop()
	h, _ := newIPHandler(rl)
	const badAddr = "not-an-address"

	// Act: 同一の判定不能アドレスから 2 回。固定キーでまとめて制限される。
	w1 := doIPRequest(h, "/health", badAddr)
	w2 := doIPRequest(h, "/health", badAddr)

	// Assert
	if w1.Result().StatusCode != http.StatusOK {
		t.Errorf("1回目: status = %d, want %d", w1.Result().StatusCode, http.StatusOK)
	}
	if w2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("2回目（判定不能IP）: status = %d, want %d（無制限通過を許さない）",
			w2.Result().StatusCode, http.StatusTooManyRequests)
	}
}

// 429 レスポンスが JSON 形式であること（既存 RateLimiter と同一フォーマットを再利用）。
func TestIPRateLimiter_429ResponseIsJSON(t *testing.T) {
	// Arrange
	rl := NewIPRateLimiter(testIPRateLimiterConfig(rate.Limit(1), 1))
	defer rl.Stop()
	h, _ := newIPHandler(rl)
	const addr = "203.0.113.50:50000"

	// Act
	doIPRequest(h, "/health", addr)
	w := doIPRequest(h, "/health", addr)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("レスポンスのデコードに失敗: %v", err)
	}
	if body["code"] == "" {
		t.Error("'code' フィールドが存在すべき")
	}
}

// NFR 3.1: 一定期間アクセスのない IP の内部状態が解放される。
func TestIPRateLimiter_CleanupRemovesExpiredEntries(t *testing.T) {
	// Arrange: クリーンアップ間隔を短くする（TTL = 2 倍 = 100ms）。
	cfg := IPRateLimiterConfig{
		Rate:            rate.Limit(2),
		Burst:           5,
		CleanupInterval: 50 * time.Millisecond,
	}
	rl := NewIPRateLimiter(cfg)
	defer rl.Stop()
	h, _ := newIPHandler(rl)

	// Act: エントリを作成。
	doIPRequest(h, "/health", "203.0.113.60:50000")
	if rl.LimiterCount() == 0 {
		t.Fatal("少なくとも 1 件のリミッターエントリが存在すべき")
	}

	// クリーンアップが実行されるのを待つ（TTL 100ms に対し 200ms 待機）。
	time.Sleep(200 * time.Millisecond)

	// Assert
	if count := rl.LimiterCount(); count != 0 {
		t.Errorf("クリーンアップ後のエントリ数 = %d, want 0", count)
	}
}

// DefaultIPRateLimiterConfig: 30 req/min から rate/burst が構築されること、不正値フォールバック。
func TestDefaultIPRateLimiterConfig(t *testing.T) {
	cases := []struct {
		name      string
		reqPerMin int
		wantBurst int
		wantRate  rate.Limit
	}{
		{"30req/minのときrate0.5burst30", 30, 30, rate.Limit(30.0 / 60.0)},
		{"0以下のとき最低1にフォールバック", 0, 1, rate.Limit(1.0 / 60.0)},
		{"負値のとき最低1にフォールバック", -5, 1, rate.Limit(1.0 / 60.0)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cfg := DefaultIPRateLimiterConfig(tt.reqPerMin)
			// Assert
			if cfg.Burst != tt.wantBurst {
				t.Errorf("Burst = %d, want %d", cfg.Burst, tt.wantBurst)
			}
			if cfg.Rate != tt.wantRate {
				t.Errorf("Rate = %v, want %v", cfg.Rate, tt.wantRate)
			}
			if cfg.CleanupInterval <= 0 {
				t.Errorf("CleanupInterval = %v, should be positive", cfg.CleanupInterval)
			}
		})
	}
}
