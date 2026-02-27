package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestFaviconFetcher_ImplementsInterface はFaviconFetcherがインターフェースを満たすことを検証する。
func TestFaviconFetcher_ImplementsInterface(t *testing.T) {
	var _ FaviconFetcherService = (*FaviconFetcher)(nil)
}

// TestNewFaviconFetcher はFaviconFetcherが正しく初期化されることを検証する。
func TestNewFaviconFetcher_Initializes(t *testing.T) {
	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)
	if fetcher == nil {
		t.Fatal("expected non-nil fetcher")
	}
}

// TestFaviconFetcher_FetchFavicon_Success はfavicon取得成功時にデータとMIMEタイプを返すことをテストする。
func TestFaviconFetcher_FetchFavicon_Success(t *testing.T) {
	// PNG画像のヘッダー（最小限のテストデータ）
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)

	data, mimeType, err := fetcher.FetchFavicon(context.Background(), server.URL+"/favicon.ico")
	if err != nil {
		t.Fatalf("FetchFavicon returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty favicon data")
	}
	if mimeType != "image/png" {
		t.Errorf("expected MIME type 'image/png', got %q", mimeType)
	}
}

// TestFaviconFetcher_FetchFavicon_404 はfavicon取得が404の場合にnilデータを返すことをテストする。
func TestFaviconFetcher_FetchFavicon_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)

	data, mimeType, err := fetcher.FetchFavicon(context.Background(), server.URL+"/favicon.ico")
	// 取得失敗時はエラーではなくnilデータを返す（要件: 取得失敗時はnullとして保存）
	if err != nil {
		t.Fatalf("FetchFavicon should not return error on 404, got: %v", err)
	}
	if data != nil {
		t.Error("expected nil favicon data on 404")
	}
	if mimeType != "" {
		t.Errorf("expected empty MIME type on 404, got %q", mimeType)
	}
}

// TestFaviconFetcher_FetchFavicon_EmptyURL は空URLの場合にnilデータを返すことをテストする。
func TestFaviconFetcher_FetchFavicon_EmptyURL(t *testing.T) {
	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)

	data, mimeType, err := fetcher.FetchFavicon(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchFavicon should not return error on empty URL, got: %v", err)
	}
	if data != nil {
		t.Error("expected nil favicon data on empty URL")
	}
	if mimeType != "" {
		t.Errorf("expected empty MIME type on empty URL, got %q", mimeType)
	}
}

// TestFaviconFetcher_FetchFaviconForSite はサイトURLからfaviconを取得することをテストする。
func TestFaviconFetcher_FetchFaviconForSite_FromFaviconICO(t *testing.T) {
	icoData := []byte{0x00, 0x00, 0x01, 0x00}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write(icoData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)

	data, mimeType, err := fetcher.FetchFaviconForSite(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchFaviconForSite returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty favicon data")
	}
	if mimeType != "image/x-icon" {
		t.Errorf("expected MIME type 'image/x-icon', got %q", mimeType)
	}
}

// TestFaviconFetcher_FetchFaviconForSite_Failure はサイトURLからfavicon取得に失敗した場合にnilを返すテスト。
func TestFaviconFetcher_FetchFaviconForSite_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)

	data, mimeType, err := fetcher.FetchFaviconForSite(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchFaviconForSite should not return error, got: %v", err)
	}
	if data != nil {
		t.Error("expected nil favicon data")
	}
	if mimeType != "" {
		t.Errorf("expected empty MIME type, got %q", mimeType)
	}
}

// TestFaviconFetcher_FetchFavicon_SSRFBlocked はSSRFガードがブロックした場合にnilデータを返すテスト。
func TestFaviconFetcher_FetchFavicon_SSRFBlocked(t *testing.T) {
	guard := &mockSSRFGuard{blockAll: true}
	fetcher := NewFaviconFetcher(guard)

	data, mimeType, err := fetcher.FetchFavicon(context.Background(), "http://192.168.1.1/favicon.ico")
	// SSRF検証失敗時もエラーではなくnilデータを返す
	if err != nil {
		t.Fatalf("FetchFavicon should not return error on SSRF block, got: %v", err)
	}
	if data != nil {
		t.Error("expected nil favicon data on SSRF block")
	}
	if mimeType != "" {
		t.Errorf("expected empty MIME type on SSRF block, got %q", mimeType)
	}
}

// TestFaviconFetcher_FetchFavicon_LargeResponse はレスポンスが大きすぎる場合にnilデータを返すテスト。
func TestFaviconFetcher_FetchFavicon_LargeResponse(t *testing.T) {
	// 2MBを超えるデータ（faviconの最大サイズ制限）
	largeData := make([]byte, 2*1024*1024+1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(largeData)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	fetcher := NewFaviconFetcher(guard)

	data, _, err := fetcher.FetchFavicon(context.Background(), server.URL+"/favicon.ico")
	if err != nil {
		t.Fatalf("FetchFavicon should not return error on large response, got: %v", err)
	}
	if data != nil {
		t.Error("expected nil favicon data for large response")
	}
}

// mockSSRFGuardForFavicon は既にdetector_test.goで定義されているmockSSRFGuardを使用する。
// ここではdetector_test.goのモックが同パッケージ内で利用可能。

// --- 以下はdetector_test.goに定義済みのモックを利用 ---
// mockSSRFGuard は detector_test.go に定義済み。

// testHelperHTTPClient はテスト用のHTTPクライアントを返す。
func testHelperHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// TestGuessDefaultFaviconURL はサイトURLからデフォルトのfavicon URLを推測する関数のテスト。
func TestGuessDefaultFaviconURL(t *testing.T) {
	tests := []struct {
		siteURL  string
		expected string
	}{
		{"https://example.com", "https://example.com/favicon.ico"},
		{"https://example.com/", "https://example.com/favicon.ico"},
		{"https://example.com/blog", "https://example.com/favicon.ico"},
		{"https://example.com:8080", "https://example.com:8080/favicon.ico"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("siteURL=%s", tt.siteURL), func(t *testing.T) {
			result := guessDefaultFaviconURL(tt.siteURL)
			if result != tt.expected {
				t.Errorf("guessDefaultFaviconURL(%q) = %q, want %q", tt.siteURL, result, tt.expected)
			}
		})
	}
}
