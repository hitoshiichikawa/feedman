package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// --- SessionCreator 用の record-and-return モック ---
//
// mockAuthCodes は同 package の token_service_test.go で定義済みのものをそのまま
// 再利用する（CLAUDE.md §4「共有ヘルパーの再利用」）。SessionCreator は本 test file
// で最小限のモックを新規追加する。
type mockSessionCreator struct {
	createFn func(ctx context.Context, s *model.Session) error

	createCalls int
	created     *model.Session
}

func (m *mockSessionCreator) Create(ctx context.Context, s *model.Session) error {
	m.createCalls++
	m.created = s
	if m.createFn != nil {
		return m.createFn(ctx, s)
	}
	return nil
}

// --- テスト用ヘルパー ---

const (
	// sessionExchangeTestTTL は SessionExchangeService に注入する任意の TTL。
	// 現行 SessionMaxAge（7 日）と重ならない値を選ぶことで、テスト内の
	// ExpiresAt 検証が確かに sessionTTL 依存であることを明示する。
	sessionExchangeTestTTL = 8 * 24 * time.Hour
)

// fixedSessionExchangeIssuedAt は ExchangeAuthCodeForSession 内で now を固定する基準時刻。
// token_service_test.go の fixedTokenServiceIssuedAt とは別変数で持つことで、
// 万一片方の値が変わっても他方に影響しない。
var fixedSessionExchangeIssuedAt = time.Date(2026, 7, 27, 12, 34, 56, 0, time.UTC)

// sessionIDHexPattern は generateSessionID の出力形式（32 バイト → hex = 64 文字 / lowercase）。
var sessionIDHexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// newSessionExchangeAuthCode は FindByHash の戻り値として使う AuthCode の fixture。
// PKCEChallenge には既存 pkce_test.go の RFC 7636 Appendix B test vector に対応する
// challenge を設定し、testVerifier（= rfc7636AppendixBVerifier）と一致させる。
func newSessionExchangeAuthCode(userID string) *model.AuthCode {
	return &model.AuthCode{
		ID:            "session-exchange-auth-code-id-1",
		CodeHash:      HashNativeSecret("plain-session-auth-code"),
		UserID:        userID,
		PKCEChallenge: validS256Challenge,
		ExpiresAt:     fixedSessionExchangeIssuedAt.Add(30 * time.Second),
		Used:          false,
	}
}

// newSessionExchangeSvcWithMocks は固定 now + 固定 sessionTTL の
// SessionExchangeService と 2 つの mock 参照を返す。
//
// Issue #231 §Delta 1: session 構築は共有 SessionFactory に委譲される。テストは
// 固定 now + 既定 newID（generateSessionID）を持つ factory を in-package で直接構築して
// 注入し、生成 session の ID（64 文字 hex）/ CreatedAt=now / ExpiresAt=now+TTL の整合を維持する。
func newSessionExchangeSvcWithMocks(t *testing.T) (*SessionExchangeService, *mockAuthCodes, *mockSessionCreator) {
	t.Helper()
	authCodes := &mockAuthCodes{}
	sessions := &mockSessionCreator{}
	factory := &SessionFactory{
		ttl:   sessionExchangeTestTTL,
		now:   func() time.Time { return fixedSessionExchangeIssuedAt },
		newID: generateSessionID,
	}
	svc := NewSessionExchangeService(authCodes, sessions, factory)
	return svc, authCodes, sessions
}

// fakeSessionFactory は SessionExchangeService が session 構築を factory に委譲することを
// 検証するための record-and-return モック（Issue #231 §Delta 1）。err を設定すると
// NewSession が失敗し、factory 失敗時に session を永続化しないことを検証できる。
type fakeSessionFactory struct {
	session    *model.Session
	err        error
	calls      int
	lastUserID string
}

