"use client";

import { useQuery } from "@tanstack/react-query";
import { createApiClient } from "@/lib/api";
import type { Subscription } from "@/types/feed";

/**
 * フィード一覧を取得するカスタムフック
 *
 * GET /api/subscriptions を TanStack Query で呼び出し、
 * ユーザーの購読一覧（フィードタイトル、favicon、fetch_status、未読数を含む）を返す。
 */
export function useFeeds() {
  const api = createApiClient();

  return useQuery<Subscription[]>({
    queryKey: ["feeds"],
    queryFn: () => api.get<Subscription[]>("/api/subscriptions"),
  });
}
