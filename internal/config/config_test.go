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
	// Req 2.1: 未認証 IP レート制限の閾値の既定値は 30 req/min/IP。
	if cfg.RateLimitUnauthIP != 30 {
		t.Errorf("RateLimitUnauthIP = %d, want %d", cfg.RateLimitUnauthIP, 30)
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

	// Security defaults: HSTS は未設定時 false（本機能導入前と等価）。
	if cfg.HSTSEnabled != false {
		t.Errorf("HSTSEnabled = %v, want %v (default)", cfg.HSTSEnabled, false)
	}

	// Metrics defaults: 未設定時 MetricsPort は "9090"、TrustedCIDRs は空。
	if cfg.MetricsPort != "9090" {
		t.Errorf("MetricsPort = %q, want %q", cfg.MetricsPort, "9090")
	}
	if len(cfg.TrustedCIDRs) != 0 {
		t.Errorf("TrustedCIDRs = %v, want empty (default)", cfg.TrustedCIDRs)
	}

	// Passkey / WebAuthn defaults (Issue #216 / Req 8.2 / NFR 2.2):
	// 全 env 未設定でも起動継続し、RP display name と challenge TTL は既定値を採用する。
	if cfg.WebAuthnRPID != "" {
		t.Errorf("WebAuthnRPID = %q, want empty (default)", cfg.WebAuthnRPID)
	}
	if cfg.WebAuthnRPDisplayName != "Feedman" {
		t.Errorf("WebAuthnRPDisplayName = %q, want %q (default)", cfg.WebAuthnRPDisplayName, "Feedman")
	}
	if len(cfg.WebAuthnOrigins) != 0 {
		t.Errorf("WebAuthnOrigins = %v, want empty (default)", cfg.WebAuthnOrigins)
	}
	if cfg.WebAuthnIOSAppID != "" {
		t.Errorf("WebAuthnIOSAppID = %q, want empty (default)", cfg.WebAuthnIOSAppID)
	}
	if cfg.PasskeyChallengeTTL != 300*time.Second {
		t.Errorf("PasskeyChallengeTTL = %v, want %v (default)", cfg.PasskeyChallengeTTL, 300*time.Second)
	}
}

// TestLoad_MetricsTrustedCIDRs は METRICS_TRUSTED_CIDRS のカンマ区切りパースを検証する。
// Requirement 4.1（信頼 CIDR の設定）/ NFR 2.1（未設定時は空のまま保持し検証はミドルウェアに委譲）に対応。
func TestLoad_MetricsTrustedCIDRs(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{
			name: "単一CIDRのとき1要素のスライスになる",
			env:  "10.0.0.0/8",
			want: []string{"10.0.0.0/8"},
		},
		{
			name: "複数CIDRのとき要素ごとに分割される",
			env:  "10.0.0.0/8,192.168.0.0/16",
			want: []string{"10.0.0.0/8", "192.168.0.0/16"},
		},
		{
			name: "要素前後に空白があるときトリムされる",
			env:  " 10.0.0.0/8 , 192.168.0.0/16 ",
			want: []string{"10.0.0.0/8", "192.168.0.0/16"},
		},
		{
			name: "空要素が含まれるとき除外される",
			env:  "10.0.0.0/8,,192.168.0.0/16,",
			want: []string{"10.0.0.0/8", "192.168.0.0/16"},
		},
		{
			name: "不正なCIDR文字列でもパース段階では除外せず保持する",
			env:  "not-a-cidr,10.0.0.0/8",
			want: []string{"not-a-cidr", "10.0.0.0/8"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			setRequiredEnvVars(t)
			t.Setenv("METRICS_TRUSTED_CIDRS", tc.env)

			// Act
			cfg, err := Load()

			// Assert
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if len(cfg.TrustedCIDRs) != len(tc.want) {
				t.Fatalf("TrustedCIDRs length = %d (%v), want %d (%v)",
					len(cfg.TrustedCIDRs), cfg.TrustedCIDRs, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if cfg.TrustedCIDRs[i] != w {
					t.Errorf("TrustedCIDRs[%d] = %q, want %q", i, cfg.TrustedCIDRs[i], w)
				}
			}
		})
	}

	t.Run("未設定（空文字）のとき空スライスを保持する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("METRICS_TRUSTED_CIDRS", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(cfg.TrustedCIDRs) != 0 {
			t.Errorf("TrustedCIDRs = %v, want empty", cfg.TrustedCIDRs)
		}
	})

	t.Run("空白のみのとき空スライスを保持する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("METRICS_TRUSTED_CIDRS", "   ")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(cfg.TrustedCIDRs) != 0 {
			t.Errorf("TrustedCIDRs = %v, want empty", cfg.TrustedCIDRs)
		}
	})
}

