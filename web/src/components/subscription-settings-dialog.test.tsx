import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SubscriptionSettingsDialog } from "./subscription-settings-dialog";
import type { Subscription } from "@/types/feed";
import type { ReactNode } from "react";

// グローバル fetch のモック
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
        {children}
      </QueryClientProvider>
    );
  };
}

/** テスト用のアクティブな購読データ */
const mockSubscription: Subscription = {
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

function setupMockFetch() {
  mockFetch.mockResolvedValue({
    ok: true,
    json: async () => ({}),
  });
}

describe("SubscriptionSettingsDialog コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("open=true かつ subscription が与えられたとき SubscriptionSettings が描画されること", () => {
    render(
      <SubscriptionSettingsDialog
        open={true}
        subscription={mockSubscription}
        onOpenChange={() => {}}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // タイトルが表示される（AC 2.5 関連: ダイアログヘッダ）
    expect(screen.getByText("フィードの設定")).toBeInTheDocument();
    // SubscriptionSettings の中身が描画される（購読解除ボタン）
    expect(screen.getByTestId("unsubscribe-button")).toBeInTheDocument();
    // 更新間隔セレクトも表示される
    expect(screen.getByTestId("fetch-interval-select")).toBeInTheDocument();
  });

  it("open=false のとき内容が描画されないこと", () => {
    render(
      <SubscriptionSettingsDialog
        open={false}
        subscription={mockSubscription}
        onOpenChange={() => {}}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // Dialog が閉じているので中身は描画されない
    expect(screen.queryByText("フィードの設定")).not.toBeInTheDocument();
    expect(screen.queryByTestId("unsubscribe-button")).not.toBeInTheDocument();
  });

  it("subscription === null のとき open=true でも SubscriptionSettings が描画されないこと", () => {
    render(
      <SubscriptionSettingsDialog
        open={true}
        subscription={null}
        onOpenChange={() => {}}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // タイトルは表示される（Dialog は open）
    expect(screen.getByText("フィードの設定")).toBeInTheDocument();
    // が、SubscriptionSettings の中身は描画されない（防御的ガード）
    expect(screen.queryByTestId("unsubscribe-button")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("fetch-interval-select")
    ).not.toBeInTheDocument();
  });

  it("購読解除成功時に親の onUnsubscribed が feed_id 引数で呼ばれ、onOpenChange(false) も呼ばれること（AC 4.4）", async () => {
    const user = userEvent.setup();
    const onUnsubscribed = vi.fn();
    const onOpenChange = vi.fn();

    // DELETE /api/subscriptions/sub-1 を成功させる
    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/subscriptions/sub-1" && options?.method === "DELETE") {
        return Promise.resolve({
          ok: true,
          json: async () => ({}),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    render(
      <SubscriptionSettingsDialog
        open={true}
        subscription={mockSubscription}
        onOpenChange={onOpenChange}
        onUnsubscribed={onUnsubscribed}
      />,
      { wrapper: createWrapper() }
    );

    // 購読解除ボタン押下 → 確認ダイアログ表示
    await user.click(screen.getByTestId("unsubscribe-button"));

    await waitFor(() => {
      expect(screen.getByText("購読を解除しますか？")).toBeInTheDocument();
    });

    // 確認ダイアログの「購読解除」ボタン押下
    await user.click(screen.getByRole("button", { name: "購読解除" }));

    // mutation 成功後、親の onUnsubscribed が feed_id 引数で呼ばれる
    await waitFor(() => {
      expect(onUnsubscribed).toHaveBeenCalledTimes(1);
    });
    expect(onUnsubscribed).toHaveBeenCalledWith("feed-1");

    // 同時に onOpenChange(false) も呼ばれる（ダイアログ閉鎖）
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it("Esc キーで onOpenChange(false) が呼ばれること（AC 2.5 / NFR 2.2: radix-ui 既定挙動）", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <SubscriptionSettingsDialog
        open={true}
        subscription={mockSubscription}
        onOpenChange={onOpenChange}
        onUnsubscribed={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // ダイアログが表示されていることを確認
    expect(screen.getByText("フィードの設定")).toBeInTheDocument();

    // Esc キー押下
    await user.keyboard("{Escape}");

    // onOpenChange(false) が呼ばれる
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});
