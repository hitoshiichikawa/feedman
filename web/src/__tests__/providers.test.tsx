import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { useQueryClient } from "@tanstack/react-query";
import { Providers } from "@/components/providers";

/**
 * Providers コンポーネントのテスト
 *
 * Task 9.1: QueryClientProvider を含む Providers コンポーネントが
 * 正しく設定されていることを検証する
 */

// QueryClient が利用可能かテストするためのヘルパーコンポーネント
function QueryClientConsumer() {
  const queryClient = useQueryClient();
  return (
    <div data-testid="query-client-available">
      {queryClient ? "QueryClient利用可能" : "QueryClient未設定"}
    </div>
  );
}

describe("Providers コンポーネント", () => {
  it("子コンポーネントを正しくレンダリングする", () => {
    render(
      <Providers>
        <div data-testid="child">テスト子要素</div>
      </Providers>
    );

    expect(screen.getByTestId("child")).toBeInTheDocument();
    expect(screen.getByTestId("child")).toHaveTextContent("テスト子要素");
  });

  it("QueryClientProvider を通じて QueryClient が利用可能である", () => {
    render(
      <Providers>
        <QueryClientConsumer />
      </Providers>
    );

    expect(screen.getByTestId("query-client-available")).toHaveTextContent(
      "QueryClient利用可能"
    );
  });

  it("QueryClientProvider なしでは QueryClient が利用できずエラーになる", () => {
    // QueryClientProvider でラップしていない場合はエラーが発生することを確認
    expect(() => {
      render(<QueryClientConsumer />);
    }).toThrow();
  });
});
