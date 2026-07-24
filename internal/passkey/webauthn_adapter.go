package passkey

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// 本ファイルは go-webauthn/webauthn の呼び出しを service 層から隔離する
// Adapter 境界を提供する（Issue #216 / design.md WebAuthnAdapter /
// Req 1.1, 1.2, 1.7, 2.1, 2.2, 2.5, 3.1, 3.2, 3.7, 4.5, NFR 1.4, NFR 4.1）。
//
// 平文 request body / attestation blob / assertion blob / sessionData は
// slog 等のログには出さない（NFR 1.2）。エラー応答は理由詳細なしの
// ErrRegistrationFailed / ErrAuthenticationFailed に正規化して返す。

// WebAuthnUser は go-webauthn/webauthn が要求する User interface の型エイリアスである。
//
// design.md では独立した interface として書かれているが、library へ渡す際の摩擦と
// 定義の二重管理を避けるためエイリアスを採用した。上位（RegistrationService /
// AuthenticationService）は本エイリアスを実装するだけで library API へそのまま
// 渡せる。
type WebAuthnUser = webauthn.User

// ParsedCredential は WebAuthn credential のうち、Feedman 側の永続化・
// counter 更新に必要な最小情報だけを抽出した DTO である。
//
// library の webauthn.Credential 全フィールドを service / repository に露出させると
// 依存が広がりすぎるため、adapter 境界でこの構造体に射影する。
type ParsedCredential struct {
	// ID は credential ID（WebAuthn credential.id）。
	ID []byte
	// PublicKey は COSE 形式の credential 公開鍵。
	PublicKey []byte
	// SignCount は認証カウンタの初期値（登録時）または更新後値（認証時）。
	SignCount uint32
	// AttestationType は "none" / "basic_full" / "packed" 等の attestation 種別。
	AttestationType string
	// AAGUID は authenticator の識別子（nullable 相当。未提供時は空スライス）。
	AAGUID []byte
	// Transports は "internal" / "usb" / "nfc" / "ble" 等の transport 名リスト。
	Transports []string
}

// WebAuthnAdapter は service 層へ公開する adapter interface である。
//
// go-webauthn/webauthn の高レベル API（*http.Request を要求する FinishRegistration
// 等）ではなく、raw bytes を扱う低レベル API（protocol.Parse*Bytes + CreateCredential
// / ValidateDiscoverableLogin）に依存することで、handler 層で `*http.Request` を
// 再構築する煩雑さを避けている。
type WebAuthnAdapter interface {
	// BeginRegistration は新規登録または追加登録の challenge を生成する。
	// excludeCredentials は追加登録時に既登録の credential ID を渡すために用いる
	// （新規登録では nil / 空スライスで良い）。返り値の options / sessionData は
	// いずれも JSON marshal 済み。rawChallenge は ChallengeStore が hash 化して
	// 永続化するための base64url challenge 文字列の byte 表現。
	BeginRegistration(user WebAuthnUser, excludeCredentials [][]byte) (
		options []byte, sessionData []byte, rawChallenge []byte, err error,
	)

	// FinishRegistration は registration 応答を検証し、credential を返す。
	// 検証失敗（不正な attestation / 期限切れ challenge 等）は理由を潰して
	// ErrRegistrationFailed を返す（Req 1.7 / 3.7）。
	FinishRegistration(user WebAuthnUser, sessionData []byte, requestBody []byte) (
		*ParsedCredential, error,
	)

	// BeginLogin は認証 challenge を生成する（discoverable / usernameless。
	// allowCredentials 空 = Req 2.6 の存在有無非開示に整合）。
	BeginLogin() (options []byte, sessionData []byte, rawChallenge []byte, err error)

	// FinishLogin は認証応答を検証し、userHandle / credentialID / 更新後 SignCount を返す。
	// credentialLookup は raw credentialID から (User, ParsedCredential, error) を返す
	// callback で、service 層が repository を用いて解決する。counter 後退（library の
	// Authenticator.CloneWarning）は ErrAuthenticationFailed に正規化して返す
	// （NFR 1.4）。
	FinishLogin(sessionData []byte, requestBody []byte,
		credentialLookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
	) (userHandle []byte, credentialID []byte, updatedSignCount uint32, err error)
}

