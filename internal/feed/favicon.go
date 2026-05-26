// Package feed はフィード登録・管理のドメインロジックを提供する。
package feed

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxFaviconSize はfaviconの最大サイズ（2MB）。
const maxFaviconSize = 2 * 1024 * 1024

// faviconTimeout はfavicon取得のタイムアウト。
const faviconTimeout = 5 * time.Second

// FaviconFetcherService はfavicon取得のインターフェース。
type FaviconFetcherService interface {
	// FetchFavicon は指定URLからfaviconを取得する。
	// 取得失敗時はnilデータと空MIMEを返す（エラーは返さない）。
	FetchFavicon(ctx context.Context, faviconURL string) (data []byte, mimeType string, err error)

	// FetchFaviconForSite はサイトURLからfaviconを推測して取得する。
	// /favicon.ico を試行し、取得失敗時はnilデータと空MIMEを返す。
	FetchFaviconForSite(ctx context.Context, siteURL string) (data []byte, mimeType string, err error)
}

// FaviconFetcher はfavicon取得機能の実装。
type FaviconFetcher struct {
	ssrfGuard SSRFValidator
	// httpClient はリクエスト間で再利用するHTTPクライアント。
	// コンストラクタで一度だけ生成し、生成後は read-only なフィールド参照となるため、
	// 複数 goroutine からの同時アクセスでもデータ競合は発生しない（NFR 2.1）。
	httpClient *http.Client
}

// NewFaviconFetcher はFaviconFetcherの新しいインスタンスを生成する。
// HTTPクライアントはここで一度だけ生成し、以降のリクエストで使い回す
// （コネクションプールを共有して無駄な TCP/TLS ハンドシェイクを抑制する）。
func NewFaviconFetcher(ssrfGuard SSRFValidator) *FaviconFetcher {
	return &FaviconFetcher{
		ssrfGuard:  ssrfGuard,
		httpClient: newFaviconHTTPClient(ssrfGuard),
	}
}

// newFaviconHTTPClient はfavicon取得用のHTTPクライアントを生成する。
// SSRFGuardが設定されている場合はSSRF防止付きクライアントを返す。
func newFaviconHTTPClient(ssrfGuard SSRFValidator) *http.Client {
	if ssrfGuard != nil {
		return ssrfGuard.NewSafeClient(faviconTimeout, maxFaviconSize)
	}
	return &http.Client{Timeout: faviconTimeout}
}

// FetchFavicon は指定URLからfaviconを取得する。
// 取得失敗時はnilデータと空MIMEを返す（要件: 取得失敗時はnullとして保存）。
func (f *FaviconFetcher) FetchFavicon(ctx context.Context, faviconURL string) ([]byte, string, error) {
	if faviconURL == "" {
		return nil, "", nil
	}

	// SSRF検証
	if f.ssrfGuard != nil {
		if err := f.ssrfGuard.ValidateURL(faviconURL); err != nil {
			slog.Warn("favicon取得: SSRFブロック", "url", faviconURL, "error", err)
			return nil, "", nil
		}
	}

	// HTTPクライアント取得
	client := f.getHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, faviconURL, nil)
	if err != nil {
		slog.Warn("favicon取得: リクエスト作成失敗", "url", faviconURL, "error", err)
		return nil, "", nil
	}
	req.Header.Set("User-Agent", "Feedman/1.0 RSS Reader")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("favicon取得: HTTPリクエスト失敗", "url", faviconURL, "error", err)
		return nil, "", nil
	}
	defer resp.Body.Close()

	// 2xx以外はfavicon取得失敗として扱う
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("favicon取得: HTTPステータス異常", "url", faviconURL, "status", resp.StatusCode)
		return nil, "", nil
	}

	// レスポンスボディを読み込み（最大2MB）
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFaviconSize+1))
	if err != nil {
		slog.Warn("favicon取得: レスポンス読み取り失敗", "url", faviconURL, "error", err)
		return nil, "", nil
	}

	// サイズ超過チェック
	if int64(len(body)) > maxFaviconSize {
		slog.Warn("favicon取得: サイズ超過", "url", faviconURL, "size", len(body))
		return nil, "", nil
	}

	// Content-Typeを取得
	contentType := resp.Header.Get("Content-Type")
	mimeType := extractMimeType(contentType)

	// 画像でない場合はnilを返す
	if !isImageMime(mimeType) {
		slog.Warn("favicon取得: 画像以外のContent-Type", "url", faviconURL, "contentType", contentType)
		return nil, "", nil
	}

	return body, mimeType, nil
}

// FetchFaviconForSite はサイトURLからfaviconを推測して取得する。
// /favicon.ico を試行し、取得失敗時はnilデータと空MIMEを返す。
func (f *FaviconFetcher) FetchFaviconForSite(ctx context.Context, siteURL string) ([]byte, string, error) {
	faviconURL := guessDefaultFaviconURL(siteURL)
	if faviconURL == "" {
		return nil, "", nil
	}
	return f.FetchFavicon(ctx, faviconURL)
}

// getHTTPClient はコンストラクタで生成済みの再利用HTTPクライアントを返す。
// リクエストごとに新しいクライアントを生成せず、コネクションプールを共有する。
func (f *FaviconFetcher) getHTTPClient() *http.Client {
	return f.httpClient
}

// guessDefaultFaviconURL はサイトURLからデフォルトのfavicon URLを推測する。
func guessDefaultFaviconURL(siteURL string) string {
	if siteURL == "" {
		return ""
	}

	u, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}

	// パスを/favicon.icoに設定
	u.Path = "/favicon.ico"
	u.RawQuery = ""
	u.Fragment = ""

	return u.String()
}

// extractMimeType はContent-Typeヘッダーからメディアタイプを抽出する。
func extractMimeType(contentType string) string {
	if contentType == "" {
		return ""
	}
	// セミコロンの前の部分（charset等を除去）
	parts := strings.SplitN(contentType, ";", 2)
	return strings.TrimSpace(strings.ToLower(parts[0]))
}

// isImageMime はMIMEタイプが画像かどうかを判定する。
func isImageMime(mimeType string) bool {
	if mimeType == "" {
		return false
	}
	imageTypes := []string{
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/svg+xml",
		"image/x-icon",
		"image/vnd.microsoft.icon",
		"image/webp",
		"image/bmp",
		"image/ico",
	}
	for _, t := range imageTypes {
		if mimeType == t {
			return true
		}
	}
	// image/ で始まるものは許可
	return strings.HasPrefix(mimeType, "image/")
}

// compile-time interface check
var _ FaviconFetcherService = (*FaviconFetcher)(nil)
