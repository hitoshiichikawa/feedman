/**
 * PKCE (RFC 7636) の code_verifier / code_challenge (S256) をブラウザ WebCrypto で生成する
 * 純粋 utility。
 *
 * サーバ側（`internal/auth/pkce.go` の `VerifyPKCES256Verifier`）は
 * `sha256.Sum256([]byte(verifier))` を計算する。ここで入力される `verifier` は
 * code_verifier "文字列" そのものである。本モジュールは対称に、code_verifier 文字列
 * （43 文字 base64url）を `TextEncoder` でエンコードしたバイト列を SHA-256 に入力し、
 * その結果を base64url 化して code_challenge を得る。生成された 32 バイト crypto
 * random を直接 SHA-256 に食わせるとサーバと一致しないため注意。
 *
 * 副作用なし・throw なし（WebCrypto API 由来の rejection はそのまま呼び出し側に伝播）。
 * 生成値は本モジュール内で保持せず、返却値としてのみ渡す（NFR 1.1）。
 */

/** PKCE code_verifier と code_challenge (S256) の組。 */
export interface PkcePair {
  /** 32 バイト crypto random を base64url 化した 43 文字文字列（無 padding） */
  codeVerifier: string;
  /** SHA-256(code_verifier as UTF-8 bytes) を base64url 化した 43 文字文字列（無 padding） */
  codeChallenge: string;
}

/**
 * Uint8Array を base64url 文字列（`+/=` を含まない）にエンコードする内部ヘルパ。
 *
 * `btoa` は Latin-1 のみ受け付けるため、まず `String.fromCharCode` で byte-per-char の
 * バイナリ文字列に組み直してから base64 化し、base64 → base64url 差分（`+`→`-`,
 * `/`→`_`, 末尾 `=` 除去）を適用する。
 */
function bytesToBase64url(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const b64 = btoa(binary);
  return b64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * code_verifier 文字列から PKCE code_challenge (S256) を導出する純粋関数。
 *
 * サーバ側 `internal/auth/pkce.go` の `sha256.Sum256([]byte(verifier))` に対称の入力を
 * 与えるため、`TextEncoder.encode(codeVerifier)`（= UTF-8/ASCII バイト列）を
 * `crypto.subtle.digest("SHA-256", ...)` に渡す。同関数を独立に export しておくことで
 * RFC 7636 Appendix B の既知ベクトル（固定 verifier → 期待 challenge）で SHA-256 派生
 * の正しさを検証できる（`generatePkcePair` は乱数入力のため直接には検証不能）。
 *
 * @param codeVerifier 43〜128 文字の RFC 7636 §4.1 code_verifier 文字列
 * @returns S256 派生の base64url 化 code_challenge（43 文字、無 padding）
 */
export async function deriveCodeChallengeS256(codeVerifier: string): Promise<string> {
  const verifierBytes = new TextEncoder().encode(codeVerifier);
  const digest = await crypto.subtle.digest("SHA-256", verifierBytes);
  return bytesToBase64url(new Uint8Array(digest));
}

/**
 * PKCE code_verifier / code_challenge (S256) をブラウザ WebCrypto で生成する。
 *
 * - code_verifier: `crypto.getRandomValues(new Uint8Array(32))` の base64url 化（43 文字）
 * - code_challenge: `deriveCodeChallengeS256(codeVerifier)`（= サーバ検証と対称の入力を SHA-256）
 *
 * 生成値は本関数の返却値としてのみ渡され、モジュール内に保持しない（NFR 1.1）。
 * WebCrypto API 側の rejection は握り潰さず呼び出し側に伝播する。
 */
export async function generatePkcePair(): Promise<PkcePair> {
  const randomBytes = new Uint8Array(32);
  crypto.getRandomValues(randomBytes);
  const codeVerifier = bytesToBase64url(randomBytes);
  const codeChallenge = await deriveCodeChallengeS256(codeVerifier);
  return { codeVerifier, codeChallenge };
}
