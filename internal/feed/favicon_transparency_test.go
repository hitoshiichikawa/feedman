package feed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 本ファイルは Issue #148「透明/空の /favicon.ico を成功扱いし favicon フォールバック・
// 既定アイコンが効かない」の要件に対応するテストをまとめる。

// --- checkFaviconTransparency 単体テスト ---

// TestCheckFaviconTransparency_FullyTransparentPNG は全ピクセル alpha=0 の PNG が
// 透明判定で true を返すことをテストする（要件 1.1, 1.2）。
func TestCheckFaviconTransparency_FullyTransparentPNG(t *testing.T) {
	// Arrange
	data := newFullyTransparentPNG(t, 16, 16)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/png")

	// Assert
	if err != nil {
		t.Fatalf("デコード失敗にならないべき: %v", err)
	}
	if !transparent {
		t.Error("全ピクセル alpha=0 の PNG は透明と判定されるべき")
	}
}

// TestCheckFaviconTransparency_OpaquePNG は完全不透明 PNG が
// 透明判定で false を返すことをテストする（要件 4.1）。
func TestCheckFaviconTransparency_OpaquePNG(t *testing.T) {
	// Arrange
	data := newOpaquePNG(t, 16, 16)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/png")

	// Assert
	if err != nil {
		t.Fatalf("デコード失敗にならないべき: %v", err)
	}
	if transparent {
		t.Error("不透明 PNG は透明と判定されるべきでない")
	}
}

// TestCheckFaviconTransparency_PartiallyTransparentPNG は 1 ピクセルでも
// alpha != 0 を含む PNG が成功扱い（transparent=false）になることをテストする（要件 4.1）。
func TestCheckFaviconTransparency_PartiallyTransparentPNG(t *testing.T) {
	// Arrange
	data := newPartiallyTransparentPNG(t, 16, 16)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/png")

	// Assert
	if err != nil {
		t.Fatalf("デコード失敗にならないべき: %v", err)
	}
	if transparent {
		t.Error("1 ピクセル以上 alpha != 0 を含む PNG は透明と判定されるべきでない（要件 4.1）")
	}
}

// TestCheckFaviconTransparency_JPEGIsNotChecked は MIME=image/jpeg は alpha を持たない
// 形式として透明判定対象外（false, nil）を返すことをテストする（要件 1.3, 4.2）。
func TestCheckFaviconTransparency_JPEGIsNotChecked(t *testing.T) {
	// Arrange
	data := newJPEGLikeBytes()

	// Act
	transparent, err := checkFaviconTransparency(data, "image/jpeg")

	// Assert: JPEG は透明判定対象外として false, nil を返し成功扱いさせる
	if err != nil {
		t.Fatalf("JPEG はデコード対象外なのでエラーを返すべきでない: %v", err)
	}
	if transparent {
		t.Error("JPEG は透明判定対象外なので false を返すべき")
	}
}

// TestCheckFaviconTransparency_BrokenPNG は壊れた PNG（マジック先頭のみ、本体破損）が
// デコード失敗扱いになることをテストする（要件 1.4）。
func TestCheckFaviconTransparency_BrokenPNG(t *testing.T) {
	// Arrange: PNG マジック 8 バイトのみで IHDR 以降が存在しない
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	// Act
	_, err := checkFaviconTransparency(data, "image/png")

	// Assert: errDecodeFailure を wrap した err が返る
	if err == nil {
		t.Fatal("壊れた PNG はデコード失敗エラーを返すべき")
	}
	if !errors.Is(err, errDecodeFailure) {
		t.Errorf("err は errDecodeFailure を含むべき。got %v", err)
	}
}

// TestCheckFaviconTransparency_EmptyBody は空ボディがデコード失敗扱いになることを
// テストする（要件 1.4 境界値）。
func TestCheckFaviconTransparency_EmptyBody(t *testing.T) {
	// Act
	_, err := checkFaviconTransparency(nil, "image/png")

	// Assert
	if err == nil {
		t.Fatal("空ボディはデコード失敗エラーを返すべき")
	}
	if !errors.Is(err, errDecodeFailure) {
		t.Errorf("err は errDecodeFailure を含むべき。got %v", err)
	}
}

