package passkey

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// stubChallengeRepo は challengeRepo interface を差し替えるためのテスト用スタブ。
// 各メソッドを関数フィールドで差し込むことで、テストごとに挙動を制御する。
type stubChallengeRepo struct {
	createFn       func(ctx context.Context, ch *model.PasskeyChallenge) error
	findByIDFn     func(ctx context.Context, id string) (*model.PasskeyChallenge, error)
	markConsumedFn func(ctx context.Context, id string) error

	createCalled       int
	findByIDCalled     int
	markConsumedCalled int
	lastCreated        *model.PasskeyChallenge
	lastMarkedID       string
}

func (s *stubChallengeRepo) Create(ctx context.Context, ch *model.PasskeyChallenge) error {
	s.createCalled++
	s.lastCreated = ch
	if s.createFn != nil {
		return s.createFn(ctx, ch)
	}
	// 既定挙動: 実 repository と同様に DB 側デフォルトを想定して ID を設定する。
	if ch.ID == "" {
		ch.ID = "stub-generated-id"
	}
	return nil
}

func (s *stubChallengeRepo) FindByID(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
	s.findByIDCalled++
	if s.findByIDFn != nil {
		return s.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (s *stubChallengeRepo) MarkConsumed(ctx context.Context, id string) error {
	s.markConsumedCalled++
	s.lastMarkedID = id
	if s.markConsumedFn != nil {
		return s.markConsumedFn(ctx, id)
	}
	return nil
}

// fixedNow は決定的な now 関数を生成するヘルパー。
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestChallengeStore(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	ttl := 5 * time.Minute
	rawChallenge := []byte("dGVzdC1jaGFsbGVuZ2UtcmF3LWJ5dGVz") // 適当な base64url 風
	expectedHash := auth.HashNativeSecret(string(rawChallenge))

	t.Run("Issue で作成した challenge を Consume で成功裏に消費できる", func(t *testing.T) {
		// Arrange
		repo := &stubChallengeRepo{
			createFn: func(ctx context.Context, ch *model.PasskeyChallenge) error {
				ch.ID = "issued-id"
				return nil
			},
			findByIDFn: func(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
				// Create 済みの状態を模擬。
				return &model.PasskeyChallenge{
					ID:            id,
					ChallengeHash: expectedHash,
					Kind:          model.PasskeyChallengeKindRegistrationNew,
					SessionData:   []byte(`{"challenge":"x"}`),
					ExpiresAt:     baseTime.Add(ttl),
				}, nil
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)
		pending := "alice"

		// Act
		id, err := store.Issue(ctx, model.PasskeyChallengeKindRegistrationNew,
			nil, &pending, []byte(`{"challenge":"x"}`), rawChallenge)
		if err != nil {
			t.Fatalf("Issue: unexpected error: %v", err)
		}
		consumed, err := store.Consume(ctx, id, model.PasskeyChallengeKindRegistrationNew)

		// Assert
		if err != nil {
			t.Fatalf("Consume: unexpected error: %v", err)
		}
		if consumed == nil || consumed.ID != id {
			t.Fatalf("Consume: expected challenge id %q, got %#v", id, consumed)
		}
		if repo.lastCreated.ChallengeHash != expectedHash {
			t.Errorf("Create: expected hash %q, got %q", expectedHash, repo.lastCreated.ChallengeHash)
		}
		if !repo.lastCreated.ExpiresAt.Equal(baseTime.Add(ttl)) {
			t.Errorf("Create: expected ExpiresAt %v, got %v", baseTime.Add(ttl), repo.lastCreated.ExpiresAt)
		}
		if repo.lastCreated.PendingUsername == nil || *repo.lastCreated.PendingUsername != pending {
			t.Errorf("Create: expected PendingUsername %q, got %v", pending, repo.lastCreated.PendingUsername)
		}
		if repo.markConsumedCalled != 1 {
			t.Errorf("MarkConsumed called %d times, expected 1", repo.markConsumedCalled)
		}
		if repo.lastMarkedID != id {
			t.Errorf("MarkConsumed called with %q, expected %q", repo.lastMarkedID, id)
		}
	})

	t.Run("期限切れ challenge の Consume は ErrChallengeNotUsable を返す", func(t *testing.T) {
		// Arrange
		repo := &stubChallengeRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
				return &model.PasskeyChallenge{
					ID:        id,
					Kind:      model.PasskeyChallengeKindAuthentication,
					ExpiresAt: baseTime.Add(-time.Second), // 過去 = 期限切れ
				}, nil
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act
		_, err := store.Consume(ctx, "expired-id", model.PasskeyChallengeKindAuthentication)

		// Assert
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Fatalf("expected ErrChallengeNotUsable, got %v", err)
		}
		if repo.markConsumedCalled != 0 {
			t.Errorf("MarkConsumed must not be called for expired challenge, got %d", repo.markConsumedCalled)
		}
	})

	t.Run("kind 不一致は MarkConsumed を呼ばずに ErrChallengeNotUsable を返す", func(t *testing.T) {
		// Arrange
		repo := &stubChallengeRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
				return &model.PasskeyChallenge{
					ID:        id,
					Kind:      model.PasskeyChallengeKindAuthentication, // stored kind
					ExpiresAt: baseTime.Add(ttl),
				}, nil
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act — 呼び出しは registration_new を期待するが stored は authentication
		_, err := store.Consume(ctx, "mismatched-id", model.PasskeyChallengeKindRegistrationNew)

		// Assert
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Fatalf("expected ErrChallengeNotUsable, got %v", err)
		}
		if repo.markConsumedCalled != 0 {
			t.Errorf("MarkConsumed must not be called on kind mismatch, got %d", repo.markConsumedCalled)
		}
	})

	t.Run("未存在 challenge の Consume は ErrChallengeNotUsable を返す", func(t *testing.T) {
		// Arrange — repo.FindByID が (nil, nil) を返す既定挙動を使う
		repo := &stubChallengeRepo{}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act
		_, err := store.Consume(ctx, "missing-id", model.PasskeyChallengeKindAuthentication)

		// Assert
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Fatalf("expected ErrChallengeNotUsable, got %v", err)
		}
		if repo.markConsumedCalled != 0 {
			t.Errorf("MarkConsumed must not be called for missing challenge, got %d", repo.markConsumedCalled)
		}
	})

	t.Run("二重消費（repository.ErrChallengeNotUsable）は passkey 側 sentinel に re-map される", func(t *testing.T) {
		// Arrange
		repo := &stubChallengeRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
				return &model.PasskeyChallenge{
					ID:        id,
					Kind:      model.PasskeyChallengeKindRegistrationAdd,
					ExpiresAt: baseTime.Add(ttl),
					Consumed:  false, // 事前判定は通過させ、atomic UPDATE 側で race を再現
				}, nil
			},
			markConsumedFn: func(ctx context.Context, id string) error {
				// 別プロセスが先に消費した状況を模擬。
				return repository.ErrChallengeNotUsable
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act
		_, err := store.Consume(ctx, "race-id", model.PasskeyChallengeKindRegistrationAdd)

		// Assert
		if !errors.Is(err, ErrChallengeNotUsable) {
			t.Fatalf("expected passkey.ErrChallengeNotUsable, got %v", err)
		}
		// re-map が effective であること（repository sentinel も内包していて構わないが、
		// 呼び出し側は passkey 側 sentinel での errors.Is 判定のみで判別できることを保証する）。
	})

	t.Run("FindByID の infra エラーは ErrChallengeNotUsable に落とさず wrap 済みで返す", func(t *testing.T) {
		// Arrange
		otherErr := errors.New("db connection lost")
		repo := &stubChallengeRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
				return nil, otherErr
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act
		_, err := store.Consume(ctx, "infra-fail-id", model.PasskeyChallengeKindAuthentication)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, ErrChallengeNotUsable) {
			t.Errorf("infra error must not be re-mapped to ErrChallengeNotUsable: %v", err)
		}
		if !errors.Is(err, otherErr) {
			t.Errorf("expected wrapped %v, got %v", otherErr, err)
		}
	})

	t.Run("MarkConsumed の infra エラーは ErrChallengeNotUsable に落とさず wrap 済みで返す", func(t *testing.T) {
		// Arrange
		otherErr := errors.New("db deadlock")
		repo := &stubChallengeRepo{
			findByIDFn: func(ctx context.Context, id string) (*model.PasskeyChallenge, error) {
				return &model.PasskeyChallenge{
					ID:        id,
					Kind:      model.PasskeyChallengeKindAuthentication,
					ExpiresAt: baseTime.Add(ttl),
				}, nil
			},
			markConsumedFn: func(ctx context.Context, id string) error {
				return otherErr
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act
		_, err := store.Consume(ctx, "mark-infra-fail-id", model.PasskeyChallengeKindAuthentication)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, ErrChallengeNotUsable) {
			t.Errorf("infra error must not be re-mapped to ErrChallengeNotUsable: %v", err)
		}
		if !errors.Is(err, otherErr) {
			t.Errorf("expected wrapped %v, got %v", otherErr, err)
		}
	})

	t.Run("Issue の repository エラーは wrap 済みで返す", func(t *testing.T) {
		// Arrange
		otherErr := errors.New("db unavailable")
		repo := &stubChallengeRepo{
			createFn: func(ctx context.Context, ch *model.PasskeyChallenge) error {
				return otherErr
			},
		}
		store := NewChallengeStore(repo, ttl)
		store.now = fixedNow(baseTime)

		// Act
		_, err := store.Issue(ctx, model.PasskeyChallengeKindRegistrationNew,
			nil, nil, []byte(`{}`), rawChallenge)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, otherErr) {
			t.Errorf("expected wrapped %v, got %v", otherErr, err)
		}
	})
}
