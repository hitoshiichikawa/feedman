/**
 * 記事関連の型定義
 *
 * バックエンドAPI (GET /api/feeds/:feedId/items) のレスポンスに対応する型。
 * design.md の ItemSummary, ItemDetail 型定義に準拠。
 */

/** フィルタの種類 */
export type ItemFilter = "all" | "unread" | "starred";

/** 記事サマリー（一覧表示用） */
export interface ItemSummary {
  id: string;
  feed_id: string;
  title: string;
  link: string;
  published_at: string;
  is_date_estimated: boolean;
  is_read: boolean;
  is_starred: boolean;
  hatebu_count: number;
  /** はてなブックマーク取得日時（未取得時はnull） */
  hatebu_fetched_at: string | null;
}

/** 記事詳細（展開表示用） */
export interface ItemDetail extends ItemSummary {
  /** サニタイズ済みHTMLコンテンツ */
  content: string;
  summary: string;
  author: string;
}

/** 記事一覧APIレスポンス */
export interface ItemListResponse {
  items: ItemSummary[];
  next_cursor: string | null;
  has_more: boolean;
}

/** 記事状態更新リクエスト */
export interface ItemStateRequest {
  is_read?: boolean | null;
  is_starred?: boolean | null;
}
