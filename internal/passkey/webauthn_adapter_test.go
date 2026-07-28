package passkey

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/descope/virtualwebauthn"
	"github.com/go-webauthn/webauthn/webauthn"
)

// 本 test は仮想 authenticator（github.com/descope/virtualwebauthn）を用いて
// register → login の round trip・counter 後退・不正 body・challenge entropy を
// 外部ネットワーク非依存で検証する（NFR 4.1）。
//
// virtualwebauthn は本番 binary には含まれない test-only 依存で、design.md の
// Technology Stack には記載外（設計側は go-webauthn のみを列挙）だが、round trip を
// 小さく確実に組むための test-only 追加として impl-notes.md に記録する。

// testRP は virtualwebauthn 側の Relying Party 設定。
// Adapter 側の RP 設定と origin / RPID が一致する必要がある。
var testRP = virtualwebauthn.RelyingParty{
	Name:   "Feedman",
	ID:     "example.com",
	Origin: "https://example.com",
}

// testAdapterOrigins は上記 testRP.Origin と一致させる。
var testAdapterOrigins = []string{"https://example.com"}

// testUser は WebAuthnUser を実装する最小のテスト用構造体。
// WebAuthnCredentials() は register 直後は空、login 時は
// 登録済み credential 1 件を返すよう調整する。
type testUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *testUser) WebAuthnID() []byte                         { return u.id }
func (u *testUser) WebAuthnName() string                       { return u.name }
func (u *testUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *testUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// newTestAdapter は毎テストで新しい GoWebAuthnAdapter を返すヘルパー。
func newTestAdapter(t *testing.T) *GoWebAuthnAdapter {
	t.Helper()
	adapter, err := NewGoWebAuthnAdapter(testRP.ID, testRP.Name, testAdapterOrigins)
	if err != nil {
		t.Fatalf("NewGoWebAuthnAdapter: %v", err)
	}
	return adapter
}

func TestNewGoWebAuthnAdapter_RejectsEmptyOrigins(t *testing.T) {
	// Arrange & Act — RPOrigins 空だと webauthn.New が error を返す。
	_, err := NewGoWebAuthnAdapter("example.com", "Feedman", nil)

	// Assert
	if err == nil {
		t.Fatal("expected error for empty origins, got nil")
	}
}

func TestWebAuthnAdapter_RegisterAndLoginRoundTrip(t *testing.T) {
	// Arrange
	adapter := newTestAdapter(t)
	user := &testUser{
		id:          []byte("user-handle-alice"),
		name:        "alice",
		displayName: "Alice",
	}
	authenticator := virtualwebauthn.NewAuthenticator()
	credEC2 := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	// === Registration ceremony ===
	optionsJSON, sessionData, rawChallenge, err := adapter.BeginRegistration(user, nil)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	if len(rawChallenge) == 0 {
		t.Fatal("BeginRegistration: rawChallenge is empty")
	}
	attOpts, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	// Ensure that the mock credential isn't excluded by the attestation options
	if credEC2.IsExcludedForAttestation(*attOpts) {
		t.Fatal("credential should not be excluded (initial registration)")
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(testRP, authenticator, credEC2, *attOpts)

	// Act — FinishRegistration
	parsed, err := adapter.FinishRegistration(user, sessionData, []byte(attestationResp))
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}

	// Assert (registration)
	if parsed == nil {
		t.Fatal("FinishRegistration: parsed credential is nil")
	}
	if len(parsed.ID) == 0 {
		t.Error("FinishRegistration: parsed.ID is empty")
	}
	if len(parsed.PublicKey) == 0 {
		t.Error("FinishRegistration: parsed.PublicKey is empty")
	}
	// Issue #234 task 3: default synthetic authenticator は BackupEligible=false /
	// BackupState=false を報告するため、parsed 側にも false が propagate されることを確認する
	// （Req 1.4 / 2.3 の BE=0 baseline）。
	if parsed.BackupEligible {
		t.Errorf("parsed.BackupEligible = true, want false (default synthetic authenticator reports BE=0)")
	}
	if parsed.BackupState {
		t.Errorf("parsed.BackupState = true, want false (default synthetic authenticator reports BS=0)")
	}

	// === Login ceremony (discoverable) ===
	// Set the user handle on the mock authenticator so it echos it in the assertion.
	authenticator.Options.UserHandle = user.WebAuthnID()
	// Increment the credential counter to be > registered sign count so login succeeds cleanly.
	credEC2.Counter = parsed.SignCount + 1
	authenticator.AddCredential(credEC2)

	loginOptions, loginSession, loginRawChallenge, err := adapter.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	if len(loginRawChallenge) == 0 {
		t.Fatal("BeginLogin: rawChallenge is empty")
	}
	assOpts, err := virtualwebauthn.ParseAssertionOptions(string(loginOptions))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(testRP, authenticator, credEC2, *assOpts)

	// Reconstruct a webauthn.Credential for the login user so ValidateDiscoverableLogin
	// can look it up and verify the signature. Issue #234: Flags には stored BE/BS を
	// 反映する（本ケースでは BE=0/BS=0 なので既存挙動と等価だが、library の BE 一致判定
	// [login.go:371] を通す canonical 経路として明示的に設定する）。
	loginUser := &testUser{
		id:          user.WebAuthnID(),
		name:        user.name,
		displayName: user.displayName,
		creds: []webauthn.Credential{
			{
				ID:        parsed.ID,
				PublicKey: parsed.PublicKey,
				Flags: webauthn.CredentialFlags{
					BackupEligible: parsed.BackupEligible,
					BackupState:    parsed.BackupState,
				},
				Authenticator: webauthn.Authenticator{
					AAGUID:    parsed.AAGUID,
					SignCount: parsed.SignCount,
				},
			},
		},
	}
	lookup := func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error) {
		// Sanity: the library must pass the same credential ID.
		if !bytesEqual(credentialID, parsed.ID) {
			return nil, nil, errors.New("credential id mismatch")
		}
		return loginUser, parsed, nil
	}

	// Act — FinishLogin
	userHandle, credentialID, updatedSignCount, updatedBackupState, err := adapter.FinishLogin(loginSession, []byte(assertionResp), lookup)
	if err != nil {
		t.Fatalf("FinishLogin: %v", err)
	}

	// Assert (login)
	if !bytesEqual(userHandle, user.WebAuthnID()) {
		t.Errorf("userHandle mismatch: got %q, want %q", userHandle, user.WebAuthnID())
	}
	if !bytesEqual(credentialID, parsed.ID) {
		t.Errorf("credentialID mismatch: got %x, want %x", credentialID, parsed.ID)
	}
	if updatedSignCount != credEC2.Counter {
		t.Errorf("updatedSignCount: got %d, want %d", updatedSignCount, credEC2.Counter)
	}
	// Issue #234 task 3 / Req 4.2: BE=0/BS=0 default synthetic authenticator に対しては
	// updatedBackupState=false が返る（library が assertion から更新した Flags.BackupState を反映）。
	if updatedBackupState {
		t.Errorf("updatedBackupState = true, want false (BE=0/BS=0 default authenticator)")
	}
}

