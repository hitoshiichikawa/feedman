// Package auth はOAuth認証フロー、セッション管理を提供する。
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// OAuthUserInfo はOAuthプロバイダーから取得したユーザー情報を表す。
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	Name           string
	Provider       string // "google", "github" 等
}

// OAuthProvider はOAuth認証プロバイダーのインターフェース。
// 将来的に複数IdP（Google, GitHub等）に対応するための抽象化。
type OAuthProvider interface {
	// GetLoginURL はOAuth認証URLを生成する。
	GetLoginURL(state string) string
	// ExchangeCode は認可コードをトークンに交換し、ユーザー情報を取得する。
	ExchangeCode(ctx context.Context, code string) (*OAuthUserInfo, error)
}

// ServiceConfig は認証サービスの設定。
type ServiceConfig struct {
	SessionMaxAge int // セッション有効期間（秒）
}

// Service は認証に関するビジネスロジックを提供する。
type Service struct {
	oauth       OAuthProvider
	userRepo    repository.UserRepository
	identRepo   repository.IdentityRepository
	sessionRepo repository.SessionRepository
	config      ServiceConfig
}

// NewService はServiceを生成する。
func NewService(
	oauth OAuthProvider,
	userRepo repository.UserRepository,
	identRepo repository.IdentityRepository,
	sessionRepo repository.SessionRepository,
	config ServiceConfig,
) *Service {
	return &Service{
		oauth:       oauth,
		userRepo:    userRepo,
		identRepo:   identRepo,
		sessionRepo: sessionRepo,
		config:      config,
	}
}

// GetLoginURL はOAuth認証URLを生成する。
func (s *Service) GetLoginURL(state string) string {
	return s.oauth.GetLoginURL(state)
}

// HandleCallback はOAuthコールバックを処理し、セッションを発行する。
// 未登録ユーザーの場合はusersレコードとidentitiesレコードを同時に自動作成する。
// 登録済みユーザーの場合はidentitiesテーブルで既存ユーザーを特定しログインする。
func (s *Service) HandleCallback(ctx context.Context, code string) (*model.Session, error) {
	// 1. 認可コードをトークンに交換し、ユーザー情報を取得
	userInfo, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange oauth code: %w", err)
	}

	// 2. identitiesテーブルで既存ユーザーを検索
	identity, err := s.identRepo.FindByProviderAndProviderUserID(ctx, userInfo.Provider, userInfo.ProviderUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to find identity: %w", err)
	}

	var userID string

	if identity != nil {
		// 3a. 既存ユーザー: identityからユーザーIDを取得
		userID = identity.UserID
		slog.Info("existing user logged in",
			slog.String("user_id", userID),
			slog.String("provider", userInfo.Provider),
		)
	} else {
		// 3b. 新規ユーザー: usersレコードとidentitiesレコードを同時に作成
		newUserID := uuid.New().String()
		newIdentityID := uuid.New().String()
		now := time.Now()

		newUser := &model.User{
			ID:        newUserID,
			Email:     userInfo.Email,
			Name:      userInfo.Name,
			CreatedAt: now,
			UpdatedAt: now,
		}

		newIdentity := &model.Identity{
			ID:             newIdentityID,
			UserID:         newUserID,
			Provider:       userInfo.Provider,
			ProviderUserID: userInfo.ProviderUserID,
			CreatedAt:      now,
		}

		if err := s.userRepo.CreateWithIdentity(ctx, newUser, newIdentity); err != nil {
			return nil, fmt.Errorf("failed to create user and identity: %w", err)
		}

		userID = newUserID
		slog.Info("new user created",
			slog.String("user_id", userID),
			slog.String("email", userInfo.Email),
			slog.String("provider", userInfo.Provider),
		)
	}

	// 4. セッションを発行
	session, err := s.createSession(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// Logout はセッションを破棄する。
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}

	if err := s.sessionRepo.DeleteByID(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	slog.Info("user logged out", slog.String("session_id", sessionID))
	return nil
}

// GetCurrentUser はセッションから現在のユーザーを取得する。
func (s *Service) GetCurrentUser(ctx context.Context, sessionID string) (*model.User, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}

	session, err := s.sessionRepo.FindByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("session not found or expired")
	}

	user, err := s.userRepo.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return user, nil
}

// createSession はセッションを作成し永続化する。
func (s *Service) createSession(ctx context.Context, userID string) (*model.Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	session := &model.Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Duration(s.config.SessionMaxAge) * time.Second),
		CreatedAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// generateSessionID は暗号的に安全なセッションIDを生成する。
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
