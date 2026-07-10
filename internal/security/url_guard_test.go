package security

import "testing"

// TestSafeExternalURL は許可スキーム（http/https）のみを通し、危険・相対・不正な
// URL を空文字列へ無害化することを検証する。
func TestSafeExternalURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// 正常系（http/https を維持）
		{name: "https を許可する", in: "https://example.com/article", want: "https://example.com/article"},
		{name: "http を許可する", in: "http://example.com/article", want: "http://example.com/article"},
		{name: "大文字スキームも許可する", in: "HTTPS://example.com", want: "HTTPS://example.com"},
		{name: "クエリ付き URL を維持する", in: "https://example.com/a?b=c&d=e", want: "https://example.com/a?b=c&d=e"},
		{name: "前後の空白を除去して維持する", in: "  https://example.com  ", want: "https://example.com"},

		// 異常系（危険スキームを無害化）
		{name: "javascript スキームを拒否する", in: "javascript:alert(1)", want: ""},
		{name: "大文字混在の javascript を拒否する", in: "JaVaScRiPt:alert(1)", want: ""},
		{name: "先頭空白付き javascript を拒否する", in: "   javascript:alert(1)", want: ""},
		{name: "data スキームを拒否する", in: "data:text/html,<script>alert(1)</script>", want: ""},
		{name: "vbscript スキームを拒否する", in: "vbscript:msgbox(1)", want: ""},
		{name: "file スキームを拒否する", in: "file:///etc/passwd", want: ""},

		// 境界値（相対・空・プロトコル相対）
		{name: "相対 URL を拒否する", in: "/path/to/article", want: ""},
		{name: "プロトコル相対 URL を拒否する", in: "//evil.example.com", want: ""},
		{name: "空文字列は空文字列を返す", in: "", want: ""},
		{name: "空白のみは空文字列を返す", in: "   ", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SafeExternalURL(tc.in)
			if got != tc.want {
				t.Errorf("SafeExternalURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
