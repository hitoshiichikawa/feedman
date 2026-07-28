import { render, screen } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";

// LoginPage は PasskeyButtons（`usePasskeyCapability` / `usePasskeyAuthentication` 経由で
// QueryClient に依存）と PasskeySignupDialog（`usePasskeyRegistration` 経由で QueryClient
// に依存）を render するため、bare `render(<LoginPage />)` では QueryClient 不在で例外を
// 投げる。既存 4 テスト（Google 導線）を render 呼び出しごと不変（Requirement 6.1 / 6.4）
// で維持するため、3 hook をモジュール冒頭で `vi.mock` して QueryClient 依存を除去する。
// 実物の PasskeyButtons / PasskeySignupDialog は render される（hook のみ差し替え）。
vi.mock("@/hooks/use-passkey-capability", () => ({
  usePasskeyCapability: vi.fn(() => ({ isLoading: false, available: false })),
}));

vi.mock("@/hooks/use-passkey-authentication", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/use-passkey-authentication")
  >("@/hooks/use-passkey-authentication");
  return {
    ...actual,
    usePasskeyAuthentication: vi.fn(() => ({
      mutate: vi.fn(),
      reset: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })),
  };
});

vi.mock("@/hooks/use-passkey-registration", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/use-passkey-registration")
  >("@/hooks/use-passkey-registration");
  return {
    ...actual,
    usePasskeyRegistration: vi.fn(() => ({
      mutate: vi.fn(),
      reset: vi.fn(),
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    })),
  };
});

import { usePasskeyCapability } from "@/hooks/use-passkey-capability";
import { LoginPage } from "./login-page";

describe("LoginPage コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 既存テスト維持のためデフォルトを capability false（Google 単体構成）に固定する。
    vi.mocked(usePasskeyCapability).mockReturnValue({
      isLoading: false,
      available: false,
    });
  });

  it("Googleアカウントでのログインボタンが表示されること", () => {
    render(<LoginPage />);

    const loginButton = screen.getByRole("link", {
      name: /Googleアカウントでログイン/,
    });
    expect(loginButton).toBeInTheDocument();
  });

  it("ログインボタンがOAuthエンドポイントにリンクしていること", () => {
    render(<LoginPage />);

    const loginButton = screen.getByRole("link", {
      name: /Googleアカウントでログイン/,
    });
    expect(loginButton).toHaveAttribute("href", "/auth/google/login");
  });

  it("アプリケーション名が表示されること", () => {
    render(<LoginPage />);

    expect(screen.getByText("Feedman")).toBeInTheDocument();
  });

  it("アプリケーションの説明が表示されること", () => {
    render(<LoginPage />);

    expect(
      screen.getByText(/RSS\/Atom フィードリーダー/)
    ).toBeInTheDocument();
  });

  it("capability が available: true のときパスキー導線（PasskeyButtons）が render されること（Req 1.1, 1.3, 1.4）", () => {
    // Arrange
    vi.mocked(usePasskeyCapability).mockReturnValue({
      isLoading: false,
      available: true,
    });

    // Act
    render(<LoginPage />);

    // Assert
    expect(
      screen.getByRole("button", { name: "パスキーでログイン" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "アカウント新規作成" }),
    ).toBeInTheDocument();
  });

  it("capability が available: false のとき Google 導線のみ表示され、パスキー導線が DOM に存在しないこと（Req 5.3, 6.1, 6.4）", () => {
    // Arrange
    vi.mocked(usePasskeyCapability).mockReturnValue({
      isLoading: false,
      available: false,
    });

    // Act
    render(<LoginPage />);

    // Assert: Google 導線は本 spec 導入前と同一に維持
    expect(
      screen.getByRole("link", { name: /Googleアカウントでログイン/ }),
    ).toBeInTheDocument();
    // Assert: パスキー導線は DOM に存在しない（`PasskeyButtons` が null 返却）
    expect(
      screen.queryByRole("button", { name: "パスキーでログイン" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "アカウント新規作成" }),
    ).not.toBeInTheDocument();
  });
});
