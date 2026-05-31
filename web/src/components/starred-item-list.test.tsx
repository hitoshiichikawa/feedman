import { render, screen, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect } from "react";
import { StarredItemList } from "./starred-item-list";
import {
  AppStateProvider,
  useAppDispatch,
  useAppState,
} from "@/contexts/app-state";
import type { StarredItemListResponse } from "@/types/item";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserverのモックを差し替えて、observe された callback を保持し
// テストから手動で「visible になった」ことをシミュレートできるようにする。
// new で呼ばれる前提のため、class として実装する（vi.fn().mockImplementation だと
// "is not a constructor" になるため）。
type ObserverCallback = (entries: IntersectionObserverEntry[]) => void;
const observedCallbacks: ObserverCallback[] = [];
class MockIntersectionObserver {
  private callback: ObserverCallback;
  constructor(callback: ObserverCallback) {
    this.callback = callback;
    observedCallbacks.push(callback);
  }
  observe() {}
  unobserve() {}
  disconnect() {
    const idx = observedCallbacks.indexOf(this.callback);
    if (idx >= 0) observedCallbacks.splice(idx, 1);
  }
  takeRecords() {
    return [];
  }
}
global.IntersectionObserver =
  MockIntersectionObserver as unknown as typeof IntersectionObserver;

/**
 * dispatch / state を外部から観測するための probe コンポーネント。
 * AppStateProvider 配下に StarredItemList と兄弟として配置する。
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

/** 横断スター記事一覧の標準レスポンス（複数フィードにまたがる） */
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
      title: "Feed B のスター記事",
      link: "https://example.com/b/article-2",
      summary: "Feed B のスター記事の概要",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: true,
      hatebu_count: 5,
      hatebu_fetched_at: null,
    },
  ],
  next_cursor: "2026-02-26T10:00:00Z",
  has_more: true,
};

/** 2 ページ目（末尾） */
const mockStarredPage2: StarredItemListResponse = {
  items: [
    {
      id: "item-3",
      feed_id: "feed-a",
      feed_title: "Feed A",
      title: "Feed A のもっと古いスター記事",
      link: "https://example.com/a/article-3",
      summary: "",
      published_at: "2026-02-25T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: true,
      hatebu_count: 0,
      hatebu_fetched_at: null,
    },
  ],
  next_cursor: null,
  has_more: false,
};

const emptyResponse: StarredItemListResponse = {
  items: [],
  next_cursor: null,
  has_more: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  observedCallbacks.length = 0;
});

