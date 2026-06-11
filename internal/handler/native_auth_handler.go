package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
)

// TokenExchangeService は NativeAuthHandler が必要とする最小サービス IF。
// auth.TokenService が構造的に充足する（interface segregation）。
type TokenExchangeService interface {
	ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error)
}

// NativeAuthHandler は POST /api/auth/token を処理する HTTP ハンドラ（Issue #166）。
// JSON I/O のみを担当し、ビジネスロジックは TokenExchangeService に委譲する。
type NativeAuthHandler struct {
	svc TokenExchangeService
}

// NewNativeAuthHandler は TokenExchangeService を注入して NativeAuthHandler を生成する。
func NewNativeAuthHandler(svc TokenExchangeService) *NativeAuthHandler {
	return &NativeAuthHandler{svc: svc}
}

// tokenRequest は POST /api/auth/token のリクエストボディ。
// SERVER.md §1.3 の契約に従い、snake_case で受ける。
type tokenRequest struct {
	AuthCode     string `json:"auth_code"`
	CodeVerifier string `json:"code_verifier"`
}

// tokenResponse は POST /api/auth/token の 200 応答ボディ。
// SERVER.md §1.3 の契約に従い、すべて snake_case のフィールド名を使う（Req 1.6）。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// Token は POST /api/auth/token を処理する（Issue #166）。
//
//   - 200: {access_token, refresh_token, token_type: "Bearer", expires_in: 900}（Req 1.1, 1.6）
//   - 400 INVALID_REQUEST: JSON 不正・必須フィールド欠落（Req 2.5）
//   - 400 INVALID_GRANT:   ErrInvalidGrant に正規化された拒否（Req 2.6）
//   - 500 INTERNAL_ERROR:  上記以外（DB 障害・JWT 発行失敗など、NFR 1.5）
//
// 認証不要グループに登録されるため、セッション / Bearer なしで呼び出される（Req 1.5）。
func (h *NativeAuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	// JSON 不正・必須フィールド欠落は 400 INVALID_REQUEST に合流する。
	// ボディ上限超過（MaxBytesReader）も json.Decode のエラーとしてここで合流する。
	var req tokenRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// クライアント入力値・パーサ詳細はクライアントへ反射しない（NFR 1.5）。
		slog.Info("token exchange rejected: invalid request body")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRequestError())
		return
	}
	if req.AuthCode == "" || req.CodeVerifier == "" {
		slog.Info("token exchange rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRequestError())
		return
	}

	pair, err := h.svc.ExchangeAuthCode(r.Context(), req.AuthCode, req.CodeVerifier)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidGrant) {
			// auth_code 不明 / 期限切れ / 使用済み / verifier 不一致を区別しない（Req 2.6）。
			slog.Info("token exchange rejected: invalid grant")
			middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidGrantError())
			return
		}
		// インフラ起因（DB 障害 / JWT 発行失敗）。詳細は slog のみ、応答は固定メッセージ。
		slog.Error("token exchange failed", slog.String("error", err.Error()))
		middleware.WriteInternalServerError(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    pair.ExpiresIn,
	})
}

// invalidRequestError は 400 INVALID_REQUEST の固定 APIError を返す。
// クライアント入力値・内部詳細を反射しないこと（NFR 1.5）。
func invalidRequestError() *model.APIError {
	return &model.APIError{
		Code:     "INVALID_REQUEST",
		Message:  "リクエストの形式が不正です。",
		Category: "validation",
		Action:   "auth_code と code_verifier を正しい JSON 形式で送信してください。",
	}
}

// invalidGrantError は 400 INVALID_GRANT の固定 APIError を返す。
// auth_code の存在有無 / 期限切れ / 使用済み / verifier 不一致を区別しない（Req 2.6）。
func invalidGrantError() *model.APIError {
	return &model.APIError{
		Code:     "INVALID_GRANT",
		Message:  "auth_code を交換できませんでした。",
		Category: "auth",
		Action:   "ログインからやり直してください。",
	}
}
