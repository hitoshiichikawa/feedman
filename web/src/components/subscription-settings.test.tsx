import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CSRFProvider } from "@/lib/csrf";
import { SubscriptionSettings } from "./subscription-settings";
import type { Subscription } from "@/types/feed";
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

/** テスト用のアクティブな購読データ */
const mockActiveSubscription: Subscription = {
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
};

/** テスト用の停止中の購読データ */
const mockStoppedSubscription: Subscription = {
  ...mockActiveSubscription,
  id: "sub-2",
  feed_id: "feed-2",
  feed_status: "stopped",
  error_message: "404 Not Found",
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
    return Promise.resolve({
      ok: true,
      json: async () => ({}),
    });
  });
}

describe("SubscriptionSettings コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("フェッチ間隔設定のセレクトが表示されること", () => {
    render(
      <SubscriptionSettings
        subscription={mockActiveSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // フェッチ間隔の表示が存在すること
    expect(screen.getByTestId("fetch-interval-select")).toBeInTheDocument();
  });

  it("現在のフェッチ間隔が選択状態で表示されること", () => {
    render(
      <SubscriptionSettings
        subscription={mockActiveSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // 60分 = 1時間として表示
    const trigger = screen.getByTestId("fetch-interval-select");
    expect(trigger).toHaveTextContent("1時間");
  });

  it("購読解除ボタンが表示されること", () => {
    render(
      <SubscriptionSettings
        subscription={mockActiveSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByTestId("unsubscribe-button")).toBeInTheDocument();
  });

  it("購読解除ボタンをクリックすると確認ダイアログが表示されること", async () => {
    const user = userEvent.setup();

    render(
      <SubscriptionSettings
        subscription={mockActiveSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByTestId("unsubscribe-button"));

    await waitFor(() => {
      expect(
        screen.getByText("購読を解除しますか？")
      ).toBeInTheDocument();
    });
  });

  it("確認ダイアログでキャンセルすると購読解除されないこと", async () => {
    const user = userEvent.setup();
    const onUnsubscribed = vi.fn();

    render(
      <SubscriptionSettings
        subscription={mockActiveSubscription}
        onUnsubscribed={onUnsubscribed}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByTestId("unsubscribe-button"));

    await waitFor(() => {
      expect(screen.getByText("購読を解除しますか？")).toBeInTheDocument();
    });

    // キャンセルボタンをクリック
    await user.click(screen.getByRole("button", { name: "キャンセル" }));

    // APIが呼ばれていないこと
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/subscriptions/sub-1",
      expect.objectContaining({ method: "DELETE" })
    );
  });

  it("停止中フィードの「再開」ボタンが表示されること", () => {
    render(
      <SubscriptionSettings
        subscription={mockStoppedSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByTestId("resume-button")).toBeInTheDocument();
  });

  it("アクティブなフィードでは「再開」ボタンが表示されないこと", () => {
    render(
      <SubscriptionSettings
        subscription={mockActiveSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.queryByTestId("resume-button")).not.toBeInTheDocument();
  });

  it("停止中フィードのエラーメッセージが表示されること", () => {
    render(
      <SubscriptionSettings
        subscription={mockStoppedSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("404 Not Found")).toBeInTheDocument();
  });

  it("再開ボタンをクリックするとAPIが呼ばれること", async () => {
    const user = userEvent.setup();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/csrf-token") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ token: "test-csrf-token" }),
        });
      }
      if (url === "/api/feeds/feed-2/resume" && options?.method === "POST") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "sub-2",
            feed_id: "feed-2",
            feed_status: "active",
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    render(
      <SubscriptionSettings
        subscription={mockStoppedSubscription}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByTestId("resume-button"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/feeds/feed-2/resume",
        expect.objectContaining({ method: "POST" })
      );
    });
  });
});
