package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/hitoshi/feedman/internal/model"
)

// ErrorResponseBody はAPIエラーレスポンスの統一フォーマット。
// 原因カテゴリと対処方法を含む。
type ErrorResponseBody struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Category string `json:"category"`
	Action   string `json:"action"`
}

// WriteErrorResponse は統一エラーフォーマットでHTTPエラーレスポンスを書き込む。
// すべてのAPIエンドポイントで一貫したエラーレスポンスを提供する。
func WriteErrorResponse(w http.ResponseWriter, statusCode int, apiErr *model.APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponseBody{
		Code:     apiErr.Code,
		Message:  apiErr.Message,
		Category: apiErr.Category,
		Action:   apiErr.Action,
	})
}

// WriteInternalServerError は内部サーバーエラーの統一レスポンスを書き込む。
// 詳細はログのみに記録し、ユーザーには一般的なメッセージを返す。
func WriteInternalServerError(w http.ResponseWriter) {
	WriteErrorResponse(w, http.StatusInternalServerError, &model.APIError{
		Code:     "INTERNAL_ERROR",
		Message:  "内部エラーが発生しました。",
		Category: "system",
		Action:   "しばらく待ってから再度お試しください。",
	})
}
