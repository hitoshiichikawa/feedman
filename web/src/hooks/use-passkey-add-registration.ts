"use client";

import {
  useMutation,
  type UseMutationResult,
} from "@tanstack/react-query";
import {
  apiClient,
  ApiError,
  RequestPreparationError,
  ResponseParseError,
} from "@/lib/api";
import {
  decodeCreationOptions,
  encodeAttestationResponse,
} from "@/lib/webauthn";
import type {
  PasskeyBeginResponse,
  PasskeyFinishRequest,
} from "@/types/passkey";

/**
 * 追加パスキー登録 mutation で発生しうるエラー種別。
 *
 * 既存の `PasskeyRegistrationErrorKind`（新規作成用）は
 * `InvalidStateError` を `cancelled` に集約するが、追加登録では excludeCredentials
 * による「同一 authenticator の重複登録」を区別してユーザーに提示する必要がある
 * （Req 3.1 / 3.4）。そのため本 spec 専用の `already_registered` を独立列挙する。
 *
 * - `cancelled`: ブラウザのパスキー作成 UI 上でユーザーが Cancel した / dismiss した
 *   （`NotAllowedError` / `AbortError`）。UI は「画面を壊さず戻す」（Req 4.1）
 * - `already_registered`: `excludeCredentials` で拒否された（`InvalidStateError`）。
 *   ブラウザが finish 前に拒否するため、サーバの finish endpoint を呼ばない（Req 3.2）
 * - `server_rejected`: サーバの finish endpoint が 4xx で拒否した
 *   （REGISTRATION_FAILED / チャレンジ期限切れ / attestation 不正 / レート制限等）。
 *   内部区別は UI に反射しない（Req 4.2 / NFR 2.2）
 * - `server_error`: 5xx / preparation 失敗 / decode 失敗 等の再試行可能な内部エラー
 * - `network_error`: fetch reject / AbortError による通信不通
 */
export type PasskeyAddRegistrationErrorKind =
  | "cancelled"
  | "already_registered"
  | "server_rejected"
  | "server_error"
  | "network_error";

/**
 * `usePasskeyAddRegistration` mutation が throw するドメインエラー。
 *
 * サーバ拒否理由の内部詳細（`ApiError.body`）は message に反射せず、`kind` の enum で
 * のみ表現する（NFR 2.2）。UI は `kind` を判別材料として、内部エラー詳細を DOM に
 * 出さないこと。
 */
export class PasskeyAddRegistrationError extends Error {
  readonly kind: PasskeyAddRegistrationErrorKind;
  constructor(
    kind: PasskeyAddRegistrationErrorKind,
    options?: { message?: string },
  ) {
    super(options?.message ?? kind);
    this.name = "PasskeyAddRegistrationError";
    this.kind = kind;
  }
}

// begin / browser create / finish の発生位置ごとにエラーを分類するための step index。
const STEP_ADD_BEGIN = 1;
const STEP_NAV_CREATE = 2;
const STEP_ADD_FINISH = 3;

// `navigator.credentials.create()` の DOMException のうち、ユーザーキャンセル系として
// 「画面を壊さず戻す」（Req 4.1）に集約する name の集合。既存 `use-passkey-registration`
// は `InvalidStateError` もこの集合に含めるが、本フックでは重複を独立 kind に区別する
// ため、意図的に含めない。
const CANCEL_ERROR_NAMES = new Set(["NotAllowedError", "AbortError"]);

/**
 * `navigator.credentials.create()` が throw する DOMException を判別する。
 *
 * jsdom / 本番ブラウザ双方で DOMException が定義されない状況を考慮し、
 * `instanceof DOMException` に加えて `err.name` によるフォールバック判定を持つ
 * （既存 `use-passkey-registration` と同じ idiom）。
 */
function isDomExceptionWithName(err: unknown, name: string): boolean {
  if (typeof DOMException !== "undefined" && err instanceof DOMException) {
    return err.name === name;
  }
  return err instanceof Error && err.name === name;
}

