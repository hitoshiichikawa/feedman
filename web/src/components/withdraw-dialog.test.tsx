import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CSRFProvider } from "@/lib/csrf";
import { WithdrawDialog } from "./withdraw-dialog";
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

describe("WithdrawDialog コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("退会ボタンが表示されること", () => {
    render(<WithdrawDialog />, { wrapper: createWrapper() });

    expect(screen.getByTestId("withdraw-trigger")).toBeInTheDocument();
  });

  it("退会ボタンをクリックすると確認ダイアログが開くこと", async () => {
    const user = userEvent.setup();

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });
  });

  it("確認ダイアログに警告メッセージが表示されること", async () => {
    const user = userEvent.setup();

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(
        screen.getByText(/すべてのデータが削除されます/)
      ).toBeInTheDocument();
    });
  });

  it("キャンセルボタンで退会が実行されないこと", async () => {
    const user = userEvent.setup();

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "キャンセル" }));

    // 退会APIが呼ばれていないこと
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/account",
      expect.objectContaining({ method: "DELETE" })
    );
  });

  it("退会実行ボタンをクリックするとAPIが呼ばれること", async () => {
    const user = userEvent.setup();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/csrf-token") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ token: "test-csrf-token" }),
        });
      }
      if (url === "/api/account" && options?.method === "DELETE") {
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

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    // 退会実行ボタンをクリック
    await user.click(screen.getByTestId("withdraw-confirm"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/account",
        expect.objectContaining({ method: "DELETE" })
      );
    });
  });

  it("退会成功時にonWithdrawnコールバックが呼ばれること", async () => {
    const user = userEvent.setup();
    const onWithdrawn = vi.fn();

    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/csrf-token") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ token: "test-csrf-token" }),
        });
      }
      if (url === "/api/account" && options?.method === "DELETE") {
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

    render(<WithdrawDialog onWithdrawn={onWithdrawn} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-confirm"));

    await waitFor(() => {
      expect(onWithdrawn).toHaveBeenCalled();
    });
  });
});
