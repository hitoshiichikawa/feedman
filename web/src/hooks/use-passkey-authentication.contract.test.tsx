import { renderHook, waitFor, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

/**
 * `use-passkey-authentication` の **契約テスト**。
 *
 * 他の hook テスト（`use-passkey-authentication.test.tsx`）は `@/lib/api` の `apiClient` を
 * 丸ごとモックするため、共通 API クライアント（`request<T>`）が `POST /api/auth/session` の
 * **204 No Content** をどう扱うかを検証できない。本ファイルは意図的に `apiClient` を
 * **モックせず**、`global.fetch` のみをモックすることで、実 `apiClient` を経由した際に
 * 204 応答で正常系 chain（`onSuccess` → `invalidateQueries`）が成立することを保証する
 * （PR #229 review Blocker: 204 に対し `response.json()` を無条件に呼ぶと `SyntaxError` で
 * 正常系が失敗する回帰の防止）。
 *
 * PKCE / WebAuthn / navigator は jsdom に実体がないためモックする（NFR 3.1）が、
 * `@/lib/api` は実物を使う点が本テストの肝である。
 */

const mockFetch = vi.fn();
global.fetch = mockFetch;

vi.mock("@/lib/pkce", () => ({
  generatePkcePair: vi.fn(),
}));

vi.mock("@/lib/webauthn", () => ({
  decodeRequestOptions: vi.fn(),
  encodeAssertionResponse: vi.fn(),
}));

import { usePasskeyAuthentication } from "./use-passkey-authentication";
import { generatePkcePair } from "@/lib/pkce";
import {
  decodeRequestOptions,
  encodeAssertionResponse,
} from "@/lib/webauthn";

function createWrapper(): {
  Wrapper: ({ children }: { children: ReactNode }) => React.ReactElement;
  queryClient: QueryClient;
} {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { Wrapper, queryClient };
}

/** JSON ボディを持つ 200 応答を模す fetch 戻り値。 */
function jsonOk(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

describe("usePasskeyAuthentication（契約テスト: apiClient 非モック）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(generatePkcePair).mockResolvedValue({
      codeVerifier: "test-verifier",
      codeChallenge: "test-challenge",
    });
    vi.mocked(decodeRequestOptions).mockReturnValue({
      publicKey: {
        challenge: new Uint8Array([1]).buffer,
      } as unknown as PublicKeyCredentialRequestOptions,
    });
    vi.mocked(encodeAssertionResponse).mockReturnValue({
      id: "cred-id",
      type: "public-key",
      rawId: "AQID",
      response: {
        clientDataJSON: "cdj",
        authenticatorData: "auth",
        signature: "sig",
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("session 交換が 204 No Content を返しても、実 apiClient 経由で成功し invalidateQueries が呼ばれること", async () => {
    // Arrange: navigator.credentials.get は assertion を返す
    const cred = {
      id: "cred-id",
      type: "public-key",
      rawId: new Uint8Array([1, 2, 3]).buffer,
      response: {},
    } as unknown as PublicKeyCredential;
    vi.stubGlobal("navigator", { credentials: { get: vi.fn().mockResolvedValue(cred) } });

    // fetch を URL 別に応答分岐。/api/auth/session は 204（ボディなし = json 未提供）。
    // 実 request<T> が 204 で json() を呼ばず正常終了することを検証する。
    mockFetch.mockImplementation(async (url: string) => {
      if (url === "/api/passkey/authentication/begin") {
        return jsonOk({ challenge_id: "chal-1", options: { publicKey: {} } });
      }
      if (url === "/api/passkey/authentication/finish") {
        return jsonOk({ auth_code: "auth-1" });
      }
      if (url === "/api/auth/session") {
        // 204: json は敢えて呼ばれてはならない（呼ばれたら SyntaxError で失敗する）
        return {
          ok: true,
          status: 204,
          json: async () => {
            throw new SyntaxError("Unexpected end of JSON input");
          },
        };
      }
      throw new Error(`unexpected URL: ${url}`);
    });

    const { Wrapper, queryClient } = createWrapper();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => usePasskeyAuthentication(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate();
    });

    // Assert: 204 でも成功扱いになる（Blocker 回帰防止）
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expect(result.current.isError).toBe(false);
    // session 交換が実際に fetch された
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/auth/session",
      expect.objectContaining({ method: "POST" }),
    );
    // AuthGuard 再判定のための invalidate
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["auth", "me"] });
  });
});
