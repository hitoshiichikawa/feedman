"use client";

import { cn } from "@/lib/utils";
import { useAppDispatch, useAppState } from "@/contexts/app-state";
import { Star } from "lucide-react";

/**
 * 左ペイン「お気に入り」固定ナビ項目。
 *
 * フィード一覧の上に常時 1 件表示され、クリックで右ペインを横断スター記事一覧
 * （`StarredItemList`）に切り替える。`selectedView === "starred"` のときに
 * `feed-list.tsx` と同じアクティブクラスを適用する（要件 1.2 / 1.3）。
 *
 * - 表示テキスト: 「お気に入り」
 * - アイコン: lucide-react の `Star`（既存スターアイコンと同じ視覚言語）
 * - クリック: `useAppDispatch()({ type: "SELECT_STARRED" })`
 * - アクティブクラス: `bg-accent text-accent-foreground font-medium`
 *   （`feed-list.tsx` のフィード行と完全一致）
 */
export function StarredNavItem() {
  const state = useAppState();
  const dispatch = useAppDispatch();

  const isActive = state.selectedView === "starred";

  return (
    <button
      type="button"
      data-testid="starred-nav-item"
      data-selected={isActive ? "true" : "false"}
      className={cn(
        "flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors",
        "hover:bg-accent hover:text-accent-foreground",
        isActive && "bg-accent text-accent-foreground font-medium"
      )}
      onClick={() => dispatch({ type: "SELECT_STARRED" })}
    >
      <span className="flex-shrink-0 w-4 h-4 inline-flex items-center justify-center text-muted-foreground">
        <Star className="w-4 h-4" aria-hidden="true" />
      </span>
      <span className="flex-1 truncate">お気に入り</span>
    </button>
  );
}
