package passkey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// 本ファイルは challenge の TTL 付き単回利用永続化を service 層から repository 層へ
// 橋渡しする ChallengeStore を提供する（Issue #216 / design.md ChallengeStore /
// Req 4.1, 4.2, 4.3, 4.4, 4.5）。
//
// NFR 1.2 準拠: 生 challenge / sessionData / user_id 等はログ・エラーメッセージに
// 出さない。ChallengeHash は auth.HashNativeSecret（SHA-256 hex）で生成する。

// challengeRepo は ChallengeStore が依存する最小 interface（interface segregation）。
// repository.PostgresPasskeyChallengeRepo が構造的にこれを充足するため、
// wiring 時は具体型をそのまま渡せる。テストでは stub 実装を差し込む。
type challengeRepo interface {
	Create(ctx context.Context, ch *model.PasskeyChallenge) error
	FindByID(ctx context.Context, id string) (*model.PasskeyChallenge, error)
	MarkConsumed(ctx context.Context, id string) error
}

// ChallengeStore は WebAuthn ceremony の challenge を TTL 付きで発行・消費する。
//
// Issue（Begin* から返る sessionData / rawChallenge）と Consume（Finish* に渡す
// sessionData の復元）を対で提供し、単回利用性は repository.MarkConsumed の
// atomic UPDATE 判定に委ねる（並行アクセス下の race を防ぐ）。
type ChallengeStore struct {
	repo challengeRepo
	ttl  time.Duration
	now  func() time.Time
}

// NewChallengeStore は challenge repository と TTL を固定した ChallengeStore を生成する。
// 内部の now は time.Now を既定とし、テストからは同一 package 内で差し替え可能。
func NewChallengeStore(repo challengeRepo, ttl time.Duration) *ChallengeStore {
	return &ChallengeStore{
		repo: repo,
		ttl:  ttl,
		now:  time.Now,
	}
}

// Issue は新規 challenge を発行して永続化し、opaque challenge_id を返す
// （Req 4.1 / 4.2 / 4.5）。
//
// rawChallenge は WebAuthnAdapter.Begin* が返す raw challenge byte 列で、
// 本 store 側で HashNativeSecret により hash 化して保存する（NFR 1.2: 平文非保存）。
// sessionData は Finish* で復元される go-webauthn.SessionData を JSON marshal した
// もの。userID / pendingUsername は challenge kind に応じて呼び出し側が nil か
// 実値を渡す（registration_new: pendingUsername / registration_add: userID /
// authentication: どちらも nil）。
//
// challenge の有効期限は now() + ttl とし、単回利用性は Consume 側で保証する。
func (s *ChallengeStore) Issue(
	ctx context.Context,
	kind model.PasskeyChallengeKind,
	userID *string,
	pendingUsername *string,
	sessionData []byte,
	rawChallenge []byte,
) (string, error) {
	ch := &model.PasskeyChallenge{
		ChallengeHash:   auth.HashNativeSecret(string(rawChallenge)),
		Kind:            kind,
		UserID:          userID,
		PendingUsername: pendingUsername,
		SessionData:     sessionData,
		ExpiresAt:       s.now().Add(s.ttl),
	}
	if err := s.repo.Create(ctx, ch); err != nil {
		// NFR 1.2: challenge_hash / sessionData / user_id 等をメッセージに含めない。
		return "", fmt.Errorf("failed to issue passkey challenge: %w", err)
	}
	return ch.ID, nil
}

// Consume は opaque challenge_id で challenge を逆引きし、TTL / 単回 / kind の
// 一致を検証したうえで sessionData を含む PasskeyChallenge を返す（Req 4.3 / 4.4）。
//
// 以下のいずれかに該当する場合は ErrChallengeNotUsable を返す（uniform 拒否）:
//   - 未存在 / repository.FindByID が (nil, nil) を返した場合
//   - kind が expectedKind と一致しない場合（MarkConsumed を呼ばず先行拒否）
//   - now() > ExpiresAt（期限切れ）または既に Consumed = true（先行拒否）
//   - MarkConsumed が repository.ErrChallengeNotUsable を返した場合（並行 race）
//
// 上記以外の infra エラー（DB 障害等）は wrap して返す。呼び出し側は
// errors.Is(err, ErrChallengeNotUsable) で拒否を判定できる。
func (s *ChallengeStore) Consume(
	ctx context.Context,
	challengeID string,
	expectedKind model.PasskeyChallengeKind,
) (*model.PasskeyChallenge, error) {
	ch, err := s.repo.FindByID(ctx, challengeID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup passkey challenge: %w", err)
	}
	if ch == nil {
		return nil, ErrChallengeNotUsable
	}
	if ch.Kind != expectedKind {
		return nil, ErrChallengeNotUsable
	}
	// 早期拒否: MarkConsumed の atomic 判定が最終権威だが、事前判定でも
	// uniform に拒否する（NFR 1.2: 期限切れ / consumed の区別を上位へ伝えない）。
	if ch.Consumed || !s.now().Before(ch.ExpiresAt) {
		return nil, ErrChallengeNotUsable
	}
	if err := s.repo.MarkConsumed(ctx, challengeID); err != nil {
		// repository 層 sentinel は passkey 側 sentinel に re-map する
		// （task 2 impl-notes 参照。二層で個別に errors.Is する取りこぼしを防ぐ）。
		if errors.Is(err, repository.ErrChallengeNotUsable) {
			return nil, ErrChallengeNotUsable
		}
		return nil, fmt.Errorf("failed to consume passkey challenge: %w", err)
	}
	return ch, nil
}
