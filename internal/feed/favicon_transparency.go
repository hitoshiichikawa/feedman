// Package feed の favicon 透明判定ヘルパー。
//
// 本ファイルは Issue #148 で導入された「全面透明 favicon を取得失敗扱いとする」
// 仕様を実装する。配信ドメインの /favicon.ico が「全ピクセルの alpha チャネルが 0」の
// 透明 ICO を返すケース（例: rocketnews24.com）で、後続フォールバック段階が
// 起動しない問題を修正するためのモジュール。
package feed

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"

	// 透明判定のため alpha チャネルを持ち得る形式をデコード可能にする。
	// image.Decode が登録済みデコーダから自動選択する。
	_ "image/gif"
	_ "image/png"
)

// errDecodeFailure は受領画像のデコードに失敗したことを示すセンチネルエラー。
// FetchFavicon 内では構造化ログのラベル分岐に利用する（要件 3.2）。
var errDecodeFailure = errors.New("favicon: decode failure")

// errFullyTransparent は受領画像が全ピクセル alpha=0 と判定されたことを示すセンチネルエラー。
// FetchFavicon 内では構造化ログのラベル分岐に利用する（要件 3.1）。
var errFullyTransparent = errors.New("favicon: fully transparent image")

// hasAlphaChannel は MIME タイプが alpha チャネルを持ち得る形式かを判定する。
// 要件 1.3 により、alpha チャネルを持たない形式（JPEG 等）は透明判定対象外として
// 従来どおり成功扱いするため、本関数で対象を絞り込む。
//
// 対象形式（true を返す）:
//   - image/png         RGBA / RGBA64 / NRGBA 系
//   - image/gif         パレットの透明色インデックス
//   - image/x-icon      ICO（PNG 内包 or 32bpp BMP の alpha バイト）
//   - image/vnd.microsoft.icon ICO の別 MIME 表記
//   - image/ico         ICO の慣用 MIME
//   - image/webp        WebP（VP8L / VP8X の alpha チャネル）。本実装ではデコーダ未登録のため透明判定対象外として扱う
//
// SVG（image/svg+xml）は透明判定対象外。XML テキストで Go の image パッケージで
// デコードできず、また透明色塗りつぶし XML を作る攻撃面も限定的なため、本実装では
// 範囲外とする（要件 Out of Scope の「部分透明」と同列に扱う）。
func hasAlphaChannel(mimeType string) bool {
	switch strings.ToLower(mimeType) {
	case "image/png",
		"image/gif",
		"image/x-icon",
		"image/vnd.microsoft.icon",
		"image/ico":
		return true
	}
	return false
}

// checkFaviconTransparency は受領した favicon バイト列を MIME に応じてデコードし、
// 全ピクセル alpha=0 であるかを判定する。
//
// 戻り値:
//   - transparent: true なら全面透明、false なら 1 ピクセル以上が alpha != 0
//   - err: デコード失敗時のみ非 nil（errDecodeFailure を wrap）。
//     mimeType が alpha チャネルを持たない形式の場合は (false, nil) を返し、
//     呼び出し側で従来どおり成功扱いさせる（要件 1.3 / 4.2）。
//
// 本関数は favicon 取得経路のサイズ上限（2MB）以内・画像 MIME・HTTP 2xx を
// 満たした画像のみに呼ばれる前提（NFR 2.2）。
func checkFaviconTransparency(data []byte, mimeType string) (transparent bool, err error) {
	if !hasAlphaChannel(mimeType) {
		// alpha を持たない形式は透明判定対象外（要件 1.3）。
		return false, nil
	}
	if len(data) == 0 {
		// 空ボディは MIME に関わらずデコード不可。
		return false, fmt.Errorf("%w: empty body", errDecodeFailure)
	}

	mt := strings.ToLower(mimeType)
	// ICO は Go 標準 image パッケージにデコーダがないため自前パースする。
	// その他（PNG / GIF）は image.Decode に委譲する。
	if mt == "image/x-icon" || mt == "image/vnd.microsoft.icon" || mt == "image/ico" {
		return isFullyTransparentICO(data)
	}

	img, _, decodeErr := image.Decode(bytes.NewReader(data))
	if decodeErr != nil {
		return false, fmt.Errorf("%w: %v", errDecodeFailure, decodeErr)
	}
	return isFullyTransparentImage(img), nil
}

