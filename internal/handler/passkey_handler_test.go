package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hitoshi/feedman/internal/middleware"
	"github.com/hitoshi/feedman/internal/passkey"
)

// 本 test は PasskeyHandler の HTTP 応答契約を検証する（Issue #216 / Req 1.1, 1.2, 1.4,
// 1.5, 1.7, 2.1, 2.2, 2.5, 2.6, 3.1, 3.2, 3.5, 3.6, 3.7, NFR 1.3）。
//
// 全依存（PasskeyRegistrationService / PasskeyAuthenticationService）を stub 化し、
// 外部ネットワーク非依存で正常系・異常系・境界値を検証する。エラーマッピングは
// tasks.md L173-177 に厳密準拠する（passkey.ErrXxx → APIError code）。

// ------------------------------------------------------------
// stubs
// ------------------------------------------------------------

type stubPasskeyRegistrationService struct {
	beginNewFn  func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error)
	finishNewFn func(ctx context.Context, challengeID string, requestBody []byte) (string, error)
	beginAddFn  func(ctx context.Context, authenticatedUserID string) (string, []byte, error)
	finishAddFn func(ctx context.Context, authenticatedUserID, challengeID string, requestBody []byte) error

	beginNewCalled  int
	finishNewCalled int
	beginAddCalled  int
	finishAddCalled int

	// 記録用: 最後に受け取った引数
	lastUsername      string
	lastEmail         string
	lastCodeChallenge string
	lastChallengeID   string
	lastRequestBody   []byte
	lastAuthenticated string
}

func (s *stubPasskeyRegistrationService) BeginRegistrationNew(ctx context.Context,
	rawUsername, optionalEmail, codeChallenge string,
) (string, []byte, error) {
	s.beginNewCalled++
	s.lastUsername = rawUsername
	s.lastEmail = optionalEmail
	s.lastCodeChallenge = codeChallenge
	if s.beginNewFn != nil {
		return s.beginNewFn(ctx, rawUsername, optionalEmail, codeChallenge)
	}
	return "", nil, errors.New("beginNewFn not configured")
}

func (s *stubPasskeyRegistrationService) FinishRegistrationNew(ctx context.Context,
	challengeID string, requestBody []byte,
) (string, error) {
	s.finishNewCalled++
	s.lastChallengeID = challengeID
	s.lastRequestBody = append([]byte(nil), requestBody...)
	if s.finishNewFn != nil {
		return s.finishNewFn(ctx, challengeID, requestBody)
	}
	return "", errors.New("finishNewFn not configured")
}

func (s *stubPasskeyRegistrationService) BeginAddCredential(ctx context.Context,
	authenticatedUserID string,
) (string, []byte, error) {
	s.beginAddCalled++
	s.lastAuthenticated = authenticatedUserID
	if s.beginAddFn != nil {
		return s.beginAddFn(ctx, authenticatedUserID)
	}
	return "", nil, errors.New("beginAddFn not configured")
}

func (s *stubPasskeyRegistrationService) FinishAddCredential(ctx context.Context,
	authenticatedUserID, challengeID string, requestBody []byte,
) error {
	s.finishAddCalled++
	s.lastAuthenticated = authenticatedUserID
	s.lastChallengeID = challengeID
	s.lastRequestBody = append([]byte(nil), requestBody...)
	if s.finishAddFn != nil {
		return s.finishAddFn(ctx, authenticatedUserID, challengeID, requestBody)
	}
	return errors.New("finishAddFn not configured")
}

type stubPasskeyAuthenticationService struct {
	beginFn  func(ctx context.Context, codeChallenge string) (string, []byte, error)
	finishFn func(ctx context.Context, requestBody []byte, challengeID string) (string, error)

	beginCalled  int
	finishCalled int

	lastCodeChallenge string
	lastChallengeID   string
	lastRequestBody   []byte
}

func (s *stubPasskeyAuthenticationService) BeginAuthentication(ctx context.Context,
	codeChallenge string,
) (string, []byte, error) {
	s.beginCalled++
	s.lastCodeChallenge = codeChallenge
	if s.beginFn != nil {
		return s.beginFn(ctx, codeChallenge)
	}
	return "", nil, errors.New("beginFn not configured")
}

