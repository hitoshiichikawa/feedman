import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import {
  useCrossFeedItems,
  useTouchCrossFeedLastSeen,
} from "./use-cross-feed-items";
import type { CrossFeedListResponse } from "@/types/crossfeed";

// グローバル fetch のモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// AppStateContext の useAppState をモックして crossFeedSessionSince を制御する。
// `useAppState` は AppStateProvider に依存するため、テスト用にモック差し替えする。
const mockUseAppState = vi.fn();
vi.mock("@/contexts/app-state", () => ({
  useAppState: () => mockUseAppState(),
}));

/** テスト用の QueryClientProvider ラッパー */
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

/** モック AppState の最小フィールド設定ヘルパー */
function setAppStateSince(since: string | null) {
  mockUseAppState.mockReturnValue({ crossFeedSessionSince: since });
}

/** mockFetch のルーティング設定ヘルパー */
function setupMockFetch(
  apiHandler: (url: string, options?: RequestInit) => Promise<unknown>
) {
  mockFetch.mockImplementation(apiHandler);
}

/** テスト用の CrossFeedListResponse（1 ページ目、続きあり） */
const mockPage1: CrossFeedListResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-a",
      feed_title: "Feed A",
      feed_favicon_url: "data:image/png;base64,AAAA",
      title: "Feed A の新着記事 1",
      link: "https://example.com/a/1",
      summary: "概要 1",
      published_at: "2026-05-27T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: false,
      hatebu_count: 0,
    },
    {
      id: "item-2",
      feed_id: "feed-b",
      feed_title: "Feed B",
      feed_favicon_url: null,
      title: "Feed B の新着記事 1",
      link: "https://example.com/b/1",
      summary: "",
      published_at: "2026-05-27T09:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: false,
      hatebu_count: 3,
    },
  ],
  next_cursor: "2026-05-27T09:00:00Z:item-2",
  has_more: true,
  since_time: "2026-05-26T10:00:00Z",
};

describe("useCrossFeedItems", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("crossFeedSessionSince が null のとき URL に since を含まず /api/items/cross-feed?limit=50 を呼ぶこと", async () => {
    setAppStateSince(null);
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/items/cross-feed")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const { result } = renderHook(() => useCrossFeedItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const firstCallUrl = mockFetch.mock.calls[0][0] as string;
    expect(firstCallUrl).toContain("/api/items/cross-feed?limit=50");
    expect(firstCallUrl).not.toContain("since=");
    expect(firstCallUrl).not.toContain("cursor=");
  });

  it("crossFeedSessionSince が非 null のとき URL に &since=<encoded> が含まれること（Req 4.7）", async () => {
    const since = "2026-05-26T10:00:00Z";
    setAppStateSince(since);
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/items/cross-feed")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const { result } = renderHook(() => useCrossFeedItems(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const firstCallUrl = mockFetch.mock.calls[0][0] as string;
    // encodeURIComponent は ':' を '%3A' にエンコードする
    expect(firstCallUrl).toContain(`&since=${encodeURIComponent(since)}`);
    expect(firstCallUrl).toContain("/api/items/cross-feed?limit=50");
  });

  it("crossFeedSessionSince が変化したとき queryKey も変化し refetch が発火すること", async () => {
    setAppStateSince(null);
    setupMockFetch((url: string) => {
      if (url.startsWith("/api/items/cross-feed")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const Wrapper = createWrapper();
    const { result, rerender } = renderHook(() => useCrossFeedItems(), {
      wrapper: Wrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const callCountBefore = mockFetch.mock.calls.length;

    // crossFeedSessionSince を変化させて rerender
    setAppStateSince("2026-05-26T10:00:00Z");
    rerender();

    // 新しい queryKey のため再 fetch が発火する
    await waitFor(() => {
      expect(mockFetch.mock.calls.length).toBeGreaterThan(callCountBefore);
    });

    const newCallUrl = mockFetch.mock.calls[mockFetch.mock.calls.length - 1][0] as string;
    expect(newCallUrl).toContain("since=");
  });
});

describe("useTouchCrossFeedLastSeen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("mutate 実行で PUT /api/users/me/cross-feed-last-seen を送ること（Req 4.3）", async () => {
    setAppStateSince(null);
    setupMockFetch((url: string, options?: RequestInit) => {
      if (
        url.includes("/api/users/me/cross-feed-last-seen") &&
        options?.method === "PUT"
      ) {
        return Promise.resolve({
          ok: true,
          json: async () => ({}),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const { result } = renderHook(() => useTouchCrossFeedLastSeen(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate();
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/users/me/cross-feed-last-seen",
      expect.objectContaining({ method: "PUT" })
    );
  });
});
