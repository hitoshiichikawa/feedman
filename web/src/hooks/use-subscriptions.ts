"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createApiClient } from "@/lib/api";

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
  const api = createApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      subscriptionId,
      fetchIntervalMinutes,
    }: UpdateFetchIntervalParams) =>
      api.patch(`/api/subscriptions/${subscriptionId}`, {
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
  const api = createApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (subscriptionId: string) =>
      api.delete(`/api/subscriptions/${subscriptionId}`),
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
  const api = createApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (feedId: string) => api.post(`/api/feeds/${feedId}/resume`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}
