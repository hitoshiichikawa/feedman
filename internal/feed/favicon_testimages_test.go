package feed

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
)

// 本ファイルは Issue #148 の透明判定テスト用の画像生成ヘルパーをまとめる。
// 既存テスト（favicon_test.go / favicon_fallback_test.go 等）が PNG マジック
// バイト 8 バイトのみを成功扱いとしていたが、透明判定（要件 1.4）導入により
// デコードが失敗するため、これら既存テストも本ファイルのヘルパーで生成した
// 有効な PNG を利用するよう更新している。

// newOpaquePNG は指定サイズの完全不透明 PNG バイト列を返す。
// 全ピクセル alpha=255 のため、透明判定では成功扱いとなる（要件 4.1）。
func newOpaquePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	opaqueRed := color.RGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, opaqueRed)
		}
	}
	return encodePNG(t, img)
}

// newFullyTransparentPNG は全ピクセル alpha=0 の PNG バイト列を返す。
// 透明判定では全面透明として段階失敗扱いになる（要件 1.1, 1.2）。
func newFullyTransparentPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	transparent := color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, transparent)
		}
	}
	return encodePNG(t, img)
}

// newPartiallyTransparentPNG は 1 ピクセルのみ alpha != 0 で残りは alpha=0 の
// PNG バイト列を返す。透明判定では成功扱いになる（要件 4.1）。
func newPartiallyTransparentPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	transparent := color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, transparent)
		}
	}
	// 1 ピクセルだけ非透明
	img.Set(0, 0, color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	return encodePNG(t, img)
}

// encodePNG は image.Image を PNG バイト列に変換する。
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}
	return buf.Bytes()
}

// generateOpaquePNGForInit はパッケージレベル変数の初期化用ヘルパー。
// testing.T が無い文脈（init / var 初期化）でも呼べる。
// 失敗時は panic でテスト全体を露見させる。
func generateOpaquePNGForInit(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	opaqueRed := color.RGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, opaqueRed)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic("generateOpaquePNGForInit: png.Encode failed: " + err.Error())
	}
	return buf.Bytes()
}

// newOpaqueGIF は完全不透明な 1x1 GIF を返す（透明判定で成功扱い）。
func newOpaqueGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{
		color.RGBA{R: 0xFF, A: 0xFF},
	})
	img.SetColorIndex(0, 0, 0)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("gif.Encode failed: %v", err)
	}
	return buf.Bytes()
}

// newJPEGLikeBytes は image MIME=image/jpeg を装うが image.Decode に失敗する
// バイト列ではなく、本テスト用途では JPEG マジック + 最小限の構造を返す。
// ただし jpeg.Decode で必ず成功する保証は無いため、透明判定では alpha 形式外
// として扱われる経路を意図する場合は MIME image/jpeg のみ重要。実バイト列は
// 任意で良い（hasAlphaChannel が false を返し checkFaviconTransparency が
// (false, nil) を返す）。
func newJPEGLikeBytes() []byte {
	// SOI(FFD8) + EOI(FFD9) の最小プレースホルダ。
	// 透明判定対象外（hasAlphaChannel=false）のため、デコードされずに通過する。
	return []byte{0xFF, 0xD8, 0xFF, 0xD9}
}

// newFullyTransparentICO は ICO 形式で全ピクセル alpha=0 の 16x16 BMP（32bpp）を
// 内包した ICO バイト列を返す。rocketnews24.com の透明 favicon を模した fixture。
// 透明判定では全面透明として段階失敗扱いになる（要件 1.1, 1.2）。
func newFullyTransparentICO(t *testing.T, width, height int) []byte {
	t.Helper()
	return newICOWithBMPAlpha(t, width, height, 0x00)
}

// newOpaqueICO は ICO 形式で全ピクセル alpha=255 の BMP（32bpp）を内包した
// ICO バイト列を返す。透明判定では成功扱い。
func newOpaqueICO(t *testing.T, width, height int) []byte {
	t.Helper()
	return newICOWithBMPAlpha(t, width, height, 0xFF)
}

