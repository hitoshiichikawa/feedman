package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTVerifier は JWTIssuer（#166）が発行した access token（HS256 JWT）を検証する
// （Issue #169）。
//
// 検証は署名鍵と時刻のみで完結し、保存済みセッション情報・外部サービスへの照会を
// 行わない（NFR 1.2: stateless 検証）。鍵 (secret) はコンストラクタで固定する。
// テストでは内部 now func の上書きで時刻を決定論的に注入する（Req 4.5。JWTIssuer と
// 同パターン）。本構造体は immutable であり、複数の goroutine から並行使用しても安全。
type JWTVerifier struct {
	secret []byte
	now    func() time.Time
}

// NewJWTVerifier は secret を保持する JWTVerifier を生成する。
//
// secret は JWTIssuer と同一の env 値（NATIVE_AUTH_JWT_SECRET）から渡すこと（Req 4.1:
// 発行側と同一の鍵設定で検証する）。
func NewJWTVerifier(secret []byte) *JWTVerifier {
	return &JWTVerifier{
		secret: secret,
		now:    time.Now,
	}
}

// VerifyAccessToken は tokenString を検証し、成立時に userID（sub claim）を返す。
//
// 検証規則（design.md 検証規則表 / #166 発行規約との対応）:
//   - alg: HS256 限定（jwt.WithValidMethods。none / RS256 等の alg 偽装を拒否 / Req 2.1）
//   - 署名: secret による HMAC 署名検証（Req 2.1）
//   - exp: 必須（jwt.WithExpirationRequired）かつ now 超過で拒否。leeway 0（Req 2.2）
//   - token_use: "access" 厳密一致以外は拒否（refresh 等の流用防止 / Req 2.3）
//   - sub: 非空必須。成功時の返り値 userID
//   - iat: 存在時はライブラリ標準検証に委ねる（presence は要求しない）
//   - jti / header kid: 参照しない（失効リスト・複数鍵受理は Out of Scope）
//
// 失敗理由は error に含まれるが、呼び出し側は理由を HTTP 応答に反映しないこと
// （Req 2.7: 拒否理由を区別できる情報を返さない）。token 文字列自体も error に
// 含まれない（NFR 1.1）。
func (v *JWTVerifier) VerifyAccessToken(tokenString string) (string, error) {
	token, err := jwt.Parse(
		tokenString,
		func(t *jwt.Token) (any, error) { return v.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return v.now() }),
	)
	if err != nil {
		// jwt ライブラリの error は token 値を含まない（NFR 1.1）。
		return "", fmt.Errorf("access token validation failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("access token validation failed: unexpected claims type")
	}

	// token_use == "access" の厳密一致（refresh token 等の認証流用を拒否 / Req 2.3）
	if use, _ := claims["token_use"].(string); use != accessTokenUse {
		return "", errors.New("access token validation failed: token_use is not access")
	}

	// sub 非空必須（ユーザー識別の正本 / Req 1.1 の userID 解決元）
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("access token validation failed: sub is empty")
	}

	return sub, nil
}
