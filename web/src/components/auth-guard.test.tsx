import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CSRFProvider } from "@/lib/csrf";
import { AuthGuard } from "./auth-guard";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// window.location のモック
const mockAssign = vi.fn();
Object.defineProperty(window, "location", {
  value: { assign: mockAssign, href: "http://localhost:3000", pathname: "/dashboard" },
  writable: true,
});

/** テスト用ラッパー */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
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

describe("AuthGuard コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("認証済みの場合は子コンポーネントを表示すること", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/csrf-token") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ token: "test-csrf-token" }),
        });
      }
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

    render(
      <AuthGuard>
        <div data-testid="protected-content">保護されたコンテンツ</div>
      </AuthGuard>,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByTestId("protected-content")).toBeInTheDocument();
    });
  });

  it("未認証の場合はログインページにリダイレクトすること", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/csrf-token") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ token: "test-csrf-token" }),
        });
      }
      if (url === "/auth/me") {
        return Promise.resolve({
          ok: false,
          status: 401,
          json: async () => ({ message: "Unauthorized" }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    render(
      <AuthGuard>
        <div data-testid="protected-content">保護されたコンテンツ</div>
      </AuthGuard>,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.queryByTestId("protected-content")).not.toBeInTheDocument();
    });

    // ログインページへのリダイレクトが発生すること
    await waitFor(() => {
      expect(screen.getByTestId("auth-redirect")).toBeInTheDocument();
    });
  });

  it("認証確認中はローディング表示すること", () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/csrf-token") {
        return Promise.resolve({
          ok: true,
          json: async () => ({ token: "test-csrf-token" }),
        });
      }
      // auth/me は解決しない
      return new Promise(() => {});
    });

    render(
      <AuthGuard>
        <div data-testid="protected-content">保護されたコンテンツ</div>
      </AuthGuard>,
      { wrapper: createWrapper() }
    );

    // 保護されたコンテンツは表示されないこと
    expect(screen.queryByTestId("protected-content")).not.toBeInTheDocument();
    // ローディング表示があること
    expect(screen.getByTestId("auth-loading")).toBeInTheDocument();
  });
});
