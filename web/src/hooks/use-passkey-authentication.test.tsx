import { renderHook, waitFor, act } from "@testing-library/react";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

// `@/lib/api` の apiClient をモックしてサーバ応答（200 / 4xx / 5xx / reject）を制御する。
// `ApiError` は実物を再 export し、hook 側の instanceof 判定がリアルな型で通るようにする。
vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>(
    "@/lib/api",
  );
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

// `@/lib/pkce` の `generatePkcePair` をモックして code_verifier / code_challenge の
// 生成結果を固定する（既知の verifier を session 交換の payload assertion に使う）。
vi.mock("@/lib/pkce", () => ({
  generatePkcePair: vi.fn(),
}));

// `@/lib/webauthn` の decode / encode を差し替え、hook 内の変換部を副作用なしにする。
// jsdom は WebAuthn 実装を持たないため、実物を呼ぶ意義が薄く mock 化が妥当（NFR 3.1）。
vi.mock("@/lib/webauthn", () => ({
  decodeRequestOptions: vi.fn(),
  encodeAssertionResponse: vi.fn(),
}));

import {
  PasskeyAuthError,
  usePasskeyAuthentication,
} from "./use-passkey-authentication";
import { apiClient, ApiError } from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
import {
  decodeRequestOptions,
  encodeAssertionResponse,
} from "@/lib/webauthn";

interface WrapperResult {
  Wrapper: ({ children }: { children: ReactNode }) => React.ReactElement;
  queryClient: QueryClient;
}

/**
 * `QueryClientProvider` を毎テスト新規に作るラッパ。invalidateQueries の spy 用に
 * queryClient も返す（既存 `use-manual-refresh.test.tsx` と同 idiom）。
 */
function createWrapper(): WrapperResult {
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

/**
 * `PublicKeyCredential` 相当のダミー assertion オブジェクト。encode 関数を mock 化
 * しているため、hook はフィールドを見ず参照透過に扱える。
 */
function makeAssertionCredential(): PublicKeyCredential {
  return {
    id: "cred-id",
    type: "public-key",
    rawId: new Uint8Array([1, 2, 3]).buffer,
    response: {},
  } as unknown as PublicKeyCredential;
}

/**
 * DOMException が存在する環境では `new DOMException(msg, name)` を、
 * そうでない環境では `.name` を持つ `Error` を返す（jsdom は DOMException を持つ）。
 */
function makeCancelDomException(name: string): Error {
  if (typeof DOMException !== "undefined") {
    return new DOMException("cancelled", name);
  }
  const err = new Error("cancelled");
  err.name = name;
  return err;
}

describe("usePasskeyAuthentication", () => {
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
    // navigator を stub したケースを次テストに漏らさない
    vi.unstubAllGlobals();
  });

  it("正常系: PKCE → begin → get → finish → session の順に呼び、成功時に invalidateQueries が呼ばれること", async () => {
    // Arrange
    const cred = makeAssertionCredential();
    const credentialsGet = vi.fn().mockResolvedValue(cred);
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/authentication/finish") {
          return { auth_code: "auth-1" };
        }
        if (url === "/api/auth/session") {
          return undefined;
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper, queryClient } = createWrapper();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => usePasskeyAuthentication(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate();
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // begin: code_challenge が pkce 生成結果と一致
    expect(apiClient.post).toHaveBeenNthCalledWith(
      1,
      "/api/passkey/authentication/begin",
      { code_challenge: "test-challenge" },
    );
    // navigator.credentials.get が decodeRequestOptions の返却をそのまま受け取る
    expect(credentialsGet).toHaveBeenCalledTimes(1);
    expect(decodeRequestOptions).toHaveBeenCalledWith({ publicKey: {} });
    // finish: begin の challenge_id と encodeAssertionResponse の返却を送る
    expect(apiClient.post).toHaveBeenNthCalledWith(
      2,
      "/api/passkey/authentication/finish",
      expect.objectContaining({
        challenge_id: "chal-1",
        credential: expect.objectContaining({ id: "cred-id" }),
      }),
    );
    // session: finish の auth_code と pkce の code_verifier を送る
    expect(apiClient.post).toHaveBeenNthCalledWith(
      3,
      "/api/auth/session",
      { auth_code: "auth-1", code_verifier: "test-verifier" },
    );
    // AuthGuard を再判定させる invalidate
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["auth", "me"] });
  });

  it("cancelled: navigator.credentials.get が NotAllowedError を throw したときエラー分類が cancelled になること（Req 4.5）", async () => {
    // Arrange
    const credentialsGet = vi
      .fn()
      .mockRejectedValue(makeCancelDomException("NotAllowedError"));
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "chal-1", options: { publicKey: {} } };
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAuthentication(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate();
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error).toBeInstanceOf(PasskeyAuthError);
    expect(result.current.error?.kind).toBe("cancelled");
    // finish / session は呼ばれない
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/authentication/finish",
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });

  it("server_rejected: authentication/finish が 400 AUTHENTICATION_FAILED を返したとき server_rejected に分類され、session 交換に進まないこと（Req 4.6）", async () => {
    // Arrange
    const cred = makeAssertionCredential();
    const credentialsGet = vi.fn().mockResolvedValue(cred);
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/authentication/finish") {
          throw new ApiError(400, { code: "AUTHENTICATION_FAILED" });
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAuthentication(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate();
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("server_rejected");
    // NFR 1.2: サーバ拒否理由の内部詳細（"AUTHENTICATION_FAILED"）が message に反射されない
    expect(result.current.error?.message).not.toContain("AUTHENTICATION_FAILED");
    // session 交換に進まない
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });

  it("session_exchange_failed: /api/auth/session が 400 INVALID_GRANT を返したとき step 5 の失敗として session_exchange_failed に分類されること（Req 4.7）", async () => {
    // Arrange
    const cred = makeAssertionCredential();
    const credentialsGet = vi.fn().mockResolvedValue(cred);
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/authentication/finish") {
          return { auth_code: "auth-1" };
        }
        if (url === "/api/auth/session") {
          // 400 INVALID_GRANT を返し、step 5 の失敗のみを session_exchange_failed に振り分ける
          // 判定分岐を検証する（step 4 で同じ 400 が来た場合との対比）
          throw new ApiError(400, { code: "INVALID_GRANT" });
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAuthentication(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate();
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("session_exchange_failed");
    // session が実際に呼ばれた（step 5 に到達した）ことを確認
    expect(apiClient.post).toHaveBeenCalledWith("/api/auth/session", {
      auth_code: "auth-1",
      code_verifier: "test-verifier",
    });
  });

  it("server_error: authentication/begin が 500 を返したとき server_error に分類され、以降の chain に進まないこと（Req 4.7）", async () => {
    // Arrange
    // begin が 500 で失敗するため、navigator.credentials.get は呼ばれないが navigator の
    // shape を保つため stub しておく（未 stub のまま begin で失敗するのが理想だが、
    // hook 内で decodeRequestOptions 到達前に throw するため実質未使用）。
    vi.stubGlobal("navigator", { credentials: { get: vi.fn() } });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/authentication/begin") {
          throw new ApiError(500, null);
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAuthentication(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate();
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("server_error");
    // 以降のいずれの endpoint も呼ばれない
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/authentication/finish",
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });
});
