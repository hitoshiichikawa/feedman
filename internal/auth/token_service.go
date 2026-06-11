package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// ErrInvalidGrant は auth_code 不明・期限切れ・使用済み・PKCE 不一致を
// 区別せず表す sentinel error（Req 2.6: 拒否応答 uniform 化）。
//
// handler 層は errors.Is(err, auth.ErrInvalidGrant) で 400 INVALID_GRANT への
// 振り分けを行う。本 sentinel をそのまま返さず %w で wrap して具体的な内部理由を
// 添えてもよいが、その内部理由は **クライアントへ反射しないこと**（NFR 1.5）。
var ErrInvalidGrant = errors.New("invalid grant")

// RefreshTokenTTL は POST /api/auth/token で発行する refresh token の有効期間。
// SERVER.md §1.4（30 日）に従う（Req 1.3 / design.md Data Models）。
const RefreshTokenTTL = 30 * 24 * time.Hour

// refreshTokenByteLen は refresh token の生成時に crypto/rand から取り出すバイト数。
// 32 byte = 256 bit のエントロピーで NFR 1.1 を満たす。
const refreshTokenByteLen = 32

// AuthCodeConsumer は TokenService が auth_code の参照・単回消費に必要とする最小 IF。
// repository.AuthCodeRepository が構造的に充足する（interface segregation）。
type AuthCodeConsumer interface {
	// FindByHash は code_hash で AuthCode を取得する。未検出は (nil, nil)。
	FindByHash(ctx context.Context, codeHash string) (*model.AuthCode, error)
	// MarkUsed は当該 ID の auth_code を used = true に遷移させる。
	// 既に使用済み・期限切れ・存在しない場合は repository.ErrAuthCodeNotUsable を返す。
	MarkUsed(ctx context.Context, id string) error
}

// RefreshTokenStore は TokenService が refresh token の新規発行に必要とする最小 IF。
// repository.RefreshTokenRepository が構造的に充足する。
type RefreshTokenStore interface {
	CreateFamily(ctx context.Context, family *model.RefreshTokenFamily) error
	CreateToken(ctx context.Context, token *model.RefreshToken) error
}

// AccessTokenIssuer は TokenService が access token (JWT) の発行に必要とする最小 IF。
// 本パッケージ内の *JWTIssuer が構造的に充足する。
type AccessTokenIssuer interface {
	IssueAccessToken(userID string) (string, error)
}

// TokenPair は POST /api/auth/token の成功レスポンスに載せる平文トークン値。
// 平文 RefreshToken はレスポンス用にのみ保持され、永続化されない（永続化は hash のみ）。
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn は AccessToken の有効秒数（= AccessTokenTTL）。
	// SERVER.md §1.3 の `expires_in` フィールドに対応する。
	ExpiresIn int
}

// TokenService は POST /api/auth/token の交換ロジックを担う。
//
// 拒否はすべて ErrInvalidGrant に正規化される（Req 2.6）。本 service の依存
// （authCodes / refreshTokens / issuer）は最小 IF として受け取り、テストではモックを
// 差し込んで外部ネットワーク依存なしで検証可能（NFR 3.1）。
type TokenService struct {
	authCodes     AuthCodeConsumer
	refreshTokens RefreshTokenStore
	issuer        AccessTokenIssuer
	now           func() time.Time
}

// NewTokenService は依存を受け取って TokenService を生成する。
// テストは生成後に内部 now を上書きできる（package 内）。
func NewTokenService(authCodes AuthCodeConsumer, refreshTokens RefreshTokenStore, issuer AccessTokenIssuer) *TokenService {
	return &TokenService{
		authCodes:     authCodes,
		refreshTokens: refreshTokens,
		issuer:        issuer,
		now:           time.Now,
	}
}

