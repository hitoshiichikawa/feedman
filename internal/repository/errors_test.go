package repository

import (
	"errors"
	"fmt"
	"testing"
)

// TestSentinelErrors_AreDistinct は ErrAuthCodeNotUsable と ErrRefreshTokenAlreadyRotated が
// errors.Is で互いに区別されることを検証する（Req 4.3 / NFR 1.2）。
//
// 検証観点:
//  1. 自身に対しては errors.Is が true を返す
//  2. 異なる sentinel に対しては errors.Is が false を返す
//  3. fmt.Errorf("...: %w", ErrXxx) で wrap しても判別が保たれる
//  4. エラーメッセージが固定文言で、機密値（hash / user_id / id）を含まない（NFR 1.2）
func TestSentinelErrors_AreDistinct(t *testing.T) {
	// Case 1: 自己一致（自身に対しては errors.Is が true）
	t.Run("自身に対してerrors.Isがtrueを返す", func(t *testing.T) {
		if !errors.Is(ErrAuthCodeNotUsable, ErrAuthCodeNotUsable) {
			t.Error("errors.Is(ErrAuthCodeNotUsable, ErrAuthCodeNotUsable) が false を返した")
		}
		if !errors.Is(ErrRefreshTokenAlreadyRotated, ErrRefreshTokenAlreadyRotated) {
			t.Error("errors.Is(ErrRefreshTokenAlreadyRotated, ErrRefreshTokenAlreadyRotated) が false を返した")
		}
	})

	// Case 2: 異なる sentinel に対しては errors.Is が false（相互区別）
	t.Run("異なるsentinelに対してerrors.Isがfalseを返す", func(t *testing.T) {
		if errors.Is(ErrAuthCodeNotUsable, ErrRefreshTokenAlreadyRotated) {
			t.Error("errors.Is(ErrAuthCodeNotUsable, ErrRefreshTokenAlreadyRotated) が true を返した（区別できていない）")
		}
		if errors.Is(ErrRefreshTokenAlreadyRotated, ErrAuthCodeNotUsable) {
			t.Error("errors.Is(ErrRefreshTokenAlreadyRotated, ErrAuthCodeNotUsable) が true を返した（区別できていない）")
		}
	})

	// Case 3: fmt.Errorf("...: %w", ErrXxx) で wrap しても判別可能
	t.Run("wrapしても判別可能", func(t *testing.T) {
		wrappedAuthCode := fmt.Errorf("operation context: %w", ErrAuthCodeNotUsable)
		wrappedRefresh := fmt.Errorf("operation context: %w", ErrRefreshTokenAlreadyRotated)

		if !errors.Is(wrappedAuthCode, ErrAuthCodeNotUsable) {
			t.Error("wrap した ErrAuthCodeNotUsable が errors.Is で検出できない")
		}
		if !errors.Is(wrappedRefresh, ErrRefreshTokenAlreadyRotated) {
			t.Error("wrap した ErrRefreshTokenAlreadyRotated が errors.Is で検出できない")
		}
		// wrap 後も相互区別が保たれる
		if errors.Is(wrappedAuthCode, ErrRefreshTokenAlreadyRotated) {
			t.Error("wrap した ErrAuthCodeNotUsable が ErrRefreshTokenAlreadyRotated として検出された")
		}
		if errors.Is(wrappedRefresh, ErrAuthCodeNotUsable) {
			t.Error("wrap した ErrRefreshTokenAlreadyRotated が ErrAuthCodeNotUsable として検出された")
		}
	})

	// Case 4: エラーメッセージが固定文言で機密値を含まない（NFR 1.2 回帰）
	t.Run("エラーメッセージが固定文言で機密値を含まない", func(t *testing.T) {
		// 想定する固定文言（design.md / Task 3 で確定済み）
		if got, want := ErrAuthCodeNotUsable.Error(), "auth_code is not usable"; got != want {
			t.Errorf("ErrAuthCodeNotUsable.Error() = %q, want %q", got, want)
		}
		if got, want := ErrRefreshTokenAlreadyRotated.Error(), "refresh_token already rotated"; got != want {
			t.Errorf("ErrRefreshTokenAlreadyRotated.Error() = %q, want %q", got, want)
		}
	})
}
