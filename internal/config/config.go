package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config はアプリケーション全体の設定を保持する。
// 環境変数から起動時に1回読み込み、イミュータブルとして扱う。
type Config struct {
	// Database
	DatabaseURL string

	// OAuth
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	// Session
	SessionSecret string
	SessionMaxAge int

	// Fetch
	FetchTimeout       time.Duration
	FetchMaxSize       int64
	FetchMaxConcurrent int
	FetchInterval      time.Duration

	// Rate Limit
	RateLimitGeneral int
	RateLimitFeedReg int
	// RateLimitUnauthIP は未認証エンドポイント（/auth/google/login・/auth/google/callback・
	// /health）に適用する IP 単位レート制限の閾値（req/min/IP）。
	// RATE_LIMIT_UNAUTH_IP から読み込む。既定値は 30。不正値時は既定値にフォールバックする。
	RateLimitUnauthIP int

	// Hatebu
	HatebuTTL              time.Duration
	HatebuBatchInterval    time.Duration
	HatebuAPIInterval      time.Duration
	HatebuMaxCallsPerCycle int

	// Logging
	LogRetentionDays int

	// Server
	ServerPort string
	BaseURL    string

	// Cookie
	CookieSecure bool
	CookieDomain string

	// CORS
	CORSAllowedOrigin string

	// Security
	// HSTSEnabled は HSTS（Strict-Transport-Security）ヘッダーの出力可否を制御する。
	// 既定値は false（HSTS 非出力 = 本機能導入前と等価）。
	HSTSEnabled bool

	// Metrics
	// TrustedCIDRs は /metrics エンドポイントへのアクセスを許可する信頼ネットワーク範囲（CIDR 表記）。
	// METRICS_TRUSTED_CIDRS（カンマ区切り）から読み込む。未設定時は空スライス。
	// 各要素の検証（不正 CIDR の判定）はミドルウェア側に委譲する。
	TrustedCIDRs []string
	// MetricsPort は worker プロセスがメトリクスを公開する listener のポート。
	// METRICS_PORT から読み込む。既定値は "9090"。
	MetricsPort string

	// Native Auth (Issue #166)
	// NativeAuthJWTSecret は POST /api/auth/token が発行する access token (JWT) の
	// 署名鍵（HS256 対称鍵）。NATIVE_AUTH_JWT_SECRET から読み込む。
	// 未設定（空文字）の場合は POST /api/auth/token をルーティングに登録せず、
	// 既存デプロイの起動・挙動を変更しない（fail-closed、Req 3.2 / NFR 2.2）。
	// 平文をログに残さないこと（NFR 1.2）。
	NativeAuthJWTSecret string
	// NativeAuthJWTKid は access token (JWT) ヘッダに含める鍵識別子（kid）。
	// NATIVE_AUTH_JWT_KID から読み込み、未設定時は "v1" を採用する（Req 3.5）。
	// 将来の鍵ローテーションに備えて識別子を含めるためだけのもので、ローテーション実装は別 Issue。
	NativeAuthJWTKid string

	// Passkey / WebAuthn (Issue #216)
	// WebAuthnRPID は WebAuthn Relying Party ID（scheme / port を含まないドメイン、例: "example.com"）。
	// WEBAUTHN_RP_ID から読み込む。空文字の場合、WEBAUTHN_ORIGINS の設定有無に関わらず passkey
	// handler は生成されず、`/api/passkey/*` 系ルートは登録されない（fail-closed / NFR 2.2 / Req 8.2）。
	WebAuthnRPID string
	// WebAuthnRPDisplayName はユーザーの authenticator 同意画面等に表示される RP 名。
	// WEBAUTHN_RP_DISPLAY_NAME から読み込む。未設定時は "Feedman" を採用する。
	WebAuthnRPDisplayName string
	// WebAuthnOrigins は WebAuthn ceremony が許容する origin リスト
	// （scheme + host [+ port] の完全形。例: "https://example.com,feedman://"）。
	// WEBAUTHN_ORIGINS からカンマ区切りで読み込む（既存 parseCommaSeparated を再利用）。
	// 空スライスの場合、WebAuthnRPID の設定有無に関わらず passkey handler は生成されない（fail-closed）。
	WebAuthnOrigins []string
	// WebAuthnIOSAppID は AASA の `webcredentials.apps` に載せる iOS App ID
	// （`TEAM_ID.com.example.feedman` 形式を想定するが、config 層では形式検証を行わない）。
	// WEBAUTHN_IOS_APP_ID から読み込む。空文字の場合は AASA handler が生成されず
	// `/.well-known/apple-app-site-association` は 404 になる（fail-closed / Req 5.1〜5.4）。
	// PasskeyHandler の nil 判定とは独立に決まる（iOS 連携を切り分けて無効化可能）。
	WebAuthnIOSAppID string
	// PasskeyChallengeTTL は WebAuthn challenge の有効期限（issue から consume までの上限）。
	// PASSKEY_CHALLENGE_TTL_SECONDS（秒単位の整数）から読み込む。未設定・不正値時は
	// 既定値 300 秒（5 分）にフォールバックする（Req 4.4 / 8.4）。
	PasskeyChallengeTTL time.Duration
}

