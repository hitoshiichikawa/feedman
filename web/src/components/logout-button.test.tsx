import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LogoutButton } from "./logout-button";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// window.location のモック
const mockAssign = vi.fn();
Object.defineProperty(window, "location", {
  value: { assign: mockAssign, href: "http://localhost:3000" },
  writable: true,
});

/** テスト用ラッパー。テストごとに新しい QueryClient を返す。 */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return {
    queryClient,
    Wrapper({ children }: { children: ReactNode }) {
      return (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      );
    },
  };
}

describe("LogoutButton コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 既定は 204 No Content 相当（サーバ #235 修正後の JSON クライアント応答）
    mockFetch.mockResolvedValue({
      ok: true,
      status: 204,
      json: async () => ({}),
    });
  });

  it("ログアウトボタンが表示されること", () => {
    const { Wrapper } = createWrapper();
    render(<LogoutButton />, { wrapper: Wrapper });

    expect(
      screen.getByRole("button", { name: "ログアウト" })
    ).toBeInTheDocument();
  });

  it("ログアウトボタンをクリックするとAPIが呼ばれること", async () => {
    const user = userEvent.setup();
    const { Wrapper } = createWrapper();

    render(<LogoutButton />, { wrapper: Wrapper });

    await user.click(screen.getByRole("button", { name: "ログアウト" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/auth/logout",
        expect.objectContaining({
          method: "POST",
        })
      );
    });
  });

  // Issue #235 Requirement 1.2 / 4.1 / 4.2:
  //   ログアウト成功時は Feedman ルーティング上に実在する `/` に遷移する
  //   （`/login` は存在しないパスなので遷移してはならない）。
  it("ログアウト成功後は `/` に遷移し、`/login` には遷移しないこと", async () => {
    const user = userEvent.setup();
    const { Wrapper } = createWrapper();

    render(<LogoutButton />, { wrapper: Wrapper });

    await user.click(screen.getByRole("button", { name: "ログアウト" }));

    await waitFor(() => {
      expect(mockAssign).toHaveBeenCalledWith("/");
    });
    expect(mockAssign).not.toHaveBeenCalledWith("/login");
    expect(mockAssign).toHaveBeenCalledTimes(1);
  });

  // Requirement 3.1:
  //   ログアウト完了時に前ユーザーのクライアント側キャッシュがクリアされること。
  it("ログアウト成功後は QueryClient のキャッシュがクリアされること（Requirement 3.1）", async () => {
    const user = userEvent.setup();
    const { queryClient, Wrapper } = createWrapper();

    // Arrange: 認証済みユーザーが観察していたキャッシュを事前に格納
    queryClient.setQueryData(["auth", "me"], {
      id: "user-1",
      email: "prev@example.com",
      name: "Prev User",
    });
    queryClient.setQueryData(["feeds"], [{ id: "feed-1", title: "F1" }]);

    render(<LogoutButton />, { wrapper: Wrapper });

    // sanity: 前提としてキャッシュに値が入っている
    expect(queryClient.getQueryData(["auth", "me"])).toBeDefined();
    expect(queryClient.getQueryData(["feeds"])).toBeDefined();

    // Act
    await user.click(screen.getByRole("button", { name: "ログアウト" }));
    await waitFor(() => {
      expect(mockAssign).toHaveBeenCalledWith("/");
    });

    // Assert: ログアウト成功後、全キャッシュが除去されている
    expect(queryClient.getQueryData(["auth", "me"])).toBeUndefined();
    expect(queryClient.getQueryData(["feeds"])).toBeUndefined();
  });

  // Requirement 1.4:
  //   ログアウト処理中はボタンが再クリックできない（多重クリック抑止）。
  it("ログアウト処理中はボタンが非活性となり多重クリックを抑止すること", async () => {
    const user = userEvent.setup();
    const { Wrapper } = createWrapper();

    // fetch を pending 状態にして isPending 分岐を観察できるようにする
    let resolveFetch: (value: {
      ok: true;
      status: 204;
      json: () => Promise<unknown>;
    }) => void = () => undefined;
    mockFetch.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        })
    );

    render(<LogoutButton />, { wrapper: Wrapper });
    const button = screen.getByRole("button", { name: "ログアウト" });

    // Act: 1 回目のクリックで pending に入る
    await user.click(button);
    await waitFor(() => {
      expect(button).toBeDisabled();
    });

    // Act: 2 回目のクリックを試みる（disabled なので fetch は発火しない想定）
    await user.click(button);
    // 追加 fetch が発火していないことを確認
    expect(mockFetch).toHaveBeenCalledTimes(1);

    // クリーンアップ: pending を解放
    resolveFetch({ ok: true, status: 204, json: async () => ({}) });
    await waitFor(() => {
      expect(mockAssign).toHaveBeenCalledWith("/");
    });
  });

  // Requirement 5.1 (ネットワーク到達失敗) と Requirement 5.2 (5xx) の共通挙動:
  //   ユーザーに失敗した旨を認識可能な表示を提示し、ボタンを再操作可能な状態に戻す。
  it("ネットワーク到達失敗時はエラー表示を提示し、ボタンを再操作可能に戻すこと（Requirement 5.1）", async () => {
    const user = userEvent.setup();
    const { Wrapper } = createWrapper();

    mockFetch.mockRejectedValueOnce(new TypeError("Failed to fetch"));

    render(<LogoutButton />, { wrapper: Wrapper });

    await user.click(screen.getByRole("button", { name: "ログアウト" }));

    // エラー表示が role="alert" で提示されること
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("ログアウトに失敗しました");

    // 遷移していないこと（認証済み状態のまま留まる）
    expect(mockAssign).not.toHaveBeenCalled();

    // ボタンが再度活性化していること（再操作可能）
    expect(
      screen.getByRole("button", { name: "ログアウト" })
    ).not.toBeDisabled();
  });

  it("サーバ 5xx 応答時はエラー表示を提示し、ボタンを再操作可能に戻すこと（Requirement 5.2）", async () => {
    const user = userEvent.setup();
    const { Wrapper } = createWrapper();

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ message: "internal error" }),
    });

    render(<LogoutButton />, { wrapper: Wrapper });

    await user.click(screen.getByRole("button", { name: "ログアウト" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("ログアウトに失敗しました");
    expect(mockAssign).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "ログアウト" })
    ).not.toBeDisabled();
  });

  // Requirement 5.4:
  //   エラー表示にサーバ応答の内部詳細（スタックトレース・内部エラー原因）を
  //   反射しないこと。ユーザー可視のテキストは固定文言のみ。
  it("エラー表示にサーバ応答の内部詳細（メッセージ / status 数値）を反射しないこと（Requirement 5.4）", async () => {
    const user = userEvent.setup();
    const { Wrapper } = createWrapper();

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({
        // サーバ由来のスタック風文字列。ユーザーに露出してはならない。
        message:
          "stack trace: at internal.session_store.destroy(/opt/api/internal/session/store.go:42)",
        code: "SESSION_STORE_INTERNAL_ERROR",
      }),
    });

    render(<LogoutButton />, { wrapper: Wrapper });
    await user.click(screen.getByRole("button", { name: "ログアウト" }));

    const alert = await screen.findByRole("alert");
    const text = alert.textContent ?? "";
    expect(text).not.toContain("stack trace");
    expect(text).not.toContain("SESSION_STORE_INTERNAL_ERROR");
    expect(text).not.toContain("500");
    expect(text).not.toContain("session/store.go");
  });
});
