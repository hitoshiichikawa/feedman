package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// --- IsDirectFeed のテスト ---

// TestIsDirectFeed_RSSContentType はContent-Typeがapplication/rss+xmlの場合にtrueを返すことをテストする。
func TestIsDirectFeed_RSSContentType(t *testing.T) {
	d := NewFeedDetector(nil)
	if !d.IsDirectFeed("application/rss+xml", nil) {
		t.Error("application/rss+xml はフィードと判定されるべき")
	}
}

// TestIsDirectFeed_AtomContentType はContent-Typeがapplication/atom+xmlの場合にtrueを返すことをテストする。
func TestIsDirectFeed_AtomContentType(t *testing.T) {
	d := NewFeedDetector(nil)
	if !d.IsDirectFeed("application/atom+xml", nil) {
		t.Error("application/atom+xml はフィードと判定されるべき")
	}
}

// TestIsDirectFeed_XMLContentTypeWithRSSBody はContent-Typeがtext/xmlでボディがRSSの場合にtrueを返すことをテストする。
func TestIsDirectFeed_XMLContentTypeWithRSSBody(t *testing.T) {
	d := NewFeedDetector(nil)
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
	if !d.IsDirectFeed("text/xml", body) {
		t.Error("text/xml + RSSボディ はフィードと判定されるべき")
	}
}

