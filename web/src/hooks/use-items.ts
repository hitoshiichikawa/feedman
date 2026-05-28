"use client";

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ItemDetail, ItemFilter, ItemListResponse } from "@/types/item";

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
  return useInfiniteQuery<ItemListResponse>({
    queryKey: ["items", feedId, filter],
    queryFn: async ({ pageParam }) => {
      const params = new URLSearchParams();
      params.set("filter", filter);
      params.set("limit", "50");
      if (pageParam) {
        params.set("cursor", pageParam as string);
      }
      return apiClient.get<ItemListResponse>(
        `/api/feeds/${feedId}/items?${params.toString()}`
      );
    },
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_cursor : undefined,
    enabled: feedId !== null,
  });
}

/**
 * 記事詳細（本文を含む）を取得するカスタムフック
 *
 * GET /api/items/:id を TanStack Query の useQuery で呼び出し、ItemDetail 形状を取得する。
 * 一覧 API はサマリー（content なし）のみ返すため、記事詳細の展開時に本文を別取得する用途で使う。
 * itemId が null の場合（記事詳細が未展開の場合）はクエリを無効化し、リクエストを送信しない。
 *
 * @param itemId - 取得対象の記事ID（null の場合はクエリ無効化）
 */
export function useItemDetail(itemId: string | null) {
  return useQuery<ItemDetail>({
    queryKey: ["item", itemId],
    queryFn: () => apiClient.get<ItemDetail>(`/api/items/${itemId}`),
    enabled: itemId !== null,
  });
}
