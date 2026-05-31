import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useItemSearch } from "./use-item-search";
import type { ItemSearchResponse } from "@/types/item";
import type { ReactNode } from "react";

// グローバル fetch のモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

/** テスト用の QueryClient ラッパー */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

/** テスト用の検索結果 1 ページ目（横断検索想定） */
const mockSearchPage1: ItemSearchResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-a",
      title: "TypeScript 入門",
      link: "https://example.com/article-1",
      summary: "TypeScript の基礎をまとめた記事",
      published_at: "2026-02-27T10:00:00Z",
      is_date_estimated: false,
      hatebu_count: 12,
      hatebu_fetched_at: null,
      feed_title: "Web Frontend Blog",
      favicon_url: "data:image/png;base64,AAA",
      is_read: false,
      is_starred: false,
    },
    {
      id: "item-2",
      feed_id: "feed-b",
      title: "TypeScript の型推論",
      link: "https://example.com/article-2",
      summary: "",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: true,
      hatebu_count: 0,
      hatebu_fetched_at: null,
      feed_title: "Tech Notes",
      favicon_url: null,
      is_read: true,
      is_starred: true,
    },
  ],
  next_cursor: "2026-02-26T10:00:00Z|item-2",
  has_more: true,
};

/** テスト用の検索結果 2 ページ目 */
const mockSearchPage2: ItemSearchResponse = {
  items: [
    {
      id: "item-3",
      feed_id: "feed-c",
      title: "古い TypeScript 記事",
      link: "https://example.com/article-3",
      summary: "v3 系の話題",
      published_at: "2026-02-25T10:00:00Z",
      is_date_estimated: false,
      hatebu_count: 3,
      hatebu_fetched_at: null,
      feed_title: "Archive",
      favicon_url: "data:image/png;base64,BBB",
      is_read: false,
      is_starred: false,
    },
  ],
  next_cursor: null,
  has_more: false,
};

describe("useItemSearch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("scope='global' でクエリを指定したとき q と limit パラメータ付きで /api/items/search を呼ぶこと", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSearchPage1,
    });

    const { result } = renderHook(
      () => useItemSearch("typescript", "global", null),
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // 検索結果が取得できていること
    const allItems = result.current.data?.pages.flatMap((p) => p.items) ?? [];
    expect(allItems).toHaveLength(2);
    expect(allItems[0].title).toBe("TypeScript 入門");

    // q=typescript と limit=50 を含む URL が呼ばれている
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain("/api/items/search?");
    expect(calledUrl).toContain("q=typescript");
    expect(calledUrl).toContain("limit=50");
    // 横断検索では feed_id を付与しない
    expect(calledUrl).not.toContain("feed_id=");
  });

  it("scope='feed' で feedId を指定したとき feed_id パラメータ付きで呼ぶこと", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSearchPage1,
    });

    const { result } = renderHook(
      () => useItemSearch("rust", "feed", "feed-42"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain("q=rust");
    expect(calledUrl).toContain("feed_id=feed-42");
    expect(calledUrl).toContain("limit=50");
  });

  it("空クエリ（空文字）のときはクエリを無効化し fetch を呼ばないこと", () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSearchPage1,
    });

    const { result } = renderHook(() => useItemSearch("", "global", null), {
      wrapper: createWrapper(),
    });

    expect(result.current.isFetching).toBe(false);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("空白のみクエリのときはクエリを無効化し fetch を呼ばないこと", () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSearchPage1,
    });

    const { result } = renderHook(
      () => useItemSearch("   ", "global", null),
      { wrapper: createWrapper() }
    );

    expect(result.current.isFetching).toBe(false);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("scope='feed' で feedId が null のときはクエリを無効化し fetch を呼ばないこと", () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSearchPage1,
    });

    const { result } = renderHook(() => useItemSearch("rust", "feed", null), {
      wrapper: createWrapper(),
    });

    expect(result.current.isFetching).toBe(false);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("has_more が true のとき fetchNextPage で次ページが取得できること", async () => {
    let callCount = 0;
    mockFetch.mockImplementation(() => {
      callCount += 1;
      const page = callCount === 1 ? mockSearchPage1 : mockSearchPage2;
      return Promise.resolve({ ok: true, json: async () => page });
    });

    const { result } = renderHook(
      () => useItemSearch("typescript", "global", null),
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.hasNextPage).toBe(true);

    await act(async () => {
      await result.current.fetchNextPage();
    });

    await waitFor(() => {
      const allItems =
        result.current.data?.pages.flatMap((p) => p.items) ?? [];
      expect(allItems).toHaveLength(3);
    });

    // 次ページ取得時に cursor が付与される
    const secondCallUrl = mockFetch.mock.calls[1][0] as string;
    expect(secondCallUrl).toContain(
      `cursor=${encodeURIComponent("2026-02-26T10:00:00Z|item-2")}`
    );

    // 次ページで has_more=false / next_cursor=null になり hasNextPage が false
    expect(result.current.hasNextPage).toBe(false);
  });

  it("has_more が true でも next_cursor が空文字のときは次ページなしと扱うこと", async () => {
    // impl-notes Task 4: 末尾項目の published_at がゼロ値のとき NextCursor は空文字となる
    const pageWithEmptyCursor: ItemSearchResponse = {
      items: mockSearchPage1.items,
      next_cursor: "",
      has_more: true,
    };
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => pageWithEmptyCursor,
    });

    const { result } = renderHook(
      () => useItemSearch("typescript", "global", null),
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // next_cursor が空文字なら hasNextPage は false
    expect(result.current.hasNextPage).toBe(false);
  });

  it("API エラー時はエラー状態になること（Req 4.5）", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({ message: "Internal Server Error" }),
    });

    const { result } = renderHook(
      () => useItemSearch("typescript", "global", null),
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });

  it("URL エンコードが必要な文字を含むクエリでも正しくエンコードされること", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSearchPage1,
    });

    const { result } = renderHook(
      () => useItemSearch("hello world & special", "global", null),
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const calledUrl = mockFetch.mock.calls[0][0] as string;
    // スペースは + または %20、& は %26 にエンコードされる
    expect(calledUrl).toMatch(/q=hello(\+|%20)world(\+|%20)%26(\+|%20)special/);
  });
});