// TestIsDirectFeed_XMLContentTypeWithAtomBody はContent-Typeがtext/xmlでボディがAtomの場合にtrueを返すことをテストする。
func TestIsDirectFeed_XMLContentTypeWithAtomBody(t *testing.T) {
	d := NewFeedDetector(nil)
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Test</title></feed>`)
	if !d.IsDirectFeed("text/xml", body) {
		t.Error("text/xml + Atomボディ はフィードと判定されるべき")
	}
}

// TestIsDirectFeed_ApplicationXMLWithRSSBody はContent-Typeがapplication/xmlでRSSボディの場合にtrueを返すことをテストする。
func TestIsDirectFeed_ApplicationXMLWithRSSBody(t *testing.T) {
	d := NewFeedDetector(nil)
	body := []byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
	if !d.IsDirectFeed("application/xml", body) {
		t.Error("application/xml + RSSボディ はフィードと判定されるべき")
	}
}

// TestIsDirectFeed_HTMLContentType はContent-Typeがtext/htmlの場合にfalseを返すことをテストする。
func TestIsDirectFeed_HTMLContentType(t *testing.T) {
	d := NewFeedDetector(nil)
	if d.IsDirectFeed("text/html", nil) {
		t.Error("text/html はフィードと判定されるべきではない")
	}
}

// TestIsDirectFeed_ContentTypeWithCharset はContent-Typeにcharsetパラメータが含まれる場合も正しく判定することをテストする。
func TestIsDirectFeed_ContentTypeWithCharset(t *testing.T) {
	d := NewFeedDetector(nil)
	if !d.IsDirectFeed("application/rss+xml; charset=utf-8", nil) {
		t.Error("application/rss+xml; charset=utf-8 はフィードと判定されるべき")
	}
}

// TestIsDirectFeed_XMLContentTypeWithHTMLBody はContent-Typeがtext/xmlだがHTMLボディの場合にfalseを返すことをテストする。
func TestIsDirectFeed_XMLContentTypeWithHTMLBody(t *testing.T) {
	d := NewFeedDetector(nil)
	body := []byte(`<?xml version="1.0"?><html><head><title>Test</title></head></html>`)
	if d.IsDirectFeed("text/xml", body) {
		t.Error("text/xml + HTMLボディ はフィードと判定されるべきではない")
	}
}

// --- ParseFeedLinksFromHTML のテスト ---

// TestParseFeedLinksFromHTML_SingleRSSLink はHTMLから単一のRSSリンクを検出することをテストする。
func TestParseFeedLinksFromHTML_SingleRSSLink(t *testing.T) {
	d := NewFeedDetector(nil)
	html := `<html><head>
		<link rel="alternate" type="application/rss+xml" title="RSS Feed" href="https://example.com/feed.xml">
	</head><body></body></html>`

	links := d.ParseFeedLinksFromHTML([]byte(html), "https://example.com")

	if len(links) != 1 {
		t.Fatalf("期待: 1リンク, 結果: %d リンク", len(links))
	}
	if links[0].URL != "https://example.com/feed.xml" {
		t.Errorf("期待URL: https://example.com/feed.xml, 結果: %s", links[0].URL)
	}
	if links[0].FeedType != FeedTypeRSS {
		t.Errorf("期待タイプ: RSS, 結果: %s", links[0].FeedType)
	}
}

// TestParseFeedLinksFromHTML_SingleAtomLink はHTMLから単一のAtomリンクを検出することをテストする。
func TestParseFeedLinksFromHTML_SingleAtomLink(t *testing.T) {
	d := NewFeedDetector(nil)
	html := `<html><head>
		<link rel="alternate" type="application/atom+xml" title="Atom Feed" href="https://example.com/atom.xml">
	</head><body></body></html>`

	links := d.ParseFeedLinksFromHTML([]byte(html), "https://example.com")

	if len(links) != 1 {
		t.Fatalf("期待: 1リンク, 結果: %d リンク", len(links))
	}
	if links[0].FeedType != FeedTypeAtom {
		t.Errorf("期待タイプ: Atom, 結果: %s", links[0].FeedType)
	}
}

// TestParseFeedLinksFromHTML_MultipleLinks はHTMLから複数のフィードリンクを検出することをテストする。
func TestParseFeedLinksFromHTML_MultipleLinks(t *testing.T) {
	d := NewFeedDetector(nil)
	html := `<html><head>
		<link rel="alternate" type="application/rss+xml" title="RSS" href="/rss.xml">
		<link rel="alternate" type="application/atom+xml" title="Atom" href="/atom.xml">
	</head><body></body></html>`

	links := d.ParseFeedLinksFromHTML([]byte(html), "https://example.com")

	if len(links) != 2 {
		t.Fatalf("期待: 2リンク, 結果: %d リンク", len(links))
	}
}

// TestParseFeedLinksFromHTML_RelativeURL は相対URLが正しく絶対URLに解決されることをテストする。
func TestParseFeedLinksFromHTML_RelativeURL(t *testing.T) {
	d := NewFeedDetector(nil)
	html := `<html><head>
		<link rel="alternate" type="application/rss+xml" href="/feed/rss.xml">
	</head><body></body></html>`

	links := d.ParseFeedLinksFromHTML([]byte(html), "https://blog.example.com/page")

	if len(links) != 1 {
		t.Fatalf("期待: 1リンク, 結果: %d リンク", len(links))
	}
	if links[0].URL != "https://blog.example.com/feed/rss.xml" {
		t.Errorf("期待URL: https://blog.example.com/feed/rss.xml, 結果: %s", links[0].URL)
	}
}

// TestParseFeedLinksFromHTML_NoLinks はフィードリンクがないHTMLで空スライスを返すことをテストする。
func TestParseFeedLinksFromHTML_NoLinks(t *testing.T) {
	d := NewFeedDetector(nil)
	html := `<html><head><title>No Feed</title></head><body></body></html>`

	links := d.ParseFeedLinksFromHTML([]byte(html), "https://example.com")

	if len(links) != 0 {
		t.Errorf("期待: 0リンク, 結果: %d リンク", len(links))
	}
}

// TestParseFeedLinksFromHTML_IgnoreNonAlternate はrel="alternate"以外のlinkタグを無視することをテストする。
func TestParseFeedLinksFromHTML_IgnoreNonAlternate(t *testing.T) {
	d := NewFeedDetector(nil)
	html := `<html><head>
		<link rel="stylesheet" type="text/css" href="/style.css">
		<link rel="icon" href="/favicon.ico">
		<link rel="alternate" type="application/rss+xml" href="/feed.xml">
	</head><body></body></html>`

	links := d.ParseFeedLinksFromHTML([]byte(html), "https://example.com")

	if len(links) != 1 {
		t.Fatalf("期待: 1リンク, 結果: %d リンク", len(links))
	}
}

// --- SelectBestFeed（優先順位ロジック）のテスト ---

// TestSelectBestFeed_SameHostPreferred は同一ホストのフィードが優先されることをテストする。
func TestSelectBestFeed_SameHostPreferred(t *testing.T) {
	d := NewFeedDetector(nil)
	candidates := []FeedCandidate{
		{URL: "https://other.com/feed.xml", FeedType: FeedTypeAtom, Title: "Other"},
		{URL: "https://example.com/feed.xml", FeedType: FeedTypeRSS, Title: "Same Host"},
	}

	best := d.SelectBestFeed(candidates, "https://example.com")

	if best.URL != "https://example.com/feed.xml" {
		t.Errorf("同一ホストのフィードが優先されるべき。期待: https://example.com/feed.xml, 結果: %s", best.URL)
	}
}

// TestSelectBestFeed_AtomPreferredOverRSS は同一ホスト内でAtomがRSSより優先されることをテストする。
func TestSelectBestFeed_AtomPreferredOverRSS(t *testing.T) {
	d := NewFeedDetector(nil)
	candidates := []FeedCandidate{
		{URL: "https://example.com/rss.xml", FeedType: FeedTypeRSS, Title: "RSS"},
		{URL: "https://example.com/atom.xml", FeedType: FeedTypeAtom, Title: "Atom"},
	}

	best := d.SelectBestFeed(candidates, "https://example.com")

	if best.URL != "https://example.com/atom.xml" {
		t.Errorf("Atomが優先されるべき。期待: https://example.com/atom.xml, 結果: %s", best.URL)
	}
}

// TestSelectBestFeed_FirstWhenSameCondition は同条件の場合に先頭が選択されることをテストする。
func TestSelectBestFeed_FirstWhenSameCondition(t *testing.T) {
	d := NewFeedDetector(nil)
	candidates := []FeedCandidate{
		{URL: "https://example.com/feed1.xml", FeedType: FeedTypeRSS, Title: "First"},
		{URL: "https://example.com/feed2.xml", FeedType: FeedTypeRSS, Title: "Second"},
	}

	best := d.SelectBestFeed(candidates, "https://example.com")

	if best.URL != "https://example.com/feed1.xml" {
		t.Errorf("同条件なら先頭が選択されるべき。期待: https://example.com/feed1.xml, 結果: %s", best.URL)
	}
}

// TestSelectBestFeed_SingleCandidate は候補が1つの場合にそれが選択されることをテストする。
func TestSelectBestFeed_SingleCandidate(t *testing.T) {
	d := NewFeedDetector(nil)
	candidates := []FeedCandidate{
		{URL: "https://other.com/feed.xml", FeedType: FeedTypeRSS, Title: "Only"},
	}

	best := d.SelectBestFeed(candidates, "https://example.com")

	if best.URL != "https://other.com/feed.xml" {
		t.Errorf("単一候補はそのまま選択されるべき。期待: https://other.com/feed.xml, 結果: %s", best.URL)
	}
}

// TestSelectBestFeed_EmptyCandidates は候補が0件の場合にnilを返すことをテストする。
func TestSelectBestFeed_EmptyCandidates(t *testing.T) {
	d := NewFeedDetector(nil)
	candidates := []FeedCandidate{}

	best := d.SelectBestFeed(candidates, "https://example.com")

	if best != nil {
		t.Error("候補が0件の場合はnilを返すべき")
	}
}

// TestSelectBestFeed_ComplexPriority は複雑な優先順位ケースをテストする。
// 同一ホストのAtom > 同一ホストのRSS > 他ホストのAtom > 他ホストのRSS
func TestSelectBestFeed_ComplexPriority(t *testing.T) {
	d := NewFeedDetector(nil)
	candidates := []FeedCandidate{
		{URL: "https://other.com/rss.xml", FeedType: FeedTypeRSS, Title: "Other RSS"},
		{URL: "https://other.com/atom.xml", FeedType: FeedTypeAtom, Title: "Other Atom"},
		{URL: "https://example.com/rss.xml", FeedType: FeedTypeRSS, Title: "Same RSS"},
		{URL: "https://example.com/atom.xml", FeedType: FeedTypeAtom, Title: "Same Atom"},
	}

	best := d.SelectBestFeed(candidates, "https://example.com")

	if best.URL != "https://example.com/atom.xml" {
		t.Errorf("同一ホストのAtomが最優先されるべき。期待: https://example.com/atom.xml, 結果: %s", best.URL)
	}
}

// --- DetectFeedURL（統合テスト）---

// TestDetectFeedURL_DirectRSSFeed はRSSフィードURLが直接入力された場合のテスト。
func TestDetectFeedURL_DirectRSSFeed(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
    <description>Test</description>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rssXML)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	feedURL, err := d.DetectFeedURL(context.Background(), server.URL+"/feed.xml")
	if err != nil {
		t.Fatalf("DetectFeedURL returned error: %v", err)
	}
	if feedURL != server.URL+"/feed.xml" {
		t.Errorf("期待URL: %s/feed.xml, 結果: %s", server.URL, feedURL)
	}
}

// TestDetectFeedURL_DirectAtomFeed はAtomフィードURLが直接入力された場合のテスト。
func TestDetectFeedURL_DirectAtomFeed(t *testing.T) {
	atomXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Test Atom Feed</title>
  <link href="https://example.com"/>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		fmt.Fprint(w, atomXML)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	feedURL, err := d.DetectFeedURL(context.Background(), server.URL+"/atom.xml")
	if err != nil {
		t.Fatalf("DetectFeedURL returned error: %v", err)
	}
	if feedURL != server.URL+"/atom.xml" {
		t.Errorf("期待URL: %s/atom.xml, 結果: %s", server.URL, feedURL)
	}
}

// TestDetectFeedURL_HTMLWithFeedLink はHTMLページにフィードリンクがある場合のテスト。
func TestDetectFeedURL_HTMLWithFeedLink(t *testing.T) {
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><head>
				<link rel="alternate" type="application/rss+xml" href="%s/feed.xml">
			</head><body></body></html>`, serverURL)
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	feedURL, err := d.DetectFeedURL(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatalf("DetectFeedURL returned error: %v", err)
	}
	if feedURL != server.URL+"/feed.xml" {
		t.Errorf("期待URL: %s/feed.xml, 結果: %s", server.URL, feedURL)
	}
}

