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
    it("GETリクエストを正しく送信すること", async () => {
      const api = createApiClient();

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

      const api = createApiClient();
      const response = await api.get("/api/feeds");

      expect(response).toEqual({ feeds: [] });
    });
  });

  describe("POSTリクエスト", () => {
    it("POSTリクエストを正しく送信すること", async () => {
      const api = createApiClient();

      await api.post("/api/feeds", { url: "https://example.com/feed.xml" });

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
        body: JSON.stringify({ url: "https://example.com/feed.xml" }),
      });
    });

    it("ボディなしのPOSTリクエストを送信できること", async () => {
      const api = createApiClient();

      await api.post("/api/some-action");

      expect(mockFetch).toHaveBeenCalledWith("/api/some-action", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
      });
    });
  });

  describe("PUTリクエスト", () => {
    it("PUTリクエストを正しく送信すること", async () => {
      const api = createApiClient();

      await api.put("/api/items/1/state", { is_read: true });

      expect(mockFetch).toHaveBeenCalledWith("/api/items/1/state", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
        body: JSON.stringify({ is_read: true }),
      });
    });
  });

  describe("PATCHリクエスト", () => {
    it("PATCHリクエストを正しく送信すること", async () => {
      const api = createApiClient();

      await api.patch("/api/feeds/1", { feed_url: "https://new-url.com" });

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds/1", {
        method: "PATCH",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
        body: JSON.stringify({ feed_url: "https://new-url.com" }),
      });
    });
  });

  describe("DELETEリクエスト", () => {
    it("DELETEリクエストを正しく送信すること", async () => {
      const api = createApiClient();

      await api.delete("/api/feeds/1");

      expect(mockFetch).toHaveBeenCalledWith("/api/feeds/1", {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
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

      const api = createApiClient();

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

      const api = createApiClient();

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
