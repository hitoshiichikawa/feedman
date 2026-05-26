package auth

import "strings"

// maskedLocalSuffix はローカル部の伏字に使う固定文字列。
// 元のローカル部の長さを推測されないよう固定長とする。
const maskedLocalSuffix = "***"

// maskEmail はログ出力用にメールアドレスをマスクする。
// ローカル部（最初の @ より前）の先頭1文字のみを残し、残余を固定の伏字 "***" に置換する。
// ドメイン部（最初の @ 以降）はそのまま保持する（例: "hitoshi@example.com" -> "h***@example.com"）。
//
// 元の値を復元できないようにするため、以下の安全側処理を行う:
//   - ローカル部が1文字以下の場合は先頭文字も露出させず、伏字のみを出力する
//   - 空文字や @ を含まない不正形式の場合は、ドメインを伴わない固定マスク値 "***" を返す
//     （復元可能な平文を一切出力しない）
//
// 本関数はパニックせず、いかなる入力でも安全にマスク値を返す。
func maskEmail(email string) string {
	atIndex := strings.IndexByte(email, '@')
	// @ を含まない（空文字を含む）不正形式は復元可能な平文を出さず固定マスクを返す。
	if atIndex < 0 {
		return maskedLocalSuffix
	}

	local := email[:atIndex]
	domain := email[atIndex:] // "@" 以降をそのまま保持

	// ローカル部が2文字以上の場合のみ先頭1文字を残す。
	// 1文字以下では先頭文字＝ローカル部全体となり平文相当が漏れるため、先頭も伏せる。
	if len(local) >= 2 {
		return local[:1] + maskedLocalSuffix + domain
	}
	return maskedLocalSuffix + domain
}