/** ユーザーキャンセル系のブラウザ拒否か判定する（重複エラーは含めない）。 */
function isCancelledError(err: unknown): boolean {
  if (typeof DOMException !== "undefined" && err instanceof DOMException) {
    return CANCEL_ERROR_NAMES.has(err.name);
  }
  return err instanceof Error && CANCEL_ERROR_NAMES.has(err.name);
}

/**
 * `navigator.credentials.create()` が excludeCredentials 由来で拒否したかを判定する。
 *
 * WebAuthn 仕様上、「同一アカウントに既に登録済みの authenticator」がブラウザ側で
 * 拒否される場合は `InvalidStateError` (DOMException) が throw される。
 */
function isAlreadyRegisteredError(err: unknown): boolean {
  return isDomExceptionWithName(err, "InvalidStateError");
}

/**
 * finish の fetch 中断（AbortController 相当）を判別する。
 * create 側の UI キャンセルとは区別し、通信断として `network_error` に集約する。
 */
function isAbortError(err: unknown): boolean {
  return isDomExceptionWithName(err, "AbortError");
}

/**
 * 内部例外を `PasskeyAddRegistrationError` に分類する。step index に基づき、
 *
 * - STEP_NAV_CREATE の `InvalidStateError` → `already_registered`（Req 3）
 * - STEP_NAV_CREATE の `NotAllowedError` / `AbortError` → `cancelled`（Req 4.1）
 * - STEP_ADD_FINISH の 4xx → `server_rejected`（Req 4.2）
 * - STEP_ADD_FINISH の 5xx / fetch reject / AbortError → `network_error` / `server_error`
 *   （Req 4.3。追加登録は begin から再試行することで整合を復元できるため、finish の
 *   uncertain 状態を独立 kind で扱わない）
 * - preparation 失敗 / decode の TypeError → `server_error`
 * - STEP_ADD_BEGIN の fetch reject → `network_error`
 *
 * NFR 2.2: 内部エラー詳細（ApiError.body / status など）を UI に反射しないため、
 * kind の列挙のみで表現し message には status / body 内容を書き込まない。
 */
function classifyError(
  err: unknown,
  step: number,
): PasskeyAddRegistrationError {
  if (err instanceof PasskeyAddRegistrationError) {
    return err;
  }

  if (step === STEP_NAV_CREATE) {
    if (isAlreadyRegisteredError(err)) {
      return new PasskeyAddRegistrationError("already_registered");
    }
    if (isCancelledError(err)) {
      return new PasskeyAddRegistrationError("cancelled");
    }
    // decodeCreationOptions が TypeError を投げる場合（サーバ応答が不正）を含む
    if (err instanceof TypeError) {
      return new PasskeyAddRegistrationError("server_error");
    }
    return new PasskeyAddRegistrationError("server_error");
  }

  if (step === STEP_ADD_FINISH) {
    // dispatch 前の preparation 失敗はサーバ側に届いていないことが確定
    if (err instanceof RequestPreparationError) {
      return new PasskeyAddRegistrationError("server_error");
    }
    // 追加登録は fetch reject / abort でも再試行時に excludeCredentials が
    // ブラウザ側で重複を弾くため、uncertain 状態を独立 kind として扱わず
    // 通信不通として network_error に集約する（Req 4.3）
    if (isAbortError(err) || err instanceof TypeError) {
      return new PasskeyAddRegistrationError("network_error");
    }
    if (err instanceof ApiError && err.status >= 500) {
      return new PasskeyAddRegistrationError("server_error");
    }
    if (err instanceof ApiError && err.status >= 400 && err.status < 500) {
      // finish の 4xx は拒否確定。status/code の内部差は UI に反射しない
      return new PasskeyAddRegistrationError("server_rejected");
    }
    // 204 no-content の場合 apiClient は undefined を返すため、ここには到達しない。
    // ResponseParseError（サーバが 2xx で JSON を返しつつ parse 失敗した稀ケース）は
    // 追加登録の性質上、実際にはサーバが 204 を返す契約のため通常発生しない。
    // 発生した場合は再試行可能な server_error として扱う。
    if (err instanceof ResponseParseError) {
      return new PasskeyAddRegistrationError("server_error");
    }
    return new PasskeyAddRegistrationError("server_error");
  }

  // STEP_ADD_BEGIN 系
  if (
    err instanceof RequestPreparationError ||
    err instanceof ResponseParseError
  ) {
    return new PasskeyAddRegistrationError("server_error");
  }
  if (err instanceof TypeError) {
    // fetch reject（ネットワーク断・DNS 失敗等）
    return new PasskeyAddRegistrationError("network_error");
  }
  if (err instanceof ApiError) {
    if (err.status >= 500) {
      return new PasskeyAddRegistrationError("server_error");
    }
    return new PasskeyAddRegistrationError("server_rejected");
  }
  return new PasskeyAddRegistrationError("server_error");
}

