package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// pngBody はテスト用 favicon ペイロード（不透明 PNG）。
// Issue #148 の透明判定でデコードされるため、8 バイトのマジックバイトのみではなく
// 有効な PNG（不透明 4x4）を遅延初期化する。各テストは t.Helper() 経由で生成する。
//
// 旧仕様: 8 バイトの PNG マジック値だけで「画像として成功」を判定していたが、
// 透明判定導入後はデコード可能でかつ alpha>0 のピクセルを 1 件以上含むことを要求する。
var pngBody = mustGenerateOpaquePNG(4, 4)

// mustGenerateOpaquePNG は init 時に呼ばれる pngBody 生成用ヘルパー。
// パッケージレベル変数初期化のため testing.T を取らない（panic 経路で失敗を露見させる）。
func mustGenerateOpaquePNG(width, height int) []byte {
	return generateOpaquePNGForInit(width, height)
}

// --- parseFaviconURLFromHTML のテスト（要件 2.4） ---

// TestParseFaviconURLFromHTML_IconAbsolute はHTML内の rel="icon" + 絶対URLを抽出するテスト。
func TestParseFaviconURLFromHTML_IconAbsolute(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="icon" href="https://cdn.example.com/icon.png">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	want := "https://cdn.example.com/icon.png"
	if got != want {
		t.Errorf("parseFaviconURLFromHTML = %q, want %q", got, want)
	}
}

// TestParseFaviconURLFromHTML_RelativeResolved は rel="icon" + 相対URLを baseURL で絶対化するテスト。
func TestParseFaviconURLFromHTML_RelativeResolved(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="icon" href="/static/favicon.svg">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/blog")
	want := "https://example.com/static/favicon.svg"
	if got != want {
		t.Errorf("parseFaviconURLFromHTML = %q, want %q", got, want)
	}
}

// TestParseFaviconURLFromHTML_ShortcutIcon は rel="shortcut icon" を検出するテスト。
func TestParseFaviconURLFromHTML_ShortcutIcon(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="shortcut icon" href="/favicon.ico">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	want := "https://example.com/favicon.ico"
	if got != want {
		t.Errorf("parseFaviconURLFromHTML = %q, want %q", got, want)
	}
}

// TestParseFaviconURLFromHTML_AppleTouchIcon は rel="apple-touch-icon" を検出するテスト。
func TestParseFaviconURLFromHTML_AppleTouchIcon(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="apple-touch-icon" href="/apple-icon.png">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	want := "https://example.com/apple-icon.png"
	if got != want {
		t.Errorf("parseFaviconURLFromHTML = %q, want %q", got, want)
	}
}

// TestParseFaviconURLFromHTML_PriorityIconOverShortcut は icon と shortcut icon が両方ある場合に
// 優先度の高い rel="icon" を選択するテスト。
func TestParseFaviconURLFromHTML_PriorityIconOverShortcut(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="shortcut icon" href="/old.ico">
		<link rel="icon" href="/new.png">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	want := "https://example.com/new.png"
	if got != want {
		t.Errorf("rel=\"icon\" は shortcut icon より優先されるべき。got %q, want %q", got, want)
	}
}

// TestParseFaviconURLFromHTML_PriorityShortcutOverAppleTouch は shortcut icon と apple-touch-icon
// が両方ある場合に shortcut icon が選択されるテスト。
func TestParseFaviconURLFromHTML_PriorityShortcutOverAppleTouch(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="apple-touch-icon" href="/apple.png">
		<link rel="shortcut icon" href="/favicon.ico">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	want := "https://example.com/favicon.ico"
	if got != want {
		t.Errorf("shortcut icon は apple-touch-icon より優先されるべき。got %q, want %q", got, want)
	}
}

// TestParseFaviconURLFromHTML_NoMatch は icon 系の link が存在しない場合に空文字を返すテスト。
func TestParseFaviconURLFromHTML_NoMatch(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="stylesheet" href="/style.css">
		<link rel="alternate" type="application/rss+xml" href="/feed.xml">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	if got != "" {
		t.Errorf("icon系の link がない場合は空文字を返すべき。got %q", got)
	}
}

