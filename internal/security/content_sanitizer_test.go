package security

import (
	"strings"
	"testing"
)

// TestSanitize_AllowedTags は許可タグが正しく通過することを検証する。
func TestSanitize_AllowedTags(t *testing.T) {
	sanitizer := NewContentSanitizer()

	tests := []struct {
		name  string
		input string
		// want に含まれるべき部分文字列
		wantContains []string
	}{
		{
			name:         "pタグが許可される",
			input:        "<p>テスト段落</p>",
			wantContains: []string{"<p>テスト段落</p>"},
		},
		{
			name:         "brタグが許可される",
			input:        "行1<br>行2",
			wantContains: []string{"<br>", "行1", "行2"},
		},
		{
			name:         "brタグ（自己閉じ）が許可される",
			input:        "行1<br/>行2",
			wantContains: []string{"行1", "行2"},
		},
		{
			name:         "aタグが許可される",
			input:        `<a href="https://example.com">リンク</a>`,
			wantContains: []string{"<a", "href", "https://example.com", "リンク", "</a>"},
		},
		{
			name:         "ulタグとliタグが許可される",
			input:        "<ul><li>項目1</li><li>項目2</li></ul>",
			wantContains: []string{"<ul>", "<li>", "項目1", "項目2", "</li>", "</ul>"},
		},
		{
			name:         "olタグとliタグが許可される",
			input:        "<ol><li>項目1</li><li>項目2</li></ol>",
			wantContains: []string{"<ol>", "<li>", "項目1", "項目2", "</li>", "</ol>"},
		},
		{
			name:         "blockquoteタグが許可される",
			input:        "<blockquote>引用テキスト</blockquote>",
			wantContains: []string{"<blockquote>引用テキスト</blockquote>"},
		},
		{
			name:         "preタグとcodeタグが許可される",
			input:        "<pre><code>func main() {}</code></pre>",
			wantContains: []string{"<pre>", "<code>", "func main() {}", "</code>", "</pre>"},
		},
		{
			name:         "strongタグが許可される",
			input:        "<strong>太字テキスト</strong>",
			wantContains: []string{"<strong>太字テキスト</strong>"},
		},
		{
			name:         "emタグが許可される",
			input:        "<em>強調テキスト</em>",
			wantContains: []string{"<em>強調テキスト</em>"},
		},
		{
			name:         "imgタグがhttps srcで許可される",
			input:        `<img src="https://example.com/image.png" alt="画像">`,
			wantContains: []string{"<img", "src", "https://example.com/image.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.Sanitize(tt.input)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Sanitize(%q) = %q, expected to contain %q", tt.input, got, want)
				}
			}
		})
	}
}

// TestSanitize_ForbiddenTags は禁止タグが除去されることを検証する。
func TestSanitize_ForbiddenTags(t *testing.T) {
	sanitizer := NewContentSanitizer()

	tests := []struct {
		name          string
		input         string
		wantAbsent    []string
		wantContains  []string
	}{
		{
			name:       "scriptタグが除去される",
			input:      `<p>テスト</p><script>alert('xss')</script><p>安全</p>`,
			wantAbsent: []string{"<script", "</script>", "alert"},
			wantContains: []string{"テスト", "安全"},
		},
		{
			name:       "iframeタグが除去される",
			input:      `<p>テスト</p><iframe src="https://evil.com"></iframe>`,
			wantAbsent: []string{"<iframe", "</iframe>", "evil.com"},
			wantContains: []string{"テスト"},
		},
		{
			name:       "styleタグが除去される",
			input:      `<p>テスト</p><style>body{display:none}</style>`,
			wantAbsent: []string{"<style", "</style>", "display:none"},
			wantContains: []string{"テスト"},
		},
		{
			name:       "許可されていないタグ（div）が除去される",
			input:      `<div><p>テスト</p></div>`,
			wantAbsent: []string{"<div", "</div>"},
			wantContains: []string{"<p>テスト</p>"},
		},
		{
			name:       "許可されていないタグ（span）が除去される",
			input:      `<span>テスト</span>`,
			wantAbsent: []string{"<span", "</span>"},
			wantContains: []string{"テスト"},
		},
		{
			name:       "許可されていないタグ（form）が除去される",
			input:      `<form action="https://evil.com"><input type="text"></form>`,
			wantAbsent: []string{"<form", "</form>", "<input"},
		},
		{
			name:       "objectタグが除去される",
			input:      `<object data="https://evil.com/flash.swf"></object>`,
			wantAbsent: []string{"<object", "</object>", "flash.swf"},
		},
		{
			name:       "embedタグが除去される",
			input:      `<embed src="https://evil.com/plugin">`,
			wantAbsent: []string{"<embed", "plugin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.Sanitize(tt.input)
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Sanitize(%q) = %q, should NOT contain %q", tt.input, got, absent)
				}
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Sanitize(%q) = %q, expected to contain %q", tt.input, got, want)
				}
			}
		})
	}
}

