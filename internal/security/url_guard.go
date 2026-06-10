package security

import (
	"net/url"
	"strings"
)

// SafeExternalURL は外部リンク（フィード記事の link 等）として安全に href へ出力できる
// URL のみを通すための scheme 検証を行う。
//
// 許可するのは絶対 URL の http / https スキームのみ（大文字小文字は無視）。それ以外
// （javascript: / data: / vbscript: / file: などの実行可能・危険スキーム、相対 URL、
// プロトコル相対 URL `//host`、パースできない値、空文字列）は空文字列を返す。
//
// フィード由来の URL は攻撃者が制御しうるため、bluemonday を通る記事本文 HTML とは別に、
// 記事自身の link（gofeed が <link> から抽出する値）に対してもこの検証を適用し、
// href に javascript: 等が出力される保存型 XSS を防ぐ。前後の空白は除去して返す。
func SafeExternalURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return trimmed
	default:
		return ""
	}
}
