package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

// --- HTTP クライアント再利用のテスト（Requirement 3 / 4 / 5 / 6） ---

// TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithGuard はSSRFガード有効時に
// 同一インスタンスでgetHTTPClientを複数回呼んでも同一クライアントを使い回し、
// NewSafeClientの追加生成が発生しないことをテストする（AC 3.1, 3.2, 5.2）。
func TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithGuard(t *testing.T) {
	// Arrange
	guard := &mockSSRFGuard{}
	f := NewFaviconFetcher(guard)

	// Act
	c1 := f.getHTTPClient()
	c2 := f.getHTTPClient()
	c3 := f.getHTTPClient()

	// Assert
	if c1 != c2 || c2 != c3 {
		t.Error("同一インスタンスのgetHTTPClientは同一のHTTPクライアントを返すべき")
	}
	if guard.newSafeClientCalls() > 1 {
		t.Errorf("NewSafeClientの呼び出しは1回までであるべき。結果: %d 回", guard.newSafeClientCalls())
	}
}

// TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithoutGuard はSSRFガード無効時(nil)に
// 同一インスタンスでgetHTTPClientを複数回呼んでも同一クライアントを使い回すことをテストする（AC 3.1, 3.2）。
func TestFaviconFetcher_GetHTTPClient_ReusesSameInstanceWithoutGuard(t *testing.T) {
	// Arrange
	f := NewFaviconFetcher(nil)

	// Act
	c1 := f.getHTTPClient()
	c2 := f.getHTTPClient()

	// Assert
	if c1 != c2 {
		t.Error("SSRFガード無効時も同一インスタンスのgetHTTPClientは同一クライアントを返すべき")
	}
	if c1 == nil {
		t.Fatal("getHTTPClientはnilを返すべきではない")
	}
}

// TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest は同一インスタンスから
// 複数回favicon取得しても新しいHTTPクライアントが追加生成されず、結果が一致することをテストする（AC 3.2, 4.1, 3）。
func TestFaviconFetcher_FetchFavicon_NoAdditionalClientPerRequest(t *testing.T) {
	// Arrange
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	f := NewFaviconFetcher(guard)

	// Act: 同一インスタンスから3回取得
	for i := 0; i < 3; i++ {
		data, mimeType, err := f.FetchFavicon(context.Background(), server.URL+"/favicon.ico")
		if err != nil {
			t.Fatalf("iteration %d: FetchFavicon returned error: %v", i, err)
		}
		// Assert: 各回で結果が一致（AC 4.1）
		if len(data) != len(pngData) {
			t.Errorf("iteration %d: 期待データ長 %d, 結果 %d", i, len(pngData), len(data))
		}
		if mimeType != "image/png" {
			t.Errorf("iteration %d: 期待MIME image/png, 結果 %q", i, mimeType)
		}
	}

	// Assert: クライアント追加生成なし（AC 3.2）
	if guard.newSafeClientCalls() > 1 {
		t.Errorf("複数回取得してもNewSafeClientは1回までであるべき。結果: %d 回", guard.newSafeClientCalls())
	}
}

// TestFaviconFetcher_FetchFavicon_FailureResultStable は同一インスタンスから
// 取得失敗(404)を複数回試行しても各回で同一の挙動(nil・空MIME・エラーなし)を返すことをテストする（AC 4.2）。
func TestFaviconFetcher_FetchFavicon_FailureResultStable(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	f := NewFaviconFetcher(guard)

	// Act & Assert: 2回連続で同一の失敗挙動
	for i := 0; i < 2; i++ {
		data, mimeType, err := f.FetchFavicon(context.Background(), server.URL+"/favicon.ico")
		if err != nil {
			t.Fatalf("iteration %d: 取得失敗時はエラーを返すべきでない: %v", i, err)
		}
		if data != nil {
			t.Errorf("iteration %d: 取得失敗時はnilデータであるべき", i)
		}
		if mimeType != "" {
			t.Errorf("iteration %d: 取得失敗時は空MIMEであるべき。結果: %q", i, mimeType)
		}
	}
}

// TestFaviconFetcher_SSRFBlocked_StableAfterReuse はクライアント再利用後もSSRFブロックが
// 維持され、複数回試行で各回ともValidateURL経由でブロックされることをテストする（AC 5.2, 5.4）。
func TestFaviconFetcher_SSRFBlocked_StableAfterReuse(t *testing.T) {
	// Arrange
	guard := &mockSSRFGuard{blockAll: true}
	f := NewFaviconFetcher(guard)

	// Act & Assert: 2回連続でブロック（nil・空MIME・エラーなし）
	for i := 0; i < 2; i++ {
		data, mimeType, err := f.FetchFavicon(context.Background(), "http://192.168.1.1/favicon.ico")
		if err != nil {
			t.Fatalf("iteration %d: SSRFブロック時はエラーを返すべきでない: %v", i, err)
		}
		if data != nil {
			t.Errorf("iteration %d: SSRFブロック時はnilデータであるべき", i)
		}
		if mimeType != "" {
			t.Errorf("iteration %d: SSRFブロック時は空MIMEであるべき。結果: %q", i, mimeType)
		}
	}
	// 各リクエストでValidateURLが呼ばれている
	if guard.validateURLCalls() != 2 {
		t.Errorf("ValidateURLは各リクエストで呼ばれるべき。期待: 2回, 結果: %d 回", guard.validateURLCalls())
	}
}

// TestFaviconFetcher_GetHTTPClient_TimeoutPreserved は再利用クライアントが既存の
// タイムアウト値(faviconTimeout=5秒)を維持していることをテストする（AC 6.2）。
func TestFaviconFetcher_GetHTTPClient_TimeoutPreserved(t *testing.T) {
	// Arrange
	f := NewFaviconFetcher(nil)

	// Act
	client := f.getHTTPClient()

	// Assert
	if client.Timeout != faviconTimeout {
		t.Errorf("FaviconFetcherのタイムアウトはfaviconTimeout(%v)であるべき。結果: %v", faviconTimeout, client.Timeout)
	}
}

// TestFaviconFetcher_Concurrent_NoDataRace は複数goroutineから同一インスタンスを
// 同時利用してもデータ競合が発生しないことをテストする（NFR 2.1。-race と併用）。
func TestFaviconFetcher_Concurrent_NoDataRace(t *testing.T) {
	// Arrange
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	f := NewFaviconFetcher(guard)

	// Act: 10 goroutineから同時取得
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = f.FetchFavicon(context.Background(), server.URL+"/favicon.ico")
		}()
	}
	wg.Wait()

	// Assert: 競合が無いことは -race フラグが検出する。
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
