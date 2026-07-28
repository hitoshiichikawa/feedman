"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  usePasskeyRegistration,
  type PasskeyRegistrationErrorKind,
} from "@/hooks/use-passkey-registration";
import { usePasskeyAuthentication } from "@/hooks/use-passkey-authentication";

/**
 * `PasskeySignupDialog` のプロパティ。open state は親（`LoginPage`）が管理する
 * 制御コンポーネント（`DialogTrigger` は使わない）。
 */
export interface PasskeySignupDialogProps {
  /** Dialog の open 状態。親から渡される制御プロパティ。 */
  open: boolean;
  /**
   * open 状態が変化したときのコールバック。ユーザーが Esc / 外側クリック / Close 操作で
   * 閉じたときも `false` で呼ばれる。成功時も本コンポーネント内部から `false` で呼び出す。
   */
  onOpenChange: (open: boolean) => void;
  /**
   * 旧 post-finish エラーとの互換用 callback。`registration_uncertain` は専用 UI を
   * Dialog 内に維持するため、この callback を呼ばない。
   */
  onAccountCreatedNeedsLogin?: () => void;
}

/**
 * `error.kind` → ユーザー可視の固定文言マッピング。
 *
 * - `cancelled` はキー不在（= 表示しない）: Requirement 2.7「画面を壊さず戻す」を UI 上で
 *   実現するため、汎用エラーとしても表示しない
 * - `session_exchange_failed`: 旧登録 chain との互換用汎用エラー文言
 * - `server_rejected` / `server_error` / `network_error`: Requirement 2.8 の汎用エラー文言
 *   （拒否理由の内部区別を反射しない）
 *
 * NFR 1.2: `ApiError.body` の内部詳細（サーバ側 error code や stack 等）を DOM に一切
 * 反射せず、kind 別の固定文言のみを表示する。
 */
const ERROR_MESSAGES: Partial<Record<PasskeyRegistrationErrorKind, string>> = {
  invalid_username:
    "ユーザー名の形式が不正です（3〜32 文字、英数字とハイフン・アンダースコアのみ）",
  username_taken: "このユーザー名は既に使用されています",
  session_exchange_failed:
    "ログインに問題が発生しました。ログイン画面から再度お試しください",
  server_rejected: "エラーが発生しました。時間をおいて再度お試しください",
  server_error: "エラーが発生しました。時間をおいて再度お試しください",
  network_error: "エラーが発生しました。時間をおいて再度お試しください",
};

/**
 * パスキーによるアカウント新規作成 Dialog。
 *
 * 責務:
 * - username 入力欄（recovery email 入力欄は設けない / Requirement 2.4）と「作成」ボタンを提供
 * - `usePasskeyRegistration()` mutation の状態に応じた UI 表示:
 *   - `isPending`: 「作成」ボタンを disable、文言を「作成中...」に切り替え
 *   - `isError` かつ `error.kind`:
 *     - `invalid_username`: 対応する形式エラー文言を alert 表示（Requirement 2.5）
 *     - `username_taken`: 対応する重複エラー文言を alert 表示（Requirement 2.6）
 *     - `cancelled`: エラー表示なし + `mutation.reset()` で state を初期化
 *       （Requirement 2.7「画面を壊さずに戻す」）
 *     - `registration_uncertain`: Dialog を維持して discoverable login を先に提示し、
 *       finish 400 AUTHENTICATION_FAILED 後だけ再作成を提示
 *     - `session_exchange_failed`: 旧登録 chain 互換として親のログイン復旧へ委譲
 *     - `server_rejected` / `server_error` / `network_error`: 汎用エラー文言
 *       （Requirement 2.8）
 *   - `isSuccess`: `onOpenChange(false)` で Dialog を閉じる。`AuthGuard` が
 *     `queryClient.invalidateQueries(["auth","me"])` で再判定して 2 ペイン UI に
 *     自動遷移する（Requirement 3.1 / 3.2 / router 遷移は不要）
 *
 * NFR 1.1 遵守: `console.*` を呼ばず、username 以外の機密（auth_code / attestation
 * 生バイト等）は本コンポーネントで扱わない。
 * NFR 1.2 遵守: `error.message` / `ApiError.body` を DOM に反射させず、kind 別の固定
 * 文言のみを表示する。
 */