// TestLoad_MetricsPort は METRICS_PORT の読み込みと既定値を検証する。
// Requirement 4.1 に対応。
func TestLoad_MetricsPort(t *testing.T) {
	t.Run("METRICS_PORTが設定されているとき値を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("METRICS_PORT", "9999")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.MetricsPort != "9999" {
			t.Errorf("MetricsPort = %q, want %q", cfg.MetricsPort, "9999")
		}
	})

	t.Run("METRICS_PORTが未設定のとき既定値9090を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("METRICS_PORT", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.MetricsPort != "9090" {
			t.Errorf("MetricsPort = %q, want %q (default)", cfg.MetricsPort, "9090")
		}
	})
}

// TestLoad_NativeAuthJWT は NATIVE_AUTH_JWT_SECRET / NATIVE_AUTH_JWT_KID の
// 読み込みと未設定時の挙動を検証する（Issue #166 / Req 3.1, 3.2, 3.5, NFR 2.2）。
//
// secret は未設定でも起動成功し、空文字として保持される（後段の wiring で fail-closed 判定）。
// kid は未設定時に "v1" が採用される。
func TestLoad_NativeAuthJWT(t *testing.T) {
	t.Run("NATIVE_AUTH_JWT_SECRETが設定されているとき値を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("NATIVE_AUTH_JWT_SECRET", "test-jwt-secret-32bytes-long-12345")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.NativeAuthJWTSecret != "test-jwt-secret-32bytes-long-12345" {
			t.Errorf("NativeAuthJWTSecret = %q, want %q",
				cfg.NativeAuthJWTSecret, "test-jwt-secret-32bytes-long-12345")
		}
	})

	t.Run("NATIVE_AUTH_JWT_SECRETが未設定のとき空文字を保持し起動を継続する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("NATIVE_AUTH_JWT_SECRET", "")

		// Act
		cfg, err := Load()

		// Assert: secret 未設定でも起動成功（fail-closed は後段の wiring 側で判定）
		if err != nil {
			t.Fatalf("expected no error (secret 未設定でも起動継続), got %v", err)
		}
		if cfg.NativeAuthJWTSecret != "" {
			t.Errorf("NativeAuthJWTSecret = %q, want empty", cfg.NativeAuthJWTSecret)
		}
	})

	t.Run("NATIVE_AUTH_JWT_KIDが設定されているとき値を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("NATIVE_AUTH_JWT_KID", "v2")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.NativeAuthJWTKid != "v2" {
			t.Errorf("NativeAuthJWTKid = %q, want %q", cfg.NativeAuthJWTKid, "v2")
		}
	})

	t.Run("NATIVE_AUTH_JWT_KIDが未設定のとき既定値v1を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("NATIVE_AUTH_JWT_KID", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.NativeAuthJWTKid != "v1" {
			t.Errorf("NativeAuthJWTKid = %q, want %q (default)", cfg.NativeAuthJWTKid, "v1")
		}
	})
}

