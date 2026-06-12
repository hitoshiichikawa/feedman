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
// の整合を検証する。FindByHash / MarkRotated は Issue #167 の rotation で、
// RevokeFamily は Issue #168 の再利用検知昇格 / revoke で追加された。
type mockRefreshTokens struct {
	createFamilyFn func(ctx context.Context, f *model.RefreshTokenFamily) error
	createTokenFn  func(ctx context.Context, t *model.RefreshToken) error
	findByHashFn   func(ctx context.Context, tokenHash string) (*model.RefreshToken, error)
	markRotatedFn  func(ctx context.Context, id string, rotatedAt time.Time) error
	revokeFamilyFn func(ctx context.Context, familyID string, revokedAt time.Time) error

	createFamilyCalls  int
	createTokenCalls   int
	findByHashCalls    int
	markRotatedCalls   int
	revokeFamilyCalls  int
	storedFamily       *model.RefreshTokenFamily
	storedToken        *model.RefreshToken
	lastFindHash       string
	lastMarkID         string
	lastMarkAt         time.Time
	lastRevokeFamilyID string
	lastRevokeAt       time.Time
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

func (m *mockRefreshTokens) FindByHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	m.findByHashCalls++
	m.lastFindHash = tokenHash
	if m.findByHashFn != nil {
		return m.findByHashFn(ctx, tokenHash)
	}
	return nil, nil
}

func (m *mockRefreshTokens) MarkRotated(ctx context.Context, id string, rotatedAt time.Time) error {
	m.markRotatedCalls++
	m.lastMarkID = id
	m.lastMarkAt = rotatedAt
	if m.markRotatedFn != nil {
		return m.markRotatedFn(ctx, id, rotatedAt)
	}
	return nil
}

