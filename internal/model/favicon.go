package model

import (
	"encoding/base64"
	"fmt"
)

// FaviconDataURL はfaviconの生バイトとMIMEタイプから data URL（`data:<mime>;base64,<data>`）を
// 構築して返す。
//
// data が空、または mime が空の場合は nil を返す（呼び出し側はそのまま *string フィールドへ
// 代入でき、欠落時は nil が保持される）。subscription / crossfeed / 記事検索の各レスポンス整形で
// 共通利用する（同一の整形ロジックの重複を避けるため）。
func FaviconDataURL(data []byte, mime string) *string {
	if len(data) == 0 || mime == "" {
		return nil
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	return &dataURL
}