// TestParseFaviconURLFromHTML_IgnoreOutsideHead は head 外の link 要素を無視するテスト。
func TestParseFaviconURLFromHTML_IgnoreOutsideHead(t *testing.T) {
	htmlBody := []byte(`<html><head><title>x</title></head>
		<body><link rel="icon" href="/should-be-ignored.png"></body></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	if got != "" {
		t.Errorf("body 内の link は無視されるべき。got %q", got)
	}
}

// TestParseFaviconURLFromHTML_EmptyHref は href が空の場合に空文字を返すテスト。
func TestParseFaviconURLFromHTML_EmptyHref(t *testing.T) {
	htmlBody := []byte(`<html><head>
		<link rel="icon" href="">
	</head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "https://example.com/")
	if got != "" {
		t.Errorf("href が空の場合は空文字を返すべき。got %q", got)
	}
}

// TestParseFaviconURLFromHTML_InvalidBaseURL は baseURL が不正な場合に空文字を返すテスト。
func TestParseFaviconURLFromHTML_InvalidBaseURL(t *testing.T) {
	htmlBody := []byte(`<html><head><link rel="icon" href="/icon.png"></head></html>`)

	got := parseFaviconURLFromHTML(htmlBody, "://invalid-url")
	if got != "" {
		t.Errorf("baseURL が不正なら空文字を返すべき。got %q", got)
	}
}

// --- originURL のテスト ---

// TestOriginURL は様々な URL に対して scheme + host を抽出することをテストする。
func TestOriginURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://example.com/path?q=1#h", "https://example.com"},
		{"https://example.com:8080/", "https://example.com:8080"},
		{"http://example.com", "http://example.com"},
		{"", ""},
		{"not-a-url", ""},
		{"//example.com/path", ""}, // scheme なし → 空
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := originURL(tt.in)
			if got != tt.want {
				t.Errorf("originURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- FetchFaviconWithFallback の統合テスト（要件 1, 2） ---

// TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds は段階 (a) の /favicon.ico が
// 取得できる場合に他の段階を試行せずに採用するテスト（要件 2.1, 2.2, 4.1）。
func TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds(t *testing.T) {
	var stageBHit, stageCHit, stageDHit atomic.Bool

	// Issue #148: 透明判定で ICO は内包 BMP の alpha バイトを走査するため有効な ICO バイト列を返す
	icoBody := newOpaqueICO(t, 8, 8)

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(icoBody)
		case "/":
			// 段階 (b) で来る HTML
			stageBHit.Store(true)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/never.png"></head></html>`))
		case "/feed.xml":
			// 段階 (c) のためのフィード本体
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stageCHit.Store(true)
		stageDHit.Store(true)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer siteSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("段階 (a) で favicon が取得できるべき")
	}
	if mime != "image/x-icon" {
		t.Errorf("MIME = %q, want image/x-icon", mime)
	}
	if stageBHit.Load() {
		t.Error("段階 (a) で成功している場合、段階 (b) は試行されてはならない")
	}
	if stageCHit.Load() {
		t.Error("段階 (a) で成功している場合、段階 (c) は試行されてはならない")
	}
	if stageDHit.Load() {
		t.Error("段階 (a) で成功している場合、段階 (d) は試行されてはならない")
	}
}

// TestFetchFaviconWithFallback_StageB_FeedOriginHTMLSucceeds は段階 (a) が 404 で
// 段階 (b) の HTML 内 <link rel="icon"> から取得できるテスト（要件 2.4）。
func TestFetchFaviconWithFallback_StageB_FeedOriginHTMLSucceeds(t *testing.T) {
	var stageCHit, stageDHit atomic.Bool

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			// 段階 (a) は失敗
			w.WriteHeader(http.StatusNotFound)
		case "/":
			// 段階 (b) で来る HTML、icon を宣言
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/declared-icon.png"></head></html>`))
		case "/declared-icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBody)
		case "/feed.xml":
			// 段階 (c) のためのフィード本体
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stageCHit.Store(true)
		stageDHit.Store(true)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer siteSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("段階 (b) で favicon が取得できるべき")
	}
	if mime != "image/png" {
		t.Errorf("MIME = %q, want image/png", mime)
	}
	if stageCHit.Load() || stageDHit.Load() {
		t.Error("段階 (b) で成功している場合、(c)(d) は試行されてはならない")
	}
}

// TestFetchFaviconWithFallback_StageC_SiteOriginICOSucceeds は段階 (a)(b) が失敗、
// 段階 (c)（記事リンクから推測したサイト本体ドメインの /favicon.ico）で取得できるテスト
// （要件 1.1, 1.2, 2.1）。
func TestFetchFaviconWithFallback_StageC_SiteOriginICOSucceeds(t *testing.T) {
	var siteFaviconHit, siteHTMLHit atomic.Bool
	var siteHostBase string

	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			siteFaviconHit.Store(true)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBody)
		case "/":
			siteHTMLHit.Store(true)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer siteSrv.Close()
	siteHostBase = siteSrv.URL

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.WriteHeader(http.StatusNotFound) // 段階 (a) 失敗
		case "/":
			w.WriteHeader(http.StatusNotFound) // 段階 (b) HTML 取得失敗
		case "/feed.xml":
			// フィード本体: 記事リンクが別ドメイン
			rss := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Test</title>
