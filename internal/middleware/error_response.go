package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/hitoshi/feedman/internal/model"
)

// ErrorResponseBody はAPIエラーレスポンスの統一フォーマット。
// 原因カテゴリと対処方法を含む。
// Details は任意の構造化追加情報（429 等で retry_after_seconds 等を載せる）。
// nil の場合は JSON シリアライズ時に出力されない（omitempty 相当）。
type ErrorResponseBody struct {
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Category string         `json:"category"`
	Action   string         `json:"action"`
	Details  map[string]any `json:"details,omitempty"`
}

// WriteErrorResponse は統一エラーフォーマットでHTTPエラーレスポンスを書き込む。
// すべてのAPIエンドポイントで一貫したエラーレスポンスを提供する。
// apiErr.Details が nil でない場合は JSON に `details` フィールドとして含める（Issue #115 Req 2.2）。
func WriteErrorResponse(w http.ResponseWriter, statusCode int, apiErr *model.APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponseBody{
		Code:     apiErr.Code,
		Message:  apiErr.Message,
		Category: apiErr.Category,
		Action:   apiErr.Action,
		Details:  apiErr.Details,
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
