import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useItems } from "./use-items";
import type { ItemListResponse } from "@/types/item";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

/** テスト用のQueryClientラッパー */
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
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

/** テスト用の記事一覧レスポンス（1ページ目） */
const mockPage1: ItemListResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-1",
      title: "最新の記事",
      link: "https://example.com/article-1",
      published_at: "2026-02-27T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: false,
      hatebu_count: 10,
      hatebu_fetched_at: "2026-02-27T09:00:00Z",
    },
    {
      id: "item-2",
      feed_id: "feed-1",
      title: "少し古い記事",
      link: "https://example.com/article-2",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: true,
      is_read: true,
      is_starred: true,
      hatebu_count: 0,
      hatebu_fetched_at: null,
    },
  ],
  next_cursor: "2026-02-26T10:00:00Z_item-2",
  has_more: true,
};

/** テスト用の記事一覧レスポンス（2ページ目） */
const mockPage2: ItemListResponse = {
  items: [
    {
      id: "item-3",
      feed_id: "feed-1",
      title: "もっと古い記事",
      link: "https://example.com/article-3",
      published_at: "2026-02-25T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: false,
      hatebu_count: 5,
      hatebu_fetched_at: "2026-02-26T00:00:00Z",
    },
  ],
  next_cursor: null,
  has_more: false,
};

/** mockFetchのルーティング設定ヘルパー */
function setupMockFetch(
  apiHandler: (url: string, options?: RequestInit) => Promise<unknown>
) {
  mockFetch.mockImplementation(apiHandler);
}

describe("useItems", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("feedIdが指定された場合に記事一覧を取得できること", async () => {
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/feed-1/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockPage1,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useItems("feed-1", "all"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // 最初のページのデータが取得されていること
    const allItems = result.current.data?.pages.flatMap((p) => p.items) ?? [];
    expect(allItems).toHaveLength(2);
    expect(allItems[0].title).toBe("最新の記事");
    expect(allItems[1].title).toBe("少し古い記事");
  });

  it("feedIdがnullの場合はクエリを無効化すること", () => {
    setupMockFetch(() =>
      Promise.resolve({ ok: true, json: async () => ({}) })
    );

    const { result } = renderHook(() => useItems(null, "all"), {
      wrapper: createWrapper(),
    });

    // クエリが無効化されているため、fetchPendingの状態のまま
    expect(result.current.isFetching).toBe(false);
  });

  it("フィルタパラメータがAPIリクエストに含まれること", async () => {
    setupMockFetch((url: string) => {
      if (url.includes("/api/feeds/feed-1/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockPage1,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useItems("feed-1", "unread"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // filter=unread がURLに含まれること
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("filter=unread"),
      expect.any(Object)
    );
  });

  it("hasMoreがtrueの場合にfetchNextPageで次ページを取得できること", async () => {
    let callCount = 0;
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/feed-1/items")) {
        callCount++;
        const page = callCount === 1 ? mockPage1 : mockPage2;
        return Promise.resolve({
          ok: true,
          json: async () => page,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useItems("feed-1", "all"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // 最初のページ取得後、hasNextPageがtrue
    expect(result.current.hasNextPage).toBe(true);

    // 次ページを取得
    await act(async () => {
      await result.current.fetchNextPage();
    });

    await waitFor(() => {
      const allItems =
        result.current.data?.pages.flatMap((p) => p.items) ?? [];
      expect(allItems).toHaveLength(3);
    });

    // 次ページ取得後、hasNextPageがfalse
    expect(result.current.hasNextPage).toBe(false);
  });

  it("APIエラー時はエラー状態になること", async () => {
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/feed-1/items")) {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ message: "Internal Server Error" }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useItems("feed-1", "all"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });
});