// GoWebAuthnAdapter は WebAuthnAdapter の go-webauthn/webauthn 実装である。
type GoWebAuthnAdapter struct {
	wa *webauthn.WebAuthn
}

// NewGoWebAuthnAdapter は Relying Party 設定を固定した GoWebAuthnAdapter を生成する。
//
// rpID は RP の effective domain（"example.com" のような scheme / port を含まない値）。
// rpDisplayName は authenticator の同意画面に表示される表示名。
// origins は許容 origin の完全形（"https://example.com" 等）を 1 件以上含めること。
// 空の origins を渡すと webauthn.New が error を返す。
func NewGoWebAuthnAdapter(rpID, rpDisplayName string, origins []string) (*GoWebAuthnAdapter, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpDisplayName,
		RPOrigins:     origins,
	})
	if err != nil {
		// NFR 1.2: 生 origin 文字列や RPID の値をエラーメッセージに含めない
		// （呼び出し側の設定値を直接反射しないよう wrap のみ行う）。
		return nil, fmt.Errorf("failed to construct webauthn adapter: %w", err)
	}
	return &GoWebAuthnAdapter{wa: wa}, nil
}

// BeginRegistration は go-webauthn の BeginRegistration を wrap する。
//
// excludeCredentials に既登録 credential ID を渡すと、library 側で
// PublicKeyCredentialCreationOptions.excludeCredentials に反映される。
// 追加登録時は同一 authenticator が同一 user に重複登録されるのを防ぐために利用する
// （Req 3.1 の excludeCredentials 用途 / Req 4.5 の 128bit 以上 challenge は
// library の 32byte random に依拠）。
func (a *GoWebAuthnAdapter) BeginRegistration(user WebAuthnUser, excludeCredentials [][]byte) (
	[]byte, []byte, []byte, error,
) {
	opts := []webauthn.RegistrationOption{
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	}
	if len(excludeCredentials) > 0 {
		descriptors := make([]protocol.CredentialDescriptor, 0, len(excludeCredentials))
		for _, id := range excludeCredentials {
			descriptors = append(descriptors, protocol.CredentialDescriptor{
				Type:         protocol.PublicKeyCredentialType,
				CredentialID: id,
			})
		}
		opts = append(opts, webauthn.WithExclusions(descriptors))
	}

	creation, session, err := a.wa.BeginRegistration(user, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to begin webauthn registration: %w", err)
	}
	return marshalBeginResults(creation, session)
}

// FinishRegistration は attestation 応答を検証し ParsedCredential を返す。
// 拒否は理由を潰して ErrRegistrationFailed に正規化する（Req 1.7 / 3.7）。
func (a *GoWebAuthnAdapter) FinishRegistration(user WebAuthnUser, sessionData []byte, requestBody []byte) (
	*ParsedCredential, error,
) {
	session, err := unmarshalSession(sessionData)
	if err != nil {
		// sessionData 破損は上位契約違反だが、拒否として一律扱う（NFR 1.3）。
		return nil, ErrRegistrationFailed
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(requestBody)
	if err != nil {
		return nil, ErrRegistrationFailed
	}
	cred, err := a.wa.CreateCredential(user, *session, parsed)
	if err != nil {
		return nil, ErrRegistrationFailed
	}
	return toParsedCredential(cred), nil
}

// BeginLogin は discoverable login（allowCredentials 空）の challenge を生成する。
//
// allowCredentials を空にすることで iOS platform authenticator が credential を選び、
// サーバは credential 存在有無を応答段階まで開示しない（Req 2.6 に整合）。
func (a *GoWebAuthnAdapter) BeginLogin() ([]byte, []byte, []byte, error) {
	assertion, session, err := a.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to begin webauthn login: %w", err)
	}
	return marshalBeginResults(assertion, session)
}

