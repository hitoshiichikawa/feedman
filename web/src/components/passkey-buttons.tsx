"use client";

import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { usePasskeyCapability } from "@/hooks/use-passkey-capability";
import {
  usePasskeyAuthentication,
  type PasskeyAuthErrorKind,
} from "@/hooks/use-passkey-authentication";

/**
 * `PasskeyButtons` のプロパティ。新規作成 Dialog は親（`LoginPage`）が管理するため、
 * 「アカウント新規作成」クリック時のコールバックだけを親から受け取る。
 */
export interface PasskeyButtonsProps {
  /** 「アカウント新規作成」ボタン押下時に親に通知するコールバック。 */
  onSignupClick: () => void;
}

/**
 * `error.kind` → ユーザー可視の固定文言マッピング。
 *
 * - `cancelled` はキー不在（= 表示しない）: Requirement 4.5「画面を壊さずに戻す」を UI 上で
 *   実現するため、汎用エラーとしても表示しない
 * - `session_exchange_failed` / `server_rejected` / `server_error` / `network_error`:
 *   Requirement 4.6 / 4.7 の汎用エラー文言（拒否理由の内部区別を反射しない / NFR 1.2）
 *
 * NFR 1.2: `ApiError.body` の内部詳細（サーバ側 error code や stack 等）を DOM に一切
 * 反射せず、kind 別の固定文言のみを表示する（同 spec の
 * `passkey-signup-dialog.tsx` の ERROR_MESSAGES と同 idiom）。
 */
const ERROR_MESSAGES: Partial<Record<PasskeyAuthErrorKind, string>> = {
  session_exchange_failed: "認証に失敗しました。時間をおいて再度お試しください",
  server_rejected: "認証に失敗しました。時間をおいて再度お試しください",
  server_error: "認証に失敗しました。時間をおいて再度お試しください",
  network_error: "認証に失敗しました。時間をおいて再度お試しください",
};

/**
 * ログイン画面上の「パスキーでログイン」「アカウント新規作成」2 ボタンを提供する。
 *
 * 責務:
 * - `usePasskeyCapability()` の判定に応じて表示・非表示を切り替え
 *   （`isLoading` 中または `available === false` のとき `null` 返却 = 非表示
 *   / Requirement 5.1 / 5.2 の「非表示」選択肢を採用）
 * - 「パスキーでログイン」クリック → `usePasskeyAuthentication().mutate()`
 * - 「アカウント新規作成」クリック → 親の `onSignupClick` callback を呼ぶ（Dialog を親が管理）
 * - authentication mutation の状態表示:
 *   - `isPending`: 両ボタン disabled + 「認証中...」表示
 *   - `error.kind === "cancelled"`: エラー表示なし + `mutation.reset()`（Requirement 4.5）
 *   - `error.kind === その他`: 汎用エラー文言を alert 表示（Requirement 4.6 / 4.7）
 *
 * NFR 1.1 遵守: `console.*` を呼ばず、認証中の生バイト・auth_code 等は本コンポーネントで扱わない
 * （すべて `usePasskeyAuthentication` の mutation 内 closure に閉じ込められる）。
 * NFR 1.2 遵守: `error.message` / `ApiError.body` を DOM に反射させず、kind 別の固定文言のみを表示する。
 */
export function PasskeyButtons({
  onSignupClick,
}: PasskeyButtonsProps) {
  // React hooks のルール: early return より前にすべての hook を無条件で呼ぶ。
  const capability = usePasskeyCapability();
  const mutation = usePasskeyAuthentication();
  const { isPending, isError, error, reset, mutate } = mutation;
  const errorKind: PasskeyAuthErrorKind | undefined = isError
    ? error?.kind
    : undefined;

  // Requirement 4.5: cancelled は「画面を壊さず戻す」— エラー表示を出さず mutation state を初期化する
  useEffect(() => {
    if (errorKind === "cancelled") {
      reset();
    }
  }, [errorKind, reset]);

  // Requirement 5.1 / 5.2: 初回 mount の isLoading 中と capability false のとき非表示。
  // isLoading 中も null 返却でちらつきを防止（tasks.md L240-241, L264）。
  if (capability.isLoading || !capability.available) {
    return null;
  }

  const errorMessage = errorKind ? ERROR_MESSAGES[errorKind] : undefined;

  return (
    <div className="space-y-2">
      {errorMessage && (
        <div
          className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
          role="alert"
        >
          <p className="font-medium text-destructive">{errorMessage}</p>
        </div>
      )}
      <Button
        type="button"
        variant="outline"
        className="w-full"
        size="lg"
        disabled={isPending}
        onClick={() => mutate()}
      >
        {isPending ? "認証中..." : "パスキーでログイン"}
      </Button>
      <Button
        type="button"
        variant="outline"
        className="w-full"
        size="lg"
        disabled={isPending}
        onClick={onSignupClick}
      >
        アカウント新規作成
      </Button>
    </div>
  );
}
