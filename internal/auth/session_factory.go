package auth

import (
	"time"

	"github.com/hitoshi/feedman/internal/model"
)

// SessionFactory は model.Session を「ID + 単一 now + CreatedAt + ExpiresAt」で一貫生成する
// 共有ファクトリである（Issue #231 design.md §Delta 1 §共有 session factory）。
//
// パスキーログインの SessionExchangeService と、Web 直接登録 session（RegistrationService）が
// 同一の session 構築ロジック（generateSessionID + now + TTL）を共有するために新設する。
// generateSessionID（internal/auth/service.go / 同一 package の unexported 関数）を既定
// newID として束ね、複製を排除する。Google OAuth の Service.createSession は既存の独立
// 実装を維持し、本 factory へは移行しない。
type SessionFactory struct {
	ttl   time.Duration
	now   func() time.Time       // テスト差し替え可（既定 time.Now）
	newID func() (string, error) // ID 生成の test seam（既定 = generateSessionID）。
	// crypto/rand.Reader の global 差替えを避け、rand 失敗を局所注入できる（Blocker #2）。
}

// NewSessionFactory は production 既定（now=time.Now / newID=generateSessionID）で factory を
// 構築する。generateSessionID は同一 package（internal/auth）の unexported 関数のため、
// export helper を新設せずそのまま既定 newID に束ねる（薄い public NewSessionID は追加しない
// / Blocker #2）。
func NewSessionFactory(ttl time.Duration) *SessionFactory {
	return &SessionFactory{ttl: ttl, now: time.Now, newID: generateSessionID}
}

// NewSession は 1 度の now を用いて ID / CreatedAt / ExpiresAt を整合させた Session を返す。
//
// ID 生成失敗（newID 失敗 = rand.Read 失敗）はそのまま error として返し、部分構築した
// Session は返さない。CreatedAt と ExpiresAt は同一の now から算出するため常に整合する。
func (f *SessionFactory) NewSession(userID string) (*model.Session, error) {
	id, err := f.newID() // 既定は generateSessionID。テストは失敗する newID を注入して検証（Blocker #2）
	if err != nil {
		return nil, err
	}
	now := f.now()
	return &model.Session{
		ID:        id,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(f.ttl),
	}, nil
}

// SessionFactoryFunc は RegistrationService と SessionExchangeService が受ける最小 IF
// （interface segregation / CLAUDE.md §5）。*SessionFactory が構造的にこれを充足する。
type SessionFactoryFunc interface {
	// NewSession は userID に紐づく新規 Session を生成する。ID 生成失敗時は error を返す。
	NewSession(userID string) (*model.Session, error)
}
