"use client";

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import { apiClient, ApiError } from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
import {
  decodeRequestOptions,
  encodeAssertionResponse,
} from "@/lib/webauthn";
import type {
  AuthenticationFinishResponse,
  PasskeyBeginResponse,
} from "@/types/passkey";

/**
 * パスキーログイン mutation で発生しうるエラー種別。
 *
 * design.md §Components (use-passkey-authentication.ts) では 4 種のみ例示されているが、
 * tasks.md L155 と design.md §Error Handling L1042 で追加要件化された
 * `session_exchange_failed`（`POST /api/auth/session` 失敗時）を含めた 5 種を扱う。
 * UI は本 kind の enum に対して文言・復帰動作を分岐する（Requirement 3.4 / 4.6 / 4.7）。
 */
export type PasskeyAuthErrorKind =
  | "cancelled"
  | "authentication_failed"
  | "server_error"
  | "server_rejected"
  | "network_error"
  | "session_exchange_failed";

/**
 * `usePasskeyAuthentication` mutation が throw するドメインエラー。
 *
 * サーバ拒否理由の内部詳細（`ApiError.body`）は message に反射せず、`kind` の enum で
 * のみ表現する（NFR 1.2「サーバ内部詳細をユーザー画面・console に反射しない」）。
 * 消費側の UI は `kind` を判別材料として、内部エラー詳細を DOM に出さないこと。
 */
export class PasskeyAuthError extends Error {
  readonly kind: PasskeyAuthErrorKind;
  constructor(kind: PasskeyAuthErrorKind, message?: string) {
    super(message ?? kind);
    this.name = "PasskeyAuthError";
    this.kind = kind;
  }
}

// step 追跡用のシンボリック定数。session 交換（step 5）の失敗のみを
// `session_exchange_failed` に分類し、それ以外の step の ApiError は
// `server_rejected` / `server_error` に振り分けるためのラベル。
const STEP_PKCE = 1;
const STEP_AUTH_BEGIN = 2;
const STEP_NAV_GET = 3;
const STEP_AUTH_FINISH = 4;
const STEP_SESSION_EXCHANGE = 5;

// WebAuthn の DOMException のうち、ユーザーキャンセル / abort / 起動失敗として
// UI 側で「画面を壊さず戻す」（Requirement 4.5）に集約する name の集合。
const CANCEL_ERROR_NAMES = new Set([
  "NotAllowedError",
  "AbortError",
  "InvalidStateError",
]);

/**
 * `navigator.credentials.get()` が throw するキャンセル系エラーを判別する。
 *
 * jsdom / 本番ブラウザ双方で DOMException が定義されない状況（SSR 相当 / 古いランタイム）
 * を考慮し、`instanceof DOMException` に加えて `err.name` によるフォールバック判定を持つ。
 */
function isCancelledError(err: unknown): boolean {
  if (typeof DOMException !== "undefined" && err instanceof DOMException) {
    return CANCEL_ERROR_NAMES.has(err.name);
  }
  return err instanceof Error && CANCEL_ERROR_NAMES.has(err.name);
}

/** ApiError body から machine-readable code 文字列だけを安全に取り出す。 */
function extractApiErrorCode(err: ApiError): string | null {
  const body = err.body;
  if (typeof body !== "object" || body === null || !("code" in body)) {
    return null;
  }
  const code = (body as { code: unknown }).code;
  return typeof code === "string" ? code : null;
}

/**
 * 内部例外を `PasskeyAuthError` に分類する。step index に基づき、session 交換
 * （step 5）の `ApiError` のみを `session_exchange_failed` に振り分ける
 * （Requirement 3.4 / tasks.md L155）。
 */
