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
  /**
   * ユーザー指定 ID（パスキー登録時に設定）。
   * Google 由来ユーザーは常に null。API は常にこのキーを返す
   * （Req 2.1 / Req 2.3 / Issue #241）。
   */
  username: string | null;
  created_at: string;
}
