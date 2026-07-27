import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

// 本ファイルは LoginPage → PasskeyButtons / PasskeySignupDialog → 各 hook を **実物のまま**
// 結線した component integration test（review #4 / Requirement 3.4）。hook 内部ロジック
// （registration chain の分類・registered 判定・Dialog の復旧 effect）が UI 上で正しく
// 連動することを検証する。外部境界（API / PKCE / WebAuthn 変換 / ブラウザ判定 / navigator）
// のみ mock 化し、それ以外はすべて実物を通す。

// ブラウザ側 WebAuthn 判定は常に true（capability の browser 軸を成立させる）。
vi.mock("@/lib/passkey-capability", () => ({
  isPasskeyBrowserSupported: vi.fn(() => true),
}));

// apiClient を mock し、capability / registration chain のサーバ応答を制御する。
// ApiError は実物を再 export し、hook 側の instanceof / status 判定を本番同等にする。
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

// PKCE / WebAuthn 変換は jsdom に実装がないため mock 化（値は参照透過に扱われる）。
vi.mock("@/lib/pkce", () => ({
  generatePkcePair: vi.fn(),
}));
vi.mock("@/lib/webauthn", () => ({
  decodeCreationOptions: vi.fn(() => ({ publicKey: {} })),
  decodeRequestOptions: vi.fn(() => ({ publicKey: {} })),
  encodeAttestationResponse: vi.fn(() => ({ id: "att" })),
  encodeAssertionResponse: vi.fn(() => ({ id: "asr" })),
}));

import { apiClient, ApiError } from "@/lib/api";
import { generatePkcePair } from "@/lib/pkce";
import { LoginPage } from "./login-page";

function renderLoginPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return render(<LoginPage />, { wrapper: Wrapper });
}

function makeCredential(id: string): PublicKeyCredential {
  return {
    id,
    type: "public-key",
    rawId: new Uint8Array([1, 2, 3]).buffer,
    response: {},
  } as unknown as PublicKeyCredential;
}

/**
 * registration chain（begin → create → finish → auth begin → get → auth finish → session）を
 * 走らせ、`/api/auth/session` の段で失敗させる apiClient.post mock を設定する。
 * これにより mutation は `session_exchange_failed` / registered=true となり、Dialog の
 * 復旧 effect が発火する（Requirement 3.4 の合流失敗経路）。
 */
function stubRegistrationChainFailingAtSession() {
  vi.mocked(apiClient.post).mockImplementation(
    (async (url: string) => {
      if (url === "/api/passkey/registration/begin") {
        return { challenge_id: "reg-1", options: { publicKey: {} } };
      }
      if (url === "/api/passkey/registration/finish") {
        return { user_id: "user-1" };
      }
      if (url === "/api/passkey/authentication/begin") {
        return { challenge_id: "auth-1", options: { publicKey: {} } };
      }
      if (url === "/api/passkey/authentication/finish") {
        return { auth_code: "code-1" };
      }
      if (url === "/api/auth/session") {
        // 合流失敗（作成後失敗）→ registered=true
        throw new ApiError(400, { code: "INVALID_GRANT" });
      }
      throw new Error(`unexpected URL: ${url}`);
    }) as typeof apiClient.post,
  );
}

describe("LoginPage 復旧フロー統合（review #4 / Requirement 3.4）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // capability: サーバ 200（available:true）× ブラウザ対応 → パスキー導線表示
    vi.mocked(apiClient.get).mockResolvedValue({ available: true });
    vi.mocked(generatePkcePair).mockResolvedValue({
      codeVerifier: "verifier",
      codeChallenge: "challenge",
    });
    // navigator.credentials は create / get とも成功させる（合流手前まで到達させる）
    vi.stubGlobal("navigator", {
      credentials: {
        create: vi.fn().mockResolvedValue(makeCredential("att")),
        get: vi.fn().mockResolvedValue(makeCredential("asr")),
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("アカウント作成が合流失敗したとき Dialog が閉じ、汎用復旧バナーが出て、パスキーログイン導線が使え、再度の新規作成で Dialog が即閉じしないこと", async () => {
    // Arrange
    stubRegistrationChainFailingAtSession();
    const user = userEvent.setup();
    renderLoginPage();

    // capability 解決 → パスキー導線が表示される
    const openCreateButton = await screen.findByRole("button", {
      name: "アカウント新規作成",
    });
    expect(
      screen.getByRole("button", { name: "パスキーでログイン" }),
    ).toBeInTheDocument();

    // Act 1: 新規作成 Dialog を開き、username を入力して作成
    await user.click(openCreateButton);
    const usernameInput = await screen.findByLabelText("ユーザー名");
    await user.type(usernameInput, "alice");
    await user.click(screen.getByRole("button", { name: "作成" }));

    // Assert 1: 合流失敗 → Dialog が閉じ（username 入力欄が消える）、汎用復旧バナーが出る
    await waitFor(() => {
      expect(
        screen.getByText(
          /アカウントは作成されましたが、ログイン処理に問題が発生しました/,
        ),
      ).toBeInTheDocument();
    });
    expect(screen.queryByLabelText("ユーザー名")).not.toBeInTheDocument();

    // Assert 2: パスキーログイン導線は引き続き利用可能（活性）
    const loginButton = screen.getByRole("button", {
      name: "パスキーでログイン",
    });
    expect(loginButton).toBeEnabled();

    // Act 2: 「アカウント新規作成」を再度押す（review #3 の再操作破綻がないことを確認）
    await user.click(
      screen.getByRole("button", { name: "アカウント新規作成" }),
    );

    // Assert 3: Dialog が開いたまま維持される（即座に閉じない）+ 復旧バナーは消える
    const reopenedInput = await screen.findByLabelText("ユーザー名");
    expect(reopenedInput).toBeInTheDocument();
    // 少し待っても閉じられない（残留 error による即閉じ回帰がないこと）
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.getByLabelText("ユーザー名")).toBeInTheDocument();
    expect(
      screen.queryByText(
        /アカウントは作成されましたが、ログイン処理に問題が発生しました/,
      ),
    ).not.toBeInTheDocument();
  });
});