function classifyError(err: unknown, step: number): PasskeyAuthError {
  if (err instanceof PasskeyAuthError) {
    return err;
  }
  if (isCancelledError(err)) {
    return new PasskeyAuthError("cancelled");
  }
  if (err instanceof TypeError) {
    // fetch reject（ネットワーク断・DNS 失敗等）
    return new PasskeyAuthError("network_error");
  }
  if (err instanceof ApiError) {
    if (
      step === STEP_AUTH_FINISH &&
      err.status === 400 &&
      extractApiErrorCode(err) === "AUTHENTICATION_FAILED"
    ) {
      // 復旧 UI が再作成を提示してよい唯一の観察可能な失敗。
      // raw body や credential 未解決等の内部理由は Error に含めない。
      return new PasskeyAuthError("authentication_failed");
    }
    if (step === STEP_SESSION_EXCHANGE) {
      // step 5 の失敗は理由に関わらず session 合流失敗に集約
      return new PasskeyAuthError("session_exchange_failed");
    }
    if (err.status === 400 || err.status === 409) {
      // 400 AUTHENTICATION_FAILED / 409 系は server_rejected（内部区別を反射しない）
      return new PasskeyAuthError("server_rejected");
    }
    // 500 系および想定外 status は server_error
    return new PasskeyAuthError("server_error");
  }
  // その他の予期しない例外は安全側に server_error とし、詳細を UI に露出させない
  return new PasskeyAuthError("server_error");
}

/**
 * パスキーログインの mutation chain を 1 フックで提供する。
 *
 * chain 5 段（design.md §Flows「ログインフロー」）:
 *   1. `generatePkcePair()` — code_verifier / code_challenge を closure 変数で生成
 *   2. `POST /api/passkey/authentication/begin { code_challenge }` → `{challenge_id, options}`
 *   3. `decodeRequestOptions(options)` → `navigator.credentials.get({publicKey})` → assertion
 *   4. `encodeAssertionResponse(cred)` → `POST /api/passkey/authentication/finish` → `{auth_code}`
 *   5. `POST /api/auth/session { auth_code, code_verifier }` → 204 + Set-Cookie
 *
 * 成功時: `queryClient.invalidateQueries({queryKey: ["auth", "me"]})` で `AuthGuard` を
 * 再判定させ、2 ペイン UI へ自動遷移させる（Requirement 4.2 / 4.3）。
 *
 * NFR 1.1 遵守:
 * - `codeVerifier` / `auth_code` / assertion 生値は `mutationFn` の closure 変数のみに保持
 * - `console.*` / `localStorage` / `sessionStorage` / URL クエリ / エラー message に一切残さない
 * - mutation 終了（成功・失敗・cancel）で closure が GC 対象になることを前提とする
 *
 * NFR 1.4 遵守: `apiClient` を経由することで同一オリジン相対パス（`API_BASE_URL = ""`）に
 * 閉じる（`web/src/lib/api.ts` の既存契約）。
 */
export function usePasskeyAuthentication(): UseMutationResult<
  void,
  PasskeyAuthError,
  void
> {
  const queryClient = useQueryClient();
  return useMutation<void, PasskeyAuthError, void>({
    mutationFn: async () => {
      let step = STEP_PKCE;
      try {
        const { codeVerifier, codeChallenge } = await generatePkcePair();

        step = STEP_AUTH_BEGIN;
        const begin = await apiClient.post<PasskeyBeginResponse>(
          "/api/passkey/authentication/begin",
          { code_challenge: codeChallenge },
        );

        step = STEP_NAV_GET;
        const requestOptions = decodeRequestOptions(begin.options);
        const cred = (await navigator.credentials.get(
          requestOptions,
        )) as PublicKeyCredential | null;
        if (!cred) {
          // Requirement 4.5: ブラウザ側の null 解決（キャンセル相当）を「壊さずに戻す」に集約
          throw new PasskeyAuthError("cancelled");
        }

        step = STEP_AUTH_FINISH;
        const finish = await apiClient.post<AuthenticationFinishResponse>(
          "/api/passkey/authentication/finish",
          {
            challenge_id: begin.challenge_id,
            credential: encodeAssertionResponse(cred),
          },
        );

        step = STEP_SESSION_EXCHANGE;
        await apiClient.post<void>("/api/auth/session", {
          auth_code: finish.auth_code,
          code_verifier: codeVerifier,
        });
      } catch (err) {
        throw classifyError(err, step);
      }
    },
    onSuccess: () => {
      // AuthGuard を再判定させ、既存 Cookie session に基づき 2 ペイン UI に遷移させる
      queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });
}
