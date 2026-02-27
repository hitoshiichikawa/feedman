"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Star } from "lucide-react";
import { cn } from "@/lib/utils";
import { useItems } from "@/hooks/use-items";
import type { ItemFilter, ItemSummary } from "@/types/item";

/** ItemList コンポーネントのプロパティ */
interface ItemListProps {
  /** 表示対象のフィードID（null = 未選択） */
  feedId: string | null;
  /** 記事選択イベントハンドラ */
  onSelectItem: (itemId: string) => void;
  /** 現在展開中の記事ID（null = 未展開） */
  expandedItemId: string | null;
}

/**
 * 記事一覧パネル（右ペイン）
 *
 * フィード選択に応じた記事一覧をpublished_at降順で表示する。
 * 推定フラグ付き日付の表示、無限スクロール、フィルタ切替UIを提供する。
 */
export function ItemList({ feedId, onSelectItem, expandedItemId }: ItemListProps) {
  const [filter, setFilter] = useState<ItemFilter>("all");
  const sentinelRef = useRef<HTMLDivElement>(null);

  const {
    data,
    isLoading,
    isError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useItems(feedId, filter);

  // フィード切替時にフィルタをリセット
  useEffect(() => {
    setFilter("all");
  }, [feedId]);

  // Intersection Observerによる無限スクロール
  const handleObserver = useCallback(
    (entries: IntersectionObserverEntry[]) => {
      const [entry] = entries;
      if (entry.isIntersecting && hasNextPage && !isFetchingNextPage) {
        fetchNextPage();
      }
    },
    [hasNextPage, isFetchingNextPage, fetchNextPage]
  );

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(handleObserver, {
      rootMargin: "100px",
    });
    observer.observe(sentinel);

    return () => {
      observer.disconnect();
    };
  }, [handleObserver]);

  // フィード未選択時
  if (feedId === null) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        フィードを選択してください
      </div>
    );
  }

  // ローディング中
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        読み込み中...
      </div>
    );
  }

  // エラー時
  if (isError) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-destructive">
        記事の読み込みに失敗しました
      </div>
    );
  }

  const allItems = data?.pages.flatMap((page) => page.items) ?? [];

  return (
    <div className="flex flex-col h-full">
      {/* フィルタタブ */}
      <div className="flex-shrink-0 border-b px-4 py-2">
        <Tabs
          value={filter}
          onValueChange={(value) => setFilter(value as ItemFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">全て</TabsTrigger>
            <TabsTrigger value="unread">未読</TabsTrigger>
            <TabsTrigger value="starred">スター</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* 記事一覧 */}
      <div className="flex-1 overflow-y-auto">
        {allItems.length === 0 ? (
          <div className="flex items-center justify-center h-32 text-sm text-muted-foreground">
            記事がありません
          </div>
        ) : (
          <div className="flex flex-col">
            {allItems.map((item) => (
              <ItemRow
                key={item.id}
                item={item}
                isExpanded={item.id === expandedItemId}
                onClick={() => onSelectItem(item.id)}
              />
            ))}
          </div>
        )}

        {/* 無限スクロール用sentinel */}
        <div ref={sentinelRef} data-testid="scroll-sentinel" className="h-1" />

        {/* 次ページ読み込み中 */}
        {isFetchingNextPage && (
          <div className="flex items-center justify-center py-4 text-sm text-muted-foreground">
            読み込み中...
          </div>
        )}
      </div>
    </div>
  );
}

/** 記事行のプロパティ */
interface ItemRowProps {
  item: ItemSummary;
  isExpanded: boolean;
  onClick: () => void;
}

/**
 * 記事行コンポーネント
 *
 * 記事タイトル、日付（推定フラグ付き）、既読/スター状態を表示する。
 */
function ItemRow({ item, isExpanded, onClick }: ItemRowProps) {
  const date = new Date(item.published_at);
  const formattedDate = formatDate(date);

  return (
    <button
      data-testid={`item-row-${item.id}`}
      data-read={item.is_read ? "true" : "false"}
      className={cn(
        "flex flex-col gap-1 px-4 py-3 text-left border-b transition-colors",
        "hover:bg-accent/50",
        isExpanded && "bg-accent",
        item.is_read && "opacity-60"
      )}
      onClick={onClick}
    >
      <div className="flex items-start gap-2">
        {/* タイトル */}
        <span
          className={cn(
            "flex-1 text-sm line-clamp-2",
            !item.is_read && "font-medium"
          )}
        >
          {item.title}
        </span>

        {/* スターアイコン */}
        {item.is_starred && (
          <Star
            className="flex-shrink-0 w-4 h-4 fill-yellow-400 text-yellow-400"
            data-testid={`star-${item.id}`}
          />
        )}
      </div>

      {/* 日付表示 */}
      <div className="flex items-center gap-1 text-xs text-muted-foreground">
        <time dateTime={item.published_at}>{formattedDate}</time>
        {item.is_date_estimated && (
          <span
            data-testid="date-estimated"
            className="text-xs text-orange-500"
            title="公開日が不明なため、取得日時を表示しています"
          >
            (推定)
          </span>
        )}
      </div>
    </button>
  );
}

/** 日付をフォーマットする */
function formatDate(date: Date): string {
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
