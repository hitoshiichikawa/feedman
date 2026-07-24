package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 本 test は AASAHandler の HTTP 応答契約を検証する（Issue #216 / Req 5.1〜5.4）。
//
// 対応 AC:
//   - Req 5.1: GET で 200 応答
//   - Req 5.2: Content-Type / Cache-Control ヘッダ付与
//   - Req 5.3: ユーザー個別のトークン・秘密情報・環境依存ホスト名を含めない
//   - Req 5.4: 認証・IP レート制限の外側であることは router 側テスト（router_test.go）で担保

// TestAASAHandler_Serve_Returns200WithWebcredentials は AASA 応答が 200 で JSON を
// 返し、webcredentials.apps に指定した iOS App ID を 1 件だけ含めることを検証する
// （Req 5.1 / Req 5.3: 環境変数由来の App ID のみ）。
func TestAASAHandler_Serve_Returns200WithWebcredentials(t *testing.T) {
	// Arrange
	const appID = "TEAM1234.com.example.feedman"
	h := NewAASAHandler(appID)
	if h == nil {
		t.Fatalf("NewAASAHandler returned nil for non-empty appID")
	}
	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()

	// Act
	h.Serve(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		Webcredentials struct {
			Apps []string `json:"apps"`
		} `json:"webcredentials"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Webcredentials.Apps) != 1 || body.Webcredentials.Apps[0] != appID {
		t.Errorf("webcredentials.apps = %v, want [%q]", body.Webcredentials.Apps, appID)
	}
}

// TestAASAHandler_Serve_SetsContentTypeAndCacheControl はレスポンスヘッダに
// Content-Type: application/json と Cache-Control: public, max-age=3600 を付与することを
// 検証する（Req 5.2）。
func TestAASAHandler_Serve_SetsContentTypeAndCacheControl(t *testing.T) {
	// Arrange
	h := NewAASAHandler("TEAM1234.com.example.feedman")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()

	// Act
	h.Serve(w, req)

	// Assert
	resp := w.Result()
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=3600")
	}
}

// TestAASAHandler_Serve_DoesNotIncludeUserSecrets はレスポンス JSON にユーザー個別の
// トークン・secret・環境依存ホスト名等が含まれないことを検証する（Req 5.3）。
// webcredentials.apps 以外のトップレベルキーが存在しないことも併せて確認する。
func TestAASAHandler_Serve_DoesNotIncludeUserSecrets(t *testing.T) {
	// Arrange
	h := NewAASAHandler("TEAM1234.com.example.feedman")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w := httptest.NewRecorder()

	// Act
	h.Serve(w, req)

	// Assert: raw JSON をトップレベル map に decode し、キーが `webcredentials` のみで
	// あることを確認する。
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(w.Result().Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["webcredentials"]; !ok {
		t.Errorf("missing top-level key 'webcredentials'")
	}
	for key := range raw {
		if key != "webcredentials" {
			t.Errorf("unexpected top-level key %q in AASA response (Req 5.3)", key)
		}
	}
	// 応答本文に秘密情報 / ホスト名を思わせる部分文字列（"password" / "token" /
	// "secret" / "cookie" / "session" / "example.com" 等）が含まれないことを緩く検査する。
	rawBody := w.Body.String()
	for _, forbidden := range []string{"password", "token", "secret", "cookie", "session"} {
		if strings.Contains(strings.ToLower(rawBody), forbidden) {
			t.Errorf("AASA body contains forbidden substring %q (Req 5.3)", forbidden)
		}
	}
}

// TestNewAASAHandler_EmptyAppID_ReturnsNil は iOS App ID が空文字のとき handler 生成が
// nil を返すことを検証する（Req 5.4 の fail-closed 前提 / NFR 2.2）。router 側では nil
// ハンドラを検知して route 登録をスキップするため、iOSAppID 未設定時は
// `/.well-known/apple-app-site-association` が 404 になる。
func TestNewAASAHandler_EmptyAppID_ReturnsNil(t *testing.T) {
	if h := NewAASAHandler(""); h != nil {
		t.Errorf("NewAASAHandler(\"\") = %+v, want nil (fail-closed / NFR 2.2)", h)
	}
}

// TestAASAHandler_Serve_ImmutableBody は複数回リクエストしても同じ byte 列を返すことを
// 検証する（実装は pre-marshal 済みバイト列を保持する契約なので、書き込み内容が
// リクエスト間で変化しないことを確認する）。
func TestAASAHandler_Serve_ImmutableBody(t *testing.T) {
	// Arrange
	h := NewAASAHandler("TEAM1234.com.example.feedman")

	// Act
	req1 := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w1 := httptest.NewRecorder()
	h.Serve(w1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil)
	w2 := httptest.NewRecorder()
	h.Serve(w2, req2)

	// Assert
	if w1.Body.String() != w2.Body.String() {
		t.Errorf("response body differs between requests\n1st: %s\n2nd: %s", w1.Body.String(), w2.Body.String())
	}
}
