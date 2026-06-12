package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// newFixedVerifier は now を固定時刻に差し替えた JWTVerifier を返すテストヘルパー。
// package 内のため非公開 now を直接書き換えできる（Req 4.5: テストでは固定時刻注入。
// jwt_issuer_test.go の newFixedIssuer と同パターン・同一 fixture を共有する）。
func newFixedVerifier(t *testing.T, secret []byte, now time.Time) *JWTVerifier {
	t.Helper()
	verifier := NewJWTVerifier(secret)
	verifier.now = func() time.Time { return now }
	return verifier
}

// issueFixedToken は固定 secret / 固定 now の JWTIssuer（#166）で access token を発行する。
// 発行 ↔ 検証の規約整合（Req 4.1）を実物ペアで検証するための共通 Arrange。
func issueFixedToken(t *testing.T, userID string) string {
	t.Helper()
	issuer := newFixedIssuer(t, "v1")
	tokenString, err := issuer.IssueAccessToken(userID)
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}
	return tokenString
}

// signTokenWithClaims は任意 claims / 任意 method / 任意鍵で JWT を作るテストヘルパー。
// 用途不一致・sub 欠落・alg 偽装など、正規 issuer では発行できない token を組み立てる。
func signTokenWithClaims(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString returned error: %v", err)
	}
	return signed
}

// TestVerifyAccessToken_IssuerVerifierRoundTrip は #166 JWTIssuer と固定 secret /
// 固定 now を共有して発行した token を VerifyAccessToken が受理し、userID（sub）を
// 返すことを検証する（Testing Strategy 1 / Req 4.1: 発行 ↔ 検証の規約整合）。
func TestVerifyAccessToken_IssuerVerifierRoundTrip(t *testing.T) {
	// Arrange: 発行直後（exp 内）の時刻で検証する
	const userID = "user-test-1"
	tokenString := issueFixedToken(t, userID)
	verifier := newFixedVerifier(t, fixedJWTSecret, fixedJWTIssuedAt.Add(1*time.Minute))

	// Act
	got, err := verifier.VerifyAccessToken(tokenString)

	// Assert
	if err != nil {
		t.Fatalf("VerifyAccessToken returned error: %v", err)
	}
	if got != userID {
		t.Errorf("userID = %q, want %q (sub claim の解決 / Req 1.1)", got, userID)
	}
}

// TestVerifyAccessToken_Expired は exp 超過の token を拒否することを検証する
// （Testing Strategy 2 / Req 2.2。境界値: ちょうど exp 時刻も leeway 0 で拒否）。
func TestVerifyAccessToken_Expired(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
	}{
		{
			name: "exp を 1 秒超過したとき拒否",
			now:  fixedJWTIssuedAt.Add(AccessTokenTTL + 1*time.Second),
		},
		{
			name: "exp 丁度のとき拒否（境界値: leeway 0）",
			now:  fixedJWTIssuedAt.Add(AccessTokenTTL),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tokenString := issueFixedToken(t, "user-test-1")
			verifier := newFixedVerifier(t, fixedJWTSecret, tc.now)

			// Act
			got, err := verifier.VerifyAccessToken(tokenString)

			// Assert
			if err == nil {
				t.Fatal("err = nil, want non-nil (期限切れは拒否 / Req 2.2)")
			}
			if got != "" {
				t.Errorf("userID = %q, want empty", got)
			}
		})
	}
}

// TestVerifyAccessToken_WrongSecret は異なる secret で署名された token を拒否することを
// 検証する（Testing Strategy 3 / Req 2.1: 署名検証不能の拒否）。
func TestVerifyAccessToken_WrongSecret(t *testing.T) {
	// Arrange: 正規 issuer と異なる鍵で署名する
	otherSecret := []byte("another-jwt-secret-32bytes-long-x")
	issuer := NewJWTIssuer(otherSecret, "v1")
	issuer.now = func() time.Time { return fixedJWTIssuedAt }
	tokenString, err := issuer.IssueAccessToken("user-test-1")
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}
	verifier := newFixedVerifier(t, fixedJWTSecret, fixedJWTIssuedAt.Add(1*time.Minute))

	// Act
	got, err := verifier.VerifyAccessToken(tokenString)

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil (署名不正は拒否 / Req 2.1)")
	}
	if got != "" {
		t.Errorf("userID = %q, want empty", got)
	}
}

