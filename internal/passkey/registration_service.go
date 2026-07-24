package passkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/model"
	"github.com/hitoshi/feedman/internal/repository"
)

// 本ファイルは新規登録（未認証）と追加登録（認証済み）の WebAuthn ceremony を担う
// RegistrationService を提供する（Issue #216 / design.md L559-633 /
// Req 1.1〜1.7, 3.1〜3.7）。
//
// 拒否は理由詳細を反射せず ErrRegistrationFailed / ErrUsernameTaken /
// ErrInvalidUsername に uniform 化する（Req 1.7 / 3.6 / 3.7 / NFR 1.3）。
// 平文 attestation / requestBody / challenge / username 生値をログ・エラー
// メッセージ・レスポンスに出さない（NFR 1.2 / 3.2）。

// UserWriter は RegistrationService が users 表アクセスに必要とする最小 interface である
// （interface segregation / CLAUDE.md §5）。
//
// repository.PostgresUserRepo は構造的にこれを充足するため、wiring 時に具体型を
// そのまま渡せる。テストでは stub 実装を差し込む。
type UserWriter interface {
	// FindByNormalizedUsername は正規化済みユーザー名（lowercase）で user を検索する
	// （Req 1.4 の begin 時 pre-check）。未存在は (nil, nil)。
	FindByNormalizedUsername(ctx context.Context, normalized string) (*model.User, error)

	// CreateUserOnly は identity を持たないユーザー行のみを INSERT する（Req 1.2 / 1.6）。
	// username_normalized の UNIQUE 制約違反時は repository.ErrUsernameTaken を返す（race 防衛）。
	CreateUserOnly(ctx context.Context, u *model.User) error

	// FindByID は追加登録の begin 時に既存 user を取得する（Req 3.1）。
	// 未存在は (nil, nil)。
	FindByID(ctx context.Context, id string) (*model.User, error)
}

// PasskeyCredentialWriter は RegistrationService が passkey_credentials 表アクセスに
// 必要とする最小 interface である（interface segregation）。
//
// repository.PostgresPasskeyCredentialRepo は構造的にこれを充足する。
type PasskeyCredentialWriter interface {
	// FindByCredentialID は raw credential_id で credential を検索する
	// （追加登録の Req 3.6 pre-check）。未存在は (nil, nil)。
	FindByCredentialID(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error)

	// ListByUserID は当該 user に紐付く全 credential を返す
	// （追加登録の Req 3.1 excludeCredentials 用途）。
	ListByUserID(ctx context.Context, userID string) ([]*model.PasskeyCredential, error)

	// Create は credential を新規保存する。credential_id UNIQUE 衝突時は
	// repository.ErrCredentialAlreadyRegistered を返す（Req 3.6 / 1.7 の防衛線）。
	Create(ctx context.Context, c *model.PasskeyCredential) error
}

// challengeStore は RegistrationService が challenge lifecycle に必要とする最小
// interface である（interface segregation）。ChallengeStore（本 package）が
// 構造的にこれを充足するため wiring 時に具体型をそのまま渡せる。
type challengeStore interface {
	Issue(ctx context.Context, kind model.PasskeyChallengeKind, userID *string,
		pendingUsername *string, sessionData []byte, rawChallenge []byte) (string, error)
	Consume(ctx context.Context, challengeID string,
		expectedKind model.PasskeyChallengeKind) (*model.PasskeyChallenge, error)
}

// RegistrationService は新規登録（未認証）と追加登録（認証済み）の WebAuthn ceremony
// を担う service である（Req 1.1〜1.7, 3.1〜3.7）。
//
// 依存はすべて最小 interface として宣言し（interface segregation / CLAUDE.md §5）、
// テストでは stub / mock を差し込むことで外部ネットワーク非依存の検証を可能にする（NFR 4.1）。
type RegistrationService struct {
	adapter     WebAuthnAdapter
	challenges  challengeStore
	users       UserWriter
	credentials PasskeyCredentialWriter
	now         func() time.Time
}

// NewRegistrationService は RegistrationService を生成する。
//
// now が nil の場合は time.Now を既定として採用する（テストからは差し替え可能）。
func NewRegistrationService(
	adapter WebAuthnAdapter,
	challenges challengeStore,
	users UserWriter,
	credentials PasskeyCredentialWriter,
	now func() time.Time,
) *RegistrationService {
	if now == nil {
		now = time.Now
	}
	return &RegistrationService{
		adapter:     adapter,
		challenges:  challenges,
		users:       users,
		credentials: credentials,
		now:         now,
	}
}

