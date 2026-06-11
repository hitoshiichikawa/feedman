package model

import "testing"

// TestFaviconDataURL は data URL 構築と欠落時 nil の挙動を検証する。
func TestFaviconDataURL(t *testing.T) {
	t.Run("data と mime が揃うとき data URL を返すこと", func(t *testing.T) {
		got := FaviconDataURL([]byte("abc"), "image/png")
		if got == nil {
			t.Fatal("expected non-nil data URL")
		}
		// base64("abc") = "YWJj"
		const want = "data:image/png;base64,YWJj"
		if *got != want {
			t.Errorf("FaviconDataURL = %q, want %q", *got, want)
		}
	})

	t.Run("data が空のとき nil を返すこと", func(t *testing.T) {
		if got := FaviconDataURL(nil, "image/png"); got != nil {
			t.Errorf("expected nil for empty data, got %q", *got)
		}
		if got := FaviconDataURL([]byte{}, "image/png"); got != nil {
			t.Errorf("expected nil for zero-length data, got %q", *got)
		}
	})

	t.Run("mime が空のとき nil を返すこと", func(t *testing.T) {
		if got := FaviconDataURL([]byte("abc"), ""); got != nil {
			t.Errorf("expected nil for empty mime, got %q", *got)
		}
	})
}
