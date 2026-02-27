// Package feed はフィード登録・管理のドメインロジックを提供する。
package feed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"golang.org/x/net/html"
)

// FeedType はフィードの種類（RSS/Atom）を表す。
type FeedType string

const (
	// FeedTypeRSS はRSSフィード。
	FeedTypeRSS FeedType = "rss"
	// FeedTypeAtom はAtomフィード。
	FeedTypeAtom FeedType = "atom"
)

// FeedCandidate はHTMLから検出されたフィード候補を表す。
type FeedCandidate struct {
	URL      string
	FeedType FeedType
	Title    string
}

// SSRFValidator はSSRF検証のインターフェース。
// security.SSRFGuardServiceを抽象化してテスタビリティを向上させる。
type SSRFValidator interface {
	ValidateURL(rawURL string) error
	NewSafeClient(timeout time.Duration, maxResponseSize int64) *http.Client
}

// FeedDetector はフィード自動検出機能を提供する。
type FeedDetector struct {
	ssrfGuard SSRFValidator
}

// NewFeedDetector はFeedDetectorの新しいインスタンスを生成する。
func NewFeedDetector(ssrfGuard SSRFValidator) *FeedDetector {
	return &FeedDetector{
		ssrfGuard: ssrfGuard,
	}
}

// feedContentTypes はフィードとして認識するContent-Typeのリスト。
var feedContentTypes = []string{
	"application/rss+xml",
	"application/atom+xml",
}

// xmlContentTypes はXMLとして認識するContent-Type（ボディ解析が必要）。
var xmlContentTypes = []string{
	"text/xml",
	"application/xml",
}

// IsDirectFeed はContent-Typeとボディを解析して、
// 指定されたレスポンスがRSS/Atomフィードかどうかを判定する。
func (d *FeedDetector) IsDirectFeed(contentType string, body []byte) bool {
	// Content-Typeからメディアタイプを抽出（charsetなどのパラメータを除去）
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	mediaType = strings.ToLower(mediaType)

	// RSS/Atom固有のContent-Typeの場合は直接判定
	for _, feedCT := range feedContentTypes {
		if mediaType == feedCT {
			return true
		}
	}

	// 汎用XML Content-Typeの場合はボディ解析が必要
	isXML := false
	for _, xmlCT := range xmlContentTypes {
		if mediaType == xmlCT {
			isXML = true
			break
		}
	}

	if !isXML || len(body) == 0 {
		return false
	}

	// ボディの先頭部分を解析してRSS/Atomか判定
	return isRSSOrAtomXML(body)
}

// isRSSOrAtomXML はXMLボディの先頭部分を解析してRSS/Atomフィードかを判定する。
func isRSSOrAtomXML(body []byte) bool {
	// 先頭4KBを検査（XMLプロローグ + ルート要素が含まれるのに十分）
	checkSize := 4096
	if len(body) < checkSize {
		checkSize = len(body)
	}
	prefix := strings.ToLower(string(body[:checkSize]))

	// RSSの判定: <rss タグまたは <rdf:RDF タグ
	if strings.Contains(prefix, "<rss") {
		return true
	}
	if strings.Contains(prefix, "<rdf:rdf") {
		return true
	}

	// Atomの判定: <feed タグ（Atom namespaceを含む）
	if strings.Contains(prefix, "<feed") && strings.Contains(prefix, "http://www.w3.org/2005/atom") {
		return true
	}

	return false
}

// ParseFeedLinksFromHTML はHTMLのheadタグからRSS/Atomフィードリンクを解析・検出する。
// 相対URLはbaseURLを基準に絶対URLに解決される。
func (d *FeedDetector) ParseFeedLinksFromHTML(htmlBody []byte, baseURL string) []FeedCandidate {
	var candidates []FeedCandidate

	baseU, err := url.Parse(baseURL)
	if err != nil {
		return candidates
	}

	tokenizer := html.NewTokenizer(bytes.NewReader(htmlBody))
	inHead := false

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return candidates

		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := tokenizer.TagName()
			tagName := string(tn)

			if tagName == "head" {
				inHead = true
				continue
			}

			if tagName == "body" {
				// bodyに入ったらheadの解析を終了
				return candidates
			}

			if !inHead || tagName != "link" || !hasAttr {
				continue
			}

			// link要素の属性を解析
			var rel, linkType, href, title string
			for {
				key, val, more := tokenizer.TagAttr()
				k := strings.ToLower(string(key))
				v := string(val)
				switch k {
				case "rel":
					rel = strings.ToLower(v)
				case "type":
					linkType = strings.ToLower(v)
				case "href":
					href = v
				case "title":
					title = v
				}
				if !more {
					break
				}
			}

			// rel="alternate" かつ RSS/Atom Content-Type のリンクのみ対象
			if rel != "alternate" || href == "" {
				continue
			}

			var feedType FeedType
			switch linkType {
			case "application/rss+xml":
				feedType = FeedTypeRSS
			case "application/atom+xml":
				feedType = FeedTypeAtom
			default:
				continue
			}

			// 相対URLを絶対URLに解決
			resolvedURL := resolveURL(baseU, href)
			if resolvedURL == "" {
				continue
			}

			candidates = append(candidates, FeedCandidate{
				URL:      resolvedURL,
				FeedType: feedType,
				Title:    title,
			})

		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			if string(tn) == "head" {
				return candidates
			}
		}
	}
}

