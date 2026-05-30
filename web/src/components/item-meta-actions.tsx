"use client";

import type { MouseEvent } from "react";
import { Star, Bookmark } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** ItemMetaActions コンポーネントのプロパティ */
interface ItemMetaActionsProps {
  /** 記事 ID（onToggleStar のキー / data-testid 接尾辞） */
  itemId: string;
  /** 現在のスター状態（true = スター付き / false = スター無し） */
  isStarred: boolean;
  /** はてブ数（hatebuFetchedAt が null のときは表示されない） */
  hatebuCount: number;
  /** はてブ取得日時。null のとき数値ではなく `-` を表示する */
  hatebuFetchedAt: string | null;
  /** スター⭐️切替コールバック（楽観更新は呼び出し側 mutation で行う） */
  onToggleStar: (itemId: string, nextStarred: boolean) => void;
}

/**
 * 記事一覧行右端の「はてブ数表示 + スター⭐️トグル」共通コンポーネント
 *
 * 3 つの記事一覧（通常記事一覧 / スター横断一覧 / 検索結果一覧）から
 * 再利用される。本コンポーネントは描画と callback 発火のみを行い、
 * mutation や API 呼び出しは呼び出し側の `useToggleStar` 責務とする。
 *
 * - はてブ数: `hatebuFetchedAt === null` のとき `-` / それ以外は `hatebuCount` の整数表示
 * - スター⭐️: 状態に応じて塗りつぶし（黄色）/ アウトラインを切替
 * - クリック時に `e.stopPropagation()` を呼び、行クリック展開への伝播を抑止
 * - aria-label / aria-pressed で現状態を支援技術へ提示
 * - `Button size="icon-sm"` で最小 32px × 32px のヒット領域を確保
 */
export function ItemMetaActions({
  itemId,
  isStarred,
  hatebuCount,
  hatebuFetchedAt,
  onToggleStar,
}: ItemMetaActionsProps) {
  // はてブ数の表示テキスト。未取得（hatebuFetchedAt === null）と 0 件は区別して表示する
  // （`-` と `0` の差別化、Req 1.3 / 1.4 / 5.3 / 5.4）。
  const hatebuDisplay = hatebuFetchedAt === null ? "-" : String(hatebuCount);
  // スター⭐️トグルのアクセシブル名（NFR 1.1）。現状態と用途を支援技術へ伝える。
  const starLabel = isStarred ? "スターを解除する" : "スターを付ける";
  // はてブ数のツールチップ。未取得時は数値ではなく取得状況を伝える。
  const hatebuTitle = hatebuFetchedAt
    ? `はてなブックマーク数: ${hatebuCount}`
    : "はてなブックマーク数未取得";

  // クリックハンドラ。行クリック展開への伝播を抑止（Req 2.3 / NFR 2.1）した後、
  // スター状態を反転させる callback を発火する（Req 2.1）。
  const handleClick = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    onToggleStar(itemId, !isStarred);
  };

  return (
    <div
      data-testid={`item-meta-actions-${itemId}`}
      className="flex flex-shrink-0 items-center gap-1"
    >
      {/* はてブ数（アイコン + 数値 / `-`、ツールチップで意味を補足）。 */}
      <span
        data-testid={`item-hatebu-count-${itemId}`}
        className="inline-flex items-center gap-1 text-sm text-muted-foreground px-1"
        title={hatebuTitle}
      >
        <Bookmark className="w-4 h-4" aria-hidden="true" />
        {hatebuDisplay}
      </span>

      {/* スター⭐️トグル（NFR 1.1〜1.4）。
         size="icon-sm" で 32px の正方形ヒット領域を確保し、
         ghost variant のホバー時 bg-accent と rounded-full で丸い背景強調を提供する。 */}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        data-testid={`item-star-toggle-${itemId}`}
        aria-label={starLabel}
        aria-pressed={isStarred}
        title={starLabel}
        className="rounded-full"
        onClick={handleClick}
      >
        <Star
          aria-hidden="true"
          className={cn(
            "w-4 h-4",
            isStarred
              ? "fill-yellow-400 text-yellow-400"
              : "text-muted-foreground"
          )}
        />
      </Button>
    </div>
  );
}
