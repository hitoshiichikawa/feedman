"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ItemSearchResponse, SearchScope } from "@/types/item";

/** 検索結果 1 ページあたりの取得件数（既存 ItemList と同じ 50 件） */
const SEARCH_PAGE_SIZE = 50;

/**
 * 記事検索を実行するカスタムフック（無限スクロール対応）。
 *
 * GET /api/items/search を TanStack Query の useInfiniteQuery で呼び出し、
 * カーソルベースページネーション（{@link SEARCH_PAGE_SIZE} 件 / 回）で検索結果を取得する。
 *
 * - 横断検索（`scope === 'global'`）: クエリパラメータ `q`, `limit`, 任意 `cursor` を付与する
 * - フィード内検索（`scope === 'feed'`）: 上記に加えて `feed_id` を付与する
 *
 * `enabled` ガード:
 * - クエリ正規化後（前後空白 trim 後）に空文字なら検索しない（Req 1.5）
 * - `scope === 'feed'` かつ `feedId` が null / 空文字なら検索しない（Req 1.2 / NFR 2.3 と整合）
 *
 * `getNextPageParam` ガード:
 * - `has_more === false` のときは次ページなし
 * - `next_cursor` が null / 空文字のときも次ページなし（impl-notes Task 4 / 5 の判断）
 *
 * @param query - 検索キーワード（前後空白は呼び出し側で正規化済みでも良い）
 * @param scope - 検索スコープ（'global' = 横断 / 'feed' = フィード内）
 * @param feedId - フィード内検索の対象フィード ID（scope='feed' のときのみ必須）
 */
export function useItemSearch(
  query: string,
  scope: SearchScope,
  feedId: string | null
) {
  const trimmedQuery = query.trim();
  const isEnabled =
    trimmedQuery.length > 0 && !(scope === "feed" && !feedId);

  return useInfiniteQuery<ItemSearchResponse>({
    queryKey: ["item-search", trimmedQuery, scope, feedId],
    queryFn: async ({ pageParam }) => {
      const params = new URLSearchParams();
      params.set("q", trimmedQuery);
      params.set("limit", String(SEARCH_PAGE_SIZE));
      if (scope === "feed" && feedId) {
        params.set("feed_id", feedId);
      }
      if (pageParam) {
        params.set("cursor", pageParam as string);
      }
      return apiClient.get<ItemSearchResponse>(
        `/api/items/search?${params.toString()}`
      );
    },
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) =>
      lastPage.has_more && lastPage.next_cursor
        ? lastPage.next_cursor
        : undefined,
    enabled: isEnabled,
  });
}
