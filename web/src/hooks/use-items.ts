"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { createApiClient } from "@/lib/api";
import type { ItemFilter, ItemListResponse } from "@/types/item";

/**
 * 記事一覧を取得するカスタムフック（無限スクロール対応）
 *
 * GET /api/feeds/:feedId/items をTanStack QueryのuseInfiniteQueryで呼び出し、
 * カーソルベースページネーション（50件/回）を実装する。
 *
 * @param feedId - 取得対象のフィードID（nullの場合はクエリ無効化）
 * @param filter - フィルタ種別（all / unread / starred）
 */
export function useItems(feedId: string | null, filter: ItemFilter) {
  const api = createApiClient();

  return useInfiniteQuery<ItemListResponse>({
    queryKey: ["items", feedId, filter],
    queryFn: async ({ pageParam }) => {
      const params = new URLSearchParams();
      params.set("filter", filter);
      params.set("limit", "50");
      if (pageParam) {
        params.set("cursor", pageParam as string);
      }
      return api.get<ItemListResponse>(
        `/api/feeds/${feedId}/items?${params.toString()}`
      );
    },
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_cursor : undefined,
    enabled: feedId !== null,
  });
}
