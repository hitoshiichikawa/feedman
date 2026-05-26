package middleware

import (
	"log/slog"
	"net"
	"net/http"
)

// NewTrustedCIDRMiddleware は信頼 CIDR 範囲内の送信元のみ通過させるミドルウェアを返す。
//
// 起動時に cidrs を net.ParseCIDR でパースして []*net.IPNet を構築する。不正な CIDR 文字列は
// スキップして Warn ログを出力し、有効分のみを判定に採用する。
//
// リクエスト時は r.RemoteAddr（host:port 形式）から host を抽出して net.ParseIP し、いずれかの
// 信頼 CIDR に含まれれば next を呼び、含まれなければ 403 Forbidden を返す。
//
// 信頼 CIDR が空（未設定）の場合は全リクエストを 403 で拒否する（安全側 / NFR 2.1）。
// X-Forwarded-For は信頼せず、判定には r.RemoteAddr のみを用いる（Requirement 4.3）。
// 拒否時は next を呼ばず http.Error で即終了し、メトリクス本文を一切応答に含めない（NFR 2.2）。
func NewTrustedCIDRMiddleware(cidrs []string) func(next http.Handler) http.Handler {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			slog.Warn("信頼 CIDR のパースに失敗したためスキップします",
				slog.String("cidr", c),
				slog.String("error", err.Error()),
			)
			continue
		}
		nets = append(nets, ipNet)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isTrustedRemoteAddr(r.RemoteAddr, nets) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isTrustedRemoteAddr は remoteAddr（host:port 形式）の IP が nets のいずれかに含まれるかを判定する。
// パース不能な remoteAddr / IP、および nets が空の場合は false（拒否）を返す（安全側）。
//
// IP の抽出は clientIPFromRemoteAddr に委譲する（X-Forwarded-For を信頼しない方針を共有するため）。
func isTrustedRemoteAddr(remoteAddr string, nets []*net.IPNet) bool {
	if len(nets) == 0 {
		return false
	}
	ipStr := clientIPFromRemoteAddr(remoteAddr)
	if ipStr == "" {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
