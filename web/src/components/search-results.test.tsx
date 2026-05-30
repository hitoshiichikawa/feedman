import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { SearchResults } from "./search-results";
import {
  AppStateProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from "@/contexts/app-state";
import type { ItemSearchResponse } from "@/types/item";

// グローバル fetch のモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserver のモック（無限スクロール sentinel 用）
const mockIntersectionObserver = vi.fn();
mockIntersectionObserver.mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
global.IntersectionObserver = mockIntersectionObserver;

/**
 * テスト用のラッパー: QueryClient + AppStateProvider を結合し、
 * 初期 dispatch（検索状態のセットアップ）を 1 度だけ実行してから子をレンダする。
 */
function renderWithProviders(
  ui: ReactNode,
  initialDispatch?: (dispatch: (a: AppAction) => void) => void
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  let observed: ReturnType<typeof useAppState> | null = null;
  function Probe() {
    const dispatch = useAppDispatch();
    observed = useAppState();
    const ready = useDispatchOnce(() => {
      if (initialDispatch) initialDispatch(dispatch);
    });
    return ready ? <>{ui}</> : null;
  }
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <AppStateProvider>
        <Probe />
      </AppStateProvider>
    </QueryClientProvider>
  );
  return { ...utils, getState: () => observed! };
}

function useDispatchOnce(fn: () => void): boolean {
  const [done, setDone] = useState(false);
  useEffect(() => {
    fn();
    setDone(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return done;
}

/** テスト用検索結果（横断検索想定: 2 フィードから 2 件） */
const mockGlobalResults: ItemSearchResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-a",
      title: "TypeScript の型推論",
      link: "https://example.com/article-1",
      summary: "型推論の仕組みを解説",
      published_at: "2026-02-27T10:00:00Z",
      is_date_estimated: false,
      hatebu_count: 12,
      hatebu_fetched_at: null,
      feed_title: "Frontend Weekly",
      favicon_url: "data:image/png;base64,AAA",
      is_read: false,
      is_starred: false,
    },
    {
      id: "item-2",
      feed_id: "feed-b",
      title: "TypeScript 5 の新機能",
      link: "https://example.com/article-2",
      summary: "",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: true,
      hatebu_count: 3,
      hatebu_fetched_at: null,
      feed_title: "TS Newsletter",
      favicon_url: null,
      is_read: true,
      is_starred: true,
    },
  ],
  next_cursor: null,
  has_more: false,
};

/** テスト用検索結果（フィード内検索想定: 同一 feed のみ） */
const mockFeedResults: ItemSearchResponse = {
  items: [
    {
      id: "item-10",
      feed_id: "feed-target",
      title: "Kubernetes 入門",
      link: "https://example.com/article-10",
      summary: "K8s のアーキテクチャ概要",
      published_at: "2026-02-27T10:00:00Z",
      is_date_estimated: false,
      hatebu_count: 5,
      hatebu_fetched_at: null,
      feed_title: "DevOps Blog",
      favicon_url: "data:image/png;base64,XYZ",
      is_read: false,
      is_starred: false,
    },
  ],
  next_cursor: null,
  has_more: false,
};