// TestLoad_Passkey は WEBAUTHN_* / PASSKEY_CHALLENGE_TTL_SECONDS 環境変数の読み込みと
// 未設定時の挙動を検証する（Issue #216 / Req 5.1, 5.2, 5.3, 5.4, 8.1, 8.2, 8.3, 8.4, 8.5,
// NFR 2.1, NFR 2.2）。
//
// 全 env は任意で、未設定時は起動を継続する（fail-closed 縮退は wiring 層の責務）。
// RP display name は未設定時 "Feedman"、challenge TTL は未設定・不正値時に既定値 300s。
func TestLoad_Passkey(t *testing.T) {
	t.Run("WEBAUTHN_RP_IDが設定されているとき値を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_RP_ID", "example.com")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.WebAuthnRPID != "example.com" {
			t.Errorf("WebAuthnRPID = %q, want %q", cfg.WebAuthnRPID, "example.com")
		}
	})

	t.Run("WEBAUTHN_RP_IDが未設定のとき空文字を保持し起動を継続する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_RP_ID", "")

		// Act
		cfg, err := Load()

		// Assert: fail-closed 判定は wiring 側の責務。config 層では起動を継続する（NFR 2.2）
		if err != nil {
			t.Fatalf("expected no error (RPID 未設定でも起動継続), got %v", err)
		}
		if cfg.WebAuthnRPID != "" {
			t.Errorf("WebAuthnRPID = %q, want empty", cfg.WebAuthnRPID)
		}
	})

	t.Run("WEBAUTHN_RP_DISPLAY_NAMEが設定されているとき値を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_RP_DISPLAY_NAME", "Feedman Staging")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.WebAuthnRPDisplayName != "Feedman Staging" {
			t.Errorf("WebAuthnRPDisplayName = %q, want %q", cfg.WebAuthnRPDisplayName, "Feedman Staging")
		}
	})

	t.Run("WEBAUTHN_RP_DISPLAY_NAMEが未設定のとき既定値Feedmanを採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_RP_DISPLAY_NAME", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.WebAuthnRPDisplayName != "Feedman" {
			t.Errorf("WebAuthnRPDisplayName = %q, want %q (default)", cfg.WebAuthnRPDisplayName, "Feedman")
		}
	})

	t.Run("WEBAUTHN_ORIGINSがカンマ区切りのとき要素ごとに分割される", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_ORIGINS", "https://example.com,https://staging.example.com,feedman://")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		want := []string{"https://example.com", "https://staging.example.com", "feedman://"}
		if len(cfg.WebAuthnOrigins) != len(want) {
			t.Fatalf("WebAuthnOrigins length = %d (%v), want %d (%v)",
				len(cfg.WebAuthnOrigins), cfg.WebAuthnOrigins, len(want), want)
		}
		for i, w := range want {
			if cfg.WebAuthnOrigins[i] != w {
				t.Errorf("WebAuthnOrigins[%d] = %q, want %q", i, cfg.WebAuthnOrigins[i], w)
			}
		}
	})

	t.Run("WEBAUTHN_ORIGINSが要素前後に空白を含むときトリムされる", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_ORIGINS", " https://example.com , https://staging.example.com ")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		want := []string{"https://example.com", "https://staging.example.com"}
		if len(cfg.WebAuthnOrigins) != len(want) {
			t.Fatalf("WebAuthnOrigins length = %d (%v), want %d",
				len(cfg.WebAuthnOrigins), cfg.WebAuthnOrigins, len(want))
		}
		for i, w := range want {
			if cfg.WebAuthnOrigins[i] != w {
				t.Errorf("WebAuthnOrigins[%d] = %q, want %q", i, cfg.WebAuthnOrigins[i], w)
			}
		}
	})

	t.Run("WEBAUTHN_ORIGINSが未設定のとき空スライスを保持する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_ORIGINS", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(cfg.WebAuthnOrigins) != 0 {
			t.Errorf("WebAuthnOrigins = %v, want empty", cfg.WebAuthnOrigins)
		}
	})

	t.Run("WEBAUTHN_IOS_APP_IDが設定されているとき値を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_IOS_APP_ID", "ABCD1234.com.example.feedman")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.WebAuthnIOSAppID != "ABCD1234.com.example.feedman" {
			t.Errorf("WebAuthnIOSAppID = %q, want %q",
				cfg.WebAuthnIOSAppID, "ABCD1234.com.example.feedman")
		}
	})

	t.Run("WEBAUTHN_IOS_APP_IDが未設定のとき空文字を保持する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("WEBAUTHN_IOS_APP_ID", "")

		// Act
		cfg, err := Load()

		// Assert: AASA は wiring 側で fail-closed 縮退（handler 非生成 → 404）。config 層では継続。
		if err != nil {
			t.Fatalf("expected no error (IOS_APP_ID 未設定でも起動継続), got %v", err)
		}
		if cfg.WebAuthnIOSAppID != "" {
			t.Errorf("WebAuthnIOSAppID = %q, want empty", cfg.WebAuthnIOSAppID)
		}
	})

	t.Run("PASSKEY_CHALLENGE_TTL_SECONDSが設定されているとき値を秒単位で採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("PASSKEY_CHALLENGE_TTL_SECONDS", "600")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.PasskeyChallengeTTL != 600*time.Second {
			t.Errorf("PasskeyChallengeTTL = %v, want %v",
				cfg.PasskeyChallengeTTL, 600*time.Second)
		}
	})

	t.Run("PASSKEY_CHALLENGE_TTL_SECONDSが未設定のとき既定値300秒を採用する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("PASSKEY_CHALLENGE_TTL_SECONDS", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.PasskeyChallengeTTL != 300*time.Second {
			t.Errorf("PasskeyChallengeTTL = %v, want %v (default)",
				cfg.PasskeyChallengeTTL, 300*time.Second)
		}
	})

	t.Run("PASSKEY_CHALLENGE_TTL_SECONDSが不正値のとき既定値300秒にフォールバックし起動を継続する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("PASSKEY_CHALLENGE_TTL_SECONDS", "not-a-number")

		// Act
		cfg, err := Load()

		// Assert: getEnvInt が不正値時に Warn を出して既定値にフォールバックする既存挙動を踏襲
		if err != nil {
			t.Fatalf("expected no error (should continue startup with default), got %v", err)
		}
		if cfg.PasskeyChallengeTTL != 300*time.Second {
			t.Errorf("PasskeyChallengeTTL = %v, want %v (default for invalid value)",
				cfg.PasskeyChallengeTTL, 300*time.Second)
		}
	})
}

