package handler

import (
	"encoding/json"
	"net/http"
)

// 本ファイルは iOS プラットフォームパスキー API が要求するドメイン所有権表明
// （apple-app-site-association / AASA）を配信する `AASAHandler` を提供する
// （Issue #216 / design.md AASAHandler / Req 5.1〜5.4）。
//
// 応答 JSON はサーバ起動時に生成した immutable byte slice をキャッシュし、
// リクエストごとに json.Marshal を走らせない（性能・安全性）。
// ユーザー個別のトークン・秘密情報は一切含めず、環境変数由来の iOS App ID のみを
// `webcredentials.apps` に載せる（Req 5.3）。

// AASAHandler は `/.well-known/apple-app-site-association` を配信する HTTP ハンドラ
// である。生成時に指定された iOS App ID から静的 JSON を pre-marshal して保持し、
// リクエストごとに同じ byte 列を書き出す。
//
// 認証ミドルウェア・IP レート制限の対象外として router に登録される（Req 5.4）。
type AASAHandler struct {
	// body は Serve が書き出す pre-marshal 済みの JSON バイト列。
	body []byte
}

// aasaBody は AASA の JSON 構造。Apple の webcredentials 用途に限定した最小構造とし、
// applinks / appclips 等の他用途は本 spec のスコープ外（Req 5.3）。
type aasaBody struct {
	Webcredentials aasaWebcredentials `json:"webcredentials"`
}

type aasaWebcredentials struct {
	Apps []string `json:"apps"`
}

// NewAASAHandler は iOS App ID から AASAHandler を生成する。
//
// iOSAppID が空文字の場合は nil を返す（fail-closed）。router 側でも nil ハンドラを
// 検知して route 登録をスキップするため、iOSAppID 未設定の環境では
// `/.well-known/apple-app-site-association` が 404 になる（NFR 2.2）。
//
// iOSAppID は Apple の `TEAM_ID.com.example.feedman` 形式を想定するが、本 handler は
// 形式検証を行わず、環境変数から読み込んだ値をそのまま `webcredentials.apps` に含める
// （検証は config 層の責務）。
func NewAASAHandler(iOSAppID string) *AASAHandler {
	if iOSAppID == "" {
		return nil
	}
	body, err := json.Marshal(aasaBody{
		Webcredentials: aasaWebcredentials{
			Apps: []string{iOSAppID},
		},
	})
	if err != nil {
		// 上記構造は必ず marshal 成功する（string / stringスライスのみ）。
		// 万一失敗した場合は fail-closed として nil を返し、404 で応答させる。
		return nil
	}
	return &AASAHandler{body: body}
}

// Serve は `/.well-known/apple-app-site-association` の GET リクエストに応答する
// （Req 5.1, 5.2）。
//
//   - 200: pre-marshal 済み JSON バイト列（webcredentials.apps に指定 App ID）
//   - Content-Type: application/json
//   - Cache-Control: public, max-age=3600（1 時間キャッシュ、CDN 経由運用の余地を確保）
//
// 本 handler は認証・IP レート制限の外側に配置されるため、Session / Bearer なしで
// 呼び出される（Req 5.4）。応答内容にユーザー個別情報は含まれない（Req 5.3）。
func (h *AASAHandler) Serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	// h.body は起動時に marshal 済みで immutable。書き込み失敗はネットワーク断等に
	// 起因するため、追加ハンドリングは行わない（既存 healthHandler と同流儀）。
	_, _ = w.Write(h.body)
}
