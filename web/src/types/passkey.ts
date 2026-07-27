/**
 * サーバ側パスキー API（Issue #216 追加 + 本 spec 追加分）の request / response 型定義。
 *
 * フィールド名は Go サーバ側 DTO の `json` タグと厳密一致させる snake_case を採用する
 * （design.md §types/passkey.ts / `internal/handler/passkey_handler.go` /
 * `internal/handler/native_auth_handler.go`）。`unknown` 型で保持しているフィールド
 * （`options` / `credential`）は `web/src/lib/webauthn.ts` の encode/decode 関数が
 * base64url ↔ ArrayBuffer 変換を担う。
 */

/** POST /api/passkey/registration/begin リクエスト */
export interface RegistrationBeginRequest {
  username: string;
  email?: string;
  code_challenge: string;
}

/**
 * POST /api/passkey/registration/begin / authentication/begin レスポンス。
 *
 * `options` は `{publicKey: {...}}` 形式の生 JSON。`web/src/lib/webauthn.ts` の
 * `decodeCreationOptions` / `decodeRequestOptions` が `navigator.credentials.*` に
 * 渡せる `CredentialCreationOptions` / `CredentialRequestOptions` に変換する。
 */
export interface PasskeyBeginResponse {
  challenge_id: string;
  options: unknown;
}

/**
 * POST /api/passkey/registration/finish / authentication/finish リクエスト。
 *
 * `credential` は `web/src/lib/webauthn.ts` の `encodeAttestationResponse` /
 * `encodeAssertionResponse` が生成する base64url 化 JSON。
 */
export interface PasskeyFinishRequest {
  challenge_id: string;
  credential: unknown;
}

/** POST /api/passkey/registration/finish レスポンス */
export interface RegistrationFinishResponse {
  user_id: string;
}

/** POST /api/passkey/authentication/begin リクエスト */
export interface AuthenticationBeginRequest {
  code_challenge: string;
}

/** POST /api/passkey/authentication/finish レスポンス */
export interface AuthenticationFinishResponse {
  auth_code: string;
}

/**
 * POST /api/auth/session リクエスト（本 spec で新規追加のサーバ endpoint / task 2）。
 *
 * サーバは auth_code + code_verifier を PKCE 検証付きで単回消費し、成功時は
 * 応答ボディなしで Set-Cookie: session_id=... を返す（`credentials: "include"` の
 * fetch 経由で自動的に Cookie が保存される / design.md §NativeAuthHandler.Session）。
 */
export interface SessionExchangeRequest {
  auth_code: string;
  code_verifier: string;
}

/**
 * GET /api/passkey/capability レスポンス（本 spec で新規追加 / task 3）。
 *
 * サーバがパスキー機能を提供している構成ではハンドラが登録され 200 で
 * `{available: true}` を返す。未提供構成では route 自体が未登録で 404 になり、
 * Web 側は `use-passkey-capability` フックの catch で server=false と扱う。
 */
export interface CapabilityResponse {
  available: boolean;
}
