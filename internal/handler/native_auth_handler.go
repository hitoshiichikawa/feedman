package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
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

// SessionExchanger は NativeAuthHandler.Session が必要とする最小サービス IF（Issue #223 / task 2）。
// auth.SessionExchangeService が構造的に充足する（interface segregation / CLAUDE.md §5）。
// 既存 TokenExchangeService と同じ idiom で、handler 層は具体型に依存しない。
type SessionExchanger interface {
	ExchangeAuthCodeForSession(ctx context.Context, authCode, codeVerifier string) (*model.Session, error)
}

// NativeAuthHandler は POST /api/auth/token を処理する HTTP ハンドラ（Issue #166）。
// JSON I/O のみを担当し、ビジネスロジックは TokenExchangeService に委譲する。
//
// Issue #223 / task 2 で SessionExchanger（POST /api/auth/session 用）と Cookie 発行の
// 設定値（Domain / Secure / Max-Age）を注入するフィールドを追加した。既存 Token /
// Refresh / Revoke の挙動は不変（NFR 2.1）。sessionExchange が nil のときは Session()
// を呼ばない構成に限定される（router 側で NativeAuthHandler != nil の gate で連動制御）。
type NativeAuthHandler struct {
	svc TokenExchangeService

	// Session 経路（Issue #223 / task 2）で使う依存。未注入時は nil（既存 test 経路との
	// 互換維持のため variadic Option で任意注入とする）。
	sessionExchange SessionExchanger
	cookieDomain    string
	cookieSecure    bool
	sessionMaxAge   int

	// allowedOrigin は POST /api/auth/session の CSRF 対策で許可する strict exact Origin。
	// 空文字・Origin 不在・不一致はいずれも fail-closed で拒否する。wiring は
	// cfg.WebPasskeyAllowedOrigin を注入する（Issue #231 Delta 4/6）。
	allowedOrigin string
}

// NativeAuthHandlerOption は NewNativeAuthHandler の functional option。
// 既存呼び出し `NewNativeAuthHandler(svc)` の互換を保ちつつ Session 経路の依存を
// 任意注入するために採用（既存 item.WithMetrics / fetchpkg.WithMetrics と同 idiom）。
type NativeAuthHandlerOption func(*NativeAuthHandler)

// WithSessionExchange は POST /api/auth/session の依存を注入する Option。
// Cookie 属性（Domain / Secure / Max-Age）は既存 Google OAuth Callback と一致させるため
// 呼び出し側の設定値（cfg.CookieDomain / cfg.CookieSecure / cfg.SessionMaxAge）を
// そのまま受け取る。Cookie 名 / HttpOnly / SameSite / Path は Session() 内でハードコード
// （既存 sessionCookieName / http.SameSiteLaxMode / true / "/" と一致）。
func WithSessionExchange(exchange SessionExchanger, cookieDomain string, cookieSecure bool, sessionMaxAge int) NativeAuthHandlerOption {
	return func(h *NativeAuthHandler) {
		h.sessionExchange = exchange
		h.cookieDomain = cookieDomain
		h.cookieSecure = cookieSecure
		h.sessionMaxAge = sessionMaxAge
	}
}

// WithSessionAllowedOrigin は POST /api/auth/session の CSRF 対策で許可する strict exact
// Origin を注入する。cfg.WebPasskeyAllowedOrigin を渡し、空文字の場合は Session が
// 全リクエストを 403 に倒す（Issue #231 Delta 4/6）。既存 WithSessionExchange と
// 直交する additive Option。
func WithSessionAllowedOrigin(allowedOrigin string) NativeAuthHandlerOption {
	return func(h *NativeAuthHandler) {
		h.allowedOrigin = allowedOrigin
	}
}

