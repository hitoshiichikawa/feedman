"use client";

import { useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  usePasskeyAddRegistration,
  type PasskeyAddRegistrationErrorKind,
} from "@/hooks/use-passkey-add-registration";

/**
 * `error.kind` → ユーザー可視の固定文言マッピング。
 *
 * - `cancelled` はキー不在（= 表示しない）: Requirement 4.1「画面を壊さずに戻す」を
 *   UI 上で実現するため、汎用エラーとしても表示しない
 * - `already_registered`: 重複登録である旨をユーザーが認識できる形（Req 3.1 / 3.4）。
 *   他のキャンセル・拒否理由と区別できる独自文言を採用する
 * - `server_rejected` / `server_error` / `network_error`: 汎用エラー文言
 *   （拒否理由の内部区別を反射しない / Req 4.2 / NFR 2.2）
 *
 * NFR 2.2: `ApiError.body` の内部詳細（サーバ側 error code や stack 等）を DOM に一切
 * 反射せず、kind 別の固定文言のみを表示する。
 */
const ERROR_MESSAGES: Partial<Record<PasskeyAddRegistrationErrorKind, string>> =
  {
    already_registered:
      "この認証器はすでにアカウントに登録されています。別の端末や別のパスキー同期先を選んでください。",
    server_rejected:
      "パスキーの追加に失敗しました。時間をおいて再度お試しください。",
    server_error:
      "パスキーの追加に失敗しました。時間をおいて再度お試しください。",
    network_error:
      "通信状況を確認して、時間をおいて再度お試しください。",
  };

/**
 * アカウント設定ダイアログ内の「パスキーを追加」セクション（Req 1 / 2 / 3 / 4 / 5 / 6）。
 *
 * 責務:
 * - ロスト対策説明文の表示（Req 6.1 / 6.2）
 * - 「パスキーを追加」ボタンの提示と `usePasskeyAddRegistration` mutation の起動（Req 1.5）
 * - mutation 状態の UI 反映:
 *   - `isPending`: ボタン disabled + 「登録中...」文言（Req 4.4 の多重送信防止）
 *   - `isSuccess`: 成功フィードバック表示（Req 2.4）+ 次の追加試行のためにボタンを復元
 *   - `error.kind === "already_registered"`: 重複エラー文言（Req 3.1 / 3.4）
 *   - `error.kind === "cancelled"`: エラー表示なし + `mutation.reset()`（Req 4.1）
 *   - `error.kind === その他`: 汎用エラー文言（Req 4.2 / 4.3 / NFR 2.2）
 * - mutation 終了時（成功・失敗・キャンセル）でボタンを再操作可能な状態に戻す（Req 4.5）
 *
 * 認証済みユーザーが Account Settings UI を開いた場合にのみ描画される（Req 1.1）。
 * 未認証・取得前・取得失敗時は親コンポーネント（`AccountSettingsBody`）が描画自体を
 * 抑止するため、本コンポーネント内では表示可否判定を持たない（Req 1.2 / 1.3 / 1.4）。
 *
 * Google OAuth 由来ユーザー（パスキー未登録を含む）でも同一導線・同一操作性で
 * 表示・動作する（Req 5.1 / 5.3）。パスキー登録済み件数を判定材料としないため、
 * 「初回パスキー登録」も「2 つ目以降の追加登録」も本セクションから同じ mutation で
 * 実行できる（Req 5.1 / 5.2 / 5.3）。
 *
 * NFR 2.1 遵守: `console.*` を呼ばず、attestation 生バイト等は本コンポーネントで扱わない
 * （すべて `usePasskeyAddRegistration` の mutation 内 closure に閉じ込められる）。
 * NFR 2.2 遵守: `error.message` / `ApiError.body` を DOM に反射させず、kind 別の固定
 * 文言のみを表示する。
 */
export function PasskeyAddSection() {
  const mutation = usePasskeyAddRegistration();
  const { isPending, isError, isSuccess, error, reset, mutate } = mutation;
  const [dismissedSuccess, setDismissedSuccess] = useState(false);
  const errorKind: PasskeyAddRegistrationErrorKind | undefined = isError
    ? error?.kind
    : undefined;

  // Req 4.1: cancelled は「画面を壊さず戻す」— エラー表示を出さず mutation state を初期化する
  useEffect(() => {
    if (errorKind === "cancelled") {
      reset();
    }
  }, [errorKind, reset]);

  const errorMessage = errorKind ? ERROR_MESSAGES[errorKind] : undefined;
  const showSuccess = isSuccess && !dismissedSuccess;

  // isSuccess が false → true に切り替わる（新しい成功が届く）たびに dismiss をリセットし、
  // 直近の成功通知を可視化する。既に true のままの再レンダーではフラグを触らない。
  useEffect(() => {
    if (isSuccess) {
      setDismissedSuccess(false);
    }
  }, [isSuccess]);

  const handleAdd = () => {
    // 前回の成功通知を隠して次の試行を開始する。実運用では reset() 後に mutate() で
    // isSuccess が false→true 遷移するため、上の useEffect が新しい成功を可視化する。
    if (isSuccess) {
      setDismissedSuccess(true);
      reset();
    }
    mutate();
  };

  return (
    <section
      data-testid="passkey-add-section"
      className="space-y-3 rounded-lg border p-4"
    >
      <div className="space-y-1">
        <h3 className="text-sm font-medium">パスキーを追加</h3>
        {/* Req 6.1: ロスト対策の説明文。credential 識別子等の機密情報は含めない (Req 6.2) */}
        <p
          data-testid="passkey-add-loss-prevention"
          className="text-xs text-muted-foreground"
        >
          別の端末や同期先のパスキーを追加しておくと、片方の端末を失っても
          アカウントにアクセスできなくなるリスクを下げられます。
        </p>
      </div>

      {/* 重複エラー (Req 3.1 / 3.4)。他の汎用エラーと文言・testid を分離することで
          ユーザーが区別できるようにする。 */}
      {errorKind === "already_registered" && (
        <div
          data-testid="passkey-add-error-already-registered"
          role="alert"
          className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
        >
          <p className="font-medium text-destructive">
            {ERROR_MESSAGES.already_registered}
          </p>
        </div>
      )}

      {/* 汎用エラー (Req 4.2 / 4.3)。サーバ拒否・通信断・内部エラーを 1 系統に集約し、
          内部詳細を反射しない (NFR 2.2)。 */}
      {errorMessage && errorKind !== "already_registered" && (
        <div
          data-testid="passkey-add-error"
          role="alert"
          className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm"
        >
          <p className="font-medium text-destructive">{errorMessage}</p>
        </div>
      )}

      {/* 成功フィードバック (Req 2.4)。role="status" で a11y 通知を伴う。 */}
      {showSuccess && (
        <div
          data-testid="passkey-add-success"
          role="status"
          className="rounded-md border border-emerald-500/50 bg-emerald-500/10 p-3 text-sm"
        >
          <p className="font-medium text-emerald-700 dark:text-emerald-300">
            パスキーを追加しました。
          </p>
        </div>
      )}

      <div className="pt-1">
        <Button
          type="button"
          variant="outline"
          size="sm"
          data-testid="passkey-add-trigger"
          disabled={isPending}
          onClick={handleAdd}
        >
          <KeyRound className="w-4 h-4" />
          {isPending ? "登録中..." : "パスキーを追加"}
        </Button>
      </div>
    </section>
  );
}
