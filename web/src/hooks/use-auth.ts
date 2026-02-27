"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createApiClient } from "@/lib/api";
import type { User } from "@/types/auth";

/**
 * 現在のユーザー情報を取得するカスタムフック
 *
 * GET /auth/me を TanStack Query で呼び出し、
 * 認証済みユーザー情報を返す。
 * 未認証の場合は401エラーとなりisErrorがtrueになる。
 */
export function useCurrentUser() {
  const api = createApiClient();

  return useQuery<User>({
    queryKey: ["auth", "me"],
    queryFn: () => api.get<User>("/auth/me"),
    retry: false, // 認証エラーの場合はリトライしない
    staleTime: 5 * 60 * 1000, // 5分間キャッシュ
  });
}

/**
 * ログアウト処理を実行するカスタムフック
 *
 * POST /auth/logout を呼び出してセッションを破棄する。
 * 成功時にQueryClientのキャッシュをクリアする。
 */
export function useLogout() {
  const api = createApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.post("/auth/logout"),
    onSuccess: () => {
      // 全キャッシュをクリア
      queryClient.clear();
    },
  });
}
