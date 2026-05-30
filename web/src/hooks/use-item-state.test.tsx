import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  QueryClient,
  QueryClientProvider,
  type InfiniteData,
} from "@tanstack/react-query";

import { useMarkAsRead, useToggleStar } from "./use-item-state";
import type { ReactNode } from "react";
import type {
  ItemListResponse,
  ItemSearchResponse,
  ItemSearchHit,
  ItemSummary,
} from "@/types/item";

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

/** ItemSummary 用の最小 fixture */
function makeItem(
  id: string,
  overrides: Partial<ItemSummary> = {}
): ItemSummary {
  return {
    id,
    feed_id: "feed-1",
    title: `title-${id}`,
    link: `https://example.com/${id}`,
    summary: "",
    published_at: "2026-01-01T00:00:00Z",
    is_date_estimated: false,
    is_read: false,
    is_starred: false,
    hatebu_count: 0,
    hatebu_fetched_at: null,
    ...overrides,
  };
}

/** ItemSearchHit 用の最小 fixture */
function makeHit(
  id: string,
  overrides: Partial<ItemSearchHit> = {}
): ItemSearchHit {
  return {
    id,
    feed_id: "feed-1",
    title: `title-${id}`,
    link: `https://example.com/${id}`,
    summary: "",
    published_at: "2026-01-01T00:00:00Z",
    is_date_estimated: false,
    hatebu_count: 0,
    hatebu_fetched_at: null,
    feed_title: "Feed 1",
    favicon_url: null,
    is_read: false,
    is_starred: false,
    ...overrides,
  };
}

describe("useToggleStar - キャッシュ拡張（Issue #154）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  /**
   * 共有 QueryClient を持つ wrapper を返すヘルパー。
   * renderHook 経由でも queryClient.setQueryData / getQueryData を
   * テスト本体から操作・観測できる。
   */
  function createSharedClientWrapper() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    return { queryClient, wrapper };
  }

  it("['item-search'] キャッシュへの楽観反映: mutate 発火で is_starred が反転すること", async () => {
    // Arrange
    setupMockFetch();
    const { queryClient, wrapper } = createSharedClientWrapper();
    const searchKey = ["item-search", "global", "react"];
    const initialSearch: InfiniteData<ItemSearchResponse> = {
      pages: [
        {
          items: [
            makeHit("item-1", { is_starred: false }),
            makeHit("item-2", { is_starred: false }),
          ],
          next_cursor: null,
          has_more: false,
        },
      ],
      pageParams: [null],
    };
    queryClient.setQueryData(searchKey, initialSearch);

    const { result } = renderHook(() => useToggleStar(), { wrapper });

    // Act
    await act(async () => {
      result.current.mutate({ itemId: "item-1", isStarred: true });
    });

    // Assert: onMutate 時点で楽観反映されている
    const updated = queryClient.getQueryData<InfiniteData<ItemSearchResponse>>(
      searchKey
    );
    expect(updated?.pages[0].items[0].is_starred).toBe(true);
    expect(updated?.pages[0].items[1].is_starred).toBe(false);
  });

  it("onError 発火時に ['item-search'] キャッシュがロールバックされること", async () => {
    // Arrange: PUT が 500 エラーを返すよう mock
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
      text: async () => "internal error",
    });
    const { queryClient, wrapper } = createSharedClientWrapper();
    const searchKey = ["item-search", "feed", "feed-1", "react"];
    const originalHit = makeHit("item-1", { is_starred: false });
    const initialSearch: InfiniteData<ItemSearchResponse> = {
      pages: [
        {
          items: [originalHit],
          next_cursor: null,
          has_more: false,
        },
      ],
      pageParams: [null],
    };
    queryClient.setQueryData(searchKey, initialSearch);

    const { result } = renderHook(() => useToggleStar(), { wrapper });

    // Act
    await act(async () => {
      result.current.mutate({ itemId: "item-1", isStarred: true });
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    // Assert: ロールバック後は元の is_starred=false に戻っている
    const rolledBack = queryClient.getQueryData<
      InfiniteData<ItemSearchResponse>
    >(searchKey);
    expect(rolledBack?.pages[0].items[0].is_starred).toBe(false);
  });

  it("['items'] 系の既存挙動が非回帰であること（楽観反映 + ロールバック）", async () => {
    // Arrange: 失敗系で ["items"] のロールバックを確認
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
      text: async () => "internal error",
    });
    const { queryClient, wrapper } = createSharedClientWrapper();
    const itemsKey = ["items", "feed-1", "all"];
    const initial: InfiniteData<ItemListResponse> = {
      pages: [
        {
          items: [makeItem("item-1", { is_starred: false })],
          next_cursor: null,
          has_more: false,
        },
      ],
      pageParams: [null],
    };
    queryClient.setQueryData(itemsKey, initial);

    const { result } = renderHook(() => useToggleStar(), { wrapper });

    // Act
    await act(async () => {
      result.current.mutate({ itemId: "item-1", isStarred: true });
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    // Assert: ["items"] 系もロールバックされていること
    const rolledBack = queryClient.getQueryData<
      InfiniteData<ItemListResponse>
    >(itemsKey);
    expect(rolledBack?.pages[0].items[0].is_starred).toBe(false);
  });

  it("['item-search'] キャッシュが存在しない場合でも mutate が成功し、['items'] の楽観更新が機能すること", async () => {
    // Arrange: ["item-search"] キャッシュなし
    setupMockFetch();
    const { queryClient, wrapper } = createSharedClientWrapper();
    const itemsKey = ["items", "feed-1", "all"];
    const initial: InfiniteData<ItemListResponse> = {
      pages: [
        {
          items: [makeItem("item-1", { is_starred: false })],
          next_cursor: null,
          has_more: false,
        },
      ],
      pageParams: [null],
    };
    queryClient.setQueryData(itemsKey, initial);

    const { result } = renderHook(() => useToggleStar(), { wrapper });

    // Act
    await act(async () => {
      result.current.mutate({ itemId: "item-1", isStarred: true });
    });

    // Assert: 楽観更新が反映されること
    const updated = queryClient.getQueryData<InfiniteData<ItemListResponse>>(
      itemsKey
    );
    expect(updated?.pages[0].items[0].is_starred).toBe(true);
  });
});
