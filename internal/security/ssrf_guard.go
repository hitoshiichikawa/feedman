// Package security はアプリケーションのセキュリティ機能を提供する。
package security

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/doyensec/safeurl"
)

// SSRFGuardService はSSRF防止機能のインターフェースを定義する。
// フィード登録時とフェッチ時の両方で使用される。
type SSRFGuardService interface {
	// NewSafeClient はSSRF防止機能付きのHTTPクライアントを生成する。
	// safeurlライブラリにより、プライベートIP、ループバック、リンクローカル、
	// メタデータIPへのリクエストが自動的にブロックされる。
	// DNS再バインディング攻撃への対策も有効化される。
	NewSafeClient(timeout time.Duration, maxResponseSize int64) *http.Client

	// ValidateURL はURLの安全性を事前に検証する。
	// スキーム、ホスト、IPアドレスの検証を行い、
	// 危険なURLの場合はエラーを返す。
	ValidateURL(rawURL string) error
}

// allowedSchemes はSSRF防止で許可されるURLスキーム。
var allowedSchemes = []string{"http", "https"}

// blockedNetworks はSSRF防止でブロックされるネットワーク範囲。
// パッケージ初期化時に1回だけパースし、ValidateURLでの検証に使用する。
// safeurlはnet.DialerレベルでDNS解決後のIPアドレスも検証するため、
// DNS再バインディング攻撃にも対応している。
var blockedNetworks []net.IPNet

func init() {
	cidrs := []string{
		// プライベートIPアドレス (RFC 1918)
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		// ループバック (RFC 1122)
		"127.0.0.0/8",
		// リンクローカル (RFC 3927) - クラウドメタデータIP (169.254.169.254) を含む
		"169.254.0.0/16",
		// カレントネットワーク
		"0.0.0.0/8",
		// IPv6ループバック
		"::1/128",
		// IPv6リンクローカル
		"fe80::/10",
		// IPv6ユニークローカル
		"fc00::/7",
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR in blockedNetworks: %s: %v", cidr, err))
		}
		blockedNetworks = append(blockedNetworks, *network)
	}
}

// ssrfGuard はSSRFGuardServiceの実装。
type ssrfGuard struct{}

// NewSSRFGuard はSSRFGuardServiceの新しいインスタンスを生成する。
func NewSSRFGuard() *ssrfGuard {
	return &ssrfGuard{}
}

// NewSafeClient はSSRF防止機能付きのHTTPクライアントを生成する。
// safeurlのデフォルト設定により以下がブロックされる:
//   - プライベートIPアドレス (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
//   - ループバックアドレス (127.0.0.0/8, ::1)
//   - リンクローカルアドレス (169.254.0.0/16, fe80::/10)
//   - メタデータIPアドレス (169.254.169.254)
//
// safeurlはnet.DialerのControlフックでDNS解決後のIPアドレスを検証するため、
// DNS再バインディング攻撃にも対応している。
func (g *ssrfGuard) NewSafeClient(timeout time.Duration, maxResponseSize int64) *http.Client {
	config := safeurl.GetConfigBuilder().
		SetTimeout(timeout).
		SetAllowedSchemes(allowedSchemes...).
		SetAllowedPorts(80, 443).
		Build()

	wrappedClient := safeurl.Client(config)
	return wrappedClient.Client
}

// ValidateURL はURLの安全性を事前に検証する。
// DNS解決を伴わない静的な検証を行う。
// フィード登録時にHTTPリクエストを送信する前の事前チェックとして使用する。
// 注意: この検証はDNS解決前の静的チェックであるため、DNS再バインディング攻撃は
// NewSafeClientが生成するHTTPクライアント側のDialer検証で防止される。
func (g *ssrfGuard) ValidateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("empty URL")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// スキーム検証: http/httpsのみ許可
	scheme := strings.ToLower(parsed.Scheme)
	if !isAllowedScheme(scheme) {
		return fmt.Errorf("disallowed scheme: %s (allowed: %v)", scheme, allowedSchemes)
	}

	// ホスト検証: 空ホストを拒否
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("empty host in URL: %s", rawURL)
	}

	// IPアドレスの場合: ブロック対象CIDRとの照合
	ip := net.ParseIP(host)
	if ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked IP address: %s", ip.String())
		}
		return nil
	}

	// ホスト名の場合: localhost等の危険なホスト名を拒否
	if isBlockedHostname(host) {
		return fmt.Errorf("blocked host: %s", host)
	}

	return nil
}

// isAllowedScheme はURLスキームが許可リストに含まれるかを検証する。
func isAllowedScheme(scheme string) bool {
	for _, allowed := range allowedSchemes {
		if strings.EqualFold(scheme, allowed) {
			return true
		}
	}
	return false
}

// isBlockedIP はIPアドレスがブロック対象のネットワーク範囲に含まれるかを検証する。
func isBlockedIP(ip net.IP) bool {
	for _, network := range blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// blockedHostnames はブロック対象のホスト名。
var blockedHostnames = []string{
	"localhost",
}

// isBlockedHostname はホスト名がブロック対象かを検証する。
func isBlockedHostname(host string) bool {
	lower := strings.ToLower(host)
	for _, blocked := range blockedHostnames {
		if lower == blocked {
			return true
		}
	}
	return false
}
