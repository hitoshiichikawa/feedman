/**
 * ブラウザ側の WebAuthn (パスキー) 対応判定を提供する純粋 utility。
 *
 * サーバ側の capability endpoint (`GET /api/passkey/capability`) と併用して、
 * `use-passkey-capability` hook で「サーバ + ブラウザ」両対応の合成 boolean を作る。
 *
 * SSR / Next.js の Server Components からも呼び出されうるため、`window` 未定義
 * 環境では早期に false を返して throw しない。副作用なし・throw なし・boolean を
 * 返すのみ（NFR 1.1: 生バイト列・機密値は本関数に一切関与しない）。
 */

/**
 * ブラウザが WebAuthn（`navigator.credentials.create/get` + `PublicKeyCredential`）を
 * 提供しているかを判定する。
 *
 * 判定条件（design.md §lib/passkey-capability.ts / Requirement 5.1・5.4）:
 * - `typeof window !== "undefined"` — SSR 環境では false
 * - `typeof window.PublicKeyCredential === "function"` — polyfill されない object 型
 *   の存在は WebAuthn 実行機能の提供とみなさず false
 *
 * `PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable` の存在確認は
 * 本関数では行わない（Face ID / Touch ID なし環境でも導線を非活性にしないため /
 * design.md §Responsibilities & Constraints）。
 *
 * @returns ブラウザが WebAuthn を提供しているとき true、そうでなければ false
 */
export function isPasskeyBrowserSupported(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return typeof window.PublicKeyCredential === "function";
}