func (m *mockRefreshTokens) RevokeFamily(ctx context.Context, familyID string, revokedAt time.Time) error {
	m.revokeFamilyCalls++
	m.lastRevokeFamilyID = familyID
	m.lastRevokeAt = revokedAt
	if m.revokeFamilyFn != nil {
		return m.revokeFamilyFn(ctx, familyID, revokedAt)
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

// --- RotateRefreshToken のテスト（Issue #167 / design.md Testing Strategy 1〜7） ---

// newStoredRefreshToken は FindByHash の戻り値として使う有効な RefreshToken の fixture を返す。
// ExpiresAt は fixedTokenServiceIssuedAt + 7 日（30 日 TTL の中盤）で「有効」を意味する。
// RotatedAt / RevokedAt は nil。
func newStoredRefreshToken(userID string) *model.RefreshToken {
	return &model.RefreshToken{
		ID:        "refresh-token-id-1",
		FamilyID:  "family-id-1",
		UserID:    userID,
		TokenHash: HashNativeSecret("plain-refresh-token"),
		ExpiresAt: fixedTokenServiceIssuedAt.Add(7 * 24 * time.Hour),
	}
}

// TestRotateRefreshToken_Success は rotation 成功時に以下を検証する（Testing Strategy 1 /
// Req 1.2, 1.3, 1.4）。
//   - TokenPair の 3 値（AccessToken / RefreshToken / ExpiresIn=900）
//   - FindByHash が plain refresh token の hash で 1 回呼ばれる
//   - MarkRotated が旧 token の ID と now で 1 回呼ばれる
//   - CreateToken の FamilyID / UserID が旧 token と同一
//   - 新 token の ExpiresAt = now + 30d（スライディング延長）
//   - 新 token の TokenHash が新平文の hash と一致する
func TestRotateRefreshToken_Success(t *testing.T) {
	// Arrange
	const userID = "user-test-1"
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken(userID)
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert: 成功で 3 値が揃う（Req 1.1 / 1.6 は handler 層のテストで担保）
	if err != nil {
		t.Fatalf("RotateRefreshToken returned error: %v", err)
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
		t.Errorf("ExpiresIn = %d, want 900 (Req 1.4 / AccessTokenTTL=15min)", pair.ExpiresIn)
	}

	// Assert: FindByHash が plain の hash で 1 回呼ばれる
	wantTokenHash := HashNativeSecret("plain-refresh-token")
	if refreshTokens.findByHashCalls != 1 {
		t.Errorf("FindByHash calls = %d, want 1", refreshTokens.findByHashCalls)
	}
	if refreshTokens.lastFindHash != wantTokenHash {
		t.Errorf("FindByHash called with hash=%q, want %q (hash 一致照合)",
			refreshTokens.lastFindHash, wantTokenHash)
	}

	// Assert: MarkRotated が旧 token の ID と固定 now で 1 回呼ばれる（Req 1.2）
	if refreshTokens.markRotatedCalls != 1 {
		t.Errorf("MarkRotated calls = %d, want 1", refreshTokens.markRotatedCalls)
	}
	if refreshTokens.lastMarkID != stored.ID {
		t.Errorf("MarkRotated called with id=%q, want %q", refreshTokens.lastMarkID, stored.ID)
	}
	if !refreshTokens.lastMarkAt.Equal(fixedTokenServiceIssuedAt) {
		t.Errorf("MarkRotated rotatedAt = %v, want %v", refreshTokens.lastMarkAt, fixedTokenServiceIssuedAt)
	}

	// Assert: CreateFamily は呼ばれない（同一 family に new token を発行する / Req 1.3）
	if refreshTokens.createFamilyCalls != 0 {
		t.Errorf("CreateFamily calls = %d, want 0 (rotation は同一 family / Req 1.3)",
			refreshTokens.createFamilyCalls)
	}

	// Assert: CreateToken が 1 回呼ばれ、FamilyID / UserID が旧 token と同一（Req 1.3）
	if refreshTokens.createTokenCalls != 1 {
		t.Errorf("CreateToken calls = %d, want 1", refreshTokens.createTokenCalls)
	}
	if refreshTokens.storedToken.FamilyID != stored.FamilyID {
		t.Errorf("new token.FamilyID = %q, want %q (= old token.FamilyID / Req 1.3)",
			refreshTokens.storedToken.FamilyID, stored.FamilyID)
	}
	if refreshTokens.storedToken.UserID != userID {
		t.Errorf("new token.UserID = %q, want %q", refreshTokens.storedToken.UserID, userID)
	}

	// Assert: 新 token の TokenHash が新平文の hash と一致する（NFR 1.2）
	if got := HashNativeSecret(pair.RefreshToken); got != refreshTokens.storedToken.TokenHash {
		t.Errorf("new token.TokenHash mismatch with HashNativeSecret(plain RefreshToken)")
	}
	// Assert: 平文がそのまま保存されていないこと（NFR 1.2）
	if refreshTokens.storedToken.TokenHash == pair.RefreshToken {
		t.Error("new token.TokenHash equals plain RefreshToken (must store only hash)")
	}
	// Assert: 旧 token とは異なる token が生成されている（rotation の本質）
	if pair.RefreshToken == "plain-refresh-token" {
		t.Error("new RefreshToken equals old plain (rotation must produce a new token)")
	}
	if refreshTokens.storedToken.TokenHash == stored.TokenHash {
		t.Error("new token.TokenHash equals old token.TokenHash (rotation must change hash)")
	}

	// Assert: 新 token の ExpiresAt = now + 30d（スライディング延長 / Req 1.4）
	wantExpiresAt := fixedTokenServiceIssuedAt.Add(RefreshTokenTTL)
	if !refreshTokens.storedToken.ExpiresAt.Equal(wantExpiresAt) {
		t.Errorf("new token.ExpiresAt = %v, want %v (= now + 30d / Req 1.4)",
			refreshTokens.storedToken.ExpiresAt, wantExpiresAt)
	}

	// Assert: 新 token の RotatedAt / RevokedAt は nil（新世代として有効）
	if refreshTokens.storedToken.RotatedAt != nil {
		t.Errorf("new token.RotatedAt = %v, want nil (新世代は未 rotate)", refreshTokens.storedToken.RotatedAt)
	}
	if refreshTokens.storedToken.RevokedAt != nil {
		t.Errorf("new token.RevokedAt = %v, want nil (新世代は未 revoke)", refreshTokens.storedToken.RevokedAt)
	}

	// Assert: issuer が当該 userID で呼ばれる
	if issuer.issueCalls != 1 {
		t.Errorf("IssueAccessToken calls = %d, want 1", issuer.issueCalls)
	}
	if issuer.lastUserID != userID {
		t.Errorf("IssueAccessToken called with userID=%q, want %q", issuer.lastUserID, userID)
	}
}

// TestRotateRefreshToken_NotFound は FindByHash が nil を返したとき
// ErrInvalidRefreshToken を返し、MarkRotated / CreateToken に進まないことを検証する
// （Testing Strategy 2 / Req 2.1, 2.6, 2.7）。
func TestRotateRefreshToken_NotFound(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return nil, nil
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "unknown-token")

	// Assert
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 2.1 / 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if refreshTokens.markRotatedCalls != 0 {
		t.Errorf("MarkRotated calls = %d, want 0 (token 不明は MarkRotated 未到達)",
			refreshTokens.markRotatedCalls)
	}
	// Req 2.7: 拒否時に新 token を永続化しない
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (Req 2.7)", refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_Expired は token の ExpiresAt が now 以下のとき
// ErrInvalidRefreshToken を返し、MarkRotated に進まないことを検証する
// （Testing Strategy 3 / Req 2.2, 2.6, 2.7）。
func TestRotateRefreshToken_Expired(t *testing.T) {
	cases := []struct {
		name      string
		expiresAt time.Time
	}{
		{
			name:      "ExpiresAt < now のとき拒否",
			expiresAt: fixedTokenServiceIssuedAt.Add(-1 * time.Hour),
		},
		{
			name:      "ExpiresAt == now のとき拒否（境界値: After(now) でなければ無効）",
			expiresAt: fixedTokenServiceIssuedAt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc, _, refreshTokens, issuer := newServiceWithMocks(t)
			stored := newStoredRefreshToken("user-test-1")
			stored.ExpiresAt = tc.expiresAt
			refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
				return stored, nil
			}

			// Act
			pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

			// Assert
			if !errors.Is(err, ErrInvalidRefreshToken) {
				t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 2.2 / 2.6)", err)
			}
			if pair != nil {
				t.Errorf("pair = %+v, want nil", pair)
			}
			if refreshTokens.markRotatedCalls != 0 {
				t.Errorf("MarkRotated calls = %d, want 0 (期限切れは MarkRotated 未到達)",
					refreshTokens.markRotatedCalls)
			}
			if refreshTokens.createTokenCalls != 0 {
				t.Errorf("CreateToken calls = %d, want 0 (Req 2.7)", refreshTokens.createTokenCalls)
			}
			if issuer.issueCalls != 0 {
				t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
			}
		})
	}
}

