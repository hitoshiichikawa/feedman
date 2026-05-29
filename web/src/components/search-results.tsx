"use client";

import { useCallback, useEffect, useRef } from "react";
import { Star } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAppState, useAppDispatch } from "@/contexts/app-state";
import { useItemSearch } from "@/hooks/use-item-search";
import { useItemDetail } from "@/hooks/use-items";
import { useMarkAsRead, useToggleStar } from "@/hooks/use-item-state";
import { ItemDetail } from "@/components/item-detail";
import type {
  ItemDetail as ItemDetailType,
  ItemSearchHit,
} from "@/types/item";

/**
 * 検索結果リスト（右ペイン）。
 *
 * `state.isSearching === true` のときに AppShell からレンダされ、AppState から
 * 検索クエリ / スコープ / フィード ID を読み取って {@link useItemSearch} を発火する。
 *
 * 状態出し分け（Req 4.3, 4.4, 4.5 / NFR 1.1）:
 * - `isLoading` ローディング表示（TanStack Query が即時 true、NFR 1.1 を満たす）
 * - `isError` エラー表示
 * - 結果 0 件で空状態表示
 * - 結果 1 件以上で時系列降順の結果リスト
 *
 * バッジ表示（Req 4.2）:
 * - `searchScope === 'global'`（横断検索）のみ各カードに feed_title + favicon を併記
 * - `searchScope === 'feed'`（フィード内検索）ではバッジを省略
 *
 * 操作整合（Req 4.6, 5.1, 5.2）:
 * - 既存 `useItemDetail` / `useMarkAsRead` / `useToggleStar` を再利用し、
 *   通常一覧と同じ展開・既読化・スター挙動を提供する
 */
