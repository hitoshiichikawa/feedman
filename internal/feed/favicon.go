// Package feed はフィード登録・管理のドメインロジックを提供する。
package feed

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

// maxFaviconSize はfaviconの最大サイズ（2MB）。
const maxFaviconSize = 2 * 1024 * 1024

// faviconTimeout はfavicon取得のタイムアウト。
const faviconTimeout = 5 * time.Second

// maxFeedFetchSizeForFavicon は favicon フォールバック用にフィード本体を取得する際の
// レスポンスサイズ上限（5MB）。Worker のフィード取得上限と同等の値を採用する。
const maxFeedFetchSizeForFavicon = 5 * 1024 * 1024

// maxHTMLFetchSize はサイト本体ドメインの HTML を解析するために取得するサイズ上限（2MB）。
// favicon と同等の上限を採用して NFR 1.2 を満たす。
const maxHTMLFetchSize = 2 * 1024 * 1024

// FaviconFetcherService はfavicon取得のインターフェース。
type FaviconFetcherService interface {
	// FetchFavicon は指定URLからfaviconを取得する。
	// 取得失敗時はnilデータと空MIMEを返す（エラーは返さない）。
	FetchFavicon(ctx context.Context, faviconURL string) (data []byte, mimeType string, err error)

	// FetchFaviconForSite はサイトURLからfaviconを推測して取得する。
	// /favicon.ico を試行し、取得失敗時はnilデータと空MIMEを返す。
	FetchFaviconForSite(ctx context.Context, siteURL string) (data []byte, mimeType string, err error)

	// FetchFaviconWithFallback はフィード配信 URL とサイト本体ドメインを起点に
	// 段階的に favicon を探索する。探索段階は以下の順序で試行され、いずれかが
	// 成功した時点で打ち切る:
	//   (a) フィード配信 URL ドメインの /favicon.ico
	//   (b) フィード配信 URL ドメインの HTML 内 <link rel="icon"> 等から抽出
	//   (c) フィード本体から抽出した記事リンクのドメインの /favicon.ico
	//   (d) (c) と同じドメインの HTML 内 <link rel="icon"> 等から抽出
	// 各段階の試行は構造化ログに記録される（NFR 2.1）。
	// すべての段階で取得できなかった場合は nil データと空 MIME を返す。
	FetchFaviconWithFallback(ctx context.Context, feedURL string) (data []byte, mimeType string, err error)
}