// FinishLogin は assertion 応答を検証し、認証成功時の (userHandle, credentialID,
// updatedSignCount) を返す。
//
// credentialLookup callback は library から (rawID, userHandle) で呼ばれ、当該
// credential を持つ User を返す責務を持つ。lookup が error を返した場合や、
// library が rejection（不正 assertion / user 未解決）を返した場合、counter 後退
// （Authenticator.CloneWarning）を検出した場合はいずれも ErrAuthenticationFailed に
// 正規化する（Req 2.5 / 2.6 / NFR 1.4）。
func (a *GoWebAuthnAdapter) FinishLogin(sessionData []byte, requestBody []byte,
	credentialLookup func(credentialID []byte) (WebAuthnUser, *ParsedCredential, error),
) ([]byte, []byte, uint32, error) {
	session, err := unmarshalSession(sessionData)
	if err != nil {
		return nil, nil, 0, ErrAuthenticationFailed
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(requestBody)
	if err != nil {
		return nil, nil, 0, ErrAuthenticationFailed
	}

	// discoverable login では library が credential ID / user handle を渡してくる
	// ため、service 層で登録済み credential と user を解決する。lookup が失敗した
	// 場合は library に error を伝播し、library 側で ErrorUnknownCredential 等に
	// 変換されて Validate が失敗する。
	var resolvedUser WebAuthnUser
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		_ = userHandle
		user, _, lookupErr := credentialLookup(rawID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		resolvedUser = user
		return user, nil
	}

	cred, err := a.wa.ValidateDiscoverableLogin(handler, *session, parsed)
	if err != nil {
		return nil, nil, 0, ErrAuthenticationFailed
	}
	// NFR 1.4: SignCount 後退（clone 疑い）は成功扱いにしない。
	if cred.Authenticator.CloneWarning {
		return nil, nil, 0, ErrAuthenticationFailed
	}
	if resolvedUser == nil {
		// Validate が成功したが lookup callback が呼ばれなかった場合の防衛的分岐。
		// 実行時到達は想定していないが、silent 成功を避けるため拒否側に倒す。
		return nil, nil, 0, ErrAuthenticationFailed
	}
	return resolvedUser.WebAuthnID(), cred.ID, cred.Authenticator.SignCount, nil
}

// marshalBeginResults は Begin* 系の返り値（options 構造体と SessionData）を
// JSON へ marshal し、raw challenge 文字列も返す共通ヘルパー。
func marshalBeginResults(optionsStruct interface{}, session *webauthn.SessionData) (
	[]byte, []byte, []byte, error,
) {
	optionsJSON, err := json.Marshal(optionsStruct)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal webauthn options: %w", err)
	}
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal webauthn session: %w", err)
	}
	// session.Challenge は base64url 文字列（library の生成規約）。ChallengeStore は
	// これを HashNativeSecret で hash 化して永続化するため、決定的な byte 列であれば良い。
	rawChallenge := []byte(session.Challenge)
	return optionsJSON, sessionJSON, rawChallenge, nil
}

// unmarshalSession は永続化した sessionData（JSON）を SessionData に復元する。
// エラーは呼び出し側で拒否側 sentinel に変換されるため、詳細を潰したまま返す。
func unmarshalSession(sessionData []byte) (*webauthn.SessionData, error) {
	if len(sessionData) == 0 {
		return nil, errors.New("empty session data")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// toParsedCredential は library の Credential から ParsedCredential へ射影する。
func toParsedCredential(cred *webauthn.Credential) *ParsedCredential {
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	return &ParsedCredential{
		ID:              cred.ID,
		PublicKey:       cred.PublicKey,
		SignCount:       cred.Authenticator.SignCount,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		Transports:      transports,
	}
}

// compile-time interface check
var _ WebAuthnAdapter = (*GoWebAuthnAdapter)(nil)
