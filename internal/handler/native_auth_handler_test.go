package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hitoshi/feedman/internal/auth"
)

// mockTokenExchangeService は TokenExchangeService 最小 IF の record-and-return モック。
type mockTokenExchangeService struct {
	exchangeFn   func(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error)
	callCount    int
	lastAuthCode string
	lastVerifier string
}

func (m *mockTokenExchangeService) ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
	m.callCount++
	m.lastAuthCode = authCode
	m.lastVerifier = codeVerifier
	if m.exchangeFn != nil {
		return m.exchangeFn(ctx, authCode, codeVerifier)
	}
	return nil, fmt.Errorf("not configured")
}

// newRequest は handler 単体テスト用の JSON POST リクエストを生成する。
func newRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// --- 正常系 ---

// TestNativeAuthHandler_Token_Success は service が成功した場合の応答契約を検証する
// （Req 1.1, 1.6: JSON フィールド名 / token_type=Bearer / expires_in=900）。
func TestNativeAuthHandler_Token_Success(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{
		exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
			return &auth.TokenPair{
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
				ExpiresIn:    900,
			}, nil
		},
	}
	h := NewNativeAuthHandler(svc)
	req := newRequest(`{"auth_code":"plain-auth-code","code_verifier":"plain-verifier"}`)
	w := httptest.NewRecorder()

	// Act
	h.Token(w, req)

	// Assert: 200 / Content-Type
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	// Assert: ボディが snake_case の 4 フィールドを持つ（Req 1.6）
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["access_token"] != "test-access-token" {
		t.Errorf("access_token = %v, want %q", body["access_token"], "test-access-token")
	}
	if body["refresh_token"] != "test-refresh-token" {
		t.Errorf("refresh_token = %v, want %q", body["refresh_token"], "test-refresh-token")
	}
	if body["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want %q (Req 1.1)", body["token_type"], "Bearer")
	}
	// JSON number は float64 でデコードされる
	if v, _ := body["expires_in"].(float64); v != 900 {
		t.Errorf("expires_in = %v, want 900 (Req 1.4)", v)
	}

	// Assert: service が plain な値で呼ばれた（hash 化は service 内部で行う）
	if svc.callCount != 1 {
		t.Errorf("service called %d times, want 1", svc.callCount)
	}
	if svc.lastAuthCode != "plain-auth-code" {
		t.Errorf("service.lastAuthCode = %q, want %q", svc.lastAuthCode, "plain-auth-code")
	}
	if svc.lastVerifier != "plain-verifier" {
		t.Errorf("service.lastVerifier = %q, want %q", svc.lastVerifier, "plain-verifier")
	}
}

// --- 異常系 ---

// TestNativeAuthHandler_Token_InvalidGrant は service が ErrInvalidGrant を返したとき
// 400 INVALID_GRANT 応答を返すことを検証する（Req 2.1, 2.2, 2.3, 2.6）。
func TestNativeAuthHandler_Token_InvalidGrant(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "sentinel そのものを返したとき 400 INVALID_GRANT", err: auth.ErrInvalidGrant},
		{
			name: "ErrInvalidGrant を wrap したエラーでも 400 INVALID_GRANT",
			err:  fmt.Errorf("verifier mismatch: %w", auth.ErrInvalidGrant),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockTokenExchangeService{
				exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
					return nil, tc.err
				},
			}
			h := NewNativeAuthHandler(svc)
			req := newRequest(`{"auth_code":"x","code_verifier":"y"}`)
			w := httptest.NewRecorder()

			// Act
			h.Token(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != "INVALID_GRANT" {
				t.Errorf("code = %v, want %q (Req 2.6)", body["code"], "INVALID_GRANT")
			}
			if body["category"] != "auth" {
				t.Errorf("category = %v, want %q", body["category"], "auth")
			}
			// 応答に詳細な拒否理由が反射されていない（Req 2.6 / NFR 1.5）
			if msg, _ := body["message"].(string); strings.Contains(msg, "verifier") || strings.Contains(msg, "expired") || strings.Contains(msg, "used") {
				t.Errorf("message %q must not differentiate rejection reasons (Req 2.6)", msg)
			}
		})
	}
}

