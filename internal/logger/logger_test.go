package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestSetup_ReturnsJSONLogger(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(&buf)

	if l == nil {
		t.Fatal("expected non-nil logger")
	}

	l.Info("test message", slog.String("key", "value"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON log output, got error: %v\nraw output: %s", err, buf.String())
	}

	if entry["msg"] != "test message" {
		t.Errorf("msg = %q, want %q", entry["msg"], "test message")
	}
	if entry["key"] != "value" {
		t.Errorf("key = %q, want %q", entry["key"], "value")
	}
}

func TestSetup_IncludesTimeField(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(&buf)

	l.Info("test")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if _, ok := entry["time"]; !ok {
		t.Error("expected 'time' field in JSON log output")
	}
}

func TestSetup_IncludesLevelField(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(&buf)

	l.Warn("warning test")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry["level"] != "WARN" {
		t.Errorf("level = %q, want %q", entry["level"], "WARN")
	}
}

func TestSetup_MultipleAttributes(t *testing.T) {
	var buf bytes.Buffer
	l := Setup(&buf)

	l.Info("fetch completed",
		slog.String("user_id", "u-123"),
		slog.String("feed_id", "f-456"),
		slog.String("url", "https://example.com/feed"),
		slog.Int("http_status", 200),
		slog.Int("items_count", 25),
	)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry["user_id"] != "u-123" {
		t.Errorf("user_id = %q, want %q", entry["user_id"], "u-123")
	}
	if entry["feed_id"] != "f-456" {
		t.Errorf("feed_id = %q, want %q", entry["feed_id"], "f-456")
	}
	if entry["url"] != "https://example.com/feed" {
		t.Errorf("url = %q, want %q", entry["url"], "https://example.com/feed")
	}
	if entry["http_status"] != float64(200) {
		t.Errorf("http_status = %v, want %v", entry["http_status"], 200)
	}
	if entry["items_count"] != float64(25) {
		t.Errorf("items_count = %v, want %v", entry["items_count"], 25)
	}
}

func TestSetupDefault_SetsGlobalLogger(t *testing.T) {
	var buf bytes.Buffer
	SetupDefault(&buf)

	slog.Default().Info("global test", slog.String("test_key", "test_val"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v\nraw: %s", err, buf.String())
	}

	if entry["msg"] != "global test" {
		t.Errorf("msg = %q, want %q", entry["msg"], "global test")
	}
	if entry["test_key"] != "test_val" {
		t.Errorf("test_key = %q, want %q", entry["test_key"], "test_val")
	}
}
