// Package passkey はパスキー（WebAuthn）認証のドメインロジックを提供する。
//
// 本ファイルは username の形式検証と正規化を担う純粋関数群を提供する
// （UsernameValidator 境界、Issue #216 / Req 1.4, 1.5）。
package passkey

import "errors"

// usernameMinLength は username の許容最小長（文字数 = byte 数、ASCII 前提）である。
const usernameMinLength = 3

// usernameMaxLength は username の許容最大長（文字数 = byte 数、ASCII 前提）である。
const usernameMaxLength = 32

// ErrInvalidUsername は username の形式が要件（文字種 / 長さ）を満たさないことを示す
// sentinel エラーである。errors.Is で判定する。
var ErrInvalidUsername = errors.New("username format invalid")

// ValidateAndNormalize は username を検証し、canonical 形式（lowercase）に正規化した
// 文字列を返す。
//
// 検証ルール:
//   - 文字種: ASCII の [a-zA-Z0-9_-] のみ（Unicode / 空白 / 記号 / 絵文字 / 制御文字は不正）
//   - 長さ: 3 文字以上 32 文字以下
//
// 正規化ルール:
//   - lowercase 変換のみ（trim なし・NFKC なし）
//
// 検証に失敗した場合は空文字と ErrInvalidUsername を返す。副作用は無く、外部依存も
// 持たない純粋関数として実装している。長さ判定は許容文字が全て 1 byte の ASCII で
// あるため byte 長で十分。
func ValidateAndNormalize(raw string) (string, error) {
	if len(raw) < usernameMinLength || len(raw) > usernameMaxLength {
		return "", ErrInvalidUsername
	}

	// 許容 byte を lowercase に変換しながら組み立てる。
	// 文字種違反を発見した時点で ErrInvalidUsername を返す。
	buf := make([]byte, len(raw))
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch {
		case b >= 'a' && b <= 'z':
			buf[i] = b
		case b >= 'A' && b <= 'Z':
			buf[i] = b + ('a' - 'A')
		case b >= '0' && b <= '9':
			buf[i] = b
		case b == '_' || b == '-':
			buf[i] = b
		default:
			return "", ErrInvalidUsername
		}
	}

	return string(buf), nil
}