// TestSanitize_OnEventAttributes はon*イベント属性が除去されることを検証する。
func TestSanitize_OnEventAttributes(t *testing.T) {
	sanitizer := NewContentSanitizer()

	tests := []struct {
		name       string
		input      string
		wantAbsent []string
	}{
		{
			name:       "onclickが除去される",
			input:      `<p onclick="alert('xss')">テスト</p>`,
			wantAbsent: []string{"onclick", "alert"},
		},
		{
			name:       "onloadが除去される",
			input:      `<img src="https://example.com/img.png" onload="alert('xss')">`,
			wantAbsent: []string{"onload", "alert"},
		},
		{
			name:       "onerrorが除去される",
			input:      `<img src="https://example.com/img.png" onerror="alert('xss')">`,
			wantAbsent: []string{"onerror", "alert"},
		},
		{
			name:       "onmouseoverが除去される",
			input:      `<a href="https://example.com" onmouseover="alert('xss')">リンク</a>`,
			wantAbsent: []string{"onmouseover", "alert"},
		},
		{
			name:       "onfocusが除去される",
			input:      `<a href="https://example.com" onfocus="alert('xss')">リンク</a>`,
			wantAbsent: []string{"onfocus", "alert"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.Sanitize(tt.input)
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Sanitize(%q) = %q, should NOT contain %q", tt.input, got, absent)
				}
			}
		})
	}
}

// TestSanitize_ImgHTTPSOnly はimgタグのsrc属性がhttpsスキームのみ許可されることを検証する。
func TestSanitize_ImgHTTPSOnly(t *testing.T) {
	sanitizer := NewContentSanitizer()

	tests := []struct {
		name         string
		input        string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "https imgが許可される",
			input:        `<img src="https://example.com/image.png" alt="安全な画像">`,
			wantContains: []string{"<img", "https://example.com/image.png"},
		},
		{
			name:       "http imgが拒否される",
			input:      `<img src="http://example.com/image.png" alt="危険な画像">`,
			wantAbsent: []string{"http://example.com/image.png"},
		},
		{
			name:       "javascript imgが拒否される",
			input:      `<img src="javascript:alert('xss')" alt="XSS">`,
			wantAbsent: []string{"javascript:", "alert"},
		},
		{
			name:       "data URI imgが拒否される",
			input:      `<img src="data:image/png;base64,abc" alt="データ">`,
			wantAbsent: []string{"data:image"},
		},
		{
			name:       "ftp imgが拒否される",
			input:      `<img src="ftp://example.com/image.png" alt="FTP">`,
			wantAbsent: []string{"ftp://"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.Sanitize(tt.input)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Sanitize(%q) = %q, expected to contain %q", tt.input, got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Sanitize(%q) = %q, should NOT contain %q", tt.input, got, absent)
				}
			}
		})
	}
}

