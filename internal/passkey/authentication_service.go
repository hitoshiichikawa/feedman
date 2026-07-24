package passkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/hitoshi/feedman/internal/auth"
	"github.com/hitoshi/feedman/internal/model"
)

// 本ファイルはパスキー認証 ceremony を担い、成功時に既存 native auth の auth_code
// 発行契約へ合流させる AuthenticationService を提供する（Issue #216 / design.md
// AuthenticationService / Req 2.1〜2.6, NFR 1.2, NFR 1.4, NFR 2.1, NFR 4.1）。
//
// 拒否は理由詳細を反射せず ErrAuthenticationFailed に uniform 化する
// （Req 2.5 / 2.6 / NFR 1.3）。平文 assertion / requestBody / challenge / auth_code
// 平文をログ・エラーメッセージ・レスポンスに出さない（NFR 1.2 / NFR 3.2）。

// PasskeyCredentialReader は AuthenticationService が passkey_credentials 表アクセスに
// 必要とする最小 interface である（interface segregation / CLAUDE.md §5）。
//
// repository.PostgresPasskeyCredentialRepo は構造的にこれを充足するため、wiring 時は
// 具体型をそのまま渡せる。テストでは stub 実装を差し込む。
type PasskeyCredentialReader interface {
	// FindByCredentialID は raw credential_id で credential を検索する（Req 2.2）。
	// 未存在は (nil, nil)。
	FindByCredentialID(ctx context.Context, credentialID []byte) (*model.PasskeyCredential, error)

	// UpdateSignCount は当該 credential の sign_count と last_used_at を更新する
	// （NFR 1.4 の counter 記録用途）。
	UpdateSignCount(ctx context.Context, id string, signCount uint32, lastUsedAt time.Time) error
}

// UserReader は AuthenticationService が users 表アクセスに必要とする最小 interface
// である（interface segregation）。
//
// repository.PostgresUserRepo は構造的にこれを充足するため、wiring 時は具体型を
// そのまま渡せる。テストでは stub 実装を差し込む。
type UserReader interface {
	// FindByID は credential.UserID から user を解決する（Req 2.2）。
	// 未存在は (nil, nil)。
	FindByID(ctx context.Context, id string) (*model.User, error)
}

// authnSession は authentication kind の challenge に紐付けて保存する封筒である。
//
// model.PasskeyChallenge / WebAuthnAdapter は task 1〜3 で確定・変更不可のため、PKCE の
// codeChallenge を finish 段階まで運ぶ経路として ChallengeStore の SessionData 引数を
// opaque byte 列として利用する。inner の WebAuthnSession（go-webauthn.SessionData の
// JSON marshaled bytes）はそのまま無改変で保持し、CodeChallenge を addressable
// メタ情報として並列に持つ。ChallengeStore はバイト列として opaque に扱うため
// 封筒化しても意味的な差分は生じない（Req 2.3 / 2.4 / NFR 2.1）。
type authnSession struct {
	// WebAuthnSession は adapter.BeginLogin が返した webauthn.SessionData の JSON bytes。
	// FinishLogin にそのまま渡すため RawMessage で無改変に保持する。
	WebAuthnSession json.RawMessage `json:"webauthn_session"`
	// CodeChallenge は begin 時に検証した PKCE S256 challenge（base64url 43 文字）。
	// finish 時に auth_code の PKCEChallenge として設定される（Req 2.4 の合流準備）。
	CodeChallenge string `json:"code_challenge"`
}

// AuthenticationService はパスキー認証 ceremony を担い、成功時に既存 native auth の
// auth_code 発行契約に合流させる service である（Req 2.1〜2.6, NFR 1.4）。
//
// 依存はすべて最小 interface として宣言し（interface segregation / CLAUDE.md §5）、
// テストでは stub / mock を差し込むことで外部ネットワーク非依存の検証を可能にする（NFR 4.1）。
type AuthenticationService struct {
	adapter     WebAuthnAdapter
	challenges  challengeStore
	credentials PasskeyCredentialReader
	users       UserReader
	authCodes   auth.AuthCodeCreator
	now         func() time.Time
}