// TestRotateRefreshToken_Revoked は RevokedAt が set 済みの token に対して
// ErrInvalidRefreshToken を返すことを検証する（Testing Strategy 4 / Req 2.3, 2.6, 2.7）。
func TestRotateRefreshToken_Revoked(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	revokedAt := fixedTokenServiceIssuedAt.Add(-30 * time.Minute)
	stored.RevokedAt = &revokedAt
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 2.3 / 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if refreshTokens.markRotatedCalls != 0 {
		t.Errorf("MarkRotated calls = %d, want 0 (失効済みは MarkRotated 未到達)",
			refreshTokens.markRotatedCalls)
	}
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (Req 2.7)", refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_AlreadyRotated は RotatedAt が set 済みの token（再利用）に対して
// ErrInvalidRefreshToken を返し、MarkRotated / CreateToken に進まないことを検証する
// （Testing Strategy 5 / Req 2.4, 2.6, 2.7）。
// #168 で family 失効への昇格が追加された（昇格時の RevokeFamily 呼び出しは
// TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke が検証する。本テストの
// 検証内容＝拒否応答と新 token 未作成は #167 から不変）。
func TestRotateRefreshToken_AlreadyRotated(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	rotatedAt := fixedTokenServiceIssuedAt.Add(-1 * time.Hour)
	stored.RotatedAt = &rotatedAt
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 2.4 / 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// MarkRotated は呼ばれない（事前判定で拒否）
	if refreshTokens.markRotatedCalls != 0 {
		t.Errorf("MarkRotated calls = %d, want 0 (再利用は事前判定で拒否)",
			refreshTokens.markRotatedCalls)
	}
	// Req 2.7: 新 token は永続化されない
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (再利用時は新 token 未作成 / Req 2.7)",
			refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_MarkRotatedRaceLoser は並行 rotation race の敗者
// （MarkRotated が ErrRefreshTokenAlreadyRotated を返す）に対して
// ErrInvalidRefreshToken を返し、新 token を作成しないことを検証する
// （Testing Strategy 6 / Req 3.1: 高々 1 件のみ成功）。
//
// 事前判定（RotatedAt nil）を通過した後で、atomic な MarkRotated が race の敗者を
// 検出する正本ゲートの挙動を検証する。
func TestRotateRefreshToken_MarkRotatedRaceLoser(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}
	refreshTokens.markRotatedFn = func(ctx context.Context, id string, rotatedAt time.Time) error {
		// 並行 rotation の敗者として ErrRefreshTokenAlreadyRotated を返す
		return repository.ErrRefreshTokenAlreadyRotated
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 3.1 race 敗者 / Req 2.6)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// MarkRotated は呼ばれる（race の敗者を検出する正本ゲート）
	if refreshTokens.markRotatedCalls != 1 {
		t.Errorf("MarkRotated calls = %d, want 1 (race ゲートに到達する)",
			refreshTokens.markRotatedCalls)
	}
	// Req 3.1 + Req 2.7: 敗者は新 token を作成しない（= 高々 1 件のみ成功）
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (race 敗者は新 token 未作成 / Req 3.1)",
			refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_CreateTokenFailure は新 token 永続化が失敗したとき、
// ErrInvalidRefreshToken に正規化されず（インフラ起因の 500 として上層に渡す）に
// 上層に伝播することを検証する（Testing Strategy 7）。
// 旧 token は MarkRotated 済み（Open Questions: 安全側に倒して燃やす）。
func TestRotateRefreshToken_CreateTokenFailure(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}
	wantErr := errors.New("db unavailable")
	refreshTokens.createTokenFn = func(ctx context.Context, t *model.RefreshToken) error {
		return wantErr
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, must not be ErrInvalidRefreshToken (インフラ起因の 500 として上層に渡す)", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// MarkRotated 済み（旧 token は消費済み / Open Questions）
	if refreshTokens.markRotatedCalls != 1 {
		t.Errorf("MarkRotated calls = %d, want 1 (旧 token は MarkRotated 済み)",
			refreshTokens.markRotatedCalls)
	}
	// CreateToken は呼ばれたが失敗した
	if refreshTokens.createTokenCalls != 1 {
		t.Errorf("CreateToken calls = %d, want 1", refreshTokens.createTokenCalls)
	}
	// access token は発行されない（CreateToken 失敗で打ち切り）
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0 (CreateToken 失敗で打ち切り)",
			issuer.issueCalls)
	}
}

