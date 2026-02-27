import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import {
  useUpdateFetchInterval,
  useUnsubscribe,
  useResumeFeed,
} from "./use-subscriptions";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

/** テスト用のQueryClientラッパー */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

/** mockFetchの設定ヘルパー */
function setupMockFetch(
  apiHandler?: (url: string, options?: RequestInit) => Promise<unknown>
) {
  if (apiHandler) {
    mockFetch.mockImplementation(apiHandler);
  } else {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({}),
    });
  }
}

describe("useUpdateFetchInterval", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("フェッチ間隔を更新するAPIを呼び出せること", async () => {
    setupMockFetch((url: string, options?: RequestInit) => {
      if (
        url === "/api/subscriptions/sub-1" &&
        options?.method === "PATCH"
      ) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "sub-1",
            fetch_interval_minutes: 120,
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useUpdateFetchInterval(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        subscriptionId: "sub-1",
        fetchIntervalMinutes: 120,
      });
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/subscriptions/sub-1",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ fetch_interval_minutes: 120 }),
      })
    );
  });
});

describe("useUnsubscribe", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("購読解除APIを呼び出せること", async () => {
    setupMockFetch((url: string, options?: RequestInit) => {
      if (
        url === "/api/subscriptions/sub-1" &&
        options?.method === "DELETE"
      ) {
        return Promise.resolve({
          ok: true,
          json: async () => ({}),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useUnsubscribe(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate("sub-1");
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/subscriptions/sub-1",
      expect.objectContaining({
        method: "DELETE",
      })
    );
  });
});

describe("useResumeFeed", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("フィード再開APIを呼び出せること", async () => {
    setupMockFetch((url: string, options?: RequestInit) => {
      if (
        url === "/api/feeds/feed-1/resume" &&
        options?.method === "POST"
      ) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "sub-1",
            feed_id: "feed-1",
            feed_status: "active",
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useResumeFeed(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate("feed-1");
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/feeds/feed-1/resume",
      expect.objectContaining({
        method: "POST",
      })
    );
  });
});
