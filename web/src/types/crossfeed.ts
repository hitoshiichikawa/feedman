/**
 * フィード横断新着一覧（Cross-Feed Timeline）の API レスポンス型定義（Issue #121）。
 *
 * バックエンド側 DTO は `internal/handler/crossfeed_handler.go` の
 * `crossFeedItemResponse` / `crossFeedListResponse` に対応する。
 *
 * - `feed_favicon_url` は **常に存在し**、未設定時は明示的に `null` を返す
 *   （`omitempty` を外している。Task 5 の判断 (1) と整合）。
 *   フロント側は `string | null` として受け取り、null のとき `Rss` 代替アイコンに
 *   フォールバックする（Req 3.3）。
 * - `since_time` はサーバが採用した新着判定基準時刻（RFC3339 文字列）。クライアントは
 *   セッション初回 fetch 完了時にこの値で `crossFeedSessionSince` を固定する（Req 4.7）。
 */
export interface CrossFeedItem {
  id: string;
  feed_id: string;
  feed_title: string;
  /**
   * 記事が属するフィードの favicon URL（`data:<mime>;base64,...` 形式）。
   * favicon 未設定時は null（API レスポンス上は常に存在する nullable フィールド）。
   */
  feed_favicon_url: string | null;
  title: string;
  link: string;
  /** サニタイズ済みの概要テキスト（空の場合は空文字列） */
  summary: string;
  /** RFC3339 形式の公開日時 */
  published_at: string;
  is_date_estimated: boolean;
  is_read: boolean;
  is_starred: boolean;
  hatebu_count: number;
}

/**
 * GET /api/items/cross-feed のレスポンス。
 *
 * カーソルベースページネーション（50 件/回、上限 200）。
 * `next_cursor` は `<RFC3339Nano>:<itemID>` 形式の文字列または null。
 * `has_more` が false のとき次ページなしとして扱う。
 * `since_time` は当該レスポンスでサーバが採用した新着判定基準時刻（RFC3339）。
 */
export interface CrossFeedListResponse {
  items: CrossFeedItem[];
  next_cursor: string | null;
  has_more: boolean;
  since_time: string;
}
