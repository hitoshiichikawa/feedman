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
	"time"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/model"
)

// mockTokenExchangeService は TokenExchangeService 最小 IF の record-and-return モック。
// rotateFn / lastRefreshToken / rotateCalls は Issue #167 で、revokeFn / revokeCalls /
// lastRevokeToken は Issue #168 で追加された。
type mockTokenExchangeService struct {
	exchangeFn       func(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error)
	rotateFn         func(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	revokeFn         func(ctx context.Context, refreshToken string) error
	callCount        int
	rotateCalls      int
	revokeCalls      int
	lastAuthCode     string
	lastVerifier     string
	lastRefreshToken string
	lastRevokeToken  string
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

func (m *mockTokenExchangeService) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	m.revokeCalls++
	m.lastRevokeToken = refreshToken
	if m.revokeFn != nil {
		return m.revokeFn(ctx, refreshToken)
	}
	return nil
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

// --- Revoke のテスト（Issue #168 / design.md Testing Strategy 8） ---

// newRevokeRequest は Revoke handler 単体テスト用の JSON POST リクエストを生成する。
func newRevokeRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/revoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestNativeAuthHandler_Revoke_Success は service が成功した場合に 204 + ボディなしを
// 返すことを検証する（Req 2.1）。
func TestNativeAuthHandler_Revoke_Success(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{}
	h := NewNativeAuthHandler(svc)
	req := newRevokeRequest(`{"refresh_token":"plain-refresh-token"}`)
	w := httptest.NewRecorder()

	// Act
	h.Revoke(w, req)

	// Assert: 204 / ボディなし
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (Req 2.1)", resp.StatusCode, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body = %q, want empty (204 はボディなし)", w.Body.String())
	}

	// Assert: service が plain な値で呼ばれた（hash 化は service 内部で行う）
	if svc.revokeCalls != 1 {
		t.Errorf("service.RevokeRefreshToken called %d times, want 1", svc.revokeCalls)
	}
	if svc.lastRevokeToken != "plain-refresh-token" {
		t.Errorf("service.lastRevokeToken = %q, want %q",
			svc.lastRevokeToken, "plain-refresh-token")
	}
}

// TestNativeAuthHandler_Revoke_UnknownTokenAlso204 は不明 token（service が no-op nil を
// 返す）でも既知 token と区別できない同一の 204 を返すことを検証する
// （Req 2.2 / NFR 1.2: 冪等・列挙オラクルなし）。
func TestNativeAuthHandler_Revoke_UnknownTokenAlso204(t *testing.T) {
	// Arrange: service は不明 token に対して nil（no-op 成功）を返す
	svc := &mockTokenExchangeService{
		revokeFn: func(ctx context.Context, refreshToken string) error {
			return nil
		},
	}
	h := NewNativeAuthHandler(svc)
	req := newRevokeRequest(`{"refresh_token":"unknown-token"}`)
	w := httptest.NewRecorder()

	// Act
	h.Revoke(w, req)

	// Assert: 既知 token の場合と同一の 204 + ボディなし（区別できない）
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (Req 2.2: 不明 token でも同一応答)", resp.StatusCode, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body = %q, want empty (NFR 1.2: 存在有無を推測させない)", w.Body.String())
	}
}

// TestNativeAuthHandler_Revoke_InvalidJSON は不正 JSON ボディに対して
// 400 INVALID_REQUEST を返し、service に到達しないことを検証する（Req 2.5）。
func TestNativeAuthHandler_Revoke_InvalidJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "壊れた JSON", body: `{"refresh_token":`},
		{name: "JSON ではない文字列", body: `not-json`},
		{name: "空ボディ", body: ``},
		{name: "未知フィールド", body: `{"refresh_token":"x","extra":"y"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockTokenExchangeService{}
			h := NewNativeAuthHandler(svc)
			req := newRevokeRequest(tc.body)
			w := httptest.NewRecorder()

			// Act
			h.Revoke(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (Req 2.5)", resp.StatusCode, http.StatusBadRequest)
			}
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != "INVALID_REQUEST" {
				t.Errorf("code = %v, want %q", body["code"], "INVALID_REQUEST")
			}
			if svc.revokeCalls != 0 {
				t.Errorf("service called %d times, want 0 (入力不正は service 到達前に拒否)", svc.revokeCalls)
			}
		})
	}
}

// TestNativeAuthHandler_Revoke_MissingField は refresh_token 欠落（または空文字）に対して
// 400 INVALID_REQUEST を返すことを検証する（Req 2.5 / 境界値: 空入力）。
func TestNativeAuthHandler_Revoke_MissingField(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "refresh_token フィールドなし", body: `{}`},
		{name: "refresh_token が空文字", body: `{"refresh_token":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockTokenExchangeService{}
			h := NewNativeAuthHandler(svc)
			req := newRevokeRequest(tc.body)
			w := httptest.NewRecorder()

			// Act
			h.Revoke(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (Req 2.5)", resp.StatusCode, http.StatusBadRequest)
			}
			if svc.revokeCalls != 0 {
				t.Errorf("service called %d times, want 0", svc.revokeCalls)
			}
		})
	}
}