// TestSanitize_AnchorAttributes はaタグにtarget="_blank"とrel="noopener noreferrer"が自動付与されることを検証する。
func TestSanitize_AnchorAttributes(t *testing.T) {
	sanitizer := NewContentSanitizer()

	tests := []struct {
		name         string
		input        string
		wantContains []string
	}{
		{
			name:  "aタグにtarget=_blankが付与される",
			input: `<a href="https://example.com">リンク</a>`,
			wantContains: []string{
				`target="_blank"`,
				"https://example.com",
				"リンク",
			},
		},
		{
			name:  "aタグにrel=noopener noreferrerが付与される",
			input: `<a href="https://example.com">リンク</a>`,
			wantContains: []string{
				"noopener",
				"noreferrer",
			},
		},
		{
			name:  "既存のtargetが上書きされる",
			input: `<a href="https://example.com" target="_self">リンク</a>`,
			wantContains: []string{
				`target="_blank"`,
			},
		},
		{
			name:  "既存のrelが上書きされる",
			input: `<a href="https://example.com" rel="nofollow">リンク</a>`,
			wantContains: []string{
				"noopener",
				"noreferrer",
			},
		},
		{
			name:  "href属性のないaタグも安全に処理される",
			input: `<a>テキストリンク</a>`,
			wantContains: []string{
				"テキストリンク",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.Sanitize(tt.input)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Sanitize(%q) = %q, expected to contain %q", tt.input, got, want)
				}
			}
		})
	}
}

// TestSanitize_AnchorNoTargetSelf はtarget="_self"が残らないことを検証する。
func TestSanitize_AnchorNoTargetSelf(t *testing.T) {
	sanitizer := NewContentSanitizer()

	input := `<a href="https://example.com" target="_self">リンク</a>`
	got := sanitizer.Sanitize(input)

	if strings.Contains(got, `target="_self"`) {
		t.Errorf("Sanitize(%q) = %q, should NOT contain target=\"_self\"", input, got)
	}
}

// TestSanitize_EmptyInput は空文字列の入力を安全に処理できることを検証する。
func TestSanitize_EmptyInput(t *testing.T) {
	sanitizer := NewContentSanitizer()

	got := sanitizer.Sanitize("")
	if got != "" {
		t.Errorf("Sanitize(\"\") = %q, expected empty string", got)
	}
}

// TestSanitize_PlainText はプレーンテキストがそのまま通過することを検証する。
func TestSanitize_PlainText(t *testing.T) {
	sanitizer := NewContentSanitizer()

	input := "これはプレーンテキストです。HTMLタグを含みません。"
	got := sanitizer.Sanitize(input)
	if got != input {
		t.Errorf("Sanitize(%q) = %q, expected unchanged", input, got)
	}
}

// TestSanitize_Idempotent は同一入力に対して常に同一出力（冪等性）を検証する。
func TestSanitize_Idempotent(t *testing.T) {
	sanitizer := NewContentSanitizer()

	input := `<p>テスト<strong>太字</strong></p><a href="https://example.com">リンク</a><img src="https://example.com/img.png" alt="画像">`

	result1 := sanitizer.Sanitize(input)
	result2 := sanitizer.Sanitize(input)
	result3 := sanitizer.Sanitize(result1) // 二重サニタイズ

	if result1 != result2 {
		t.Errorf("冪等性違反: 1回目=%q, 2回目=%q", result1, result2)
	}
	if result1 != result3 {
		t.Errorf("二重サニタイズで結果が変わった: 1回目=%q, 二重=%q", result1, result3)
	}
}

