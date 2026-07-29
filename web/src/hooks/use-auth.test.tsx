import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { useCurrentUser, useLogout } from "./use-auth";
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

describe("useCurrentUser", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("認証済みユーザー情報を取得できること", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/auth/me") {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            id: "user-1",
            email: "test@example.com",
            name: "Test User",
            // Issue #241 / Req 2.1・2.2:
            //   /auth/me は常に username キーを返し、パスキー登録済みユーザーでは
            //   文字列を保持する。フックの decode 経路が新フィールドを含めて
            //   User 型として受け渡せることを検証する。
            username: "test-user",
            created_at: "2026-01-01T00:00:00Z",
          }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual({
      id: "user-1",
      email: "test@example.com",
      name: "Test User",
      username: "test-user",
      created_at: "2026-01-01T00:00:00Z",
    });
  });

  it("未認証時（401）はエラー状態になること", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/auth/me") {
        return Promise.resolve({
          ok: false,
          status: 401,
          json: async () => ({ message: "Unauthorized" }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const { result } = renderHook(() => useCurrentUser(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });
});

describe("useLogout", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("ログアウトAPIを呼び出せること", async () => {
    mockFetch.mockImplementation((url: string, options?: RequestInit) => {
      if (url === "/auth/logout" && options?.method === "POST") {
        return Promise.resolve({
          ok: true,
          json: async () => ({}),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    const { result } = renderHook(() => useLogout(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/auth/logout",
      expect.objectContaining({
        method: "POST",
      })
    );
  });

  // Issue #235:
  //   fetch はサーバの content negotiation を成立させるため Accept: application/json を
  //   送信する必要がある。これが無いと従来クライアントとして 303 + Location が返り、
  //   fetch の既定 redirect: "follow" で遷移先 HTML が JSON パースに失敗する。
  it("useLogout は Accept: application/json を送信すること（Issue #235 サーバ content negotiation 前提）", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 204,
      json: async () => ({}),
    });

    const { result } = renderHook(() => useLogout(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const [, options] = mockFetch.mock.calls[0] as [string, RequestInit];
    const headers = options.headers as Record<string, string>;
    expect(headers.Accept).toBe("application/json");
  });

  // Requirement 3.1 の中核: mutation success 時にキャッシュをクリアする。
  // useLogout フック側にも onSuccess 契約があり、logout-button の onSuccess とは
  // 独立した責務（キャッシュクリア）を担う。
  it("useLogout は成功時に QueryClient のキャッシュをクリアすること（Requirement 3.1）", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 204,
      json: async () => ({}),
    });

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    queryClient.setQueryData(["auth", "me"], { id: "user-x" });

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useLogout(), { wrapper });

    result.current.mutate();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // 保持していたキャッシュエントリが除去される
    expect(queryClient.getQueryData(["auth", "me"])).toBeUndefined();
  });
});
