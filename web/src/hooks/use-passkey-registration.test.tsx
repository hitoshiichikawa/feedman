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

vi.mock("@/lib/pkce", () => ({
  generatePkcePair: vi.fn(),
}));

vi.mock("@/lib/webauthn", () => ({
  decodeCreationOptions: vi.fn(),
  encodeAttestationResponse: vi.fn(),
}));

import {
  usePasskeyRegistration,
} from "./use-passkey-registration";
import {
  apiClient,
  ApiError,
  RequestPreparationError,
  ResponseParseError,
} from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
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

function stubFinishFailure(error: unknown) {
  vi.mocked(apiClient.post).mockImplementation(
    (async (url: string) => {
      if (url === "/api/passkey/registration/begin") {
        return { challenge_id: "reg-1", options: { publicKey: {} } };
      }
      if (url === "/api/passkey/registration/finish") {
        throw error;
      }
      throw new Error(`unexpected URL: ${url}`);
    }) as typeof apiClient.post,
  );
}

describe("usePasskeyRegistration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(generatePkcePair).mockResolvedValue({
      codeVerifier: "unused-verifier",
      codeChallenge: "challenge",
    });
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

  it("begin → create → finish だけで完了し、二度目 ceremony と session 交換を呼ばない", async () => {
    const { create, get } = stubSuccessfulCreate();
    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          return { challenge_id: "reg-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/registration/finish") {
          return { user_id: "user-1" };
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );
    const { Wrapper, queryClient } = createWrapper();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate({ username: "alice" }));

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenNthCalledWith(
      1,
      "/api/passkey/registration/begin",
      { username: "alice", email: "", code_challenge: "challenge" },
    );
    expect(create).toHaveBeenCalledTimes(1);
    expect(apiClient.post).toHaveBeenNthCalledWith(
      2,
      "/api/passkey/registration/finish",
      {
        challenge_id: "reg-1",
        credential: expect.objectContaining({ id: "attestation-id" }),
      },
    );
    expect(apiClient.post).toHaveBeenCalledTimes(2);
    expect(get).not.toHaveBeenCalled();
    expect(apiClient.post).not.toHaveBeenCalledWith(
      expect.stringContaining("/authentication/"),
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["auth", "me"] });
  });

  it("begin の INVALID_USERNAME と 409 を既存 kind に分類する", async () => {
    const { Wrapper } = createWrapper();
    vi.mocked(apiClient.post).mockRejectedValueOnce(
      new ApiError(400, { code: "INVALID_USERNAME", internal: "secret" }),
    );
    const first = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    act(() => first.result.current.mutate({ username: "x" }));
    await waitFor(() => expect(first.result.current.isError).toBe(true));
    expect(first.result.current.error?.kind).toBe("invalid_username");
    expect(first.result.current.error?.message).not.toContain("secret");

    const secondWrapper = createWrapper();
    vi.mocked(apiClient.post).mockRejectedValueOnce(
      new ApiError(409, { code: "USERNAME_TAKEN" }),
    );
    const second = renderHook(() => usePasskeyRegistration(), {
      wrapper: secondWrapper.Wrapper,
    });
    act(() => second.result.current.mutate({ username: "alice" }));
    await waitFor(() => expect(second.result.current.isError).toBe(true));
    expect(second.result.current.error?.kind).toBe("username_taken");
  });

  it("create のユーザーキャンセルは cancelled のまま維持する", async () => {
    vi.stubGlobal("navigator", {
      credentials: {
        create: vi.fn().mockRejectedValue(makeDomException("NotAllowedError")),
      },
    });
    vi.mocked(apiClient.post).mockResolvedValue({
      challenge_id: "reg-1",
      options: { publicKey: {} },
    });
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate({ username: "alice" }));

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("cancelled");
    expect(result.current.error?.registered).toBe(false);
  });

  it.each([400, 401, 403, 404, 409, 422])(
    "finish の %i は拒否確定として server_rejected に分類する",
    async (status) => {
      stubSuccessfulCreate();
      stubFinishFailure(
        new ApiError(status, {
          code: "REGISTRATION_FAILED",
          internal_reason: "must-not-leak",
        }),
      );
      const { Wrapper } = createWrapper();
      const { result } = renderHook(() => usePasskeyRegistration(), {
        wrapper: Wrapper,
      });

      act(() => result.current.mutate({ username: "alice" }));

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error?.kind).toBe("server_rejected");
      expect(result.current.error?.registered).toBe(false);
      expect(result.current.error?.message).not.toContain("must-not-leak");
    },
  );

  it.each([
    ["fetch reject", new TypeError("Failed to fetch")],
    ["5xx", new ApiError(500, { code: "INTERNAL" })],
    ["AbortError", makeDomException("AbortError")],
    ["2xx parse failure", new ResponseParseError(200, new SyntaxError())],
  ])(
    "finish の %s は registration_uncertain に分類する",
    async (_label, failure) => {
      stubSuccessfulCreate();
      stubFinishFailure(failure);
      const { Wrapper } = createWrapper();
      const { result } = renderHook(() => usePasskeyRegistration(), {
        wrapper: Wrapper,
      });

      act(() => result.current.mutate({ username: "alice" }));

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(result.current.error?.kind).toBe("registration_uncertain");
      expect(result.current.error?.registered).toBe(true);
    },
  );

  it("finish dispatch 前の RequestPreparationError は server_error で uncertain にしない", async () => {
    stubSuccessfulCreate();
    stubFinishFailure(
      new RequestPreparationError(new TypeError("circular JSON")),
    );
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate({ username: "alice" }));

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("server_error");
    expect(result.current.error?.registered).toBe(false);
  });

  it("begin の plain TypeError は従来どおり network_error で uncertain にはしない", async () => {
    vi.mocked(apiClient.post).mockRejectedValueOnce(
      new TypeError("Failed to fetch"),
    );
    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    act(() => result.current.mutate({ username: "alice" }));

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.kind).toBe("network_error");
  });
});
