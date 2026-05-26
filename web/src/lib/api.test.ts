import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { createApiClient, apiClient } from "./api";

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

  describe("NEXT_PUBLIC_API_URL 非依存", () => {
    afterEach(() => {
      vi.unstubAllEnvs();
      vi.resetModules();
    });

    it("NEXT_PUBLIC_API_URL が設定されていても相対パスで fetch されること", async () => {
      // Arrange: 環境変数に絶対 URL を設定したうえでモジュールを読み込み直す
      // （API_BASE_URL はモジュールロード時に評価される const のため）
      vi.stubEnv("NEXT_PUBLIC_API_URL", "https://api.example.com");
      vi.resetModules();
      const { createApiClient: createApiClientWithEnv } = await import("./api");
      const api = createApiClientWithEnv();

      // Act
      await api.get("/api/feeds");

      // Assert: 絶対 URL ではなく相対パスで呼ばれる
      expect(mockFetch).toHaveBeenCalledWith("/api/feeds", {
        method: "GET",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
      });
    });

    it("NEXT_PUBLIC_API_URL が設定されていても API_BASE_URL が空文字のままであること", async () => {
      // Arrange
      vi.stubEnv("NEXT_PUBLIC_API_URL", "https://api.example.com/");
      vi.resetModules();

      // Act
      const { API_BASE_URL } = await import("./api");

      // Assert
      expect(API_BASE_URL).toBe("");
    });
  });

  describe("共有インスタンス apiClient", () => {
    it("モジュールから公開された共有インスタンスが定義されていること", () => {
      // Arrange / Act / Assert: モジュール初期化時に生成された共有インスタンスが存在する
      // (Requirement 1.1)
      expect(apiClient).toBeDefined();
      expect(typeof apiClient.get).toBe("function");
      expect(typeof apiClient.post).toBe("function");
      expect(typeof apiClient.put).toBe("function");
      expect(typeof apiClient.patch).toBe("function");
      expect(typeof apiClient.delete).toBe("function");
    });

    it("複数回参照しても同一の共有インスタンス参照を返すこと", async () => {
      // Arrange: 異なる消費者が同一モジュールを import する状況を模す。
      // モジュールレジストリがリセットされていない限り、import は同一モジュールを返す。
      // (Requirement 1.2 / 1.3 / NFR 2.1: 複数消費者が同一インスタンスを共有する)
      // Act
      const moduleA = await import("./api");
      const moduleB = await import("./api");

      // Assert: 2 つの import が返す共有インスタンスは同一参照（追加生成されない）
      expect(moduleA.apiClient).toBe(moduleB.apiClient);
    });

    it("同一参照を複数回読み出しても同じオブジェクトであること", () => {
      // Arrange / Act: 静的 import した共有インスタンスを複数回参照する
      const refA = apiClient;
      const refB = apiClient;

      // Assert: 同一の共有インスタンス（Requirement 1.1 / 1.2）
      expect(refA).toBe(refB);
      expect(refA).toBe(apiClient);
    });

    it("共有インスタンスが createApiClient で生成した新規インスタンスとは別オブジェクトであること", () => {
      // Arrange / Act: createApiClient は呼ぶたびに新しいインスタンスを返す（温存された関数）
      const fresh = createApiClient();

      // Assert: 共有インスタンスは固定で、新規生成インスタンスとは別参照
      // (Requirement 3.2: createApiClient はモック差し替え用途で温存される)
      expect(fresh).not.toBe(apiClient);
      const freshAgain = createApiClient();
      expect(freshAgain).not.toBe(fresh);
    });

    it("共有インスタンス経由でも正しい URL・メソッドで fetch されること", async () => {
      // Arrange
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: "ok" }),
      });

      // Act
      await apiClient.get("/api/subscriptions");

      // Assert (Requirement 2.x: 利用側挙動の不変性)
      expect(mockFetch).toHaveBeenCalledWith("/api/subscriptions", {
        method: "GET",
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
