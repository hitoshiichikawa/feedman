import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Button } from "@/components/ui/button";

/**
 * shadcn/ui Button コンポーネントのテスト
 *
 * Task 9.1: shadcn/ui コンポーネントが正しく導入されていることを検証する
 */
describe("Button コンポーネント (shadcn/ui)", () => {
  it("デフォルトのボタンをレンダリングする", () => {
    render(<Button>テストボタン</Button>);
    const button = screen.getByRole("button", { name: "テストボタン" });
    expect(button).toBeInTheDocument();
  });

  it("data-slot 属性が設定される", () => {
    render(<Button>テスト</Button>);
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("data-slot", "button");
  });

  it("variant プロパティを受け付ける", () => {
    render(<Button variant="destructive">削除</Button>);
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("data-variant", "destructive");
  });

  it("size プロパティを受け付ける", () => {
    render(<Button size="sm">小さいボタン</Button>);
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("data-size", "sm");
  });

  it("カスタム className を適用できる", () => {
    render(<Button className="custom-class">カスタム</Button>);
    const button = screen.getByRole("button");
    expect(button).toHaveClass("custom-class");
  });

  it("disabled 状態を反映する", () => {
    render(<Button disabled>無効ボタン</Button>);
    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });
});
