import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useFeeds } from "./use-feeds";
import { CSRFProvider } from "@/lib/csrf";
import type { Subscription } from "@/types/feed";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

/** テスト用のQueryClient + CSRFProvider ラッパー */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <CSRFProvider>{children}</CSRFProvider>
      </QueryClientProvider>
    );
  };
}

/** テスト用の購読データ */
const mockSubscriptions: Subscription[] = [
  {
    id: "sub-1",
    user_id: "user-1",
    feed_id: "feed-1",
    feed_title: "Tech Blog",
    feed_url: "https://example.com/feed.xml",
    favicon_url: "https://example.com/favicon.ico",
    fetch_interval_minutes: 60,
    feed_status: "active",
    unread_count: 5,
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "sub-2",
    user_id: "user-1",
    feed_id: "feed-2",
    feed_title: "News Feed",
    feed_url: "https://news.example.com/rss",
    favicon_url: null,
    fetch_interval_minutes: 30,
    feed_status: "stopped",
    error_message: "404 Not Found",
    unread_count: 0,
    created_at: "2026-01-02T00:00:00Z",
  },
];

/**
 * mockFetchの呼び出しを設定するヘルパー。
 * CSRFProvider が /api/csrf-token を最初にfetchするため、
 * それに対するレスポンスも設定する必要がある。
 */
function setupMockFetch(apiResponse: Parameters<typeof mockFetch.mockImplementation>[0]) {
  mockFetch.mockImplementation((url: string) => {
    if (url === "/api/csrf-token") {
      return Promise.resolve({
        ok: true,
        json: async () => ({ token: "test-csrf-token" }),
      });
    }
    // API呼び出し
    if (typeof apiResponse === "function") {
      return apiResponse(url);
    }
    return apiResponse;
  });
}

describe("useFeeds", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("購読一覧を取得できること", async () => {
    setupMockFetch(() =>
      Promise.resolve({
        ok: true,
        json: async () => mockSubscriptions,
      })
    );

    const { result } = renderHook(() => useFeeds(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockSubscriptions);
    // /api/subscriptions への呼び出しがあること
    expect(mockFetch).toHaveBeenCalledWith("/api/subscriptions", {
      method: "GET",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
    });
  });

  it("取得中はローディング状態であること", () => {
    setupMockFetch((url: string) => {
      if (url === "/api/subscriptions") {
        return new Promise(() => {}); // 解決しない
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({ token: "test-csrf-token" }),
      });
    });

    const { result } = renderHook(() => useFeeds(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeUndefined();
  });

  it("APIエラー時はエラー状態になること", async () => {
    setupMockFetch((url: string) => {
      if (url === "/api/subscriptions") {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ message: "Internal Server Error" }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({ token: "test-csrf-token" }),
      });
    });

    const { result } = renderHook(() => useFeeds(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });
});