// TestLoad_HSTSEnabled は HSTS_ENABLED 環境変数の読み込みを検証する。
// Requirement 3.3（未指定・不正値時は既定値採用で起動継続）と NFR 1.2 に対応。
func TestLoad_HSTSEnabled(t *testing.T) {
	t.Run("HSTS_ENABLEDがtrueのときHSTSEnabledがtrueになる", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("HSTS_ENABLED", "true")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !cfg.HSTSEnabled {
			t.Error("HSTSEnabled = false, want true")
		}
	})

	t.Run("HSTS_ENABLEDが未設定のときHSTSEnabledがfalseになる", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("HSTS_ENABLED", "")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.HSTSEnabled {
			t.Error("HSTSEnabled = true, want false (default for unset)")
		}
	})

	t.Run("HSTS_ENABLEDが不正値のときHSTSEnabledが既定値falseになり起動を継続する", func(t *testing.T) {
		// Arrange
		setRequiredEnvVars(t)
		t.Setenv("HSTS_ENABLED", "not-a-bool")

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error (should continue startup with default), got %v", err)
		}
		if cfg.HSTSEnabled {
			t.Error("HSTSEnabled = true, want false (default for invalid value)")
		}
	})
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
	t.Setenv("RATE_LIMIT_UNAUTH_IP", "15")
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
	// Req 2.2: 指定された閾値を適用する。
	if cfg.RateLimitUnauthIP != 15 {
		t.Errorf("RateLimitUnauthIP = %d, want %d", cfg.RateLimitUnauthIP, 15)
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

// TestLoad_InvalidRateLimitUnauthIP_FallsBackToDefault は Req 2.3 を検証する。
// 不正な RATE_LIMIT_UNAUTH_IP が指定されてもエラーにせず、既定値 30 で起動を継続する。
func TestLoad_InvalidRateLimitUnauthIP_FallsBackToDefault(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("RATE_LIMIT_UNAUTH_IP", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("不正値でも起動継続すべき（エラーなし）, got %v", err)
	}
	if cfg.RateLimitUnauthIP != 30 {
		t.Errorf("RateLimitUnauthIP = %d, want %d (既定値フォールバック)", cfg.RateLimitUnauthIP, 30)
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

// TestGetEnvBool は getEnvBool のパース失敗時警告ログ・フォールバック・正常系を検証する。
// Requirement 3.3（未指定・不正値時は既定値採用で起動継続）に対応。
func TestGetEnvBool(t *testing.T) {
	const key = "TEST_GET_ENV_BOOL"
	const defaultVal = false

	t.Run("不正値のときデフォルト値を採用しWarnを1件出力する", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "yesnt")

		// Act
		got := getEnvBool(key, defaultVal)

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
		t.Setenv(key, "yesnt")

		// Act
		getEnvBool(key, defaultVal)

		// Assert
		recs := h.warnRecords()
		if len(recs) != 1 {
			t.Fatalf("warn records = %d, want 1", len(recs))
		}
		r := recs[0]
		assertAttr(t, r, "key", key)
		assertAttr(t, r, "value", "yesnt")
		assertAttr(t, r, "default", "false")
	})

	t.Run("正常値のとき値を採用しWarnを出力しない", func(t *testing.T) {
		// Arrange
		h := installCaptureLogger(t)
		t.Setenv(key, "true")

		// Act
		got := getEnvBool(key, defaultVal)

		// Assert
		if got != true {
			t.Errorf("got = %v, want %v", got, true)
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
		got := getEnvBool(key, defaultVal)

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
