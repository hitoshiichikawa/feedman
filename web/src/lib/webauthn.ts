/**
 * WebAuthn の options (サーバ返却 JSON) と `navigator.credentials.create/get` の
 * 引数（`ArrayBuffer`）の相互変換、および `PublicKeyCredential` の応答を
 * サーバ finish endpoint 用 JSON にシリアライズする純粋 utility。
 *
 * サーバ側 (`internal/handler/passkey_handler.go` + go-webauthn の
 * `protocol.ParseCredentialCreationResponseBytes` /
 * `ParseCredentialRequestResponseBytes`) が要求する JSON 形式に合わせて
 * top-level `id` (base64url 文字列) / `type` ("public-key") / `rawId` (base64url) /
 * `response` フィールド群 (すべて base64url) を出力する。
 *
 * 副作用なし・throw なし（形式不正はドメイン境界で TypeError を throw）。
 * サーバから受け取った base64url 生値・credential 生バイトは本モジュール内で
 * `console.*` に出力しない（NFR 1.1）。
 *
 * 依存追加なし: base64url ↔ ArrayBuffer 変換はブラウザ標準 `atob` / `btoa` /
 * `String.fromCharCode` / `Uint8Array` のみで実装する。
 *
 * base64url ↔ Uint8Array の変換ロジックは `web/src/lib/pkce.ts` の private helper
 * (`bytesToBase64url`) と類似するが、本 file の Boundary は `lib/webauthn` に
 * 限定されており `pkce.ts` の Boundary 外を編集する変更は行わない。共有化は
 * `pkce.ts` を書き換える別 spec の責務として据え置く（詳細は impl-notes.md
 * Task 5 の残存課題を参照）。
 */

/**
 * base64url 文字列を ArrayBuffer にデコードする（padding 有無どちらも許容）。
 *
 * base64url → base64（`-`→`+`, `_`→`/`）に変換し、必要な `=` padding を補完してから
 * ブラウザ標準 `atob` でデコードする。`atob` は形式不正な入力に対し
 * `InvalidCharacterError` (DOMException) を throw するため、その throw は
 * 呼び出し側にそのまま伝播させる（形式不正はドメイン境界で throw する契約）。
 *
 * @param b64url base64url エンコード文字列（padding なし想定だが padding 付きも許容）
 * @returns デコードされた ArrayBuffer
 * @throws TypeError 入力が string でない場合
 */
