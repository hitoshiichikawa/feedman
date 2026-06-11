package auth

import (
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
