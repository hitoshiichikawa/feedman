import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { useManualRefresh } from "./use-manual-refresh";
import type { ReactNode } from "react";

// グローバル fetch のモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

interface WrapperResult {
  Wrapper: ({ children }: { children: ReactNode }) => React.ReactElement;
  queryClient: QueryClient;
}

/** テスト用の QueryClient ラッパー（invalidateQueries 観測のため queryClient も返す） */
function createWrapper(): WrapperResult {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { Wrapper, queryClient };
}

describe("useManualRefresh", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("POST /api/subscriptions/:id/fetch を呼び出して 200 で解決すること", async () => {
    // Arrange
    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/subscriptions/sub-1/fetch" && options?.method === "POST") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ id: "sub-1" }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => useManualRefresh("feed-1"), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate("sub-1");
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/subscriptions/sub-1/fetch",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("成功時に items と feeds のキャッシュが invalidate されること", async () => {
    // Arrange
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ id: "sub-1" }),
    });
    const { Wrapper, queryClient } = createWrapper();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useManualRefresh("feed-1"), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate("sub-1");
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    // ["items", feedId] と ["feeds"] の 2 回 invalidate が呼ばれる（Req 6.1 / 6.2）
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["items", "feed-1"],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["feeds"] });
    expect(invalidateSpy).toHaveBeenCalledTimes(2);
  });

  it("API がエラーを返したときに invalidate が呼ばれないこと（Req 7.5）", async () => {
    // Arrange: 429 FEED_COOLDOWN を返す
    mockFetch.mockResolvedValue({
      ok: false,
      status: 429,
      json: async () => ({
        error: {
          code: "FEED_COOLDOWN",
          message: "cooldown",
          category: "feed",
          action: "再試行待機",
          details: { retry_after_seconds: 300 },
        },
      }),
    });
    const { Wrapper, queryClient } = createWrapper();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useManualRefresh("feed-1"), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate("sub-1");
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.status).toBe(429);
    expect(invalidateSpy).not.toHaveBeenCalled();
  });
});
