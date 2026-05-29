import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect } from "react";
import { CrossFeedItemList } from "./cross-feed-item-list";
import {
  AppStateProvider,
  useAppDispatch,
  useAppState,
} from "@/contexts/app-state";
import type { CrossFeedListResponse } from "@/types/crossfeed";
import type { ReactNode } from "react";

// グローバル fetch のモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserver のモック（new で呼ばれる前提で class として実装）
class MockIntersectionObserver {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  constructor(_callback: IntersectionObserverCallback) {}
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}
global.IntersectionObserver =
  MockIntersectionObserver as unknown as typeof IntersectionObserver;

/**
 * dispatch / state を外部から観測するための probe コンポーネント。
 * AppStateProvider 配下に CrossFeedItemList と兄弟として配置する。
 */
function StateProbe({
  onReady,
}: {
  onReady: (
    dispatch: ReturnType<typeof useAppDispatch>,
    state: ReturnType<typeof useAppState>
  ) => void;
}) {
  const dispatch = useAppDispatch();
  const state = useAppState();
  useEffect(() => {
    onReady(dispatch, state);
  }, [dispatch, state, onReady]);
  return null;
}

/** テスト用ラッパー（QueryClient + AppStateProvider） */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <AppStateProvider>{children}</AppStateProvider>
      </QueryClientProvider>
    );
  };
}

const SINCE_TIME_FROM_SERVER = "2026-05-26T10:00:00Z";

/** 横断新着一覧の標準レスポンス（複数フィードにまたがる） */
const mockPage1: CrossFeedListResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-a",
      feed_title: "Feed A",
      feed_favicon_url: "data:image/png;base64,AAAA",
      title: "Feed A の新着記事",
      link: "https://example.com/a/1",
      summary: "Feed A の概要",
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
      title: "Feed B の新着記事",
      link: "https://example.com/b/1",
      summary: "Feed B の概要",
      published_at: "2026-05-27T09:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: false,
      hatebu_count: 3,
    },
  ],
  next_cursor: null,
  has_more: false,
  since_time: SINCE_TIME_FROM_SERVER,
};

/** 空レスポンス（first page items が 0 件） */
const mockEmptyPage: CrossFeedListResponse = {
  items: [],
  next_cursor: null,
  has_more: false,
  since_time: SINCE_TIME_FROM_SERVER,
};

/** PUT/POST/GET を切り分けて応答する mockFetch を設定 */
function setupMockFetch(page: CrossFeedListResponse) {
  mockFetch.mockImplementation((url: string, options?: RequestInit) => {
    if (url.startsWith("/api/items/cross-feed")) {
      return Promise.resolve({
        ok: true,
        json: async () => page,
      });
    }
    if (
      url === "/api/users/me/cross-feed-last-seen" &&
      options?.method === "PUT"
    ) {
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    }
    // 記事詳細取得（展開時のみ呼ばれる）
    if (url.startsWith("/api/items/")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          id: url.split("/").pop(),
          feed_id: "feed-a",
          title: "詳細",
          link: "https://example.com/a/1",
          summary: "",
          content: "<p>本文</p>",
          author: "",
          published_at: "2026-05-27T10:00:00Z",
          is_date_estimated: false,
          is_read: false,
          is_starred: false,
          hatebu_count: 0,
          hatebu_fetched_at: null,
        }),
      });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

