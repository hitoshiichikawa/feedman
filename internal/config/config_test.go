package config

import (
	"testing"
	"time"
)

func setRequiredEnvVars(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/feedman?sslmode=disable")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback")
	t.Setenv("SESSION_SECRET", "test-session-secret-32bytes-long!")
	t.Setenv("BASE_URL", "http://localhost:8080")
}

func TestLoad_AllRequiredVarsSet_ReturnsConfig(t *testing.T) {
	setRequiredEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/feedman?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://user:pass@localhost:5432/feedman?sslmode=disable")
	}
	if cfg.GoogleClientID != "test-client-id" {
		t.Errorf("GoogleClientID = %q, want %q", cfg.GoogleClientID, "test-client-id")
	}
	if cfg.GoogleClientSecret != "test-client-secret" {
		t.Errorf("GoogleClientSecret = %q, want %q", cfg.GoogleClientSecret, "test-client-secret")
	}
	if cfg.GoogleRedirectURL != "http://localhost:8080/auth/google/callback" {
		t.Errorf("GoogleRedirectURL = %q, want %q", cfg.GoogleRedirectURL, "http://localhost:8080/auth/google/callback")
	}
	if cfg.SessionSecret != "test-session-secret-32bytes-long!" {
		t.Errorf("SessionSecret = %q, want %q", cfg.SessionSecret, "test-session-secret-32bytes-long!")
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:8080")
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	setRequiredEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Session defaults
	if cfg.SessionMaxAge != 86400 {
		t.Errorf("SessionMaxAge = %d, want %d", cfg.SessionMaxAge, 86400)
	}

	// Fetch defaults
	if cfg.FetchTimeout != 10*time.Second {
		t.Errorf("FetchTimeout = %v, want %v", cfg.FetchTimeout, 10*time.Second)
	}
	if cfg.FetchMaxSize != 5242880 {
		t.Errorf("FetchMaxSize = %d, want %d", cfg.FetchMaxSize, 5242880)
	}
	if cfg.FetchMaxConcurrent != 10 {
		t.Errorf("FetchMaxConcurrent = %d, want %d", cfg.FetchMaxConcurrent, 10)
	}
	if cfg.FetchInterval != 5*time.Minute {
		t.Errorf("FetchInterval = %v, want %v", cfg.FetchInterval, 5*time.Minute)
	}

	// Rate limit defaults
	if cfg.RateLimitGeneral != 120 {
		t.Errorf("RateLimitGeneral = %d, want %d", cfg.RateLimitGeneral, 120)
	}
	if cfg.RateLimitFeedReg != 10 {
		t.Errorf("RateLimitFeedReg = %d, want %d", cfg.RateLimitFeedReg, 10)
	}

	// Hatebu defaults
	if cfg.HatebuTTL != 24*time.Hour {
		t.Errorf("HatebuTTL = %v, want %v", cfg.HatebuTTL, 24*time.Hour)
	}
	if cfg.HatebuBatchInterval != 10*time.Minute {
		t.Errorf("HatebuBatchInterval = %v, want %v", cfg.HatebuBatchInterval, 10*time.Minute)
	}
	if cfg.HatebuAPIInterval != 5*time.Second {
		t.Errorf("HatebuAPIInterval = %v, want %v", cfg.HatebuAPIInterval, 5*time.Second)
	}
	if cfg.HatebuMaxCallsPerCycle != 100 {
		t.Errorf("HatebuMaxCallsPerCycle = %d, want %d", cfg.HatebuMaxCallsPerCycle, 100)
	}

	// Log retention defaults
	if cfg.LogRetentionDays != 14 {
		t.Errorf("LogRetentionDays = %d, want %d", cfg.LogRetentionDays, 14)
	}

	// Server defaults
	if cfg.ServerPort != "8080" {
		t.Errorf("ServerPort = %q, want %q", cfg.ServerPort, "8080")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnvVars(t)

	t.Setenv("SESSION_MAX_AGE", "3600")
	t.Setenv("FETCH_TIMEOUT", "30s")
	t.Setenv("FETCH_MAX_SIZE", "10485760")
	t.Setenv("FETCH_MAX_CONCURRENT", "5")
	t.Setenv("FETCH_INTERVAL", "10m")
	t.Setenv("RATE_LIMIT_GENERAL", "60")
	t.Setenv("RATE_LIMIT_FEED_REG", "5")
	t.Setenv("HATEBU_TTL", "12h")
	t.Setenv("HATEBU_BATCH_INTERVAL", "20m")
	t.Setenv("HATEBU_API_INTERVAL", "10s")
	t.Setenv("HATEBU_MAX_CALLS_PER_CYCLE", "50")
	t.Setenv("SERVER_PORT", "3000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.SessionMaxAge != 3600 {
		t.Errorf("SessionMaxAge = %d, want %d", cfg.SessionMaxAge, 3600)
	}
	if cfg.FetchTimeout != 30*time.Second {
		t.Errorf("FetchTimeout = %v, want %v", cfg.FetchTimeout, 30*time.Second)
	}
	if cfg.FetchMaxSize != 10485760 {
		t.Errorf("FetchMaxSize = %d, want %d", cfg.FetchMaxSize, 10485760)
	}
	if cfg.FetchMaxConcurrent != 5 {
		t.Errorf("FetchMaxConcurrent = %d, want %d", cfg.FetchMaxConcurrent, 5)
	}
	if cfg.FetchInterval != 10*time.Minute {
		t.Errorf("FetchInterval = %v, want %v", cfg.FetchInterval, 10*time.Minute)
	}
	if cfg.RateLimitGeneral != 60 {
		t.Errorf("RateLimitGeneral = %d, want %d", cfg.RateLimitGeneral, 60)
	}
	if cfg.RateLimitFeedReg != 5 {
		t.Errorf("RateLimitFeedReg = %d, want %d", cfg.RateLimitFeedReg, 5)
	}
	if cfg.HatebuTTL != 12*time.Hour {
		t.Errorf("HatebuTTL = %v, want %v", cfg.HatebuTTL, 12*time.Hour)
	}
	if cfg.HatebuBatchInterval != 20*time.Minute {
		t.Errorf("HatebuBatchInterval = %v, want %v", cfg.HatebuBatchInterval, 20*time.Minute)
	}
	if cfg.HatebuAPIInterval != 10*time.Second {
		t.Errorf("HatebuAPIInterval = %v, want %v", cfg.HatebuAPIInterval, 10*time.Second)
	}
	if cfg.HatebuMaxCallsPerCycle != 50 {
		t.Errorf("HatebuMaxCallsPerCycle = %d, want %d", cfg.HatebuMaxCallsPerCycle, 50)
	}
	if cfg.ServerPort != "3000" {
		t.Errorf("ServerPort = %q, want %q", cfg.ServerPort, "3000")
	}
}

func TestLoad_MissingDatabaseURL_ReturnsError(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
}

func TestLoad_MissingGoogleClientID_ReturnsError(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("GOOGLE_CLIENT_ID", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GOOGLE_CLIENT_ID, got nil")
	}
}

func TestLoad_MissingGoogleClientSecret_ReturnsError(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("GOOGLE_CLIENT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GOOGLE_CLIENT_SECRET, got nil")
	}
}

func TestLoad_MissingGoogleRedirectURL_ReturnsError(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("GOOGLE_REDIRECT_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GOOGLE_REDIRECT_URL, got nil")
	}
}

func TestLoad_MissingSessionSecret_ReturnsError(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("SESSION_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing SESSION_SECRET, got nil")
	}
}

func TestLoad_MissingBaseURL_ReturnsError(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("BASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing BASE_URL, got nil")
	}
}