// TestWebAuthnAdapter_RegisterAndLoginRoundTrip_BackupEligible は Issue #234 の中核 regression。
//
// BackupEligible=1 / BackupState=1 を報告する synthetic authenticator（virtualwebauthn
// v1.0.5 の AuthenticatorOptions.BackupEligible/BackupState）で登録 → 認証を通し、
// 以下を検証する:
//   - 登録時に parsed.BackupEligible=true / parsed.BackupState=true が propagate される (Req 1.1, 1.3)
//   - login の lookup で返す webauthn.Credential.Flags に stored BE/BS を反映すれば
//     library の BE 一致判定 (login.go:371) を通過し、認証が成功する (Req 2.1, 2.2, 3.1)
//   - FinishLogin の戻り値 updatedBackupState=true が assertion 由来の最新 BS を反映する (Req 4.2)
//
// このテストが green にならないと、実ブラウザで登録された同期パスキー（BE=1）は library
// の "Backup Eligible flag inconsistency detected" で必ず reject されるため、本 spec の
// 修正が意味を持つ CI 保険となる。
func TestWebAuthnAdapter_RegisterAndLoginRoundTrip_BackupEligible(t *testing.T) {
	// Arrange — BE=1/BS=1 authenticator を用いる
	adapter := newTestAdapter(t)
	user := &testUser{
		id:          []byte("user-handle-bob"),
		name:        "bob",
		displayName: "Bob",
	}
	authenticator := virtualwebauthn.NewAuthenticator()
	authenticator.Options.BackupEligible = true
	authenticator.Options.BackupState = true
	credEC2 := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	// === Registration ceremony ===
	optionsJSON, sessionData, _, err := adapter.BeginRegistration(user, nil)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	attOpts, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(testRP, authenticator, credEC2, *attOpts)

	// Act — FinishRegistration
	parsed, err := adapter.FinishRegistration(user, sessionData, []byte(attestationResp))
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}

	// Assert (registration) — BE=1/BS=1 が parsed に propagate される (Req 1.1, 1.3)
	if !parsed.BackupEligible {
		t.Errorf("parsed.BackupEligible = false, want true (authenticator reports BE=1)")
	}
	if !parsed.BackupState {
		t.Errorf("parsed.BackupState = false, want true (authenticator reports BS=1)")
	}

	// === Login ceremony (discoverable) ===
	authenticator.Options.UserHandle = user.WebAuthnID()
	credEC2.Counter = parsed.SignCount + 1
	authenticator.AddCredential(credEC2)

	loginOptions, loginSession, _, err := adapter.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	assOpts, err := virtualwebauthn.ParseAssertionOptions(string(loginOptions))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(testRP, authenticator, credEC2, *assOpts)

	// 核心: lookup で返す webauthn.Credential.Flags に stored BE/BS を **必ず** 反映する。
	// これを設定しないと library の BE 一致判定 (login.go:371 "Backup Eligible flag
	// inconsistency detected") で必ず reject され、認証は成立しない（本 fix の直接原因）。
	loginUser := &testUser{
		id:          user.WebAuthnID(),
		name:        user.name,
		displayName: user.displayName,
		creds: []webauthn.Credential{
			{
				ID:        parsed.ID,
				PublicKey: parsed.PublicKey,
				Flags: webauthn.CredentialFlags{
					BackupEligible: parsed.BackupEligible,
					BackupState:    parsed.BackupState,
				},
				Authenticator: webauthn.Authenticator{
					AAGUID:    parsed.AAGUID,
					SignCount: parsed.SignCount,
				},
			},
		},
	}
	lookup := func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error) {
		if !bytesEqual(credentialID, parsed.ID) {
			return nil, nil, errors.New("credential id mismatch")
		}
		return loginUser, parsed, nil
	}

	// Act — FinishLogin
	userHandle, credentialID, updatedSignCount, updatedBackupState, err := adapter.FinishLogin(loginSession, []byte(assertionResp), lookup)
	if err != nil {
		t.Fatalf("FinishLogin: %v (BE=1 authenticator should authenticate successfully when stored BE/BS is reflected)", err)
	}

	// Assert (login)
	if !bytesEqual(userHandle, user.WebAuthnID()) {
		t.Errorf("userHandle mismatch: got %q, want %q", userHandle, user.WebAuthnID())
	}
	if !bytesEqual(credentialID, parsed.ID) {
		t.Errorf("credentialID mismatch: got %x, want %x", credentialID, parsed.ID)
	}
	if updatedSignCount != credEC2.Counter {
		t.Errorf("updatedSignCount: got %d, want %d", updatedSignCount, credEC2.Counter)
	}
	// Req 4.2: 認証成功時の updatedBackupState は library が assertion から反映した
	// 最新の Flags.BackupState を返す。BE=1/BS=1 authenticator では BS=1 のまま。
	if !updatedBackupState {
		t.Errorf("updatedBackupState = false, want true (BE=1/BS=1 authenticator reports BS=1)")
	}
}

