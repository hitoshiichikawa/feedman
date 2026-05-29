"use client";

import { ApiError } from "@/lib/api";
import type { ManualFetchErrorBody } from "@/types/feed";

/** ManualRefreshBanner のプロパティ */
interface ManualRefreshBannerProps {
  /** 直前の手動フェッチエラー（null なら非表示） */
  error: ApiError | null;
}

/**
 * 手動更新の失敗を、status / code 別の説明文付きでフィルタタブ直下に表示する
 * インラインバナー。
 *
 * - 429 / FEED_COOLDOWN: 残り秒数を含むクールダウン中メッセージ（Req 7.1）
 * - 409 / FEED_FETCH_IN_PROGRESS: フェッチ進行中メッセージ（Req 7.2）
 * - 401 / UNAUTHORIZED: セッション切れメッセージ（Req 7.3）
 * - 500-503 / ネットワーク（非 ApiError） / それ以外: 一時的失敗メッセージ（Req 7.4）
 *
 * `error === null` のときは描画しない（成功時・初期状態）。
 */
export function ManualRefreshBanner({ error }: ManualRefreshBannerProps) {
  if (error === null) {
    return null;
  }

  const message = resolveMessage(error);

  return (
    <div
      data-testid="manual-refresh-banner"
      role="alert"
      className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive"
    >
      {message}
    </div>
  );
}

/**
 * `ApiError` の status / body から表示メッセージを決定する。
 * 非 `ApiError`（ネットワーク到達不能時）も想定し、any 由来を防御的に扱う。
 */
function resolveMessage(error: ApiError): string {
  const status = error.status;
  const retryAfterSeconds = extractRetryAfterSeconds(error.body);

  if (status === 429) {
    if (retryAfterSeconds !== null) {
      return `最終更新から ${retryAfterSeconds} 秒以内のため再試行できません。クールダウン解除までお待ちください。`;
    }
    return "最終更新から間もないため再試行できません。クールダウン解除までお待ちください。";
  }
  if (status === 409) {
    return "現在フェッチ中です。少し時間をおいてから再試行してください。";
  }
  if (status === 401) {
    return "セッションが切れました。ページを再読み込みしてログインしてください。";
  }
  // 500-503 / その他 / ネットワーク不調（status=0 等）: 一時的失敗扱い
  return "一時的なエラーが発生しました。しばらくしてから再試行してください。";
}

/**
 * `ApiError.body` から `details.retry_after_seconds` を取り出す。
 * 想定形状でなければ null を返す（防御的）。
 */
function extractRetryAfterSeconds(body: unknown): number | null {
  if (body === null || typeof body !== "object") {
    return null;
  }
  const errorField = (body as Partial<ManualFetchErrorBody>).error;
  if (!errorField || typeof errorField !== "object") {
    return null;
  }
  const details = errorField.details;
  if (!details || typeof details !== "object") {
    return null;
  }
  const value = details.retry_after_seconds;
  return typeof value === "number" ? value : null;
}
