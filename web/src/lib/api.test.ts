import { describe, it, expect, beforeEach, vi } from "vitest";
import { createApiClient } from "./api";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("apiClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ data: "test" }),
    });
  });

  describe("GETリクエスト", () => {
    it("GETリクエストにはX-CSRF-Tokenヘッダーを付与しないこと", async () => {
      const api = createApiClient(() => "csrf-token-123");

      await api.get("/api/feeds");

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds", {
        method: "GET",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
      });
    });

    it("GETリクエストでレスポンスを返すこと", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ feeds: [] }),
      });

      const api = createApiClient(() => "token");
      const response = await api.get("/api/feeds");

      expect(response).toEqual({ feeds: [] });
    });
  });

  describe("POSTリクエスト", () => {
    it("POSTリクエストにX-CSRF-Tokenヘッダーを自動付与すること", async () => {
      const api = createApiClient(() => "csrf-token-456");

      await api.post("/api/feeds", { url: "https://example.com/feed.xml" });

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": "csrf-token-456",
        },
        credentials: "include",
        body: JSON.stringify({ url: "https://example.com/feed.xml" }),
      });
    });

    it("ボディなしのPOSTリクエストでもCSRFトークンを付与すること", async () => {
      const api = createApiClient(() => "csrf-token-789");

      await api.post("/api/some-action");

      expect(mockFetch).toHaveBeenCalledWith("/api/some-action", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": "csrf-token-789",
        },
        credentials: "include",
      });
    });
  });

  describe("PUTリクエスト", () => {
    it("PUTリクエストにX-CSRF-Tokenヘッダーを自動付与すること", async () => {
      const api = createApiClient(() => "csrf-put-token");

      await api.put("/api/items/1/state", { is_read: true });

      expect(mockFetch).toHaveBeenCalledWith("/api/items/1/state", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": "csrf-put-token",
        },
        credentials: "include",
        body: JSON.stringify({ is_read: true }),
      });
    });
  });

  describe("PATCHリクエスト", () => {
    it("PATCHリクエストにX-CSRF-Tokenヘッダーを自動付与すること", async () => {
      const api = createApiClient(() => "csrf-patch-token");

      await api.patch("/api/feeds/1", { feed_url: "https://new-url.com" });

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds/1", {
        method: "PATCH",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": "csrf-patch-token",
        },
        credentials: "include",
        body: JSON.stringify({ feed_url: "https://new-url.com" }),
      });
    });
  });

  describe("DELETEリクエスト", () => {
    it("DELETEリクエストにX-CSRF-Tokenヘッダーを自動付与すること", async () => {
      const api = createApiClient(() => "csrf-delete-token");

      await api.delete("/api/feeds/1");

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds/1", {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": "csrf-delete-token",
        },
        credentials: "include",
      });
    });
  });

  describe("CSRFトークンがnullの場合", () => {
    it("mutation系リクエストでもX-CSRF-Tokenヘッダーを付与しないこと", async () => {
      const api = createApiClient(() => null);

      await api.post("/api/feeds", { url: "https://example.com" });

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
        body: JSON.stringify({ url: "https://example.com" }),
      });
    });
  });

  describe("エラーハンドリング", () => {
    it("レスポンスが非OKの場合にエラーをスローすること", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => ({ message: "リソースが見つかりません" }),
      });

      const api = createApiClient(() => "token");

      await expect(api.get("/api/feeds/999")).rejects.toThrow();
    });

    it("エラーレスポンスにステータスコードとボディが含まれること", async () => {
      const errorBody = {
        code: "NOT_FOUND",
        message: "フィードが見つかりません",
      };
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
        json: async () => errorBody,
      });

      const api = createApiClient(() => "token");

      try {
        await api.get("/api/feeds/999");
        // ここに到達しないはず
        expect(true).toBe(false);
      } catch (error: unknown) {
        const apiError = error as { status: number; body: unknown };
        expect(apiError.status).toBe(404);
        expect(apiError.body).toEqual(errorBody);
      }
    });
  });
});