// NewAuthenticationService は AuthenticationService を生成する。
//
// now が nil の場合は time.Now を既定として採用する（テストからは差し替え可能）。
// authCodes は既存 `auth.AuthCodeCreator` interface を流用しており、
// repository.PostgresAuthCodeRepo が構造的にこれを充足するため、native auth と共用の
// wiring がそのまま使える（NFR 2.1）。
func NewAuthenticationService(
	adapter WebAuthnAdapter,
	challenges challengeStore,
	credentials PasskeyCredentialReader,
	users UserReader,
	authCodes auth.AuthCodeCreator,
	now func() time.Time,
) *AuthenticationService {
	if now == nil {
		now = time.Now
	}
	return &AuthenticationService{
		adapter:     adapter,
		challenges:  challenges,
		credentials: credentials,
		users:       users,
		authCodes:   authCodes,
		now:         now,
	}
}

// authnUser は AuthenticationService の credentialLookup が組み立てる
// WebAuthnUser（= webauthn.User）実装である。
//
// go-webauthn の ValidateDiscoverableLogin は登録済み credential の SignCount を
// stored 値として要求し、assertion の Counter と比較して CloneWarning を判定する
// （NFR 1.4）。そのため WebAuthnCredentials() には Authenticator.SignCount を必ず
// 含めた credential を返す（RegistrationService.registrationUser は SignCount を
// 詰めていないため authentication では別途本型を使う）。
type authnUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *authnUser) WebAuthnID() []byte                         { return u.id }
func (u *authnUser) WebAuthnName() string                       { return u.name }
func (u *authnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *authnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// BeginAuthentication はパスキー認証 ceremony の begin 段階を担う（Req 2.1, 2.6）。
//
// design.md L655-656 のシグネチャは引数無しだが、tasks.md L135-139 および design.md の
// 「設計判断: PKCE 相互作用」節（L674 以降）が PKCE 合流方式を採用しており、
// codeChallenge を begin 時に受け取って challenge に紐付ける必要がある。task 4 の
// BeginRegistrationNew と同じ判断（tasks.md 優先）に基づき、`codeChallenge string`
// 引数を追加した（本差分は impl-notes.md「確認事項」に記録）。
//
// 処理フロー:
//  1. PKCE code_challenge の形式を検証（auth.ValidatePKCES256 / S256 固定）
//     形式不正は ErrAuthenticationFailed に uniform 化（Req 2.5 / 2.6）
//  2. WebAuthnAdapter.BeginLogin（discoverable / allowCredentials 空 / Req 2.6 の
//     存在有無非開示に整合）で options / sessionData / rawChallenge を生成
//  3. sessionData を authnSession 封筒に包み codeChallenge を並列保存
//     （model.PasskeyChallenge を変更禁止のため、SessionData に PKCE を封筒化して運ぶ）
//  4. ChallengeStore.Issue(kind=authentication, userID=nil, pendingUsername=nil)
//
// 拒否の伝播:
//   - ErrAuthenticationFailed: PKCE 形式不正
//   - 上記以外の infra エラー（adapter / ChallengeStore の DB 障害等）: wrap して返す
func (s *AuthenticationService) BeginAuthentication(
	ctx context.Context,
	codeChallenge string,
) (string, []byte, error) {
	if err := auth.ValidatePKCES256(codeChallenge, "S256"); err != nil {
		// NFR 1.2: codeChallenge の値は logRejection に含めない（prefix も無し）。
		s.logRejection("passkey authentication begin rejected: invalid pkce", "")
		return "", nil, ErrAuthenticationFailed
	}

	options, sessionData, rawChallenge, err := s.adapter.BeginLogin()
	if err != nil {
		return "", nil, fmt.Errorf("failed to begin webauthn login: %w", err)
	}

	envelope := authnSession{
		WebAuthnSession: sessionData,
		CodeChallenge:   codeChallenge,
	}
	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		// json.Marshal は上記構造なら失敗しないが、防衛的に wrap する。
		return "", nil, fmt.Errorf("failed to marshal authentication session: %w", err)
	}

	challengeID, err := s.challenges.Issue(
		ctx,
		model.PasskeyChallengeKindAuthentication,
		nil, // Req 2.6: authentication では userID を event 発生時点で紐付けない
		nil, // pendingUsername は authentication では常に nil
		envelopeBytes,
		rawChallenge,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to issue authentication challenge: %w", err)
	}
	return challengeID, options, nil
}

