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

// `@/lib/api` の apiClient をモックしてサーバ応答（200 / 400 / 409 / 500 / reject）を制御する。
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
  decodeCreationOptions: vi.fn(),
  decodeRequestOptions: vi.fn(),
  encodeAttestationResponse: vi.fn(),
  encodeAssertionResponse: vi.fn(),
}));

import {
  PasskeyRegistrationError,
  usePasskeyRegistration,
} from "./use-passkey-registration";
import { apiClient, ApiError } from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
import {
  decodeCreationOptions,
  decodeRequestOptions,
  encodeAssertionResponse,
  encodeAttestationResponse,
} from "@/lib/webauthn";

interface WrapperResult {
  Wrapper: ({ children }: { children: ReactNode }) => React.ReactElement;
  queryClient: QueryClient;
}

/**
 * `QueryClientProvider` を毎テスト新規に作るラッパ。invalidateQueries の spy 用に
 * queryClient も返す（既存 `use-manual-refresh.test.tsx` / task 7 テストと同 idiom）。
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
 * `PublicKeyCredential` 相当のダミーオブジェクト。encode 関数を mock 化しているため、
 * hook はフィールドを見ず参照透過に扱える。
 */
function makeCredential(id: string): PublicKeyCredential {
  return {
    id,
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

describe("usePasskeyRegistration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(generatePkcePair).mockResolvedValue({
      codeVerifier: "test-verifier",
      codeChallenge: "test-challenge",
    });
    vi.mocked(decodeCreationOptions).mockReturnValue({
      publicKey: {
        challenge: new Uint8Array([1]).buffer,
      } as unknown as PublicKeyCredentialCreationOptions,
    });
    vi.mocked(decodeRequestOptions).mockReturnValue({
      publicKey: {
        challenge: new Uint8Array([1]).buffer,
      } as unknown as PublicKeyCredentialRequestOptions,
    });
    vi.mocked(encodeAttestationResponse).mockReturnValue({
      id: "attestation-id",
      type: "public-key",
      rawId: "AQID",
      response: {
        clientDataJSON: "cdj",
        attestationObject: "att",
      },
    });
    vi.mocked(encodeAssertionResponse).mockReturnValue({
      id: "assertion-id",
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

  it("正常系: 登録 → 認証 → session の全 API を順序どおり呼び、invalidateQueries が呼ばれること（Req 2.2, 2.3, 2.4, 3.1, 3.2）", async () => {
    // Arrange
    const attestationCred = makeCredential("attestation-id");
    const assertionCred = makeCredential("assertion-id");
    const credentialsCreate = vi.fn().mockResolvedValue(attestationCred);
    const credentialsGet = vi.fn().mockResolvedValue(assertionCred);
    vi.stubGlobal("navigator", {
      credentials: { create: credentialsCreate, get: credentialsGet },
    });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          return { challenge_id: "reg-chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/registration/finish") {
          return { user_id: "user-1" };
        }
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "auth-chal-1", options: { publicKey: {} } };
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
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate({ username: "alice" });
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // registration/begin: username + email:"" + code_challenge を送る（Req 2.4）
    expect(apiClient.post).toHaveBeenNthCalledWith(
      1,
      "/api/passkey/registration/begin",
      { username: "alice", email: "", code_challenge: "test-challenge" },
    );
    // navigator.credentials.create が decodeCreationOptions の返却を受け取る
    expect(credentialsCreate).toHaveBeenCalledTimes(1);
    expect(decodeCreationOptions).toHaveBeenCalledWith({ publicKey: {} });
    // registration/finish: begin の challenge_id と encodeAttestationResponse の返却を送る
    expect(apiClient.post).toHaveBeenNthCalledWith(
      2,
      "/api/passkey/registration/finish",
      expect.objectContaining({
        challenge_id: "reg-chal-1",
        credential: expect.objectContaining({ id: "attestation-id" }),
      }),
    );
    // authentication/begin: 同じ code_challenge を再送
    expect(apiClient.post).toHaveBeenNthCalledWith(
      3,
      "/api/passkey/authentication/begin",
      { code_challenge: "test-challenge" },
    );
    // navigator.credentials.get が decodeRequestOptions の返却を受け取る
    expect(credentialsGet).toHaveBeenCalledTimes(1);
    // authentication/finish: authentication/begin の challenge_id と assertion を送る
    expect(apiClient.post).toHaveBeenNthCalledWith(
      4,
      "/api/passkey/authentication/finish",
      expect.objectContaining({
        challenge_id: "auth-chal-1",
        credential: expect.objectContaining({ id: "assertion-id" }),
      }),
    );
    // session: authentication/finish の auth_code と pkce の code_verifier を送る
    expect(apiClient.post).toHaveBeenNthCalledWith(
      5,
      "/api/auth/session",
      { auth_code: "auth-1", code_verifier: "test-verifier" },
    );
    // AuthGuard を再判定させる invalidate
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["auth", "me"] });
  });

  it("invalid_username: registration/begin が 400 code=INVALID_USERNAME を返したとき invalid_username に分類され、以降の chain に進まないこと（Req 2.5）", async () => {
    // Arrange
    // begin が失敗するため create / get は呼ばれないが navigator の shape を保つため stub
    vi.stubGlobal("navigator", {
      credentials: { create: vi.fn(), get: vi.fn() },
    });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          throw new ApiError(400, { code: "INVALID_USERNAME" });
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate({ username: "bad!!!name" });
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error).toBeInstanceOf(PasskeyRegistrationError);
    expect(result.current.error?.kind).toBe("invalid_username");
    // NFR 1.2: サーバの内部コード "INVALID_USERNAME" が message に反射されない
    expect(result.current.error?.message).not.toContain("INVALID_USERNAME");
    // 以降の endpoint は呼ばれない
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/registration/finish",
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });

  it("username_taken: registration/begin が 409 を返したとき username_taken に分類され、以降の chain に進まないこと（Req 2.6）", async () => {
    // Arrange
    vi.stubGlobal("navigator", {
      credentials: { create: vi.fn(), get: vi.fn() },
    });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          throw new ApiError(409, { code: "USERNAME_TAKEN" });
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate({ username: "taken" });
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("username_taken");
    // NFR 1.2: サーバの内部コード "USERNAME_TAKEN" が message に反射されない
    expect(result.current.error?.message).not.toContain("USERNAME_TAKEN");
    // 以降の endpoint は呼ばれない
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/registration/finish",
      expect.anything(),
    );
  });

  it("cancelled: navigator.credentials.create が NotAllowedError を throw したときエラー分類が cancelled になること（Req 2.7）", async () => {
    // Arrange
    const credentialsCreate = vi
      .fn()
      .mockRejectedValue(makeCancelDomException("NotAllowedError"));
    vi.stubGlobal("navigator", {
      credentials: { create: credentialsCreate, get: vi.fn() },
    });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          return { challenge_id: "reg-chal-1", options: { publicKey: {} } };
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate({ username: "alice" });
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("cancelled");
    // registration/finish 以降は呼ばれない
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/registration/finish",
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });

  it("server_rejected: registration/finish が 400 REGISTRATION_FAILED を返したとき server_rejected に分類され、認証 chain に進まないこと（Req 2.8, NFR 1.2）", async () => {
    // Arrange
    const attestationCred = makeCredential("attestation-id");
    const credentialsCreate = vi.fn().mockResolvedValue(attestationCred);
    const credentialsGet = vi.fn();
    vi.stubGlobal("navigator", {
      credentials: { create: credentialsCreate, get: credentialsGet },
    });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          return { challenge_id: "reg-chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/registration/finish") {
          throw new ApiError(400, { code: "REGISTRATION_FAILED" });
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate({ username: "alice" });
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("server_rejected");
    // NFR 1.2: サーバ拒否理由の内部詳細（"REGISTRATION_FAILED"）が message に反射されない
    expect(result.current.error?.message).not.toContain("REGISTRATION_FAILED");
    // 認証 chain は起動しない
    expect(credentialsGet).not.toHaveBeenCalled();
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/passkey/authentication/begin",
      expect.anything(),
    );
    expect(apiClient.post).not.toHaveBeenCalledWith(
      "/api/auth/session",
      expect.anything(),
    );
  });

  it("session_exchange_failed: /api/auth/session が 400 INVALID_GRANT を返したとき step 8 の失敗として session_exchange_failed に分類されること（Req 3.4）", async () => {
    // Arrange
    const attestationCred = makeCredential("attestation-id");
    const assertionCred = makeCredential("assertion-id");
    const credentialsCreate = vi.fn().mockResolvedValue(attestationCred);
    const credentialsGet = vi.fn().mockResolvedValue(assertionCred);
    vi.stubGlobal("navigator", {
      credentials: { create: credentialsCreate, get: credentialsGet },
    });

    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          return { challenge_id: "reg-chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/registration/finish") {
          return { user_id: "user-1" };
        }
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "auth-chal-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/authentication/finish") {
          return { auth_code: "auth-1" };
        }
        if (url === "/api/auth/session") {
          // 400 INVALID_GRANT を返し、step 8 の失敗のみを session_exchange_failed に
          // 振り分ける判定分岐を検証する（step 4 で同じ 400 が来た場合との対比で、
          // 上の "server_rejected" ケースと組み合わせて step index 追跡の正しさを担保）
          throw new ApiError(400, { code: "INVALID_GRANT" });
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );

    const { Wrapper } = createWrapper();
    const { result } = renderHook(() => usePasskeyRegistration(), {
      wrapper: Wrapper,
    });

    // Act
    await act(async () => {
      result.current.mutate({ username: "alice" });
    });

    // Assert
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.kind).toBe("session_exchange_failed");
    // session が実際に呼ばれた（step 8 に到達した）ことを確認
    expect(apiClient.post).toHaveBeenCalledWith("/api/auth/session", {
      auth_code: "auth-1",
      code_verifier: "test-verifier",
    });
  });
});
