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

	return cfg, nil
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
