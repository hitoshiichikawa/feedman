package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hitoshi/feedman/internal/model"
)

// TestWriteErrorResponse_WritesUnifiedFormat は統一エラーフォーマットでレスポンスが書き込まれることを検証する。
func TestWriteErrorResponse_WritesUnifiedFormat(t *testing.T) {
	w := httptest.NewRecorder()

	apiErr := &model.APIError{
		Code:     "TEST_ERROR",
		Message:  "テストエラーです。",
		Category: "validation",
		Action:   "正しい値を入力してください。",
	}

	WriteErrorResponse(w, http.StatusBadRequest, apiErr)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body ErrorResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body.Code != "TEST_ERROR" {
		t.Errorf("code = %q, want %q", body.Code, "TEST_ERROR")
	}
	if body.Message != "テストエラーです。" {
		t.Errorf("message = %q, want %q", body.Message, "テストエラーです。")
	}
	if body.Category != "validation" {
		t.Errorf("category = %q, want %q", body.Category, "validation")
	}
	if body.Action != "正しい値を入力してください。" {
		t.Errorf("action = %q, want %q", body.Action, "正しい値を入力してください。")
	}
}

// TestWriteErrorResponse_DifferentStatusCodes は異なるステータスコードで正しく動作することを検証する。
func TestWriteErrorResponse_DifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		code       string
		category   string
	}{
		{"Unauthorized", http.StatusUnauthorized, "UNAUTHORIZED", "auth"},
		{"Forbidden", http.StatusForbidden, "SSRF_BLOCKED", "validation"},
		{"NotFound", http.StatusNotFound, "ITEM_NOT_FOUND", "feed"},
		{"Conflict", http.StatusConflict, "SUBSCRIPTION_LIMIT", "feed"},
		{"Internal", http.StatusInternalServerError, "INTERNAL_ERROR", "system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			WriteErrorResponse(w, tt.statusCode, &model.APIError{
				Code:     tt.code,
				Message:  "test",
				Category: tt.category,
				Action:   "test action",
			})

			resp := w.Result()
			if resp.StatusCode != tt.statusCode {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.statusCode)
			}

			var body ErrorResponseBody
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}

			if body.Code != tt.code {
				t.Errorf("code = %q, want %q", body.Code, tt.code)
			}
			if body.Category != tt.category {
				t.Errorf("category = %q, want %q", body.Category, tt.category)
			}
		})
	}
}

// TestInternalServerError_ReturnsSystemError は内部エラーが統一フォーマットで返ることを検証する。
func TestInternalServerError_ReturnsSystemError(t *testing.T) {
	w := httptest.NewRecorder()

	WriteInternalServerError(w)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var body ErrorResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if body.Code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want %q", body.Code, "INTERNAL_ERROR")
	}
	if body.Category != "system" {
		t.Errorf("category = %q, want %q", body.Category, "system")
	}
	if body.Action == "" {
		t.Error("action should not be empty")
	}
}

// TestErrorResponseBody_AllFieldsPresent は全フィールドがJSONレスポンスに含まれることを検証する。
func TestErrorResponseBody_AllFieldsPresent(t *testing.T) {
	w := httptest.NewRecorder()

	WriteErrorResponse(w, http.StatusBadRequest, &model.APIError{
		Code:     "CODE",
		Message:  "MSG",
		Category: "CAT",
		Action:   "ACT",
	})

	var raw map[string]interface{}
	if err := json.NewDecoder(w.Result().Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	requiredFields := []string{"code", "message", "category", "action"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}
}
