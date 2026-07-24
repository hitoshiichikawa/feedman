package passkey

import "errors"

// 本ファイルはパスキー（WebAuthn）関連の sentinel error を集約する
// （Issue #216 / design.md Error Handling / errors.go）。
//
// `ErrInvalidUsername` は username 形式検証を担う `username.go` に既に定義済みの
// ため、本ファイルでは再定義せず 5 つの sentinel のみを集約する。
//
// 拒否応答は要求単位で uniform に正規化される（Req 1.7 / 2.5 / 2.6 / 3.6 / 3.7）。
// 上位（handler）はサービス層が返した sentinel を APIError に 1:1 マッピングする。
// エラーメッセージ本文には challenge_hash / credential_id / username 等の機密値・
// 入力値を反射しない（NFR 1.2 / NFR 1.3）。

var (
	// ErrUsernameTaken は登録要求のユーザー名が既存ユーザーと重複したため
	// 新規アカウントを作成しなかったことを示す（Req 1.4）。
	// handler は 409 USERNAME_TAKEN を返す。
	ErrUsernameTaken = errors.New("username taken")

	// ErrRegistrationFailed はパスキー登録 ceremony が失敗した拒否事象を
	// 理由詳細なしで表現する uniform sentinel である（Req 1.7 / 3.6 / 3.7）。
	// attestation 検証失敗・challenge 期限切れ・credential 重複などの
	// 全 4xx 拒否事象がここに正規化される。handler は 400 REGISTRATION_FAILED を返す。
	ErrRegistrationFailed = errors.New("passkey registration failed")

	// ErrAuthenticationFailed はパスキー認証 ceremony が失敗した拒否事象を
	// 理由詳細なしで表現する uniform sentinel である（Req 2.5 / 2.6 / NFR 1.4）。
	// assertion 検証失敗・user 未解決・counter 後退・challenge 期限切れ等が
	// 存在有無を区別されずここに正規化される。handler は 400 AUTHENTICATION_FAILED を返す。
	ErrAuthenticationFailed = errors.New("passkey authentication failed")

	// ErrChallengeNotUsable は challenge が二重消費・期限切れ・kind 不一致・
	// 未存在のいずれかにより再利用不能であることを示す（Req 4.3 / 4.4）。
	// repository 層の `repository.ErrChallengeNotUsable` は、`ChallengeStore.Consume`
	// が `errors.Is` で受け止め、本 sentinel に再マップしてから呼び出し側へ返す
	// （二層で個別に errors.Is する取りこぼしを防ぐ / task 2 impl-notes 参照）。
	ErrChallengeNotUsable = errors.New("challenge not usable")

	// ErrCredentialAlreadyRegistered は追加登録要求のパスキー応答が
	// 別ユーザーに既に登録済みの credential 識別子を提示したことを示す（Req 3.6）。
	// 呼び出し側（RegistrationService）はこれを ErrRegistrationFailed に再正規化して
	// handler へ返し、内部詳細を反射しない安全なエラーとして応答する。
	ErrCredentialAlreadyRegistered = errors.New("credential already registered to another user")
)
