package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// fixedJWTSecret は jwt_issuer のテストで使う固定 secret（32 byte）。
// 本番運用では openssl rand -base64 32 で生成した値を NATIVE_AUTH_JWT_SECRET に設定する。
var fixedJWTSecret = []byte("test-jwt-secret-32bytes-long-1234")

// fixedJWTIssuedAt は IssueAccessToken の発行時刻として注入する固定時刻（UTC）。
// テスト内で iat / exp を検証する基準として全テストで共有する。
var fixedJWTIssuedAt = time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

// newFixedIssuer は now を固定時刻に差し替えた JWTIssuer を返すテストヘルパー。
// package 内のため非公開 now を直接書き換えできる（Req 3.4: テストでは固定時刻注入）。
func newFixedIssuer(t *testing.T, kid string) *JWTIssuer {
	t.Helper()
	issuer := NewJWTIssuer(fixedJWTSecret, kid)
	issuer.now = func() time.Time { return fixedJWTIssuedAt }
	return issuer
}

// TestNewJWTIssuer_DefaultsNowToTimeNow は now がコンストラクタ既定で time.Now を
// 指していること（テスト注入が無ければ実時刻ベースで発行される）を確認する。
func TestNewJWTIssuer_DefaultsNowToTimeNow(t *testing.T) {
	// Arrange
	issuer := NewJWTIssuer(fixedJWTSecret, "v1")

	// Act
	before := time.Now()
	got := issuer.now()
	after := time.Now()

	// Assert
	if got.Before(before) || got.After(after) {
		t.Errorf("issuer.now()=%v, want time within [%v, %v]", got, before, after)
	}
}

// TestIssueAccessToken_SuccessfulIssuance は固定 secret / 固定 now で発行した JWT を
// 同ライブラリで parse し、Req 1.4 / 3.5 のフィールドを検証する。
func TestIssueAccessToken_SuccessfulIssuance(t *testing.T) {
	// Arrange
	const userID = "user-test-1"
	const kid = "v1"
	issuer := newFixedIssuer(t, kid)

	// Act
	tokenString, err := issuer.IssueAccessToken(userID)
	if err != nil {
		t.Fatalf("IssueAccessToken returned error: %v", err)
	}

	// Assert: token は 3 つの dot 区切りで構成される
	if parts := strings.Split(tokenString, "."); len(parts) != 3 {
		t.Fatalf("token string parts = %d, want 3 (header.payload.signature)", len(parts))
	}

	// Assert: 同ライブラリで parse し署名検証が成功すること
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			t.Errorf("unexpected signing method: %v", token.Method)
		}
		return fixedJWTSecret, nil
	})
	if err != nil {
		t.Fatalf("jwt.Parse returned error: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("parsed token is not Valid")
	}

	// Assert: ヘッダの kid が一致する（Req 3.5）
	gotKid, ok := parsed.Header["kid"].(string)
	if !ok {
		t.Fatalf("header.kid is missing or not a string, got %T", parsed.Header["kid"])
	}
	if gotKid != kid {
		t.Errorf("header.kid = %q, want %q", gotKid, kid)
	}

	// Assert: alg が HS256 であること（design.md technology stack）
	if gotAlg, _ := parsed.Header["alg"].(string); gotAlg != "HS256" {
		t.Errorf("header.alg = %q, want %q", gotAlg, "HS256")
	}

	// Assert: claims の値を検証
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims type = %T, want jwt.MapClaims", parsed.Claims)
	}
	if got, _ := claims["sub"].(string); got != userID {
		t.Errorf("claims.sub = %q, want %q", got, userID)
	}
	if got, _ := claims["token_use"].(string); got != "access" {
		t.Errorf("claims.token_use = %q, want %q (refresh と区別)", got, "access")
	}
	// jti は非空の文字列
	jti, _ := claims["jti"].(string)
	if jti == "" {
		t.Error("claims.jti is empty, want non-empty UUID")
	}

	// iat は固定 now の Unix 時刻
	gotIat, _ := claims["iat"].(float64)
	wantIat := float64(fixedJWTIssuedAt.Unix())
	if gotIat != wantIat {
		t.Errorf("claims.iat = %v, want %v", gotIat, wantIat)
	}

	// exp は iat + 900 秒（AccessTokenTTL = 15 分）
	gotExp, _ := claims["exp"].(float64)
	wantExp := float64(fixedJWTIssuedAt.Add(AccessTokenTTL).Unix())
	if gotExp != wantExp {
		t.Errorf("claims.exp = %v, want %v (= iat + 900s, Req 1.4)", gotExp, wantExp)
	}
	if int64(gotExp-gotIat) != 900 {
		t.Errorf("exp - iat = %d seconds, want 900 (Req 1.4 / SERVER.md §1.4)", int64(gotExp-gotIat))
	}
}