describe("StarredItemList コンポーネント", () => {
  it("ヘッダにコンテキストタイトル「お気に入り」を表示すること（Req 2.1）", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve({ ok: true, json: async () => emptyResponse })
    );

    render(<StarredItemList />, { wrapper: createWrapper() });

    expect(screen.getByTestId("starred-item-list-title")).toHaveTextContent(
      "お気に入り"
    );
  });

  it("記事 0 件のときに空状態「記事がありません」を表示すること（Req 2.6）", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({ ok: true, json: async () => emptyResponse });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<StarredItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("starred-item-list-empty")).toBeInTheDocument();
    });
    expect(screen.getByText("記事がありません")).toBeInTheDocument();
    // エラーメッセージが同時に出ないこと（空状態とエラー状態の区別 / Req 2.7）
    expect(
      screen.queryByText("記事の読み込みに失敗しました")
    ).not.toBeInTheDocument();
  });

  it("API 取得に失敗したときエラー状態「記事の読み込みに失敗しました」を表示すること（Req 2.7）", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ message: "Internal Server Error" }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<StarredItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("starred-item-list-error")).toBeInTheDocument();
    });
    expect(screen.getByText("記事の読み込みに失敗しました")).toBeInTheDocument();
    // 空状態メッセージが同時に出ないこと（区別 / Req 2.7）
    expect(screen.queryByText("記事がありません")).not.toBeInTheDocument();
  });

  it("複数フィードのスター記事と各行の feed_title を併記して表示すること（Req 2.3 / 2.4）", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<StarredItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Feed A の最新スター記事")).toBeInTheDocument();
    });
    expect(screen.getByText("Feed B のスター記事")).toBeInTheDocument();

    // 各行に feed_title が併記される
    const feedTitleA = screen.getByTestId("starred-item-feed-title-item-1");
    const feedTitleB = screen.getByTestId("starred-item-feed-title-item-2");
    expect(feedTitleA).toHaveTextContent("Feed A");
    expect(feedTitleB).toHaveTextContent("Feed B");
    // 薄い文字色 / 小さめのテキスト（Req 2.4: 薄い文字色で 1 行）
    expect(feedTitleA.className).toContain("text-muted-foreground");
    expect(feedTitleA.className).toContain("text-xs");
  });

  it("sentinel が visible になったとき次ページを fetch し cursor を付与すること（Req 2.5）", async () => {
    let callCount = 0;
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        callCount++;
        const page = callCount === 1 ? mockStarredPage1 : mockStarredPage2;
        return Promise.resolve({ ok: true, json: async () => page });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<StarredItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Feed A の最新スター記事")).toBeInTheDocument();
    });

    // Intersection Observer の登録が行われ、callback が観測可能になっている
    expect(observedCallbacks.length).toBeGreaterThan(0);

    // sentinel が visible になったことをシミュレート
    await act(async () => {
      observedCallbacks.forEach((cb) =>
        cb([
          {
            isIntersecting: true,
            target: document.createElement("div"),
          } as unknown as IntersectionObserverEntry,
        ])
      );
    });

    await waitFor(() => {
      expect(screen.getByText("Feed A のもっと古いスター記事")).toBeInTheDocument();
    });

    // 2 回目の fetch に cursor が含まれていること（生 or encoded どちらでも許容）
    const secondCallUrl = mockFetch.mock.calls[1][0] as string;
    expect(
      secondCallUrl.includes("cursor=2026-02-26T10:00:00Z") ||
        secondCallUrl.includes("cursor=2026-02-26T10%3A00%3A00Z")
    ).toBe(true);
  });

  // --- Issue #154 / Task 5: 横断スター一覧での ItemMetaActions 配線 ---
  //
  // (a) item-hatebu-count + item-star-toggle が各行右端に出現する
  // (b) スター⭐️トグルクリックで mutation が呼ばれ、expandedItemId が変化しない（伝播抑止）
  // (c) is_starred=true/false の見た目分岐（既存 page1 は両方 true のため null 補助は別記事行で確認）
  // (d) 既存無限スクロール / 既読薄表示 / feed_title 併記の非回帰は既存テスト群で担保
  it("横断スター一覧の各行右端に ItemMetaActions（item-hatebu-count + item-star-toggle）が出現し、既存 star-${id} が撤去されること（Req 1.1 / 1.2 / 4.2 / NFR 3.2）", async () => {
    // Arrange / Act
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<StarredItemList />, { wrapper: createWrapper() });

    // Assert
    await waitFor(() => {
      expect(screen.getByText("Feed A の最新スター記事")).toBeInTheDocument();
    });

    expect(
      screen.getByTestId("item-hatebu-count-item-1")
    ).toBeInTheDocument();
    expect(screen.getByTestId("item-star-toggle-item-1")).toBeInTheDocument();
    expect(
      screen.getByTestId("item-hatebu-count-item-2")
    ).toBeInTheDocument();
    expect(screen.getByTestId("item-star-toggle-item-2")).toBeInTheDocument();
    // 既存読み取り専用 Star testid（star-${id}）は撤去された
    expect(screen.queryByTestId("star-item-1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("star-item-2")).not.toBeInTheDocument();
  });

  it("hatebu_fetched_at の値に応じて `-` / 整数値を切り替えて表示すること（Req 1.3 / 1.4）", async () => {
    // Arrange / Act
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<StarredItemList />, { wrapper: createWrapper() });

    // Assert: item-1 は取得済み hatebu_count=10 → "10"、item-2 は未取得 → "-"
    await waitFor(() => {
      expect(
        screen.getByTestId("item-hatebu-count-item-1")
      ).toBeInTheDocument();
    });
    expect(screen.getByTestId("item-hatebu-count-item-1")).toHaveTextContent(
      "10"
    );
    expect(screen.getByTestId("item-hatebu-count-item-2")).toHaveTextContent(
      "-"
    );
  });

  it("一覧上のスター⭐️トグルクリックで mutation が呼ばれ、expandedItemId が変化しないこと（Req 2.1 / 2.3 / NFR 2.1）", async () => {
    // Arrange
    const user = userEvent.setup();
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    let latestState: ReturnType<typeof useAppState> | null = null;
    render(
      <>
        <StarredItemList />
        <StateProbe
          onReady={(_d, state) => {
            latestState = state;
          }}
        />
      </>,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByTestId("item-star-toggle-item-1")).toBeInTheDocument();
    });

    // Initial state: expandedItemId is null
    expect(latestState?.expandedItemId).toBeNull();

    // Act: click star toggle (item-1 is currently is_starred=true so PUT is_starred=false)
    await user.click(screen.getByTestId("item-star-toggle-item-1"));

    // Assert: PUT /api/items/item-1/state が is_starred=false で発火
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/items/item-1/state",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ is_starred: false }),
        })
      );
    });
    // expandedItemId は null のまま（行クリック展開へ伝播していない）
    expect(latestState?.expandedItemId).toBeNull();
  });

  it("is_starred の状態に応じて aria-pressed / aria-label が切り替わること（Req 1.5 / 1.6 / NFR 1.1 / 1.2）", async () => {
    // Arrange: item-1 / item-2 は両方 is_starred=true、追加でカスタムレスポンスで 1 件 false を返す
    const mixedResponse: StarredItemListResponse = {
      items: [
        {
          ...mockStarredPage1.items[0],
          id: "item-mixed-true",
          is_starred: true,
        },
        {
          ...mockStarredPage1.items[0],
          id: "item-mixed-false",
          is_starred: false,
        },
      ],
      next_cursor: null,
      has_more: false,
    };
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({ ok: true, json: async () => mixedResponse });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    // Act
    render(<StarredItemList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(
        screen.getByTestId("item-star-toggle-item-mixed-true")
      ).toBeInTheDocument();
    });

    // Assert
    const toggleTrue = screen.getByTestId("item-star-toggle-item-mixed-true");
    const toggleFalse = screen.getByTestId("item-star-toggle-item-mixed-false");
    expect(toggleTrue).toHaveAttribute("aria-pressed", "true");
    expect(toggleTrue).toHaveAttribute("aria-label", "スターを解除する");
    expect(toggleFalse).toHaveAttribute("aria-pressed", "false");
    expect(toggleFalse).toHaveAttribute("aria-label", "スターを付ける");
  });

  it("記事行クリックで EXPAND_ITEM が dispatch されて expandedItemId が更新されること（Req 2.8 / 3.1）", async () => {
    const user = userEvent.setup();
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.startsWith("/api/feeds/starred/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockStarredPage1,
        });
      }
      // 詳細取得 mock（クリック後に走るが本テストでは内容を検証しない）
      if (typeof url === "string" && url.startsWith("/api/items/")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "item-1",
            feed_id: "feed-a",
            title: "Feed A の最新スター記事",
            link: "https://example.com/a/article-1",
            summary: "",
            published_at: "2026-02-27T10:00:00Z",
            is_date_estimated: false,
            is_read: false,
            is_starred: true,
            hatebu_count: 10,
            hatebu_fetched_at: "2026-02-27T09:00:00Z",
            content: "<p>本文</p>",
            author: "",
          }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    let latestState: ReturnType<typeof useAppState> | null = null;
    render(
      <>
        <StarredItemList />
        <StateProbe
          onReady={(_d, state) => {
            latestState = state;
          }}
        />
      </>,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("Feed A の最新スター記事")).toBeInTheDocument();
    });

    // 初期状態では expandedItemId は null
    expect(latestState?.expandedItemId).toBeNull();

    // 記事行クリック
    await user.click(screen.getByTestId("item-row-item-1"));

    // EXPAND_ITEM dispatch が走り state.expandedItemId が更新される
    await waitFor(() => {
      expect(latestState?.expandedItemId).toBe("item-1");
    });
  });
});
