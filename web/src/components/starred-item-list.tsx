"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAppDispatch, useAppState } from "@/contexts/app-state";
import { useStarredItems } from "@/hooks/use-starred-items";
import { useItemDetail } from "@/hooks/use-items";
import { useMarkAsRead, useToggleStar } from "@/hooks/use-item-state";
import { ItemRow, ItemDetailArea } from "@/components/item-list";

/**
 * 横断スター記事一覧パネル（右ペイン）
 *
 * 現ユーザーがスターを付与した全フィードの記事を `published_at` 降順で表示する。
 * 各行は既存 `ItemRow` を再利用し、その直下に `feed_title` を薄い文字色で 1 行併記
 * する（Req 2.3 / 2.4）。Intersection Observer による無限スクロール、空状態 /
 * エラー状態、`AppState.expandedItemId` 経由の排他展開を提供する（Req 2.5〜2.8）。
 *
 * ヘッダにコンテキストタイトル「お気に入り」を表示する（Req 2.1）。フィルタタブは
 * 表示しない（Non-Goals: サブフィルタ UI 切替を提供しない）。
 */
export function StarredItemList() {
  const state = useAppState();
  const dispatch = useAppDispatch();
  const sentinelRef = useRef<HTMLDivElement>(null);

  const {
    data,
    isLoading,
    isError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useStarredItems();

  // 展開中の記事詳細（本文を含む）を取得する。expandedItemId が null の間は無効化される。
  const {
    data: detail,
    isLoading: isDetailLoading,
    isError: isDetailError,
  } = useItemDetail(state.expandedItemId);

  // 既読化・スター切替 mutation。
  const markAsRead = useMarkAsRead();
  const toggleStar = useToggleStar();

  const handleSelectItem = useCallback(
    (itemId: string) => {
      dispatch({ type: "EXPAND_ITEM", itemId });
    },
    [dispatch]
  );

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

  // Intersection Observer による無限スクロール（既存 ItemList と同パターン / Req 2.5）。
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

  const allItems = data?.pages.flatMap((page) => page.items) ?? [];

  return (
    <div data-testid="starred-item-list" className="flex flex-col h-full">
      {/* ヘッダ: コンテキストタイトル「お気に入り」（Req 2.1） */}
      <div className="flex-shrink-0 border-b px-4 py-2">
        <h2
          data-testid="starred-item-list-title"
          className="text-sm font-medium"
        >
          お気に入り
        </h2>
      </div>

      {/* 記事一覧 */}
      <div className="flex-1 overflow-y-auto">
        {isError ? (
          <div
            data-testid="starred-item-list-error"
            className="flex items-center justify-center h-32 text-sm text-destructive"
          >
            記事の読み込みに失敗しました
          </div>
        ) : isLoading ? (
          <div className="flex items-center justify-center h-32 text-sm text-muted-foreground">
            読み込み中...
          </div>
        ) : allItems.length === 0 ? (
          <div
            data-testid="starred-item-list-empty"
            className="flex items-center justify-center h-32 text-sm text-muted-foreground"
          >
            記事がありません
          </div>
        ) : (
          <div className="flex flex-col">
            {allItems.map((item) => {
              const isExpanded = item.id === state.expandedItemId;
              return (
                <div key={item.id} className="flex flex-col">
                  <ItemRow
                    item={item}
                    isExpanded={isExpanded}
                    onClick={() => handleSelectItem(item.id)}
                  />
                  {/* feed_title を行直下に薄い文字色で 1 行表示（Req 2.4） */}
                  <div
                    data-testid={`starred-item-feed-title-${item.id}`}
                    className="px-4 pb-2 -mt-1 text-xs text-muted-foreground truncate"
                  >
                    {item.feed_title}
                  </div>
                  {/* 選択記事行の直下に記事詳細を展開する（既存 ItemList と同パターン / Req 2.8） */}
                  {isExpanded && (
                    <ItemDetailArea
                      isLoading={isDetailLoading}
                      isError={isDetailError}
                      detail={detail ?? null}
                      detailItemId={state.expandedItemId}
                      onMarkAsRead={handleMarkAsRead}
                      onToggleStar={handleToggleStar}
                    />
                  )}
                </div>
              );
            })}
          </div>
        )}

        {/* 無限スクロール用 sentinel（Req 2.5） */}
        <div
          ref={sentinelRef}
          data-testid="starred-scroll-sentinel"
          className="h-1"
        />

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