// TestDetectFeedURL_HTMLWithRelativeFeedLink はHTMLページの相対パスフィードリンクを解決するテスト。
func TestDetectFeedURL_HTMLWithRelativeFeedLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blog":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><head>
				<link rel="alternate" type="application/rss+xml" href="/feed.xml">
			</head><body></body></html>`)
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
		}
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	feedURL, err := d.DetectFeedURL(context.Background(), server.URL+"/blog")
	if err != nil {
		t.Fatalf("DetectFeedURL returned error: %v", err)
	}
	if feedURL != server.URL+"/feed.xml" {
		t.Errorf("期待URL: %s/feed.xml, 結果: %s", server.URL, feedURL)
	}
}

// TestDetectFeedURL_HTMLNoFeedLink はHTMLページにフィードリンクがない場合にエラーを返すテスト。
func TestDetectFeedURL_HTMLNoFeedLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>No Feed</title></head><body></body></html>`)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	_, err := d.DetectFeedURL(context.Background(), server.URL+"/")
	if err == nil {
		t.Fatal("フィード未検出時はエラーを返すべき")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("APIError型が期待されるが、%T が返された", err)
	}
	if apiErr.Code != model.ErrCodeFeedNotDetected {
		t.Errorf("期待エラーコード: %s, 結果: %s", model.ErrCodeFeedNotDetected, apiErr.Code)
	}
	if apiErr.Category != "feed" {
		t.Errorf("期待カテゴリ: feed, 結果: %s", apiErr.Category)
	}
	if apiErr.Action == "" {
		t.Error("対処方法が空であるべきではない")
	}
}

