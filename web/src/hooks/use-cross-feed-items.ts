"use client";

import { useInfiniteQuery, useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import { useAppState } from "@/contexts/app-state";
import type { CrossFeedListResponse } from "@/types/crossfeed";

/**
 * フィード横断新着一覧を取得するカスタムフック（無限スクロール対応、Issue #121）。
 *
 * GET /api/items/cross-feed を useInfiniteQuery で呼び出し、カーソルベース
 * ページネーション（50 件/回）を実装する。
 *
 * セッション内 baseline (`crossFeedSessionSince`) を AppStateContext から読み出し、
 * 非 null のときは URL に `since=<encoded>` を付与してサーバ側に明示的に渡す（Req 4.7）。
 * queryKey に `crossFeedSessionSince ?? 'initial'` を含めることで、baseline 固定の
 * 前後で別キャッシュとして扱い、確実に refetch を発火させる。
 *
 * staleTime: 0 とすることで毎マウント時に必ず最新を取得する（横断 ⇄ 個別の往復後の
 * 整合性を担保）。
 */
export function useCrossFeedItems() {
  const { crossFeedSessionSince } = useAppState();

  return useInfiniteQuery<CrossFeedListResponse>({
    queryKey: ["cross-feed-items", crossFeedSessionSince ?? "initial"],
    queryFn: ({ pageParam }) => {
      let url = "/api/items/cross-feed?limit=50";
      if (pageParam) {
        url += `&cursor=${encodeURIComponent(pageParam as string)}`;
      }
      if (crossFeedSessionSince) {
        url += `&since=${encodeURIComponent(crossFeedSessionSince)}`;
      }
      return apiClient.get<CrossFeedListResponse>(url);
    },
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_cursor : undefined,
    staleTime: 0,
  });
}

/**
 * 横断新着一覧の last_seen_at を更新する mutation フック（Issue #121 / Req 4.3）。
 *
 * PUT /api/users/me/cross-feed-last-seen を呼び出す。リクエストボディなし、
 * レスポンスは 204 No Content。`CrossFeedItemList` のセッション初回マウント時に
 * 1 回だけ呼び出される想定。
 */
export function useTouchCrossFeedLastSeen() {
  return useMutation({
    mutationFn: () => apiClient.put("/api/users/me/cross-feed-last-seen"),
  });
}