// ExchangeAuthCode は auth_code + code_verifier を access token + refresh token に
// 交換する（design.md「交換フロー」参照）。
//
// 手順:
//  1. auth_code を hash で参照（未検出 → ErrInvalidGrant）
//  2. PKCE S256 verifier 検証（不一致 / 形式不正 → ErrInvalidGrant）
//  3. auth_code を atomic に単回消費（既に used / 期限切れ → ErrInvalidGrant）
//  4. refresh family 作成 + refresh token (256bit / hash 保存 / 30 日) 発行
//  5. access token (JWT / 15 分) 発行
//
// 拒否時は refresh family / token を一切永続化しない（Req 2.7）。
// 平文 verifier / auth_code / refresh token をログ・エラーメッセージに残さない（NFR 1.2 / 1.3）。
func (s *TokenService) ExchangeAuthCode(ctx context.Context, authCode, codeVerifier string) (*TokenPair, error) {
	// 1. auth_code を hash で照合
	codeHash := HashNativeSecret(authCode)
	stored, err := s.authCodes.FindByHash(ctx, codeHash)
	if err != nil {
		return nil, fmt.Errorf("token exchange: lookup auth code: %w", err)
	}
	if stored == nil {
		// auth_code 不明（Req 2.2）
		return nil, ErrInvalidGrant
	}

	// 2. PKCE verifier 検証（Req 2.1, 2.4 / NFR 1.4）
	if !VerifyPKCES256Verifier(codeVerifier, stored.PKCEChallenge) {
		// 不一致 / 形式不正は同一 sentinel（Req 2.6）
		// 拒否時は手順 3 以降に進まないため refresh は永続化されない（Req 2.7）。
		return nil, ErrInvalidGrant
	}

	// 3. auth_code を atomic に単回消費（Req 1.2 / 2.3）
	// MarkUsed の UPDATE は WHERE used=false AND expires_at>now() を atomic に判定し、
	// 並行二重交換でも片方しか成功しない設計（#164）。
	if err := s.authCodes.MarkUsed(ctx, stored.ID); err != nil {
		if errors.Is(err, repository.ErrAuthCodeNotUsable) {
			// 既に使用済み / 期限切れ / 消失（Req 2.3 / 2.6）
			return nil, ErrInvalidGrant
		}
		return nil, fmt.Errorf("token exchange: mark used: %w", err)
	}

	// 4. refresh family 作成 + refresh token を発行（Req 1.3 / NFR 1.1 / 1.2）
	plainRefresh, refreshHash, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("token exchange: generate refresh token: %w", err)
	}

	now := s.now()
	family := &model.RefreshTokenFamily{
		ID:     uuid.New().String(),
		UserID: stored.UserID,
	}
	if err := s.refreshTokens.CreateFamily(ctx, family); err != nil {
		return nil, fmt.Errorf("token exchange: create refresh family: %w", err)
	}
	refresh := &model.RefreshToken{
		ID:        uuid.New().String(),
		FamilyID:  family.ID,
		UserID:    stored.UserID,
		TokenHash: refreshHash,
		ExpiresAt: now.Add(RefreshTokenTTL),
	}
	if err := s.refreshTokens.CreateToken(ctx, refresh); err != nil {
		return nil, fmt.Errorf("token exchange: create refresh token: %w", err)
	}

	// 5. access token を発行（Req 1.4）
	accessToken, err := s.issuer.IssueAccessToken(stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("token exchange: issue access token: %w", err)
	}

	// 機密値の追跡用に hash 先頭 8 文字のみログに残す（既存 native callback と同方針）。
	slog.Info("native token exchange succeeded",
		slog.String("user_id", stored.UserID),
		slog.String("auth_code_hash", codeHash[:8]),
		slog.String("refresh_token_hash", refreshHash[:8]),
	)

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: plainRefresh,
		ExpiresIn:    int(AccessTokenTTL / time.Second),
	}, nil
}

// generateRefreshToken は crypto/rand から 256bit を取り出し、base64url（no-pad）
// にエンコードした平文 token と、その SHA-256 hash を返す。平文は応答用、hash は
// 永続化用（Req 1.3 / NFR 1.1 / 1.2）。
func generateRefreshToken() (plain, hash string, err error) {
	b := make([]byte, refreshTokenByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	hash = HashNativeSecret(plain)
	return plain, hash, nil
}