func (f *fakeSessionFactory) NewSession(userID string) (*model.Session, error) {
	f.calls++
	f.lastUserID = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

// --- 正常系 ---

// TestExchangeAuthCodeForSession_Success は交換成功時に以下を検証する:
//   - 戻り値 Session の UserID / ExpiresAt / CreatedAt / ID が仕様どおり
//   - session ID は 32 バイト crypto random の hex（64 文字 lowercase）
//   - FindByHash が plain auth_code の SHA-256 hex hash で 1 回呼ばれる
//   - MarkUsed が stored.ID で 1 回呼ばれる
//   - sessions.Create が組み立てた session で 1 回呼ばれる
func TestExchangeAuthCodeForSession_Success(t *testing.T) {
	// Arrange
	const userID = "session-user-1"
	svc, authCodes, sessions := newSessionExchangeSvcWithMocks(t)
	stored := newSessionExchangeAuthCode(userID)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}

	// Act
	session, err := svc.ExchangeAuthCodeForSession(context.Background(),
		"plain-session-auth-code", rfc7636AppendixBVerifier)

	// Assert: 成功時に非 nil の Session が返る
	if err != nil {
		t.Fatalf("ExchangeAuthCodeForSession returned error: %v", err)
	}
	if session == nil {
		t.Fatal("returned Session is nil")
	}
	if session.UserID != userID {
		t.Errorf("session.UserID = %q, want %q", session.UserID, userID)
	}

	// Assert: session ID は 32 バイト crypto random の hex（64 文字 lowercase）
	if !sessionIDHexPattern.MatchString(session.ID) {
		t.Errorf("session.ID = %q, want 64-char lowercase hex (32 bytes)", session.ID)
	}

	// Assert: ExpiresAt = now + sessionTTL、CreatedAt = now（固定 now / 固定 TTL）
	wantExpiresAt := fixedSessionExchangeIssuedAt.Add(sessionExchangeTestTTL)
	if !session.ExpiresAt.Equal(wantExpiresAt) {
		t.Errorf("session.ExpiresAt = %v, want %v (= now + sessionTTL)",
			session.ExpiresAt, wantExpiresAt)
	}
	if !session.CreatedAt.Equal(fixedSessionExchangeIssuedAt) {
		t.Errorf("session.CreatedAt = %v, want %v (= now)",
			session.CreatedAt, fixedSessionExchangeIssuedAt)
	}

	// Assert: FindByHash が plain auth_code の hash で 1 回呼ばれる
	wantCodeHash := HashNativeSecret("plain-session-auth-code")
	if authCodes.findCalls != 1 {
		t.Errorf("FindByHash calls = %d, want 1", authCodes.findCalls)
	}
	if authCodes.lastFindHash != wantCodeHash {
		t.Errorf("FindByHash called with hash=%q, want %q (hash 一致照合)",
			authCodes.lastFindHash, wantCodeHash)
	}

	// Assert: MarkUsed が stored.ID で 1 回呼ばれる
	if authCodes.markUsedCalls != 1 {
		t.Errorf("MarkUsed calls = %d, want 1", authCodes.markUsedCalls)
	}
	if authCodes.lastMarkID != stored.ID {
		t.Errorf("MarkUsed called with id=%q, want %q", authCodes.lastMarkID, stored.ID)
	}

	// Assert: sessions.Create が 1 回呼ばれ、渡された session が戻り値と同一
	if sessions.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1", sessions.createCalls)
	}
	if sessions.created != session {
		t.Errorf("Create was not called with the returned session pointer")
	}
	// 実際に永続化された session の各フィールドも同一値
	if sessions.created.UserID != userID {
		t.Errorf("created session.UserID = %q, want %q", sessions.created.UserID, userID)
	}
	if sessions.created.ID != session.ID {
		t.Errorf("created session.ID = %q, want %q", sessions.created.ID, session.ID)
	}
	if !sessions.created.ExpiresAt.Equal(wantExpiresAt) {
		t.Errorf("created session.ExpiresAt = %v, want %v",
			sessions.created.ExpiresAt, wantExpiresAt)
	}
}

// --- 異常系: ErrInvalidGrant への uniform 化 ---