// TestRotateRefreshToken_DoesNotLeakPlainSecretsInError は ErrInvalidRefreshToken の
// メッセージに平文 refresh token が含まれないことを保証する（NFR 1.2 / 1.3）。
func TestRotateRefreshToken_DoesNotLeakPlainSecretsInError(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, _ := newServiceWithMocks(t)
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return nil, nil
	}

	const secretToken = "very-secret-refresh-token-plain-value-12345"

	// Act
	_, err := svc.RotateRefreshToken(context.Background(), secretToken)

	// Assert: ErrInvalidRefreshToken のメッセージに平文が含まれないこと
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
	msg := err.Error()
	if strings.Contains(msg, secretToken) {
		t.Errorf("error message %q contains plain refresh token (NFR 1.2 / 1.3)", msg)
	}
}

// --- 再利用検知の family 失効昇格と RevokeRefreshToken のテスト
// （Issue #168 / design.md Testing Strategy 1〜7） ---

// TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke は RotatedAt 済み token の提示
// （再利用検知）で RevokeFamily が当該 FamilyID と now で呼ばれ、ErrInvalidRefreshToken
// が返り、新 token が作成されないことを検証する（Testing Strategy 1 / Req 1.1, 1.4）。
func TestRotateRefreshToken_ReuseEscalatesToFamilyRevoke(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	rotatedAt := fixedTokenServiceIssuedAt.Add(-1 * time.Hour)
	stored.RotatedAt = &rotatedAt
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert: 拒否応答は通常の無効 token と同一 sentinel（Req 1.4）
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 1.4: 通常拒否と区別しない)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// Assert: RevokeFamily が当該 FamilyID と now で 1 回呼ばれる（Req 1.1）
	if refreshTokens.revokeFamilyCalls != 1 {
		t.Errorf("RevokeFamily calls = %d, want 1 (再利用検知で family 失効昇格 / Req 1.1)",
			refreshTokens.revokeFamilyCalls)
	}
	if refreshTokens.lastRevokeFamilyID != stored.FamilyID {
		t.Errorf("RevokeFamily called with familyID=%q, want %q",
			refreshTokens.lastRevokeFamilyID, stored.FamilyID)
	}
	if !refreshTokens.lastRevokeAt.Equal(fixedTokenServiceIssuedAt) {
		t.Errorf("RevokeFamily revokedAt = %v, want %v",
			refreshTokens.lastRevokeAt, fixedTokenServiceIssuedAt)
	}
	// Assert: 新 token は作成されない
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (再利用検知時は新 token 未作成)",
			refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_RaceLoserEscalatesToFamilyRevoke は並行 rotation race の敗者