// Load は環境変数からConfigを読み込む。
// 必須環境変数が未設定の場合はエラーを返す。
func Load() (*Config, error) {
	cfg := &Config{}

	// Required fields
	var missing []string

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}

	cfg.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	if cfg.GoogleClientID == "" {
		missing = append(missing, "GOOGLE_CLIENT_ID")
	}

	cfg.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	if cfg.GoogleClientSecret == "" {
		missing = append(missing, "GOOGLE_CLIENT_SECRET")
	}

	cfg.GoogleRedirectURL = os.Getenv("GOOGLE_REDIRECT_URL")
	if cfg.GoogleRedirectURL == "" {
		missing = append(missing, "GOOGLE_REDIRECT_URL")
	}

	cfg.SessionSecret = os.Getenv("SESSION_SECRET")
	if cfg.SessionSecret == "" {
		missing = append(missing, "SESSION_SECRET")
	}

	cfg.BaseURL = os.Getenv("BASE_URL")
	if cfg.BaseURL == "" {
		missing = append(missing, "BASE_URL")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("required environment variables are not set: %v", missing)
	}

	// Optional fields with defaults
	cfg.SessionMaxAge = getEnvInt("SESSION_MAX_AGE", 86400)
	cfg.FetchTimeout = getEnvDuration("FETCH_TIMEOUT", 10*time.Second)
	cfg.FetchMaxSize = getEnvInt64("FETCH_MAX_SIZE", 5242880)
	cfg.FetchMaxConcurrent = getEnvInt("FETCH_MAX_CONCURRENT", 10)
	cfg.FetchInterval = getEnvDuration("FETCH_INTERVAL", 5*time.Minute)
	cfg.RateLimitGeneral = getEnvInt("RATE_LIMIT_GENERAL", 120)
	cfg.RateLimitFeedReg = getEnvInt("RATE_LIMIT_FEED_REG", 10)
	cfg.RateLimitUnauthIP = getEnvInt("RATE_LIMIT_UNAUTH_IP", 30)
	cfg.HatebuTTL = getEnvDuration("HATEBU_TTL", 24*time.Hour)
	cfg.HatebuBatchInterval = getEnvDuration("HATEBU_BATCH_INTERVAL", 10*time.Minute)
	cfg.HatebuAPIInterval = getEnvDuration("HATEBU_API_INTERVAL", 5*time.Second)
	cfg.HatebuMaxCallsPerCycle = getEnvInt("HATEBU_MAX_CALLS_PER_CYCLE", 100)
	cfg.LogRetentionDays = getEnvInt("LOG_RETENTION_DAYS", 14)
	cfg.ServerPort = getEnvString("SERVER_PORT", "8080")
	cfg.CookieSecure = strings.HasPrefix(cfg.BaseURL, "https://")
	cfg.CookieDomain = getEnvString("COOKIE_DOMAIN", "")
	cfg.CORSAllowedOrigin = getEnvString("CORS_ALLOWED_ORIGIN", "http://localhost:3000")
	cfg.HSTSEnabled = getEnvBool("HSTS_ENABLED", false)
	cfg.TrustedCIDRs = parseCommaSeparated(os.Getenv("METRICS_TRUSTED_CIDRS"))
	cfg.MetricsPort = getEnvString("METRICS_PORT", "9090")

	// Native Auth (Issue #166): いずれも任意。未設定なら token 交換が無効になるだけで起動継続。
	cfg.NativeAuthJWTSecret = os.Getenv("NATIVE_AUTH_JWT_SECRET")
	cfg.NativeAuthJWTKid = getEnvString("NATIVE_AUTH_JWT_KID", "v1")

	// Passkey / WebAuthn (Issue #216): 全 env 任意。いずれも未設定なら本機能は完全に無効化され、
	// 既存挙動と等価（NFR 2.2）。fail-closed 縮退判定は wiring 層（app.go の runServe）が担う。
	cfg.WebAuthnRPID = os.Getenv("WEBAUTHN_RP_ID")
	cfg.WebAuthnRPDisplayName = getEnvString("WEBAUTHN_RP_DISPLAY_NAME", "Feedman")
	cfg.WebAuthnOrigins = parseCommaSeparated(os.Getenv("WEBAUTHN_ORIGINS"))
	cfg.WebAuthnIOSAppID = os.Getenv("WEBAUTHN_IOS_APP_ID")
	// PASSKEY_CHALLENGE_TTL_SECONDS は秒単位の整数として扱い、time.Duration に昇格させる。
	// getEnvInt が不正値時に既定値（300）にフォールバックし Warn ログを 1 件出す挙動を踏襲する。
	cfg.PasskeyChallengeTTL = time.Duration(getEnvInt("PASSKEY_CHALLENGE_TTL_SECONDS", 300)) * time.Second

	return cfg, nil
}

// parseCommaSeparated はカンマ区切りの文字列を要素スライスに分解する。
// 各要素は前後の空白を除去し、空要素は除外する。
// 入力が空文字（未設定）の場合は空スライス（nil）を返す。
func parseCommaSeparated(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getEnvString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getEnvBool は環境変数を bool として読み込む。
// 未設定（空文字）の場合は defaultVal を返す。
// 不正値（strconv.ParseBool が受け付けない値）の場合は Warn ログを出力し defaultVal を返して起動を継続する。
func getEnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("環境変数のパースに失敗したためデフォルト値を採用します",
			slog.String("key", key),
			slog.String("value", v),
			slog.Bool("default", defaultVal),
		)
		return defaultVal
	}
	return b
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("環境変数のパースに失敗したためデフォルト値を採用します",
			slog.String("key", key),
			slog.String("value", v),
			slog.Int("default", defaultVal),
		)
		return defaultVal
	}
	return i
}

func getEnvInt64(key string, defaultVal int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		slog.Warn("環境変数のパースに失敗したためデフォルト値を採用します",
			slog.String("key", key),
			slog.String("value", v),
			slog.Int64("default", defaultVal),
		)
		return defaultVal
	}
	return i
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("環境変数のパースに失敗したためデフォルト値を採用します",
			slog.String("key", key),
			slog.String("value", v),
			slog.Duration("default", defaultVal),
		)
		return defaultVal
	}
	return d
}