func (s *stubPasskeyAuthenticationService) FinishAuthentication(ctx context.Context,
	requestBody []byte, challengeID string,
) (string, error) {
	s.finishCalled++
	s.lastChallengeID = challengeID
	s.lastRequestBody = append([]byte(nil), requestBody...)
	if s.finishFn != nil {
		return s.finishFn(ctx, requestBody, challengeID)
	}
	return "", errors.New("finishFn not configured")
}

// newPasskeyReq は JSON POST リクエストを生成する共通ヘルパ。
func newPasskeyReq(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// decodeErrorResponseBody は 4xx / 5xx JSON error response を decode するヘルパ。
func decodeErrorResponseBody(t *testing.T, w *httptest.ResponseRecorder) middleware.ErrorResponseBody {
	t.Helper()
	var body middleware.ErrorResponseBody
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return body
}

// ------------------------------------------------------------
// RegistrationBegin
// ------------------------------------------------------------

// Req 1.1: 成功時 200 + challenge_id + options JSON を返す。
func TestPasskeyHandler_RegistrationBegin_Success(t *testing.T) {
	// Arrange
	reg := &stubPasskeyRegistrationService{
		beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
			return "chal-123", []byte(`{"challenge":"abc","rp":{"id":"example.com"}}`), nil
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	body := `{"username":"alice","email":"alice@example.com","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`
	req := newPasskeyReq("/api/passkey/registration/begin", body)
	w := httptest.NewRecorder()

	// Act
	h.RegistrationBegin(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	var resp passkeyBeginResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ChallengeID != "chal-123" {
		t.Errorf("challenge_id = %q, want chal-123", resp.ChallengeID)
	}
	if !strings.Contains(string(resp.Options), `"challenge":"abc"`) {
		t.Errorf("options = %s, want to contain challenge (options should pass through)", resp.Options)
	}
	if reg.beginNewCalled != 1 {
		t.Errorf("service called %d times, want 1", reg.beginNewCalled)
	}
	if reg.lastUsername != "alice" || reg.lastEmail != "alice@example.com" || reg.lastCodeChallenge == "" {
		t.Errorf("service received (%q, %q, %q), want (alice, alice@example.com, non-empty)",
			reg.lastUsername, reg.lastEmail, reg.lastCodeChallenge)
	}
}

// Req 1.5 経由: ErrInvalidUsername → 400 INVALID_USERNAME マッピング。
func TestPasskeyHandler_RegistrationBegin_InvalidUsername(t *testing.T) {
	// Arrange
	reg := &stubPasskeyRegistrationService{
		beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
			return "", nil, passkey.ErrInvalidUsername
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"username":"al","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	// Act
	h.RegistrationBegin(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INVALID_USERNAME" {
		t.Errorf("code = %q, want INVALID_USERNAME", body.Code)
	}
}

// Req 1.4 経由: ErrUsernameTaken → 409 USERNAME_TAKEN マッピング。
func TestPasskeyHandler_RegistrationBegin_UsernameTaken(t *testing.T) {
	// Arrange
	reg := &stubPasskeyRegistrationService{
		beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
			return "", nil, passkey.ErrUsernameTaken
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	// Act
	h.RegistrationBegin(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "USERNAME_TAKEN" {
		t.Errorf("code = %q, want USERNAME_TAKEN", body.Code)
	}
}

// Req 1.7 経由: ErrRegistrationFailed → 400 REGISTRATION_FAILED マッピング。
func TestPasskeyHandler_RegistrationBegin_RegistrationFailed(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
			return "", nil, passkey.ErrRegistrationFailed
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	h.RegistrationBegin(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "REGISTRATION_FAILED" {
		t.Errorf("code = %q, want REGISTRATION_FAILED", body.Code)
	}
}

// infra エラー → 500 INTERNAL_ERROR マッピング（NFR 1.3: 内部詳細を反射しない）。
func TestPasskeyHandler_RegistrationBegin_InfraError(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
			return "", nil, errors.New("db connection lost")
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	h.RegistrationBegin(w, req)

	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want INTERNAL_ERROR", body.Code)
	}
	if strings.Contains(w.Body.String(), "db connection lost") {
		t.Errorf("500 response leaked internal error message: %s", w.Body.String())
	}
}

// JSON 不正 → 400 INVALID_REQUEST。
func TestPasskeyHandler_RegistrationBegin_InvalidJSON(t *testing.T) {
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin", `{"username":`) // truncated
	w := httptest.NewRecorder()

	h.RegistrationBegin(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", body.Code)
	}
	if reg.beginNewCalled != 0 {
		t.Errorf("service should not be called on JSON error, called %d", reg.beginNewCalled)
	}
}

// 必須フィールド欠落（username 空）→ 400 INVALID_REQUEST。
func TestPasskeyHandler_RegistrationBegin_MissingUsername(t *testing.T) {
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	h.RegistrationBegin(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST (missing username)", body.Code)
	}
	if reg.beginNewCalled != 0 {
		t.Errorf("service should not be called on missing field, called %d", reg.beginNewCalled)
	}
}

// unknown フィールド → 400 INVALID_REQUEST（DisallowUnknownFields）。
func TestPasskeyHandler_RegistrationBegin_UnknownField(t *testing.T) {
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM","extra":"x"}`)
	w := httptest.NewRecorder()

	h.RegistrationBegin(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", body.Code)
	}
}

// Req 1.6: email 未指定でも service に空文字が渡されて 200 を返す。
func TestPasskeyHandler_RegistrationBegin_EmailOptional(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
			if optionalEmail != "" {
				t.Errorf("email = %q, want empty (Req 1.6)", optionalEmail)
			}
			return "chal-x", []byte(`{"opt":true}`), nil
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	// email フィールド無しでも成功する（Req 1.6）
	req := newPasskeyReq("/api/passkey/registration/begin",
		`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	h.RegistrationBegin(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (email optional / Req 1.6)", w.Result().StatusCode)
	}
}

// ------------------------------------------------------------
// RegistrationFinish
// ------------------------------------------------------------

// Req 1.2 / 1.3: 成功時 200 + user_id を返す。
func TestPasskeyHandler_RegistrationFinish_Success(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		finishNewFn: func(ctx context.Context, challengeID string, requestBody []byte) (string, error) {
			if challengeID != "chal-1" {
				t.Errorf("challengeID = %q, want chal-1", challengeID)
			}
			if !bytes.Contains(requestBody, []byte(`"raw":true`)) {
				t.Errorf("requestBody did not pass through as JSON RawMessage: %s", requestBody)
			}
			return "user-abc", nil
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	body := `{"challenge_id":"chal-1","credential":{"raw":true}}`
	req := newPasskeyReq("/api/passkey/registration/finish", body)
	w := httptest.NewRecorder()

	h.RegistrationFinish(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	var resp registrationFinishNewResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.UserID != "user-abc" {
		t.Errorf("user_id = %q, want user-abc", resp.UserID)
	}
}

// Req 1.7: ErrRegistrationFailed → 400 REGISTRATION_FAILED。
func TestPasskeyHandler_RegistrationFinish_Rejected(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		finishNewFn: func(ctx context.Context, challengeID string, requestBody []byte) (string, error) {
			return "", passkey.ErrRegistrationFailed
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/finish",
		`{"challenge_id":"chal-1","credential":{"raw":true}}`)
	w := httptest.NewRecorder()

	h.RegistrationFinish(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "REGISTRATION_FAILED" {
		t.Errorf("code = %q, want REGISTRATION_FAILED", body.Code)
	}
}

// challenge_id 空 → 400 INVALID_REQUEST（missing field 判定）。
func TestPasskeyHandler_RegistrationFinish_MissingChallengeID(t *testing.T) {
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/finish",
		`{"credential":{"raw":true}}`)
	w := httptest.NewRecorder()

	h.RegistrationFinish(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", body.Code)
	}
	if reg.finishNewCalled != 0 {
		t.Errorf("service should not be called, called %d", reg.finishNewCalled)
	}
}

// ------------------------------------------------------------
// RegistrationAddBegin / Finish（認証済み）
// ------------------------------------------------------------

// Req 3.1: 成功時 200 + challenge_id + options（context userID が service に渡される）。
func TestPasskeyHandler_RegistrationAddBegin_Success(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		beginAddFn: func(ctx context.Context, authenticatedUserID string) (string, []byte, error) {
			if authenticatedUserID != "user-authn-1" {
				t.Errorf("authenticatedUserID = %q, want user-authn-1", authenticatedUserID)
			}
			return "chal-add", []byte(`{"add":true}`), nil
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/add/begin", `{}`)
	// context に userID を注入（middleware 通過後の状態を模擬）
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-authn-1"))
	w := httptest.NewRecorder()

	h.RegistrationAddBegin(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	var resp passkeyBeginResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ChallengeID != "chal-add" {
		t.Errorf("challenge_id = %q, want chal-add", resp.ChallengeID)
	}
}

// Req 3.5: 未認証呼び出し（context に userID 無し）→ 401。
func TestPasskeyHandler_RegistrationAddBegin_Unauthenticated(t *testing.T) {
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/add/begin", `{}`)
	// context に userID を注入しない
	w := httptest.NewRecorder()

	h.RegistrationAddBegin(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Result().StatusCode)
	}
	if reg.beginAddCalled != 0 {
		t.Errorf("service should not be called, called %d", reg.beginAddCalled)
	}
}

// 空 body（Content-Length 0）でも 200 を返す（design.md L709: {} または body 無し許容）。
func TestPasskeyHandler_RegistrationAddBegin_EmptyBodyAccepted(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		beginAddFn: func(ctx context.Context, authenticatedUserID string) (string, []byte, error) {
			return "chal-add-2", []byte(`{}`), nil
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	// body 全く無し
	req := httptest.NewRequest(http.MethodPost, "/api/passkey/registration/add/begin", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-authn-1"))
	w := httptest.NewRecorder()

	h.RegistrationAddBegin(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty body allowed for add/begin)", w.Result().StatusCode)
	}
}

// Req 3.2: FinishAddCredential 成功時 204 を返す。
func TestPasskeyHandler_RegistrationAddFinish_Success(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		finishAddFn: func(ctx context.Context, authenticatedUserID, challengeID string, requestBody []byte) error {
			if authenticatedUserID != "user-authn-1" || challengeID != "chal-add" {
				t.Errorf("finishAddFn args = (%q, %q), want (user-authn-1, chal-add)",
					authenticatedUserID, challengeID)
			}
			return nil
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/add/finish",
		`{"challenge_id":"chal-add","credential":{"raw":true}}`)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-authn-1"))
	w := httptest.NewRecorder()

	h.RegistrationAddFinish(w, req)

	if w.Result().StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Result().StatusCode)
	}
	if reg.finishAddCalled != 1 {
		t.Errorf("service called %d times, want 1", reg.finishAddCalled)
	}
}

// Req 3.6 / 3.7: ErrRegistrationFailed → 400 REGISTRATION_FAILED。
func TestPasskeyHandler_RegistrationAddFinish_Rejected(t *testing.T) {
	reg := &stubPasskeyRegistrationService{
		finishAddFn: func(ctx context.Context, authenticatedUserID, challengeID string, requestBody []byte) error {
			return passkey.ErrRegistrationFailed
		},
	}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/add/finish",
		`{"challenge_id":"chal-add","credential":{"raw":true}}`)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-authn-1"))
	w := httptest.NewRecorder()

	h.RegistrationAddFinish(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "REGISTRATION_FAILED" {
		t.Errorf("code = %q, want REGISTRATION_FAILED", body.Code)
	}
}

// Req 3.5: 未認証呼び出し → 401（handler 単体でも context 無しで 401）。
func TestPasskeyHandler_RegistrationAddFinish_Unauthenticated(t *testing.T) {
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	req := newPasskeyReq("/api/passkey/registration/add/finish",
		`{"challenge_id":"chal-add","credential":{"raw":true}}`)
	// context に userID を注入しない
	w := httptest.NewRecorder()

	h.RegistrationAddFinish(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Result().StatusCode)
	}
	if reg.finishAddCalled != 0 {
		t.Errorf("service should not be called when unauthenticated, called %d", reg.finishAddCalled)
	}
}

// ------------------------------------------------------------
// AuthenticationBegin / Finish
// ------------------------------------------------------------

// Req 2.1: 成功時 200 + challenge_id + options。
func TestPasskeyHandler_AuthenticationBegin_Success(t *testing.T) {
	authn := &stubPasskeyAuthenticationService{
		beginFn: func(ctx context.Context, codeChallenge string) (string, []byte, error) {
			if codeChallenge == "" {
				t.Errorf("codeChallenge is empty")
			}
			return "chal-auth", []byte(`{"authn":true}`), nil
		},
	}
	h := NewPasskeyHandler(&stubPasskeyRegistrationService{}, authn)
	req := newPasskeyReq("/api/passkey/authentication/begin",
		`{"code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
	w := httptest.NewRecorder()

	h.AuthenticationBegin(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	var resp passkeyBeginResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ChallengeID != "chal-auth" {
		t.Errorf("challenge_id = %q, want chal-auth", resp.ChallengeID)
	}
}

// code_challenge 欠落 → 400 INVALID_REQUEST。
func TestPasskeyHandler_AuthenticationBegin_MissingCodeChallenge(t *testing.T) {
	authn := &stubPasskeyAuthenticationService{}
	h := NewPasskeyHandler(&stubPasskeyRegistrationService{}, authn)
	req := newPasskeyReq("/api/passkey/authentication/begin", `{}`)
	w := httptest.NewRecorder()

	h.AuthenticationBegin(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", body.Code)
	}
	if authn.beginCalled != 0 {
		t.Errorf("service should not be called on missing field, called %d", authn.beginCalled)
	}
}

// Req 2.5 経由: ErrAuthenticationFailed → 400 AUTHENTICATION_FAILED。
func TestPasskeyHandler_AuthenticationBegin_Rejected(t *testing.T) {
	authn := &stubPasskeyAuthenticationService{
		beginFn: func(ctx context.Context, codeChallenge string) (string, []byte, error) {
			return "", nil, passkey.ErrAuthenticationFailed
		},
	}
	h := NewPasskeyHandler(&stubPasskeyRegistrationService{}, authn)
	req := newPasskeyReq("/api/passkey/authentication/begin",
		`{"code_challenge":"invalid"}`)
	w := httptest.NewRecorder()

	h.AuthenticationBegin(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
	body := decodeErrorResponseBody(t, w)
	if body.Code != "AUTHENTICATION_FAILED" {
		t.Errorf("code = %q, want AUTHENTICATION_FAILED", body.Code)
	}
}

// Req 2.2: 成功時 200 + auth_code を返す（service が返した平文をそのまま反映）。
func TestPasskeyHandler_AuthenticationFinish_Success(t *testing.T) {
	authn := &stubPasskeyAuthenticationService{
		finishFn: func(ctx context.Context, requestBody []byte, challengeID string) (string, error) {
			if challengeID != "chal-auth" {
				t.Errorf("challengeID = %q, want chal-auth", challengeID)
			}
			return "plain-auth-code-abc", nil
		},
	}
	h := NewPasskeyHandler(&stubPasskeyRegistrationService{}, authn)
	req := newPasskeyReq("/api/passkey/authentication/finish",
		`{"challenge_id":"chal-auth","credential":{"raw":true}}`)
	w := httptest.NewRecorder()

	h.AuthenticationFinish(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	var resp authenticationFinishResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AuthCode != "plain-auth-code-abc" {
		t.Errorf("auth_code = %q, want plain-auth-code-abc", resp.AuthCode)
	}
}

// Req 2.5 / 2.6 / NFR 1.4: ErrAuthenticationFailed → 400 AUTHENTICATION_FAILED。
// counter 後退 / user 未解決 / challenge 期限切れの区別を応答から判別できないこと。
func TestPasskeyHandler_AuthenticationFinish_UniformRejection(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"assertion_invalid", passkey.ErrAuthenticationFailed},
		{"counter_regression", passkey.ErrAuthenticationFailed},
		{"challenge_expired", passkey.ErrAuthenticationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var capturedBodies []string
			for i := 0; i < 3; i++ {
				authn := &stubPasskeyAuthenticationService{
					finishFn: func(ctx context.Context, requestBody []byte, challengeID string) (string, error) {
						return "", tc.err
					},
				}
				h := NewPasskeyHandler(&stubPasskeyRegistrationService{}, authn)
				req := newPasskeyReq("/api/passkey/authentication/finish",
					`{"challenge_id":"chal-auth","credential":{"raw":true}}`)
				w := httptest.NewRecorder()

				// Act
				h.AuthenticationFinish(w, req)

				if w.Result().StatusCode != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", w.Result().StatusCode)
				}
				capturedBodies = append(capturedBodies, w.Body.String())
			}
			// Assert: いずれの拒否理由でも同一の応答本文（Req 2.6 の存在有無非開示）。
			for i := 1; i < len(capturedBodies); i++ {
				if capturedBodies[i] != capturedBodies[0] {
					t.Errorf("uniform rejection violated: body[%d] differs from body[0]", i)
				}
			}
		})
	}
}

// AuthenticationFinish で infra エラー → 500 INTERNAL_ERROR。
func TestPasskeyHandler_AuthenticationFinish_InfraError(t *testing.T) {
	authn := &stubPasskeyAuthenticationService{
		finishFn: func(ctx context.Context, requestBody []byte, challengeID string) (string, error) {
			return "", errors.New("some db failure with credential=raw-bytes-secret")
		},
	}
	h := NewPasskeyHandler(&stubPasskeyRegistrationService{}, authn)
	req := newPasskeyReq("/api/passkey/authentication/finish",
		`{"challenge_id":"chal-auth","credential":{"raw":true}}`)
	w := httptest.NewRecorder()

	h.AuthenticationFinish(w, req)

	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Result().StatusCode)
	}
	// NFR 1.3: 内部エラーメッセージ（secret 部分）を反射しない。
	if strings.Contains(w.Body.String(), "raw-bytes-secret") {
		t.Errorf("500 response leaked infra error internals: %s", w.Body.String())
	}
}

// ------------------------------------------------------------
// Body 上限超過（MaxBodyBytes middleware 経由）
// ------------------------------------------------------------

// MaxBodyBytesMiddleware で wrap した状態でボディ上限を超過すると、handler の
// json.Decode が MaxBytesError を返し 400 INVALID_REQUEST に合流することを検証する
// （既存 native_auth_handler と同流儀）。
func TestPasskeyHandler_RegistrationFinish_BodyLimitExceeded(t *testing.T) {
	// Arrange: 極小の limit（1 byte）で wrap した handler chain を作る
	reg := &stubPasskeyRegistrationService{}
	h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
	mw := middleware.NewMaxBodyBytesMiddleware(1)
	chained := mw(http.HandlerFunc(h.RegistrationFinish))

	body := `{"challenge_id":"chal-1","credential":{"raw":true}}`
	req := newPasskeyReq("/api/passkey/registration/finish", body)
	w := httptest.NewRecorder()

	// Act
	chained.ServeHTTP(w, req)

	// Assert
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body limit exceeded)", w.Result().StatusCode)
	}
	respBody := decodeErrorResponseBody(t, w)
	if respBody.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", respBody.Code)
	}
	if reg.finishNewCalled != 0 {
		t.Errorf("service should not be called on body limit exceed, called %d", reg.finishNewCalled)
	}
}

// エラーマッピングの網羅を宣言的に確認する（tasks.md L173-177 に厳密準拠）。
func TestPasskeyHandler_RegistrationErrorMapping_Table(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid_username", passkey.ErrInvalidUsername, http.StatusBadRequest, "INVALID_USERNAME"},
		{"username_taken", passkey.ErrUsernameTaken, http.StatusConflict, "USERNAME_TAKEN"},
		{"registration_failed", passkey.ErrRegistrationFailed, http.StatusBadRequest, "REGISTRATION_FAILED"},
		{"infra", fmt.Errorf("wrapped: %w", errors.New("internal")), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &stubPasskeyRegistrationService{
				beginNewFn: func(ctx context.Context, rawUsername, optionalEmail, codeChallenge string) (string, []byte, error) {
					return "", nil, tc.err
				},
			}
			h := NewPasskeyHandler(reg, &stubPasskeyAuthenticationService{})
			req := newPasskeyReq("/api/passkey/registration/begin",
				`{"username":"alice","code_challenge":"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}`)
			w := httptest.NewRecorder()

			h.RegistrationBegin(w, req)

			if got := w.Result().StatusCode; got != tc.wantStatus {
				t.Errorf("status = %d, want %d", got, tc.wantStatus)
			}
			body := decodeErrorResponseBody(t, w)
			if body.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tc.wantCode)
			}
		})
	}
}
