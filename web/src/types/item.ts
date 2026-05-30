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

/**
 * 検索スコープ。
 *
 * - `'global'`: 横断検索（購読中の全フィードを対象）
 * - `'feed'`: フィード内検索（現在開いているフィードを対象）
 *
 * AppState 内の `searchScope` フィールドおよび `useItemSearch` の引数として利用する。
 * 型の真の定義源は `@/contexts/app-state` 側にあり、types/item.ts では再エクスポートする
 * （UI 状態側を canonical とし、types/item.ts は API レスポンス型と一緒に閲覧できるよう
 * 利便のため re-export する）。
 */
export type { SearchScope } from "@/contexts/app-state";

/**
 * 検索結果 1 件（横断 / フィード内検索共通）。
 *
 * ItemSummary 相当のフィールドに加え、横断検索で「どのフィード由来か」を識別するための
 * `feed_title` と `favicon_url` を併記する。`favicon_url` はサーバー側で生成された
 * `data:<mime>;base64,...` 形式の data URL、または favicon 未取得時は null。
 *
 * 既存 ItemSummary との差分:
 * - 追加: `feed_title`, `favicon_url`
 */
export interface ItemSearchHit {
  id: string;
  feed_id: string;
  title: string;
  link: string;
  /** サニタイズ済みの概要テキスト（空の場合は空文字列） */
  summary: string;
  /** RFC3339 形式の公開日時。NULL の場合のみ null */
  published_at: string | null;
  is_date_estimated: boolean;
  hatebu_count: number;
  /**
   * はてなブックマーク取得日時。未取得時は null（RFC3339 文字列または null）。
   * 検索 API は Go 側で `omitempty` のため未取得時はレスポンスから省略されることがあり、
   * 呼び出し側は `undefined ?? null` で正規化して受け取る（design.md Notes for Developers 参照）。
   */
  hatebu_fetched_at: string | null;
  /** 記事が属するフィードのタイトル（横断検索結果のバッジ表示で使用） */
  feed_title: string;
  /**
   * 記事が属するフィードの favicon URL（`data:<mime>;base64,...` 形式）。
   * favicon 未取得時は null（API レスポンスでは `omitempty` のため未送信）。
   */
  favicon_url: string | null;
  is_read: boolean;
  is_starred: boolean;
}

/**
 * 検索 API（GET /api/items/search）のレスポンス。
 *
 * カーソルベースページネーション形式で、`next_cursor` は `<RFC3339Nano>|<uuid>` 形式の
 * 文字列または null。`has_more` が false のとき、または `next_cursor` が null / 空文字の
 * ときは次ページなしとして扱う（impl-notes Task 4 / 5 の判断と整合）。
 */
export interface ItemSearchResponse {
  items: ItemSearchHit[];
  next_cursor: string | null;
  has_more: boolean;
}
