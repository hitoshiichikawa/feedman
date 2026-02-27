import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { CSRFProvider, useCSRFToken } from "./csrf";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

/** CSRFトークンの値を表示するテスト用コンポーネント */
function TestConsumer() {
  const token = useCSRFToken();
  return <span data-testid="csrf-token">{token ?? "null"}</span>;
}

describe("CSRFProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("マウント時にCSRFトークンをAPIから取得すること", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ token: "test-csrf-token-123" }),
    });

    render(
      <CSRFProvider>
        <TestConsumer />
      </CSRFProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId("csrf-token").textContent).toBe(
        "test-csrf-token-123"
      );
    });

    expect(mockFetch).toHaveBeenCalledWith("/api/csrf-token", {
      credentials: "include",
    });
  });

  it("トークン取得前はnullを返すこと", () => {
    // fetchを解決しないPromiseにする
    mockFetch.mockReturnValueOnce(new Promise(() => {}));

    render(
      <CSRFProvider>
        <TestConsumer />
      </CSRFProvider>
    );

    expect(screen.getByTestId("csrf-token").textContent).toBe("null");
  });

  it("APIエラー時はnullのままであること", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
    });

    // コンソールエラーを抑制
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <CSRFProvider>
        <TestConsumer />
      </CSRFProvider>
    );

    // fetchの完了を待つ
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByTestId("csrf-token").textContent).toBe("null");

    consoleSpy.mockRestore();
  });

  it("ネットワークエラー時はnullのままであること", async () => {
    mockFetch.mockRejectedValueOnce(new Error("Network error"));

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <CSRFProvider>
        <TestConsumer />
      </CSRFProvider>
    );

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByTestId("csrf-token").textContent).toBe("null");

    consoleSpy.mockRestore();
  });

  it("Provider外でuseCSRFTokenを使用するとエラーになること", () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    expect(() => {
      render(<TestConsumer />);
    }).toThrow();

    consoleSpy.mockRestore();
  });
});
