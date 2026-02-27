package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewSSRFGuard はSSRFGuardの生成をテストする。
func TestNewSSRFGuard(t *testing.T) {
	guard := NewSSRFGuard()
	if guard == nil {
		t.Fatal("NewSSRFGuard() returned nil")
	}
}

// TestNewSafeClient はSSRF防止付きHTTPクライアントの生成をテストする。
func TestNewSafeClient(t *testing.T) {
	guard := NewSSRFGuard()
	client := guard.NewSafeClient(10*time.Second, 5*1024*1024)
	if client == nil {
		t.Fatal("NewSafeClient() returned nil")
	}
}

// TestNewSafeClientTimeout はタイムアウト設定が反映されることをテストする。
func TestNewSafeClientTimeout(t *testing.T) {
	guard := NewSSRFGuard()
	timeout := 5 * time.Second
	client := guard.NewSafeClient(timeout, 5*1024*1024)
	if client.Timeout != timeout {
		t.Errorf("expected timeout %v, got %v", timeout, client.Timeout)
	}
}

// TestNewSafeClientHasTransport はSafeClientにカスタムTransportが設定されていることをテストする。
// safeurlはnet.DialerのControlフックでIPアドレス検証を行うため、
// Transportが標準のhttp.DefaultTransportではないことを確認する。
func TestNewSafeClientHasTransport(t *testing.T) {
	guard := NewSSRFGuard()
	client := guard.NewSafeClient(5*time.Second, 5*1024*1024)

	if client.Transport == nil {
		t.Fatal("expected custom Transport to be set, got nil")
	}
	if client.Transport == http.DefaultTransport {
		t.Fatal("expected custom Transport, got http.DefaultTransport")
	}
}

// TestNewSafeClientBlocksLoopback はSafeClientがループバックへのリクエストをブロックすることをテストする。
// httptestサーバーは127.0.0.1で起動されるため、safeurlがブロックする。
func TestNewSafeClientBlocksLoopback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	guard := NewSSRFGuard()
	client := guard.NewSafeClient(5*time.Second, 5*1024*1024)

	_, err := client.Get(ts.URL)
	if err == nil {
		t.Fatal("expected error for loopback address request, got nil")
	}
}

// TestValidateURL_PublicURL は公開URLの検証が成功することをテストする。
func TestValidateURL_PublicURL(t *testing.T) {
	guard := NewSSRFGuard()

	publicURLs := []string{
		"https://example.com",
		"https://feeds.example.com/rss.xml",
		"http://blog.example.org/feed",
	}

	for _, u := range publicURLs {
		t.Run(u, func(t *testing.T) {
			err := guard.ValidateURL(u)
			if err != nil {
				t.Errorf("ValidateURL(%q) returned error: %v", u, err)
			}
		})
	}
}

// TestValidateURL_PrivateIP はプライベートIPアドレスの拒否をテストする。
func TestValidateURL_PrivateIP(t *testing.T) {
	guard := NewSSRFGuard()

	privateURLs := []string{
		"http://10.0.0.1/feed",
		"http://10.255.255.255/feed",
		"http://172.16.0.1/feed",
		"http://172.31.255.255/feed",
		"http://192.168.0.1/feed",
		"http://192.168.1.100/feed",
	}

	for _, u := range privateURLs {
		t.Run(u, func(t *testing.T) {
			err := guard.ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should have returned error for private IP", u)
			}
		})
	}
}

// TestValidateURL_LoopbackAddress はループバックアドレスの拒否をテストする。
func TestValidateURL_LoopbackAddress(t *testing.T) {
	guard := NewSSRFGuard()

	loopbackURLs := []string{
		"http://127.0.0.1/feed",
		"http://127.0.0.2/feed",
		"http://localhost/feed",
	}

	for _, u := range loopbackURLs {
		t.Run(u, func(t *testing.T) {
			err := guard.ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should have returned error for loopback address", u)
			}
		})
	}
}

// TestValidateURL_LinkLocalAddress はリンクローカルアドレスの拒否をテストする。
func TestValidateURL_LinkLocalAddress(t *testing.T) {
	guard := NewSSRFGuard()

	linkLocalURLs := []string{
		"http://169.254.0.1/feed",
		"http://169.254.169.254/latest/meta-data/", // AWS metadata
	}

	for _, u := range linkLocalURLs {
		t.Run(u, func(t *testing.T) {
			err := guard.ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should have returned error for link-local address", u)
			}
		})
	}
}

// TestValidateURL_MetadataIP はクラウドメタデータIPアドレスの拒否をテストする。
func TestValidateURL_MetadataIP(t *testing.T) {
	guard := NewSSRFGuard()

	metadataURLs := []string{
		"http://169.254.169.254/latest/meta-data/",             // AWS
		"http://169.254.169.254/metadata/instance?api-version=2021-02-01", // Azure
		"http://169.254.169.254/computeMetadata/v1/",           // GCP
	}

	for _, u := range metadataURLs {
		t.Run(u, func(t *testing.T) {
			err := guard.ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should have returned error for metadata IP", u)
			}
		})
	}
}

// TestValidateURL_InvalidURL は無効なURLの検証が失敗することをテストする。
func TestValidateURL_InvalidURL(t *testing.T) {
	guard := NewSSRFGuard()

	invalidURLs := []string{
		"",
		"not-a-url",
		"ftp://example.com/feed",
		"file:///etc/passwd",
		"gopher://example.com",
	}

	for _, u := range invalidURLs {
		t.Run(u, func(t *testing.T) {
			err := guard.ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should have returned error for invalid URL", u)
			}
		})
	}
}

// TestValidateURL_IPv6Loopback はIPv6ループバックアドレスの拒否をテストする。
func TestValidateURL_IPv6Loopback(t *testing.T) {
	guard := NewSSRFGuard()

	err := guard.ValidateURL("http://[::1]/feed")
	if err == nil {
		t.Error("ValidateURL(\"http://[::1]/feed\") should have returned error for IPv6 loopback")
	}
}

// TestValidateURL_ZeroAddress は0.0.0.0の拒否をテストする。
func TestValidateURL_ZeroAddress(t *testing.T) {
	guard := NewSSRFGuard()

	err := guard.ValidateURL("http://0.0.0.0/feed")
	if err == nil {
		t.Error("ValidateURL(\"http://0.0.0.0/feed\") should have returned error for zero address")
	}
}

// TestSSRFGuardInterface はSSRFGuardがインターフェースを正しく実装していることをテストする。
func TestSSRFGuardInterface(t *testing.T) {
	var _ SSRFGuardService = NewSSRFGuard()
}