// NewNativeAuthHandler は TokenExchangeService を注入して NativeAuthHandler を生成する。
// Session 経路（Issue #223 / task 2）の依存は WithSessionExchange オプションで任意注入する。
// 既存 test の 1 引数呼び出し `NewNativeAuthHandler(svc)` は variadic のため互換維持される
// （NFR 2.1）。
func NewNativeAuthHandler(svc TokenExchangeService, opts ...NativeAuthHandlerOption) *NativeAuthHandler {
	h := &NativeAuthHandler{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// SessionReady は Session 経路（POST /api/auth/session）が実際に処理可能な状態
// （WithSessionExchange で SessionExchanger が注入済み）かを返す（Issue #223 review #5）。
//
// WithSessionExchange は任意 Option のため、`NewNativeAuthHandler(svc)` だけでも handler は
// 生成できるが、その状態で Session() を呼ぶと sessionExchange が nil で panic（500）になる。
// router 側はこの readiness で POST /api/auth/session と GET /api/passkey/capability の登録を
// gate し、「capability は 200 なのに session が nil 依存で 500」という不整合を fail-closed
// （両 route 未登録 = 404）に倒す。Token / Refresh / Revoke は本 readiness に依存しない
// （sessionExchange 不要）ため、引き続き NativeAuthHandler != nil のみで登録される。
func (h *NativeAuthHandler) SessionReady() bool {
	return h.sessionExchange != nil
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

// sessionRequest は POST /api/auth/session のリクエストボディ（Issue #223 / task 2）。
// snake_case で受ける（既存 tokenRequest と同一 idiom）。
type sessionRequest struct {
	AuthCode     string `json:"auth_code"`
	CodeVerifier string `json:"code_verifier"`
}

// Session は POST /api/auth/session を処理する（Issue #223 / task 2 / design.md
// §NativeAuthHandler.Session）。パスキー認証で発行された auth_code を PKCE 検証付きで
// 単回消費し、Web 用の Cookie session を発行する。
//
//   - 204: 成功。Set-Cookie: session_id=...（既存 Google OAuth Callback と完全同一属性）
//   - 400 INVALID_REQUEST:        JSON 不正・必須フィールド欠落
//   - 400 INVALID_GRANT:          ErrInvalidGrant に正規化された拒否（未検出 / PKCE 不一致 / used / 期限切れ）
//   - 403 FORBIDDEN_ORIGIN:       Origin ヘッダが許可オリジンと不一致（login CSRF 対策）
//   - 415 UNSUPPORTED_MEDIA_TYPE: Content-Type が application/json でない（login CSRF 対策）
//   - 500 INTERNAL_ERROR:         上記以外（DB 障害等 / NFR 1.1）
//
// 認証不要グループに登録されるため、セッション / Bearer なしで呼び出される。
// 既存 sessionExchange.ExchangeAuthCodeForSession は平文 authCode / codeVerifier /
// sessionID をログ・エラーメッセージに出さないため、本 handler も応答に内部詳細を
// 反射しない（NFR 1.1）。
//
// CSRF 対策（Issue #223 review #2）: 本 endpoint は Cookie セッションを新規発行する
// browser-facing な副作用を持つため、既存の SameSite=Lax + auth_code 単回消費 + PKCE 束縛に
// 加えて、以下の 2 段を defense-in-depth で適用する:
//   - Content-Type: application/json 必須化 — cross-site の HTML form POST（simple request）を
//     弾き、cross-origin fetch には CORS preflight を強制する。Go の json.Decoder は text/plain
//     でも JSON をパースするため、明示検証しないと form ベース CSRF が成立し得る。
//   - Origin allowlist — allowedOrigin が非空かつ Origin が完全一致する場合だけ通す。
//     許可 Origin 未設定・Origin 不在・不一致はすべて 403 に倒す（fail-closed）。
func (h *NativeAuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	// CSRF 対策（Content-Type / Origin）を JSON decode より前に適用する。
	if !hasJSONContentType(r) {
		slog.Info("session exchange rejected: unsupported content-type")
		middleware.WriteErrorResponse(w, http.StatusUnsupportedMediaType, unsupportedMediaTypeError())
		return
	}
	if origin := r.Header.Get("Origin"); h.allowedOrigin == "" || origin == "" || origin != h.allowedOrigin {
		// Origin を検証できない構成・不在・不一致をすべて遮断（login CSRF 対策）。
		// origin 生値はクライアントへ反射しない（固定メッセージのみ / NFR 1.1）。
		slog.Info("session exchange rejected: disallowed origin")
		middleware.WriteErrorResponse(w, http.StatusForbidden, forbiddenOriginError())
		return
	}

	// JSON 不正・必須フィールド欠落は 400 INVALID_REQUEST に合流する。
	// ボディ上限超過（MaxBytesReader）も json.Decode のエラーとしてここで合流する。
	var req sessionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// クライアント入力値・パーサ詳細はクライアントへ反射しない（NFR 1.1）。
		slog.Info("session exchange rejected: invalid request body")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRequestError())
		return
	}
	if req.AuthCode == "" || req.CodeVerifier == "" {
		slog.Info("session exchange rejected: missing required field")
		middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidRequestError())
		return
	}

	session, err := h.sessionExchange.ExchangeAuthCodeForSession(r.Context(), req.AuthCode, req.CodeVerifier)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidGrant) {
			// auth_code 不明 / 期限切れ / 使用済み / verifier 不一致を区別しない
			// （NFR 1.1 と既存 Token / Refresh と同方針）。
			slog.Info("session exchange rejected: invalid grant")
			middleware.WriteErrorResponse(w, http.StatusBadRequest, invalidGrantError())
			return
		}
		// インフラ起因（DB 障害等）。詳細は slog のみ、応答は固定メッセージ。
		slog.Error("session exchange failed", slog.String("error", err.Error()))
		middleware.WriteInternalServerError(w)
		return
	}

	// 既存 Google OAuth Callback（internal/handler/auth_handler.go の Callback 手順 5）
	// と完全同一の Cookie 属性で Set-Cookie する（Name / Path / Domain / MaxAge /
	// HttpOnly / Secure / SameSite）。canonical builder（session_cookie.go）を 3 経路で
	// 共有し、Domain / Secure / MaxAge は wiring 時に注入された値を使用する。
	http.SetCookie(w, buildSessionCookie(
		sessionCookieName, session.ID,
		h.cookieDomain, h.cookieSecure, h.sessionMaxAge,
	))

	// 応答ボディは空（204 No Content）。Web は credentials: "include" により自動的に
	// Cookie を保存し、以降の /auth/me が認証済み状態になる（design.md §NativeAuthHandler.Session）。
	w.WriteHeader(http.StatusNoContent)
}