// newICOWithBMPAlpha は内部ヘルパー。32bpp BMP（DIB）を内包する ICO を生成する。
// alpha は全ピクセル共通の alpha バイト値（0 = 透明 / 255 = 不透明）。
//
// ICO レイアウト:
//
//   - ICONDIR (6 bytes)
//   - ICONDIRENTRY (16 bytes)
//   - BITMAPINFOHEADER (40 bytes)
//   - ピクセル配列 (width*height*4 bytes, BGRA 順)
//   - AND mask (簡略化のため省略可。透明判定実装は BMP 部分のみ参照)
func newICOWithBMPAlpha(t *testing.T, width, height int, alpha byte) []byte {
	t.Helper()
	if width <= 0 || height <= 0 || width > 255 || height > 255 {
		t.Fatalf("invalid ICO dimensions: %dx%d", width, height)
	}

	const headerSize = 40
	pixelSize := width * height * 4
	bmpTotal := headerSize + pixelSize
	imageOff := 6 + 16 // ICONDIR + ICONDIRENTRY

	buf := &bytes.Buffer{}

	// ICONDIR
	binary.Write(buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1)) // type=ICO
	binary.Write(buf, binary.LittleEndian, uint16(1)) // count=1

	// ICONDIRENTRY
	buf.WriteByte(byte(width))                         // width
	buf.WriteByte(byte(height))                        // height
	buf.WriteByte(0)                                   // colorCount
	buf.WriteByte(0)                                   // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1))  // planes
	binary.Write(buf, binary.LittleEndian, uint16(32)) // bitCount
	binary.Write(buf, binary.LittleEndian, uint32(bmpTotal))
	binary.Write(buf, binary.LittleEndian, uint32(imageOff))

	// BITMAPINFOHEADER
	binary.Write(buf, binary.LittleEndian, uint32(headerSize)) // size
	binary.Write(buf, binary.LittleEndian, int32(width))       // width
	binary.Write(buf, binary.LittleEndian, int32(height*2))    // height (DIB 規約で XOR+AND の合計)
	binary.Write(buf, binary.LittleEndian, uint16(1))          // planes
	binary.Write(buf, binary.LittleEndian, uint16(32))         // bitCount
	binary.Write(buf, binary.LittleEndian, uint32(0))          // compression=BI_RGB
	binary.Write(buf, binary.LittleEndian, uint32(pixelSize))  // sizeImage
	binary.Write(buf, binary.LittleEndian, int32(0))           // xPelsPerMeter
	binary.Write(buf, binary.LittleEndian, int32(0))           // yPelsPerMeter
	binary.Write(buf, binary.LittleEndian, uint32(0))          // clrUsed
	binary.Write(buf, binary.LittleEndian, uint32(0))          // clrImportant

	// ピクセル配列（BGRA 順、全ピクセル同色）
	for i := 0; i < width*height; i++ {
		buf.WriteByte(0x00)  // B
		buf.WriteByte(0x00)  // G
		buf.WriteByte(0x00)  // R
		buf.WriteByte(alpha) // A
	}

	return buf.Bytes()
}

// newPNGEmbeddedICO は ICO 形式で PNG 内包の透明 favicon バイト列を返す。
// alpha=0 なら全面透明として段階失敗扱いになる（要件 1.1）。
// 32bpp BMP 内包と PNG 内包の両ルートをテストするためのヘルパー。
func newPNGEmbeddedICO(t *testing.T, pngBytes []byte) []byte {
	t.Helper()
	imageOff := 6 + 16
	bmpTotal := len(pngBytes)

	buf := &bytes.Buffer{}

	// ICONDIR
	binary.Write(buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1)) // type=ICO
	binary.Write(buf, binary.LittleEndian, uint16(1)) // count=1

	// ICONDIRENTRY
	buf.WriteByte(0)                                   // width=0 means 256
	buf.WriteByte(0)                                   // height=0 means 256
	buf.WriteByte(0)                                   // colorCount
	buf.WriteByte(0)                                   // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1))  // planes
	binary.Write(buf, binary.LittleEndian, uint16(32)) // bitCount
	binary.Write(buf, binary.LittleEndian, uint32(bmpTotal))
	binary.Write(buf, binary.LittleEndian, uint32(imageOff))

	// PNG 本体
	buf.Write(pngBytes)

	return buf.Bytes()
}
