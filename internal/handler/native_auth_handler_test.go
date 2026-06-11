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
// rotateFn / lastRefreshToken / rotateCalls は Issue #167 で追加された。
type mockTokenExchangeService struct {
	exchangeFn       func(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error)
	rotateFn         func(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	callCount        int
	rotateCalls      int
	lastAuthCode     string
	lastVerifier     string
	lastRefreshToken string
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

func (m *mockTokenExchangeService) RotateRefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	m.rotateCalls++
	m.lastRefreshToken = refreshToken
	if m.rotateFn != nil {
		return m.rotateFn(ctx, refreshToken)
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

// --- Refresh handler のテスト（Issue #167） ---

// newRefreshRequest は handler 単体テスト用の JSON POST /api/auth/refresh リクエストを生成する。
func newRefreshRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestNativeAuthHandler_Refresh_Success は service が成功した場合の応答契約を検証する
// （Req 1.1, 1.6: JSON フィールド名 / token_type=Bearer / expires_in=900）。
func TestNativeAuthHandler_Refresh_Success(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{
		rotateFn: func(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
			return &auth.TokenPair{
				AccessToken:  "new-access-token",
				RefreshToken: "new-refresh-token",
				ExpiresIn:    900,
			}, nil
		},
	}
	h := NewNativeAuthHandler(svc)
	req := newRefreshRequest(`{"refresh_token":"plain-refresh-token"}`)
	w := httptest.NewRecorder()

	// Act
	h.Refresh(w, req)

	// Assert: 200 / Content-Type
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	// Assert: ボディが snake_case の 4 フィールドを持つ（Req 1.6: token 交換と同形）
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["access_token"] != "new-access-token" {
		t.Errorf("access_token = %v, want %q", body["access_token"], "new-access-token")
	}
	if body["refresh_token"] != "new-refresh-token" {
		t.Errorf("refresh_token = %v, want %q", body["refresh_token"], "new-refresh-token")
	}
	if body["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want %q (Req 1.1)", body["token_type"], "Bearer")
	}
	if v, _ := body["expires_in"].(float64); v != 900 {
		t.Errorf("expires_in = %v, want 900 (Req 1.1)", v)
	}

	// Assert: service が plain な値で呼ばれた（hash 化は service 内部で行う）
	if svc.rotateCalls != 1 {
		t.Errorf("service.RotateRefreshToken called %d times, want 1", svc.rotateCalls)
	}
	if svc.lastRefreshToken != "plain-refresh-token" {
		t.Errorf("service.lastRefreshToken = %q, want %q",
			svc.lastRefreshToken, "plain-refresh-token")
	}
}

// TestNativeAuthHandler_Refresh_InvalidRefreshToken は service が ErrInvalidRefreshToken を
// 返したとき 401 INVALID_REFRESH_TOKEN 応答を返すことを検証する（Req 2.1, 2.2, 2.3, 2.4, 2.6 /
// SERVER.md §1.3）。
func TestNativeAuthHandler_Refresh_InvalidRefreshToken(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "sentinel そのものを返したとき 401 INVALID_REFRESH_TOKEN", err: auth.ErrInvalidRefreshToken},
		{
			name: "ErrInvalidRefreshToken を wrap したエラーでも 401 INVALID_REFRESH_TOKEN",
			err:  fmt.Errorf("token expired: %w", auth.ErrInvalidRefreshToken),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockTokenExchangeService{
				rotateFn: func(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
					return nil, tc.err
				},
			}
			h := NewNativeAuthHandler(svc)
			req := newRefreshRequest(`{"refresh_token":"plain-refresh-token"}`)
			w := httptest.NewRecorder()

			// Act
			h.Refresh(w, req)

			// Assert: 401 status code（SERVER.md §1.3。token 交換の 400 INVALID_GRANT とは異なる）
			resp := w.Result()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (SERVER.md §1.3: 401 INVALID_REFRESH_TOKEN)",
					resp.StatusCode, http.StatusUnauthorized)
			}
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != "INVALID_REFRESH_TOKEN" {
				t.Errorf("code = %v, want %q (Req 2.6)", body["code"], "INVALID_REFRESH_TOKEN")
			}
			if body["category"] != "auth" {
				t.Errorf("category = %v, want %q", body["category"], "auth")
			}
			// 応答に詳細な拒否理由が反射されていない（Req 2.6 / NFR 1.3）
			if msg, _ := body["message"].(string); strings.Contains(msg, "expired") || strings.Contains(msg, "revoked") || strings.Contains(msg, "rotated") || strings.Contains(msg, "unknown") {
				t.Errorf("message %q must not differentiate rejection reasons (Req 2.6)", msg)
			}
		})
	}
}

