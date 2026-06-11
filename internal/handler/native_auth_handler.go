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
//
// RotateRefreshToken は Issue #167 で、RevokeRefreshToken は Issue #168 で追加された。
// Token / Refresh / Revoke handler のいずれも auth.TokenService の各メソッドを呼ぶだけで、
// handler 層はビジネスロジックを持たない。
type TokenExchangeService interface {
	ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*auth.TokenPair, error)
	RotateRefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	RevokeRefreshToken(ctx context.Context, refreshToken string) error
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

// refreshRequest は POST /api/auth/refresh のリクエストボディ（Issue #167）。
// SERVER.md §1.3 の契約に従い、snake_case で受ける。
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh は POST /api/auth/refresh を処理する（Issue #167）。
//
//   - 200: {access_token, refresh_token, token_type: "Bearer", expires_in: 900}（Req 1.1, 1.6）
//     成功応答は Token と同一の tokenResponse 形式（Req 1.6: token 交換と同一フィールド）。
//   - 400 INVALID_REQUEST:       JSON 不正・必須フィールド欠落（Req 2.5）
//   - 401 INVALID_REFRESH_TOKEN: ErrInvalidRefreshToken に正規化された拒否（Req 2.6 / SERVER.md §1.3）
//     token の存在有無 / 期限切れ / 失効済み / rotation 済みを区別しない（Req 2.6）。
//   - 500 INTERNAL_ERROR:        上記以外（DB 障害・JWT 発行失敗など、NFR 1.3）
//
// 認証不要グループに登録されるため、セッション / Bearer なしで呼び出される（Req 1.5）。
// token 交換の 400 INVALID_GRANT とは status code / error code が異なる点に注意。
func (h *NativeAuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// JSON 不正・必須フィールド欠落は 400 INVALID_REQUEST に合流する。
	// ボディ上限超過（MaxBytesReader）も json.Decode のエラーとしてここで合流する。
	var req refreshRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// クライアント入力値・パーサ詳細はクライアントへ反射しない（NFR 1.3）。
		slog.Info("refresh rejected: invalid request body")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRefreshRequestError())
		return
	}
	if req.RefreshToken == "" {
		slog.Info("refresh rejected: missing refresh_token")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRefreshRequestError())
		return
	}

	pair, err := h.svc.RotateRefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			// token 不明 / 期限切れ / 失効済み / rotation 済みを区別しない（Req 2.6）。
			slog.Info("refresh rejected: invalid refresh token")
			middleware.WriteErrorResponse(w, http.StatusUnauthorized, invalidRefreshTokenError())
			return
		}
		// インフラ起因（DB 障害 / JWT 発行失敗）。詳細は slog のみ、応答は固定メッセージ。
		slog.Error("refresh failed", slog.String("error", err.Error()))
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

// invalidRefreshRequestError は POST /api/auth/refresh の 400 INVALID_REQUEST 固定 APIError。
// クライアント入力値・内部詳細を反射しないこと（NFR 1.3）。
func invalidRefreshRequestError() *model.APIError {
	return &model.APIError{
		Code:     "INVALID_REQUEST",
		Message:  "リクエストの形式が不正です。",
		Category: "validation",
		Action:   "refresh_token を正しい JSON 形式で送信してください。",
	}
}

// invalidRefreshTokenError は POST /api/auth/refresh の 401 INVALID_REFRESH_TOKEN 固定 APIError。
// refresh token の存在有無 / 期限切れ / 失効済み / rotation 済みを区別しない（Req 2.6）。
// SERVER.md §1.3 の契約（401 INVALID_REFRESH_TOKEN）に対応する。
func invalidRefreshTokenError() *model.APIError {
	return &model.APIError{
		Code:     "INVALID_REFRESH_TOKEN",
		Message:  "refresh token が無効です。",
		Category: "auth",
		Action:   "ログインからやり直してください。",
	}
}

// Revoke は POST /api/auth/revoke を処理する（Issue #168）。
//
//   - 204: 失効成功または対象不明（ボディなし）。token の存在有無・状態を区別しない
//     （Req 2.1, 2.2 / NFR 1.2: 冪等・列挙オラクルなし）。
//   - 400 INVALID_REQUEST: JSON 不正・必須フィールド欠落（Req 2.5）
//   - 500 INTERNAL_ERROR:  infra エラー（DB 障害など、NFR 1.3）
//
// 認証不要グループに登録されるため、セッション / Bearer なしで呼び出される
// （Req 2.4: refresh token の所持自体を失効権限とみなす。requirements.md Open
// Questions / RFC 7009 §2.1 の public client 慣行）。
// リクエストボディは refresh と同一形式（refreshRequest を共用）。
func (h *NativeAuthHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	// JSON 不正・必須フィールド欠落は 400 INVALID_REQUEST に合流する。
	// ボディ上限超過（MaxBytesReader）も json.Decode のエラーとしてここで合流する。
	var req refreshRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// クライアント入力値・パーサ詳細はクライアントへ反射しない（NFR 1.3）。
		slog.Info("revoke rejected: invalid request body")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRefreshRequestError())
		return
	}
	if req.RefreshToken == "" {
		slog.Info("revoke rejected: missing refresh_token")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRefreshRequestError())
		return
	}

	if err := h.svc.RevokeRefreshToken(r.Context(), req.RefreshToken); err != nil {
		// infra 起因（DB 障害）のみここに到達する（不明 token は service が nil を返す）。
		// 詳細は slog のみ、応答は固定メッセージ（NFR 1.3）。
		slog.Error("revoke failed", slog.String("error", err.Error()))
		middleware.WriteInternalServerError(w)
		return
	}

	// 失効成功・対象不明のいずれも同一の 204（ボディなし / Req 2.2 / NFR 1.2）。
	w.WriteHeader(http.StatusNoContent)
}
