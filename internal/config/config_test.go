package config

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// captureHandler はテスト中の slog.Record を収集する slog.Handler 実装。
// パース失敗時の Warn ログ出力（件数・レベル・構造化フィールド）を検証するために使う。
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

// warnRecords は Warn レベルのレコードのみを返す。
func (h *captureHandler) warnRecords() []slog.Record {
	var out []slog.Record
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			out = append(out, r)
		}
	}
	return out
}

// installCaptureLogger はデフォルトロガーをテスト用の captureHandler に差し替え、
// t.Cleanup で元のロガーを復元する。返り値のハンドラから収集レコードを参照する。
func installCaptureLogger(t *testing.T) *captureHandler {
	t.Helper()
	prev := slog.Default()
	h := &captureHandler{}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})
	return h
}

// attrValue はレコードから指定キーの属性値（文字列表現）を取り出す。
// キーが存在しない場合は ok=false を返す。
func attrValue(r slog.Record, key string) (string, bool) {
	var (
		val   string
		found bool
	)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value.String()
			found = true
			return false
		}
		return true
	})
	return val, found
}

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

// TestGetEnvInt は getEnvInt のパース失敗時警告ログ・フォールバック・正常系を検証する。
// Requirement 1 (1.1/1.2/1.3) と Requirement 4 (4.1/4.2/4.3/4.4) に対応。
func TestGetEnvInt(t *testing.T) {
	const key = "TEST_GET_ENV_INT"
	const defaultVal = 42

	t.Run("不正値のときデフォルト値を採用しWarnを1件出力する", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "not-an-int")

		// Act
		got := getEnvInt(key, defaultVal)

		// Assert
		if got != defaultVal {
			t.Errorf("got = %d, want %d (default fallback)", got, defaultVal)
		}
		if n := len(h.warnRecords()); n != 1 {
			t.Fatalf("warn records = %d, want 1", n)
		}
	})

	t.Run("不正値のときWarnログにキー名・不正値・デフォルト値を構造化フィールドで含める", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "not-an-int")

		// Act
		getEnvInt(key, defaultVal)

		// Assert
		recs := h.warnRecords()
		if len(recs) != 1 {
			t.Fatalf("warn records = %d, want 1", len(recs))
		}
		r := recs[0]
		assertAttr(t, r, "key", key)
		assertAttr(t, r, "value", "not-an-int")
		assertAttr(t, r, "default", "42")
	})

	t.Run("正常値のとき値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "100")

		// Act
		got := getEnvInt(key, defaultVal)

		// Assert
		if got != 100 {
			t.Errorf("got = %d, want %d", got, 100)
		}
		if n := len(h.warnRecords()); n != 0 {
			t.Errorf("warn records = %d, want 0", n)
		}
	})

	t.Run("未設定（空文字）のときデフォルト値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "")

		// Act
		got := getEnvInt(key, defaultVal)

		// Assert
		if got != defaultVal {
			t.Errorf("got = %d, want %d (default fallback)", got, defaultVal)
		}
		if n := len(h.warnRecords()); n != 0 {
			t.Errorf("warn records = %d, want 0", n)
		}
	})
}

// TestGetEnvInt64 は getEnvInt64 のパース失敗時警告ログ・フォールバック・正常系を検証する。
// Requirement 2 (2.1/2.2/2.3) と Requirement 4 に対応。
func TestGetEnvInt64(t *testing.T) {
	const key = "TEST_GET_ENV_INT64"
	const defaultVal int64 = 5242880

	t.Run("不正値のときデフォルト値を採用しWarnを1件出力する", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "12.5")

		// Act
		got := getEnvInt64(key, defaultVal)

		// Assert
		if got != defaultVal {
			t.Errorf("got = %d, want %d (default fallback)", got, defaultVal)
		}
		if n := len(h.warnRecords()); n != 1 {
			t.Fatalf("warn records = %d, want 1", n)
		}
	})

	t.Run("不正値のときWarnログにキー名・不正値・デフォルト値を構造化フィールドで含める", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "12.5")

		// Act
		getEnvInt64(key, defaultVal)

		// Assert
		recs := h.warnRecords()
		if len(recs) != 1 {
			t.Fatalf("warn records = %d, want 1", len(recs))
		}
		r := recs[0]
		assertAttr(t, r, "key", key)
		assertAttr(t, r, "value", "12.5")
		assertAttr(t, r, "default", "5242880")
	})

	t.Run("正常値のとき値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "10485760")

		// Act
		got := getEnvInt64(key, defaultVal)

		// Assert
		if got != 10485760 {
			t.Errorf("got = %d, want %d", got, 10485760)
		}
		if n := len(h.warnRecords()); n != 0 {
			t.Errorf("warn records = %d, want 0", n)
		}
	})

	t.Run("未設定（空文字）のときデフォルト値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "")

		// Act
		got := getEnvInt64(key, defaultVal)

		// Assert
		if got != defaultVal {
			t.Errorf("got = %d, want %d (default fallback)", got, defaultVal)
		}
		if n := len(h.warnRecords()); n != 0 {
			t.Errorf("warn records = %d, want 0", n)
		}
	})
}

// TestGetEnvDuration は getEnvDuration のパース失敗時警告ログ・フォールバック・正常系を検証する。
// Requirement 3 (3.1/3.2/3.3) と Requirement 4 に対応。
func TestGetEnvDuration(t *testing.T) {
	const key = "TEST_GET_ENV_DURATION"
	const defaultVal = 10 * time.Second

	t.Run("不正値のときデフォルト値を採用しWarnを1件出力する", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "10sec")

		// Act
		got := getEnvDuration(key, defaultVal)

		// Assert
		if got != defaultVal {
			t.Errorf("got = %v, want %v (default fallback)", got, defaultVal)
		}
		if n := len(h.warnRecords()); n != 1 {
			t.Fatalf("warn records = %d, want 1", n)
		}
	})

	t.Run("不正値のときWarnログにキー名・不正値・デフォルト値を構造化フィールドで含める", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "10sec")

		// Act
		getEnvDuration(key, defaultVal)

		// Assert
		recs := h.warnRecords()
		if len(recs) != 1 {
			t.Fatalf("warn records = %d, want 1", len(recs))
		}
		r := recs[0]
		assertAttr(t, r, "key", key)
		assertAttr(t, r, "value", "10sec")
		assertAttr(t, r, "default", (10 * time.Second).String())
	})

	t.Run("正常値のとき値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "30s")

		// Act
		got := getEnvDuration(key, defaultVal)

		// Assert
		if got != 30*time.Second {
			t.Errorf("got = %v, want %v", got, 30*time.Second)
		}
		if n := len(h.warnRecords()); n != 0 {
			t.Errorf("warn records = %d, want 0", n)
		}
	})

	t.Run("未設定（空文字）のときデフォルト値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "")

		// Act
		got := getEnvDuration(key, defaultVal)

		// Assert
		if got != defaultVal {
			t.Errorf("got = %v, want %v (default fallback)", got, defaultVal)
		}
		if n := len(h.warnRecords()); n != 0 {
			t.Errorf("warn records = %d, want 0", n)
		}
	})
}

// assertAttr はレコードに指定キーの属性が存在し、値が期待文字列と一致することを検証する。
func assertAttr(t *testing.T, r slog.Record, key, want string) {
	t.Helper()
	got, ok := attrValue(r, key)
	if !ok {
		t.Errorf("attribute %q not found in record", key)
		return
	}
	if got != want {
		t.Errorf("attribute %q = %q, want %q", key, got, want)
	}
}
