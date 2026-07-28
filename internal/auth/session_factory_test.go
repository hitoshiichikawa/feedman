package auth

import (
	"errors"
	"testing"
	"time"
)

// 本ファイルは共有 SessionFactory（Issue #231 §Delta 1）の単体テスト。
//
// SessionFactory は SessionExchangeService（パスキーログイン）と RegistrationService
// （Web 直接登録 session）で session 構築ロジックを共有するための最小 factory。
// テストは newID seam に失敗関数を注入して ID 生成失敗を局所的に検証し、
// crypto/rand.Reader の global 差替え（並列テスト汚染）を避ける（Blocker #2）。

// TestSessionFactory_NewSession_UsesSingleNow は NewSession が単一の now を用いて
// CreatedAt / ExpiresAt を整合させ、ID を newID から採用することを検証する。
func TestSessionFactory_NewSession_UsesSingleNow(t *testing.T) {
	// Arrange: 固定 now / 固定 TTL / 決定的 newID を注入した factory
	const userID = "factory-user-1"
	const ttl = 7 * 24 * time.Hour
	fixedNow := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	f := &SessionFactory{
		ttl:   ttl,
		now:   func() time.Time { return fixedNow },
		newID: func() (string, error) { return "deterministic-session-id", nil },
	}

	// Act
	sess, err := f.NewSession(userID)

	// Assert
	if err != nil {
		t.Fatalf("NewSession returned error: %v", err)
	}
	if sess == nil {
		t.Fatal("returned Session is nil")
	}
	if sess.ID != "deterministic-session-id" {
		t.Errorf("session.ID = %q, want %q", sess.ID, "deterministic-session-id")
	}
	if sess.UserID != userID {
		t.Errorf("session.UserID = %q, want %q", sess.UserID, userID)
	}
	if !sess.CreatedAt.Equal(fixedNow) {
		t.Errorf("session.CreatedAt = %v, want %v (= now)", sess.CreatedAt, fixedNow)
	}
	if !sess.ExpiresAt.Equal(fixedNow.Add(ttl)) {
		t.Errorf("session.ExpiresAt = %v, want %v (= CreatedAt + TTL)", sess.ExpiresAt, fixedNow.Add(ttl))
	}
}

// TestSessionFactory_NewSession_IDGenerationFailure は newID seam に失敗関数を注入した
// とき、NewSession が error をそのまま返し、部分構築した Session を返さないことを検証する
// （Blocker #2: crypto/rand.Reader の global 差替えなしで rand 失敗を局所注入）。
func TestSessionFactory_NewSession_IDGenerationFailure(t *testing.T) {
	// Arrange
	wantErr := errors.New("rand read failed")
	f := &SessionFactory{
		ttl:   time.Hour,
		now:   func() time.Time { return time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC) },
		newID: func() (string, error) { return "", wantErr },
	}

	// Act
	sess, err := f.NewSession("factory-user-1")

	// Assert: error を透過し、Session は nil（部分構築なし）
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if sess != nil {
		t.Errorf("session = %+v, want nil (ID 生成失敗時は部分構築しない)", sess)
	}
}

// TestNewSessionFactory_DefaultsProduceHexID は production 既定
// （now=time.Now / newID=generateSessionID）で構築した factory が、
// generateSessionID と同一形式（32 バイト → 64 文字 lowercase hex）の ID と
// 正の TTL を反映した ExpiresAt を返すことを検証する。
func TestNewSessionFactory_DefaultsProduceHexID(t *testing.T) {
	// Arrange
	const ttl = 24 * time.Hour
	f := NewSessionFactory(ttl)

	// Act
	before := time.Now()
	sess, err := f.NewSession("factory-user-1")
	after := time.Now()

	// Assert
	if err != nil {
		t.Fatalf("NewSession returned error: %v", err)
	}
	if !sessionIDHexPattern.MatchString(sess.ID) {
		t.Errorf("session.ID = %q, want 64-char lowercase hex", sess.ID)
	}
	// CreatedAt は before..after の範囲内（time.Now 既定）。
	if sess.CreatedAt.Before(before) || sess.CreatedAt.After(after) {
		t.Errorf("session.CreatedAt = %v, want within [%v, %v]", sess.CreatedAt, before, after)
	}
	// ExpiresAt = CreatedAt + ttl。
	if !sess.ExpiresAt.Equal(sess.CreatedAt.Add(ttl)) {
		t.Errorf("session.ExpiresAt = %v, want %v (= CreatedAt + ttl)", sess.ExpiresAt, sess.CreatedAt.Add(ttl))
	}
}
