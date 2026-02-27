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
});
