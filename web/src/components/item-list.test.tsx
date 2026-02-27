import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ItemList } from "./item-list";
import type { ItemListResponse } from "@/types/item";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserverのモック
const mockIntersectionObserver = vi.fn();
mockIntersectionObserver.mockImplementation((callback: IntersectionObserverCallback) => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
global.IntersectionObserver = mockIntersectionObserver;

/** テスト用ラッパー */
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
        {children}
      </QueryClientProvider>
    );
  };
}

/** テスト用の記事一覧レスポンス */
const mockItemsResponse: ItemListResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-1",
      title: "最新の記事タイトル",
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
      title: "推定日付の記事",
      link: "https://example.com/article-2",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: true,
      is_read: true,
      is_starred: true,
      hatebu_count: 0,
      hatebu_fetched_at: null,
    },
  ],
  next_cursor: null,
  has_more: false,
};

/**
 * mockFetchの設定ヘルパー
 */
function setupMockFetch(response: ItemListResponse = mockItemsResponse) {
  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url.includes("/api/feeds/feed-1/items")) {
      return Promise.resolve({
        ok: true,
        json: async () => response,
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({}),
    });
  });
}

describe("ItemList コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("feedIdが指定されていない場合は案内メッセージを表示すること", () => {
    render(
      <ItemList
        feedId={null}
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("フィードを選択してください")).toBeInTheDocument();
  });

  it("記事一覧がpublished_at降順で表示されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    expect(screen.getByText("推定日付の記事")).toBeInTheDocument();

    // 順序確認: 最新の記事が先に表示される
    const items = screen.getAllByTestId(/^item-row-/);
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("最新の記事タイトル")).toBeInTheDocument();
    expect(within(items[1]).getByText("推定日付の記事")).toBeInTheDocument();
  });

  it("推定フラグ付き日付には推定マークが表示されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("推定日付の記事")).toBeInTheDocument();
    });

    // 推定日付の記事に推定マーカーが表示されること
    const estimatedItem = screen.getByTestId("item-row-item-2");
    expect(within(estimatedItem).getByTestId("date-estimated")).toBeInTheDocument();
  });

  it("フィルタ切替UI（全て/未読/スター）が表示されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });

    expect(screen.getByRole("tab", { name: "未読" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "スター" })).toBeInTheDocument();
  });

  it("フィルタを切り替えるとAPIにフィルタパラメータが送信されること", async () => {
    const user = userEvent.setup();

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "未読" })).toBeInTheDocument();
    });

    // 「未読」フィルタをクリック
    await user.click(screen.getByRole("tab", { name: "未読" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("filter=unread"),
        expect.any(Object)
      );
    });
  });

  it("記事をクリックするとonSelectItemが呼ばれること", async () => {
    const user = userEvent.setup();
    const onSelectItem = vi.fn();

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={onSelectItem}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("item-row-item-1"));

    expect(onSelectItem).toHaveBeenCalledWith("item-1");
  });

  it("既読記事は視覚的に区別されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByTestId("item-row-item-2")).toBeInTheDocument();
    });

    // 既読記事にはdata-read属性がtrueであること
    const readItem = screen.getByTestId("item-row-item-2");
    expect(readItem).toHaveAttribute("data-read", "true");

    // 未読記事にはdata-read属性がfalseであること
    const unreadItem = screen.getByTestId("item-row-item-1");
    expect(unreadItem).toHaveAttribute("data-read", "false");
  });

  it("無限スクロール用のsentinelが存在すること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    // IntersectionObserver用のsentinelが存在すること
    expect(screen.getByTestId("scroll-sentinel")).toBeInTheDocument();
  });

  it("記事が0件の場合に空の状態を表示すること", async () => {
    setupMockFetch({
      items: [],
      next_cursor: null,
      has_more: false,
    });

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("記事がありません")).toBeInTheDocument();
    });
  });
});
