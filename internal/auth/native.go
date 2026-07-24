package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
)

// NativeAuthCodeTTL は native flow で発行する auth_code の有効期間。
// SERVER.md §1.2 の契約（60 秒・単回利用）に従う。
const NativeAuthCodeTTL = 60 * time.Second

// AuthCodeCreator は native callback が必要とする auth_code 保存の最小インターフェース。
// repository.AuthCodeRepository が構造的にこれを充足する（インターフェース分離のため
// 全 AuthCodeRepository ではなく Create のみに依存する）。
type AuthCodeCreator interface {
	// Create は AuthCode を新規保存する。
	Create(ctx context.Context, code *model.AuthCode) error
}

// HashNativeSecret は auth_code / refresh token 等の native auth 機密値を
// SHA-256 hex（lowercase）へハッシュする。永続化・検索は本ハッシュ値でのみ行い、
// 平文は保存しない（#164 model.AuthCode.CodeHash の規約）。後続の token 交換
// endpoint（#166 以降）でも同一関数を使用して hash 一致照合する。
func HashNativeSecret(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// HandleNativeCallback は native flow（flow=native）の OAuth callback を処理する。
//
// OAuth 認可コードを交換してユーザーを解決（Web flow と同一規則。未登録なら自動作成）し、
// Web 用セッションは作成せず、NativeAuthCodeTTL で失効する単回利用前提の auth_code を
// hash 保存して平文 auth_code を返す。平文は戻り値（リダイレクト URL 用）以外に残さない。
func (s *Service) HandleNativeCallback(ctx context.Context, code, pkceChallenge string) (string, error) {
	userID, err := s.resolveUserFromOAuth(ctx, code)
	if err != nil {
		return "", err
	}

	plainCode, err := GenerateAuthCode()
	if err != nil {
		return "", fmt.Errorf("failed to generate auth code: %w", err)
	}

	codeHash := HashNativeSecret(plainCode)
	authCode := &model.AuthCode{
		ID:            uuid.New().String(),
		CodeHash:      codeHash,
		UserID:        userID,
		PKCEChallenge: pkceChallenge,
		ExpiresAt:     time.Now().Add(NativeAuthCodeTTL),
	}

	if err := s.authCodeRepo.Create(ctx, authCode); err != nil {
		return "", fmt.Errorf("failed to store auth code: %w", err)
	}

	// 平文はログに残さず、hash 先頭 8 文字のみで追跡可能性を確保する
	// （既存 hashSessionIDForLog と同方針）。
	slog.Info("native auth code issued",
		slog.String("user_id", userID),
		slog.String("auth_code_hash", codeHash[:8]),
	)

	return plainCode, nil
}

// GenerateAuthCode は暗号論的乱数 32 byte（256bit）を base64url（no-padding）へ
// エンコードした URL-safe な auth_code を生成する（NFR 1.1）。
//
// パスキー認証成功時にも同じ生成規則で auth_code を発行するため（Issue #216 / Req 2.2 /
// 2.3）、`internal/passkey` から参照できるよう exported にしている（既存挙動は完全に
// 不変 / NFR 2.1）。
func GenerateAuthCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