/**
 * 追加パスキー登録の mutation chain を 1 フックで提供する（Issue #216 の add endpoint 配線）。
 *
 * サーバ既存契約に従い以下 3 ステップを直列に実行する:
 *
 *   1. `POST /api/passkey/registration/add/begin` (空ボディ `{}`)
 *      → `{challenge_id, options}`。options は excludeCredentials を含む WebAuthn
 *        creation options。
 *   2. `decodeCreationOptions(options)` → `navigator.credentials.create({publicKey})`
 *      → attestation。excludeCredentials で重複拒否された場合は `InvalidStateError`
 *        が throw され、finish endpoint は呼ばれない（Req 3.2）
 *   3. `encodeAttestationResponse(cred)` →
 *      `POST /api/passkey/registration/add/finish { challenge_id, credential }`
 *      → 204 No Content。既存 `apiClient` は 204 を JSON parse しないため
 *        `void` 型で受ける（`web/src/lib/api.ts` L114-121）。
 *
 * 認証済み Cookie session（`credentials: "include"`）が既に成立している認証済みユーザーが
 * 対象。本フックは session を変えないため `auth/me` を invalidate しない（Req 2.5）。
 *
 * NFR 2.1 遵守: challenge 平文・attestation 生バイトは `mutationFn` の closure 変数のみに
 * 保持し、`console.*` / `localStorage` / `sessionStorage` / URL クエリ / エラー message に
 * 一切残さない。mutation 終了で closure が GC 対象になることを前提とする。
 *
 * NFR 2.3 遵守: `apiClient` を経由することで同一オリジン相対パス（`API_BASE_URL = ""`）
 * に閉じる（`web/src/lib/api.ts` の既存契約）。
 *
 * NFR 3.1 遵守: `decodeCreationOptions` / `encodeAttestationResponse` は既存
 * `web/src/lib/webauthn.ts` を再利用し、同等機能を新規実装しない。
 */
export function usePasskeyAddRegistration(): UseMutationResult<
  void,
  PasskeyAddRegistrationError,
  void
> {
  return useMutation<void, PasskeyAddRegistrationError, void>({
    mutationFn: async () => {
      let step = STEP_ADD_BEGIN;
      try {
        const beginResp = await apiClient.post<PasskeyBeginResponse>(
          "/api/passkey/registration/add/begin",
          {},
        );

        step = STEP_NAV_CREATE;
        const creationOptions = decodeCreationOptions(beginResp.options);
        const attestation = (await navigator.credentials.create(
          creationOptions,
        )) as PublicKeyCredential | null;
        if (!attestation) {
          // ブラウザ側の null 解決（キャンセル相当）を「壊さずに戻す」に集約
          throw new PasskeyAddRegistrationError("cancelled");
        }

        step = STEP_ADD_FINISH;
        const finishReq: PasskeyFinishRequest = {
          challenge_id: beginResp.challenge_id,
          credential: encodeAttestationResponse(attestation),
        };
        // 追加登録 finish は 204 No Content を返す。apiClient は 204 を JSON parse
        // しないため `void` 型として受ける。
        await apiClient.post<void>(
          "/api/passkey/registration/add/finish",
          finishReq,
        );
      } catch (err) {
        throw classifyError(err, step);
      }
    },
  });
}