// TestNativeAuthHandler_Refresh_InvalidJSON は不正な JSON で 400 INVALID_REQUEST を
// 返すことを検証する（Req 2.5）。
func TestNativeAuthHandler_Refresh_InvalidJSON(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{}
	h := NewNativeAuthHandler(svc)
	req := newRefreshRequest(`{not valid json`)
	w := httptest.NewRecorder()

	// Act
	h.Refresh(w, req)

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
	if svc.rotateCalls != 0 {
		t.Errorf("service called %d times, want 0 (JSON 不正は service 到達前に拒否)", svc.rotateCalls)
	}
}

// TestNativeAuthHandler_Refresh_MissingField は必須フィールド欠落で 400 INVALID_REQUEST を
// 返すことを検証する（Req 2.5）。
func TestNativeAuthHandler_Refresh_MissingField(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "refresh_token 欠落のとき 400 INVALID_REQUEST", body: `{}`},
		{name: "refresh_token が空文字のとき 400 INVALID_REQUEST", body: `{"refresh_token":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockTokenExchangeService{}
			h := NewNativeAuthHandler(svc)
			req := newRefreshRequest(tc.body)
			w := httptest.NewRecorder()

			// Act
			h.Refresh(w, req)

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
			if svc.rotateCalls != 0 {
				t.Errorf("service called %d times, want 0", svc.rotateCalls)
			}
		})
	}
}

// TestNativeAuthHandler_Refresh_InternalError は service が ErrInvalidRefreshToken 以外の
// エラーを返したとき 500 INTERNAL_ERROR を返すことを検証する（NFR 1.3）。
func TestNativeAuthHandler_Refresh_InternalError(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{
		rotateFn: func(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
			return nil, errors.New("db connection refused")
		},
	}
	h := NewNativeAuthHandler(svc)
	req := newRefreshRequest(`{"refresh_token":"x"}`)
	w := httptest.NewRecorder()

	// Act
	h.Refresh(w, req)

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
	// 内部詳細を反射しない（NFR 1.3）
	if msg, _ := body["message"].(string); strings.Contains(msg, "db connection refused") {
		t.Errorf("message %q leaks internal error detail (NFR 1.3)", msg)
	}
}

// TestNativeAuthHandler_Refresh_UnknownFieldRejected は未知のフィールドを含む JSON で
// 400 INVALID_REQUEST を返すことを検証する（DisallowUnknownFields による安全側挙動 /
// design.md Out of Scope: device_label）。
func TestNativeAuthHandler_Refresh_UnknownFieldRejected(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{}
	h := NewNativeAuthHandler(svc)
	req := newRefreshRequest(`{"refresh_token":"a","device_label":"iPhone"}`)
	w := httptest.NewRecorder()

	// Act
	h.Refresh(w, req)

	// Assert
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (未知フィールドは厳密に拒否)", resp.StatusCode, http.StatusBadRequest)
	}
	if svc.rotateCalls != 0 {
		t.Errorf("service called %d times, want 0 (未知フィールドは service 到達前に拒否)", svc.rotateCalls)
	}
}
