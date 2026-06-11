/**
 * 記事一覧・検索結果で共通利用する相対日時フォーマッタ。
 *
 * 現在時刻との差に応じて「1時間以内」「N時間前」「N日前」を返し、7 日以上前は
 * 日本語ロケールの絶対日付（例: 2026年6月10日）を返す。item-list / search-results で
 * 重複していた同一実装を一本化したもの。
 *
 * @param date フォーマット対象の日時
 * @returns 相対表現、または 7 日以上前の場合は ja-JP の絶対日付文字列
 */
export function formatRelativeDate(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffHours < 1) return "1時間以内";
  if (diffHours < 24) return `${diffHours}時間前`;
  if (diffDays < 7) return `${diffDays}日前`;

  return date.toLocaleDateString("ja-JP", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
