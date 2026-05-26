package middleware

import "net"

// clientIPFromRemoteAddr は remoteAddr（通常 host:port 形式）からクライアント IP を抽出する。
//
// 判定には接続元アドレス（r.RemoteAddr）のみを用い、X-Forwarded-For 等の
// ヘッダーは信頼しない（なりすましヘッダーによる制限回避を防ぐため）。
//
// host:port 形式でない場合は remoteAddr 全体を IP として解釈する。
// IP として解釈できない場合は空文字を返す（呼び出し側で安全側に扱う）。
//
// 戻り値は net.ParseIP を経た正規化済みの文字列（IPv4/IPv6 表記）であり、
// レート制限のキーとして用いることで表記揺れによる重複カウントを避けられる。
func clientIPFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// port を含まない形式の可能性があるため、remoteAddr 全体を IP として試す。
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	return ip.String()
}