export function PasskeySignupDialog({
  open,
  onOpenChange,
  onAccountCreatedNeedsLogin,
}: PasskeySignupDialogProps) {
  const [username, setUsername] = useState("");
  const mutation = usePasskeyRegistration();
  const recoveryAuthentication = usePasskeyAuthentication();
  const {
    isPending,
    isError,
    isSuccess,
    error,
    reset: resetRegistration,
  } = mutation;
  const {
    isPending: isRecoveryPending,
    isError: isRecoveryError,
    error: recoveryError,
    reset: resetRecoveryAuthentication,
    mutate: confirmByLogin,
  } = recoveryAuthentication;
  const errorKind: PasskeyRegistrationErrorKind | undefined = isError
    ? error?.kind
    : undefined;
  const registrationUncertain = errorKind === "registration_uncertain";
  const recoveryErrorKind = isRecoveryError
    ? recoveryError?.kind
    : undefined;
  // review #6: アカウント作成（registration/finish）成功後に発生した失敗か。
  // true のときは再作成に戻さずログイン導線へ誘導する（作成前失敗と分岐する）。
  const registered = isError ? (error?.registered ?? false) : false;

  // Requirement 3.1 / 3.2: 成功時に Dialog を閉じ、AuthGuard の再判定に委ねて 2 ペイン UI へ遷移させる
  useEffect(() => {
    if (isSuccess) {
      onOpenChange(false);
    }
  }, [isSuccess, onOpenChange]);

  // review #3 / #6 / Requirement 3.4: アカウント作成後の失敗（session 合流失敗・作成後の
  // WebAuthn キャンセル等）は、Dialog を閉じて親のログイン画面へ復旧を委譲する。親へ通知して
  // Dialog を閉じた **後に** mutation state を reset する。
  //
  // review #3: reset しないと isError/registered が残留し、次に「アカウント新規作成」を
  // 押して Dialog を再度開いた瞬間、本 effect が再発火して即座に閉じてしまう（再操作破綻）。
  // reset 後は isError=false となり本 effect のガード（isError && registered）を満たさなくなる
  // ため、再発火・二重通知は起きない。reset は「作成後失敗の後始末」であり、作成前失敗の
  // 再入力フォームへ戻すもの（旧 review #6 で回避していた挙動）とは意味が異なる。
  useEffect(() => {
    if (isError && registered && !registrationUncertain) {
      onAccountCreatedNeedsLogin?.();
      onOpenChange(false);
      resetRegistration();
    }
  }, [
    isError,
    registered,
    registrationUncertain,
    onAccountCreatedNeedsLogin,
    onOpenChange,
    resetRegistration,
  ]);

  // Requirement 2.7: 作成前の cancelled は「画面を壊さず戻す」— エラー表示を出さず
  // mutation state を初期化する。作成後（registered）の cancelled は上の効果で扱う。
  useEffect(() => {
    if (errorKind === "cancelled" && !registered) {
      resetRegistration();
    }
  }, [errorKind, registered, resetRegistration]);

  // 作成前失敗のみインラインのエラー文言を表示する。作成後失敗（registered）は Dialog を
  // 閉じてログイン画面のバナーで案内するため、ここでは表示しない。
  const errorMessage =
    errorKind && !registered && !registrationUncertain
      ? ERROR_MESSAGES[errorKind]
      : undefined;

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = username.trim();
    if (!trimmed || isPending) {
      return;
    }
    mutation.mutate({ username: trimmed });
  };

  const handleRetryRegistration = () => {
    // 復旧ログインの古い authentication_failed を次の uncertain 表示へ持ち越さない。
    // registration / authentication の双方を Idle へ戻してから入力フォームへ復帰する。
    resetRegistration();
    resetRecoveryAuthentication();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {registrationUncertain
              ? "登録が完了したかどうかを確認できませんでした"
              : "パスキーでアカウント作成"}
          </DialogTitle>
          <DialogDescription>
            {registrationUncertain
              ? "通信の問題で、サーバ側の登録の成否をブラウザ側で判別できませんでした。まずはログインで確認してください。"
              : "ユーザー名を入力し、ブラウザのパスキー作成 UI からアカウントを作成します。"}
          </DialogDescription>
        </DialogHeader>

        {registrationUncertain ? (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              パスキーでログインをお試しください。ユーザー名の入力は不要です。ログインが成功すれば登録は完了しています。
            </p>
            {recoveryErrorKind && recoveryErrorKind !== "cancelled" && (
              <div
                className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
                role="alert"
              >
                <p className="font-medium text-destructive">
                  ログインを確認できませんでした。時間をおいて再度お試しください。
                </p>
              </div>
            )}
            <DialogFooter>
              <Button
                type="button"
                disabled={isRecoveryPending}
                onClick={() => confirmByLogin()}
              >
                {isRecoveryPending ? "確認中..." : "ログインで確認する"}
              </Button>
              {recoveryErrorKind === "authentication_failed" && (
                <Button
                  type="button"
                  variant="outline"
                  disabled={isRecoveryPending}
                  onClick={handleRetryRegistration}
                >
                  再度作成する
                </Button>
              )}
            </DialogFooter>
          </div>
        ) : (
          <>
            {errorMessage && (
              <div
                className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
                role="alert"
              >
                <p className="font-medium text-destructive">{errorMessage}</p>
              </div>
            )}

            <form onSubmit={handleSubmit}>
              <div className="space-y-2">
                <Label htmlFor="passkey-signup-username">ユーザー名</Label>
                <Input
                  id="passkey-signup-username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={isPending}
                  autoFocus
                  autoComplete="username"
                />
              </div>
              <DialogFooter className="mt-4">
                <Button
                  type="submit"
                  disabled={!username.trim() || isPending}
                >
                  {isPending ? "作成中..." : "作成"}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