// isFullyTransparentImage は image.Image の全ピクセル alpha チャネルが 0 か判定する。
// 1 ピクセルでも alpha != 0 を見つけた時点で false を返す（早期終了で NFR 2.1 を満たす）。
func isFullyTransparentImage(img image.Image) bool {
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	if bounds.Empty() {
		// 0x0 画像は全面透明と判定する材料がないため、透明扱いはせず false を返す
		// （後段の通常成功判定に委ねる）。
		return false
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if hasNonZeroAlpha(img.At(x, y)) {
				return false
			}
		}
	}
	return true
}

// hasNonZeroAlpha は color.Color の alpha チャネルが非 0 かを判定する。
// RGBA() は 16bit alpha-premultiplied を返すため、A 成分が 0 かどうかで判定する。
func hasNonZeroAlpha(c color.Color) bool {
	_, _, _, a := c.RGBA()
	return a != 0
}

// --- ICO 自前デコード ---
//
// ICO ヘッダ仕様（https://en.wikipedia.org/wiki/ICO_(file_format)）:
//
//	ICONDIR (6 bytes):
//	  uint16 reserved (must be 0)
//	  uint16 type     (1 = ICO, 2 = CUR)
//	  uint16 count    (含まれる画像数)
//	ICONDIRENTRY (16 bytes) * count:
//	  uint8  width
//	  uint8  height
//	  uint8  colorCount
//	  uint8  reserved
//	  uint16 planes
//	  uint16 bitCount
//	  uint32 bytesInRes (画像データのサイズ)
//	  uint32 imageOffset (ファイル先頭からのオフセット)
//
// 各 ICONDIRENTRY が指す画像データは以下のいずれか:
//   - PNG ファイル全体（先頭 8 バイトが PNG マジック）
//   - DIB（BMP のヘッダなし版。BITMAPINFOHEADER 始まり）
//
// 本実装では透明判定が目的なので「最初のエントリ」のみを検査する。
// 全エントリを検査しないのは、典型的な favicon.ico は単一画像 or 同一画像の複数解像度を
// 持つため、最初のエントリの透明性が全体の透明性を代表すると見なせるため。

const icoDirHeaderSize = 6
const icoDirEntrySize = 16
const pngMagicSize = 8

var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// isFullyTransparentICO は ICO バイト列から最初のエントリを取り出し透明判定する。
// PNG 内包なら image/png でデコード、それ以外は 32bpp BMP の alpha バイトを直接走査する。
// 24bpp 等で alpha チャネルを持たない BMP は (false, nil) を返し成功扱いさせる。
func isFullyTransparentICO(data []byte) (bool, error) {
	if len(data) < icoDirHeaderSize+icoDirEntrySize {
		return false, fmt.Errorf("%w: ICO too short (%d bytes)", errDecodeFailure, len(data))
	}
	// ICONDIR チェック
	reserved := binary.LittleEndian.Uint16(data[0:2])
	icoType := binary.LittleEndian.Uint16(data[2:4])
	count := binary.LittleEndian.Uint16(data[4:6])
	if reserved != 0 || icoType != 1 || count == 0 {
		return false, fmt.Errorf("%w: invalid ICO header (reserved=%d type=%d count=%d)",
			errDecodeFailure, reserved, icoType, count)
	}

	// 最初のエントリのみ検査
	entryOff := icoDirHeaderSize
	bytesInRes := binary.LittleEndian.Uint32(data[entryOff+8 : entryOff+12])
	imageOff := binary.LittleEndian.Uint32(data[entryOff+12 : entryOff+16])
	if imageOff == 0 || bytesInRes == 0 {
		return false, fmt.Errorf("%w: ICO entry has zero offset/size", errDecodeFailure)
	}
	end := uint64(imageOff) + uint64(bytesInRes)
	if end > uint64(len(data)) {
		return false, fmt.Errorf("%w: ICO entry exceeds data length", errDecodeFailure)
	}
	imgData := data[imageOff:end]

	// PNG 内包か判定
	if len(imgData) >= pngMagicSize && bytes.Equal(imgData[:pngMagicSize], pngMagic) {
		img, decodeErr := decodePNGFromBytes(imgData)
		if decodeErr != nil {
			return false, fmt.Errorf("%w: ICO->PNG decode: %v", errDecodeFailure, decodeErr)
		}
		return isFullyTransparentImage(img), nil
	}

	// BMP（DIB）の場合、BITMAPINFOHEADER は先頭 4 バイトがヘッダサイズ
	return isFullyTransparentICOBMP(imgData)
}