// TestCheckFaviconTransparency_FullyTransparentICO は全ピクセル alpha=0 の ICO
// （内包 32bpp BMP）が透明と判定されることをテストする（要件 1.1）。
func TestCheckFaviconTransparency_FullyTransparentICO(t *testing.T) {
	// Arrange: rocketnews24.com 模擬 16x16 全面透明 ICO
	data := newFullyTransparentICO(t, 16, 16)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/x-icon")

	// Assert
	if err != nil {
		t.Fatalf("有効な ICO はデコード成功すべき: %v", err)
	}
	if !transparent {
		t.Error("全面透明 ICO は透明と判定されるべき")
	}
}

// TestCheckFaviconTransparency_OpaqueICO は完全不透明 ICO が成功扱いになることをテストする。
func TestCheckFaviconTransparency_OpaqueICO(t *testing.T) {
	// Arrange
	data := newOpaqueICO(t, 16, 16)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/x-icon")

	// Assert
	if err != nil {
		t.Fatalf("有効な ICO はデコード成功すべき: %v", err)
	}
	if transparent {
		t.Error("不透明 ICO は透明と判定されるべきでない")
	}
}

// TestCheckFaviconTransparency_PNGEmbeddedTransparentICO は PNG 内包 ICO で
// 全面透明な PNG を持つケースが透明と判定されることをテストする（要件 1.1）。
func TestCheckFaviconTransparency_PNGEmbeddedTransparentICO(t *testing.T) {
	// Arrange
	transparentPNG := newFullyTransparentPNG(t, 32, 32)
	data := newPNGEmbeddedICO(t, transparentPNG)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/x-icon")

	// Assert
	if err != nil {
		t.Fatalf("PNG 内包 ICO はデコード成功すべき: %v", err)
	}
	if !transparent {
		t.Error("全面透明 PNG 内包の ICO は透明と判定されるべき")
	}
}

// TestCheckFaviconTransparency_BrokenICO は壊れた ICO がデコード失敗扱いになることを
// テストする（要件 1.4）。
func TestCheckFaviconTransparency_BrokenICO(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"too short", []byte{0x00, 0x00, 0x01, 0x00}},
		{"invalid type", []byte{0x00, 0x00, 0xFF, 0xFF, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{"zero count", []byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checkFaviconTransparency(tt.data, "image/x-icon")
			if err == nil {
				t.Fatal("壊れた ICO はデコード失敗エラーを返すべき")
			}
			if !errors.Is(err, errDecodeFailure) {
				t.Errorf("err は errDecodeFailure を含むべき。got %v", err)
			}
		})
	}
}

// TestCheckFaviconTransparency_OpaqueGIF は完全不透明 GIF が成功扱いになることをテストする。
func TestCheckFaviconTransparency_OpaqueGIF(t *testing.T) {
	// Arrange
	data := newOpaqueGIF(t)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/gif")

	// Assert
	if err != nil {
		t.Fatalf("有効な GIF はデコード成功すべき: %v", err)
	}
	if transparent {
		t.Error("不透明 GIF は透明と判定されるべきでない")
	}
}

// TestCheckFaviconTransparency_SVGNotChecked は SVG が透明判定対象外として
// (false, nil) を返すことをテストする（要件 4.2 と整合）。
func TestCheckFaviconTransparency_SVGNotChecked(t *testing.T) {
	// Arrange
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)

	// Act
	transparent, err := checkFaviconTransparency(data, "image/svg+xml")

	// Assert
	if err != nil {
		t.Fatalf("SVG は透明判定対象外なのでエラーを返すべきでない: %v", err)
	}
	if transparent {
		t.Error("SVG は透明判定対象外なので false を返すべき")
	}
}

// TestHasAlphaChannel は MIME に応じた alpha チャネル有無判定をテストする。
func TestHasAlphaChannel(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"image/png", true},
		{"image/gif", true},
		{"image/x-icon", true},
		{"image/vnd.microsoft.icon", true},
		{"image/ico", true},
		{"image/jpeg", false},
		{"image/jpg", false},
		{"image/svg+xml", false},
		{"image/bmp", false},
		{"image/webp", false},
		{"", false},
		// 大文字小文字の正規化
		{"IMAGE/PNG", true},
	}
	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			got := hasAlphaChannel(tt.mime)
			if got != tt.want {
				t.Errorf("hasAlphaChannel(%q) = %v, want %v", tt.mime, got, tt.want)
			}
		})
	}
}

