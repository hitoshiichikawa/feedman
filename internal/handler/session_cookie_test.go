package handler

import (
	"net/http"
	"testing"
)

func TestBuildSessionCookie_UsesCanonicalAttributes(t *testing.T) {
	// Arrange / Act
	cookie := buildSessionCookie(
		sessionCookieName,
		"session-value",
		"example.com",
		true,
		86400,
	)

	// Assert
	if cookie.Name != sessionCookieName || cookie.Value != "session-value" {
		t.Errorf("Name/Value = %q/%q, want %q/session-value",
			cookie.Name, cookie.Value, sessionCookieName)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want /", cookie.Path)
	}
	if cookie.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", cookie.Domain)
	}
	if cookie.MaxAge != 86400 {
		t.Errorf("MaxAge = %d, want 86400", cookie.MaxAge)
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly = false, want true")
	}
	if !cookie.Secure {
		t.Error("Secure = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
	}
}