// decodePNGFromBytes は image/png でデコードして image.Image を返す。
// blank import で登録済みの image.Decode 経由でも可能だが、フォーマット明示で
// 呼び出し意図を明確にするため image.Decode を経由する。
func decodePNGFromBytes(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// isFullyTransparentICOBMP は ICO 内の DIB（BMP ヘッダなし版）に対して
// 32bpp BGRA の alpha バイトを直接走査する。24bpp 等 alpha なしは (false, nil) を返す。
//
// BITMAPINFOHEADER（最小 40 bytes）:
//
//	uint32 size
//	int32  width
//	int32  height        (ICO 内では画像高さ * 2。XOR mask + AND mask の合計)
//	uint16 planes
//	uint16 bitCount      (1/4/8/16/24/32)
//	uint32 compression
//	uint32 sizeImage
//	... (以下省略)
//
// 32bpp の場合、ピクセル配列は (width * realHeight) * 4 バイトで BGRA 順。
// realHeight = height/2（XOR mask 部分のみ。AND mask は alpha 用 1bpp）。
func isFullyTransparentICOBMP(data []byte) (bool, error) {
	const minHeaderSize = 40
	if len(data) < minHeaderSize {
		return false, fmt.Errorf("%w: BMP header too short (%d bytes)", errDecodeFailure, len(data))
	}
	headerSize := binary.LittleEndian.Uint32(data[0:4])
	if headerSize < minHeaderSize || uint64(headerSize) > uint64(len(data)) {
		return false, fmt.Errorf("%w: BMP header size invalid (%d)", errDecodeFailure, headerSize)
	}
	width := int32(binary.LittleEndian.Uint32(data[4:8]))
	rawHeight := int32(binary.LittleEndian.Uint32(data[8:12]))
	bitCount := binary.LittleEndian.Uint16(data[14:16])

	if width <= 0 || rawHeight == 0 {
		return false, fmt.Errorf("%w: BMP invalid dimensions (w=%d h=%d)", errDecodeFailure, width, rawHeight)
	}
	// ICO の DIB は通常 height = realHeight * 2（XOR mask + AND mask）。
	realHeight := rawHeight / 2
	if realHeight <= 0 {
		realHeight = rawHeight // 念のため
	}

	// alpha チャネルを持たない bitCount は透明判定対象外として false 返却（要件 4.2 と同じ扱い）。
	if bitCount != 32 {
		return false, nil
	}

	// 32bpp BGRA ピクセル配列
	pixelOff := uint64(headerSize)
	pixelBytes := uint64(width) * uint64(realHeight) * 4
	if pixelOff+pixelBytes > uint64(len(data)) {
		return false, fmt.Errorf("%w: BMP pixel array exceeds data length", errDecodeFailure)
	}
	// BGRA 順で 4 バイトごとの A バイト（オフセット 3）を走査
	for i := uint64(0); i < pixelBytes; i += 4 {
		if data[pixelOff+i+3] != 0 {
			return false, nil
		}
	}
	return true, nil
}
