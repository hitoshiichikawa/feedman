import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { LoginPage } from "./login-page";

describe("LoginPage コンポーネント", () => {
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
});
