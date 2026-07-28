import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/passkey-capability", () => ({
  isPasskeyBrowserSupported: vi.fn(() => true),
}));

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
  decodeCreationOptions: vi.fn(() => ({ publicKey: {} })),
  decodeRequestOptions: vi.fn(() => ({ publicKey: {} })),
  encodeAttestationResponse: vi.fn(() => ({ id: "attestation" })),
  encodeAssertionResponse: vi.fn(() => ({ id: "assertion" })),
}));

import { AuthGuard } from "./auth-guard";
import { apiClient, ApiError } from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";

function makeCredential(id: string): PublicKeyCredential {
  return {
    id,
    type: "public-key",
    rawId: new Uint8Array([1, 2, 3]).buffer,
    response: {},
  } as unknown as PublicKeyCredential;
}

function renderAuthFlow() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return render(
    <AuthGuard>
      <div data-testid="two-pane-ui">2 ペイン UI</div>
    </AuthGuard>,
    { wrapper: Wrapper },
  );
}

describe("登録完了不明からの復旧フロー統合", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(generatePkcePair).mockResolvedValue({
      codeVerifier: "verifier",
      codeChallenge: "challenge",
    });
    vi.stubGlobal("navigator", {
      credentials: {
        create: vi.fn().mockResolvedValue(makeCredential("attestation")),
        get: vi.fn().mockResolvedValue(makeCredential("assertion")),
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("registration_uncertain → ログイン確認成功 → auth/me 再判定で 2 ペイン UI に到達する", async () => {
    let sessionEstablished = false;
    vi.mocked(apiClient.get).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/capability") {
          return { available: true };
        }
        if (url === "/auth/me") {
          if (!sessionEstablished) {
            throw new ApiError(401, { code: "UNAUTHORIZED" });
          }
          return {
            id: "user-1",
            email: "",
            name: "alice",
            created_at: "2026-07-28T00:00:00Z",
          };
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.get,
    );
    vi.mocked(apiClient.post).mockImplementation(
      (async (url: string) => {
        if (url === "/api/passkey/registration/begin") {
          return { challenge_id: "reg-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/registration/finish") {
          throw new TypeError("response lost after dispatch");
        }
        if (url === "/api/passkey/authentication/begin") {
          return { challenge_id: "auth-1", options: { publicKey: {} } };
        }
        if (url === "/api/passkey/authentication/finish") {
          return { auth_code: "auth-code" };
        }
        if (url === "/api/auth/session") {
          sessionEstablished = true;
          return undefined;
        }
        throw new Error(`unexpected URL: ${url}`);
      }) as typeof apiClient.post,
    );
    const user = userEvent.setup();
    renderAuthFlow();

    await user.click(
      await screen.findByRole("button", { name: "アカウント新規作成" }),
    );
    await user.type(await screen.findByLabelText("ユーザー名"), "alice");
    await user.click(screen.getByRole("button", { name: "作成" }));

    expect(
      await screen.findByRole("heading", {
        name: "登録が完了したかどうかを確認できませんでした",
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "再度作成する" }),
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "ログインで確認する" }),
    );

    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith("/api/auth/session", {
        auth_code: "auth-code",
        code_verifier: "verifier",
      });
    });
    expect(
      await screen.findByTestId("two-pane-ui"),
    ).toBeInTheDocument();
  });
});
