"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Star } from "lucide-react";
import { cn } from "@/lib/utils";
import { useItems, useItemDetail } from "@/hooks/use-items";
import { useMarkAsRead, useToggleStar } from "@/hooks/use-item-state";
import { ItemDetail } from "@/components/item-detail";
import type {
  ItemDetail as ItemDetailType,
  ItemFilter,
  ItemSummary,
} from "@/types/item";

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

  // 展開中の記事詳細（本文を含む）を取得する。expandedItemId が null の間は無効化される。
  const {
    data: detail,
    isLoading: isDetailLoading,
    isError: isDetailError,
  } = useItemDetail(expandedItemId);

  // 記事詳細から既読化・スター切替を行うための mutation を配線する。
  const markAsRead = useMarkAsRead();
  const toggleStar = useToggleStar();

  const handleMarkAsRead = useCallback(
    (itemId: string) => {
      markAsRead.mutate(itemId);
    },
    [markAsRead]
  );

  const handleToggleStar = useCallback(
    (itemId: string, isStarred: boolean) => {
      toggleStar.mutate({ itemId, isStarred });
    },
    [toggleStar]
  );

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
            {allItems.map((item) => {
              const isExpanded = item.id === expandedItemId;
              return (
                <div key={item.id} className="flex flex-col">
                  <ItemRow
                    item={item}
                    isExpanded={isExpanded}
                    onClick={() => onSelectItem(item.id)}
                  />
                  {/* 選択記事行の直下に記事詳細を展開する（button の外側に兄弟要素として描画） */}
                  {isExpanded && (
                    <ItemDetailArea
                      isLoading={isDetailLoading}
                      isError={isDetailError}
                      detail={detail ?? null}
                      detailItemId={expandedItemId}
                      onMarkAsRead={handleMarkAsRead}
                      onToggleStar={handleToggleStar}
                    />
                  )}
                </div>
              );
            })}
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

/** ItemDetailArea コンポーネントのプロパティ */
interface ItemDetailAreaProps {
  /** 記事詳細の取得中フラグ */
  isLoading: boolean;
  /** 記事詳細の取得失敗フラグ */
  isError: boolean;
  /** 取得済みの記事詳細データ（未取得時は null） */
  detail: ItemDetailType | null;
  /** 現在展開中の記事ID（取得結果と一致するか判定するため） */
  detailItemId: string | null;
  /** 既読化コールバック */
  onMarkAsRead: (itemId: string) => void;
  /** スター切替コールバック */
  onToggleStar: (itemId: string, isStarred: boolean) => void;
}

/**
 * 記事詳細展開エリア
 *
 * 記事行クリック直後に枠を表示し、取得状態に応じてローディング表示／エラー表示／
 * ItemDetail（本文）を出し分ける。詳細データの取得完了を待たずに同期的に枠を描画する
 * ことで、クリックから 200ms 以内に展開表示を開始する（NFR 2.1）。
 */
function ItemDetailArea({
  isLoading,
  isError,
  detail,
  detailItemId,
  onMarkAsRead,
  onToggleStar,
}: ItemDetailAreaProps) {
  // 取得失敗時はエラー表示を提示する（AC 2.3）
  if (isError) {
    return (
      <div
        data-testid="item-detail-error"
        className="border-t bg-background px-4 py-4 text-sm text-destructive"
      >
        記事の詳細を読み込めませんでした
      </div>
    );
  }

  // 取得完了かつ展開中の記事IDと一致する詳細データがあれば本文を表示する（AC 1.2, 2.4）。
  // 取得中、または別記事の古い詳細が残っている場合はローディング表示を出す（AC 2.2）。
  if (isLoading || detail === null || detail.id !== detailItemId) {
    return (
      <div
        data-testid="item-detail-loading"
        className="border-t bg-background px-4 py-4 text-sm text-muted-foreground"
      >
        読み込み中...
      </div>
    );
  }

  return (
    <ItemDetail
      item={detail}
      onMarkAsRead={onMarkAsRead}
      onToggleStar={onToggleStar}
    />
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
 * 記事タイトル、概要、日付（推定フラグ付き）、既読/スター状態を表示する。
 * 公開日時はタイトルと同一行の右側に、概要はタイトル直下に表示する。
 */
function ItemRow({ item, isExpanded, onClick }: ItemRowProps) {
  const date = new Date(item.published_at);
  const formattedDate = formatDate(date);
  const hasSummary = item.summary.trim().length > 0;

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
      {/* タイトル行: タイトル(左) + 公開日時/スター(右) を同一行に配置 */}
      <div
        data-testid={`item-title-row-${item.id}`}
        className="flex items-start gap-2"
      >
        {/* タイトルリンク */}
        <a
          href={item.link}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(e) => e.stopPropagation()}
          className={cn(
            "flex-1 min-w-0 text-sm line-clamp-2 hover:underline cursor-pointer",
            !item.is_read && "font-medium"
          )}
        >
          {item.title}
        </a>

        {/* 公開日時（タイトル右側・縮小しすぎないよう whitespace-nowrap で判読性を維持） */}
        <span className="flex flex-shrink-0 items-center gap-1 text-xs text-muted-foreground whitespace-nowrap">
          <time dateTime={item.published_at}>{formattedDate}</time>
          {item.is_date_estimated && (
            <span
              data-testid="date-estimated"
              className="text-orange-500"
              title="公開日が不明なため、取得日時を表示しています"
            >
              (推定)
            </span>
          )}
        </span>

        {/* スターアイコン */}
        {item.is_starred && (
          <Star
            className="flex-shrink-0 w-4 h-4 fill-yellow-400 text-yellow-400"
            data-testid={`star-${item.id}`}
          />
        )}
      </div>

      {/* 概要（空のときは描画しない。最大2行で省略） */}
      {hasSummary && (
        <p
          data-testid={`item-summary-${item.id}`}
          className="text-xs text-muted-foreground line-clamp-2"
        >
          {item.summary}
        </p>
      )}
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
