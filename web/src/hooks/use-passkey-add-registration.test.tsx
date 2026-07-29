import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

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

vi.mock("@/lib/webauthn", () => ({
  decodeCreationOptions: vi.fn(),
  encodeAttestationResponse: vi.fn(),
}));

import { usePasskeyAddRegistration } from "./use-passkey-add-registration";
import {
  apiClient,
  ApiError,
  RequestPreparationError,
  ResponseParseError,
} from "@/lib/api";
import {
  decodeCreationOptions,
  encodeAttestationResponse,
} from "@/lib/webauthn";

function createWrapper() {
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

function makeCredential(): PublicKeyCredential {
  return {
    id: "attestation-id",
    type: "public-key",
    rawId: new Uint8Array([1, 2, 3]).buffer,
    response: {},
  } as unknown as PublicKeyCredential;
}

function makeDomException(name: string): Error {
  if (typeof DOMException !== "undefined") {
    return new DOMException("browser operation failed", name);
  }
  const error = new Error("browser operation failed");
  error.name = name;
  return error;
}

function stubSuccessfulCreate() {
  const create = vi.fn().mockResolvedValue(makeCredential());
  const get = vi.fn();
  vi.stubGlobal("navigator", { credentials: { create, get } });
  return { create, get };
}

function stubBeginSuccess() {
  vi.mocked(apiClient.post).mockImplementation(
    (async (url: string) => {
      if (url === "/api/passkey/registration/add/begin") {
        return { challenge_id: "add-1", options: { publicKey: {} } };
      }
      if (url === "/api/passkey/registration/add/finish") {
        // 204 No Content: apiClient は空 body で undefined を返す契約
        return undefined;
      }
      throw new Error(`unexpected URL: ${url}`);
    }) as typeof apiClient.post,
  );
}

function stubFinishFailure(error: unknown) {
  vi.mocked(apiClient.post).mockImplementation(
    (async (url: string) => {
      if (url === "/api/passkey/registration/add/begin") {
        return { challenge_id: "add-1", options: { publicKey: {} } };
      }
      if (url === "/api/passkey/registration/add/finish") {
        throw error;
      }
      throw new Error(`unexpected URL: ${url}`);
    }) as typeof apiClient.post,
  );
}

describe("usePasskeyAddRegistration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(decodeCreationOptions).mockReturnValue({
      publicKey: {
        challenge: new Uint8Array([1]).buffer,
      } as unknown as PublicKeyCredentialCreationOptions,
    });
    vi.mocked(encodeAttestationResponse).mockReturnValue({
      id: "attestation-id",
      type: "public-key",
      rawId: "AQID",
      response: {
        clientDataJSON: "client-data",
        attestationObject: "attestation",
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("begin→create→finish で成功し、認証系ceremony を呼ばず、begin は空ボディで送信されること (Req 2.1, 2.2, 2.3, 2.4)", async () => {
    const { create, get } = stubSuccessfulCreate();
    stubBeginSuccess();
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // begin は空ボディ (Req 2.1: 追加登録用チャレンジ取得は空ボディ)
    expect(apiClient.post).toHaveBeenNthCalledWith(
      1,
      "/api/passkey/registration/add/begin",
      {},
    );
    // ブラウザのパスキー作成 UI が起動される (Req 2.2)
    expect(create).toHaveBeenCalledTimes(1);
    // 生成された credential を追加登録用の確定処理へ提示 (Req 2.3)
    expect(apiClient.post).toHaveBeenNthCalledWith(
      2,
      "/api/passkey/registration/add/finish",
      {
        challenge_id: "add-1",
        credential: expect.objectContaining({ id: "attestation-id" }),
      },
    );
    expect(apiClient.post).toHaveBeenCalledTimes(2);
    // 認証系 endpoint は呼ばれない（追加登録は session を変えない / Req 2.5）
    expect(get).not.toHaveBeenCalled();
    expect(apiClient.post).not.toHaveBeenCalledWith(
      expect.stringContaining("/authentication/"),
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });

  it("同一 authenticator の重複時（InvalidStateError）は already_registered に分類し finish を呼ばないこと (Req 3.1, 3.2, 3.4)", async () => {
    const create = vi
      .fn()
      .mockRejectedValue(makeDomException("InvalidStateError"));
    vi.stubGlobal("navigator", { credentials: { create } });
    // begin だけ通り、finish は呼ばれてはならない
    vi.mocked(apiClient.post).mockResolvedValue({
      challenge_id: "add-1",
      options: { publicKey: {} },
    });
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    // 重複は cancelled とは別 kind として分類される (Req 3.4)
    expect(result.current.error?.kind).toBe("already_registered");
    // finish endpoint は呼ばれない (Req 3.2)
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/registration/add/finish",
      expect.anything(),
    );
    // begin のみ呼ばれる
    expect(apiClient.post).toHaveBeenCalledTimes(1);
    expect(apiClient.post).toHaveBeenCalledWith(
      "/api/passkey/registration/add/begin",
      {},
    );
  });

  it.each([
    ["NotAllowedError", "NotAllowedError"],
    ["AbortError", "AbortError"],
  ])(
    "create のユーザーキャンセル（%s）は cancelled として分類し finish を呼ばないこと (Req 4.1)",
    async (_label, name) => {
      const create = vi.fn().mockRejectedValue(makeDomException(name));
      vi.stubGlobal("navigator", { credentials: { create } });
      vi.mocked(apiClient.post).mockResolvedValue({
        challenge_id: "add-1",
        options: { publicKey: {} },
      });
      const { Wrapper } = createWrapper();
      const { result } = renderHook(() => usePasskeyAddRegistration(), {
        wrapper: Wrapper,
      });

      act(() => result.current.mutate());

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error?.kind).toBe("cancelled");
      expect(apiClient.post).not.toHaveBeenCalledWith(
        "/api/passkey/registration/add/finish",
        expect.anything(),
      );
    },
  );

  it("create が null を返した場合も cancelled として集約されること (Req 4.1)", async () => {
    const create = vi.fn().mockResolvedValue(null);
    vi.stubGlobal("navigator", { credentials: { create } });
    vi.mocked(apiClient.post).mockResolvedValue({
      challenge_id: "add-1",
      options: { publicKey: {} },
    });
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("cancelled");
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/registration/add/finish",
      expect.anything(),
    );
  });

  it.each([400, 401, 403, 409, 422])(
    "finish の %i は server_rejected として分類され、内部 body が message に混入しないこと (Req 4.2, NFR 2.2)",
    async (status) => {
      stubSuccessfulCreate();
      stubFinishFailure(
        new ApiError(status, {
          code: "REGISTRATION_FAILED",
          internal_reason: "must-not-leak-secret",
        }),
      );
      const { Wrapper } = createWrapper();
      const { result } = renderHook(() => usePasskeyAddRegistration(), {
        wrapper: Wrapper,
      });

      act(() => result.current.mutate());

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error?.kind).toBe("server_rejected");
      // 内部詳細は message に反射しない
      expect(result.current.error?.message).not.toContain("must-not-leak-secret");
      expect(result.current.error?.message).not.toContain("REGISTRATION_FAILED");
    },
  );

  it("finish の 5xx は server_error として分類されること (Req 4.3)", async () => {
    stubSuccessfulCreate();
    stubFinishFailure(new ApiError(500, { code: "INTERNAL" }));
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_error");
  });

  it("finish の fetch reject（TypeError）は network_error として分類されること (Req 4.3)", async () => {
    stubSuccessfulCreate();
    stubFinishFailure(new TypeError("Failed to fetch"));
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("network_error");
  });

  it("finish の AbortError は network_error として分類されること (Req 4.3)", async () => {
    stubSuccessfulCreate();
    stubFinishFailure(makeDomException("AbortError"));
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("network_error");
  });

  it("finish の RequestPreparationError は server_error として分類されること", async () => {
    stubSuccessfulCreate();
    stubFinishFailure(new RequestPreparationError(new TypeError("circular JSON")));
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_error");
  });

  it("finish の ResponseParseError は server_error として分類されること", async () => {
    stubSuccessfulCreate();
    stubFinishFailure(new ResponseParseError(200, new SyntaxError()));
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_error");
  });

  it("begin の fetch reject（TypeError）は network_error として分類されること (Req 4.3)", async () => {
    vi.mocked(apiClient.post).mockRejectedValueOnce(
      new TypeError("Failed to fetch"),
    );
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("network_error");
  });

  it("begin の 4xx は server_rejected として分類されること", async () => {
    vi.mocked(apiClient.post).mockRejectedValueOnce(
      new ApiError(400, { code: "INVALID_REQUEST", internal: "leak" }),
    );
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_rejected");
    expect(result.current.error?.message).not.toContain("leak");
  });

  it("begin の 5xx は server_error として分類されること", async () => {
    vi.mocked(apiClient.post).mockRejectedValueOnce(
      new ApiError(500, { code: "INTERNAL_ERROR" }),
    );
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_error");
  });

  it("decodeCreationOptions が TypeError を投げた場合は server_error として分類されること", async () => {
    // begin は成功、decode で TypeError
    vi.mocked(apiClient.post).mockResolvedValueOnce({
      challenge_id: "add-1",
      options: { publicKey: {} },
    });
    vi.mocked(decodeCreationOptions).mockImplementationOnce(() => {
      throw new TypeError("malformed options");
    });
    // navigator は使われないが、undefined 参照を避けるためスタブする
    vi.stubGlobal("navigator", { credentials: { create: vi.fn() } });
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_error");
    // finish は呼ばれない
    expect(apiClient.post).toHaveBeenCalledTimes(1);
  });

  it("成功時に auth/me query を invalidate しないこと（セッション維持 / Req 2.5）", async () => {
    stubSuccessfulCreate();
    stubBeginSuccess();
    const { Wrapper, queryClient } = createWrapper();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => usePasskeyAddRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // auth/me を無効化しない（他画面の再描画・既存機能を変化させない）
    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ["auth", "me"] });
  });
});