// --- FetchFavicon 経由の統合テスト ---

// TestFetchFavicon_FullyTransparentICO_ReturnsNil は HTTP サーバが全面透明 ICO を
// 返したときに FetchFavicon が nil を返すことをテストする（要件 1.2, 2.1）。
func TestFetchFavicon_FullyTransparentICO_ReturnsNil(t *testing.T) {
	// Arrange
	transparentICO := newFullyTransparentICO(t, 16, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		_, _ = w.Write(transparentICO)
	}))
	defer srv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFavicon(context.Background(), srv.URL+"/favicon.ico")

	// Assert
	if err != nil {
		t.Fatalf("FetchFavicon should not return error: %v", err)
	}
	if data != nil {
		t.Errorf("全面透明 ICO は nil として返されるべき。got %d bytes", len(data))
	}
	if mime != "" {
		t.Errorf("全面透明 ICO は空 MIME を返すべき。got %q", mime)
	}
}

// TestFetchFavicon_FullyTransparentPNG_ReturnsNil は HTTP サーバが全面透明 PNG を
// 返したときに FetchFavicon が nil を返すことをテストする（要件 1.1, 1.2）。
func TestFetchFavicon_FullyTransparentPNG_ReturnsNil(t *testing.T) {
	// Arrange
	transparentPNG := newFullyTransparentPNG(t, 16, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(transparentPNG)
	}))
	defer srv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFavicon(context.Background(), srv.URL+"/favicon.png")

	// Assert
	if err != nil {
		t.Fatalf("FetchFavicon should not return error: %v", err)
	}
	if data != nil {
		t.Errorf("全面透明 PNG は nil として返されるべき。got %d bytes", len(data))
	}
	if mime != "" {
		t.Errorf("全面透明 PNG は空 MIME を返すべき。got %q", mime)
	}
}

// TestFetchFavicon_BrokenPNG_ReturnsNil は HTTP サーバが壊れた PNG を返したときに
// FetchFavicon が段階失敗扱いで nil を返すことをテストする（要件 1.4）。
func TestFetchFavicon_BrokenPNG_ReturnsNil(t *testing.T) {
	// Arrange: PNG マジックのみで本体が破損
	brokenPNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0xFF, 0xFF}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(brokenPNG)
	}))
	defer srv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFavicon(context.Background(), srv.URL+"/favicon.png")

	// Assert
	if err != nil {
		t.Fatalf("FetchFavicon should not return error: %v", err)
	}
	if data != nil {
		t.Errorf("デコード失敗時は nil を返すべき。got %d bytes", len(data))
	}
	if mime != "" {
		t.Errorf("デコード失敗時は空 MIME を返すべき。got %q", mime)
	}
}

// TestFetchFavicon_PartiallyTransparentPNG_ReturnsData は 1 ピクセルでも alpha != 0
// なら従来どおり成功扱いになることをテストする（要件 4.1）。
func TestFetchFavicon_PartiallyTransparentPNG_ReturnsData(t *testing.T) {
	// Arrange
	partialPNG := newPartiallyTransparentPNG(t, 16, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(partialPNG)
	}))
	defer srv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFavicon(context.Background(), srv.URL+"/favicon.png")

	// Assert
	if err != nil {
		t.Fatalf("FetchFavicon should not return error: %v", err)
	}
	if len(data) == 0 {
		t.Error("部分透明 PNG は成功扱いとしてデータを返すべき（要件 4.1）")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
}