// TestExchangeAuthCodeForSession_AuthCodeNotFound は FindByHash が nil を返したとき
// ErrInvalidGrant を返し、MarkUsed / Create に進まないことを検証する（Req 3.1 / 4.2 /
// NFR 1.2: 拒否 uniform 化）。
func TestExchangeAuthCodeForSession_AuthCodeNotFound(t *testing.T) {
	// Arrange
	svc, authCodes, sessions := newSessionExchangeSvcWithMocks(t)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return nil, nil // 不明
	}

	// Act
	session, err := svc.ExchangeAuthCodeForSession(context.Background(),
		"unknown-auth-code", rfc7636AppendixBVerifier)

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant (Req 3.1 / 4.2)", err)
	}
	if session != nil {
		t.Errorf("session = %+v, want nil", session)
	}
	if authCodes.markUsedCalls != 0 {
		t.Errorf("MarkUsed calls = %d, want 0 (未検出時は単回消費しない)", authCodes.markUsedCalls)
	}
	if sessions.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (拒否時は session を永続化しない)",
			sessions.createCalls)
	}
}

// TestExchangeAuthCodeForSession_VerifierMismatch は verifier が stored.PKCEChallenge と
// 一致しないとき ErrInvalidGrant を返し、MarkUsed / Create に進まないことを検証する
// （Req 3.1 / 4.2 / NFR 1.2）。
func TestExchangeAuthCodeForSession_VerifierMismatch(t *testing.T) {
	// Arrange
	svc, authCodes, sessions := newSessionExchangeSvcWithMocks(t)
	stored := newSessionExchangeAuthCode("session-user-1")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}

	// 形式は正しいが派生 challenge が stored.PKCEChallenge と一致しない別 verifier
	// （43 文字 unreserved）。
	wrongVerifier := strings.Repeat("a", 43)

	// Act
	session, err := svc.ExchangeAuthCodeForSession(context.Background(),
		"plain-session-auth-code", wrongVerifier)

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant (Req 3.1 / 4.2)", err)
	}
	if session != nil {
		t.Errorf("session = %+v, want nil", session)
	}
	// PKCE 不一致は MarkUsed に進む前に拒否する（設計上手順 3 未到達）
	if authCodes.markUsedCalls != 0 {
		t.Errorf("MarkUsed calls = %d, want 0 (verifier 不一致時は単回消費しない)",
			authCodes.markUsedCalls)
	}
	if sessions.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0", sessions.createCalls)
	}
}

// TestExchangeAuthCodeForSession_MarkUsedNotUsable は MarkUsed が ErrAuthCodeNotUsable を
// 返したとき（既に used / 期限切れ / 消失）ErrInvalidGrant を返し、Create に進まない
// ことを検証する（Req 3.1 / 4.2 / NFR 1.2）。
func TestExchangeAuthCodeForSession_MarkUsedNotUsable(t *testing.T) {
	// Arrange
	svc, authCodes, sessions := newSessionExchangeSvcWithMocks(t)
	stored := newSessionExchangeAuthCode("session-user-1")
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return stored, nil
	}
	authCodes.markUsedFn = func(ctx context.Context, id string) error {
		return repository.ErrAuthCodeNotUsable
	}

	// Act
	session, err := svc.ExchangeAuthCodeForSession(context.Background(),
		"plain-session-auth-code", rfc7636AppendixBVerifier)

	// Assert
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, want ErrInvalidGrant (Req 3.1 / 4.2)", err)
	}
	if session != nil {
		t.Errorf("session = %+v, want nil", session)
	}
	// MarkUsed 自体は呼ばれている（race-safe 判定を repository に委譲しているため）
	if authCodes.markUsedCalls != 1 {
		t.Errorf("MarkUsed calls = %d, want 1", authCodes.markUsedCalls)
	}
	// 拒否時に session を一切永続化しない（設計不変条件）
	if sessions.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (MarkUsed 失敗時は session を永続化しない)",
			sessions.createCalls)
	}
}

// --- 異常系: infra エラーの wrap（ErrInvalidGrant に正規化しない） ---

