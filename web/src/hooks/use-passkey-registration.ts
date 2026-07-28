"use client";

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import {
  apiClient,
  ApiError,
  RequestPreparationError,
  ResponseParseError,
} from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
import {
  decodeCreationOptions,
  encodeAttestationResponse,
} from "@/lib/webauthn";
import type {
  PasskeyBeginResponse,
  RegistrationFinishResponse,
} from "@/types/passkey";

/**
 * パスキー新規作成 mutation で発生しうるエラー種別。
 *
 * design.md §Components (use-passkey-registration.ts) と §Error Handling / tasks.md
 * L182-193 に列挙された 8 種を扱う。UI は本 kind の enum に対して文言・復帰動作を
 * 分岐する（Requirement 2.5 / 2.6 / 2.7 / 2.8 / 3.4）。
 *
 * task 7（`use-passkey-authentication.ts`）の `PasskeyAuthErrorKind` の superset で、
 * 登録側固有の `invalid_username` / `username_taken` を追加する形になっている。
 */
export type PasskeyRegistrationErrorKind =
  | "invalid_username"
  | "username_taken"
  | "cancelled"
  | "server_rejected"
  | "registration_uncertain"
  | "session_exchange_failed"
  | "server_error"
  | "network_error";

/**
 * `usePasskeyRegistration` mutation が throw するドメインエラー。
 *
 * サーバ拒否理由の内部詳細（`ApiError.body`）は message に反射せず、`kind` の enum で
 * のみ表現する（NFR 1.2）。消費側の UI は `kind` を判別材料として、内部エラー詳細を
 * DOM に出さないこと。
 */
export class PasskeyRegistrationError extends Error {
  readonly kind: PasskeyRegistrationErrorKind;
  /**
   * registration/finish が commit 済みの可能性を排除できない場合は `true`。
   * `registration_uncertain` では成否を断定せず、UI がログイン確認を先に提示する。
   * 旧 post-finish エラーとの互換用にも保持する。
   */
  readonly registered: boolean;
  constructor(
    kind: PasskeyRegistrationErrorKind,
    options?: { message?: string; registered?: boolean },
  ) {
    super(options?.message ?? kind);
    this.name = "PasskeyRegistrationError";
    this.kind = kind;
    this.registered = options?.registered ?? false;
  }
}

// begin / browser create / finish の発生位置ごとに、確定拒否と完了不明を区別する。
const STEP_PKCE = 1;
const STEP_REG_BEGIN = 2;
const STEP_NAV_CREATE = 3;
const STEP_REG_FINISH = 4;

// WebAuthn の DOMException のうち、ユーザーキャンセル / abort / 起動失敗として
// UI 側で「画面を壊さず戻す」（Requirement 2.7）に集約する name の集合。
// task 7 の `use-passkey-authentication.ts` と同一集合。
const CANCEL_ERROR_NAMES = new Set([
  "NotAllowedError",
  "AbortError",
  "InvalidStateError",
]);

/**
 * `navigator.credentials.create()` が throw するキャンセル系エラーを判別する。
 *
 * jsdom / 本番ブラウザ双方で DOMException が定義されない状況（SSR 相当 / 古いランタイム）
 * を考慮し、`instanceof DOMException` に加えて `err.name` によるフォールバック判定を持つ。
 * task 7 と同 idiom（Boundary `hooks/use-passkey-registration` に閉じるためコピー）。
 */
function isCancelledError(err: unknown): boolean {
  if (typeof DOMException !== "undefined" && err instanceof DOMException) {
    return CANCEL_ERROR_NAMES.has(err.name);
  }
  return err instanceof Error && CANCEL_ERROR_NAMES.has(err.name);
}

/**
 * `ApiError.body` の `code` フィールド（サーバ side が返す error code 文字列）を
 * 安全に取り出す。body 形状不明の場合は `null` を返す。
 *
 * NFR 1.2: body の内部詳細（stack / query 等）を UI に反射しないため、`code` 列挙値の
 * 有無だけを判別材料として使う。code 以外のフィールドは参照しない。
 */
function extractApiErrorCode(err: ApiError): string | null {
  const body = err.body;
  if (typeof body === "object" && body !== null && "code" in body) {
    const code = (body as { code: unknown }).code;
    if (typeof code === "string") {
      return code;
    }
  }
  return null;
}

/**
 * finish の fetch 中断を、create 操作のユーザーキャンセルと区別する。
 */
function isAbortError(err: unknown): boolean {
  if (typeof DOMException !== "undefined" && err instanceof DOMException) {
    return err.name === "AbortError";
  }
  return err instanceof Error && err.name === "AbortError";
}

/**
 * 内部例外を `PasskeyRegistrationError` に分類する。step index に基づき、
 *
 * - step 2（registration/begin）の 400 with `code: "INVALID_USERNAME"` → `invalid_username`
 * - step 2 の 409 → `username_taken`
 * - finish の全 4xx → `server_rejected`
 * - finish の fetch reject / Abort / 5xx / 2xx parse 失敗 → `registration_uncertain`
 * - finish dispatch 前の preparation 失敗 → `server_error`
 * - create の DOMException → `cancelled`
 * - begin の fetch reject → `network_error`
 *
 * に振り分ける（tasks.md L182-193 / Req 2.5, 2.6, 2.7, 2.8, 3.4）。
 */
