/**
 * フィード関連の型定義
 *
 * バックエンドAPI (GET /api/subscriptions) のレスポンスに対応する型。
 * design.md の Subscription 型定義に準拠。
 */

/** フィードのフェッチステータス */
export type FeedStatus = "active" | "stopped" | "error";

/** 購読情報（フィード一覧表示用） */
export interface Subscription {
  id: string;
  user_id: string;
  feed_id: string;
  feed_title: string;
  feed_url: string;
  favicon_url?: string | null;
  fetch_interval_minutes: number;
  feed_status: FeedStatus;
  error_message?: string | null;
  unread_count: number;
  created_at: string;
}

/**
 * 手動フェッチ API のエラーレスポンスボディ。
 *
 * `ApiError.body` にこの形状で詰められて返る前提（バックエンド
 * `internal/middleware/WriteErrorResponse` の wire format に対応）。
 * `details.retry_after_seconds` は HTTP 429 / FEED_COOLDOWN のときのみ含まれる。
 */
export interface ManualFetchErrorBody {
  error: {
    code: string;
    message: string;
    category: string;
    action: string;
    details?: {
      retry_after_seconds?: number;
    };
  };
}