// TestExchangeAuthCodeForSession_InfraErrorsWrap は各層の infra エラー
// （FindByHash / MarkUsed 非-ErrAuthCodeNotUsable / sessions.Create）が ErrInvalidGrant に
// 正規化されず、原因 error を wrap した非 nil error として上層に伝播することを検証する。
// handler 層は本条件で 500 INTERNAL_ERROR に振り分ける（design.md §Error Handling）。
func TestExchangeAuthCodeForSession_InfraErrorsWrap(t *testing.T) {
	wantErr := errors.New("db unavailable")

	cases := []struct {
		name              string
		setup             func(m *mockAuthCodes, s *mockSessionCreator)
		wantMarkUsedCalls int
		wantCreateCalls   int
	}{
		{
			name: "FindByHash の infra error はそのまま wrap される",
			setup: func(m *mockAuthCodes, s *mockSessionCreator) {
				m.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
					return nil, wantErr
				}
			},
			wantMarkUsedCalls: 0,
			wantCreateCalls:   0,
		},
		{
			name: "MarkUsed の非-ErrAuthCodeNotUsable な infra error は wrap される",
			setup: func(m *mockAuthCodes, s *mockSessionCreator) {
				m.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
					return newSessionExchangeAuthCode("session-user-1"), nil
				}
				m.markUsedFn = func(ctx context.Context, id string) error {
					return wantErr
				}
			},
			wantMarkUsedCalls: 1,
			wantCreateCalls:   0,
		},
		{
			name: "sessions.Create の infra error は wrap される",
			setup: func(m *mockAuthCodes, s *mockSessionCreator) {
				m.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
					return newSessionExchangeAuthCode("session-user-1"), nil
				}
				s.createFn = func(ctx context.Context, sess *model.Session) error {
					return wantErr
				}
			},
			wantMarkUsedCalls: 1,
			wantCreateCalls:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc, authCodes, sessions := newSessionExchangeSvcWithMocks(t)
			tc.setup(authCodes, sessions)

			// Act
			session, err := svc.ExchangeAuthCodeForSession(context.Background(),
				"plain-session-auth-code", rfc7636AppendixBVerifier)

			// Assert: 非 nil error / non-ErrInvalidGrant / wantErr を wrap
			if err == nil {
				t.Fatal("err = nil, want non-nil (infra エラーは 500 として上層に渡す)")
			}
			if errors.Is(err, ErrInvalidGrant) {
				t.Errorf("err = %v, must not be ErrInvalidGrant (infra エラーは 500 系)", err)
			}
			if !errors.Is(err, wantErr) {
				t.Errorf("err = %v, want wrap of %v", err, wantErr)
			}
			if session != nil {
				t.Errorf("session = %+v, want nil", session)
			}
			if authCodes.markUsedCalls != tc.wantMarkUsedCalls {
				t.Errorf("MarkUsed calls = %d, want %d",
					authCodes.markUsedCalls, tc.wantMarkUsedCalls)
			}
			if sessions.createCalls != tc.wantCreateCalls {
				t.Errorf("Create calls = %d, want %d",
					sessions.createCalls, tc.wantCreateCalls)
			}
		})
	}
}

// TestExchangeAuthCodeForSession_DoesNotLeakPlainSecretsInError は返る error（ErrInvalidGrant /
// infra wrap の両方）のメッセージに平文 authCode / codeVerifier が含まれないことを保証する
// （NFR 1.1）。既存 TokenService.TestExchangeAuthCode_DoesNotLeakPlainSecretsInError と
// 同型の観点で、SessionExchangeService でも同じ非漏出契約を維持する。
func TestExchangeAuthCodeForSession_DoesNotLeakPlainSecretsInError(t *testing.T) {
	const secretCode = "very-secret-session-auth-code-value"
	const secretVerifier = "very-secret-verifier-value-98765432109876543"
	infraErr := fmt.Errorf("connection refused to db")

	cases := []struct {
		name  string
		setup func(m *mockAuthCodes, s *mockSessionCreator)
	}{
		{
			name: "ErrInvalidGrant 経路（FindByHash が nil）",
			setup: func(m *mockAuthCodes, s *mockSessionCreator) {
				m.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
					return nil, nil
				}
			},
		},
		{
			name: "infra wrap 経路（FindByHash の DB エラー）",
			setup: func(m *mockAuthCodes, s *mockSessionCreator) {
				m.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
					return nil, infraErr
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			svc, authCodes, sessions := newSessionExchangeSvcWithMocks(t)
			tc.setup(authCodes, sessions)

			// Act
			_, err := svc.ExchangeAuthCodeForSession(context.Background(),
				secretCode, secretVerifier)

			// Assert
			if err == nil {
				t.Fatal("err = nil, want non-nil for this scenario")
			}
			msg := err.Error()
			if strings.Contains(msg, secretCode) {
				t.Errorf("error message %q contains plain authCode (NFR 1.1)", msg)
			}
			if strings.Contains(msg, secretVerifier) {
				t.Errorf("error message %q contains plain codeVerifier (NFR 1.1)", msg)
			}
		})
	}
}

