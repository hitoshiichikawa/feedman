package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

func TestInit_WithValidConfig_Succeeds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/feedman?sslmode=disable")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback")
	t.Setenv("SESSION_SECRET", "test-session-secret-32bytes-long!")
	t.Setenv("BASE_URL", "http://localhost:8080")

	var buf bytes.Buffer
	cfg, err := Init(&buf)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/feedman?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want postgres://...", cfg.DatabaseURL)
	}

	// Verify that slog global logger is configured for JSON output
	slog.Default().Info("init test")
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected JSON log output, got error: %v\nraw: %s", err, buf.String())
	}
	if entry["msg"] != "init test" {
		t.Errorf("msg = %q, want %q", entry["msg"], "init test")
	}
}

// TestInit_WithoutWebAuthnEnv_UsesDefaults は WEBAUTHN_* / PASSKEY_CHALLENGE_TTL_SECONDS
// env が未設定でも Init が成功し、runServe 側の fail-closed 判定に必要な既定値
// （空文字 / 空スライス / "Feedman" / 300s）が Config に載ることを検証する
// （Issue #216 / Req 8.2 / NFR 2.2 / NFR 2.1）。
//
// runServe の実際の fail-closed 分岐（handler nil 化）は DB 接続を伴うため本テストの対象外
// だが、config 層で env 未設定時に defaults が確実に採用されることを保証することで、
// 既存デプロイ（passkey 関連 env 未設定）が本機能導入前と等価に動作することを担保する。
func TestInit_WithoutWebAuthnEnv_UsesDefaults(t *testing.T) {
	// Arrange: 必須 env のみ設定し、WEBAUTHN_* は明示的に空文字にする
	// （t.Setenv は現在の値を保存しテスト終了時に復元するため、テスト間の env leak を防ぐ）。
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/feedman?sslmode=disable")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback")
	t.Setenv("SESSION_SECRET", "test-session-secret-32bytes-long!")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("WEBAUTHN_RP_ID", "")
	t.Setenv("WEBAUTHN_RP_DISPLAY_NAME", "")
	t.Setenv("WEBAUTHN_ORIGINS", "")
	t.Setenv("WEBAUTHN_IOS_APP_ID", "")
	t.Setenv("PASSKEY_CHALLENGE_TTL_SECONDS", "")

	var buf bytes.Buffer

	// Act
	cfg, err := Init(&buf)

	// Assert: env 未設定でも起動継続（既存 Init 挙動と等価 / NFR 2.2）
	if err != nil {
		t.Fatalf("expected no error (WebAuthn env 未設定でも起動継続), got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.WebAuthnRPID != "" {
		t.Errorf("WebAuthnRPID = %q, want empty", cfg.WebAuthnRPID)
	}
	if cfg.WebAuthnRPDisplayName != "Feedman" {
		t.Errorf("WebAuthnRPDisplayName = %q, want %q (default)",
			cfg.WebAuthnRPDisplayName, "Feedman")
	}
	if len(cfg.WebAuthnOrigins) != 0 {
		t.Errorf("WebAuthnOrigins = %v, want empty", cfg.WebAuthnOrigins)
	}
	if cfg.WebAuthnIOSAppID != "" {
		t.Errorf("WebAuthnIOSAppID = %q, want empty", cfg.WebAuthnIOSAppID)
	}
	if cfg.PasskeyChallengeTTL != 300*time.Second {
		t.Errorf("PasskeyChallengeTTL = %v, want %v (default)",
			cfg.PasskeyChallengeTTL, 300*time.Second)
	}
}

// TestInit_WithWebAuthnEnv_LoadsAllFields は WEBAUTHN_* を full set した場合に、
// runServe 側の passkey / AASA 両 wiring が発火可能な値（RPID 非空 + Origins 非空 +
// IOSAppID 非空 + TTL 非デフォルト）が Config に載ることを検証する（Req 5.1〜5.4 /
// Req 8.1〜8.5）。
//
// 実際の handler 組み立ては runServe（DB 接続必須）で行われるため、handler 実体の
// 非 nil 検証は本テストの対象外。config → wiring の入力が正しく渡ることを config 層
// で担保する。
func TestInit_WithWebAuthnEnv_LoadsAllFields(t *testing.T) {
	// Arrange
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/feedman?sslmode=disable")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback")
	t.Setenv("SESSION_SECRET", "test-session-secret-32bytes-long!")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("WEBAUTHN_RP_ID", "example.com")
	t.Setenv("WEBAUTHN_RP_DISPLAY_NAME", "Feedman Prod")
	t.Setenv("WEBAUTHN_ORIGINS", "https://example.com,feedman://")
	t.Setenv("WEBAUTHN_IOS_APP_ID", "ABCD1234.com.example.feedman")
	t.Setenv("PASSKEY_CHALLENGE_TTL_SECONDS", "600")

	var buf bytes.Buffer

	// Act
	cfg, err := Init(&buf)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.WebAuthnRPID != "example.com" {
		t.Errorf("WebAuthnRPID = %q, want %q", cfg.WebAuthnRPID, "example.com")
	}
	if cfg.WebAuthnRPDisplayName != "Feedman Prod" {
		t.Errorf("WebAuthnRPDisplayName = %q, want %q",
			cfg.WebAuthnRPDisplayName, "Feedman Prod")
	}
	if len(cfg.WebAuthnOrigins) != 2 ||
		cfg.WebAuthnOrigins[0] != "https://example.com" ||
		cfg.WebAuthnOrigins[1] != "feedman://" {
		t.Errorf("WebAuthnOrigins = %v, want [https://example.com feedman://]",
			cfg.WebAuthnOrigins)
	}
	if cfg.WebAuthnIOSAppID != "ABCD1234.com.example.feedman" {
		t.Errorf("WebAuthnIOSAppID = %q, want %q",
			cfg.WebAuthnIOSAppID, "ABCD1234.com.example.feedman")
	}
	if cfg.PasskeyChallengeTTL != 600*time.Second {
		t.Errorf("PasskeyChallengeTTL = %v, want %v",
			cfg.PasskeyChallengeTTL, 600*time.Second)
	}
}

func TestInit_WithMissingConfig_ReturnsError(t *testing.T) {
	// Clear all required env vars
	t.Setenv("DATABASE_URL", "")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("GOOGLE_CLIENT_SECRET", "")
	t.Setenv("GOOGLE_REDIRECT_URL", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("BASE_URL", "")

	var buf bytes.Buffer
	cfg, err := Init(&buf)
	if err == nil {
		t.Fatal("expected error for missing required env vars, got nil")
	}
	if cfg != nil {
		t.Error("expected nil config on error")
	}
}
