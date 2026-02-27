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
});