// --- Issue #231 §Delta 1: 共有 SessionFactory への委譲 ---

// TestExchangeAuthCodeForSession_DelegatesToSessionFactory は session 構築が
// SessionFactory.NewSession(stored.UserID) に委譲され、生成された session が
// そのまま永続化・返却されることを検証する（Issue #231 §Delta 1）。
func TestExchangeAuthCodeForSession_DelegatesToSessionFactory(t *testing.T) {
	// Arrange
	const userID = "session-user-1"
	authCodes := &mockAuthCodes{}
	sessions := &mockSessionCreator{}
	want := &model.Session{
		ID:        "factory-built-session-id",
		UserID:    userID,
		CreatedAt: fixedSessionExchangeIssuedAt,
		ExpiresAt: fixedSessionExchangeIssuedAt.Add(sessionExchangeTestTTL),
	}
	factory := &fakeSessionFactory{session: want}
	svc := NewSessionExchangeService(authCodes, sessions, factory)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return newSessionExchangeAuthCode(userID), nil
	}

	// Act
	session, err := svc.ExchangeAuthCodeForSession(context.Background(),
		"plain-session-auth-code", rfc7636AppendixBVerifier)

	// Assert: factory が stored.UserID で 1 回呼ばれる
	if err != nil {
		t.Fatalf("ExchangeAuthCodeForSession returned error: %v", err)
	}
	if factory.calls != 1 {
		t.Errorf("SessionFactory.NewSession calls = %d, want 1", factory.calls)
	}
	if factory.lastUserID != userID {
		t.Errorf("SessionFactory.NewSession userID = %q, want %q", factory.lastUserID, userID)
	}
	// Assert: factory が返した session がそのまま返却・永続化される
	if session != want {
		t.Errorf("returned session = %+v, want factory-built session %+v", session, want)
	}
	if sessions.created != want {
		t.Errorf("Create was not called with the factory-built session")
	}
}

// TestExchangeAuthCodeForSession_FactoryFailure は SessionFactory.NewSession が失敗した
// とき、ErrInvalidGrant ではなく wrap された非 nil error を返し、session を永続化しない
// ことを検証する（Issue #231 §Delta 1。ID 生成失敗 = rand 失敗は 500 系 infra エラー）。
// MarkUsed は factory 呼び出しより前に成功済みのため 1 回呼ばれる。
func TestExchangeAuthCodeForSession_FactoryFailure(t *testing.T) {
	// Arrange
	authCodes := &mockAuthCodes{}
	sessions := &mockSessionCreator{}
	factoryErr := errors.New("session id generation failed")
	factory := &fakeSessionFactory{err: factoryErr}
	svc := NewSessionExchangeService(authCodes, sessions, factory)
	authCodes.findFn = func(ctx context.Context, hash string) (*model.AuthCode, error) {
		return newSessionExchangeAuthCode("session-user-1"), nil
	}

	// Act
	session, err := svc.ExchangeAuthCodeForSession(context.Background(),
		"plain-session-auth-code", rfc7636AppendixBVerifier)

	// Assert: infra エラーとして wrap（ErrInvalidGrant に正規化しない）
	if err == nil {
		t.Fatal("err = nil, want non-nil (factory 失敗は 500 系 infra エラー)")
	}
	if errors.Is(err, ErrInvalidGrant) {
		t.Errorf("err = %v, must not be ErrInvalidGrant (factory 失敗は infra エラー)", err)
	}
	if !errors.Is(err, factoryErr) {
		t.Errorf("err = %v, want wrap of %v", err, factoryErr)
	}
	if session != nil {
		t.Errorf("session = %+v, want nil", session)
	}
	// MarkUsed は factory 呼び出しより前（手順 3）で成功済み。
	if authCodes.markUsedCalls != 1 {
		t.Errorf("MarkUsed calls = %d, want 1", authCodes.markUsedCalls)
	}
	// factory 失敗時は session を一切永続化しない。
	if sessions.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (factory 失敗時は session を永続化しない)", sessions.createCalls)
	}
}
