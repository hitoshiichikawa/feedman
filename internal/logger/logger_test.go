package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
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

// parseLogEntries はバッファ内の改行区切り JSON ログを 1 件ずつパースして返すヘルパー。
func parseLogEntries(t *testing.T, raw string) []map[string]interface{} {
	t.Helper()
	var entries []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse JSON line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// emitAllLevels は対象 logger に DEBUG / INFO / WARN / ERROR の 4 レベルを 1 件ずつ出力する。
func emitAllLevels(l *slog.Logger) {
	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")
}

// Requirement 1.1〜1.4 / 2.1 / 2.2 / 4.1 / 4.2: LOG_LEVEL の値に応じた出力境界を検証する。
func TestSetup_RespectsLogLevelEnv(t *testing.T) {
	cases := []struct {
		name     string
		setEnv   bool
		envValue string
		wantMsgs []string // 出力されるべき msg
		wantWarn bool     // 不正値フォールバック警告ログが出るか
	}{
		// Requirement 1.1: DEBUG は全レベル出力
		{name: "DEBUGのとき全レベル出力", setEnv: true, envValue: "DEBUG", wantMsgs: []string{"debug msg", "info msg", "warn msg", "error msg"}},
		// Requirement 1.2: INFO は DEBUG 抑制
		{name: "INFOのときDEBUG抑制", setEnv: true, envValue: "INFO", wantMsgs: []string{"info msg", "warn msg", "error msg"}},
		// Requirement 1.3: WARN は DEBUG/INFO 抑制
		{name: "WARNのときDEBUGとINFO抑制", setEnv: true, envValue: "WARN", wantMsgs: []string{"warn msg", "error msg"}},
		// Requirement 1.4: ERROR は ERROR のみ
		{name: "ERRORのときERRORのみ出力", setEnv: true, envValue: "ERROR", wantMsgs: []string{"error msg"}},
		// Requirement 2.1: 未設定は INFO 相当
		{name: "未設定のときINFO相当", setEnv: false, wantMsgs: []string{"info msg", "warn msg", "error msg"}},
		// Requirement 2.2: 空文字は未設定と同一
		{name: "空文字のときINFO相当", setEnv: true, envValue: "", wantMsgs: []string{"info msg", "warn msg", "error msg"}},
		// Requirement 4.1: 小文字は大文字と同一視
		{name: "小文字debugのときDEBUG扱い", setEnv: true, envValue: "debug", wantMsgs: []string{"debug msg", "info msg", "warn msg", "error msg"}},
		// Requirement 4.2: 大文字小文字混在も同一視
		{name: "混在Warnのときwarn扱い", setEnv: true, envValue: "Warn", wantMsgs: []string{"warn msg", "error msg"}},
		// Requirement 3.1 / 3.2 / 3.3: 不正値は INFO フォールバック + 警告ログ
		{name: "不正値VERBOSEのときINFOフォールバックと警告", setEnv: true, envValue: "VERBOSE", wantMsgs: []string{"info msg", "warn msg", "error msg"}, wantWarn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			if tc.setEnv {
				t.Setenv("LOG_LEVEL", tc.envValue)
			} else {
				t.Setenv("LOG_LEVEL", "")
				os.Unsetenv("LOG_LEVEL")
			}
			var buf bytes.Buffer

			// Act
			l := Setup(&buf)
			emitAllLevels(l)

			// Assert
			entries := parseLogEntries(t, buf.String())
			var gotMsgs []string
			gotWarn := false
			for _, e := range entries {
				msg, _ := e["msg"].(string)
				if msg == invalidLevelWarnMsg {
					gotWarn = true
					continue
				}
				gotMsgs = append(gotMsgs, msg)
			}

			if len(gotMsgs) != len(tc.wantMsgs) {
				t.Fatalf("emitted msgs = %v, want %v", gotMsgs, tc.wantMsgs)
			}
			for i, want := range tc.wantMsgs {
				if gotMsgs[i] != want {
					t.Errorf("msg[%d] = %q, want %q", i, gotMsgs[i], want)
				}
			}
			if gotWarn != tc.wantWarn {
				t.Errorf("invalid-value warn emitted = %v, want %v", gotWarn, tc.wantWarn)
			}
		})
	}
}

// Requirement 3.2: 不正値フォールバック時の警告ログにキー・値・採用デフォルトが含まれることを検証する。
func TestSetup_InvalidLevelWarnIncludesContext(t *testing.T) {
	// Arrange
	t.Setenv("LOG_LEVEL", "VERBOSE")
	var buf bytes.Buffer

	// Act
	Setup(&buf)

	// Assert
	entries := parseLogEntries(t, buf.String())
	var warnEntry map[string]interface{}
	for _, e := range entries {
		if e["msg"] == invalidLevelWarnMsg {
			warnEntry = e
			break
		}
	}
	if warnEntry == nil {
		t.Fatalf("expected warn entry %q, got entries: %s", invalidLevelWarnMsg, buf.String())
	}
	if warnEntry["level"] != "WARN" {
		t.Errorf("warn level = %q, want WARN", warnEntry["level"])
	}
	if warnEntry["key"] != envLogLevel {
		t.Errorf("warn key = %q, want %q", warnEntry["key"], envLogLevel)
	}
	if warnEntry["value"] != "VERBOSE" {
		t.Errorf("warn value = %q, want %q", warnEntry["value"], "VERBOSE")
	}
	if warnEntry["default"] != "INFO" {
		t.Errorf("warn default = %q, want %q", warnEntry["default"], "INFO")
	}
}

// Requirement 3.3: 不正値でも Setup は nil を返さず起動を継続できることを検証する。
func TestSetup_InvalidLevelDoesNotFail(t *testing.T) {
	// Arrange
	t.Setenv("LOG_LEVEL", "NOPE")
	var buf bytes.Buffer

	// Act
	l := Setup(&buf)

	// Assert
	if l == nil {
		t.Fatal("expected non-nil logger even with invalid LOG_LEVEL")
	}
}