// TestNativeAuthHandler_Revoke_InternalError は service が infra エラーを返した場合に
// 500 INTERNAL_ERROR を返し、内部詳細を反射しないことを検証する（NFR 1.3）。
func TestNativeAuthHandler_Revoke_InternalError(t *testing.T) {
	// Arrange
	svc := &mockTokenExchangeService{
		revokeFn: func(ctx context.Context, refreshToken string) error {
			return errors.New("db connection refused")
		},
	}
	h := NewNativeAuthHandler(svc)
	req := newRevokeRequest(`{"refresh_token":"x"}`)
	w := httptest.NewRecorder()

	// Act
	h.Revoke(w, req)

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

// --- Session handler のテスト（Issue #223 / task 2 / design.md §NativeAuthHandler.Session） ---

// mockSessionExchangeService は SessionExchanger 最小 IF の record-and-return モック。
// 既存 mockTokenExchangeService と同 idiom（testability 優先）。
type mockSessionExchangeService struct {
	exchangeFn   func(ctx context.Context, authCode, codeVerifier string) (*model.Session, error)
	callCount    int
	lastAuthCode string
	lastVerifier string
}

func (m *mockSessionExchangeService) ExchangeAuthCodeForSession(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
	m.callCount++
	m.lastAuthCode = authCode
	m.lastVerifier = codeVerifier
	if m.exchangeFn != nil {
		return m.exchangeFn(ctx, authCode, codeVerifier)
	}
	return nil, fmt.Errorf("not configured")
}

// newSessionRequest は Session handler 単体テスト用の JSON POST リクエストを生成する。
func newSessionRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// newSessionHandlerWith は SessionExchanger と Cookie 設定値を注入した NativeAuthHandler を返す。
// 既存 Token / Refresh / Revoke tests への影響を避けるため WithSessionExchange オプションを用いる。
func newSessionHandlerWith(exchange SessionExchanger, cookieDomain string, cookieSecure bool, sessionMaxAge int) *NativeAuthHandler {
	// TokenExchangeService は Session 経路で不要のため空 mock を渡す（既存パターン踏襲）。
	return NewNativeAuthHandler(
		&mockTokenExchangeService{},
		WithSessionExchange(exchange, cookieDomain, cookieSecure, sessionMaxAge),
	)
}

// TestNativeAuthHandler_Session_Success は service が成功した場合の 204 応答と
// Set-Cookie 属性が既存 Google OAuth Callback（AuthHandler.Callback 手順 5）と完全一致する
// ことを検証する（Req 3.1 / 4.2 / design.md §NativeAuthHandler.Session）。
func TestNativeAuthHandler_Session_Success(t *testing.T) {
	// Arrange
	const (
		cookieDomain  = "feedman.example.com"
		cookieSecure  = true
		sessionMaxAge = 86400
	)
	svc := &mockSessionExchangeService{
		exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
			return &model.Session{
				ID:        "new-session-id",
				UserID:    "user-123",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				CreatedAt: time.Now(),
			}, nil
		},
	}
	h := newSessionHandlerWith(svc, cookieDomain, cookieSecure, sessionMaxAge)
	req := newSessionRequest(`{"auth_code":"plain-auth-code","code_verifier":"plain-verifier"}`)
	w := httptest.NewRecorder()

	// Act
	h.Session(w, req)

	// Assert: 204 / ボディなし
	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (Req 3.1 / 4.2: 成功時 204)", resp.StatusCode, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body = %q, want empty (204 はボディなし)", w.Body.String())
	}

	// Assert: Set-Cookie 属性が既存 Google OAuth Callback（auth_handler.go 手順 5）と一致
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be set")
	}
	if sessionCookie.Value != "new-session-id" {
		t.Errorf("cookie Value = %q, want %q", sessionCookie.Value, "new-session-id")
	}
	if sessionCookie.Path != "/" {
		t.Errorf("cookie Path = %q, want %q", sessionCookie.Path, "/")
	}
	if sessionCookie.Domain != cookieDomain {
		t.Errorf("cookie Domain = %q, want %q", sessionCookie.Domain, cookieDomain)
	}
	if sessionCookie.MaxAge != sessionMaxAge {
		t.Errorf("cookie MaxAge = %d, want %d", sessionCookie.MaxAge, sessionMaxAge)
	}
	if !sessionCookie.HttpOnly {
		t.Error("cookie should be HttpOnly (既存 OAuth Callback 準拠)")
	}
	if !sessionCookie.Secure {
		t.Error("cookie should be Secure when cookieSecure=true (既存 OAuth Callback 準拠)")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want %v (既存 OAuth Callback 準拠)",
			sessionCookie.SameSite, http.SameSiteLaxMode)
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

