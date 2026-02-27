package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
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