// （MarkRotated が ErrRefreshTokenAlreadyRotated を返す = atomic な rotation 確定で
// 先行された提示）でも同様に RevokeFamily へ昇格することを検証する
// （Testing Strategy 2 / Req 1.2, 1.4）。
func TestRotateRefreshToken_RaceLoserEscalatesToFamilyRevoke(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}
	refreshTokens.markRotatedFn = func(ctx context.Context, id string, rotatedAt time.Time) error {
		return repository.ErrRefreshTokenAlreadyRotated
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (Req 1.4)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	// Assert: race 敗者も family 失効に昇格する（Req 1.2: 検知漏れ防止）
	if refreshTokens.revokeFamilyCalls != 1 {
		t.Errorf("RevokeFamily calls = %d, want 1 (race 敗者も昇格 / Req 1.2)",
			refreshTokens.revokeFamilyCalls)
	}
	if refreshTokens.lastRevokeFamilyID != stored.FamilyID {
		t.Errorf("RevokeFamily called with familyID=%q, want %q",
			refreshTokens.lastRevokeFamilyID, stored.FamilyID)
	}
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0", refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_ReuseRevokeFamilyFailureStillRejects は昇格時の RevokeFamily が
// 失敗しても、拒否（ErrInvalidRefreshToken）が維持され新 token が発行されないことを
// 検証する（Testing Strategy 3 / Req 1.5: 安全側に倒す）。
func TestRotateRefreshToken_ReuseRevokeFamilyFailureStillRejects(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	rotatedAt := fixedTokenServiceIssuedAt.Add(-1 * time.Hour)
	stored.RotatedAt = &rotatedAt
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}
	refreshTokens.revokeFamilyFn = func(ctx context.Context, familyID string, revokedAt time.Time) error {
		return errors.New("db unavailable")
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert: RevokeFamily 失敗でも拒否は維持（内部エラーへ昇格させない / Req 1.5）
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken (RevokeFamily 失敗でも拒否を維持 / Req 1.5)", err)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if refreshTokens.revokeFamilyCalls != 1 {
		t.Errorf("RevokeFamily calls = %d, want 1", refreshTokens.revokeFamilyCalls)
	}
	// Req 1.5: 新 token は発行されない
	if refreshTokens.createTokenCalls != 0 {
		t.Errorf("CreateToken calls = %d, want 0 (Req 1.5)", refreshTokens.createTokenCalls)
	}
	if issuer.issueCalls != 0 {
		t.Errorf("IssueAccessToken calls = %d, want 0", issuer.issueCalls)
	}
}