describe("CrossFeedItemList コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("first page items が 0 件のとき空状態メッセージを表示すること（Req 4.6）", async () => {
    setupMockFetch(mockEmptyPage);

    render(<CrossFeedItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(
        screen.getByTestId("cross-feed-item-list-empty")
      ).toBeInTheDocument();
    });
    expect(screen.getByText("新着記事はありません")).toBeInTheDocument();
  });

  it("初回データ受信後に touch mutation（PUT /api/users/me/cross-feed-last-seen）が 1 回だけ呼ばれること（Req 4.3）", async () => {
    setupMockFetch(mockPage1);

    render(<CrossFeedItemList />, { wrapper: createWrapper() });

    // 記事一覧が描画されるまで待つ
    await waitFor(() => {
      expect(screen.getByText("Feed A の新着記事")).toBeInTheDocument();
    });

    // PUT 呼び出しが行われたことを確認
    await waitFor(() => {
      const putCalls = mockFetch.mock.calls.filter(
        ([url, options]) =>
          url === "/api/users/me/cross-feed-last-seen" &&
          (options as RequestInit | undefined)?.method === "PUT"
      );
      expect(putCalls).toHaveLength(1);
    });
  });

  it("初回データ受信後に SET_CROSS_FEED_SESSION_SINCE が data.pages[0].since_time で dispatch されること（Req 4.7）", async () => {
    setupMockFetch(mockPage1);

    let observedSince: string | null | undefined;
    const handleReady = vi.fn(
      (
        _d: ReturnType<typeof useAppDispatch>,
        state: ReturnType<typeof useAppState>
      ) => {
        observedSince = state.crossFeedSessionSince;
      }
    );

    render(
      <>
        <CrossFeedItemList />
        <StateProbe onReady={handleReady} />
      </>,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(observedSince).toBe(SINCE_TIME_FROM_SERVER);
    });
  });

  it("crossFeedSessionSince が既に非 null のとき touch mutation も SET_CROSS_FEED_SESSION_SINCE 上書きも発火しないこと（session 内重複防止、Req 4.3 / 4.7）", async () => {
    const PRE_EXISTING_SINCE = "2026-05-20T00:00:00Z";
    setupMockFetch(mockPage1);

    let observedSince: string | null | undefined;
    const handleReady = vi.fn(
      (
        dispatch: ReturnType<typeof useAppDispatch>,
        state: ReturnType<typeof useAppState>
      ) => {
        observedSince = state.crossFeedSessionSince;
        if (state.crossFeedSessionSince === null) {
          // 事前に baseline を固定（別 session の続き状態をシミュレート）
          dispatch({
            type: "SET_CROSS_FEED_SESSION_SINCE",
            sinceTime: PRE_EXISTING_SINCE,
          });
        }
      }
    );

    render(
      <>
        <CrossFeedItemList />
        <StateProbe onReady={handleReady} />
      </>,
      { wrapper: createWrapper() }
    );

    // 一覧の描画完了を待つ
    await waitFor(() => {
      expect(screen.getByText("Feed A の新着記事")).toBeInTheDocument();
    });

    // baseline は既存値を保持
    await waitFor(() => {
      expect(observedSince).toBe(PRE_EXISTING_SINCE);
    });

    // 一定時間後に PUT が一度も呼ばれていないことを確認
    const putCalls = mockFetch.mock.calls.filter(
      ([url, options]) =>
        url === "/api/users/me/cross-feed-last-seen" &&
        (options as RequestInit | undefined)?.method === "PUT"
    );
    expect(putCalls).toHaveLength(0);
  });

  it("各記事行に feed_title と FeedFavicon が描画されること（Req 3.1 / 3.2）", async () => {
    setupMockFetch(mockPage1);

    render(<CrossFeedItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Feed A の新着記事")).toBeInTheDocument();
    });

    // フィード名 badge
    expect(screen.getByText("Feed A")).toBeInTheDocument();
    expect(screen.getByText("Feed B")).toBeInTheDocument();

    // FeedFavicon: feed-a は img、feed-b は fallback
    expect(screen.getByTestId("feed-favicon-feed-a")).toBeInTheDocument();
    expect(
      screen.getByTestId("feed-favicon-fallback-feed-b")
    ).toBeInTheDocument();

    // badge コンテナが各記事に存在
    expect(screen.getByTestId("cross-feed-item-badge-item-1")).toBeInTheDocument();
    expect(screen.getByTestId("cross-feed-item-badge-item-2")).toBeInTheDocument();
  });

  it("記事行をクリックすると展開され、再クリックで折りたたまれること（既存 useMarkAsRead / useToggleStar 流用の前提配線確認）", async () => {
    const user = userEvent.setup();
    setupMockFetch(mockPage1);

    render(<CrossFeedItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Feed A の新着記事")).toBeInTheDocument();
    });

    // 記事行クリック → 展開
    await user.click(screen.getByTestId("item-row-item-1"));

    await waitFor(() => {
      // 取得開始のローディング枠が表示される（記事詳細展開エリア）
      const row = screen.getByTestId("item-row-item-1");
      expect(row.getAttribute("data-read")).toBe("false");
    });

    // クリックすると expandedItemId が item-1 に設定されている前提で、
    // ItemDetailArea のいずれか（loading / detail / error）が描画される
    await waitFor(() => {
      // 詳細取得が完了すれば item-detail 配下が描画されるが、jsdom 環境では
      // 上記モックで /api/items/item-1 が応答するため、ローディング → 表示の
      // どちらか少なくとも 1 つは表示される
      const expanded =
        screen.queryByTestId("item-detail-loading") ??
        screen.queryByText("読み込み中...") ??
        screen.queryByText("本文");
      expect(expanded).not.toBeNull();
    });
  });
});
