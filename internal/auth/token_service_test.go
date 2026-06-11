package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- 最小 IF のテスト用 mock ---

// mockAuthCodes は AuthCodeConsumer 最小 IF の record-and-return モック。
type mockAuthCodes struct {
	findFn     func(ctx context.Context, hash string) (*model.AuthCode, error)
	markUsedFn func(ctx context.Context, id string) error

	findCalls     int
	markUsedCalls int
	lastFindHash  string
	lastMarkID    string
}

func (m *mockAuthCodes) FindByHash(ctx context.Context, hash string) (*model.AuthCode, error) {
	m.findCalls++
	m.lastFindHash = hash
	if m.findFn != nil {
		return m.findFn(ctx, hash)
	}
	return nil, nil
}

func (m *mockAuthCodes) MarkUsed(ctx context.Context, id string) error {
	m.markUsedCalls++
	m.lastMarkID = id
	if m.markUsedFn != nil {
		return m.markUsedFn(ctx, id)
	}
	return nil
}

// mockRefreshTokens は RefreshTokenStore 最小 IF の record-and-return モック。
// 永続化された family / token を捕捉して、テストで TokenHash / FamilyID / ExpiresAt
// の整合を検証する。
type mockRefreshTokens struct {
	createFamilyFn func(ctx context.Context, f *model.RefreshTokenFamily) error
	createTokenFn  func(ctx context.Context, t *model.RefreshToken) error

	createFamilyCalls int
	createTokenCalls  int
	storedFamily      *model.RefreshTokenFamily
	storedToken       *model.RefreshToken
}

func (m *mockRefreshTokens) CreateFamily(ctx context.Context, f *model.RefreshTokenFamily) error {
	m.createFamilyCalls++
	m.storedFamily = f
	if m.createFamilyFn != nil {
		return m.createFamilyFn(ctx, f)
	}
	return nil
}

func (m *mockRefreshTokens) CreateToken(ctx context.Context, t *model.RefreshToken) error {
	m.createTokenCalls++
	m.storedToken = t
	if m.createTokenFn != nil {
		return m.createTokenFn(ctx, t)
	}
	return nil
}

// mockIssuer は AccessTokenIssuer 最小 IF の固定値モック。
type mockIssuer struct {
	tokenFn      func(userID string) (string, error)
	issueCalls   int
	lastUserID   string
	defaultToken string
}

func (m *mockIssuer) IssueAccessToken(userID string) (string, error) {
	m.issueCalls++
	m.lastUserID = userID
	if m.tokenFn != nil {
		return m.tokenFn(userID)
	}
	return m.defaultToken, nil
}

// --- テスト用ヘルパー ---

const (
	// testVerifier は RFC 7636 Appendix B の test vector を使用。
	// 派生 challenge は validS256Challenge と一致する。
	testVerifier = rfc7636AppendixBVerifier
)

// fixedTokenServiceIssuedAt は ExchangeAuthCode 内で now を固定する基準時刻（UTC）。
var fixedTokenServiceIssuedAt = time.Date(2026, 6, 12, 15, 30, 0, 0, time.UTC)

// newAuthCode は FindByHash の戻り値として使う AuthCode の fixture を返す。
func newAuthCode(userID string) *model.AuthCode {
	return &model.AuthCode{
		ID:            "auth-code-id-1",
		CodeHash:      HashNativeSecret("plain-auth-code"),
		UserID:        userID,
		PKCEChallenge: validS256Challenge,
		ExpiresAt:     fixedTokenServiceIssuedAt.Add(30 * time.Second),
		Used:          false,
	}
}

// newServiceWithMocks は固定 now の TokenService と 3 つの mock 参照を返す。
func newServiceWithMocks(t *testing.T) (*TokenService, *mockAuthCodes, *mockRefreshTokens, *mockIssuer) {
	t.Helper()
	authCodes := &mockAuthCodes{}
	refreshTokens := &mockRefreshTokens{}
	issuer := &mockIssuer{defaultToken: "test-access-token"}
	svc := NewTokenService(authCodes, refreshTokens, issuer)
	svc.now = func() time.Time { return fixedTokenServiceIssuedAt }
	return svc, authCodes, refreshTokens, issuer
}