// TestFetchFavicon_JPEG_ReturnsDataWithoutTransparencyCheck は JPEG が透明判定対象外として
// 従来どおり成功扱いになることをテストする（要件 1.3, 4.2）。
func TestFetchFavicon_JPEG_ReturnsDataWithoutTransparencyCheck(t *testing.T) {
	// Arrange: JPEG マジック（透明判定対象外なのでデコードされない）
	jpegBody := newJPEGLikeBytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegBody)
	}))
	defer srv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFavicon(context.Background(), srv.URL+"/favicon.jpg")

	// Assert
	if err != nil {
		t.Fatalf("FetchFavicon should not return error: %v", err)
	}
	if len(data) == 0 {
		t.Error("JPEG は透明判定対象外として成功扱いとしてデータを返すべき（要件 4.2）")
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", mime)
	}
}

// --- FetchFaviconWithFallback での段階制御テスト ---

// TestFetchFaviconWithFallback_StageA_TransparentICO_FallsThroughToStageB は段階 (a) の
// /favicon.ico が全面透明 ICO を返したとき、段階 (b) の HTML link で有効 favicon を
// 取得することをテストする（要件 1.5, 2.2）。
func TestFetchFaviconWithFallback_StageA_TransparentICO_FallsThroughToStageB(t *testing.T) {
	// Arrange
	transparentICO := newFullyTransparentICO(t, 16, 16)
	opaquePNG := newOpaquePNG(t, 16, 16)
	var stageBHit atomic.Bool

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			// 段階 (a) は透明 ICO を返す → 段階失敗扱い
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(transparentICO)
		case "/":
			stageBHit.Store(true)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/declared-icon.png"></head></html>`))
		case "/declared-icon.png":
			// 段階 (b) で参照される有効な PNG
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(opaquePNG)
		case "/feed.xml":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")

	// Assert
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback should not return error: %v", err)
	}
	if !stageBHit.Load() {
		t.Error("段階 (a) で透明扱い → 段階 (b) が試行されるべき（要件 1.5）")
	}
	if len(data) == 0 {
		t.Error("段階 (b) で有効 favicon が取得できるべき")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
}

// TestFetchFaviconWithFallback_AllStagesTransparent_ReturnsNil は全段階が透明/失敗で
// 最終的に nil を返すことをテストする（要件 2.1）。
func TestFetchFaviconWithFallback_AllStagesTransparent_ReturnsNil(t *testing.T) {
	// Arrange
	transparentICO := newFullyTransparentICO(t, 16, 16)
	transparentPNG := newFullyTransparentPNG(t, 16, 16)
	var siteHostBase string

	siteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(transparentICO) // 段階 (c) も透明
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/transparent.png"></head></html>`))
		case "/transparent.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(transparentPNG) // 段階 (d) も透明
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer siteSrv.Close()
	siteHostBase = siteSrv.URL

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(transparentICO) // 段階 (a) も透明
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/transparent.png"></head></html>`))
		case "/transparent.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(transparentPNG) // 段階 (b) も透明
		case "/feed.xml":
			rss := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Test</title>
<link>%s</link>
<item><title>x</title><link>%s/a</link></item>
</channel></rss>`, siteHostBase, siteHostBase)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer feedSrv.Close()

	f := NewFaviconFetcher(&mockSSRFGuard{})

	// Act
	data, mime, err := f.FetchFaviconWithFallback(context.Background(), feedSrv.URL+"/feed.xml")

	// Assert
	if err != nil {
		t.Fatalf("FetchFaviconWithFallback should not return error: %v", err)
	}
	if data != nil {
		t.Errorf("全段階透明なら nil を返すべき。got %d bytes", len(data))
	}
	if mime != "" {
		t.Errorf("全段階透明なら空 MIME を返すべき。got %q", mime)
	}
}

// --- NFR 2.1: 透明判定処理は 1 画像あたり 100ms 以内 ---

// TestCheckFaviconTransparency_Performance は典型的なサイズ（256x256 PNG）の
// 透明判定が 100ms 以内に完了することをテストする（NFR 2.1）。
func TestCheckFaviconTransparency_Performance(t *testing.T) {
	// Arrange: 比較的大きい favicon サイズ（256x256）の全面透明 PNG
	data := newFullyTransparentPNG(t, 256, 256)

	// Act
	start := time.Now()
	_, err := checkFaviconTransparency(data, "image/png")
	elapsed := time.Since(start)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("透明判定は 100ms 以内であるべき。actual: %v", elapsed)
	}
}
