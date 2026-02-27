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