// --- 正常系（Testing Strategy 3-A） ---

// TestExchangeAuthCode_Success は交換成功時に以下を検証する:
//   - 戻り値の AccessToken / RefreshToken が非空、ExpiresIn = 900
//   - MarkUsed が当該 ID で 1 回呼ばれる
//   - 保存された RefreshToken の TokenHash / ExpiresAt / FamilyID が整合する
//   - 平文 RefreshToken はレスポンスのみに含まれ、TokenHash は SHA-256 hex
func TestExchangeAuthCode_Success(t *testing.T) {
	// Arrange
	const userID = "user-test-1"
	svc, authCodes, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newAuthCode(userID)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}

	// Act
	pair, err := svc.ExchangeAuthCode(context.Background(), "plain-auth-code", testVerifier)

	// Assert: 成功で 3 値が揃う
	if err != nil {
		t.Fatalf("ExchangeAuthCode returned error: %v", err)
	}
	if pair == nil {
		t.Fatal("returned TokenPair is nil")
	}
	if pair.AccessToken == "" {
		t.Error("AccessToken is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken is empty")
	}
	if pair.ExpiresIn != 900 {
		t.Errorf("ExpiresIn = %d, want 900 (Req 1.4 / SERVER.md §1.4)", pair.ExpiresIn)
	}

	// Assert: MarkUsed が stored.ID で 1 回呼ばれる（Req 1.2 / 2.3）
	if authCodes.markUsedCalls != 1 {
		t.Errorf("MarkUsed call count = %d, want 1", authCodes.markUsedCalls)
	}
	if authCodes.lastMarkID != stored.ID {
		t.Errorf("MarkUsed called with id=%q, want %q", authCodes.lastMarkID, stored.ID)
	}

	// Assert: FindByHash は plain auth_code の hash で 1 回呼ばれる
	wantCodeHash := HashNativeSecret("plain-auth-code")
	if authCodes.lastFindHash != wantCodeHash {
		t.Errorf("FindByHash called with hash=%q, want %q (hash 一致照合)",
			authCodes.lastFindHash, wantCodeHash)
	}

	// Assert: CreateFamily / CreateToken が 1 回ずつ呼ばれる（Req 1.3）
	if refreshTokens.createFamilyCalls != 1 {
		t.Errorf("CreateFamily calls = %d, want 1", refreshTokens.createFamilyCalls)
	}
	if refreshTokens.createTokenCalls != 1 {
		t.Errorf("CreateToken calls = %d, want 1", refreshTokens.createTokenCalls)
	}

	// Assert: 保存された family / token の整合
	if refreshTokens.storedFamily.UserID != userID {
		t.Errorf("stored family.UserID = %q, want %q", refreshTokens.storedFamily.UserID, userID)
	}
	if refreshTokens.storedToken.FamilyID != refreshTokens.storedFamily.ID {
		t.Errorf("stored token.FamilyID = %q, want %q (= family.ID)",
			refreshTokens.storedToken.FamilyID, refreshTokens.storedFamily.ID)
	}
	if refreshTokens.storedToken.UserID != userID {
		t.Errorf("stored token.UserID = %q, want %q", refreshTokens.storedToken.UserID, userID)
	}

	// Assert: TokenHash は SHA-256 hex（64 文字）で、平文の hash に一致する
	if len(refreshTokens.storedToken.TokenHash) != 64 {
		t.Errorf("stored token.TokenHash length = %d, want 64 (SHA-256 hex)",
			len(refreshTokens.storedToken.TokenHash))
	}
	if got := HashNativeSecret(pair.RefreshToken); got != refreshTokens.storedToken.TokenHash {
		t.Errorf("stored token.TokenHash mismatch with HashNativeSecret(plain RefreshToken)")
	}
	// Assert: 平文がそのまま保存されていないこと（NFR 1.2）
	if refreshTokens.storedToken.TokenHash == pair.RefreshToken {
		t.Error("stored token.TokenHash equals plain RefreshToken (must store only hash)")
	}

	// Assert: ExpiresAt が固定 now + 30 日（Req 1.3）
	wantExpiresAt := fixedTokenServiceIssuedAt.Add(RefreshTokenTTL)
	if !refreshTokens.storedToken.ExpiresAt.Equal(wantExpiresAt) {
		t.Errorf("stored token.ExpiresAt = %v, want %v (= now + 30d)",
			refreshTokens.storedToken.ExpiresAt, wantExpiresAt)
	}

	// Assert: issuer が当該 userID で呼ばれる（Req 1.4）
	if issuer.issueCalls != 1 {
		t.Errorf("IssueAccessToken calls = %d, want 1", issuer.issueCalls)
	}
	if issuer.lastUserID != userID {
		t.Errorf("IssueAccessToken called with userID=%q, want %q", issuer.lastUserID, userID)
	}
	if pair.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q (issuer 戻り値)", pair.AccessToken, "test-access-token")
	}
}

