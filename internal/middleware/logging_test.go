package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLoggingMiddleware_LogsRequestFields はリクエストログに必要なフィールドが含まれることを検証する。
func TestLoggingMiddleware_LogsRequestFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := NewLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/feeds", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v\nraw: %s", err, buf.String())
	}

	// 必須フィールドの検証
	if entry["method"] != "GET" {
		t.Errorf("method = %q, want %q", entry["method"], "GET")
	}
	if entry["path"] != "/api/feeds" {
		t.Errorf("path = %q, want %q", entry["path"], "/api/feeds")
	}
	if _, ok := entry["status"]; !ok {
		t.Error("expected 'status' field in log entry")
	}
	if status, ok := entry["status"].(float64); ok && status != 200 {
		t.Errorf("status = %v, want 200", status)
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected 'duration_ms' field in log entry")
	}
}

// TestLoggingMiddleware_IncludesUserID はユーザーIDがログに含まれることを検証する。
func TestLoggingMiddleware_IncludesUserID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := NewLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/feeds", nil)
	ctx := context.WithValue(req.Context(), userIDContextKey, "user-123")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if entry["user_id"] != "user-123" {
		t.Errorf("user_id = %q, want %q", entry["user_id"], "user-123")
	}
}

// TestLoggingMiddleware_NoUserID_OmitsField はユーザーIDがない場合にフィールドが空であることを検証する。
func TestLoggingMiddleware_NoUserID_OmitsField(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := NewLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/feeds", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	// user_idフィールドは存在しないか空
	if val, ok := entry["user_id"]; ok && val != "" {
		t.Errorf("user_id should be empty for unauthenticated request, got %q", val)
	}
}

// TestLoggingMiddleware_CapturesStatusCode はステータスコードが正しくキャプチャされることを検証する。
func TestLoggingMiddleware_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

			handler := NewLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			var entry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("failed to parse JSON log: %v", err)
			}

			if status := int(entry["status"].(float64)); status != tt.statusCode {
				t.Errorf("status = %d, want %d", status, tt.statusCode)
			}
		})
	}
}

// TestLoggingMiddleware_DurationIsPositive は処理時間が正の値であることを検証する。
func TestLoggingMiddleware_DurationIsPositive(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := NewLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	duration := entry["duration_ms"].(float64)
	if duration < 0 {
		t.Errorf("duration_ms = %v, should be >= 0", duration)
	}
}

// TestLoggingMiddleware_BodyWriteCapture はレスポンスボディ書き込み後もステータスが記録されることを検証する。
func TestLoggingMiddleware_BodyWriteCapture(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := NewLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WriteHeaderを呼ばずにWriteすると暗黙的に200が設定される
		w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if status := int(entry["status"].(float64)); status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
}
