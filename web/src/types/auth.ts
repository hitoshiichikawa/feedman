/**
 * 認証関連の型定義
 *
 * バックエンドAPI (GET /auth/me) のレスポンスに対応する型。
 */

/** 現在のユーザー情報 */
export interface User {
  id: string;
  email: string;
  name: string;
  created_at: string;
}
