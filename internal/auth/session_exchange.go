package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// SessionCreator は SessionExchangeService が session 永続化に必要とする最小 IF。
// repository.SessionRepository が構造的にこれを充足する（interface segregation /
// CLAUDE.md §5）。TokenService のように refresh token family / access token を扱わない
// ため、依存を Create 1 メソッドに絞ることで結合を最小化する。
type SessionCreator interface {
	// Create は Session を新規保存する。
	Create(ctx context.Context, s *model.Session) error
}

// SessionExchangeService はパスキー認証で発行された auth_code + PKCE code_verifier を
// Web 用の Cookie session に交換するサービス（design.md §SessionExchangeService）。
//
// 拒否（未検出 / PKCE 不一致 / used / 期限切れ）は既存 ErrInvalidGrant に uniform
// 化される（Req 3.1 / 4.2 / NFR 1.2）。既存 TokenService.ExchangeAuthCode の前半 3 段
// （HashNativeSecret → FindByHash → VerifyPKCES256Verifier → MarkUsed）を再利用し、
// 以降を session 発行（generateSessionID → SessionCreator.Create）に差し替える。
//
// TokenService とは異なり refresh token family / access token JWT を扱わないため、
// 依存を AuthCodeConsumer + SessionCreator の 2 最小 IF に絞る（interface segregation）。
type SessionExchangeService struct {
	authCodes      AuthCodeConsumer
	sessions       SessionCreator
	sessionFactory SessionFactoryFunc
}

// NewSessionExchangeService は依存を受け取って SessionExchangeService を生成する。
//
// session の ID / CreatedAt / ExpiresAt 生成は共有 SessionFactory（Issue #231 §Delta 1）に
// 委譲する。これにより registration direct session（RegistrationService）と同一の session
// 構築ロジック（generateSessionID + now + TTL）を共有する。auth_code 交換の外部挙動は不変。
func NewSessionExchangeService(authCodes AuthCodeConsumer, sessions SessionCreator, sessionFactory SessionFactoryFunc) *SessionExchangeService {
	return &SessionExchangeService{
		authCodes:      authCodes,
		sessions:       sessions,
		sessionFactory: sessionFactory,
	}
}

// ExchangeAuthCodeForSession は auth_code + code_verifier を Web 用 Cookie session に
// 交換する（design.md §SessionExchangeService の Contracts）。
//
// 手順（既存 TokenService.ExchangeAuthCode と共通の 3 段を踏み、以降を session 発行に
// 差し替え）:
//  1. auth_code を HashNativeSecret → AuthCodeConsumer.FindByHash（未検出 → ErrInvalidGrant）
//  2. VerifyPKCES256Verifier（不一致 / 形式不正 → ErrInvalidGrant）
//  3. AuthCodeConsumer.MarkUsed（既に used / 期限切れ / 消失 → ErrInvalidGrant）
//  4. SessionFactory.NewSession で ID / CreatedAt / ExpiresAt を一貫生成
//  5. SessionCreator.Create で永続化
//
// 拒否時は session を一切永続化しない（手順 4 / 5 に進まない）。infra エラーは
// fmt.Errorf("...: %w", err) で wrap し ErrInvalidGrant に正規化しない（handler で
// 500 INTERNAL_ERROR に振り分けるため / design.md §Error Handling）。
//
// 平文 authCode / codeVerifier / sessionID をログ・エラーメッセージに残さない
// （NFR 1.1）。追跡ログは既存 TokenService と同方針で code_hash / session_id_hash の
// 先頭 8 文字のみを出力する。
func (s *SessionExchangeService) ExchangeAuthCodeForSession(ctx context.Context, authCode, codeVerifier string) (*model.Session, error) {
	// 1. auth_code を hash で照合（未検出は ErrInvalidGrant / Req 3.1 / 4.2 / NFR 1.2）
	codeHash := HashNativeSecret(authCode)
	stored, err := s.authCodes.FindByHash(ctx, codeHash)
	if err != nil {
		return nil, fmt.Errorf("session exchange: lookup auth code: %w", err)
	}
	if stored == nil {
		return nil, ErrInvalidGrant
	}

	// 2. PKCE verifier 検証（不一致 / 形式不正は ErrInvalidGrant / Req 3.1 / 4.2）
	// 拒否時は手順 3 以降に進まないため MarkUsed / Create は呼ばれない。
	if !VerifyPKCES256Verifier(codeVerifier, stored.PKCEChallenge) {
		return nil, ErrInvalidGrant
	}

	// 3. auth_code を atomic に単回消費（既存 TokenService と同流儀 / Req 3.1 / 4.2）
	if err := s.authCodes.MarkUsed(ctx, stored.ID); err != nil {
		if errors.Is(err, repository.ErrAuthCodeNotUsable) {
			// 既に使用済み / 期限切れ / 消失
			return nil, ErrInvalidGrant
		}
		return nil, fmt.Errorf("session exchange: mark used: %w", err)
	}

	// 4. session 生成（共有 SessionFactory / Issue #231 §Delta 1）。
	//    既存 Service.createSession と同じ crypto random 32 バイト / hex エンコードの
	//    generateSessionID を factory 経由で再利用し、ID / CreatedAt / ExpiresAt を単一 now で整合させる。
	session, err := s.sessionFactory.NewSession(stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("session exchange: generate session: %w", err)
	}

	// 5. session 永続化
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("session exchange: create session: %w", err)
	}

	// 機密値の追跡用に hash 先頭 8 文字のみログに残す（既存 TokenService.ExchangeAuthCode
	// と同方針 / NFR 1.1）。平文 authCode / codeVerifier / sessionID は残さない。
	sessionIDHash := HashNativeSecret(session.ID)
	slog.Info("session exchange succeeded",
		slog.String("user_id", stored.UserID),
		slog.String("auth_code_hash", codeHash[:8]),
		slog.String("session_id_hash", sessionIDHash[:8]),
	)

	return session, nil
}
