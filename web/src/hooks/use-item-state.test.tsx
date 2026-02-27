import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { useMarkAsRead, useToggleStar } from "./use-item-state";
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

describe("useMarkAsRead", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("記事を既読にするAPIを呼び出せること", async () => {
    setupMockFetch((url: string, options?: RequestInit) => {
      if (url.includes("/api/items/item-1/state") && options?.method === "PUT") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            user_id: "user-1",
            item_id: "item-1",
            is_read: true,
            is_starred: false,
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useMarkAsRead(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate("item-1");
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // PUT /api/items/item-1/state にis_read: trueが送信されること
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/items/item-1/state",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ is_read: true }),
      })
    );
  });
});

describe("useToggleStar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("スターを切り替えるAPIを呼び出せること", async () => {
    setupMockFetch((url: string, options?: RequestInit) => {
      if (url.includes("/api/items/item-1/state") && options?.method === "PUT") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            user_id: "user-1",
            item_id: "item-1",
            is_read: true,
            is_starred: true,
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useToggleStar(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ itemId: "item-1", isStarred: true });
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // PUT /api/items/item-1/state にis_starred: trueが送信されること
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/items/item-1/state",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ is_starred: true }),
      })
    );
  });

  it("スターを外すAPIを呼び出せること", async () => {
    setupMockFetch((url: string, options?: RequestInit) => {
      if (url.includes("/api/items/item-1/state") && options?.method === "PUT") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            user_id: "user-1",
            item_id: "item-1",
            is_read: true,
            is_starred: false,
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useToggleStar(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ itemId: "item-1", isStarred: false });
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/items/item-1/state",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ is_starred: false }),
      })
    );
  });
});
