// Package security はアプリケーションのセキュリティ機能を提供する。
//
// ContentSanitizerService はフィード記事のHTMLコンテンツをサニタイズし、
// XSS攻撃などのセキュリティリスクからユーザーを保護する。
// bluemondayライブラリを使用した許可リストベースのポリシーで、
// 安全なタグと属性のみを通過させる。
package security

import (
	"net/url"

	"github.com/microcosm-cc/bluemonday"
)

// ContentSanitizerService はHTMLコンテンツのサニタイズ機能のインターフェースを定義する。
// フィード記事のコンテンツ保存前およびAPI応答時に使用される。
type ContentSanitizerService interface {
	// Sanitize はHTMLコンテンツをサニタイズして安全なHTMLを返す。
	// 許可タグ（p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img）のみを通過させ、
	// script, iframe, styleタグおよびon*イベント属性を除去する。
	// imgタグのsrc属性はhttpsスキームのみ許可される。
	// aタグにはtarget="_blank"とrel="noopener noreferrer"が自動付与される。
	// 空文字列の入力には空文字列を返す。
	// 同一入力に対して常に同一出力を返す（冪等）。
	Sanitize(rawHTML string) string
}

// contentSanitizer はContentSanitizerServiceの実装。
// bluemondayのポリシーを保持し、スレッドセーフにサニタイズ処理を行う。
type contentSanitizer struct {
	policy *bluemonday.Policy
}

// NewContentSanitizer はContentSanitizerServiceの新しいインスタンスを生成する。
// 初期化時にbluemondayのカスタムポリシーを構築する。
// ポリシーの内容:
//   - 許可タグ: p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img
//   - 禁止タグ: script, iframe, style および全てのon*イベント属性
//   - imgのsrc属性: httpsスキームのみ許可
//   - aタグ: target="_blank" と rel="noopener noreferrer" を自動付与
func NewContentSanitizer() *contentSanitizer {
	p := bluemonday.NewPolicy()

	// 許可タグの設定（属性なしのシンプルなタグ）
	// 要件11.1: p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img
	// script, iframe, style等は許可リストに含めないことで自動的に除去される（要件11.2）
	// on*イベント属性はbluemondayのデフォルトで許可されないため除去される（要件11.2）
	p.AllowElements(
		"p", "br", "ul", "ol", "li",
		"blockquote", "pre", "code",
		"strong", "em",
	)

	// aタグの設定（要件11.4）:
	// - href属性を許可
	// - 相対URLは不許可（フィードコンテンツには不適切）
	// - target="_blank"を全リンクに強制付与
	// - rel="noreferrer noopener"を強制付与
	p.AllowAttrs("href").OnElements("a")
	p.AllowRelativeURLs(false)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnLinks(true)

	// imgタグの設定（要件11.3）:
	// - src属性はhttpsスキームのみ許可（http, javascript, data等は拒否）
	// - alt属性を許可（アクセシビリティ確保）
	p.AllowAttrs("src").OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowURLSchemeWithCustomPolicy("https", func(u *url.URL) bool {
		return true
	})

	return &contentSanitizer{
		policy: p,
	}
}

// Sanitize はHTMLコンテンツをサニタイズして安全なHTMLを返す。
func (s *contentSanitizer) Sanitize(rawHTML string) string {
	return s.policy.Sanitize(rawHTML)
}