// TestDetectFeedURL_SSRFBlocked はSSRF検証で拒否されるURLのテスト。
func TestDetectFeedURL_SSRFBlocked(t *testing.T) {
	guard := &mockSSRFGuard{blockAll: true}
	d := NewFeedDetector(guard)

	_, err := d.DetectFeedURL(context.Background(), "http://192.168.1.1/feed.xml")
	if err == nil {
		t.Fatal("SSRF検証でブロックされるURLはエラーを返すべき")
	}

	apiErr, ok := err.(*model.APIError)
	if !ok {
		t.Fatalf("APIError型が期待されるが、%T が返された", err)
	}
	if apiErr.Code != model.ErrCodeSSRFBlocked {
		t.Errorf("期待エラーコード: %s, 結果: %s", model.ErrCodeSSRFBlocked, apiErr.Code)
	}
}

// TestDetectFeedURL_EmptyURL は空URLがエラーを返すことをテストする。
func TestDetectFeedURL_EmptyURL(t *testing.T) {
	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	_, err := d.DetectFeedURL(context.Background(), "")
	if err == nil {
		t.Fatal("空URLはエラーを返すべき")
	}
}

// TestDetectFeedURL_XMLContentTypeWithRSSBody はContent-Type text/xmlでRSSボディの場合にフィードとして検出するテスト。
func TestDetectFeedURL_XMLContentTypeWithRSSBody(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprint(w, rssXML)
	}))
	defer server.Close()

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	feedURL, err := d.DetectFeedURL(context.Background(), server.URL+"/feed")
	if err != nil {
		t.Fatalf("DetectFeedURL returned error: %v", err)
	}
	if feedURL != server.URL+"/feed" {
		t.Errorf("期待URL: %s/feed, 結果: %s", server.URL, feedURL)
	}
}

