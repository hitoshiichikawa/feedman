"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";

/** フェッチ間隔更新のパラメータ */
interface UpdateFetchIntervalParams {
  subscriptionId: string;
  fetchIntervalMinutes: number;
}

/**
 * フェッチ間隔を更新するmutationフック
 *
 * PATCH /api/subscriptions/:id に { fetch_interval_minutes: number } を送信する。
 */
export function useUpdateFetchInterval() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      subscriptionId,
      fetchIntervalMinutes,
    }: UpdateFetchIntervalParams) =>
      apiClient.patch(`/api/subscriptions/${subscriptionId}`, {
        fetch_interval_minutes: fetchIntervalMinutes,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}

/**
 * 購読を解除するmutationフック
 *
 * DELETE /api/subscriptions/:id を呼び出す。
 * 成功時にfeedsキャッシュを無効化する。
 */
export function useUnsubscribe() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (subscriptionId: string) =>
      apiClient.delete(`/api/subscriptions/${subscriptionId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}

/**
 * 停止中フィードのフェッチを再開するmutationフック
 *
 * POST /api/feeds/:feedId/resume を呼び出す。
 * 成功時にfeedsキャッシュを無効化する。
 */
export function useResumeFeed() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (feedId: string) => apiClient.post(`/api/feeds/${feedId}/resume`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}
