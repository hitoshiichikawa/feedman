/**
 * API共通の型定義
 *
 * バックエンドAPIの統一エラーフォーマットに対応する型。
 * design.md の APIError 型定義に準拠。
 */

/** APIエラーレスポンスの型 */
export interface ApiErrorResponse {
  code: string;
  message: string;
  category: string; // auth, validation, feed, system
  action: string; // ユーザー向け対処方法
}

/** フィード登録レスポンスの型 */
export interface FeedRegistrationResponse {
  id: string;
  feed_url: string;
  site_url?: string;
  title: string;
  favicon_url?: string | null;
  feed_status: string;
  created_at: string;
}
