"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { ThemeProvider } from "@/components/theme-provider";
import { CSRFProvider } from "@/lib/csrf";
import { AppStateProvider } from "@/contexts/app-state";

/**
 * アプリケーション全体のプロバイダーコンポーネント
 *
 * QueryClientProvider、ThemeProvider、CSRFProvider を含む各種プロバイダーで
 * アプリケーションをラップする。
 * Client Component として実装し、layout.tsx (Server Component) から使用する。
 */
export function Providers({ children }: { children: React.ReactNode }) {
  // useState で QueryClient を生成し、再レンダリング時の再生成を防ぐ
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // サーバーデータのデフォルトキャッシュ設定
            staleTime: 60 * 1000, // 1分間はキャッシュを新鮮とみなす
            retry: 1, // リトライは1回まで
          },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <CSRFProvider>
          <AppStateProvider>{children}</AppStateProvider>
        </CSRFProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