export function base64urlToArrayBuffer(b64url: string): ArrayBuffer {
  if (typeof b64url !== "string") {
    throw new TypeError("base64urlToArrayBuffer: input must be a string");
  }
  const b64 = b64url.replace(/-/g, "+").replace(/_/g, "/");
  const pad = b64.length % 4 === 0 ? "" : "=".repeat(4 - (b64.length % 4));
  const binary = atob(b64 + pad);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

/**
 * ArrayBuffer / Uint8Array を base64url 文字列（padding なし）にエンコードする。
 *
 * `btoa` は Latin-1 のみ受け付けるため、`String.fromCharCode` で byte-per-char の
 * バイナリ文字列に組み直してから base64 化し、base64 → base64url 差分
 * （`+`→`-`, `/`→`_`, 末尾 `=` 除去）を適用する。
 *
 * @param buf ArrayBuffer もしくは Uint8Array
 * @returns base64url エンコード文字列（無 padding）
 */
export function arrayBufferToBase64url(buf: ArrayBuffer | Uint8Array): string {
  const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const b64 = btoa(binary);
  return b64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * サーバ返却 options JSON (`{publicKey: {...}}`) を
 * `navigator.credentials.create()` 引数の `CredentialCreationOptions` に変換する。
 *
 * 変換対象:
 * - `publicKey.challenge` (base64url) → `ArrayBuffer`
 * - `publicKey.user.id` (base64url) → `ArrayBuffer`
 * - `publicKey.excludeCredentials[].id` (base64url) → `ArrayBuffer`
 *
 * 他フィールド（`rp` / `user.name` / `user.displayName` / `pubKeyCredParams` /
 * `authenticatorSelection` / `attestation` / `timeout` / `extensions` 等）は
 * 透過的にコピーする。
 *
 * @param raw サーバ返却の options JSON（`{publicKey: {...}}` 形式）
 * @returns navigator.credentials.create() に渡せる `CredentialCreationOptions`
 * @throws TypeError raw が期待する形式でない場合（publicKey 不在 / challenge 非文字列等）
 */
export function decodeCreationOptions(raw: unknown): CredentialCreationOptions {
  const src = extractPublicKey(raw, "decodeCreationOptions");
  const userSrc = requireObject(
    src.user,
    "decodeCreationOptions: publicKey.user must be an object",
  );
  const converted: Record<string, unknown> = {
    ...src,
    challenge: base64urlToArrayBuffer(
      requireString(
        src.challenge,
        "decodeCreationOptions: publicKey.challenge must be a base64url string",
      ),
    ),
    user: {
      ...userSrc,
      id: base64urlToArrayBuffer(
        requireString(
          userSrc.id,
          "decodeCreationOptions: publicKey.user.id must be a base64url string",
        ),
      ),
    },
  };
  if (Array.isArray(src.excludeCredentials)) {
    converted.excludeCredentials = src.excludeCredentials.map((cred, idx) =>
      decodeCredentialDescriptor(cred, `decodeCreationOptions: publicKey.excludeCredentials[${idx}]`),
    );
  }
  return { publicKey: converted as unknown as PublicKeyCredentialCreationOptions };
}

/**
 * サーバ返却 options JSON (`{publicKey: {...}}`) を
 * `navigator.credentials.get()` 引数の `CredentialRequestOptions` に変換する。
 *
 * 変換対象:
 * - `publicKey.challenge` (base64url) → `ArrayBuffer`
 * - `publicKey.allowCredentials[].id` (base64url) → `ArrayBuffer`
 *
 * 他フィールド（`rpId` / `userVerification` / `timeout` / `extensions` 等）は
 * 透過的にコピーする。discoverable login では allowCredentials 自体が省略される
 * ため、Array 判定で存在確認してから変換する。
 *
 * @param raw サーバ返却の options JSON（`{publicKey: {...}}` 形式）
 * @returns navigator.credentials.get() に渡せる `CredentialRequestOptions`
 * @throws TypeError raw が期待する形式でない場合
 */
export function decodeRequestOptions(raw: unknown): CredentialRequestOptions {
  const src = extractPublicKey(raw, "decodeRequestOptions");
  const converted: Record<string, unknown> = {
    ...src,
    challenge: base64urlToArrayBuffer(
      requireString(
        src.challenge,
        "decodeRequestOptions: publicKey.challenge must be a base64url string",
      ),
    ),
  };
  if (Array.isArray(src.allowCredentials)) {
    converted.allowCredentials = src.allowCredentials.map((cred, idx) =>
      decodeCredentialDescriptor(cred, `decodeRequestOptions: publicKey.allowCredentials[${idx}]`),
    );
  }
  return { publicKey: converted as unknown as PublicKeyCredentialRequestOptions };
}

/**
 * `navigator.credentials.create()` の結果 (`PublicKeyCredential`) を、サーバの
 * registration/finish endpoint が期待する JSON にエンコードする。
 *
 * 出力形状（サーバ go-webauthn `CredentialCreationResponse.Parse()` に対称。
 * 同 Parse は `id == ""` と `type != "public-key"` を reject するため top-level
 * `id` / `type` を必ず含める）:
 * - `id`: cred.id（ブラウザ側で既に base64url 文字列）
 * - `type`: cred.type（常に "public-key"）
 * - `rawId`: cred.rawId を base64url 化
 * - `response.clientDataJSON`: base64url
 * - `response.attestationObject`: base64url
 *
 * @param cred `navigator.credentials.create()` の返り値
 * @returns finish endpoint の `credential` フィールドとして送信する JSON
 */
export function encodeAttestationResponse(cred: PublicKeyCredential): unknown {
  const response = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    type: cred.type,
    rawId: arrayBufferToBase64url(cred.rawId),
    response: {
      clientDataJSON: arrayBufferToBase64url(response.clientDataJSON),
      attestationObject: arrayBufferToBase64url(response.attestationObject),
    },
  };
}

/**
 * `navigator.credentials.get()` の結果 (`PublicKeyCredential`) を、サーバの
 * authentication/finish endpoint が期待する JSON にエンコードする。
 *
 * 出力形状（サーバ go-webauthn `CredentialAssertionResponse` に対称。Parse は
 * `id == ""` / `type != "public-key"` を reject するため top-level `id` / `type`
 * を必ず含める）:
 * - `id`: cred.id
 * - `type`: cred.type
 * - `rawId`: base64url
 * - `response.clientDataJSON`: base64url
 * - `response.authenticatorData`: base64url
 * - `response.signature`: base64url
 * - `response.userHandle`: base64url（null / undefined ならフィールド自体を省略。
 *   go-webauthn の `UserHandle` は `omitempty` 相当）
 *
 * @param cred `navigator.credentials.get()` の返り値
 * @returns finish endpoint の `credential` フィールドとして送信する JSON
 */
export function encodeAssertionResponse(cred: PublicKeyCredential): unknown {
  const response = cred.response as AuthenticatorAssertionResponse;
  const responseJson: Record<string, unknown> = {
    clientDataJSON: arrayBufferToBase64url(response.clientDataJSON),
    authenticatorData: arrayBufferToBase64url(response.authenticatorData),
    signature: arrayBufferToBase64url(response.signature),
  };
  if (response.userHandle) {
    responseJson.userHandle = arrayBufferToBase64url(response.userHandle);
  }
  return {
    id: cred.id,
    type: cred.type,
    rawId: arrayBufferToBase64url(cred.rawId),
    response: responseJson,
  };
}

// --- private helpers ---

/**
 * `raw.publicKey` を取り出し、オブジェクトであることを検証して返す。
 * `raw` 自体または `publicKey` が欠落・非オブジェクトなら TypeError。
 */
function extractPublicKey(raw: unknown, fnName: string): Record<string, unknown> {
  const obj = requireObject(raw, `${fnName}: input must be an object with publicKey`);
  return requireObject(obj.publicKey, `${fnName}: publicKey must be an object`);
}

/**
 * `PublicKeyCredentialDescriptor` 相当の `{type, id, transports?}` を検証し、
 * `id` を base64url → ArrayBuffer に変換した新オブジェクトを返す。
 */
function decodeCredentialDescriptor(cred: unknown, context: string): Record<string, unknown> {
  const c = requireObject(cred, `${context} must be an object`);
  return {
    ...c,
    id: base64urlToArrayBuffer(requireString(c.id, `${context}.id must be a base64url string`)),
  };
}

function requireObject(value: unknown, message: string): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new TypeError(message);
  }
  return value as Record<string, unknown>;
}

function requireString(value: unknown, message: string): string {
  if (typeof value !== "string") {
    throw new TypeError(message);
  }
  return value;
}