// TestVerifyAccessToken_TokenUseMismatch は token_use が "access" でない token を拒否する
// ことを検証する（Testing Strategy 4 / Req 2.3: 用途種別の限定）。
func TestVerifyAccessToken_TokenUseMismatch(t *testing.T) {
	now := fixedJWTIssuedAt
	baseClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"sub": "user-test-1",
			"iat": now.Unix(),
			"exp": now.Add(AccessTokenTTL).Unix(),
		}
	}
	cases := []struct {
		name   string
		mutate func(c jwt.MapClaims)
	}{
		{name: "token_use が refresh のとき拒否", mutate: func(c jwt.MapClaims) { c["token_use"] = "refresh" }},
		{name: "token_use 欠落のとき拒否", mutate: func(c jwt.MapClaims) {}},
		{name: "token_use が空文字のとき拒否", mutate: func(c jwt.MapClaims) { c["token_use"] = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			claims := baseClaims()
			tc.mutate(claims)
			tokenString := signTokenWithClaims(t, jwt.SigningMethodHS256, fixedJWTSecret, claims)
			verifier := newFixedVerifier(t, fixedJWTSecret, now.Add(1*time.Minute))

			// Act
			_, err := verifier.VerifyAccessToken(tokenString)

			// Assert
			if err == nil {
				t.Fatal("err = nil, want non-nil (token_use 不一致は拒否 / Req 2.3)")
			}
		})
	}
}

// TestVerifyAccessToken_AlgConfusion は HS256 以外のアルゴリズム（none）の token を
// 拒否することを検証する（Testing Strategy 5 / alg 偽装拒否）。
func TestVerifyAccessToken_AlgConfusion(t *testing.T) {
	// Arrange: alg=none の unsigned token を組み立てる
	now := fixedJWTIssuedAt
	claims := jwt.MapClaims{
		"sub":       "user-test-1",
		"iat":       now.Unix(),
		"exp":       now.Add(AccessTokenTTL).Unix(),
		"token_use": accessTokenUse,
	}
	noneToken := signTokenWithClaims(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims)
	verifier := newFixedVerifier(t, fixedJWTSecret, now.Add(1*time.Minute))

	// Act
	_, err := verifier.VerifyAccessToken(noneToken)

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil (alg=none は WithValidMethods で拒否)")
	}
}

// TestVerifyAccessToken_MalformedTokens は sub 空 / 欠落・exp 欠落・JWT 形式でない文字列・
// 空文字列を拒否することを検証する（Testing Strategy 6 / Req 2.5 関連の形式不正系）。
func TestVerifyAccessToken_MalformedTokens(t *testing.T) {
	now := fixedJWTIssuedAt
	withClaims := func(mutate func(c jwt.MapClaims)) string {
		claims := jwt.MapClaims{
			"sub":       "user-test-1",
			"iat":       now.Unix(),
			"exp":       now.Add(AccessTokenTTL).Unix(),
			"token_use": accessTokenUse,
		}
		mutate(claims)
		return signTokenWithClaims(t, jwt.SigningMethodHS256, fixedJWTSecret, claims)
	}
	cases := []struct {
		name        string
		tokenString string
	}{
		{name: "sub 空のとき拒否", tokenString: withClaims(func(c jwt.MapClaims) { c["sub"] = "" })},
		{name: "sub 欠落のとき拒否", tokenString: withClaims(func(c jwt.MapClaims) { delete(c, "sub") })},
		{name: "exp 欠落のとき拒否（WithExpirationRequired）", tokenString: withClaims(func(c jwt.MapClaims) { delete(c, "exp") })},
		{name: "JWT 形式でない文字列のとき拒否", tokenString: "abc"},
		{name: "空文字列のとき拒否", tokenString: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			verifier := newFixedVerifier(t, fixedJWTSecret, now.Add(1*time.Minute))

			// Act
			got, err := verifier.VerifyAccessToken(tc.tokenString)

			// Assert
			if err == nil {
				t.Fatal("err = nil, want non-nil")
			}
			if got != "" {
				t.Errorf("userID = %q, want empty", got)
			}
		})
	}
}

// TestVerifyAccessToken_ErrorDoesNotContainToken はエラーメッセージに token 文字列が
// 含まれないことを検証する（NFR 1.1: token 値をログ・エラーに残さない前提の担保）。
func TestVerifyAccessToken_ErrorDoesNotContainToken(t *testing.T) {
	// Arrange: 期限切れの正規 token（最も情報を持つ失敗ケース）
	tokenString := issueFixedToken(t, "user-test-1")
	verifier := newFixedVerifier(t, fixedJWTSecret, fixedJWTIssuedAt.Add(2*AccessTokenTTL))

	// Act
	_, err := verifier.VerifyAccessToken(tokenString)

	// Assert
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if strings.Contains(err.Error(), tokenString) {
		t.Errorf("error message contains token string (NFR 1.1)")
	}
}

// TestNewJWTVerifier_DefaultsNowToTimeNow は now がコンストラクタ既定で time.Now を
// 指していること（テスト注入が無ければ実時刻ベースで検証される）を確認する。
func TestNewJWTVerifier_DefaultsNowToTimeNow(t *testing.T) {
	// Arrange
	verifier := NewJWTVerifier(fixedJWTSecret)

	// Act
	before := time.Now()
	got := verifier.now()
	after := time.Now()

	// Assert
	if got.Before(before) || got.After(after) {
		t.Errorf("verifier.now()=%v, want time within [%v, %v]", got, before, after)
	}
}
