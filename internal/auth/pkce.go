package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"regexp"
)

// pkceS256ChallengePattern は RFC 7636 §4.2 の S256 code_challenge 形式。
// challenge = BASE64URL-ENCODE(SHA256(verifier)) は padding なし 43 文字に固定される。
var pkceS256ChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// pkceS256VerifierPattern は RFC 7636 §4.1 の code_verifier 形式。
// unreserved 文字（ALPHA / DIGIT / "-" / "." / "_" / "~"）で構成され、長さは 43〜128 文字。
// 本実装は RFC が定義する文字集合に厳密準拠する。
var pkceS256VerifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// ValidatePKCES256 は flow=native の PKCE パラメータを検証する。
//
// method は "S256" の厳密一致のみ受理し（plain・小文字・欠落は拒否、NFR 1.2）、
// challenge は RFC 7636 §4.2 の S256 形式（base64url no-padding、43 文字）のみ受理する。
// 合格時は nil、不合格時は理由を表す error を返す。呼び出し側は error 文字列を
// クライアントへ反射せず、固定メッセージで応答すること（NFR 1.3）。
func ValidatePKCES256(challenge, method string) error {
	if method != "S256" {
		return errors.New("pkce method must be S256")
	}
	if challenge == "" {
		return errors.New("pkce challenge is required")
	}
	if !pkceS256ChallengePattern.MatchString(challenge) {
		return errors.New("pkce challenge is not a valid S256 challenge")
	}
	return nil
}

// VerifyPKCES256Verifier は POST /api/auth/token の交換時に提示された code_verifier が、
// auth_code 発行時に保存された storedChallenge（S256 challenge）と一致するかを定数時間で検証する。
//
// RFC 7636 §4.1 の verifier 形式（unreserved 43〜128 文字）を満たさない場合は false。
// 形式が妥当な場合は SHA-256 を base64url（no-padding）でエンコードした派生値を求め、
// storedChallenge と crypto/subtle.ConstantTimeCompare で比較する（NFR 1.4: タイミング攻撃回避）。
//
// 戻り値は (一致するか否か) の bool のみで、理由を区別する情報は返さない（Req 2.6 の応答
// uniform 化に対応）。本関数は平文 verifier をログに出さないこと（NFR 1.3 は呼び出し側でも守る）。
func VerifyPKCES256Verifier(verifier, storedChallenge string) bool {
	// 形式チェックを最初に行うことで、明らかに不正な入力に対して SHA-256 計算を回避する。
	if !pkceS256VerifierPattern.MatchString(verifier) {
		return false
	}
	if storedChallenge == "" {
		return false
	}

	sum := sha256.Sum256([]byte(verifier))
	derived := base64.RawURLEncoding.EncodeToString(sum[:])

	return subtle.ConstantTimeCompare([]byte(derived), []byte(storedChallenge)) == 1
}
