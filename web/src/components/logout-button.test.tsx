import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CSRFProvider } from "@/lib/csrf";
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

describe("LogoutButton コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
  });

  it("ログアウトボタンが表示されること", () => {
    render(<LogoutButton />, { wrapper: createWrapper() });

    expect(
      screen.getByRole("button", { name: "ログアウト" })
    ).toBeInTheDocument();
  });

  it("ログアウトボタンをクリックするとAPIが呼ばれること", async () => {
    const user = userEvent.setup();

    render(<LogoutButton />, { wrapper: createWrapper() });

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
});