<link>%s</link>
<item><title>Item 1</title><link>%s/article-1</link></item>
</channel></rss>`, siteHostBase, siteHostBase)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("段階 (c) で favicon が取得できるべき")
	}
	if mime != "image/png" {
		t.Errorf("MIME = %q, want image/png", mime)
	}
	if !siteFaviconHit.Load() {
		t.Error("サイト本体ドメインの /favicon.ico がリクエストされるべき")
	}
	if siteHTMLHit.Load() {
		t.Error("段階 (c) で成功している場合、(d) は試行されてはならない")
	}
}

// TestFetchFaviconWithFallback_StageD_SiteOriginHTMLSucceeds は段階 (a)(b)(c) が失敗、
// 段階 (d)（サイト本体ドメインの HTML 内 icon 宣言）で取得できるテスト（要件 1.1, 2.4）。
func TestFetchFaviconWithFallback_StageD_SiteOriginHTMLSucceeds(t *testing.T) {
	var siteHostBase string

	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.WriteHeader(http.StatusNotFound) // 段階 (c) 失敗
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="shortcut icon" href="/icon.svg"></head></html>`))
		case "/icon.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg/>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer siteSrv.Close()
	siteHostBase = siteSrv.URL

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.WriteHeader(http.StatusNotFound) // 段階 (a) 失敗
		case "/":
			w.WriteHeader(http.StatusNotFound) // 段階 (b) 失敗
		case "/feed.xml":
			rss := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Test</title>
<link>%s</link>
<item><title>Item 1</title><link>%s/article-1</link></item>
</channel></rss>`, siteHostBase, siteHostBase)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("段階 (d) で favicon が取得できるべき")
	}
	if !strings.HasPrefix(mime, "image/") {
		t.Errorf("MIME は画像であるべき。got %q", mime)
	}
}

// TestFetchFaviconWithFallback_NoArticles_StopsAfterStageB はフィードに記事リンクが
// 1 件もない場合、段階 (b) で打ち切られサイト本体ドメイン側にアクセスしないテスト
// （要件 1.3）。
func TestFetchFaviconWithFallback_NoArticles_StopsAfterStageB(t *testing.T) {
	var siteHit atomic.Bool

	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		siteHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer siteSrv.Close()

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.WriteHeader(http.StatusNotFound)
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html></html>`)) // icon 宣言なし
		case "/feed.xml":
			// 記事リンク 0 件のフィード
			rss := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Empty Feed</title>
<link>https://example.com</link>
</channel></rss>`
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, _, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback returned error: %v", err)
	}
	if data != nil {
		t.Error("全段階で取得不可なら nil を返すべき")
	}
	if siteHit.Load() {
		t.Error("記事リンク 0 件の場合、サイト本体ドメインへのアクセスは行われるべきでない（要件 1.3）")
	}
}

// TestFetchFaviconWithFallback_AllStagesFail_ReturnsNil は全段階で取得失敗した場合に
// nil・空 MIME・エラーなしを返すテスト（要件 1.4）。
func TestFetchFaviconWithFallback_AllStagesFail_ReturnsNil(t *testing.T) {
	var siteHostBase string
	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer siteSrv.Close()
	siteHostBase = siteSrv.URL

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico", "/":
			w.WriteHeader(http.StatusNotFound)
		case "/feed.xml":
			rss := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Test</title>
<link>%s</link>
<item><title>x</title><link>%s/a</link></item>
</channel></rss>`, siteHostBase, siteHostBase)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback returned error: %v", err)
	}
	if data != nil {
		t.Errorf("全段階失敗時は nil を返すべき。got %d bytes", len(data))
	}
	if mime != "" {
		t.Errorf("全段階失敗時は空 MIME を返すべき。got %q", mime)
	}
}

