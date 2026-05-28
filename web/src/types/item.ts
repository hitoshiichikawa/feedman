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
  /** サニタイズ済みの概要テキスト（空の場合は空文字列） */
  summary: string;
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

/**
 * スター記事サマリー（フィード横断スター一覧用）
 *
 * GET /api/feeds/starred/items の応答に含まれる記事行。横断一覧では記事が属する
 * フィードを識別する必要があるため、`ItemSummary` に `feed_title` を追加した拡張型。
 * 既存 `ItemSummary` / `ItemListResponse` の応答スキーマは変更しない（NFR 3.1）。
 */
export interface StarredItemSummary extends ItemSummary {
  /** 記事が属するフィードのタイトル（横断一覧でフィード識別表示に使用） */
  feed_title: string;
}

/** スター記事一覧APIレスポンス（GET /api/feeds/starred/items） */
export interface StarredItemListResponse {
  items: StarredItemSummary[];
  next_cursor: string | null;
  has_more: boolean;
}
