package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessTokenTTL は POST /api/auth/token で発行する access token の有効期間。
// SERVER.md §1.4（expires_in: 900 秒 = 15 分）に従う（Req 1.4）。
const AccessTokenTTL = 15 * time.Minute

// accessTokenUse は access token と refresh token を区別するための claim 値。
// refresh token は別系統で発行されるため、検証側（#169）が "access" の存在を確認する。
const accessTokenUse = "access"

// JWTIssuer は HS256 で署名する access token (JWT) を発行する。
//
// 鍵 (secret) と鍵識別子 (kid) はコンストラクタで固定する。テストでは内部 now func の
// 上書きで時刻を決定論的に注入する（Req 3.4）。本構造体は immutable であり、複数の
// goroutine から並行使用しても安全。
type JWTIssuer struct {
	secret []byte
	kid    string
	now    func() time.Time
}

// NewJWTIssuer は secret と kid を保持する JWTIssuer を生成する。
//
// secret は HS256 の対称鍵として用いる（呼び出し側で長さの妥当性を担保する責務を負う）。
// kid は JWT ヘッダの "kid" として埋め込まれ、将来の鍵ローテーションで複数鍵受理時に
// 検証側がどの鍵で署名されたかを識別する（Req 3.5）。
func NewJWTIssuer(secret []byte, kid string) *JWTIssuer {
	return &JWTIssuer{
		secret: secret,
		kid:    kid,
		now:    time.Now,
	}
}

// IssueAccessToken は userID を sub に持つ HS256 JWT を発行する。
//
// claims: sub = userID / exp = now + AccessTokenTTL / iat = now / jti = uuid /
// token_use = "access"。JWT ヘッダに kid を含める（Req 3.5）。
//
// userID が空文字の場合は error を返す（sub なしの token を発行しないため）。
func (i *JWTIssuer) IssueAccessToken(userID string) (string, error) {
	if userID == "" {
		return "", errors.New("userID is required to issue access token")
	}

	now := i.now()
	claims := jwt.MapClaims{
		"sub":       userID,
		"iat":       now.Unix(),
		"exp":       now.Add(AccessTokenTTL).Unix(),
		"jti":       uuid.New().String(),
		"token_use": accessTokenUse,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// JWT ヘッダの "kid" は HS256 を選択した時点で付与しても検証時に影響しないが、
	// 将来の鍵ローテーション（複数鍵受理）で検証側が鍵を選択できるよう常に出力する。
	token.Header["kid"] = i.kid

	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}
	return signed, nil
}
