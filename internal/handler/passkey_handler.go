package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/passkey"
)

// 本ファイルは iOS クライアントが呼び出すパスキー登録・認証・追加登録の 6 endpoint を
// 提供する `PasskeyHandler` を実装する（Issue #216 / design.md PasskeyHandler /
// Req 1.1〜1.7, 2.1〜2.6, 3.1〜3.7, NFR 1.3）。
//
// ハンドラは JSON I/O のみを担当し、認可・ビジネスロジックは Service 層に委譲する
// （CLAUDE.md §1: レイヤリング / handler → service）。
// 拒否理由の区別を防ぐため、認証系は AUTHENTICATION_FAILED / 登録系は
// REGISTRATION_FAILED の各 1 種に uniform 化する（Req 1.7 / 2.6 / 3.6 / 3.7）。
// レスポンス JSON は既存 `middleware.WriteErrorResponse` / `WriteInternalServerError`
// を再利用する（NFR 1.3: 既存応答契約を破らない）。

// PasskeyRegistrationService は PasskeyHandler が登録・追加登録 4 メソッドに必要とする
// 最小 interface である（interface segregation / CLAUDE.md §5）。
// `*passkey.RegistrationService` が構造的にこれを充足する。
type PasskeyRegistrationService interface {
	BeginRegistrationNew(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (challengeID string, options []byte, err error)
	FinishRegistrationNew(ctx context.Context, challengeID string, requestBody []byte) (userID string, err error)
	BeginAddCredential(ctx context.Context, authenticatedUserID string) (challengeID string, options []byte, err error)
	FinishAddCredential(ctx context.Context, authenticatedUserID, challengeID string, requestBody []byte) error
}

// PasskeyAuthenticationService は PasskeyHandler が認証 2 メソッドに必要とする最小
// interface である（interface segregation）。`*passkey.AuthenticationService` が構造的に
// これを充足する。
type PasskeyAuthenticationService interface {
	BeginAuthentication(ctx context.Context, codeChallenge string) (challengeID string, options []byte, err error)
	FinishAuthentication(ctx context.Context, requestBody []byte, challengeID string) (authCodePlain string, err error)
}

// PasskeyHandler はパスキー登録・認証の HTTP endpoint 6 種を提供する。
//
// 認証必須 endpoint（追加登録の begin/finish 2 種）は BearerOrSession middleware 通過後
// に呼ばれ、`middleware.UserIDFromContext` で認証済み userID を取得する。未認証時は 401。
// 未認証 endpoint（登録新規・認証の 4 種）は unauthIPMW + MaxBodyBytes を通過する
// （router 側で設定）。
type PasskeyHandler struct {
	registration   PasskeyRegistrationService
	authentication PasskeyAuthenticationService
}

// NewPasskeyHandler は PasskeyHandler を生成する。両依存とも非 nil を要求する契約とし、
// router 側は「passkey wiring 全体が組めているとき」にのみ本 handler を生成する
// （fail-closed 縮退は wiring 層の責務）。
func NewPasskeyHandler(reg PasskeyRegistrationService, authn PasskeyAuthenticationService) *PasskeyHandler {
	return &PasskeyHandler{
		registration:   reg,
		authentication: authn,
	}
}

// --- リクエスト / レスポンス DTO ---

// passkeyBeginResponse は begin 系 endpoint の 200 応答共通形。
// `options` は WebAuthn library が生成した pre-marshal 済み JSON を透過的にクライアントへ
// 返す（NavigatorCredentials に渡せる形式）。json.RawMessage を使うことで再 marshal を回避する。
type passkeyBeginResponse struct {
	ChallengeID string          `json:"challenge_id"`
	Options     json.RawMessage `json:"options"`
}

// registrationBeginRequest は POST /api/passkey/registration/begin の request body。
// email はリカバリ用の任意項目（Req 1.6）。code_challenge は PKCE S256 43 文字を必須
// （design.md「PKCE 相互作用」節）。
type registrationBeginRequest struct {
	Username      string `json:"username"`
	Email         string `json:"email,omitempty"`
	CodeChallenge string `json:"code_challenge"`
}

// registrationFinishRequest は POST /api/passkey/registration/finish（新規登録）・
// /add/finish（追加登録）の request body 共通形。`credential` は WebAuthn client が
// 返す attestation response の生 JSON を透過的に service へ渡す（parse は adapter 側）。
type registrationFinishRequest struct {
	ChallengeID string          `json:"challenge_id"`
	Credential  json.RawMessage `json:"credential"`
}

// registrationFinishNewResponse は POST /api/passkey/registration/finish の 200 応答。
// 新規作成された user の ID を返し、以降のパスキー認証で当該ユーザーとして解決可能な
// 状態を通知する（Req 1.3）。
type registrationFinishNewResponse struct {
	UserID string `json:"user_id"`
}

// authenticationBeginRequest は POST /api/passkey/authentication/begin の request body。
// code_challenge は PKCE S256（begin 時に検証、challenge に紐付けて finish 時に auth_code
// の PKCEChallenge へ設定される / Req 2.4 の合流準備）。
type authenticationBeginRequest struct {
	CodeChallenge string `json:"code_challenge"`
}

// authenticationFinishResponse は POST /api/passkey/authentication/finish の 200 応答。
// 平文 auth_code を返す（クライアントは以降 `POST /api/auth/token` に PKCE code_verifier
// とともに送信して既存 native auth 契約に合流する / Req 2.4）。
type authenticationFinishResponse struct {
	AuthCode string `json:"auth_code"`
}

// --- ハンドラ本体 ---

// RegistrationBegin は POST /api/passkey/registration/begin を処理する（Req 1.1, 1.4, 1.5）。
//
//   - 200: {challenge_id, options}
//   - 400 INVALID_REQUEST:      JSON 不正・必須フィールド欠落・ボディ上限超過
//   - 400 INVALID_USERNAME:     ErrInvalidUsername に正規化された拒否（Req 1.5）
//   - 409 USERNAME_TAKEN:       ErrUsernameTaken に正規化された拒否（Req 1.4）
//   - 400 REGISTRATION_FAILED:  ErrRegistrationFailed に正規化された拒否（PKCE 不正 / WebAuthn 内部拒否）
//   - 500 INTERNAL_ERROR:       上記以外（DB 障害・library 内部エラー）
//
// 認証不要グループに登録されるため Session / Bearer なしで呼び出される。
func (h *PasskeyHandler) RegistrationBegin(w http.ResponseWriter, r *http.Request) {
	var req registrationBeginRequest
	if !decodeRequest(w, r, &req, "passkey registration begin") {
		return
	}
	if req.Username == "" || req.CodeChallenge == "" {
		slog.Info("passkey registration begin rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return
	}

	challengeID, options, err := h.registration.BeginRegistrationNew(
		r.Context(), req.Username, req.Email, req.CodeChallenge,
	)
	if err != nil {
		h.writeRegistrationError(w, err, "passkey registration begin failed")
		return
	}

	writeJSON(w, http.StatusOK, passkeyBeginResponse{
		ChallengeID: challengeID,
		Options:     options,
	})
}

// RegistrationFinish は POST /api/passkey/registration/finish を処理する（Req 1.2, 1.3, 1.6, 1.7）。
//
//   - 200: {user_id}
//   - 400 INVALID_REQUEST:      JSON 不正・必須フィールド欠落・ボディ上限超過
//   - 400 REGISTRATION_FAILED:  ErrRegistrationFailed に正規化された拒否（Req 1.7）
//   - 500 INTERNAL_ERROR:       上記以外
func (h *PasskeyHandler) RegistrationFinish(w http.ResponseWriter, r *http.Request) {
	var req registrationFinishRequest
	if !decodeRequest(w, r, &req, "passkey registration finish") {
		return
	}
	if req.ChallengeID == "" || len(req.Credential) == 0 {
		slog.Info("passkey registration finish rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return
	}

	userID, err := h.registration.FinishRegistrationNew(r.Context(), req.ChallengeID, req.Credential)
	if err != nil {
		h.writeRegistrationError(w, err, "passkey registration finish failed")
		return
	}

	writeJSON(w, http.StatusOK, registrationFinishNewResponse{UserID: userID})
}

// RegistrationAddBegin は POST /api/passkey/registration/add/begin を処理する（Req 3.1, 3.3, 3.5）。
//
//   - 200: {challenge_id, options}
//   - 400 INVALID_REQUEST:      JSON 不正・ボディ上限超過（body 未指定は許容）
//   - 401 UNAUTHORIZED:         BearerOrSession 通過後に user context が取得できない場合
//   - 400 REGISTRATION_FAILED:  ErrRegistrationFailed に正規化された拒否
//   - 500 INTERNAL_ERROR:       上記以外
//
// 認証必須グループに登録されるため、middleware 側で 401 が返される前提だが、context
// 取得失敗時の防衛として本 handler でも 401 を返す。
func (h *PasskeyHandler) RegistrationAddBegin(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// request body は空許容（design.md L709: {}）。空でも malformed でもない場合は
	// unknown フィールドの混入を拒否するため decode を試みる。
	if !decodeOptionalEmpty(w, r, "passkey registration add begin") {
		return
	}

	challengeID, options, err := h.registration.BeginAddCredential(r.Context(), userID)
	if err != nil {
		h.writeRegistrationError(w, err, "passkey registration add begin failed")
		return
	}

	writeJSON(w, http.StatusOK, passkeyBeginResponse{
		ChallengeID: challengeID,
		Options:     options,
	})
}

// RegistrationAddFinish は POST /api/passkey/registration/add/finish を処理する（Req 3.2, 3.6, 3.7）。
//
//   - 204: 追加登録成功（ボディなし）
//   - 400 INVALID_REQUEST:      JSON 不正・必須フィールド欠落・ボディ上限超過
//   - 401 UNAUTHORIZED:         user context 取得失敗
//   - 400 REGISTRATION_FAILED:  ErrRegistrationFailed に正規化された拒否（Req 3.6 / 3.7）
//   - 500 INTERNAL_ERROR:       上記以外
func (h *PasskeyHandler) RegistrationAddFinish(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req registrationFinishRequest
	if !decodeRequest(w, r, &req, "passkey registration add finish") {
		return
	}
	if req.ChallengeID == "" || len(req.Credential) == 0 {
		slog.Info("passkey registration add finish rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return
	}

	if err := h.registration.FinishAddCredential(r.Context(), userID, req.ChallengeID, req.Credential); err != nil {
		h.writeRegistrationError(w, err, "passkey registration add finish failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AuthenticationBegin は POST /api/passkey/authentication/begin を処理する（Req 2.1, 2.6）。
//
//   - 200: {challenge_id, options}
//   - 400 INVALID_REQUEST:         JSON 不正・必須フィールド欠落・ボディ上限超過
//   - 400 AUTHENTICATION_FAILED:   ErrAuthenticationFailed に正規化された拒否（PKCE 不正）
//   - 500 INTERNAL_ERROR:          上記以外
func (h *PasskeyHandler) AuthenticationBegin(w http.ResponseWriter, r *http.Request) {
	var req authenticationBeginRequest
	if !decodeRequest(w, r, &req, "passkey authentication begin") {
		return
	}
	if req.CodeChallenge == "" {
		slog.Info("passkey authentication begin rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return
	}

	challengeID, options, err := h.authentication.BeginAuthentication(r.Context(), req.CodeChallenge)
	if err != nil {
		h.writeAuthenticationError(w, err, "passkey authentication begin failed")
		return
	}

	writeJSON(w, http.StatusOK, passkeyBeginResponse{
		ChallengeID: challengeID,
		Options:     options,
	})
}

// AuthenticationFinish は POST /api/passkey/authentication/finish を処理する
// （Req 2.2, 2.3, 2.5, 2.6, NFR 1.4）。
//
//   - 200: {auth_code}（平文。以降のクライアント側 token 交換で使用）
//   - 400 INVALID_REQUEST:         JSON 不正・必須フィールド欠落・ボディ上限超過
//   - 400 AUTHENTICATION_FAILED:   ErrAuthenticationFailed に正規化された拒否
//     （Req 2.5 / 2.6 / NFR 1.4 の存在有無非開示に整合）
//   - 500 INTERNAL_ERROR:          上記以外
func (h *PasskeyHandler) AuthenticationFinish(w http.ResponseWriter, r *http.Request) {
	var req registrationFinishRequest
	if !decodeRequest(w, r, &req, "passkey authentication finish") {
		return
	}
	if req.ChallengeID == "" || len(req.Credential) == 0 {
		slog.Info("passkey authentication finish rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return
	}

	authCode, err := h.authentication.FinishAuthentication(r.Context(), req.Credential, req.ChallengeID)
	if err != nil {
		h.writeAuthenticationError(w, err, "passkey authentication finish failed")
		return
	}

	writeJSON(w, http.StatusOK, authenticationFinishResponse{AuthCode: authCode})
}

// --- helpers ---

// decodeRequest は JSON リクエストボディを厳格 decode し、失敗時は 400 INVALID_REQUEST を
// 返して false を返す。既存 `NativeAuthHandler` 流儀（DisallowUnknownFields）と揃える。
// エラー内容はクライアントに反射せず slog に短いメッセージだけを残す（NFR 1.3）。
func decodeRequest(w http.ResponseWriter, r *http.Request, dst any, logCtx string) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		slog.Info(logCtx + " rejected: invalid request body")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return false
	}
	return true
}

// decodeOptionalEmpty は body が空でも許容し、非空なら unknown フィールドを拒否する。
// 空 body（Content-Length 0）は io.EOF を返すが、それは正常系として扱う。
func decodeOptionalEmpty(w http.ResponseWriter, r *http.Request, logCtx string) bool {
	var empty struct{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&empty); err != nil && !errors.Is(err, io.EOF) {
		slog.Info(logCtx + " rejected: invalid request body")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, passkeyInvalidRequestError())
		return false
	}
	return true
}

// writeJSON は 200 系の成功応答を JSON で書き出す共通ヘルパー。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// レスポンス書き込み失敗はネットワーク断等に起因するため、
		// ここでの追加レスポンスは不可能（ヘッダは送信済）。ログのみ残す。
		slog.Warn("failed to encode passkey response", slog.String("error", err.Error()))
	}
}

// writeRegistrationError は passkey 登録系サービスが返した error を APIError に射影する。
//
// マッピング（tasks.md L173-177 に厳密準拠）:
//   - ErrInvalidUsername      -> 400 INVALID_USERNAME
//   - ErrUsernameTaken        -> 409 USERNAME_TAKEN
//   - ErrRegistrationFailed   -> 400 REGISTRATION_FAILED
//   - それ以外（infra エラー） -> 500 INTERNAL_ERROR
//
// slog の詳細ログは err.Error() を含めるが、クライアント応答には反射しない（NFR 1.3）。
func (h *PasskeyHandler) writeRegistrationError(w http.ResponseWriter, err error, logMsg string) {
	switch {
	case errors.Is(err, passkey.ErrInvalidUsername):
		slog.Info(logMsg + ": invalid username")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, model.NewInvalidUsernameError())
	case errors.Is(err, passkey.ErrUsernameTaken):
		slog.Info(logMsg + ": username taken")
		middleware.WriteErrorResponse(w, http.StatusConflict, model.NewUsernameTakenError())
	case errors.Is(err, passkey.ErrRegistrationFailed):
		slog.Info(logMsg + ": registration failed")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, model.NewRegistrationFailedError())
	default:
		slog.Error(logMsg, slog.String("error", err.Error()))
		middleware.WriteInternalServerError(w)
	}
}

// writeAuthenticationError は passkey 認証系サービスが返した error を APIError に射影する。
//
// マッピング（tasks.md L176-177 に厳密準拠）:
//   - ErrAuthenticationFailed -> 400 AUTHENTICATION_FAILED
//   - それ以外（infra エラー） -> 500 INTERNAL_ERROR
//
// Req 2.6 の存在有無非開示に整合するため、challenge / credential / user のいずれで
// 拒否したかを区別しない。
func (h *PasskeyHandler) writeAuthenticationError(w http.ResponseWriter, err error, logMsg string) {
	if errors.Is(err, passkey.ErrAuthenticationFailed) {
		slog.Info(logMsg + ": authentication failed")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, model.NewAuthenticationFailedError())
		return
	}
	slog.Error(logMsg, slog.String("error", err.Error()))
	middleware.WriteInternalServerError(w)
}

// passkeyInvalidRequestError は passkey ハンドラ共通の 400 INVALID_REQUEST 固定 APIError。
// 既存 `invalidRequestError` は auth 系専用の action 文言（"auth_code と ..."）を返すため、
// passkey 系は汎用の action 文言に統一する。
func passkeyInvalidRequestError() *model.APIError {
	return &model.APIError{
		Code:     "INVALID_REQUEST",
		Message:  "リクエストの形式が不正です。",
		Category: "validation",
		Action:   "リクエストの JSON 形式・必須フィールド・ボディサイズを確認してください。",
	}
}
