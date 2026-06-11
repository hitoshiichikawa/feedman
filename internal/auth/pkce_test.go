package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// validS256Challenge は SHA-256 の base64url（no-pad）表現と同じ 43 文字の有効値。
const validS256Challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

// TestValidatePKCES256 は method の厳密一致と challenge の S256 形式検証を網羅する。
func TestValidatePKCES256(t *testing.T) {
	cases := []struct {
		name      string
		challenge string
		method    string
		wantErr   bool
	}{
		// 正常系
		{name: "S256 と 43 文字 base64url のとき受理する", challenge: validS256Challenge, method: "S256", wantErr: false},
		{name: "ハイフン・アンダースコアを含む challenge を受理する", challenge: strings.Repeat("-_", 21) + "a", method: "S256", wantErr: false},

		// 異常系: method
		{name: "method 欠落のとき拒否する", challenge: validS256Challenge, method: "", wantErr: true},
		{name: "method=plain のとき拒否する", challenge: validS256Challenge, method: "plain", wantErr: true},
		{name: "method が小文字 s256 のとき拒否する", challenge: validS256Challenge, method: "s256", wantErr: true},

		// 異常系: challenge
		{name: "challenge 欠落のとき拒否する", challenge: "", method: "S256", wantErr: true},
		{name: "challenge が 42 文字のとき拒否する", challenge: validS256Challenge[:42], method: "S256", wantErr: true},
		{name: "challenge が 44 文字のとき拒否する", challenge: validS256Challenge + "a", method: "S256", wantErr: true},
		{name: "標準 base64 の + を含むとき拒否する", challenge: strings.Replace(validS256Challenge, "-", "+", 1), method: "S256", wantErr: true},
		{name: "標準 base64 の / を含むとき拒否する", challenge: strings.Replace(validS256Challenge, "-", "/", 1), method: "S256", wantErr: true},
		{name: "padding の = を含むとき拒否する", challenge: validS256Challenge[:42] + "=", method: "S256", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePKCES256(tc.challenge, tc.method)
			if tc.wantErr && err == nil {
				t.Errorf("ValidatePKCES256(%q, %q) = nil, want error", tc.challenge, tc.method)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidatePKCES256(%q, %q) = %v, want nil", tc.challenge, tc.method, err)
			}
		})
	}
}

// rfc7636AppendixBVerifier は RFC 7636 Appendix B の S256 test vector の verifier。
// 対応する challenge = SHA-256 → base64url(no-pad) = validS256Challenge と等しい。
const rfc7636AppendixBVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

// TestVerifyPKCES256Verifier は POST /api/auth/token の交換時に行う verifier 検証を
// 網羅する（Issue #166 / Req 2.1, 2.4, NFR 1.4）。RFC 7636 Appendix B の test vector
// を含めることで実装が S256 の仕様（base64url SHA-256 of verifier）に合致することを担保する。
func TestVerifyPKCES256Verifier(t *testing.T) {
	cases := []struct {
		name      string
		verifier  string
		stored    string
		wantMatch bool
	}{
		// 正常系（RFC 7636 Appendix B test vector を使用）
		{
			name:      "RFC 7636 Appendix B の verifier と一致する challenge のとき true",
			verifier:  rfc7636AppendixBVerifier,
			stored:    validS256Challenge,
			wantMatch: true,
		},
		{
			name:      "43 文字最小長の verifier と SHA-256 派生 challenge のとき true",
			verifier:  strings.Repeat("a", 43),
			stored:    sha256ChallengeOf(strings.Repeat("a", 43)),
			wantMatch: true,
		},
		{
			name:      "128 文字最大長の verifier と SHA-256 派生 challenge のとき true",
			verifier:  strings.Repeat("a", 128),
			stored:    sha256ChallengeOf(strings.Repeat("a", 128)),
			wantMatch: true,
		},
		{
			name:      "unreserved 全種（. _ ~ -）を含む verifier を受理する",
			verifier:  strings.Repeat("aA0._~-", 7), // 49 文字
			stored:    sha256ChallengeOf(strings.Repeat("aA0._~-", 7)),
			wantMatch: true,
		},

		// 異常系: 不一致
		{
			name:      "verifier が一致しない別 challenge のとき false",
			verifier:  rfc7636AppendixBVerifier,
			stored:    strings.Repeat("a", 43),
			wantMatch: false,
		},
		{
			name:      "verifier の末尾 1 文字を改変したとき false",
			verifier:  rfc7636AppendixBVerifier[:len(rfc7636AppendixBVerifier)-1] + "X",
			stored:    validS256Challenge,
			wantMatch: false,
		},
		{
			name:      "stored challenge が空文字のとき false",
			verifier:  rfc7636AppendixBVerifier,
			stored:    "",
			wantMatch: false,
		},

		// 異常系: 形式不正（境界値）
		{
			name:      "verifier が 42 文字のとき false (RFC 7636 §4.1 最小 43)",
			verifier:  strings.Repeat("a", 42),
			stored:    validS256Challenge,
			wantMatch: false,
		},
		{
			name:      "verifier が 129 文字のとき false (RFC 7636 §4.1 最大 128)",
			verifier:  strings.Repeat("a", 129),
			stored:    validS256Challenge,
			wantMatch: false,
		},
		{
			name:      "verifier が空のとき false",
			verifier:  "",
			stored:    validS256Challenge,
			wantMatch: false,
		},

		// 異常系: 形式不正（不正文字）
		{
			name:      "verifier に空白を含むとき false",
			verifier:  strings.Repeat("a", 42) + " ",
			stored:    validS256Challenge,
			wantMatch: false,
		},
		{
			name:      "verifier に + を含むとき false (unreserved 外)",
			verifier:  strings.Repeat("a", 42) + "+",
			stored:    validS256Challenge,
			wantMatch: false,
		},
		{
			name:      "verifier に / を含むとき false (unreserved 外)",
			verifier:  strings.Repeat("a", 42) + "/",
			stored:    validS256Challenge,
			wantMatch: false,
		},
		{
			name:      "verifier に = を含むとき false (padding 不可)",
			verifier:  strings.Repeat("a", 42) + "=",
			stored:    validS256Challenge,
			wantMatch: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := VerifyPKCES256Verifier(tc.verifier, tc.stored)
			if got != tc.wantMatch {
				t.Errorf("VerifyPKCES256Verifier(verifier=%q, stored=%q) = %v, want %v",
					tc.verifier, tc.stored, got, tc.wantMatch)
			}
		})
	}
}

// sha256ChallengeOf は verifier から RFC 7636 §4.6 の S256 派生値（base64url no-pad of
// SHA-256）を返すテストヘルパー。本実装の S256 仕様と独立した参照計算として、テスト側で
// 別途算出する。
func sha256ChallengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