// TestIssueAccessToken_EmptyUserID は userID が空文字のときエラーを返すことを検証する
// （sub なしの token を発行しないため、handler 層で userID を検査する前のセーフティ）。
func TestIssueAccessToken_EmptyUserID(t *testing.T) {
	// Arrange
	issuer := newFixedIssuer(t, "v1")

	// Act
	tokenString, err := issuer.IssueAccessToken("")

	// Assert
	if err == nil {
		t.Errorf("IssueAccessToken(\"\") returned token=%q, want error", tokenString)
	}
	if tokenString != "" {
		t.Errorf("IssueAccessToken(\"\") returned token=%q, want empty string", tokenString)
	}
}

// TestIssueAccessToken_KidPropagation は異なる kid を指定したとき、JWT ヘッダの kid に
// その値がそのまま出力されることを確認する（Req 3.5: 鍵ローテーション準備）。
func TestIssueAccessToken_KidPropagation(t *testing.T) {
	cases := []struct {
		name string
		kid  string
	}{
		{name: "kid=v1 のとき header.kid=v1", kid: "v1"},
		{name: "kid=v2 のとき header.kid=v2", kid: "v2"},
		{name: "kid=2026-06 のとき header.kid=2026-06", kid: "2026-06"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			issuer := newFixedIssuer(t, tc.kid)

			// Act
			tokenString, err := issuer.IssueAccessToken("user-test-1")
			if err != nil {
				t.Fatalf("IssueAccessToken: %v", err)
			}

			// Assert: parse して header.kid を確認
			parsed, err := jwt.Parse(tokenString, func(*jwt.Token) (interface{}, error) {
				return fixedJWTSecret, nil
			})
			if err != nil {
				t.Fatalf("jwt.Parse: %v", err)
			}
			gotKid, _ := parsed.Header["kid"].(string)
			if gotKid != tc.kid {
				t.Errorf("header.kid = %q, want %q", gotKid, tc.kid)
			}
		})
	}
}

// TestIssueAccessToken_DifferentJtisAcrossInvocations は連続発行で jti が常に異なる値
// （UUID）を返すことを確認する。失効リスト（将来の jti blocklist）が単一 token を識別
// できるための前提として担保する。
func TestIssueAccessToken_DifferentJtisAcrossInvocations(t *testing.T) {
	// Arrange
	issuer := newFixedIssuer(t, "v1")

	// Act
	const n = 5
	jtis := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		tokenString, err := issuer.IssueAccessToken("user-test-1")
		if err != nil {
			t.Fatalf("IssueAccessToken[%d]: %v", i, err)
		}
		parsed, err := jwt.Parse(tokenString, func(*jwt.Token) (interface{}, error) {
			return fixedJWTSecret, nil
		})
		if err != nil {
			t.Fatalf("jwt.Parse[%d]: %v", i, err)
		}
		claims := parsed.Claims.(jwt.MapClaims)
		jti, _ := claims["jti"].(string)
		if jti == "" {
			t.Fatalf("iteration %d: claims.jti is empty", i)
		}
		jtis[jti] = struct{}{}
	}

	// Assert: n 件すべて異なる jti を持つ
	if len(jtis) != n {
		t.Errorf("unique jtis = %d, want %d (5 連続発行で jti が重複)", len(jtis), n)
	}
}

// TestIssueAccessToken_WrongSecretFailsVerification は別 secret で署名検証したとき
// parse がエラーになることを確認する。鍵が秘匿されている前提が崩れない限り、別鍵で
// 検証成功する偽造 token は作成不可能（HS256 の対称鍵性質）。
func TestIssueAccessToken_WrongSecretFailsVerification(t *testing.T) {
	// Arrange
	issuer := newFixedIssuer(t, "v1")
	tokenString, err := issuer.IssueAccessToken("user-test-1")
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	// Act: 別 secret で parse
	wrongSecret := []byte("wrong-secret-32bytes-long-987654")
	parsed, err := jwt.Parse(tokenString, func(*jwt.Token) (interface{}, error) {
		return wrongSecret, nil
	})

	// Assert: 検証失敗（Valid=false または error 非 nil）
	if err == nil && parsed != nil && parsed.Valid {
		t.Error("token verified with wrong secret, want failure (HS256 対称鍵性質)")
	}
}