// FinishAuthentication はパスキー認証 ceremony の finish 段階を担い、成功時に
// 既存 native auth の auth_code 発行契約に合流する（Req 2.2, 2.3, 2.4, 2.5, 2.6,
// NFR 1.4, NFR 2.1）。
//
// 引数順は design.md L669-671 に合わせて (ctx, requestBody, challengeID) とする
// （tasks.md L140 の記述順は description であり canonical シグネチャは design 優先）。
//
// 処理フロー:
//  1. ChallengeStore.Consume(kind=authentication) → 期限切れ / 二重消費 / kind 不一致 /
//     未存在は ErrAuthenticationFailed に正規化（Req 2.5 / 2.6）
//  2. Consume で得た SessionData を authnSession 封筒として unmarshal し、
//     inner の WebAuthnSession と CodeChallenge を復元。破損 / 不完全は
//     ErrAuthenticationFailed（uniform 拒否）
//  3. credentialLookup を組み立てて WebAuthnAdapter.FinishLogin に渡す。lookup 内で
//     PasskeyCredentialReader.FindByCredentialID → UserReader.FindByID の順で解決し、
//     解決結果を外側変数にキャプチャする（FinishLogin 成功後の sign_count 更新用）。
//     lookup が error を返した場合は adapter 側で ErrAuthenticationFailed に正規化される。
//  4. adapter.FinishLogin が error（不正 assertion / lookup 失敗 / counter 後退＝
//     CloneWarning）を返した場合は全て ErrAuthenticationFailed に正規化（NFR 1.4）
//  5. 成功時: PasskeyCredentialReader.UpdateSignCount で sign_count と last_used_at を更新
//     （NFR 1.4）
//  6. auth_code 生成（auth.GenerateAuthCode で base64url 32byte 乱数）→
//     auth.HashNativeSecret で hash 化 → AuthCodeCreator.Create（既存契約: 60 秒 TTL /
//     単回 / user_id 紐付、PKCEChallenge は envelope から復元 / Req 2.3, 2.4）
//  7. 平文 auth_code は戻り値としてのみ返す（Req 2.3 / NFR 1.2）。ログには hash 先頭
//     8 文字のみを載せる（既存 native.go 流儀）。
func (s *AuthenticationService) FinishAuthentication(
	ctx context.Context,
	requestBody []byte,
	challengeID string,
) (string, error) {
	ch, err := s.challenges.Consume(ctx, challengeID, model.PasskeyChallengeKindAuthentication)
	if err != nil {
		if errors.Is(err, ErrChallengeNotUsable) {
			s.logRejection("passkey authentication finish rejected: challenge not usable",
				shortID(challengeID))
			return "", ErrAuthenticationFailed
		}
		return "", fmt.Errorf("failed to consume authentication challenge: %w", err)
	}

	var envelope authnSession
	if err := json.Unmarshal(ch.SessionData, &envelope); err != nil {
		// envelope 破損は上位契約違反だが uniform 拒否側に倒す（NFR 1.3）。
		s.logRejection("passkey authentication finish rejected: malformed session envelope",
			shortID(challengeID))
		return "", ErrAuthenticationFailed
	}
	if len(envelope.WebAuthnSession) == 0 || envelope.CodeChallenge == "" {
		// 封筒が不完全（BeginAuthentication が両方詰めた前提と不整合）。
		s.logRejection("passkey authentication finish rejected: incomplete session envelope",
			shortID(challengeID))
		return "", ErrAuthenticationFailed
	}

	// FinishLogin 成功後の sign_count 更新に必要な resolved credential を
	// lookup クロージャ内でキャプチャする（adapter 側の resolvedUser キャプチャと同型）。
	var resolvedCred *model.PasskeyCredential
	lookup := func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error) {
		cred, err := s.credentials.FindByCredentialID(ctx, credentialID)
		if err != nil {
			// infra エラー: adapter 側で ErrAuthenticationFailed に正規化される。
			return nil, nil, fmt.Errorf("failed to lookup credential: %w", err)
		}
		if cred == nil {
			// 未存在: adapter 側で ErrAuthenticationFailed に正規化される（Req 2.6 の
			// 存在有無非開示に整合）。
			return nil, nil, errors.New("credential not found")
		}
		u, err := s.users.FindByID(ctx, cred.UserID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to lookup user: %w", err)
		}
		if u == nil {
			// credential は存在するが user が退会等で消失: uniform 拒否側へ倒す。
			return nil, nil, errors.New("user not found for credential")
		}

		display := displayNameFor(u)
		wu := &authnUser{
			id:          []byte(u.ID),
			name:        display,
			displayName: display,
			creds: []webauthn.Credential{
				{
					ID:        cred.CredentialID,
					PublicKey: cred.PublicKey,
					// NFR 1.4: stored SignCount を Authenticator に反映することで
					// library の CloneWarning 判定が有効になる。
					Authenticator: webauthn.Authenticator{
						AAGUID:    cred.AAGUID,
						SignCount: cred.SignCount,
					},
				},
			},
		}
		parsed := &ParsedCredential{
			ID:              cred.CredentialID,
			PublicKey:       cred.PublicKey,
			SignCount:       cred.SignCount,
			AttestationType: cred.AttestationType,
			AAGUID:          cred.AAGUID,
			Transports:      cred.Transports,
		}
		resolvedCred = cred
		return wu, parsed, nil
	}

	_, _, updatedSignCount, err := s.adapter.FinishLogin(envelope.WebAuthnSession, requestBody, lookup)
	if err != nil {
		s.logRejection("passkey authentication finish rejected: webauthn assertion",
			shortID(challengeID))
		// adapter 側で既に ErrAuthenticationFailed に正規化済みだが、万一 wrap されて
		// いても uniform 側に倒す（Req 2.5 / 2.6 / NFR 1.4）。
		return "", ErrAuthenticationFailed
	}
	if resolvedCred == nil {
		// FinishLogin が成功したが lookup callback が呼ばれなかった防衛的分岐。
		// 実行時到達は想定していないが、silent 成功を避けるため拒否側に倒す。
		s.logRejection("passkey authentication finish rejected: credential not resolved",
			shortID(challengeID))
		return "", ErrAuthenticationFailed
	}

	// NFR 1.4: sign_count と last_used_at を更新（PK id で更新）。
	if err := s.credentials.UpdateSignCount(ctx, resolvedCred.ID, updatedSignCount, s.now()); err != nil {
		return "", fmt.Errorf("failed to update credential sign count: %w", err)
	}

	// 既存 native auth と同一契約で auth_code を発行（Req 2.2 / 2.3 / 2.4 / NFR 2.1）。
	// auth.GenerateAuthCode / HashNativeSecret / NativeAuthCodeTTL を流用することで
	// トークン交換 endpoint 側は無変更で受理できる（合流）。
	plain, err := auth.GenerateAuthCode()
	if err != nil {
		return "", fmt.Errorf("failed to generate auth code: %w", err)
	}
	codeHash := auth.HashNativeSecret(plain)
	authCode := &model.AuthCode{
		ID:            uuid.New().String(),
		CodeHash:      codeHash,
		UserID:        resolvedCred.UserID,
		PKCEChallenge: envelope.CodeChallenge, // Req 2.4: envelope から PKCE を継承
		ExpiresAt:     s.now().Add(auth.NativeAuthCodeTTL),
	}
	if err := s.authCodes.Create(ctx, authCode); err != nil {
		return "", fmt.Errorf("failed to store auth code: %w", err)
	}

	// NFR 1.2 / NFR 3.1: 平文はログに残さず、hash 先頭 8 文字のみで追跡可能性を確保する
	// （既存 native.go / HandleNativeCallback と同方針）。
	slog.Info("passkey authentication succeeded",
		slog.String("user_id", resolvedCred.UserID),
		slog.String("auth_code_hash", codeHash[:8]),
	)

	return plain, nil
}

// logRejection は拒否事象を運用ログとして記録する（NFR 3.1）。
// challenge_id_prefix は shortID（先頭 8 文字）のみを載せ、平文 challenge や
// requestBody 等の機密値を出さない（NFR 1.2 / NFR 3.2）。
func (s *AuthenticationService) logRejection(msg, challengeIDPrefix string) {
	slog.Warn(msg, slog.String("challenge_id_prefix", challengeIDPrefix))
}