// TestNativeAuthHandler_Session_InvalidRequest は JSON 不正・必須フィールド欠落で
// 400 INVALID_REQUEST を返すことを検証する（design.md §NativeAuthHandler.Session）。
func TestNativeAuthHandler_Session_InvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "壊れた JSON で 400 INVALID_REQUEST", body: `{not valid json`},
		{name: "空ボディで 400 INVALID_REQUEST", body: ``},
		{name: "auth_code 欠落のとき 400 INVALID_REQUEST", body: `{"code_verifier":"v"}`},
		{name: "code_verifier 欠落のとき 400 INVALID_REQUEST", body: `{"auth_code":"a"}`},
		{name: "両方欠落のとき 400 INVALID_REQUEST", body: `{}`},
		{name: "auth_code が空文字のとき 400 INVALID_REQUEST", body: `{"auth_code":"","code_verifier":"v"}`},
		{name: "code_verifier が空文字のとき 400 INVALID_REQUEST", body: `{"auth_code":"a","code_verifier":""}`},
		{name: "未知フィールドで 400 INVALID_REQUEST", body: `{"auth_code":"a","code_verifier":"v","extra":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := &mockSessionExchangeService{}
			h := newSessionHandlerWith(svc, "", false, 86400)
			req := newSessionRequest(tc.body)
			w := httptest.NewRecorder()

			// Act
			h.Session(w, req)

			// Assert
			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (400 INVALID_REQUEST)", resp.StatusCode, http.StatusBadRequest)
			}
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if body["code"] != "INVALID_REQUEST" {
				t.Errorf("code = %v, want %q", body["code"], "INVALID_REQUEST")
			}
			if body["category"] != "validation" {
				t.Errorf("category = %v, want %q", body["category"], "validation")
			}
			// service には到達しない（入力不正で早期 return / NFR 1.1: 入力を反射しない）
			if svc.callCount != 0 {
				t.Errorf("service called %d times, want 0 (入力不正は service 到達前に拒否)", svc.callCount)
			}
			// Set-Cookie は発行されない
			for _, c := range resp.Cookies() {
				if c.Name == "session_id" {
					t.Errorf("session_id cookie should not be set on 400 (got %q)", c.Value)
				}
			}
		})
	}
}

// TestNativeAuthHandler_Session_InvalidGrant は service が ErrInvalidGrant を返したとき
// 400 INVALID_GRANT 応答を返すことを検証する（design.md §NativeAuthHandler.Session /
// Req 3.4: 拒否理由の内部区別を反射しない / NFR 1.1）。
func TestNativeAuthHandler_Session_InvalidGrant(t *testing.T) {
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
			svc := &mockSessionExchangeService{
				exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
					return nil, tc.err
				},
			}
			h := newSessionHandlerWith(svc, "feedman.example.com", true, 86400)
			req := newSessionRequest(`{"auth_code":"a","code_verifier":"v"}`)
			w := httptest.NewRecorder()

			// Act
			h.Session(w, req)

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
				t.Errorf("code = %v, want %q", body["code"], "INVALID_GRANT")
			}
			if body["category"] != "auth" {
				t.Errorf("category = %v, want %q", body["category"], "auth")
			}
			// 応答に詳細な拒否理由が反射されていない（Req 3.4 / NFR 1.1）
			if msg, _ := body["message"].(string); strings.Contains(msg, "verifier") ||
				strings.Contains(msg, "expired") || strings.Contains(msg, "used") ||
				strings.Contains(msg, "mismatch") {
				t.Errorf("message %q must not differentiate rejection reasons (Req 3.4)", msg)
			}
			// Set-Cookie は発行されない（拒否時に session を発行しない）
			for _, c := range resp.Cookies() {
				if c.Name == "session_id" {
					t.Errorf("session_id cookie should not be set on 400 INVALID_GRANT (got %q)", c.Value)
				}
			}
		})
	}
}

// TestNativeAuthHandler_Session_InternalError は service が ErrInvalidGrant 以外の infra
// エラーを返したとき 500 INTERNAL_ERROR を返し、内部詳細を反射しないことを検証する
// （NFR 1.1）。
func TestNativeAuthHandler_Session_InternalError(t *testing.T) {
	// Arrange
	svc := &mockSessionExchangeService{
		exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
			return nil, errors.New("db connection refused")
		},
	}
	h := newSessionHandlerWith(svc, "", false, 86400)
	req := newSessionRequest(`{"auth_code":"a","code_verifier":"v"}`)
	w := httptest.NewRecorder()

	// Act
	h.Session(w, req)

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
	// 内部詳細を反射しない（NFR 1.1）
	if msg, _ := body["message"].(string); strings.Contains(msg, "db connection refused") {
		t.Errorf("message %q leaks internal error detail (NFR 1.1)", msg)
	}
	// Set-Cookie は発行されない
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			t.Errorf("session_id cookie should not be set on 500 (got %q)", c.Value)
		}
	}
}

// --- Session handler の CSRF 対策（Issue #223 review #2: Content-Type / Origin 検証） ---

// newSessionSuccessSvc は成功固定の SessionExchanger を返す（CSRF テストの Arrange 共通化）。
func newSessionSuccessSvc() *mockSessionExchangeService {
	return &mockSessionExchangeService{
		exchangeFn: func(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
			return &model.Session{
				ID:        "csrf-session-id",
				UserID:    "user-csrf",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				CreatedAt: time.Now(),
			}, nil
		},
	}
}

// newSessionHandlerWithOrigin は allowedOrigin を注入した Session handler を返す。
func newSessionHandlerWithOrigin(exchange SessionExchanger, allowedOrigin string) *NativeAuthHandler {
	return NewNativeAuthHandler(
		&mockTokenExchangeService{},
		WithSessionExchange(exchange, "example.com", true, 86400),
		WithSessionAllowedOrigin(allowedOrigin),
	)
}

// TestNativeAuthHandler_Session_RejectsNonJSONContentType は Content-Type が
// application/json でない POST を 415 UNSUPPORTED_MEDIA_TYPE で弾き、service に到達せず
// Cookie も発行しないことを検証する（Issue #223 review #2: login CSRF 対策 / form ベース
// simple request の遮断）。
func TestNativeAuthHandler_Session_RejectsNonJSONContentType(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
	}{
		{name: "text/plain（form ベース CSRF の simple request）", contentType: "text/plain"},
		{name: "application/x-www-form-urlencoded", contentType: "application/x-www-form-urlencoded"},
		{name: "multipart/form-data", contentType: "multipart/form-data; boundary=x"},
		{name: "Content-Type ヘッダ欠落", contentType: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := newSessionSuccessSvc()
			h := newSessionHandlerWithOrigin(svc, "https://feedman.example")
			req := httptest.NewRequest(http.MethodPost, "/api/auth/session",
				strings.NewReader(`{"auth_code":"a","code_verifier":"v"}`))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()

			// Act
			h.Session(w, req)

			// Assert: 415 / service 未到達 / Cookie 未発行
			resp := w.Result()
			if resp.StatusCode != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want %d (415 UNSUPPORTED_MEDIA_TYPE)",
					resp.StatusCode, http.StatusUnsupportedMediaType)
			}
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if body["code"] != "UNSUPPORTED_MEDIA_TYPE" {
				t.Errorf("code = %v, want %q", body["code"], "UNSUPPORTED_MEDIA_TYPE")
			}
			if svc.callCount != 0 {
				t.Errorf("service called %d times, want 0 (Content-Type 不正は service 到達前に拒否)", svc.callCount)
			}
			for _, c := range resp.Cookies() {
				if c.Name == "session_id" {
					t.Errorf("session_id cookie should not be set on 415 (got %q)", c.Value)
				}
			}
		})
	}
}

// TestNativeAuthHandler_Session_ContentTypeWithCharsetAccepted は
// application/json にパラメータ（charset）が付いていても受理することを検証する
// （実ブラウザは `application/json; charset=utf-8` を送る場合がある）。
func TestNativeAuthHandler_Session_ContentTypeWithCharsetAccepted(t *testing.T) {
	// Arrange
	svc := newSessionSuccessSvc()
	h := newSessionHandlerWithOrigin(svc, "")
	req := httptest.NewRequest(http.MethodPost, "/api/auth/session",
		strings.NewReader(`{"auth_code":"a","code_verifier":"v"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()

	// Act
	h.Session(w, req)

	// Assert: 204 成功
	if got := w.Result().StatusCode; got != http.StatusNoContent {
		t.Errorf("status = %d, want 204 (charset 付き application/json は受理)", got)
	}
	if svc.callCount != 1 {
		t.Errorf("service called %d times, want 1", svc.callCount)
	}
}

// TestNativeAuthHandler_Session_RejectsDisallowedOrigin は Origin ヘッダが許可オリジンと
// 一致しない cross-site POST を 403 FORBIDDEN_ORIGIN で弾き、service に到達せず Cookie も
// 発行しないことを検証する（Issue #223 review #2: login CSRF 対策）。
func TestNativeAuthHandler_Session_RejectsDisallowedOrigin(t *testing.T) {
	// Arrange
	svc := newSessionSuccessSvc()
	h := newSessionHandlerWithOrigin(svc, "https://feedman.example")
	req := httptest.NewRequest(http.MethodPost, "/api/auth/session",
		strings.NewReader(`{"auth_code":"a","code_verifier":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()

	// Act
	h.Session(w, req)

	// Assert: 403 / service 未到達 / Cookie 未発行 / origin 生値を反射しない
	resp := w.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (403 FORBIDDEN_ORIGIN)", resp.StatusCode, http.StatusForbidden)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "FORBIDDEN_ORIGIN" {
		t.Errorf("code = %v, want %q", body["code"], "FORBIDDEN_ORIGIN")
	}
	if msg, _ := body["message"].(string); strings.Contains(msg, "attacker.example") {
		t.Errorf("message %q must not reflect the origin value (NFR 1.1)", msg)
	}
	if svc.callCount != 0 {
		t.Errorf("service called %d times, want 0 (不許可 Origin は service 到達前に拒否)", svc.callCount)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			t.Errorf("session_id cookie should not be set on 403 (got %q)", c.Value)
		}
	}
}

// TestNativeAuthHandler_Session_AllowsMatchingOrAbsentOrigin は許可オリジン一致・Origin 不在・
// allowedOrigin 未配線の各ケースで 204 成功することを検証する（false-reject を避ける設計 /
// Issue #223 review #2）。
func TestNativeAuthHandler_Session_AllowsMatchingOrAbsentOrigin(t *testing.T) {
	cases := []struct {
		name          string
		allowedOrigin string
		reqOrigin     string
	}{
		{name: "Origin が許可オリジンと一致", allowedOrigin: "https://feedman.example", reqOrigin: "https://feedman.example"},
		{name: "Origin 不在（same-origin proxy 等でヘッダが落ちる）", allowedOrigin: "https://feedman.example", reqOrigin: ""},
		{name: "allowedOrigin 未配線なら Origin 検証をスキップ", allowedOrigin: "", reqOrigin: "https://anything.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc := newSessionSuccessSvc()
			h := newSessionHandlerWithOrigin(svc, tc.allowedOrigin)
			req := httptest.NewRequest(http.MethodPost, "/api/auth/session",
				strings.NewReader(`{"auth_code":"a","code_verifier":"v"}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.reqOrigin != "" {
				req.Header.Set("Origin", tc.reqOrigin)
			}
			w := httptest.NewRecorder()

			// Act
			h.Session(w, req)

			// Assert: 204 成功 + Set-Cookie
			resp := w.Result()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", resp.StatusCode)
			}
			if svc.callCount != 1 {
				t.Errorf("service called %d times, want 1", svc.callCount)
			}
			var found bool
			for _, c := range resp.Cookies() {
				if c.Name == "session_id" {
					found = true
				}
			}
			if !found {
				t.Error("session_id cookie should be set on 204 success")
			}
		})
	}
}
