"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { StarredItemListResponse } from "@/types/item";

/**
 * 全フィード横断のスター記事一覧を取得するカスタムフック（無限スクロール対応）
 *
 * GET /api/feeds/starred/items を useInfiniteQuery で呼び出し、カーソルベース
 * ページネーション（50 件/回）を実装する。
 *
 * queryKey は ["items", "starred"]。前置キー "items" を既存 useItems と共有することで、
 * useToggleStar の onSettled が発行する invalidateQueries({ queryKey: ["items"] })
 * によって横断キャッシュも自動的に invalidate される（Req 3.2 / 3.3 / 3.4）。
 */
export function useStarredItems() {
  return useInfiniteQuery<StarredItemListResponse>({
    queryKey: ["items", "starred"],
    queryFn: async ({ pageParam }) => {
      const params = new URLSearchParams();
      params.set("limit", "50");
      if (pageParam) {
        params.set("cursor", pageParam as string);
      }
      return apiClient.get<StarredItemListResponse>(
        `/api/feeds/starred/items?${params.toString()}`
      );
    },
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_cursor : undefined,
  });
}
