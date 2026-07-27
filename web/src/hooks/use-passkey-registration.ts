"use client";

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import { apiClient, ApiError } from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
import {
  decodeCreationOptions,
  decodeRequestOptions,
  encodeAssertionResponse,
  encodeAttestationResponse,
} from "@/lib/webauthn";
import type {
  AuthenticationFinishResponse,
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
   * アカウント作成（`POST /api/passkey/registration/finish`）が **成功した後** に発生した
   * 失敗なら `true`（review #6）。true のとき UI は「アカウントは既に作成済みなので、
   * 同じユーザー名で再作成させると `username_taken` になる」ことを踏まえ、再作成に戻さず
   * ログイン導線へ誘導する復旧フローに切り替える。false は作成前の失敗（再入力・再試行が妥当）。
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

// step 追跡用のシンボリック定数。session 交換（step 8）の失敗のみを
// `session_exchange_failed` に分類し、それ以外の step で発生する ApiError は
// `server_rejected` / `server_error` / `invalid_username` / `username_taken` に
// 振り分けるためのラベル（Req 3.4 と Req 4.6 / 2.5 / 2.6 の分類差を step index で
// 実現する）。
const STEP_PKCE = 1;
const STEP_REG_BEGIN = 2;
const STEP_NAV_CREATE = 3;
const STEP_REG_FINISH = 4;
const STEP_AUTH_BEGIN = 5;
const STEP_NAV_GET = 6;
const STEP_AUTH_FINISH = 7;
const STEP_SESSION_EXCHANGE = 8;

// WebAuthn の DOMException のうち、ユーザーキャンセル / abort / 起動失敗として
// UI 側で「画面を壊さず戻す」（Requirement 2.7）に集約する name の集合。
// task 7 の `use-passkey-authentication.ts` と同一集合。
const CANCEL_ERROR_NAMES = new Set([
  "NotAllowedError",
  "AbortError",
  "InvalidStateError",
]);

/**
 * `navigator.credentials.create()` / `.get()` が throw するキャンセル系エラーを判別する。
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
 * 内部例外を `PasskeyRegistrationError` に分類する。step index に基づき、
 *
 * - step 2（registration/begin）の 400 with `code: "INVALID_USERNAME"` → `invalid_username`
 * - step 2 の 409 → `username_taken`
 * - step 8（session）の失敗 → `session_exchange_failed`
 * - それ以外の step の 400/409 → `server_rejected`（内部区別を反射しない / NFR 1.2）
 * - 500 系 → `server_error`
 * - DOMException（NotAllowedError / AbortError / InvalidStateError）→ `cancelled`
 * - `TypeError`（fetch reject）→ `network_error`
 *
 * に振り分ける（tasks.md L182-193 / Req 2.5, 2.6, 2.7, 2.8, 3.4）。
 */
function classifyError(err: unknown, step: number): PasskeyRegistrationError {
  // registration/finish（step 4）完了後に発生した失敗は「アカウント作成済み」を意味する
  // （step が STEP_AUTH_BEGIN=5 以降 = 認証・session 合流フェーズ）。この区別で UI は
  // 作成前失敗（再入力・再試行）と作成後失敗（ログインへ誘導する復旧導線）を分ける（review #6）。
  const registered = step > STEP_REG_FINISH;
  if (err instanceof PasskeyRegistrationError) {
    // 手動 throw（create/get の null 解決 = cancelled）にも step 由来の registered を付与し直す。
    // create の null は step 3（作成前）、get の null は step 6（作成後）で意味が異なる。
    return new PasskeyRegistrationError(err.kind, {
      message: err.message,
      registered,
    });
  }
  if (isCancelledError(err)) {
    return new PasskeyRegistrationError("cancelled", { registered });
  }
  if (err instanceof TypeError) {
    // fetch reject（ネットワーク断・DNS 失敗等）
    return new PasskeyRegistrationError("network_error", { registered });
  }
  if (err instanceof ApiError) {
    if (step === STEP_SESSION_EXCHANGE) {
      // step 8 の失敗は理由に関わらず session 合流失敗に集約（Req 3.4）。常に作成後（registered）。
      return new PasskeyRegistrationError("session_exchange_failed", {
        registered,
      });
    }
    if (step === STEP_REG_BEGIN) {
      // registration/begin の 400 は `code: "INVALID_USERNAME"` のときのみ形式不正
      // として扱い、それ以外の 400 は server_rejected として汎用エラーへ集約する
      // （Req 2.5 / 2.8）
      if (err.status === 400 && extractApiErrorCode(err) === "INVALID_USERNAME") {
        return new PasskeyRegistrationError("invalid_username", { registered });
      }
      // Req 2.6: 409 は username 重複を UI に区別表示させる（begin でのみ意味を持つ）
      if (err.status === 409) {
        return new PasskeyRegistrationError("username_taken", { registered });
      }
    }
    if (err.status === 400 || err.status === 409) {
      // 400 REGISTRATION_FAILED / 400 AUTHENTICATION_FAILED / 409 系は
      // server_rejected（内部区別を反射しない / NFR 1.2）
      return new PasskeyRegistrationError("server_rejected", { registered });
    }
    // 500 系および想定外 status は server_error
    return new PasskeyRegistrationError("server_error", { registered });
  }
  // その他の予期しない例外は安全側に server_error とし、詳細を UI に露出させない
  return new PasskeyRegistrationError("server_error", { registered });
}

/**
 * パスキー新規作成の mutation chain を 1 フックで提供する。
 *
 * chain 8 段（design.md §Flows「新規作成フロー」）:
 *   1. `generatePkcePair()` — code_verifier / code_challenge を closure 変数で生成
 *   2. `POST /api/passkey/registration/begin { username, email:"", code_challenge }`
 *      → `{challenge_id, options}`
 *   3. `decodeCreationOptions(options)` → `navigator.credentials.create({publicKey})`
 *      → attestation
 *   4. `encodeAttestationResponse(cred)` → `POST /api/passkey/registration/finish`
 *      → `{user_id}`
 *   5. `POST /api/passkey/authentication/begin { code_challenge }`（step 2 の
 *      code_challenge を **同一値** で再送する。サーバ側 finish で PKCE 検証されるため）
 *   6. `decodeRequestOptions(options)` → `navigator.credentials.get({publicKey})`
 *      → assertion
 *   7. `encodeAssertionResponse(cred)` → `POST /api/passkey/authentication/finish`
 *      → `{auth_code}`
 *   8. `POST /api/auth/session { auth_code, code_verifier }` → 204 + Set-Cookie
 *
 * 成功時: `queryClient.invalidateQueries({queryKey: ["auth", "me"]})` で `AuthGuard` を
 * 再判定させ、2 ペイン UI へ自動遷移させる（Requirement 3.1 / 3.2）。
 *
 * email は常に `""` を送信する（Requirement 2.4「recovery email 未指定を欠落として
 * 扱わない」を実装契約で担保）。
 *
 * NFR 1.1 遵守:
 * - `codeVerifier` / `authCode` / attestation 生値 / assertion 生値は `mutationFn` の
 *   closure 変数のみに保持
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
        const { codeVerifier, codeChallenge } = await generatePkcePair();

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

        // --- 認証 chain (steps 5-7) ---
        // Requirement 2.3: 追加操作なしで合流させるため、登録直後に discoverable login
        // を実行する。code_challenge は step 2 と同じ値を再送する（サーバは
        // authentication/finish で verifier を検証するため、challenge の再利用は許容）。
        step = STEP_AUTH_BEGIN;
        const authBegin = await apiClient.post<PasskeyBeginResponse>(
          "/api/passkey/authentication/begin",
          { code_challenge: codeChallenge },
        );

        step = STEP_NAV_GET;
        const requestOptions = decodeRequestOptions(authBegin.options);
        // review #5: 登録直後の discoverable login が、同一端末上の **別の** Feedman
        // パスキーを選んでしまい別アカウントとしてログインする事故を防ぐ。直前に
        // `navigator.credentials.create()` で登録した credential（`attestation.rawId`）へ
        // `allowCredentials` を限定し、後続認証を登録した credential / user に拘束する。
        // サーバ側の challenge / 検証はそのまま（クライアントで選択候補を絞るだけ）。
        if (requestOptions.publicKey) {
          requestOptions.publicKey.allowCredentials = [
            { type: "public-key", id: attestation.rawId },
          ];
        }
        const assertion = (await navigator.credentials.get(
          requestOptions,
        )) as PublicKeyCredential | null;
        if (!assertion) {
          // ここでの null もキャンセル相当として集約（Req 2.7 の網羅性向上）
          throw new PasskeyRegistrationError("cancelled");
        }

        step = STEP_AUTH_FINISH;
        const authFinish = await apiClient.post<AuthenticationFinishResponse>(
          "/api/passkey/authentication/finish",
          {
            challenge_id: authBegin.challenge_id,
            credential: encodeAssertionResponse(assertion),
          },
        );

        // --- session 合流 (step 8) ---
        step = STEP_SESSION_EXCHANGE;
        await apiClient.post<void>("/api/auth/session", {
          auth_code: authFinish.auth_code,
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
