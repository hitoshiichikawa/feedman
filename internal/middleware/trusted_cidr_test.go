package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// metricsBodyMarker は拒否時にメトリクス本文が漏れていないことを確認するための next 側出力。
const metricsBodyMarker = "feedman_fetch_success_total 1"

func TestTrustedCIDRMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		cidrs      []string
		remoteAddr string
		wantStatus int
		wantNext   bool
		wantNoBody bool // true の場合、レスポンスボディに metricsBodyMarker が含まれないことを検証
	}{
		{
			name:       "範囲内の送信元のとき200でnextに到達する",
			cidrs:      []string{"10.0.0.0/8"},
			remoteAddr: "10.1.2.3:54321",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "範囲外の送信元のとき403で拒否する",
			cidrs:      []string{"10.0.0.0/8"},
			remoteAddr: "192.168.1.1:54321",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
			wantNoBody: true,
		},
		{
			name:       "信頼CIDRが空(未設定)のとき全リクエストを403で拒否する",
			cidrs:      nil,
			remoteAddr: "10.1.2.3:54321",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
			wantNoBody: true,
		},
		{
			name:       "不正なCIDRをスキップし有効分のみで判定する(有効範囲内は200)",
			cidrs:      []string{"not-a-cidr", "10.0.0.0/8"},
			remoteAddr: "10.5.6.7:1234",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "不正なCIDRをスキップし有効分のみで判定する(有効範囲外は403)",
			cidrs:      []string{"not-a-cidr", "10.0.0.0/8"},
			remoteAddr: "172.16.0.1:1234",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
			wantNoBody: true,
		},
		{
			name:       "全CIDRが不正のとき有効分ゼロで全拒否する",
			cidrs:      []string{"bad", "also/bad"},
			remoteAddr: "10.1.2.3:54321",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
			wantNoBody: true,
		},
		{
			name:       "パース不能なRemoteAddrのとき403で拒否する",
			cidrs:      []string{"10.0.0.0/8"},
			remoteAddr: "not-an-address",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
			wantNoBody: true,
		},
		{
			name:       "IPv6範囲内の送信元のとき200でnextに到達する",
			cidrs:      []string{"::1/128"},
			remoteAddr: "[::1]:54321",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mw := NewTrustedCIDRMiddleware(tt.cidrs)
			nextCalled := false
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
				// next が呼ばれた場合のみメトリクス本文を出力する。
				_, _ = w.Write([]byte(metricsBodyMarker))
			}))

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			req.RemoteAddr = tt.remoteAddr
			w := httptest.NewRecorder()

			// Act
			handler.ServeHTTP(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if nextCalled != tt.wantNext {
				t.Errorf("nextCalled = %v, want %v", nextCalled, tt.wantNext)
			}
			if tt.wantNoBody {
				if strings.Contains(w.Body.String(), metricsBodyMarker) {
					t.Errorf("拒否時のレスポンスボディにメトリクス本文が含まれている: %q", w.Body.String())
				}
			}
		})
	}
}
