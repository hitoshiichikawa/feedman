"use client";

import { useCallback, useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppDispatch, useAppState } from "@/contexts/app-state";
import {
  useCrossFeedItems,
  useTouchCrossFeedLastSeen,
} from "@/hooks/use-cross-feed-items";
import { useItemDetail } from "@/hooks/use-items";
import { useMarkAsRead, useToggleStar } from "@/hooks/use-item-state";
import { ItemRow, ItemDetailArea } from "@/components/item-list";
import { FeedFavicon } from "@/components/feed-favicon";
import type { CrossFeedItem } from "@/types/crossfeed";
import type { ItemSummary } from "@/types/item";

/**
 * 横断新着一覧パネル（右ペイン、Issue #121）。
 *
 * `useCrossFeedItems` で全購読フィードを横断した新着記事を取得し、
 * 既存 `ItemRow` を再利用しつつ各行に発信元フィードの favicon + フィード名 badge を
 * 併記する（Req 3.1 / 3.2 / 3.4）。
 *
 * セッション初回 fetch 完了時に 1 回だけ:
 *   1. `SET_CROSS_FEED_SESSION_SINCE` を `data.pages[0].since_time` で dispatch
 *      し、クライアント側 baseline を固定する（Req 4.7）
 *   2. `useTouchCrossFeedLastSeen().mutate()` を呼び、サーバ側 `last_seen_at` を
 *      now() に更新する（Req 4.3、次セッション / 別デバイス向け同期）
 *
 * 既存 `selectedView === 'starred'` モードと排他になるよう AppShell 側で
 * `viewMode === 'cross-feed'` 時のみ描画される前提（task 9 で配線）。
 *
 * フィルタタブは横断一覧では表示しない（Non-Goals: フィルタ機能の追加は対象外）。
 */
export function CrossFeedItemList() {
  const state = useAppState();
  const dispatch = useAppDispatch();
  const queryClient = useQueryClient();
  const sentinelRef = useRef<HTMLDivElement>(null);

  const {
    data,
    isLoading,
    isError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useCrossFeedItems();

  // 展開中の記事詳細を取得する（既存 ItemList / StarredItemList と同パターン）
  const {
    data: detail,
    isLoading: isDetailLoading,
    isError: isDetailError,
  } = useItemDetail(state.expandedItemId);

  const markAsRead = useMarkAsRead();
  const toggleStar = useToggleStar();
  const touchLastSeen = useTouchCrossFeedLastSeen();
  // mutate 関数のみを依存配列に渡すために stable な ref に保持する（mutation result
  // オブジェクト全体を依存に含めると mutation 進行中の状態変化（isPending 等）で
  // useEffect が再実行されてしまう）。
  const touchMutateRef = useRef(touchLastSeen.mutate);
  touchMutateRef.current = touchLastSeen.mutate;

  // セッション初回 fetch 完了時の baseline 固定 + touch mutation 発火（Req 4.3 / 4.7）。
  // `crossFeedSessionSince === null` のときのみ走る。既に baseline が固定済みの
  // 状態（再マウントを含む）では発火しない（session 内重複防止）。
  //
  // 実装最適化（design.md「Component Interface」/「useCrossFeedItems」節）:
  // dispatch によって useCrossFeedItems の queryKey が
  // `['cross-feed-items', 'initial']` から `['cross-feed-items', sinceTime]` に
  // 切り替わる。新 queryKey にはキャッシュが無いため一瞬 loading に戻る挙動が
  // 発生する。これを避けるため、dispatch 前に `queryClient.setQueryData` で
  // 新 queryKey に現在の data を移送し、即時に新キャッシュから描画継続できる
  // ようにする（必須ではない最適化だが、UX として推奨され、テスト上も
  // 連続的な data 観測が可能になる）。
  const hasInitializedSessionRef = useRef(false);
  useEffect(() => {
    if (hasInitializedSessionRef.current) return;
    if (state.crossFeedSessionSince !== null) return;
    const firstPage = data?.pages[0];
    if (!firstPage) return;

    hasInitializedSessionRef.current = true;
    // 現キャッシュを新 queryKey に移送（重複 fetch / 一瞬の loading を回避）。
    queryClient.setQueryData(
      ["cross-feed-items", firstPage.since_time],
      data
    );
    dispatch({
      type: "SET_CROSS_FEED_SESSION_SINCE",
      sinceTime: firstPage.since_time,
    });
    touchMutateRef.current();
  }, [data, state.crossFeedSessionSince, dispatch, queryClient]);

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

  // 無限スクロール（IntersectionObserver、既存 ItemList と同 pattern / NFR 1.3）
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

  const allItems: CrossFeedItem[] =
    data?.pages.flatMap((page) => page.items) ?? [];

  return (
    <div data-testid="cross-feed-item-list" className="flex flex-col h-full">
      {/* ヘッダ: コンテキストタイトル（StarredItemList と同パターン） */}
      <div className="flex-shrink-0 border-b px-4 py-2">
        <h2
          data-testid="cross-feed-item-list-title"
          className="text-sm font-medium"
        >
          すべての新着記事
        </h2>
      </div>

      {/* 記事一覧 */}
      <div className="flex-1 overflow-y-auto">
        {isError ? (
          <div
            data-testid="cross-feed-item-list-error"
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
            data-testid="cross-feed-item-list-empty"
            className="flex items-center justify-center h-32 text-sm text-muted-foreground"
          >
            新着記事はありません
          </div>
        ) : (
          <div className="flex flex-col">
            {allItems.map((item) => {
              const isExpanded = item.id === state.expandedItemId;
              const itemSummary = toItemSummary(item);
              return (
                <div key={item.id} className="flex flex-col">
                  <ItemRow
                    item={itemSummary}
                    isExpanded={isExpanded}
                    onClick={() => handleSelectItem(item.id)}
                  />
                  {/* フィード badge: favicon + フィード名（Req 3.1, 3.2, 3.4） */}
                  <div
                    data-testid={`cross-feed-item-badge-${item.id}`}
                    className="flex items-center gap-1.5 px-4 pb-2 -mt-1 text-xs text-muted-foreground"
                  >
                    <FeedFavicon
                      feedId={item.feed_id}
                      faviconURL={item.feed_favicon_url}
                      feedTitle={item.feed_title}
                    />
                    <span className="truncate">{item.feed_title}</span>
                  </div>
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

        {/* 無限スクロール用 sentinel */}
        <div
          ref={sentinelRef}
          data-testid="cross-feed-scroll-sentinel"
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

/**
 * `CrossFeedItem`（横断一覧の row 型）を既存 `ItemRow` の入力 `ItemSummary` 形状に
 * 射影する。横断一覧 API レスポンスには `hatebu_fetched_at` が含まれないため
 * null を補う（`ItemRow` 内では未参照のため挙動には影響しない）。
 */
function toItemSummary(item: CrossFeedItem): ItemSummary {
  return {
    id: item.id,
    feed_id: item.feed_id,
    title: item.title,
    link: item.link,
    summary: item.summary,
    published_at: item.published_at,
    is_date_estimated: item.is_date_estimated,
    is_read: item.is_read,
    is_starred: item.is_starred,
    hatebu_count: item.hatebu_count,
    hatebu_fetched_at: null,
  };
}