// TestRotateRefreshToken_RevokedIsNotEscalated は RevokedAt set 済み（失効済み）token の
// refresh が引き続き昇格なしの単純拒否であることを検証する（Testing Strategy 7 /
// #167 既存挙動の回帰。失効済み提示は再利用検知ではない）。
func TestRotateRefreshToken_RevokedIsNotEscalated(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, _ := newServiceWithMocks(t)
	stored := newStoredRefreshToken("user-test-1")
	revokedAt := fixedTokenServiceIssuedAt.Add(-30 * time.Minute)
	stored.RevokedAt = &revokedAt
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return stored, nil
	}

	// Act
	_, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, want ErrInvalidRefreshToken", err)
	}
	// 失効済みは再利用ではないため RevokeFamily は呼ばれない（昇格なし）
	if refreshTokens.revokeFamilyCalls != 0 {
		t.Errorf("RevokeFamily calls = %d, want 0 (失効済みは昇格対象外)",
			refreshTokens.revokeFamilyCalls)
	}
}

// TestRevokeRefreshToken_KnownToken は既知 token の提示で RevokeFamily が当該 FamilyID と
// now で呼ばれ、nil が返ることを検証する（Testing Strategy 4 / Req 2.1）。
// token の状態（有効・期限切れ・rotation 済み・失効済み）に関わらず family を失効する
// （design.md「Revoke フロー」: 古い世代の token しか持たないクライアントのログアウトも
// 成立させる）。
func TestRevokeRefreshToken_KnownToken(t *testing.T) {
	pastTime := fixedTokenServiceIssuedAt.Add(-1 * time.Hour)
	cases := []struct {
		name   string
		mutate func(tok *model.RefreshToken)
	}{
		{name: "有効な token のとき family を失効する", mutate: func(tok *model.RefreshToken) {}},
		{name: "期限切れ token でも family を失効する", mutate: func(tok *model.RefreshToken) {
			tok.ExpiresAt = pastTime
		}},
		{name: "rotation 済み token でも family を失効する", mutate: func(tok *model.RefreshToken) {
			tok.RotatedAt = &pastTime
		}},
		{name: "失効済み token でも冪等に成功する", mutate: func(tok *model.RefreshToken) {
			tok.RevokedAt = &pastTime
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc, _, refreshTokens, _ := newServiceWithMocks(t)
			stored := newStoredRefreshToken("user-test-1")
			tc.mutate(stored)
			refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
				return stored, nil
			}

			// Act
			err := svc.RevokeRefreshToken(context.Background(), "plain-refresh-token")

			// Assert
			if err != nil {
				t.Fatalf("RevokeRefreshToken returned error: %v", err)
			}
			if refreshTokens.findByHashCalls != 1 {
				t.Errorf("FindByHash calls = %d, want 1", refreshTokens.findByHashCalls)
			}
			if refreshTokens.lastFindHash != HashNativeSecret("plain-refresh-token") {
				t.Errorf("FindByHash called with hash=%q, want hash of plain token",
					refreshTokens.lastFindHash)
			}
			if refreshTokens.revokeFamilyCalls != 1 {
				t.Errorf("RevokeFamily calls = %d, want 1 (Req 2.1)", refreshTokens.revokeFamilyCalls)
			}
			if refreshTokens.lastRevokeFamilyID != stored.FamilyID {
				t.Errorf("RevokeFamily called with familyID=%q, want %q",
					refreshTokens.lastRevokeFamilyID, stored.FamilyID)
			}
			if !refreshTokens.lastRevokeAt.Equal(fixedTokenServiceIssuedAt) {
				t.Errorf("RevokeFamily revokedAt = %v, want %v",
					refreshTokens.lastRevokeAt, fixedTokenServiceIssuedAt)
			}
		})
	}
}

