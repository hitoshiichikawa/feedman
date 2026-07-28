package auth

import (
	"errors"
	"testing"
	"time"
)

func TestSessionFactory_NewSession(t *testing.T) {
	// Arrange
	const (
		userID    = "user-1"
		sessionID = "fixed-session-id"
	)
	fixedNow := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	ttl := 90 * time.Minute
	nowCalls := 0
	idCalls := 0
	factory := NewSessionFactory(ttl)
	factory.now = func() time.Time {
		nowCalls++
		return fixedNow
	}
	factory.newID = func() (string, error) {
		idCalls++
		return sessionID, nil
	}

	// Act
	session, err := factory.NewSession(userID)

	// Assert
	if err != nil {
		t.Fatalf("NewSession returned error: %v", err)
	}
	if session == nil {
		t.Fatal("NewSession returned nil session")
	}
	if session.ID != sessionID || session.UserID != userID {
		t.Errorf("session ID/UserID = %q/%q, want %q/%q",
			session.ID, session.UserID, sessionID, userID)
	}
	if !session.CreatedAt.Equal(fixedNow) {
		t.Errorf("CreatedAt = %v, want %v", session.CreatedAt, fixedNow)
	}
	if !session.ExpiresAt.Equal(fixedNow.Add(ttl)) {
		t.Errorf("ExpiresAt = %v, want %v", session.ExpiresAt, fixedNow.Add(ttl))
	}
	if nowCalls != 1 {
		t.Errorf("now calls = %d, want 1", nowCalls)
	}
	if idCalls != 1 {
		t.Errorf("newID calls = %d, want 1", idCalls)
	}
}

func TestSessionFactory_NewSession_IDFailureReturnsNoPartialSession(t *testing.T) {
	// Arrange
	wantErr := errors.New("random source unavailable")
	nowCalls := 0
	factory := NewSessionFactory(time.Hour)
	factory.now = func() time.Time {
		nowCalls++
		return time.Now()
	}
	factory.newID = func() (string, error) {
		return "", wantErr
	}

	// Act
	session, err := factory.NewSession("user-1")

	// Assert
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if session != nil {
		t.Errorf("session = %+v, want nil", session)
	}
	if nowCalls != 0 {
		t.Errorf("now calls = %d, want 0 when ID generation fails", nowCalls)
	}
}

func TestNewSessionFactory_DefaultsProduceHexID(t *testing.T) {
	// Arrange
	const ttl = 24 * time.Hour
	factory := NewSessionFactory(ttl)

	// Act
	before := time.Now()
	session, err := factory.NewSession("user-1")
	after := time.Now()

	// Assert
	if err != nil {
		t.Fatalf("NewSession returned error: %v", err)
	}
	if !sessionIDHexPattern.MatchString(session.ID) {
		t.Errorf("session.ID = %q, want 64-character lowercase hex", session.ID)
	}
	if session.CreatedAt.Before(before) || session.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, want within [%v, %v]", session.CreatedAt, before, after)
	}
	if !session.ExpiresAt.Equal(session.CreatedAt.Add(ttl)) {
		t.Errorf("ExpiresAt = %v, want %v", session.ExpiresAt, session.CreatedAt.Add(ttl))
	}
}