export function SearchResults() {
  const state = useAppState();
  const dispatch = useAppDispatch();

  const {
    data,
    isLoading,
    isError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useItemSearch(state.searchQuery, state.searchScope, state.searchFeedId);

  // 展開中の記事詳細（本文を含む）を取得する
  const {
    data: detail,
    isLoading: isDetailLoading,
    isError: isDetailError,
  } = useItemDetail(state.expandedItemId);

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

  // Intersection Observer による無限スクロール（ItemList と同パターン）
  const sentinelRef = useRef<HTMLDivElement>(null);
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
    return () => observer.disconnect();
  }, [handleObserver]);

  // Req 4.4 / NFR 1.1: ローディング表示を即時提示する
  if (isLoading) {
    return (
      <div
        data-testid="search-results-loading"
        className="flex items-center justify-center h-full text-sm text-muted-foreground"
      >
        検索中...
      </div>
    );
  }

  // Req 4.5: 取得失敗時はエラー表示
  if (isError) {
    return (
      <div
        data-testid="search-results-error"
        className="flex items-center justify-center h-full text-sm text-destructive"
      >
        検索結果の取得に失敗しました
      </div>
    );
  }

  const allHits = data?.pages.flatMap((page) => page.items) ?? [];

  // Req 4.3: 0 件のときは空状態表示
  if (allHits.length === 0) {
    return (
      <div
        data-testid="search-results-empty"
        className="flex items-center justify-center h-full text-sm text-muted-foreground"
      >
        検索結果はありません
      </div>
    );
  }

  // 結果リスト（Req 4.1: published_at 降順は API 側で保証済み）
  return (
    <div data-testid="search-results" className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto">
        <div className="flex flex-col">
          {allHits.map((hit) => {
            const isExpanded = hit.id === state.expandedItemId;
            return (
              <div key={hit.id} className="flex flex-col">
                <SearchResultRow
                  hit={hit}
                  isExpanded={isExpanded}
                  showFeedBadge={state.searchScope === "global"}
                  onClick={() => handleSelectItem(hit.id)}
                />
                {isExpanded && (
                  <SearchResultDetailArea
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

        {/* 無限スクロール用 sentinel */}
        <div
          ref={sentinelRef}
          data-testid="search-results-sentinel"
          className="h-1"
        />

        {isFetchingNextPage && (
          <div className="flex items-center justify-center py-4 text-sm text-muted-foreground">
            読み込み中...
          </div>
        )}
      </div>
    </div>
  );
}

/** SearchResultDetailArea のプロパティ */
interface SearchResultDetailAreaProps {
  isLoading: boolean;
  isError: boolean;
  detail: ItemDetailType | null;
  detailItemId: string | null;
  onMarkAsRead: (itemId: string) => void;
  onToggleStar: (itemId: string, isStarred: boolean) => void;
}

/**
 * 検索結果の展開エリア（ItemList の ItemDetailArea と同等パターン）。
 *
 * 取得状態に応じてローディング表示 / エラー表示 / 本文（ItemDetail）を出し分ける。
 */
function SearchResultDetailArea({
  isLoading,
  isError,
  detail,
  detailItemId,
  onMarkAsRead,
  onToggleStar,
}: SearchResultDetailAreaProps) {
  if (isError) {
    return (
      <div
        data-testid="search-result-detail-error"
        className="border-t bg-background px-4 py-4 text-sm text-destructive"
      >
        記事の詳細を読み込めませんでした
      </div>
    );
  }

  if (isLoading || detail === null || detail.id !== detailItemId) {
    return (
      <div
        data-testid="search-result-detail-loading"
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

/** SearchResultRow のプロパティ */
interface SearchResultRowProps {
  hit: ItemSearchHit;
  isExpanded: boolean;
  /** true のとき feed_title + favicon バッジを表示する（横断検索時 / Req 4.2） */
  showFeedBadge: boolean;
  onClick: () => void;
}

/**
 * 検索結果の 1 行（item-list.tsx の `ItemRow` に検索固有のフィード識別バッジを追加した派生）。
 *
 * `showFeedBadge` が true のとき favicon と feed_title を併記する（Req 4.2）。
 * 検索結果固有の構造（next_cursor / has_more が API レスポンスに含まれる）と
 * 通常一覧の `ItemRow` で構造体が異なるため、共通化はせず併設する設計を採った。
 */
function SearchResultRow({
  hit,
  isExpanded,
  showFeedBadge,
  onClick,
}: SearchResultRowProps) {
  const date = hit.published_at !== null ? new Date(hit.published_at) : null;
  const formattedDate = date !== null ? formatDate(date) : "日付不明";
  const hasSummary = hit.summary.trim().length > 0;

  return (
    <button
      data-testid={`search-result-row-${hit.id}`}
      data-read={hit.is_read ? "true" : "false"}
      className={cn(
        "flex flex-col gap-1 px-4 py-3 text-left border-b transition-colors",
        "hover:bg-accent/50",
        isExpanded && "bg-accent",
        hit.is_read && "opacity-60"
      )}
      onClick={onClick}
    >
      {/* フィード識別バッジ（横断検索のみ / Req 4.2） */}
      {showFeedBadge && (
        <div
          data-testid={`search-result-feed-badge-${hit.id}`}
          className="flex items-center gap-1.5 text-xs text-muted-foreground"
        >
          <SearchResultFavicon
            faviconURL={hit.favicon_url}
            feedTitle={hit.feed_title}
          />
          <span className="truncate">{hit.feed_title}</span>
        </div>
      )}

      {/* タイトル行: タイトル(左) + 公開日時/スター(右) */}
      <div
        data-testid={`search-result-title-row-${hit.id}`}
        className="flex items-start gap-2"
      >
        <a
          href={hit.link}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(e) => e.stopPropagation()}
          className={cn(
            "flex-1 min-w-0 text-sm line-clamp-2 hover:underline cursor-pointer",
            !hit.is_read && "font-medium"
          )}
        >
          {hit.title}
        </a>

        <span className="flex flex-shrink-0 items-center gap-1 text-xs text-muted-foreground whitespace-nowrap">
          {date !== null && (
            <time dateTime={hit.published_at ?? undefined}>
              {formattedDate}
            </time>
          )}
          {hit.is_date_estimated && (
            <span
              data-testid={`search-result-date-estimated-${hit.id}`}
              className="text-orange-500"
              title="公開日が不明なため、取得日時を表示しています"
            >
              (推定)
            </span>
          )}
        </span>

        {hit.is_starred && (
          <Star
            className="flex-shrink-0 w-4 h-4 fill-yellow-400 text-yellow-400"
            data-testid={`search-result-star-${hit.id}`}
          />
        )}
      </div>

      {/* 概要（空のときは描画しない） */}
      {hasSummary && (
        <p
          data-testid={`search-result-summary-${hit.id}`}
          className="text-xs text-muted-foreground line-clamp-2"
        >
          {hit.summary}
        </p>
      )}
    </button>
  );
}

/** SearchResultFavicon のプロパティ */
interface SearchResultFaviconProps {
  faviconURL: string | null;
  feedTitle: string;
}

/**
 * 検索結果カードの先頭に表示する favicon。
 *
 * favicon URL が null / 空文字 / 画像読み込み失敗のいずれかなら何も表示せず、
 * バッジ全体としてはタイトルだけが残る（feed-list.tsx の代替アイコンよりも
 * 軽量な扱い、検索結果カードは情報密度が高くアイコン領域を最小化したい）。
 */
function SearchResultFavicon({
  faviconURL,
  feedTitle,
}: SearchResultFaviconProps) {
  if (faviconURL === null || faviconURL === "") {
    return null;
  }
  return (
    // <img> の onError は React のステート更新を介さず DOM 側にフォールバックを
    // 任せたいため、ここでは shouldShow フラグを介さず単純に <img> を返す。
    // 読み込み失敗時の代替表示が必要なら別 Issue で改善する。
    <img
      data-testid="search-result-favicon"
      src={faviconURL}
      alt={`${feedTitle} のアイコン`}
      className="w-3.5 h-3.5 rounded-sm object-contain flex-shrink-0"
    />
  );
}

/** 日付を相対表記でフォーマットする（item-list.tsx と同等ロジック） */
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
