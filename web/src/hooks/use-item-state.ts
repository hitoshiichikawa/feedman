"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ItemListResponse, ItemSearchResponse } from "@/types/item";
import type { InfiniteData, QueryKey } from "@tanstack/react-query";

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
 * `useToggleStar` の onMutate が返す context 型。
 *
 * `["items"]` 系（既存）と `["item-search"]` 系（Issue #154 で追加）の両方の
 * スナップショットを保持し、onError でのロールバックに使用する。
 * `["cross-feed-items"]` は楽観更新の対象外（invalidate のみ）であるため context に含めない。
 */
interface ToggleStarContext {
  previousItems: Array<{
    key: QueryKey;
    data: InfiniteData<ItemListResponse> | undefined;
  }>;
  previousSearch: Array<{
    key: QueryKey;
    data: InfiniteData<ItemSearchResponse> | undefined;
  }>;
  /**
   * 後方互換のため `previousItems` のエイリアスを残す（旧 context shape）。
   * 将来 cleanup で除去予定。
   */
  previousData: Array<{
    key: QueryKey;
    data: InfiniteData<ItemListResponse> | undefined;
  }>;
}

/**
 * スターを切り替えるmutationフック（楽観的更新付き）
 *
 * PUT /api/items/:id/state に { is_starred: boolean } を送信する。
 * 楽観的更新でUIを即時反映し、エラー時にロールバックする。
 *
 * 対象キャッシュ:
 * - `["items"]`: フィード別 / `starred` 横断 一覧（前置キー共有でまとめて hit）
 * - `["item-search"]`: 検索結果一覧（Issue #154 で追加）
 * - `["cross-feed-items"]`: invalidate のみ（楽観更新は行わない / 既存挙動を維持）
 */
export function useToggleStar() {
  const queryClient = useQueryClient();

  return useMutation<unknown, Error, ToggleStarParams, ToggleStarContext>({
    mutationFn: ({ itemId, isStarred }: ToggleStarParams) =>
      apiClient.put(`/api/items/${itemId}/state`, { is_starred: isStarred }),
    onMutate: async ({ itemId, isStarred }) => {
      // 進行中のrefetchをキャンセル
      await queryClient.cancelQueries({ queryKey: ["items"] });
      await queryClient.cancelQueries({ queryKey: ["item-search"] });

      // ["items"] 系の楽観的更新
      const itemsCache = queryClient.getQueriesData<
        InfiniteData<ItemListResponse>
      >({ queryKey: ["items"] });
      const previousItems = itemsCache.map(([key, data]) => ({ key, data }));

      itemsCache.forEach(([key, data]) => {
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

      // ["item-search"] 系の楽観的更新（Issue #154 / Req 2.1, 2.2, 2.4, 2.5）
      const searchCache = queryClient.getQueriesData<
        InfiniteData<ItemSearchResponse>
      >({ queryKey: ["item-search"] });
      const previousSearch = searchCache.map(([key, data]) => ({ key, data }));

      searchCache.forEach(([key, data]) => {
        if (!data) return;
        queryClient.setQueryData<InfiniteData<ItemSearchResponse>>(key, {
          ...data,
          pages: data.pages.map((page) => ({
            ...page,
            items: page.items.map((hit) =>
              hit.id === itemId ? { ...hit, is_starred: isStarred } : hit
            ),
          })),
        });
      });

      return { previousItems, previousSearch, previousData: previousItems };
    },
    onError: (_err, _vars, context) => {
      // エラー時にロールバック
      if (context?.previousItems) {
        context.previousItems.forEach(({ key, data }) => {
          queryClient.setQueryData(key, data);
        });
      }
      if (context?.previousSearch) {
        context.previousSearch.forEach(({ key, data }) => {
          queryClient.setQueryData(key, data);
        });
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["items"] });
      // 検索結果一覧（Issue #154 / Req 2.5）もスター同期させる
      queryClient.invalidateQueries({ queryKey: ["item-search"] });
      // 横断新着一覧（Issue #121 / Req 5.3）もスター同期させる
      queryClient.invalidateQueries({ queryKey: ["cross-feed-items"] });
    },
  });
}