// registrationUser は WebAuthnUser（= webauthn.User）を実装するための最小構造体である。
// 新規登録では credential 未確定のため WebAuthnCredentials() は空スライス、追加登録では
// 既存 credential を反映して excludeCredentials 相当の生成に用いる。
type registrationUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *registrationUser) WebAuthnID() []byte                         { return u.id }
func (u *registrationUser) WebAuthnName() string                       { return u.name }
func (u *registrationUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *registrationUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// BeginRegistrationNew は未認証クライアントの新規登録 ceremony の begin 段階を担う
// （Req 1.1, 1.4, 1.5）。
//
// 処理フロー:
//  1. username を検証・正規化（ErrInvalidUsername / Req 1.5）
//  2. 既存 user との username 重複を pre-check（ErrUsernameTaken / Req 1.4）
//  3. PKCE code_challenge の形式を検証（auth.ValidatePKCES256 / S256 固定）。
//     ※ 本 task では PKCE の永続化・後段継承は行わない（design/tasks 差分は impl-notes
//     「確認事項」参照）。early validation のみを実施することで、不正 PKCE で無駄な
//     WebAuthn ceremony を起動しないよう防衛する。
//  4. 仮 UUID を発行し WebAuthnUser を組み立て、WebAuthnAdapter.BeginRegistration で
//     challenge / options / sessionData を生成
//  5. ChallengeStore.Issue(kind=registration_new, userID=nil, pendingUsername=&normalized)
//     として challenge を発行
//
// 仮 UUID は Finish 時に users.id として確定される。passkey_challenges.user_id は
// users への FK のため、この時点で未作成の仮 UUID を渡すことはできない（userID=nil）。
// 仮 UUID は sessionData（webauthn.SessionData.UserID = user handle）として保存され、
// Finish 時はそこから復元して同じ WebAuthnID を再構築する。task 5 の credentialLookup が
// [](byte)(users.id) を WebAuthnID として使う設計と整合する。
//
// 拒否の伝播:
//   - ErrInvalidUsername: username 形式不正（handler で 400 INVALID_USERNAME）
//   - ErrUsernameTaken: username 重複（handler で 409 USERNAME_TAKEN）
//   - ErrRegistrationFailed: PKCE 形式不正 / WebAuthn 内部拒否（handler で 400 REGISTRATION_FAILED）
//   - 上記以外の infra エラー: wrap して返す（handler で 500）
func (s *RegistrationService) BeginRegistrationNew(
	ctx context.Context,
	rawUsername string,
	optionalEmail string,
	codeChallenge string,
) (string, []byte, error) {
	normalized, err := ValidateAndNormalize(rawUsername)
	if err != nil {
		return "", nil, err
	}

	existing, err := s.users.FindByNormalizedUsername(ctx, normalized)
	if err != nil {
		return "", nil, fmt.Errorf("failed to check username uniqueness: %w", err)
	}
	if existing != nil {
		return "", nil, ErrUsernameTaken
	}

	// PKCE 形式 pre-check（S256 固定）。無効な PKCE で無駄な WebAuthn ceremony を回避する。
	// PKCE の永続化・後段継承は本 task では省略（impl-notes 確認事項参照）。
	if err := auth.ValidatePKCES256(codeChallenge, "S256"); err != nil {
		s.logRejection("passkey registration begin (new) rejected: invalid pkce", "")
		return "", nil, ErrRegistrationFailed
	}

	// 仮 UUID を先に発行し Finish 時の users.id / WebAuthnUser 再構築で使う。
	// 仮 UUID の byte 列表現（`[]byte(uuid)`）は、task 5 の credentialLookup が
	// `[]byte(users.id)` を WebAuthnID として使う実装と 1:1 で一致する。
	pendingUserID := uuid.New().String()
	user := &registrationUser{
		id:          []byte(pendingUserID),
		name:        rawUsername,
		displayName: rawUsername,
	}

	options, sessionData, rawChallenge, err := s.adapter.BeginRegistration(user, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to begin webauthn registration: %w", err)
	}

	pending := normalized
	challengeID, err := s.challenges.Issue(
		ctx,
		model.PasskeyChallengeKindRegistrationNew,
		nil, // user_id は users FK のため、未作成 user の仮 UUID は渡せない（sessionData 経由で運ぶ）
		&pending,
		sessionData,
		rawChallenge,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to issue registration challenge: %w", err)
	}
	// 未使用引数の警告回避（optional email は Finish 時に確定させる。本 task では
	// begin 段階では受け取るだけで検証・保存はしない設計）。email = "" 許容は Req 1.6。
	_ = optionalEmail
	return challengeID, options, nil
}

// FinishRegistrationNew は未認証クライアントの新規登録 ceremony の finish 段階を担う
// （Req 1.2, 1.3, 1.6, 1.7）。
//
// 処理フロー:
//  1. ChallengeStore.Consume(kind=registration_new) → 期限切れ / 二重消費 /
//     kind 不一致は ErrRegistrationFailed に正規化（Req 1.7）
//  2. challenge.SessionData（webauthn.SessionData.UserID = begin 時の仮 UUID）と
//     challenge.PendingUsername（normalized）で WebAuthnUser を再構築
//  3. WebAuthnAdapter.FinishRegistration で ParsedCredential を得る
//     （拒否は ErrRegistrationFailed に正規化）
//  4. CreateUserOnly で users 行を INSERT（仮 UUID を users.id として確定）。
//     UNIQUE 衝突（race）は ErrRegistrationFailed（Req 1.7）
//  5. PasskeyCredentialWriter.Create で credential 行を INSERT。
//     credential_id UNIQUE 衝突は ErrRegistrationFailed に正規化（Req 1.7）
//
// email は begin 時に受け取っていないため、本 method のシグネチャでは追加受付しない。
// design/tasks 上の想定通り email = "" のまま user 行を作成する（Req 1.6）。
//
// 平文 attestation / requestBody / challenge / username 生値をログ・エラー・レスポンスに
// 出さない（NFR 1.2 / 1.3 / 3.2）。
func (s *RegistrationService) FinishRegistrationNew(
	ctx context.Context,
	challengeID string,
	requestBody []byte,
) (string, error) {
	ch, err := s.challenges.Consume(ctx, challengeID, model.PasskeyChallengeKindRegistrationNew)
	if err != nil {
		if errors.Is(err, ErrChallengeNotUsable) {
			s.logRejection("passkey registration finish (new) rejected: challenge not usable",
				shortID(challengeID))
			return "", ErrRegistrationFailed
		}
		return "", fmt.Errorf("failed to consume registration challenge: %w", err)
	}
	if ch.PendingUsername == nil {
		// begin 段階で pendingUsername を必ず保存する契約（BeginRegistrationNew）。
		// missing は自陣契約違反だが uniform 拒否側に倒す。
		s.logRejection("passkey registration finish (new) rejected: malformed challenge",
			shortID(challengeID))
		return "", ErrRegistrationFailed
	}
	// 仮 UUID（begin 時の WebAuthn user handle）は challenge.user_id 列ではなく
	// sessionData に保存されている（user_id は users FK のため未作成 user を指せない）。
	session, err := unmarshalSession(ch.SessionData)
	if err != nil || len(session.UserID) == 0 {
		s.logRejection("passkey registration finish (new) rejected: malformed challenge",
			shortID(challengeID))
		return "", ErrRegistrationFailed
	}
	pendingUserID := string(session.UserID)
	normalized := *ch.PendingUsername

	user := &registrationUser{
		id:          []byte(pendingUserID),
		name:        normalized,
		displayName: normalized,
	}
	parsed, err := s.adapter.FinishRegistration(user, ch.SessionData, requestBody)
	if err != nil {
		s.logRejection("passkey registration finish (new) rejected: webauthn attestation",
			shortID(challengeID))
		// adapter 側で既に ErrRegistrationFailed に正規化されているが、
		// 万一 wrap されていても uniform 側へ倒す。
		if errors.Is(err, ErrRegistrationFailed) {
			return "", ErrRegistrationFailed
		}
		return "", ErrRegistrationFailed
	}

	newUser := &model.User{
		ID:                 pendingUserID,
		Email:              "", // Req 1.6: リカバリ用メールなしを許容
		Username:           normalized,
		UsernameNormalized: normalized,
	}
	if err := s.users.CreateUserOnly(ctx, newUser); err != nil {
		if errors.Is(err, repository.ErrUsernameTaken) {
			// begin 時 pre-check 後の race。tasks.md L105 に従い finish 段階の
			// username UNIQUE 衝突は ErrRegistrationFailed に正規化する（uniform 拒否）。
			s.logRejection("passkey registration finish (new) rejected: username race",
				shortID(challengeID))
			return "", ErrRegistrationFailed
		}
		return "", fmt.Errorf("failed to create user: %w", err)
	}

	cred := &model.PasskeyCredential{
		UserID:          newUser.ID,
		CredentialID:    parsed.ID,
		PublicKey:       parsed.PublicKey,
		SignCount:       parsed.SignCount,
		AttestationType: parsed.AttestationType,
		AAGUID:          parsed.AAGUID,
		Transports:      parsed.Transports,
	}
	if err := s.credentials.Create(ctx, cred); err != nil {
		if errors.Is(err, repository.ErrCredentialAlreadyRegistered) {
			// Req 1.7: 内部詳細を反射しない uniform 拒否。
			s.logRejection("passkey registration finish (new) rejected: credential duplicate",
				shortID(challengeID))
			return "", ErrRegistrationFailed
		}
		return "", fmt.Errorf("failed to save passkey credential: %w", err)
	}
	return newUser.ID, nil
}

// BeginAddCredential は認証済みクライアントの追加登録 ceremony の begin 段階を担う
// （Req 3.1）。
//
// 処理フロー:
//  1. FindByID で既存 user を取得（未存在は ErrRegistrationFailed / uniform 拒否）
//  2. ListByUserID で既存 credential を取得し excludeCredentials に反映
//     （同一 authenticator の重複登録を防ぐ WebAuthn の標準機構）
//  3. WebAuthnUser（WebAuthnID = []byte(users.id)）を組み立て BeginRegistration を呼ぶ
//     （task 5 の credentialLookup が同じ表現を用いる設計と整合）
//  4. ChallengeStore.Issue(kind=registration_add, userID=&authenticatedUserID)
//
// 既存 identities（Google 紐付け等）には触れない（Req 3.3）。
func (s *RegistrationService) BeginAddCredential(
	ctx context.Context,
	authenticatedUserID string,
) (string, []byte, error) {
	u, err := s.users.FindByID(ctx, authenticatedUserID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load user: %w", err)
	}
	if u == nil {
		// context 上は認証済みだが DB 上は未存在（退会後 token 等）。uniform 拒否側に倒す。
		s.logRejection("passkey registration begin (add) rejected: user not found",
			shortID(authenticatedUserID))
		return "", nil, ErrRegistrationFailed
	}

	existing, err := s.credentials.ListByUserID(ctx, u.ID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to list existing credentials: %w", err)
	}

	exclude := make([][]byte, 0, len(existing))
	webauthnCreds := make([]webauthn.Credential, 0, len(existing))
	for _, cred := range existing {
		exclude = append(exclude, cred.CredentialID)
		webauthnCreds = append(webauthnCreds, webauthn.Credential{
			ID:        cred.CredentialID,
			PublicKey: cred.PublicKey,
		})
	}

	user := &registrationUser{
		id:          []byte(u.ID),
		name:        displayNameFor(u),
		displayName: displayNameFor(u),
		creds:       webauthnCreds,
	}

	options, sessionData, rawChallenge, err := s.adapter.BeginRegistration(user, exclude)
	if err != nil {
		return "", nil, fmt.Errorf("failed to begin webauthn registration: %w", err)
	}

	uid := u.ID
	challengeID, err := s.challenges.Issue(
		ctx,
		model.PasskeyChallengeKindRegistrationAdd,
		&uid,
		nil,
		sessionData,
		rawChallenge,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to issue add-credential challenge: %w", err)
	}
	return challengeID, options, nil
}

// FinishAddCredential は認証済みクライアントの追加登録 ceremony の finish 段階を担う
// （Req 3.2, 3.4, 3.6, 3.7）。
//
// 処理フロー:
//  1. ChallengeStore.Consume(kind=registration_add) →
//     期限切れ / 二重消費 / kind 不一致は ErrRegistrationFailed（Req 3.7）
//  2. challenge.UserID と authenticatedUserID の一致を検証（不一致 → ErrRegistrationFailed）。
//     これにより「別 user の challenge を横取り」を防ぐ
//  3. FindByCredentialID で他 user 既登録なら ErrRegistrationFailed（Req 3.6）
//  4. FindByID で user を再取得し WebAuthnUser を再構築
//  5. WebAuthnAdapter.FinishRegistration で ParsedCredential を得る（拒否は Req 3.7 に正規化）
//  6. PasskeyCredentialWriter.Create で credential 行を INSERT
//     （同一 user への複数 credential 登録は許容 / Req 3.4）
//
// 既存 identities（Google 紐付け等）には触れない（Req 3.3）。
func (s *RegistrationService) FinishAddCredential(
	ctx context.Context,
	authenticatedUserID string,
	challengeID string,
	requestBody []byte,
) error {
	ch, err := s.challenges.Consume(ctx, challengeID, model.PasskeyChallengeKindRegistrationAdd)
	if err != nil {
		if errors.Is(err, ErrChallengeNotUsable) {
			s.logRejection("passkey registration finish (add) rejected: challenge not usable",
				shortID(challengeID))
			return ErrRegistrationFailed
		}
		return fmt.Errorf("failed to consume add-credential challenge: %w", err)
	}
	if ch.UserID == nil || *ch.UserID != authenticatedUserID {
		// 別 user の challenge を横取りしている / begin 時に userID 未設定 のいずれか。
		// いずれも uniform 拒否側に倒す。
		s.logRejection("passkey registration finish (add) rejected: user mismatch",
			shortID(challengeID))
		return ErrRegistrationFailed
	}

	// Req 3.6: 別 user に既登録の credential 提示を pre-check（Create の UNIQUE 衝突は
	// 最終防衛線として引き続き有効だが、ここで先行拒否することで attestation 検証も回避）。
	// 本 pre-check は Finish 段階では requestBody 内の credential.rawId が展開されるまで不可能な
	// ため、attestation 検証成功後に再度実施する。
	u, err := s.users.FindByID(ctx, authenticatedUserID)
	if err != nil {
		return fmt.Errorf("failed to load user: %w", err)
	}
	if u == nil {
		s.logRejection("passkey registration finish (add) rejected: user not found",
			shortID(authenticatedUserID))
		return ErrRegistrationFailed
	}

	existing, err := s.credentials.ListByUserID(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("failed to list existing credentials: %w", err)
	}
	webauthnCreds := make([]webauthn.Credential, 0, len(existing))
	for _, cred := range existing {
		webauthnCreds = append(webauthnCreds, webauthn.Credential{
			ID:        cred.CredentialID,
			PublicKey: cred.PublicKey,
		})
	}
	user := &registrationUser{
		id:          []byte(u.ID),
		name:        displayNameFor(u),
		displayName: displayNameFor(u),
		creds:       webauthnCreds,
	}

	parsed, err := s.adapter.FinishRegistration(user, ch.SessionData, requestBody)
	if err != nil {
		s.logRejection("passkey registration finish (add) rejected: webauthn attestation",
			shortID(challengeID))
		return ErrRegistrationFailed
	}

	// Req 3.6: attestation 検証後、他 user への既登録を明示的に拒否する。
	// Create の UNIQUE 制約でも防衛できるが、他 user への漏洩を最小化するため
	// INSERT を試行する前に拒否する。
	other, err := s.credentials.FindByCredentialID(ctx, parsed.ID)
	if err != nil {
		return fmt.Errorf("failed to check credential ownership: %w", err)
	}
	if other != nil && other.UserID != u.ID {
		s.logRejection("passkey registration finish (add) rejected: credential owned by another user",
			shortID(challengeID))
		return ErrRegistrationFailed
	}

	cred := &model.PasskeyCredential{
		UserID:          u.ID,
		CredentialID:    parsed.ID,
		PublicKey:       parsed.PublicKey,
		SignCount:       parsed.SignCount,
		AttestationType: parsed.AttestationType,
		AAGUID:          parsed.AAGUID,
		Transports:      parsed.Transports,
	}
	if err := s.credentials.Create(ctx, cred); err != nil {
		if errors.Is(err, repository.ErrCredentialAlreadyRegistered) {
			// 上記 pre-check と race した場合の最終防衛線。Req 1.7 / 3.6 の uniform 拒否。
			s.logRejection("passkey registration finish (add) rejected: credential duplicate race",
				shortID(challengeID))
			return ErrRegistrationFailed
		}
		return fmt.Errorf("failed to save passkey credential: %w", err)
	}
	return nil
}

// displayNameFor は既存 user のディスプレイ表示に使う短い文字列を返す。
// Username → Name → Email → users.id の順で最初の非空値を採用する。
// WebAuthnUser.WebAuthnName / WebAuthnDisplayName は表示用途で認証判定には
// 関わらないため、非空であればよい（library の内部 validation では長さ 0 だけを検査）。
func displayNameFor(u *model.User) string {
	switch {
	case u.Username != "":
		return u.Username
	case u.Name != "":
		return u.Name
	case u.Email != "":
		return u.Email
	default:
		return u.ID
	}
}

// shortID は challenge_id / user_id の先頭 8 文字だけを返し、ログに機密値・
// 追跡可能な PII を出さないための helper（NFR 1.2 / 3.2）。
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// logRejection は拒否事象を運用ログとして記録する（NFR 3.1）。
// challenge_id_prefix は shortID（先頭 8 文字）のみを載せ、平文 challenge や
// requestBody 等の機密値を出さない（NFR 1.2 / 3.2）。
func (s *RegistrationService) logRejection(msg, challengeIDPrefix string) {
	slog.Warn(msg, slog.String("challenge_id_prefix", challengeIDPrefix))
}
