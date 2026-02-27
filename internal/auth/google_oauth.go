package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultGoogleAuthURL    = "https://accounts.google.com/o/oauth2/auth"
	defaultGoogleTokenURL   = "https://oauth2.googleapis.com/token"
	defaultGoogleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// GoogleOAuthConfig はGoogle OAuthプロバイダーの設定。
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string

	// テスト用にオーバーライド可能なURL
	AuthURL     string
	TokenURL    string
	UserInfoURL string
}

// GoogleOAuthProvider はGoogle OAuth 2.0による認証を提供する。
type GoogleOAuthProvider struct {
	config GoogleOAuthConfig
}

// NewGoogleOAuthProvider はGoogleOAuthProviderを生成する。
func NewGoogleOAuthProvider(config GoogleOAuthConfig) *GoogleOAuthProvider {
	if config.AuthURL == "" {
		config.AuthURL = defaultGoogleAuthURL
	}
	if config.TokenURL == "" {
		config.TokenURL = defaultGoogleTokenURL
	}
	if config.UserInfoURL == "" {
		config.UserInfoURL = defaultGoogleUserInfoURL
	}
	return &GoogleOAuthProvider{config: config}
}

// GetLoginURL はGoogle OAuthの認証URLを生成する。
// スコープにはemail, profileを含む。
func (p *GoogleOAuthProvider) GetLoginURL(state string) string {
	params := url.Values{
		"client_id":     {p.config.ClientID},
		"redirect_uri":  {p.config.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"access_type":   {"offline"},
	}
	return p.config.AuthURL + "?" + params.Encode()
}

// googleTokenResponse はGoogleのトークンエンドポイントのレスポンス。
type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// googleUserInfo はGoogleのユーザー情報エンドポイントのレスポンス。
type googleUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ExchangeCode は認可コードをアクセストークンに交換し、ユーザー情報を取得する。
func (p *GoogleOAuthProvider) ExchangeCode(ctx context.Context, code string) (*OAuthUserInfo, error) {
	// 1. 認可コードをアクセストークンに交換
	tokenResp, err := p.exchangeToken(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	// 2. アクセストークンでユーザー情報を取得
	userInfo, err := p.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	return &OAuthUserInfo{
		ProviderUserID: userInfo.Sub,
		Email:          userInfo.Email,
		Name:           userInfo.Name,
		Provider:       "google",
	}, nil
}

// exchangeToken は認可コードをアクセストークンに交換する。
func (p *GoogleOAuthProvider) exchangeToken(ctx context.Context, code string) (*googleTokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
		"redirect_uri":  {p.config.RedirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp googleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}

	return &tokenResp, nil
}

// fetchUserInfo はアクセストークンでGoogleのユーザー情報を取得する。
func (p *GoogleOAuthProvider) fetchUserInfo(ctx context.Context, accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user info fetch failed with status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo googleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %w", err)
	}

	if userInfo.Sub == "" {
		return nil, fmt.Errorf("empty sub in user info response")
	}

	return &userInfo, nil
}

// compile-time interface check
var _ OAuthProvider = (*GoogleOAuthProvider)(nil)