// TestSanitize_ComplexHTML は複合的なHTMLコンテンツのサニタイズを検証する。
func TestSanitize_ComplexHTML(t *testing.T) {
	sanitizer := NewContentSanitizer()

	input := `<div class="article">
<h1>タイトル</h1>
<p>これは<strong>重要な</strong>記事です。</p>
<script>document.cookie</script>
<ul>
<li>項目1</li>
<li>項目2</li>
</ul>
<img src="https://example.com/photo.jpg" alt="写真" onerror="alert('xss')">
<a href="https://example.com" onclick="steal()">元記事</a>
<iframe src="https://evil.com"></iframe>
<style>.hidden{display:none}</style>
<blockquote>引用テキスト</blockquote>
<pre><code>fmt.Println("Hello")</code></pre>
</div>`

	got := sanitizer.Sanitize(input)

	// 許可タグが存在すること
	allowedParts := []string{
		"<p>", "</p>",
		"<strong>", "</strong>",
		"<ul>", "</ul>",
		"<li>", "</li>",
		"<blockquote>", "</blockquote>",
		"<pre>", "</pre>",
		"<code>", "</code>",
		"https://example.com/photo.jpg",
		"元記事",
		"引用テキスト",
		"fmt.Println(", // bluemondayはダブルクォートを&#34;にエンコードするためパーシャルマッチ
	}
	for _, part := range allowedParts {
		if !strings.Contains(got, part) {
			t.Errorf("結果に %q が含まれていない: %q", part, got)
		}
	}

	// 禁止要素が除去されていること
	forbiddenParts := []string{
		"<script", "</script>",
		"<iframe", "</iframe>",
		"<style", "</style>",
		"<div", "</div>",
		"<h1", "</h1>",
		"onclick",
		"onerror",
		"document.cookie",
		"steal()",
		"display:none",
		"evil.com",
	}
	for _, part := range forbiddenParts {
		if strings.Contains(got, part) {
			t.Errorf("結果に禁止要素 %q が含まれている: %q", part, got)
		}
	}

	// aタグにtarget/_blankとrelが付与されていること
	if !strings.Contains(got, `target="_blank"`) {
		t.Errorf("aタグにtarget=\"_blank\"が付与されていない: %q", got)
	}
	if !strings.Contains(got, "noopener") {
		t.Errorf("aタグにnoopenerが付与されていない: %q", got)
	}
	if !strings.Contains(got, "noreferrer") {
		t.Errorf("aタグにnoreferrerが付与されていない: %q", got)
	}
}

// TestSanitize_XSSPayloads は典型的なXSSペイロードが無害化されることを検証する。
func TestSanitize_XSSPayloads(t *testing.T) {
	sanitizer := NewContentSanitizer()

	tests := []struct {
		name       string
		input      string
		wantAbsent []string
	}{
		{
			name:       "SVG onloadによるXSS",
			input:      `<svg onload="alert('xss')">`,
			wantAbsent: []string{"<svg", "onload", "alert"},
		},
		{
			name:       "img onerrorによるXSS",
			input:      `<img src="x" onerror="alert('xss')">`,
			wantAbsent: []string{"onerror", "alert"},
		},
		{
			name:       "javascript URI",
			input:      `<a href="javascript:alert('xss')">クリック</a>`,
			wantAbsent: []string{"javascript:"},
		},
		{
			name:       "data URIでのスクリプト",
			input:      `<a href="data:text/html,<script>alert('xss')</script>">データ</a>`,
			wantAbsent: []string{"data:text/html"},
		},
		{
			name:       "style属性によるXSS",
			input:      `<p style="background:url(javascript:alert('xss'))">テスト</p>`,
			wantAbsent: []string{"style=", "background:", "javascript:"},
		},
		{
			name:       "イベントハンドラの大文字混在",
			input:      `<p OnClick="alert('xss')">テスト</p>`,
			wantAbsent: []string{"OnClick", "onclick", "alert"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizer.Sanitize(tt.input)
			for _, absent := range tt.wantAbsent {
				if strings.Contains(strings.ToLower(got), strings.ToLower(absent)) {
					t.Errorf("Sanitize(%q) = %q, should NOT contain %q (case-insensitive)", tt.input, got, absent)
				}
			}
		})
	}
}

// TestSanitize_ImgAltAttribute はimgタグのalt属性が保持されることを検証する。
func TestSanitize_ImgAltAttribute(t *testing.T) {
	sanitizer := NewContentSanitizer()

	input := `<img src="https://example.com/photo.jpg" alt="説明テキスト">`
	got := sanitizer.Sanitize(input)

	if !strings.Contains(got, `alt="説明テキスト"`) {
		t.Errorf("Sanitize(%q) = %q, expected alt attribute to be preserved", input, got)
	}
}

// TestContentSanitizerInterface はContentSanitizerServiceインターフェースの適合を検証する。
func TestContentSanitizerInterface(t *testing.T) {
	var _ ContentSanitizerService = NewContentSanitizer()
}
