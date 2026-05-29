"use client";

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import { apiClient, ApiError } from "@/lib/api";

/**
 * 指定購読の手動フェッチを起動する mutation フック。
 *
 * `mutate(subscriptionId)` 呼び出し中は `isPending=true` となり、
 * 成功時に `["items", feedId]` と `["feeds"]` のキャッシュを invalidate して
 * 記事一覧と未読バッジを最新化する（Req 6.1 / 6.2）。
 *
 * エラー時はキャッシュ invalidate を行わず、呼び出し側は `mutation.error`
 * 経由で `ApiError` を読み、バナー等で通知する（Req 7.5）。
 *
 * @param feedId - 対象フィードID（記事一覧のクエリキー解決に使用）
 */
export function useManualRefresh(
  feedId: string | null
): UseMutationResult<void, ApiError, string> {
  const queryClient = useQueryClient();

  return useMutation<void, ApiError, string>({
    mutationFn: async (subscriptionId: string) => {
      await apiClient.post<unknown>(
        `/api/subscriptions/${subscriptionId}/fetch`
      );
    },
    onSuccess: () => {
      // 記事一覧と未読バッジを最新化する（Req 6.1 / 6.2）
      queryClient.invalidateQueries({ queryKey: ["items", feedId] });
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
    // onError では invalidate しない: 失敗時は記事一覧の表示内容を維持する（Req 7.5）
  });
}
