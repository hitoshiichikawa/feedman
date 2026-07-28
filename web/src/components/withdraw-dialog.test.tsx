import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { WithdrawDialog } from "./withdraw-dialog";
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
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

/**
 * `/api/users/me` に対する DELETE のみを成功させる mockFetch を組み立てる。
 * 他 URL は素の 200 応答を返す（副次呼び出しがテストを failさせないように）。
 */
function setupWithdrawMockFetch(status: "success" | "fail-server" | "fail-network") {
  mockFetch.mockImplementation((url: string, options?: RequestInit) => {
    if (url === "/api/users/me" && options?.method === "DELETE") {
      if (status === "success") {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({}),
        });
      }
      if (status === "fail-server") {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ error: "internal server error" }),
        });
      }
      // fail-network: fetch reject
      return Promise.reject(new TypeError("Network error"));
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

describe("WithdrawDialog コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupWithdrawMockFetch("success");
  });

  it("退会ボタンが表示されること", () => {
    render(<WithdrawDialog />, { wrapper: createWrapper() });

    expect(screen.getByTestId("withdraw-trigger")).toBeInTheDocument();
  });

  it("退会ボタンをクリックすると確認ダイアログが開くこと (Req 3.2)", async () => {
    const user = userEvent.setup();

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });
  });

  it("確認ダイアログに削除・取り消し不能である旨の警告文が表示されること (Req 3.2)", async () => {
    const user = userEvent.setup();

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(
        screen.getByText(/すべてのデータが削除されます/)
      ).toBeInTheDocument();
    });
    expect(screen.getByText(/取り消せません/)).toBeInTheDocument();
  });

  it("キャンセルボタンで退会要求が送信されないこと (Req 3.3)", async () => {
    const user = userEvent.setup();
    const onWithdrawn = vi.fn();

    render(<WithdrawDialog onWithdrawn={onWithdrawn} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "キャンセル" }));

    // DELETE /api/users/me は呼ばれていない
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/users/me",
      expect.objectContaining({ method: "DELETE" })
    );
    // onWithdrawn も呼ばれていない（認証状態が維持される）
    expect(onWithdrawn).not.toHaveBeenCalled();
  });

  it("退会実行時に DELETE /api/users/me を送信すること (Req 3.4)", async () => {
    const user = userEvent.setup();

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-confirm"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/users/me",
        expect.objectContaining({ method: "DELETE" })
      );
    });
    // 旧エンドポイント /api/account に対しては呼ばれない（回帰防止）
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/account",
      expect.objectContaining({ method: "DELETE" })
    );
  });

  it("退会成功時に onWithdrawn が呼ばれ、確認ダイアログが閉じること (Req 3.5)", async () => {
    const user = userEvent.setup();
    const onWithdrawn = vi.fn();

    render(<WithdrawDialog onWithdrawn={onWithdrawn} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-confirm"));

    await waitFor(() => {
      expect(onWithdrawn).toHaveBeenCalledTimes(1);
    });
    // 確認ダイアログが閉じている
    await waitFor(() => {
      expect(screen.queryByText("退会しますか？")).not.toBeInTheDocument();
    });
  });

  it("退会失敗時（サーバ 500）に onWithdrawn を呼ばず、確認ダイアログ内にエラーが表示されること (Req 3.6)", async () => {
    const user = userEvent.setup();
    const onWithdrawn = vi.fn();
    setupWithdrawMockFetch("fail-server");

    render(<WithdrawDialog onWithdrawn={onWithdrawn} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByTestId("withdraw-trigger"));

    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-confirm"));

    // DELETE は発行されたが失敗
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/users/me",
        expect.objectContaining({ method: "DELETE" })
      );
    });

    // インラインでエラー通知が表示される
    await waitFor(() => {
      expect(screen.getByTestId("withdraw-error")).toBeInTheDocument();
    });
    expect(screen.getByTestId("withdraw-error")).toHaveTextContent(
      "退会に失敗しました"
    );

    // onWithdrawn は呼ばれていない（セッション維持 / Req 3.6）
    expect(onWithdrawn).not.toHaveBeenCalled();

    // 確認ダイアログは開いたまま（再試行できるように残る）
    expect(screen.getByText("退会しますか？")).toBeInTheDocument();
  });

  it("退会失敗時（ネットワーク到達不能）でも onWithdrawn が呼ばれず、エラー表示されること (Req 3.6 境界値)", async () => {
    const user = userEvent.setup();
    const onWithdrawn = vi.fn();
    setupWithdrawMockFetch("fail-network");

    render(<WithdrawDialog onWithdrawn={onWithdrawn} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByTestId("withdraw-trigger"));
    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });
    await user.click(screen.getByTestId("withdraw-confirm"));

    await waitFor(() => {
      expect(screen.getByTestId("withdraw-error")).toBeInTheDocument();
    });
    expect(onWithdrawn).not.toHaveBeenCalled();
  });

  it("応答待機中は退会確定ボタンが disabled で多重送信されないこと (Req 3.8)", async () => {
    const user = userEvent.setup();
    // DELETE は resolve せず pending 状態を維持する
    let resolveFn: ((value: { ok: boolean; json: () => Promise<unknown> }) => void) | undefined;
    const pendingPromise = new Promise<{
      ok: boolean;
      json: () => Promise<unknown>;
    }>((resolve) => {
      resolveFn = resolve;
    });
    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/api/users/me" && options?.method === "DELETE") {
        return pendingPromise;
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(<WithdrawDialog />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("withdraw-trigger"));
    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    // 1 回目の click で fetch 発行 & pending
    await user.click(screen.getByTestId("withdraw-confirm"));

    // disabled 状態になっている
    await waitFor(() => {
      expect(screen.getByTestId("withdraw-confirm")).toBeDisabled();
    });
    expect(screen.getByTestId("withdraw-confirm")).toHaveTextContent("処理中...");

    // 2 回目の click は無効化されているため fetch は増えない
    await user.click(screen.getByTestId("withdraw-confirm"));
    await user.click(screen.getByTestId("withdraw-confirm"));

    const deleteCalls = mockFetch.mock.calls.filter(
      ([url, options]) =>
        url === "/api/users/me" &&
        (options as RequestInit | undefined)?.method === "DELETE"
    );
    expect(deleteCalls).toHaveLength(1);

    // クリーンアップ: pending を resolve して leak を防ぐ
    resolveFn?.({ ok: true, json: async () => ({}) });
  });
});