// TestNativeAuthHandler_Token_InvalidJSON は不正な JSON で 400 INVALID_REQUEST を
// 返すことを検証する（Req 2.5）。
func TestNativeAuthHandler_Token_InvalidJSON(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{}
	h := NewNativeAuthHandler(svc)
	req := newRequest(`{not valid json`)
	w := httptest.NewRecorder()

	// Act
	h.Token(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "INVALID_REQUEST" {
		t.Errorf("code = %v, want %q (Req 2.5)", body["code"], "INVALID_REQUEST")
	}
	if body["category"] != "validation" {
		t.Errorf("category = %v, want %q", body["category"], "validation")
	}
	// service は呼ばれない
	if svc.callCount != 0 {
		t.Errorf("service called %d times, want 0 (JSON 不正は service 到達前に拒否)", svc.callCount)
	}
}

// TestNativeAuthHandler_Token_MissingFields は必須フィールド欠落で 400 INVALID_REQUEST
// を返すことを検証する（Req 2.5）。
func TestNativeAuthHandler_Token_MissingFields(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "auth_code 欠落のとき 400 INVALID_REQUEST", body: `{"code_verifier":"v"}`},
		{name: "code_verifier 欠落のとき 400 INVALID_REQUEST", body: `{"auth_code":"a"}`},
		{name: "両方欠落のとき 400 INVALID_REQUEST", body: `{}`},
		{name: "auth_code が空文字のとき 400 INVALID_REQUEST", body: `{"auth_code":"","code_verifier":"v"}`},
		{name: "code_verifier が空文字のとき 400 INVALID_REQUEST", body: `{"auth_code":"a","code_verifier":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockTokenExchangeService{}
			h := NewNativeAuthHandler(svc)
			req := newRequest(tc.body)
			w := httptest.NewRecorder()

			// Act
			h.Token(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if body["code"] != "INVALID_REQUEST" {
				t.Errorf("code = %v, want %q (Req 2.5)", body["code"], "INVALID_REQUEST")
			}
			if svc.callCount != 0 {
				t.Errorf("service called %d times, want 0", svc.callCount)
			}
		})
	}
}

// TestNativeAuthHandler_Token_InternalError は service が ErrInvalidGrant 以外の
// エラーを返したとき 500 INTERNAL_ERROR を返すことを検証する（NFR 1.5）。
func TestNativeAuthHandler_Token_InternalError(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{
		exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error) {
			return nil, errors.New("db connection refused")
		},
	}
	h := NewNativeAuthHandler(svc)
	req := newRequest(`{"auth_code":"a","code_verifier":"v"}`)
	w := httptest.NewRecorder()

	// Act
	h.Token(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "INTERNAL_ERROR" {
		t.Errorf("code = %v, want %q", body["code"], "INTERNAL_ERROR")
	}
	// 内部詳細を反射しない（NFR 1.5）
	if msg, _ := body["message"].(string); strings.Contains(msg, "db connection refused") {
		t.Errorf("message %q leaks internal error detail (NFR 1.5)", msg)
	}
}

// TestNativeAuthHandler_Token_UnknownFieldRejected は未知のフィールドを含む JSON で
// 400 INVALID_REQUEST を返すことを検証する（DisallowUnknownFields による安全側挙動）。
func TestNativeAuthHandler_Token_UnknownFieldRejected(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{}
	h := NewNativeAuthHandler(svc)
	req := newRequest(`{"auth_code":"a","code_verifier":"v","device_label":"iPhone"}`)
	w := httptest.NewRecorder()

	// Act
	h.Token(w, req)

	// Assert: device_label 等の未知フィールドは out of scope（design.md Out of Scope）。
	// 厳密モードで拒否することで、将来の互換的拡張時に挙動を変える余地を残す。
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (未知フィールドは厳密に拒否)", resp.StatusCode, http.StatusBadRequest)
	}
	if svc.callCount != 0 {
		t.Errorf("service called %d times, want 0 (未知フィールドは service 到達前に拒否)", svc.callCount)
	}
}