// TestDetectFeedURL_HTMLWithMultipleFeedLinks_PrioritySelection はHTMLに複数フィードリンクがある場合の優先順位テスト。
func TestDetectFeedURL_HTMLWithMultipleFeedLinks_PrioritySelection(t *testing.T) {
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			// 同一ホストのAtomリンクが最優先
			fmt.Fprintf(w, `<html><head>
				<link rel="alternate" type="application/rss+xml" href="https://external.com/rss.xml">
				<link rel="alternate" type="application/rss+xml" href="%s/rss.xml">
				<link rel="alternate" type="application/atom+xml" href="%s/atom.xml">
			</head><body></body></html>`, serverURL, serverURL)
		case "/atom.xml":
			w.Header().Set("Content-Type", "application/atom+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Test</title></feed>`)
		case "/rss.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	guard := &mockSSRFGuard{}
	d := NewFeedDetector(guard)

	feedURL, err := d.DetectFeedURL(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatalf("DetectFeedURL returned error: %v", err)
	}
	// 同一ホストのAtomが最優先
	if feedURL != server.URL+"/atom.xml" {
		t.Errorf("同一ホストのAtomが優先されるべき。期待: %s/atom.xml, 結果: %s", server.URL, feedURL)
	}
}

// --- mockSSRFGuard ---

// mockSSRFGuard はテスト用のSSRFGuardモック。
type mockSSRFGuard struct {
	blockAll bool
}

func (m *mockSSRFGuard) NewSafeClient(timeout time.Duration, maxResponseSize int64) *http.Client {
	return &http.Client{Timeout: timeout}
}

func (m *mockSSRFGuard) ValidateURL(rawURL string) error {
	if m.blockAll {
		return fmt.Errorf("blocked by SSRF guard")
	}
	return nil
}