func TestWebAuthnAdapter_FinishLogin_DetectsCounterRegression(t *testing.T) {
	// Arrange — register then set stored SignCount to 10 while presenting Counter=1
	adapter := newTestAdapter(t)
	user := &testUser{
		id:          []byte("user-handle-clone"),
		name:        "clonevictim",
		displayName: "Clone Victim",
	}
	authenticator := virtualwebauthn.NewAuthenticator()
	credEC2 := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	optionsJSON, sessionData, _, err := adapter.BeginRegistration(user, nil)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	attOpts, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(testRP, authenticator, credEC2, *attOpts)
	parsed, err := adapter.FinishRegistration(user, sessionData, []byte(attestationResp))
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	authenticator.Options.UserHandle = user.WebAuthnID()
	authenticator.AddCredential(credEC2)

	// Prepare login with credential.Counter = 1 (regressed) while stored SignCount = 10.
	credEC2.Counter = 1
	loginOptions, loginSession, _, err := adapter.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	assOpts, err := virtualwebauthn.ParseAssertionOptions(string(loginOptions))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(testRP, authenticator, credEC2, *assOpts)

	loginUser := &testUser{
		id:          user.WebAuthnID(),
		name:        user.name,
		displayName: user.displayName,
		creds: []webauthn.Credential{
			{
				ID:        parsed.ID,
				PublicKey: parsed.PublicKey,
				Authenticator: webauthn.Authenticator{
					AAGUID:    parsed.AAGUID,
					SignCount: 10, // 意図的に高い値を stored 側に置く
				},
			},
		},
	}
	lookup := func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error) {
		return loginUser, parsed, nil
	}

	// Act
	_, _, _, _, err = adapter.FinishLogin(loginSession, []byte(assertionResp), lookup)

	// Assert
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestWebAuthnAdapter_FinishRegistration_RejectsMalformedBody(t *testing.T) {
	// Arrange
	adapter := newTestAdapter(t)
	user := &testUser{id: []byte("uid"), name: "n", displayName: "d"}
	_, sessionData, _, err := adapter.BeginRegistration(user, nil)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}

	// Act — 明らかに JSON 不正な body
	_, err = adapter.FinishRegistration(user, sessionData, []byte("not-a-json"))

	// Assert
	if !errors.Is(err, ErrRegistrationFailed) {
		t.Fatalf("expected ErrRegistrationFailed, got %v", err)
	}
}