// TestFetchFaviconWithFallback_SameHostArticleLink_NoStageCDExtraFetch は記事リンクの
// オリジンがフィード配信ドメインと同一の場合、段階 (c)(d) で重複試行しないテスト
// （要件 4.1 既存正常ケースの非リグレッション）。
func TestFetchFaviconWithFallback_SameHostArticleLink_NoStageCDExtraFetch(t *testing.T) {
	var faviconReqs atomic.Int64

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			faviconReqs.Add(1)
			w.WriteHeader(http.StatusNotFound) // 段階 (a) 失敗
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html></html>`)) // 段階 (b) icon 宣言なし
		case "/feed.xml":
			// 同一ホスト記事リンク
			rss := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Test</title>
<item><link>/article-1</link></item>
</channel></rss>`
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	// 相対リンクは gofeed で解決されないので、絶対リンクで同一ホストにする
	feedURL := feedSrv.URL + "/feed.xml"
	// 上書き: 同一ホスト記事リンクをフルURLで設定
	feedSrv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			faviconReqs.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html></html>`))
		case "/feed.xml":
			rss := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Test</title>
<item><link>%s/article-1</link></item>
</channel></rss>`, feedSrv.URL)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f := NewFaviconFetcher(&mockSSRFGuard{})
	_, _, _ = f.FetchFaviconWithFallback(context.Background(), feedURL)

	// 段階 (a) で 1 回、段階 (c) で同一ホスト判定により skip されるので /favicon.ico は 1 回のみ
	if got := faviconReqs.Load(); got != 1 {
		t.Errorf("同一ホスト記事リンクの場合、/favicon.ico は段階 (a) のみ 1 回試行されるべき。got %d", got)
	}
}

// TestFetchFaviconWithFallback_EmptyFeedURL は feedURL が空文字の場合に
// nil を返すテスト（境界値）。
func TestFetchFaviconWithFallback_EmptyFeedURL(t *testing.T) {
	f := NewFaviconFetcher(&mockSSRFGuard{})
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), "")
	if err != nil {
		t.Fatalf("空 feedURL でエラーを返すべきでない: %v", err)
	}
	if data != nil || mime != "" {
		t.Errorf("空 feedURL では nil/空 MIME を返すべき。data=%v mime=%q", data, mime)
	}
}

// TestFetchFaviconWithFallback_SSRFGuardBlocksAll は SSRF ガードが全 URL を
// ブロックする場合に nil を返し、外部リクエストが発生しないテスト（NFR 1.1）。
func TestFetchFaviconWithFallback_SSRFGuardBlocksAll(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// blockAll = true の SSRF ガードを使い、各段階がブロックされることを観測する
	guard := &mockSSRFGuard{blockAll: true}
	f := NewFaviconFetcher(guard)

	data, _, err := f.FetchFaviconWithFallback(context.Background(), srv.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("SSRF ブロック時はエラーを返すべきでない: %v", err)
	}
	if data != nil {
		t.Error("SSRF ブロック時は nil データを返すべき")
	}
	if requests.Load() != 0 {
		t.Errorf("SSRF ブロック時は外部 HTTP リクエストが発生してはならない。got %d", requests.Load())
	}
}

// --- 補助関数のテスト ---

// TestFaviconRelPriority は様々な rel 値に対する優先度判定をテストする。
func TestFaviconRelPriority(t *testing.T) {
	tests := []struct {
		rel  string
		want int
	}{
		{"icon", 0},
		{"shortcut icon", 1},
		{"icon shortcut", 1},
		{"apple-touch-icon", 2},
		{"apple-touch-icon-precomposed", 2},
		{"", -1},
		{"stylesheet", -1},
		{"alternate", -1},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("rel=%q", tt.rel), func(t *testing.T) {
			got := faviconRelPriority(tt.rel)
			if got != tt.want {
				t.Errorf("faviconRelPriority(%q) = %d, want %d", tt.rel, got, tt.want)
			}
		})
	}
}

// TestSelectBestFaviconCandidate_Empty は候補ゼロ件で空文字を返すテスト。
func TestSelectBestFaviconCandidate_Empty(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	got := selectBestFaviconCandidate(nil, base)
	if got != "" {
		t.Errorf("候補 0 件で空文字を返すべき。got %q", got)
	}
}

// TestSelectBestFaviconCandidate_PicksLowestPriority は priority が最小の候補が
// 選択されることをテストする。
func TestSelectBestFaviconCandidate_PicksLowestPriority(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	cands := []candidate{
		{href: "/apple.png", priority: 2},
		{href: "/icon.png", priority: 0},
		{href: "/shortcut.png", priority: 1},
	}
	got := selectBestFaviconCandidate(cands, base)
	want := "https://example.com/icon.png"
	if got != want {
		t.Errorf("priority 最小が選択されるべき。got %q, want %q", got, want)
	}
}