// TestRevokeRefreshToken_UnknownTokenIsNoop は不明 token の提示で RevokeFamily を
// 呼ばずに nil を返すことを検証する（Testing Strategy 5 / Req 2.2: 冪等・存在オラクル
// なし）。
func TestRevokeRefreshToken_UnknownTokenIsNoop(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, _ := newServiceWithMocks(t)
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return nil, nil
	}

	// Act
	err := svc.RevokeRefreshToken(context.Background(), "unknown-token")

	// Assert
	if err != nil {
		t.Fatalf("RevokeRefreshToken returned error: %v (不明 token は成功扱い / Req 2.2)", err)
	}
	if refreshTokens.revokeFamilyCalls != 0 {
		t.Errorf("RevokeFamily calls = %d, want 0 (不明 token は no-op / Req 2.2)",
			refreshTokens.revokeFamilyCalls)
	}
}

// TestRevokeRefreshToken_InfraErrorPropagates は FindByHash / RevokeFamily の DB 障害が
// error として上層に伝播する（500 系）こと、およびエラーメッセージに平文 token が
// 含まれないことを検証する（Testing Strategy 6 / NFR 1.1, 1.3）。
func TestRevokeRefreshToken_InfraErrorPropagates(t *testing.T) {
	const secretToken = "very-secret-refresh-token-plain-value-67890"
	wantErr := errors.New("db unavailable")

	cases := []struct {
		name  string
		setup func(m *mockRefreshTokens)
	}{
		{
			name: "FindByHash の DB 障害が伝播する",
			setup: func(m *mockRefreshTokens) {
				m.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
					return nil, wantErr
				}
			},
		},
		{
			name: "RevokeFamily の DB 障害が伝播する",
			setup: func(m *mockRefreshTokens) {
				m.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
					return newStoredRefreshToken("user-test-1"), nil
				}
				m.revokeFamilyFn = func(ctx context.Context, familyID string, revokedAt time.Time) error {
					return wantErr
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc, _, refreshTokens, _ := newServiceWithMocks(t)
			tc.setup(refreshTokens)

			// Act
			err := svc.RevokeRefreshToken(context.Background(), secretToken)

			// Assert
			if err == nil {
				t.Fatal("err = nil, want non-nil (DB 障害は 500 系へ)")
			}
			if !errors.Is(err, wantErr) {
				t.Errorf("err = %v, want wrap of %v", err, wantErr)
			}
			if strings.Contains(err.Error(), secretToken) {
				t.Errorf("error message %q contains plain refresh token (NFR 1.1 / 1.3)", err.Error())
			}
		})
	}
}

// TestRotateRefreshToken_LookupErrorPropagates は FindByHash が DB エラーを返したとき、
// ErrInvalidRefreshToken に正規化されず（インフラ起因として 500 に渡す）に上層に伝播する
// ことを検証する。
func TestRotateRefreshToken_LookupErrorPropagates(t *testing.T) {
	// Arrange
	svc, _, refreshTokens, issuer := newServiceWithMocks(t)
	wantErr := fmt.Errorf("connection refused")
	refreshTokens.findByHashFn = func(ctx context.Context, hash string) (*model.RefreshToken, error) {
		return nil, wantErr
	}

	// Act
	pair, err := svc.RotateRefreshToken(context.Background(), "plain-refresh-token")

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("err = %v, must not be ErrInvalidRefreshToken (DB 障害は上層 500)", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
	if pair != nil {
		t.Errorf("pair = %+v, want nil", pair)
	}
	if refreshTokens.markRotatedCalls != 0 || refreshTokens.createTokenCalls != 0 || issuer.issueCalls != 0 {
		t.Errorf("any downstream call must not occur on lookup error")
	}
}