func TestWebAuthnAdapter_FinishLogin_RejectsMalformedBody(t *testing.T) {
	// Arrange
	adapter := newTestAdapter(t)
	_, sessionData, _, err := adapter.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	lookup := func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error) {
		return nil, nil, errors.New("should not be called")
	}

	// Act
	_, _, _, _, err = adapter.FinishLogin(sessionData, []byte("not-a-json"), lookup)

	// Assert
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestWebAuthnAdapter_FinishRegistration_RejectsEmptySessionData(t *testing.T) {
	// Arrange
	adapter := newTestAdapter(t)
	user := &testUser{id: []byte("uid"), name: "n", displayName: "d"}

	// Act
	_, err := adapter.FinishRegistration(user, nil, []byte(`{"id":"x"}`))

	// Assert
	if !errors.Is(err, ErrRegistrationFailed) {
		t.Fatalf("expected ErrRegistrationFailed, got %v", err)
	}
}

func TestWebAuthnAdapter_FinishLogin_RejectsEmptySessionData(t *testing.T) {
	// Arrange
	adapter := newTestAdapter(t)
	lookup := func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error) {
		return nil, nil, errors.New("should not be called")
	}

	// Act
	_, _, _, _, err := adapter.FinishLogin(nil, []byte(`{"id":"x"}`), lookup)

	// Assert
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
	}
}

// TestWebAuthnAdapter_ChallengeEntropy verifies that library-generated challenges
// carry >= 16 bytes (128 bit) of entropy in both Begin ceremonies (Req 4.5).
func TestWebAuthnAdapter_ChallengeEntropy(t *testing.T) {
	adapter := newTestAdapter(t)
	user := &testUser{id: []byte("uid"), name: "n", displayName: "d"}

	t.Run("BeginRegistration の challenge は 128bit 以上", func(t *testing.T) {
		_, sessionData, _, err := adapter.BeginRegistration(user, nil)
		if err != nil {
			t.Fatalf("BeginRegistration: %v", err)
		}
		var s webauthn.SessionData
		if err := json.Unmarshal(sessionData, &s); err != nil {
			t.Fatalf("Unmarshal SessionData: %v", err)
		}
		// SessionData.Challenge は base64url string。decode 後 byte 長で判定。
		decoded, err := base64.RawURLEncoding.DecodeString(s.Challenge)
		if err != nil {
			t.Fatalf("decode challenge: %v", err)
		}
		if len(decoded) < 16 {
			t.Errorf("register challenge entropy: %d bytes, want >= 16", len(decoded))
		}
	})

	t.Run("BeginLogin の challenge は 128bit 以上", func(t *testing.T) {
		_, sessionData, _, err := adapter.BeginLogin()
		if err != nil {
			t.Fatalf("BeginLogin: %v", err)
		}
		var s webauthn.SessionData
		if err := json.Unmarshal(sessionData, &s); err != nil {
			t.Fatalf("Unmarshal SessionData: %v", err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(s.Challenge)
		if err != nil {
			t.Fatalf("decode challenge: %v", err)
		}
		if len(decoded) < 16 {
			t.Errorf("login challenge entropy: %d bytes, want >= 16", len(decoded))
		}
	})
}

// bytesEqual は bytes.Equal と等価な小さなヘルパー（bytes パッケージへの依存を避けるため）。
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
