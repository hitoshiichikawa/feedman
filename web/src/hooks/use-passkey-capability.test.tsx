import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

// `@/lib/passkey-capability` はブラウザ側判定の純粋関数をエクスポートする。
// 本テストではサーバ応答 × ブラウザ対応の 2 軸を独立に制御したいため、
// vi.mock で `isPasskeyBrowserSupported` を差し替えて hook 側の合成ロジックを検証する。
vi.mock("@/lib/passkey-capability", () => ({
  isPasskeyBrowserSupported: vi.fn(),
}));

// `@/lib/api` の apiClient.get をモックしてサーバ側応答（200 / 404 / reject）を制御する。
// 実装は 404 で `ApiError` を throw するため、queryFn は catch して server=false を返す
// 想定。ApiError は `web/src/lib/api.ts` の実物を再 export する。
vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    apiClient: {
      get: vi.fn(),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
    },
  };
});

import { usePasskeyCapability } from "./use-passkey-capability";
import { isPasskeyBrowserSupported } from "@/lib/passkey-capability";
import { apiClient, ApiError } from "@/lib/api";

/**
 * TanStack Query の QueryClient を毎テスト新規に作るラッパ。
 * `retry: false` で 404 / network reject の再試行を抑制する（既存
 * `use-feeds.test.tsx` / `use-auth.test.tsx` と同 idiom）。
 */
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

describe("usePasskeyCapability", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("サーバ 200 + ブラウザあり のとき available: true を返すこと", async () => {
    // Arrange
    vi.mocked(isPasskeyBrowserSupported).mockReturnValue(true);
    vi.mocked(apiClient.get).mockResolvedValue({ available: true });

    // Act
    const { result } = renderHook(() => usePasskeyCapability(), {
      wrapper: createWrapper(),
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.available).toBe(true);
    expect(apiClient.get).toHaveBeenCalledWith("/api/passkey/capability");
  });

  it("サーバ 404 (ApiError) + ブラウザあり のとき available: false を返すこと（サーバが passkey 機能を未提供）", async () => {
    // Arrange
    vi.mocked(isPasskeyBrowserSupported).mockReturnValue(true);
    vi.mocked(apiClient.get).mockRejectedValue(new ApiError(404, null));

    // Act
    const { result } = renderHook(() => usePasskeyCapability(), {
      wrapper: createWrapper(),
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.available).toBe(false);
  });

  it("サーバ 200 + ブラウザなし のとき available: false を返すこと（合成 AND）", async () => {
    // Arrange
    vi.mocked(isPasskeyBrowserSupported).mockReturnValue(false);
    vi.mocked(apiClient.get).mockResolvedValue({ available: true });

    // Act
    const { result } = renderHook(() => usePasskeyCapability(), {
      wrapper: createWrapper(),
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.available).toBe(false);
  });

  it("サーバ fetch reject (ネットワークエラー) + ブラウザあり のとき available: false を返すこと（query 自体は success に落とし込む）", async () => {
    // Arrange: fetch reject を TypeError で表現（`request()` の `fetch(...)` reject 相当）
    vi.mocked(isPasskeyBrowserSupported).mockReturnValue(true);
    vi.mocked(apiClient.get).mockRejectedValue(new TypeError("network error"));

    // Act
    const { result } = renderHook(() => usePasskeyCapability(), {
      wrapper: createWrapper(),
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.available).toBe(false);
  });
});
