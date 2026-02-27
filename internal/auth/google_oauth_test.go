package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleOAuthProvider_GetLoginURL_ContainsRequiredParams(t *testing.T) {
	provider := NewGoogleOAuthProvider(GoogleOAuthConfig{
		ClientID:    "test-client-id",
		RedirectURL: "http://localhost:8080/auth/google/callback",
	})

	url := provider.GetLoginURL("test-state-value")

	// URLにclient_idが含まれること
	if url == "" {
		t.Fatal("expected non-empty URL")
	}

	// 基本的なパラメータの存在を確認
	tests := []struct {
		name     string
		contains string
	}{
		{"client_id", "client_id=test-client-id"},
		{"redirect_uri", "redirect_uri="},
		{"state", "state=test-state-value"},
		{"response_type", "response_type=code"},
		{"scope email", "email"},
		{"scope profile", "profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !containsStr(url, tt.contains) {
				t.Errorf("URL should contain %q, got %q", tt.contains, url)
			}
		})
	}
}

func TestGoogleOAuthProvider_ExchangeCode_Success(t *testing.T) {
	// テスト用のHTTPサーバーを立てる
	// Google Token Endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "test-refresh-token",
		})
	}))
	defer tokenServer.Close()

	// Google UserInfo Endpoint
	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Authorizationヘッダーの検証
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-access-token" {
			t.Errorf("unexpected Authorization header: %q", authHeader)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sub":   "google-sub-12345",
			"email": "user@gmail.com",
			"name":  "Google User",
		})
	}))
	defer userInfoServer.Close()

	provider := NewGoogleOAuthProvider(GoogleOAuthConfig{
		ClientID:        "test-client-id",
		ClientSecret:    "test-client-secret",
		RedirectURL:     "http://localhost:8080/auth/google/callback",
		TokenURL:        tokenServer.URL,
		UserInfoURL:     userInfoServer.URL,
	})

	ctx := context.Background()
	userInfo, err := provider.ExchangeCode(ctx, "test-auth-code")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}

	if userInfo == nil {
		t.Fatal("expected non-nil user info")
	}
	if userInfo.Provider != "google" {
		t.Errorf("provider = %q, want %q", userInfo.Provider, "google")
	}
	if userInfo.ProviderUserID != "google-sub-12345" {
		t.Errorf("providerUserID = %q, want %q", userInfo.ProviderUserID, "google-sub-12345")
	}
	if userInfo.Email != "user@gmail.com" {
		t.Errorf("email = %q, want %q", userInfo.Email, "user@gmail.com")
	}
	if userInfo.Name != "Google User" {
		t.Errorf("name = %q, want %q", userInfo.Name, "Google User")
	}
}

func TestGoogleOAuthProvider_ExchangeCode_TokenError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_grant",
			"error_description": "Code was already redeemed.",
		})
	}))
	defer tokenServer.Close()

	provider := NewGoogleOAuthProvider(GoogleOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/google/callback",
		TokenURL:     tokenServer.URL,
	})

	ctx := context.Background()
	_, err := provider.ExchangeCode(ctx, "invalid-code")
	if err == nil {
		t.Fatal("expected error from ExchangeCode with invalid code")
	}
}

func TestGoogleOAuthProvider_ExchangeCode_UserInfoError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer userInfoServer.Close()

	provider := NewGoogleOAuthProvider(GoogleOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/google/callback",
		TokenURL:     tokenServer.URL,
		UserInfoURL:  userInfoServer.URL,
	})

	ctx := context.Background()
	_, err := provider.ExchangeCode(ctx, "valid-code")
	if err == nil {
		t.Fatal("expected error from ExchangeCode when user info fetch fails")
	}
}

// containsStr は文字列sにsubstrが含まれるかチェックするヘルパー。
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