// hasJSONContentType は Content-Type が application/json（charset 等のパラメータ付き含む）
// であるかを判定する。POST /api/auth/session の CSRF 対策で、simple request な非 JSON POST を
// 弾くために使う（Issue #223 review #2）。空・非 JSON・パース不能はすべて false。
func hasJSONContentType(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

// unsupportedMediaTypeError は 415 UNSUPPORTED_MEDIA_TYPE の固定 APIError を返す。
// CSRF 対策で application/json 以外を弾いた際に使う（Issue #223 review #2）。
func unsupportedMediaTypeError() *model.APIError {
	return &model.APIError{
		Code:     "UNSUPPORTED_MEDIA_TYPE",
		Message:  "リクエストの Content-Type が不正です。",
		Category: "validation",
		Action:   "Content-Type: application/json を指定してください。",
	}
}

// forbiddenOriginError は 403 FORBIDDEN_ORIGIN の固定 APIError を返す。
// CSRF 対策で許可オリジン以外の cross-site POST を弾いた際に使う（Issue #223 review #2）。
// origin 生値・許可オリジンをクライアントへ反射しない（NFR 1.1）。
func forbiddenOriginError() *model.APIError {
	return &model.APIError{
		Code:     "FORBIDDEN_ORIGIN",
		Message:  "許可されていないオリジンからのリクエストです。",
		Category: "auth",
		Action:   "同一オリジンのログイン画面からやり直してください。",
	}
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
