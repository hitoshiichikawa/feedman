import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ThemeProvider } from "@/components/theme-provider";
import { AppStateProvider } from "@/contexts/app-state";
import { AppShell } from "./app-shell";
import type { Subscription } from "@/types/feed";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserverのモック
const mockIntersectionObserver = vi.fn();
mockIntersectionObserver.mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
global.IntersectionObserver = mockIntersectionObserver;

/** テスト用の購読データ */
const mockSubscriptions: Subscription[] = [
  {
    id: "sub-1",
    user_id: "user-1",
    feed_id: "feed-1",
    feed_title: "Tech Blog",
    feed_url: "https://example.com/feed.xml",
    favicon_url: "https://example.com/favicon.ico",
    fetch_interval_minutes: 60,
    feed_status: "active",
    unread_count: 5,
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "sub-2",
    user_id: "user-1",
    feed_id: "feed-2",
    feed_title: "News Feed",
    feed_url: "https://news.example.com/rss",
    favicon_url: null,
    fetch_interval_minutes: 30,
    feed_status: "active",
    unread_count: 0,
    created_at: "2026-01-02T00:00:00Z",
  },
];

/** テスト用ラッパー（全Provider含む） */
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
        <ThemeProvider>
          <AppStateProvider>{children}</AppStateProvider>
        </ThemeProvider>
      </QueryClientProvider>
    );
  };
}

/** mockFetchの設定ヘルパー */
function setupMockFetch() {
  mockFetch.mockImplementation((url: string) => {
    if (url === "/api/subscriptions") {
      return Promise.resolve({
        ok: true,
        json: async () => mockSubscriptions,
      });
    }
    if (typeof url === "string" && url.includes("/api/feeds/")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          items: [],
          next_cursor: null,
          has_more: false,
        }),
      });
    }
    // 検索 API は本テストでは空結果を返す（SearchResults が空状態表示に倒れる）
    if (typeof url === "string" && url.startsWith("/api/items/search")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          items: [],
          next_cursor: null,
          has_more: false,
        }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({}),
    });
  });
}

describe("AppShell コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("2ペインレイアウト（左ペイン + 右ペイン）がレンダリングされること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("app-shell")).toBeInTheDocument();
    });

    expect(screen.getByTestId("left-pane")).toBeInTheDocument();
    expect(screen.getByTestId("right-pane")).toBeInTheDocument();
  });

  it("左ペインにフィード一覧が表示されること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    });

    expect(screen.getByText("News Feed")).toBeInTheDocument();
  });

  it("フィード未選択時に右ペインに案内メッセージが表示されること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });
  });

  it("フィードをクリックすると右ペインに記事一覧が表示されること", async () => {
    const user = userEvent.setup();

    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    });

    // フィードをクリック
    await user.click(screen.getByText("Tech Blog"));

    // 記事一覧APIが呼ばれること（フィルタタブが表示されることで確認）
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });
  });

  it("アプリケーションヘッダーが表示されること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Feedman")).toBeInTheDocument();
    });
  });

  it("ヘッダー領域に横断検索バー（HeaderSearchBar）が常設されること（Req 1.1）", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    // ヘッダー直下に検索バー（role="search"）が常設されている
    await waitFor(() => {
      expect(screen.getByTestId("header-search-bar")).toBeInTheDocument();
    });
    expect(screen.getByTestId("header-search-input")).toBeInTheDocument();
  });

  it("検索バーで Enter を押下すると右ペインの ItemList が SearchResults に切り替わること（Req 4.7）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    // 初期状態: ItemList の「フィードを選択してください」が表示される（フィード未選択時）
    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });

    // ヘッダー検索バーで検索を実行
    const input = screen.getByTestId("header-search-input");
    await user.type(input, "typescript");
    await user.keyboard("{Enter}");

    // 右ペインが SearchResults に切り替わる（検索結果は空 → 空状態表示）
    await waitFor(() => {
      expect(
        screen.getByTestId("search-results-empty")
      ).toBeInTheDocument();
    });

    // ItemList の「フィードを選択してください」案内は表示されなくなる
    expect(
      screen.queryByText("フィードを選択してください")
    ).not.toBeInTheDocument();
  });

  it("CLEAR_SEARCH（クリアボタン押下）で右ペインが ItemList に戻ること（Req 1.6 / NFR 2.2）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("header-search-input")).toBeInTheDocument();
    });

    // 検索を実行 → SearchResults に切り替わる
    const input = screen.getByTestId("header-search-input");
    await user.type(input, "kubernetes");
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(
        screen.getByTestId("search-results-empty")
      ).toBeInTheDocument();
    });

    // クリアボタンを押下 → ItemList に戻る
    await user.click(screen.getByTestId("header-search-clear"));

    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });

    // SearchResults は表示されなくなる
    expect(screen.queryByTestId("search-results-empty")).not.toBeInTheDocument();
    expect(screen.queryByTestId("search-results-loading")).not.toBeInTheDocument();
    expect(screen.queryByTestId("search-results")).not.toBeInTheDocument();
  });

  it("検索モード中はフィード選択メッセージが消え、検索結果コンテナが描画されること（Req 4.7）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    });

    // フィードを選択 → ItemList が表示される
    await user.click(screen.getByText("Tech Blog"));
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });

    // ヘッダー検索バーで横断検索を実行 → 右ペインは SearchResults に切り替わる
    const input = screen.getByTestId("header-search-input");
    await user.type(input, "anything");
    await user.keyboard("{Enter}");

    // ItemList のフィルタタブは消え、SearchResults の空状態が表示される
    await waitFor(() => {
      expect(screen.getByTestId("search-results-empty")).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: "全て" })).not.toBeInTheDocument();
  });
});
