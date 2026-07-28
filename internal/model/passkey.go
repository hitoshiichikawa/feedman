package model

import "time"

// PasskeyCredential はパスキー（WebAuthn credential）の永続化用ドメイン型である。
//
// 保存対象は検証に必要な情報（公開鍵・credential 識別子・sign counter 等）のみに限定し、
// パスキー本体の秘密情報は保持しない（NFR 1.1）。
// CredentialID はサービス全体で一意（DB 側 UNIQUE 制約）で、1 user あたり複数 credential
// を保持できる（同一 UserID の複数行を許容）。
//
// Issue #234: BackupEligible / BackupState は WebAuthn CredentialFlags の永続化値。
// 登録時に authenticator が報告した値を保存し、認証時の credential 復元で
// webauthn.Credential.Flags に反映することで、library の login validation
// （BE 一致判定 / login.go:371）を通過させる。
type PasskeyCredential struct {
	ID              string     // UUID
	UserID          string     // users.id への FK
	CredentialID    []byte     // WebAuthn credential ID（UNIQUE）
	PublicKey       []byte     // COSE-encoded 公開鍵
	SignCount       uint32     // counter（後退検出に使用 / NFR 1.4）
	AttestationType string     // "none" / "packed" / "apple" 等
	AAGUID          []byte     // authenticator ごとの識別子（nullable 相当）
	Transports      []string   // "internal" / "usb" / "nfc" / "ble" 等
	CreatedAt       time.Time  // 登録時刻
	LastUsedAt      *time.Time // 最終ログイン時刻（未使用なら nil）

	// BackupEligible は WebAuthn CredentialFlags.BackupEligible の永続化値
	// （Issue #234 / Req 1.1〜1.4, 2.1〜2.3）。
	// 登録時に authenticator が報告した値を保存し、認証時の flag 一致判定に用いる。
	// Req 4.3: 認証成功時に上書き更新しない（初回登録時の値を不変で保持する）。
	BackupEligible bool

	// BackupState は WebAuthn CredentialFlags.BackupState の永続化値
	// （Issue #234 / Req 1.1〜1.4, 2.1〜2.3, 4.2）。
	// 登録時に authenticator が報告した値を保存し、認証成功時には検証層から
	// 返却される最新値へ更新する（Req 4.2）。
	BackupState bool
}

// PasskeyChallengeKind はパスキー challenge の種別を表す値オブジェクトである。
type PasskeyChallengeKind string

// PasskeyChallengeKind の canonical 値。DB 上の kind カラム値と 1:1 で対応する。
const (
	// PasskeyChallengeKindRegistrationNew は未認証クライアントの新規登録 challenge。
	PasskeyChallengeKindRegistrationNew PasskeyChallengeKind = "registration_new"
	// PasskeyChallengeKindRegistrationAdd は認証済みクライアントの追加登録 challenge。
	PasskeyChallengeKindRegistrationAdd PasskeyChallengeKind = "registration_add"
	// PasskeyChallengeKindAuthentication は未認証クライアントの認証 challenge。
	PasskeyChallengeKindAuthentication PasskeyChallengeKind = "authentication"
)

// PasskeyChallenge は登録 / 追加登録 / 認証で発行された challenge の永続化用ドメイン型である。
//
// 生 challenge 値は保存せず ChallengeHash（SHA-256 hex）のみを保持し、ログ・エラー・
// レスポンスに平文を出さない（NFR 1.2）。単回利用性は Consumed フラグを atomic UPDATE で
// 遷移させることで保証する（Req 4.3）。TTL 検証は ExpiresAt を用いる（Req 4.1 / 4.2）。
type PasskeyChallenge struct {
	ID              string               // UUID（クライアントへ返す opaque challenge_id）
	ChallengeHash   string               // SHA-256 hex（HashNativeSecret 流儀）
	Kind            PasskeyChallengeKind // 種別
	UserID          *string              // registration_add / authentication で解決済みなら非 nil
	PendingUsername *string              // registration_new のみ設定（finish 時に users 行確定に使用）
	SessionData     []byte               // go-webauthn の webauthn.SessionData JSON marshaled
	ExpiresAt       time.Time            // 有効期限（絶対時刻）
	Consumed        bool                 // true なら消費済み（単回利用）
	CreatedAt       time.Time            // 発行時刻
}
