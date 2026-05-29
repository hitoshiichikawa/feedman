import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useStarredItems } from "./use-starred-items";
import type { StarredItemListResponse } from "@/types/item";
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

/** テスト用のスター記事一覧レスポンス（1ページ目、続きあり） */
const mockStarredPage1: StarredItemListResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-a",
      feed_title: "Feed A",
      title: "Feed A の最新スター記事",
      link: "https://example.com/a/article-1",
      summary: "Feed A の最新スター記事の概要",
      published_at: "2026-02-27T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: true,
      hatebu_count: 10,
      hatebu_fetched_at: "2026-02-27T09:00:00Z",
    },
    {
      id: "item-2",
      feed_id: "feed-b",
      feed_title: "Feed B",
      title: "Feed B の少し古いスター記事",
      link: "https://example.com/b/article-2",
      summary: "",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: true,
      is_read: true,
      is_starred: true,
      hatebu_count: 0,
      hatebu_fetched_at: null,
    },
  ],
  next_cursor: "2026-02-26T10:00:00Z",
  has_more: true,
};

/** テスト用のスター記事一覧レスポンス（2ページ目、末尾） */
const mockStarredPage2: StarredItemListResponse = {
  items: [
    {
      id: "item-3",
      feed_id: "feed-a",
      feed_title: "Feed A",
      title: "もっと古いスター記事",
      link: "https://example.com/a/article-3",
      summary: "もっと古いスター記事の概要",
      published_at: "2026-02-25T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: true,
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

describe("useStarredItems", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("初回リクエストで /api/feeds/starred/items?limit=50 を呼び出すこと（cursor なし）", async () => {
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useStarredItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // 初回リクエスト URL に /api/feeds/starred/items?limit=50 が含まれること
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/feeds/starred/items?limit=50"),
      expect.any(Object)
    );
    // 初回リクエストには cursor が含まれていないこと
    const firstCallUrl = mockFetch.mock.calls[0][0] as string;
    expect(firstCallUrl).not.toContain("cursor=");
  });

  it("レスポンスの items 各要素に feed_title が含まれること", async () => {
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useStarredItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const allItems = result.current.data?.pages.flatMap((p) => p.items) ?? [];
    expect(allItems).toHaveLength(2);
    expect(allItems[0].feed_title).toBe("Feed A");
    expect(allItems[1].feed_title).toBe("Feed B");
  });

  it("has_more=true のとき fetchNextPage で next_cursor を pageParam として送ること", async () => {
    let callCount = 0;
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/starred/items")) {
        callCount++;
        const page = callCount === 1 ? mockStarredPage1 : mockStarredPage2;
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

    const { result } = renderHook(() => useStarredItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // 1 ページ目取得後、hasNextPage が true
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

    // 2 回目リクエスト URL に next_cursor が cursor として含まれること
    // URLSearchParams は ":" を "%3A" にエンコードするので、生・エンコード後の両方を許容
    const secondCallUrl = mockFetch.mock.calls[1][0] as string;
    const expectedRaw = "cursor=2026-02-26T10:00:00Z";
    const expectedEncoded = "cursor=2026-02-26T10%3A00%3A00Z";
    expect(
      secondCallUrl.includes(expectedRaw) ||
        secondCallUrl.includes(expectedEncoded)
    ).toBe(true);
  });

  it("has_more=false のとき getNextPageParam が undefined を返し hasNextPage が false になること", async () => {
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage2,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    const { result } = renderHook(() => useStarredItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // has_more=false の応答に対して hasNextPage が false（= getNextPageParam が undefined を返している）
    expect(result.current.hasNextPage).toBe(false);
  });

  it("APIエラー時はエラー状態になること", async () => {
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/feeds/starred/items")) {
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

    const { result } = renderHook(() => useStarredItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });
});
