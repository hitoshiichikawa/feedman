import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CSRFProvider } from "@/lib/csrf";
import { ItemDetail } from "./item-detail";
import type { ItemDetail as ItemDetailType } from "@/types/item";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

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
        <CSRFProvider>{children}</CSRFProvider>
      </QueryClientProvider>
    );
  };
}

/** テスト用の記事詳細データ */
const mockItem: ItemDetailType = {
  id: "item-1",
  feed_id: "feed-1",
  title: "テスト記事のタイトル",
  link: "https://example.com/article-1",
  published_at: "2026-02-27T10:00:00Z",
  is_date_estimated: false,
  is_read: true,
  is_starred: false,
  hatebu_count: 42,
  hatebu_fetched_at: "2026-02-27T09:00:00Z",
  content: "<p>これはテスト記事の<strong>本文</strong>です。</p>",
  summary: "テスト記事のサマリー",
  author: "テスト著者",
};

/** はてブ未取得の記事データ */
const mockItemNoHatebu: ItemDetailType = {
  ...mockItem,
  id: "item-2",
  hatebu_count: 0,
  hatebu_fetched_at: null,
};

/**
 * mockFetchの設定ヘルパー
 */
function setupMockFetch() {
  mockFetch.mockImplementation((url: string) => {
    if (url === "/api/csrf-token") {
      return Promise.resolve({
        ok: true,
        json: async () => ({ token: "test-csrf-token" }),
      });
    }
    if (typeof url === "string" && url.includes("/api/items/") && url.includes("/state")) {
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
}

describe("ItemDetail コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("サニタイズ済みHTMLコンテンツが展開表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // コンテンツ表示エリアが存在すること
    const contentArea = screen.getByTestId("item-content");
    expect(contentArea).toBeInTheDocument();
    // HTMLコンテンツが表示されていること（dangerouslySetInnerHTMLなのでテキストで確認）
    expect(contentArea.innerHTML).toContain("これはテスト記事の");
    expect(contentArea.innerHTML).toContain("<strong>本文</strong>");
  });

  it("元記事URLへの遷移ボタンが表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const linkButton = screen.getByTestId("original-link");
    expect(linkButton).toBeInTheDocument();
    expect(linkButton).toHaveAttribute("href", "https://example.com/article-1");
    expect(linkButton).toHaveAttribute("target", "_blank");
    expect(linkButton).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("はてなブックマーク数が表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const hatebuCount = screen.getByTestId("hatebu-count");
    expect(hatebuCount).toBeInTheDocument();
    expect(hatebuCount).toHaveTextContent("42");
  });

  it("はてブ未取得時は「-」が表示されること", () => {
    render(
      <ItemDetail
        item={mockItemNoHatebu}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const hatebuCount = screen.getByTestId("hatebu-count");
    expect(hatebuCount).toHaveTextContent("-");
  });

  it("スター切替ボタンをクリックするとonToggleStarが呼ばれること", async () => {
    const user = userEvent.setup();
    const onToggleStar = vi.fn();

    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={onToggleStar}
      />,
      { wrapper: createWrapper() }
    );

    const starButton = screen.getByTestId("star-toggle");
    await user.click(starButton);

    // 現在is_starred=falseなので、trueに切り替えるリクエストが呼ばれること
    expect(onToggleStar).toHaveBeenCalledWith("item-1", true);
  });

  it("スター付き記事のスターボタンをクリックするとfalseで呼ばれること", async () => {
    const user = userEvent.setup();
    const onToggleStar = vi.fn();
    const starredItem = { ...mockItem, is_starred: true };

    render(
      <ItemDetail
        item={starredItem}
        onMarkAsRead={() => {}}
        onToggleStar={onToggleStar}
      />,
      { wrapper: createWrapper() }
    );

    const starButton = screen.getByTestId("star-toggle");
    await user.click(starButton);

    // 現在is_starred=trueなので、falseに切り替えるリクエストが呼ばれること
    expect(onToggleStar).toHaveBeenCalledWith("item-1", false);
  });

  it("展開時にonMarkAsReadが呼ばれること", () => {
    const onMarkAsRead = vi.fn();

    render(
      <ItemDetail
        item={{ ...mockItem, is_read: false }}
        onMarkAsRead={onMarkAsRead}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // コンポーネントのマウント時（展開時）にonMarkAsReadが呼ばれること
    expect(onMarkAsRead).toHaveBeenCalledWith("item-1");
  });

  it("既読記事ではonMarkAsReadが呼ばれないこと", () => {
    const onMarkAsRead = vi.fn();

    render(
      <ItemDetail
        item={mockItem} // is_read: true
        onMarkAsRead={onMarkAsRead}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // 既に既読なのでonMarkAsReadは呼ばれない
    expect(onMarkAsRead).not.toHaveBeenCalled();
  });

  it("記事タイトルが表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("テスト記事のタイトル")).toBeInTheDocument();
  });

  it("著者名が表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("テスト著者")).toBeInTheDocument();
  });
});
