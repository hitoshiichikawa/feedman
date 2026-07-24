package passkey

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateAndNormalize はユーザー名の検証・正規化ロジックの境界値を網羅する
// table-driven テストである（Issue #216 / Req 1.4, 1.5）。
func TestValidateAndNormalize(t *testing.T) {
	t.Run("正常系: 各種有効な入力を正規化して返す", func(t *testing.T) {
		cases := []struct {
			name       string
			input      string
			wantOutput string
		}{
			{name: "下限3文字のとき成功する", input: "abc", wantOutput: "abc"},
			{name: "上限32文字のとき成功する", input: strings.Repeat("a", 32), wantOutput: strings.Repeat("a", 32)},
			{name: "大文字を含むときlowercaseに正規化される", input: "UserName", wantOutput: "username"},
			{name: "数字を含むときそのまま維持される", input: "user123", wantOutput: "user123"},
			{name: "アンダースコアを含むときそのまま維持される", input: "user_name", wantOutput: "user_name"},
			{name: "ハイフンを含むときそのまま維持される", input: "user-name", wantOutput: "user-name"},
			{name: "全大文字入力のときlowercaseに正規化される", input: "ABC", wantOutput: "abc"},
			{name: "大文字小文字混在と記号のとき正規化される", input: "Alice-99_X", wantOutput: "alice-99_x"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				input := tc.input

				// Act
				got, err := ValidateAndNormalize(input)

				// Assert
				if err != nil {
					t.Fatalf("ValidateAndNormalize(%q) returned error: %v, want nil", input, err)
				}
				if got != tc.wantOutput {
					t.Errorf("ValidateAndNormalize(%q) = %q, want %q", input, got, tc.wantOutput)
				}
			})
		}
	})

	t.Run("異常系: 形式不正の入力は ErrInvalidUsername を返す", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
		}{
			{name: "空文字のとき拒否する", input: ""},
			{name: "1文字のとき下限未満で拒否する", input: "a"},
			{name: "2文字のとき下限未満で拒否する", input: "ab"},
			{name: "33文字のとき上限超過で拒否する", input: strings.Repeat("a", 33)},
			{name: "64文字のとき上限超過で拒否する", input: strings.Repeat("a", 64)},
			{name: "先頭に空白を含むとき拒否する", input: " abc"},
			{name: "末尾に空白を含むとき拒否する", input: "abc "},
			{name: "空白が混在するとき拒否する", input: "ab c"},
			{name: "タブ制御文字を含むとき拒否する", input: "ab\tc"},
			{name: "改行制御文字を含むとき拒否する", input: "ab\nc"},
			{name: "NUL制御文字を含むとき拒否する", input: "ab\x00c"},
			{name: "許容外の記号(ドット)を含むとき拒否する", input: "a.bc"},
			{name: "許容外の記号(アットマーク)を含むとき拒否する", input: "a@bc"},
			{name: "許容外の記号(スラッシュ)を含むとき拒否する", input: "a/bc"},
			{name: "日本語カタカナを含むとき拒否する", input: "アリス"},
			{name: "絵文字を含むとき拒否する", input: "user😀"},
			{name: "非ASCIIのアクセント文字を含むとき拒否する", input: "café"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				// Arrange
				input := tc.input

				// Act
				got, err := ValidateAndNormalize(input)

				// Assert
				if !errors.Is(err, ErrInvalidUsername) {
					t.Fatalf("ValidateAndNormalize(%q) error = %v, want errors.Is(err, ErrInvalidUsername)", input, err)
				}
				if got != "" {
					t.Errorf("ValidateAndNormalize(%q) returned normalized = %q, want empty string on error", input, got)
				}
			})
		}
	})
}