// resolveURL は相対URLをベースURLを基準に絶対URLに解決する。
func resolveURL(base *url.URL, rawRef string) string {
	ref, err := url.Parse(rawRef)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// SelectBestFeed は複数のフィード候補から優先順位に従って最適なフィードを選択する。
// 優先順位: 同一ホスト > Atom > RSS > 先頭
func (d *FeedDetector) SelectBestFeed(candidates []FeedCandidate, inputURL string) *FeedCandidate {
	if len(candidates) == 0 {
		return nil
	}

	inputHost := extractHost(inputURL)

	// スコアリング: 同一ホスト(+100) + Atom(+10) + 先頭優先（インデックスが小さいほど高スコア）
	bestIdx := 0
	bestScore := -1

	for i, c := range candidates {
		score := 0

		// 同一ホスト判定
		candidateHost := extractHost(c.URL)
		if candidateHost == inputHost {
			score += 100
		}

		// フィードタイプ優先度
		if c.FeedType == FeedTypeAtom {
			score += 10
		}

		// 先頭優先（同スコアの場合はインデックスが小さい方を優先するため、
		// > ではなく >= にしない）
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return &candidates[bestIdx]
}

// extractHost はURLからホスト名を抽出する。
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// DetectFeedURL はURLがフィードかHTMLかを判定し、フィードURLを返す。
// 1. SSRF検証を実行
// 2. URLにHTTPリクエストを送信
// 3. Content-Typeとボディからフィードかどうかを判定
// 4. HTMLの場合はheadタグからフィードリンクを検出し、優先順位で選択
// 5. フィード未検出の場合はエラー（原因カテゴリ + 対処方法）を返す
func (d *FeedDetector) DetectFeedURL(ctx context.Context, inputURL string) (string, error) {
	// 空URLチェック
	if inputURL == "" {
		return "", model.NewInvalidURLError("URLが入力されていません")
	}

	// SSRF検証
	if d.ssrfGuard != nil {
		if err := d.ssrfGuard.ValidateURL(inputURL); err != nil {
			return "", model.NewSSRFBlockedError()
		}
	}

	// HTTPリクエスト送信
	client := d.getHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inputURL, nil)
	if err != nil {
		return "", model.NewInvalidURLError(err.Error())
	}
	req.Header.Set("User-Agent", "Feedman/1.0 RSS Reader")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, text/html, */*")

	resp, err := client.Do(req)
	if err != nil {
		return "", model.NewFetchFailedError(err.Error())
	}
	defer resp.Body.Close()

	// レスポンスボディを読み込み（最大5MB）
	const maxBodySize = 5 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return "", model.NewFetchFailedError(fmt.Sprintf("レスポンスの読み取りに失敗: %v", err))
	}

	contentType := resp.Header.Get("Content-Type")

	// フィード直接判定
	if d.IsDirectFeed(contentType, body) {
		return inputURL, nil
	}

	// HTMLの場合: headタグからフィードリンクを検出
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if !strings.Contains(strings.ToLower(mediaType), "html") {
		// HTMLでもフィードでもない場合
		return "", model.NewFeedNotDetectedError(inputURL)
	}

	// HTMLからフィードリンクを検出
	candidates := d.ParseFeedLinksFromHTML(body, inputURL)
	if len(candidates) == 0 {
		return "", model.NewFeedNotDetectedError(inputURL)
	}

	// 優先順位に従って最適なフィードを選択
	best := d.SelectBestFeed(candidates, inputURL)
	if best == nil {
		return "", model.NewFeedNotDetectedError(inputURL)
	}

	return best.URL, nil
}

// getHTTPClient はHTTPクライアントを取得する。
// SSRFGuardが設定されている場合はSSRF防止付きクライアントを返す。
func (d *FeedDetector) getHTTPClient() *http.Client {
	if d.ssrfGuard != nil {
		return d.ssrfGuard.NewSafeClient(10*time.Second, 5*1024*1024)
	}
	return &http.Client{Timeout: 10 * time.Second}
}
