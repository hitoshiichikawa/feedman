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

/**
 * `PasskeySignupDialog` のプロパティ。open state は親（`LoginPage`）が管理する
 * 制御コンポーネント（`DialogTrigger` は使わない）。
 */
export interface PasskeySignupDialogProps {
  /** Dialog の open 状態。親から渡される制御プロパティ。 */
  open: boolean;
  /**
   * open 状態が変化したときのコールバック。ユーザーが Esc / 外側クリック / Close 操作で
   * 閉じたときも `false` で呼ばれる。成功時 / session 合流失敗時は本コンポーネント内部
   * からも `false` で呼び出す。
   */
  onOpenChange: (open: boolean) => void;
  /**
   * アカウント作成（registration/finish）が成功した **後** に失敗が発生したときに呼ばれる
   * （review #6）。session 合流失敗・作成後の WebAuthn キャンセル等が該当する。親（`LoginPage`）は
   * これを受けて「アカウントは作成済みなのでログインへ」という復旧バナーを表示する。
   * このケースでは Dialog を閉じ、同じユーザー名での再作成（`username_taken` を誘発）に
   * 戻さないことで復旧導線を成立させる。
   */
  onAccountCreatedNeedsLogin?: () => void;
}

/**
 * `error.kind` → ユーザー可視の固定文言マッピング。
 *
 * - `cancelled` はキー不在（= 表示しない）: Requirement 2.7「画面を壊さず戻す」を UI 上で
 *   実現するため、汎用エラーとしても表示しない
 * - `session_exchange_failed`: Requirement 3.4 の汎用エラー文言
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
 *     - `session_exchange_failed`: 汎用エラー文言を alert 表示し `onOpenChange(false)`
 *       で Dialog を閉じてログイン画面に復帰させる（Requirement 3.4）
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
  const { isPending, isError, isSuccess, error, reset } = mutation;
  const errorKind: PasskeyRegistrationErrorKind | undefined = isError
    ? error?.kind
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

  // review #6 / Requirement 3.4: アカウント作成後の失敗（session 合流失敗・作成後の
  // WebAuthn キャンセル等）は、Dialog を閉じて親のログイン画面へ復旧を委譲する。同じ
  // ユーザー名での再作成に戻すと `username_taken` になるため、reset せず親へ通知する。
  useEffect(() => {
    if (isError && registered) {
      onAccountCreatedNeedsLogin?.();
      onOpenChange(false);
    }
  }, [isError, registered, onAccountCreatedNeedsLogin, onOpenChange]);

  // Requirement 2.7: 作成前の cancelled は「画面を壊さず戻す」— エラー表示を出さず
  // mutation state を初期化する。作成後（registered）の cancelled は上の効果で扱う。
  useEffect(() => {
    if (errorKind === "cancelled" && !registered) {
      reset();
    }
  }, [errorKind, registered, reset]);

  // 作成前失敗のみインラインのエラー文言を表示する。作成後失敗（registered）は Dialog を
  // 閉じてログイン画面のバナーで案内するため、ここでは表示しない。
  const errorMessage =
    errorKind && !registered ? ERROR_MESSAGES[errorKind] : undefined;

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = username.trim();
    if (!trimmed || isPending) {
      return;
    }
    mutation.mutate({ username: trimmed });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>パスキーでアカウント作成</DialogTitle>
          <DialogDescription>
            ユーザー名を入力し、ブラウザのパスキー作成 UI からアカウントを作成します。
          </DialogDescription>
        </DialogHeader>

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
      </DialogContent>
    </Dialog>
  );
}
