import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { FeedRegisterDialog } from "./feed-register-dialog";
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
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("FeedRegisterDialog コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({}),
    });
  });

  it("トリガーボタンをクリックするとダイアログが開くこと", async () => {
    const user = userEvent.setup();
    render(<FeedRegisterDialog onRegistered={() => {}} />, {
      wrapper: createWrapper(),
    });

    const triggerButton = screen.getByRole("button", { name: "フィード追加" });
    expect(triggerButton).toBeInTheDocument();

    await user.click(triggerButton);

    expect(screen.getByText("フィードを登録")).toBeInTheDocument();
  });

  it("URL入力欄が1つ表示されること", async () => {
    const user = userEvent.setup();
    render(<FeedRegisterDialog onRegistered={() => {}} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole("button", { name: "フィード追加" }));

    const urlInput = screen.getByPlaceholderText("https://example.com");
    expect(urlInput).toBeInTheDocument();
  });

  it("URLを入力して登録ボタンをクリックするとAPIが呼ばれること", async () => {
    const user = userEvent.setup();
    const onRegistered = vi.fn();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/feeds" && options?.method === "POST") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "feed-1",
            feed_url: "https://example.com/feed.xml",
            title: "Example Feed",
            site_url: "https://example.com",
            favicon_url: null,
            feed_status: "active",
            created_at: "2026-01-01T00:00:00Z",
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    render(<FeedRegisterDialog onRegistered={onRegistered} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole("button", { name: "フィード追加" }));

    const urlInput = screen.getByPlaceholderText("https://example.com");
    await user.type(urlInput, "https://example.com");

    const submitButton = screen.getByRole("button", { name: "登録" });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/feeds",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ url: "https://example.com" }),
        })
      );
    });
  });

  it("登録成功時にフィードURLが表示されユーザーが変更可能であること", async () => {
    const user = userEvent.setup();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/feeds" && options?.method === "POST") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "feed-1",
            feed_url: "https://example.com/feed.xml",
            title: "Example Feed",
            site_url: "https://example.com",
            favicon_url: null,
            feed_status: "active",
            created_at: "2026-01-01T00:00:00Z",
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    render(<FeedRegisterDialog onRegistered={() => {}} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole("button", { name: "フィード追加" }));
    await user.type(
      screen.getByPlaceholderText("https://example.com"),
      "https://example.com"
    );
    await user.click(screen.getByRole("button", { name: "登録" }));

    // 登録成功後、フィードURLが表示される
    await waitFor(() => {
      expect(screen.getByText("登録完了")).toBeInTheDocument();
    });

    // フィードURL表示欄があること
    const feedUrlInput = screen.getByDisplayValue("https://example.com/feed.xml");
    expect(feedUrlInput).toBeInTheDocument();
  });

  it("フィード未検出エラー時にエラーメッセージを表示すること", async () => {
    const user = userEvent.setup();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/feeds" && options?.method === "POST") {
        return Promise.resolve({
          ok: false,
          status: 422,
          json: async () => ({
            code: "FEED_NOT_FOUND",
            message: "指定されたURLからフィードを検出できませんでした",
            category: "feed",
            action: "RSS/AtomフィードのURLを直接入力してください",
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    render(<FeedRegisterDialog onRegistered={() => {}} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole("button", { name: "フィード追加" }));
    await user.type(
      screen.getByPlaceholderText("https://example.com"),
      "https://no-feed.example.com"
    );
    await user.click(screen.getByRole("button", { name: "登録" }));

    // エラー表示：原因カテゴリと対処方法が表示される
    await waitFor(() => {
      expect(
        screen.getByText("指定されたURLからフィードを検出できませんでした")
      ).toBeInTheDocument();
    });

    expect(
      screen.getByText("RSS/AtomフィードのURLを直接入力してください")
    ).toBeInTheDocument();
  });

  it("購読上限到達エラー時にエラーメッセージを表示すること", async () => {
    const user = userEvent.setup();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/feeds" && options?.method === "POST") {
        return Promise.resolve({
          ok: false,
          status: 409,
          json: async () => ({
            code: "SUBSCRIPTION_LIMIT",
            message: "購読上限（100件）に達しています",
            category: "validation",
            action: "不要なフィードを解除してから再度お試しください",
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    });

    render(<FeedRegisterDialog onRegistered={() => {}} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole("button", { name: "フィード追加" }));
    await user.type(
      screen.getByPlaceholderText("https://example.com"),
      "https://example.com"
    );
    await user.click(screen.getByRole("button", { name: "登録" }));

    await waitFor(() => {
      expect(
        screen.getByText("購読上限（100件）に達しています")
      ).toBeInTheDocument();
    });

    expect(
      screen.getByText("不要なフィードを解除してから再度お試しください")
    ).toBeInTheDocument();
  });

  it("URLが空のまま登録ボタンをクリックできないこと", async () => {
    const user = userEvent.setup();
    render(<FeedRegisterDialog onRegistered={() => {}} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole("button", { name: "フィード追加" }));

    const submitButton = screen.getByRole("button", { name: "登録" });
    expect(submitButton).toBeDisabled();
  });
});