// FaviconFetcher はfavicon取得機能の実装。
type FaviconFetcher struct {
	ssrfGuard SSRFValidator
	// httpClient はリクエスト間で再利用するHTTPクライアント。
	// コンストラクタで一度だけ生成し、生成後は read-only なフィールド参照となるため、
	// 複数 goroutine からの同時アクセスでもデータ競合は発生しない（NFR 2.1）。
	//
	// favicon 取得・HTML 解析・フィード本体取得の各経路で同一クライアントを再利用する。
	// レスポンスサイズの上限はクライアント側では HTML / フィード取得を許容する大きめの値
	// （maxFeedFetchSizeForFavicon = 5MB）に設定し、favicon 経路では呼び出し側で
	// LimitReader と長さ検査により厳密に maxFaviconSize（2MB）まで切り詰める（NFR 1.2）。
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
// クライアント側のレスポンスサイズ上限は HTML / フィード本体取得を許容するために
// maxFeedFetchSizeForFavicon を採用し、favicon 経路は呼び出し側で LimitReader による
// 2MB 上限を別途強制する（NFR 1.2）。
func newFaviconHTTPClient(ssrfGuard SSRFValidator) *http.Client {
	if ssrfGuard != nil {
		return ssrfGuard.NewSafeClient(faviconTimeout, maxFeedFetchSizeForFavicon)
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

	// 透明判定（Issue #148）。alpha チャネルを持ち得る形式のみデコードして
	// 全ピクセル alpha=0 なら段階失敗扱いとする（要件 1.1, 1.2, 1.4）。
	// NFR 2.2 により、HTTP 2xx・image/* MIME・サイズ上限内を満たした画像にのみ実行する。
	transparent, decodeErr := checkFaviconTransparency(body, mimeType)
	if decodeErr != nil {
		// 要件 1.4 / 3.2: デコード失敗を段階失敗として扱い構造化ログに記録する。
		slog.Warn("favicon取得: デコード失敗（段階失敗扱い）",
			"url", faviconURL,
			"mime", mimeType,
			"reason", "decode-failed",
			"error", decodeErr,
		)
		return nil, "", nil
	}
	if transparent {
		// 要件 1.2 / 3.1: 全面透明を段階失敗として扱い構造化ログに記録する。
		slog.Warn("favicon取得: 全面透明（段階失敗扱い）",
			"url", faviconURL,
			"mime", mimeType,
			"reason", "transparent",
		)
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

// FetchFaviconWithFallback はフィード配信 URL とサイト本体ドメインを起点に
// 段階的に favicon を探索する（要件 1, 2）。
// 探索段階は (a) → (b) → (c) → (d) の順で試行され、最初に成功した段階の
// favicon を返す。各段階の試行結果は構造化ログに記録される。
func (f *FaviconFetcher) FetchFaviconWithFallback(ctx context.Context, feedURL string) ([]byte, string, error) {
	if feedURL == "" {
		return nil, "", nil
	}

	feedOrigin := originURL(feedURL)

	// 段階 (a): フィード配信 URL ドメインの /favicon.ico
	if data, mime := f.tryFaviconForOrigin(ctx, feedOrigin, "feed_origin_ico"); data != nil {
		return data, mime, nil
	}

	// 段階 (b): フィード配信 URL ドメインの HTML 解析
	if data, mime := f.tryFaviconViaHTML(ctx, feedOrigin, "feed_origin_html"); data != nil {
		return data, mime, nil
	}

	// 段階 (c) / (d): フィード本体から記事リンクを取得し、別ドメインなら再探索する
	siteOrigin := f.deriveSiteOriginFromFeed(ctx, feedURL)
	if siteOrigin == "" || siteOrigin == feedOrigin {
		// 記事リンクが取得できない / 配信ドメインと同一なら再試行する意味がない。
		slog.Info("favicon取得: サイト本体ドメイン未取得（フォールバック打ち切り）",
			"feedURL", feedURL,
			"feedOrigin", feedOrigin,
		)
		return nil, "", nil
	}

	// 段階 (c)
	if data, mime := f.tryFaviconForOrigin(ctx, siteOrigin, "site_origin_ico"); data != nil {
		return data, mime, nil
	}

	// 段階 (d)
	if data, mime := f.tryFaviconViaHTML(ctx, siteOrigin, "site_origin_html"); data != nil {
		return data, mime, nil
	}

	slog.Info("favicon取得: 全段階で失敗",
		"feedURL", feedURL,
		"feedOrigin", feedOrigin,
		"siteOrigin", siteOrigin,
	)
	return nil, "", nil
}

// tryFaviconForOrigin は指定オリジンの /favicon.ico を試行する。
// 成功時は (data, mime) を返し、失敗時は (nil, "") を返す。stage は構造化ログ用ラベル。
func (f *FaviconFetcher) tryFaviconForOrigin(ctx context.Context, origin, stage string) ([]byte, string) {
	if origin == "" {
		return nil, ""
	}
	faviconURL := guessDefaultFaviconURL(origin)
	data, mime, _ := f.FetchFavicon(ctx, faviconURL)
	if data != nil {
		slog.Info("favicon取得: 成功", "stage", stage, "url", faviconURL, "mime", mime)
		return data, mime
	}
	slog.Info("favicon取得: 段階失敗", "stage", stage, "url", faviconURL)
	return nil, ""
}

// tryFaviconViaHTML は指定オリジンの HTML を取得し、<link rel="icon"> 等から
// favicon URL を抽出して取得する。stage は構造化ログ用ラベル。
func (f *FaviconFetcher) tryFaviconViaHTML(ctx context.Context, origin, stage string) ([]byte, string) {
	if origin == "" {
		return nil, ""
	}
	htmlBody, baseURL := f.fetchHTML(ctx, origin)
	if len(htmlBody) == 0 {
		slog.Info("favicon取得: HTML取得失敗", "stage", stage, "origin", origin)
		return nil, ""
	}
	iconURL := parseFaviconURLFromHTML(htmlBody, baseURL)
	if iconURL == "" {
		slog.Info("favicon取得: HTML内にicon宣言なし", "stage", stage, "origin", origin)
		return nil, ""
	}
	data, mime, _ := f.FetchFavicon(ctx, iconURL)
	if data != nil {
		slog.Info("favicon取得: 成功", "stage", stage, "url", iconURL, "mime", mime)
		return data, mime
	}
	slog.Info("favicon取得: 段階失敗", "stage", stage, "url", iconURL)
	return nil, ""
}

// fetchHTML は指定 URL の HTML を取得する。
// 取得失敗時は (nil, "") を返す。返却 baseURL は相対 URL 解決用にレスポンスの最終 URL を返す。
func (f *FaviconFetcher) fetchHTML(ctx context.Context, pageURL string) ([]byte, string) {
	if pageURL == "" {
		return nil, ""
	}
	if f.ssrfGuard != nil {
		if err := f.ssrfGuard.ValidateURL(pageURL); err != nil {
			slog.Warn("HTML取得: SSRFブロック", "url", pageURL, "error", err)
			return nil, ""
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		slog.Warn("HTML取得: リクエスト作成失敗", "url", pageURL, "error", err)
		return nil, ""
	}
	req.Header.Set("User-Agent", "Feedman/1.0 RSS Reader")
	req.Header.Set("Accept", "text/html, application/xhtml+xml, */*")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		slog.Warn("HTML取得: HTTPリクエスト失敗", "url", pageURL, "error", err)
		return nil, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("HTML取得: HTTPステータス異常", "url", pageURL, "status", resp.StatusCode)
		return nil, ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLFetchSize+1))
	if err != nil {
		slog.Warn("HTML取得: レスポンス読み取り失敗", "url", pageURL, "error", err)
		return nil, ""
	}
	if int64(len(body)) > maxHTMLFetchSize {
		slog.Warn("HTML取得: サイズ超過", "url", pageURL, "size", len(body))
		return nil, ""
	}

	// 相対 URL 解決用のベース URL は最終的なリクエスト URL（リダイレクト追従後）を採用する。
	baseURL := pageURL
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL.String()
	}
	return body, baseURL
}

// deriveSiteOriginFromFeed はフィード本体を取得・パースし、
// 最初の有効な記事リンクからサイト本体オリジンを取得する。
// 取得できない場合は空文字を返す。
func (f *FaviconFetcher) deriveSiteOriginFromFeed(ctx context.Context, feedURL string) string {
	if feedURL == "" {
		return ""
	}
	if f.ssrfGuard != nil {
		if err := f.ssrfGuard.ValidateURL(feedURL); err != nil {
			slog.Warn("favicon取得: フィード再取得 SSRFブロック", "url", feedURL, "error", err)
			return ""
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		slog.Warn("favicon取得: フィード再取得 リクエスト作成失敗", "url", feedURL, "error", err)
		return ""
	}
	req.Header.Set("User-Agent", "Feedman/1.0 RSS Reader")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		slog.Warn("favicon取得: フィード再取得 HTTPリクエスト失敗", "url", feedURL, "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("favicon取得: フィード再取得 HTTPステータス異常", "url", feedURL, "status", resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedFetchSizeForFavicon+1))
	if err != nil {
		slog.Warn("favicon取得: フィード再取得 読み取り失敗", "url", feedURL, "error", err)
		return ""
	}
	if int64(len(body)) > maxFeedFetchSizeForFavicon {
		slog.Warn("favicon取得: フィード再取得 サイズ超過", "url", feedURL, "size", len(body))
		return ""
	}

	parser := gofeed.NewParser()
	parsedFeed, err := parser.ParseString(string(body))
	if err != nil {
		slog.Warn("favicon取得: フィードパース失敗", "url", feedURL, "error", err)
		return ""
	}

	// 記事リンクから site origin を抽出する（要件 1.1）。
	// 記事が 1 件もない場合は要件 1.3 に従い空文字を返す。
	for _, item := range parsedFeed.Items {
		if item == nil || item.Link == "" {
			continue
		}
		origin := originURL(item.Link)
		if origin != "" {
			return origin
		}
	}
	slog.Info("favicon取得: 記事リンクなし（フォールバック打ち切り）", "url", feedURL)
	return ""
}

// parseFaviconURLFromHTML は HTML から favicon を宣言する link 要素を抽出し、
// 絶対 URL を返す。優先順位は以下:
//  1. rel="icon"
//  2. rel="shortcut icon"
//  3. rel="apple-touch-icon" / rel="apple-touch-icon-precomposed"
//
// 複数の候補があれば優先度の高いものを返す。見つからない場合は空文字。
func parseFaviconURLFromHTML(htmlBody []byte, baseURL string) string {
	baseU, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	var found []candidate

	tokenizer := html.NewTokenizer(bytes.NewReader(htmlBody))
	inHead := false

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return selectBestFaviconCandidate(found, baseU)
		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := tokenizer.TagName()
			tagName := string(tn)

			if tagName == "head" {
				inHead = true
				continue
			}
			if tagName == "body" {
				return selectBestFaviconCandidate(found, baseU)
			}
			if !inHead || tagName != "link" || !hasAttr {
				continue
			}

			var rel, href string
			for {
				key, val, more := tokenizer.TagAttr()
				k := strings.ToLower(string(key))
				v := string(val)
				switch k {
				case "rel":
					rel = strings.ToLower(strings.TrimSpace(v))
				case "href":
					href = strings.TrimSpace(v)
				}
				if !more {
					break
				}
			}
			if href == "" {
				continue
			}
			// rel 属性は空白区切りで複数値を持てる。各値を確認する。
			priority := faviconRelPriority(rel)
			if priority < 0 {
				continue
			}
			found = append(found, candidate{href: href, priority: priority})
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			if string(tn) == "head" {
				return selectBestFaviconCandidate(found, baseU)
			}
		}
	}
}

// faviconRelPriority は rel 属性値に含まれる favicon 関連の rel を判定し、
// 優先度を返す（低いほど優先）。該当する rel が含まれなければ -1。
func faviconRelPriority(rel string) int {
	if rel == "" {
		return -1
	}
	// 空白区切りで複数 rel を持てる: 例 "icon shortcut"
	tokens := strings.Fields(rel)
	hasIcon := false
	hasShortcutIcon := false
	hasAppleTouch := false
	for _, tok := range tokens {
		switch tok {
		case "icon":
			hasIcon = true
		case "shortcut":
			// shortcut 単独では favicon と限らないが、典型 "shortcut icon" のため
			// "icon" を併せ持つときのみ favicon 扱いとする（後段で判定）。
		case "apple-touch-icon", "apple-touch-icon-precomposed":
			hasAppleTouch = true
		}
	}
	// "shortcut icon" の組み合わせ判定: rel 全体に "shortcut" と "icon" の両方が含まれるかを直接確認
	if strings.Contains(rel, "shortcut") && hasIcon {
		hasShortcutIcon = true
	}
	switch {
	case hasIcon && !hasShortcutIcon:
		return 0
	case hasShortcutIcon:
		return 1
	case hasAppleTouch:
		return 2
	}
	return -1
}

// selectBestFaviconCandidate は candidate のうち最も優先度が高い（priority が最小の）
// 候補の href を絶対 URL に解決して返す。
func selectBestFaviconCandidate(found []candidate, baseU *url.URL) string {
	if len(found) == 0 {
		return ""
	}
	bestIdx := 0
	for i := 1; i < len(found); i++ {
		if found[i].priority < found[bestIdx].priority {
			bestIdx = i
		}
	}
	href := found[bestIdx].href
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return baseU.ResolveReference(ref).String()
}

// candidate は parseFaviconURLFromHTML 内で利用する favicon 候補。
// （selectBestFaviconCandidate のシグネチャを安定させるためにファイルスコープに置く。）
type candidate struct {
	href     string
	priority int
}

// originURL は URL から scheme + host のみを抽出した「オリジン URL」を返す。
// パース失敗時や scheme/host が空の場合は空文字を返す。
func originURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
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