/** mockFetch を検索 API の結果で応答するようセットアップする */
function setupSearchFetch(response: ItemSearchResponse) {
  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url.startsWith("/api/items/search")) {
      return Promise.resolve({ ok: true, json: async () => response });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

/** mockFetch を 500 エラーで応答するようセットアップする */
function setupErrorFetch() {
  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url.startsWith("/api/items/search")) {
      return Promise.resolve({
        ok: false,
        status: 500,
        json: async () => ({ message: "Internal Server Error" }),
      });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

describe("SearchResults", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("検索開始直後はローディング表示が即時提示されること（Req 4.4 / NFR 1.1）", () => {
    // fetch を pending のまま返すことで isLoading=true を維持
    mockFetch.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    expect(screen.getByTestId("search-results-loading")).toBeInTheDocument();
  });

  it("API エラー時はエラー表示が提示されること（Req 4.5）", async () => {
    setupErrorFetch();

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-results-error")).toBeInTheDocument();
    });
  });

  it("検索結果が 0 件のとき空状態表示が提示されること（Req 4.3）", async () => {
    setupSearchFetch({ items: [], next_cursor: null, has_more: false });

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "nonexistent",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-results-empty")).toBeInTheDocument();
    });
  });

  it("横断検索（scope='global'）の結果カードに feed_title と favicon バッジが表示されること（Req 4.2）", async () => {
    setupSearchFetch(mockGlobalResults);

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-results")).toBeInTheDocument();
    });

    // 1 件目: favicon あり + feed_title
    const badge1 = screen.getByTestId("search-result-feed-badge-item-1");
    expect(badge1).toBeInTheDocument();
    expect(badge1).toHaveTextContent("Frontend Weekly");
    const favicons = screen.getAllByTestId("search-result-favicon");
    expect(favicons.length).toBeGreaterThanOrEqual(1);
    expect(favicons[0].getAttribute("src")).toBe("data:image/png;base64,AAA");

    // 2 件目: favicon なし（null）でもバッジ自体（feed_title）は表示される
    const badge2 = screen.getByTestId("search-result-feed-badge-item-2");
    expect(badge2).toBeInTheDocument();
    expect(badge2).toHaveTextContent("TS Newsletter");

    // タイトル本文も表示されている
    expect(screen.getByText("TypeScript の型推論")).toBeInTheDocument();
    expect(screen.getByText("TypeScript 5 の新機能")).toBeInTheDocument();
  });

  it("フィード内検索（scope='feed'）の結果カードでは feed_title / favicon バッジが省略されること（Req 4.2 の補集合）", async () => {
    setupSearchFetch(mockFeedResults);

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({ type: "SELECT_FEED", feedId: "feed-target" });
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "kubernetes",
        scope: "feed",
        feedId: "feed-target",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-results")).toBeInTheDocument();
    });

    // フィード内検索ではバッジを描画しない
    expect(
      screen.queryByTestId("search-result-feed-badge-item-10")
    ).toBeNull();
    expect(screen.queryByTestId("search-result-favicon")).toBeNull();

    // タイトル本体は表示されている
    expect(screen.getByText("Kubernetes 入門")).toBeInTheDocument();
  });

  it("検索結果カードをクリックすると EXPAND_ITEM が dispatch され、当該記事が展開されること（Req 4.6）", async () => {
    setupSearchFetch(mockGlobalResults);
    const user = userEvent.setup();

    const { getState } = renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-result-row-item-1")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("search-result-row-item-1"));

    expect(getState().expandedItemId).toBe("item-1");

    // 詳細取得 hook が呼ばれる（fetch URL の確認）。GET /api/items/item-1
    await waitFor(() => {
      const detailCalls = mockFetch.mock.calls.filter(
        ([url]) => typeof url === "string" && url === "/api/items/item-1"
      );
      expect(detailCalls.length).toBeGreaterThanOrEqual(1);
    });
  });

  it("既読記事と未読記事でスタイル状態が data-read 属性で区別されること（Req 4.1 / 4.6 関連）", async () => {
    setupSearchFetch(mockGlobalResults);

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-result-row-item-1")).toBeInTheDocument();
    });

    // item-1 は未読 / item-2 は既読
    expect(
      screen.getByTestId("search-result-row-item-1").getAttribute("data-read")
    ).toBe("false");
    expect(
      screen.getByTestId("search-result-row-item-2").getAttribute("data-read")
    ).toBe("true");

    // item-2 はスター付き
    expect(
      screen.getByTestId("search-result-star-item-2")
    ).toBeInTheDocument();
    // item-1 はスターなし
    expect(
      screen.queryByTestId("search-result-star-item-1")
    ).toBeNull();
  });

  it("推定日付フラグが立っている記事には (推定) ラベルが表示されること", async () => {
    setupSearchFetch(mockGlobalResults);

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("search-results")).toBeInTheDocument();
    });

    // item-2 のみ is_date_estimated=true
    expect(
      screen.getByTestId("search-result-date-estimated-item-2")
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("search-result-date-estimated-item-1")
    ).toBeNull();
  });

  it("published_at が null の記事でも結果カードが描画されること（境界値）", async () => {
    const responseWithNullDate: ItemSearchResponse = {
      items: [
        {
          id: "item-nodate",
          feed_id: "feed-a",
          title: "日付不明の記事",
          link: "https://example.com/article-x",
          summary: "公開日未取得",
          published_at: null,
          is_date_estimated: false,
          hatebu_count: 0,
          hatebu_fetched_at: null,
          feed_title: "Some Feed",
          favicon_url: null,
          is_read: false,
          is_starred: false,
        },
      ],
      next_cursor: null,
      has_more: false,
    };
    setupSearchFetch(responseWithNullDate);

    renderWithProviders(<SearchResults />, (dispatch) => {
      dispatch({
        type: "SET_SEARCH_QUERY",
        query: "anything",
        scope: "global",
      });
    });

    await waitFor(() => {
      expect(
        screen.getByTestId("search-result-row-item-nodate")
      ).toBeInTheDocument();
    });
    // タイトルは表示されている
    expect(screen.getByText("日付不明の記事")).toBeInTheDocument();
  });
});
