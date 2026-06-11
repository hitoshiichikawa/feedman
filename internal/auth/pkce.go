package auth

import (
	"errors"
	"regexp"
)

// pkceS256ChallengePattern は RFC 7636 §4.2 の S256 code_challenge 形式。
// challenge = BASE64URL-ENCODE(SHA256(verifier)) は padding なし 43 文字に固定される。
var pkceS256ChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

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
