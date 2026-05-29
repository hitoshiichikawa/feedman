"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ItemListResponse } from "@/types/item";
import type { InfiniteData } from "@tanstack/react-query";

/**
 * 記事を既読にするmutationフック
 *
 * PUT /api/items/:id/state に { is_read: true } を送信する。
 * 成功時にitemsクエリキャッシュを無効化する。
 */
export function useMarkAsRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (itemId: string) =>
      apiClient.put(`/api/items/${itemId}/state`, { is_read: true }),
    onSuccess: () => {
      // itemsとfeedsの未読数キャッシュを無効化
      queryClient.invalidateQueries({ queryKey: ["items"] });
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
      // 横断新着一覧（Issue #121 / Req 5.3）も既読同期させる
      queryClient.invalidateQueries({ queryKey: ["cross-feed-items"] });
    },
  });
}

/** スター切替のパラメータ */
interface ToggleStarParams {
  itemId: string;
  isStarred: boolean;
}

/**
 * スターを切り替えるmutationフック（楽観的更新付き）
 *
 * PUT /api/items/:id/state に { is_starred: boolean } を送信する。
 * 楽観的更新でUIを即時反映し、エラー時にロールバックする。
 */
export function useToggleStar() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId, isStarred }: ToggleStarParams) =>
      apiClient.put(`/api/items/${itemId}/state`, { is_starred: isStarred }),
    onMutate: async ({ itemId, isStarred }) => {
      // 進行中のrefetchをキャンセル
      await queryClient.cancelQueries({ queryKey: ["items"] });

      // 楽観的更新: itemsキャッシュ内の該当記事のis_starredを即時更新
      const queryCache = queryClient.getQueriesData<
        InfiniteData<ItemListResponse>
      >({ queryKey: ["items"] });

      const previousData = queryCache.map(([key, data]) => ({ key, data }));

      queryCache.forEach(([key, data]) => {
        if (!data) return;
        queryClient.setQueryData<InfiniteData<ItemListResponse>>(key, {
          ...data,
          pages: data.pages.map((page) => ({
            ...page,
            items: page.items.map((item) =>
              item.id === itemId ? { ...item, is_starred: isStarred } : item
            ),
          })),
        });
      });

      return { previousData };
    },
    onError: (_err, _vars, context) => {
      // エラー時にロールバック
      if (context?.previousData) {
        context.previousData.forEach(({ key, data }) => {
          queryClient.setQueryData(key, data);
        });
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["items"] });
      // 横断新着一覧（Issue #121 / Req 5.3）もスター同期させる
      queryClient.invalidateQueries({ queryKey: ["cross-feed-items"] });
    },
  });
}
