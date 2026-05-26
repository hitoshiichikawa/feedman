package auth

import (
	"strings"
	"testing"
)

// TestMaskEmail はmaskEmailのマスク仕様（正常系・異常系・境界値・空入力）を検証する。
func TestMaskEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		// 正常系: ローカル部の先頭1文字のみ残し残余を伏字、ドメイン部は保持する（Req 2.1, 2.2）
		{
			name:  "通常のメールアドレスのときローカル部先頭のみ残しドメインを保持する",
			input: "hitoshi@example.com",
			want:  "h***@example.com",
		},
		{
			name:  "別ドメインのメールアドレスのときドメイン部をそのまま保持する",
			input: "alice@feedman.test",
			want:  "a***@feedman.test",
		},
		// 境界値: ローカル部1文字でも先頭文字を平文相当で漏らさない（Req 2.3）
		{
			name:  "ローカル部が1文字のとき先頭文字を露出せず伏字のみを出力する",
			input: "h@example.com",
			want:  "***@example.com",
		},
		// 境界値: ローカル部のみ（ドメイン空）でも復元可能な平文を出さない
		{
			name:  "ドメイン部が空のとき復元可能な平文を出力しない",
			input: "user@",
			want:  "u***@",
		},
		// 異常系: 空文字でもパニックせず復元可能な平文を出さない（Req 4.1, 4.3）
		{
			name:  "空文字のときパニックせず固定マスク値を返す",
			input: "",
			want:  "***",
		},
		// 異常系: @を含まない不正形式でも復元可能な平文を出さない（Req 4.2, 4.3）
		{
			name:  "@を含まない不正形式のとき復元可能な平文を出力しない",
			input: "notanemail",
			want:  "***",
		},
		// 境界値: ドメインのみ（ローカル部空）でもパニックせず安全に処理する
		{
			name:  "ローカル部が空でドメインのみのとき先頭を伏字にしドメインを保持する",
			input: "@example.com",
			want:  "***@example.com",
		},
		// 境界値: 複数@を含む場合は最初の@で分割しドメイン側（2つ目の@含む）を保持する
		{
			name:  "複数の@を含むとき最初の@より前のローカル部のみマスクする",
			input: "ab@b@example.com",
			want:  "a***@b@example.com",
		},
		// 境界値: 複数@かつローカル部1文字のとき先頭も伏せ復元可能な平文を出さない
		{
			name:  "複数の@かつローカル部1文字のとき先頭文字を露出しない",
			input: "a@b@example.com",
			want:  "***@b@example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := maskEmail(tc.input)

			// Assert
			if got != tc.want {
				t.Errorf("maskEmail(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestMaskEmailDoesNotLeakLocalPart はマスク値がローカル部の2文字目以降を漏らさないことを検証する（Req 2.3, NFR 1.1）。
func TestMaskEmailDoesNotLeakLocalPart(t *testing.T) {
	// Arrange: ローカル部が複数文字で2文字目以降に固有の情報を持つメールアドレス
	const input = "secretuser@example.com"

	// Act
	got := maskEmail(input)

	// Assert: ローカル部の2文字目以降（"ecretuser"）がマスク値に含まれないこと
	leaked := "ecretuser"
	if strings.Contains(got, leaked) {
		t.Errorf("maskEmail(%q) = %q, leaked local part %q", input, got, leaked)
	}
}