// --- 異常系（Testing Strategy 3-B〜E） ---

// TestExchangeAuthCode_AuthCodeNotFound は FindByHash が nil を返したとき
// ErrInvalidGrant を返し、MarkUsed / refresh 永続化に進まないことを検証する（Req 2.2 / 2.6 / 2.7）。
func TestExchangeAuthCode_AuthCodeNotFound(t *testing.T) {
	// Arrange
	svc, authCodes, refreshTokens, issuer := newServiceWithMocks(t)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return nil, nil // 不明
	}

	// Act
	pair, err := svc.ExchangeAuthCode(context.Background(), "unknown-code", testVerifier)

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant (Req 2.2 / 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if authCodes.markUsedCalls != 0 {
		t.Errorf("MarkUsed calls = %d, want 0 (拒否時は単回消費しない)", authCodes.markUsedCalls)
	}
	// Req 2.7: 拒否時に refresh を永続化しない
	if refreshTokens.createFamilyCalls != 0 || refreshTokens.createTokenCalls != 0 {
		t.Errorf("refresh persistence calls (family=%d, token=%d), want 0/0 (Req 2.7)",
			refreshTokens.createFamilyCalls, refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestExchangeAuthCode_VerifierMismatch は verifier が一致しないとき ErrInvalidGrant を
// 返し、MarkUsed / refresh 永続化に進まないことを検証する（Req 2.1 / 2.6 / 2.7）。
func TestExchangeAuthCode_VerifierMismatch(t *testing.T) {
	// Arrange
	svc, authCodes, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newAuthCode("user-test-1")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}

	// 形式は正しいが派生 challenge が一致しない別 verifier（43 文字 unreserved）。
	wrongVerifier := strings.Repeat("a", 43)

	// Act
	pair, err := svc.ExchangeAuthCode(context.Background(), "plain-auth-code", wrongVerifier)

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// Req 2.7 / 設計手順 4 未到達: MarkUsed を呼ぶ前に拒否する
	if authCodes.markUsedCalls != 0 {
		t.Errorf("MarkUsed calls = %d, want 0 (verifier 不一致時は単回消費しない)",
			authCodes.markUsedCalls)
	}
	if refreshTokens.createFamilyCalls != 0 || refreshTokens.createTokenCalls != 0 {
		t.Errorf("refresh persistence calls (family=%d, token=%d), want 0/0 (Req 2.7)",
			refreshTokens.createFamilyCalls, refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestExchangeAuthCode_VerifierMalformed は verifier 形式不正（短すぎる）で
// ErrInvalidGrant を返すことを検証する（Req 2.4 / 2.6）。
func TestExchangeAuthCode_VerifierMalformed(t *testing.T) {
	// Arrange
	svc, authCodes, _, _ := newServiceWithMocks(t)
	stored := newAuthCode("user-test-1")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}

	// Act: 42 文字（最小 43 未満）の verifier
	pair, err := svc.ExchangeAuthCode(context.Background(), "plain-auth-code",
		strings.Repeat("a", 42))

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant (Req 2.4 / 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if authCodes.markUsedCalls != 0 {
		t.Errorf("MarkUsed calls = %d, want 0 (形式不正で拒否)", authCodes.markUsedCalls)
	}
}

// TestExchangeAuthCode_MarkUsedNotUsable は MarkUsed が ErrAuthCodeNotUsable を
// 返したとき（既に使用済み / 期限切れ / 消失）に ErrInvalidGrant を返すことを検証する
// （Req 2.3 / 2.6）。
func TestExchangeAuthCode_MarkUsedNotUsable(t *testing.T) {
	// Arrange
	svc, authCodes, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newAuthCode("user-test-1")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}
	authCodes.markUsedFn = func(ctx context.Context, id string) error {
		return repository.ErrAuthCodeNotUsable
	}

	// Act
	pair, err := svc.ExchangeAuthCode(context.Background(), "plain-auth-code", testVerifier)

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant (Req 2.3 / 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// MarkUsed 自体は呼ばれている（race-safe 判定を repository に委譲しているため）
	if authCodes.markUsedCalls != 1 {
		t.Errorf("MarkUsed calls = %d, want 1", authCodes.markUsedCalls)
	}
	// Req 2.7: MarkUsed 失敗時も refresh を永続化しない
	if refreshTokens.createFamilyCalls != 0 || refreshTokens.createTokenCalls != 0 {
		t.Errorf("refresh persistence calls (family=%d, token=%d), want 0/0 (Req 2.7)",
			refreshTokens.createFamilyCalls, refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestExchangeAuthCode_CreateFamilyFailure は refresh family 永続化が失敗したときの
// エラーが ErrInvalidGrant に正規化されず（インフラ起因の 500 として上層に渡る）、
// かつ CreateToken に進まないことを検証する。
func TestExchangeAuthCode_CreateFamilyFailure(t *testing.T) {
	// Arrange
	svc, authCodes, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newAuthCode("user-test-1")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}
	wantErr := errors.New("db unavailable")
	refreshTokens.createFamilyFn = func(ctx context.Context, f *model.RefreshTokenFamily) error {
		return wantErr
	}

	// Act
	pair, err := svc.ExchangeAuthCode(context.Background(), "plain-auth-code", testVerifier)

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, must not be ErrInvalidGrant (インフラ起因の 500 として上層に渡す)", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// CreateFamily は呼ばれたが、CreateToken には進まない
	if refreshTokens.createFamilyCalls != 1 {
		t.Errorf("CreateFamily calls = %d, want 1", refreshTokens.createFamilyCalls)
	}
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (family 失敗で打ち切り)",
			refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestExchangeAuthCode_DoesNotLeakPlainSecretsInError は ErrInvalidGrant の
// メッセージに平文 auth_code / verifier / refresh token が含まれないことを保証する
// （NFR 1.2 / 1.3 / 1.5）。
func TestExchangeAuthCode_DoesNotLeakPlainSecretsInError(t *testing.T) {
	// Arrange
	svc, authCodes, _, _ := newServiceWithMocks(t)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return nil, nil
	}

	const secretCode = "very-secret-auth-code-value"
	const secretVerifier = "very-secret-verifier-value-12345678901234567"

	// Act
	_, err := svc.ExchangeAuthCode(context.Background(), secretCode, secretVerifier)

	// Assert: ErrInvalidGrant のメッセージに平文が含まれないこと
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("err = %v, want ErrInvalidGrant", err)
	}
	msg := err.Error()
	if strings.Contains(msg, secretCode) {
		t.Errorf("error message %q contains plain auth_code (NFR 1.3)", msg)
	}
	if strings.Contains(msg, secretVerifier) {
		t.Errorf("error message %q contains plain code_verifier (NFR 1.3)", msg)
	}
}

// TestExchangeAuthCode_LookupErrorPropagates は FindByHash が DB エラーを返したとき、
// ErrInvalidGrant に正規化されず（インフラ起因として 500 に渡す）に上層に伝播する
// ことを検証する。
func TestExchangeAuthCode_LookupErrorPropagates(t *testing.T) {
	// Arrange
	svc, authCodes, refreshTokens, issuer := newServiceWithMocks(t)
	wantErr := fmt.Errorf("connection refused")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return nil, wantErr
	}

	// Act
	pair, err := svc.ExchangeAuthCode(context.Background(), "plain-auth-code", testVerifier)

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, must not be ErrInvalidGrant (DB 障害は上層 500)", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if authCodes.markUsedCalls != 0 || refreshTokens.createFamilyCalls != 0 || issuer.issueCalls != 0 {
		t.Errorf("any downstream call must not occur on lookup error")
	}
}