function classifyError(err: unknown, step: number): PasskeyRegistrationError {
  if (err instanceof PasskeyRegistrationError) {
    return err;
  }

  if (step === STEP_REG_FINISH) {
    // dispatch 前の preparation 失敗は commit 不成立が確定するため uncertain にしない。
    if (err instanceof RequestPreparationError) {
      return new PasskeyRegistrationError("server_error");
    }
    // fetch 後の 2xx parse 失敗、送達可否不明の fetch reject / abort、5xx は、
    // server 側で registration が commit 済みの可能性を排除できない。
    if (
      err instanceof ResponseParseError ||
      isAbortError(err) ||
      err instanceof TypeError ||
      (err instanceof ApiError && err.status >= 500)
    ) {
      return new PasskeyRegistrationError("registration_uncertain", {
        registered: true,
      });
    }
    // finish が返した任意の 4xx は拒否確定。status/code の内部差は UI に反射しない。
    if (err instanceof ApiError && err.status >= 400 && err.status < 500) {
      return new PasskeyRegistrationError("server_rejected");
    }
    return new PasskeyRegistrationError("server_error");
  }

  if (isCancelledError(err)) {
    return new PasskeyRegistrationError("cancelled");
  }
  if (err instanceof RequestPreparationError || err instanceof ResponseParseError) {
    return new PasskeyRegistrationError("server_error");
  }
  if (err instanceof TypeError) {
    // fetch reject（ネットワーク断・DNS 失敗等）
    return new PasskeyRegistrationError("network_error");
  }
  if (err instanceof ApiError) {
    if (step === STEP_REG_BEGIN) {
      // registration/begin の 400 は `code: "INVALID_USERNAME"` のときのみ形式不正
      // として扱い、それ以外の 400 は server_rejected として汎用エラーへ集約する
      // （Req 2.5 / 2.8）
      if (err.status === 400 && extractApiErrorCode(err) === "INVALID_USERNAME") {
        return new PasskeyRegistrationError("invalid_username");
      }
      // Req 2.6: 409 は username 重複を UI に区別表示させる（begin でのみ意味を持つ）
      if (err.status === 409) {
        return new PasskeyRegistrationError("username_taken");
      }
    }
    if (err.status === 400 || err.status === 409) {
      // 400 REGISTRATION_FAILED / 400 AUTHENTICATION_FAILED / 409 系は
      // server_rejected（内部区別を反射しない / NFR 1.2）
      return new PasskeyRegistrationError("server_rejected");
    }
    // 500 系および想定外 status は server_error
    return new PasskeyRegistrationError("server_error");
  }
  // その他の予期しない例外は安全側に server_error とし、詳細を UI に露出させない
  return new PasskeyRegistrationError("server_error");
}

/**
 * パスキー新規作成の mutation chain を 1 フックで提供する。
 *
 * #231 Delta 1 の直接 session chain:
 *   1. `generatePkcePair()` — #216 request 契約用 code_challenge を生成
 *   2. `POST /api/passkey/registration/begin { username, email:"", code_challenge }`
 *      → `{challenge_id, options}`
 *   3. `decodeCreationOptions(options)` → `navigator.credentials.create({publicKey})`
 *      → attestation
 *   4. `encodeAttestationResponse(cred)` → `POST /api/passkey/registration/finish`
 *      → `{user_id}` + Set-Cookie（server が user/credential/session を atomic commit）
 *
 * 成功時: `queryClient.invalidateQueries({queryKey: ["auth", "me"]})` で `AuthGuard` を
 * 再判定させ、2 ペイン UI へ自動遷移させる（Requirement 3.1 / 3.2）。
 *
 * email は常に `""` を送信する（Requirement 2.4「recovery email 未指定を欠落として
 * 扱わない」を実装契約で担保）。
 *
 * NFR 1.1 遵守:
 * - code_challenge / attestation 生値は `mutationFn` の closure 変数のみに保持
 * - `console.*` / `localStorage` / `sessionStorage` / URL クエリ / エラー message に
 *   一切残さない
 * - mutation 終了（成功・失敗・cancel）で closure が GC 対象になることを前提とする
 *
 * NFR 1.4 遵守: `apiClient` を経由することで同一オリジン相対パス（`API_BASE_URL = ""`）
 * に閉じる（`web/src/lib/api.ts` の既存契約）。
 */
export function usePasskeyRegistration(): UseMutationResult<
  void,
  PasskeyRegistrationError,
  { username: string }
> {
  const queryClient = useQueryClient();
  return useMutation<void, PasskeyRegistrationError, { username: string }>({
    mutationFn: async ({ username }) => {
      let step = STEP_PKCE;
      try {
        const { codeChallenge } = await generatePkcePair();

        // --- 登録 chain (steps 2-4) ---
        step = STEP_REG_BEGIN;
        const regBegin = await apiClient.post<PasskeyBeginResponse>(
          "/api/passkey/registration/begin",
          {
            username,
            email: "",
            code_challenge: codeChallenge,
          },
        );

        step = STEP_NAV_CREATE;
        const creationOptions = decodeCreationOptions(regBegin.options);
        const attestation = (await navigator.credentials.create(
          creationOptions,
        )) as PublicKeyCredential | null;
        if (!attestation) {
          // Requirement 2.7: ブラウザ側の null 解決（キャンセル相当）を「壊さずに戻す」に集約
          throw new PasskeyRegistrationError("cancelled");
        }

        step = STEP_REG_FINISH;
        await apiClient.post<RegistrationFinishResponse>(
          "/api/passkey/registration/finish",
          {
            challenge_id: regBegin.challenge_id,
            credential: encodeAttestationResponse(attestation),
          },
        );
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
