import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ErrorDisplay } from "./error-display";

describe("ErrorDisplay コンポーネント", () => {
  it("原因カテゴリが表示されること", () => {
    render(
      <ErrorDisplay
        category="feed"
        message="フィードの取得に失敗しました"
        action="しばらく時間をおいてから再度お試しください"
      />
    );

    expect(
      screen.getByText("フィードの取得に失敗しました")
    ).toBeInTheDocument();
  });

  it("対処方法が表示されること", () => {
    render(
      <ErrorDisplay
        category="feed"
        message="フィードの取得に失敗しました"
        action="しばらく時間をおいてから再度お試しください"
      />
    );

    expect(
      screen.getByText("しばらく時間をおいてから再度お試しください")
    ).toBeInTheDocument();
  });

  it("role=alertが設定されていること", () => {
    render(
      <ErrorDisplay
        category="system"
        message="システムエラーが発生しました"
        action="管理者に連絡してください"
      />
    );

    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("カテゴリに応じたラベルが表示されること（認証エラー）", () => {
    render(
      <ErrorDisplay
        category="auth"
        message="認証に失敗しました"
        action="再度ログインしてください"
      />
    );

    expect(screen.getByTestId("error-category")).toHaveTextContent("認証エラー");
  });

  it("カテゴリに応じたラベルが表示されること（バリデーションエラー）", () => {
    render(
      <ErrorDisplay
        category="validation"
        message="入力値が不正です"
        action="入力内容を確認してください"
      />
    );

    expect(screen.getByTestId("error-category")).toHaveTextContent(
      "入力エラー"
    );
  });

  it("カテゴリに応じたラベルが表示されること（フィードエラー）", () => {
    render(
      <ErrorDisplay
        category="feed"
        message="フィードの取得に失敗しました"
        action="URLを確認してください"
      />
    );

    expect(screen.getByTestId("error-category")).toHaveTextContent(
      "フィードエラー"
    );
  });

  it("カテゴリに応じたラベルが表示されること（システムエラー）", () => {
    render(
      <ErrorDisplay
        category="system"
        message="システムエラーが発生しました"
        action="しばらく時間をおいてから再度お試しください"
      />
    );

    expect(screen.getByTestId("error-category")).toHaveTextContent(
      "システムエラー"
    );
  });

  it("未知のカテゴリでもエラーが表示されること", () => {
    render(
      <ErrorDisplay
        category="unknown"
        message="不明なエラー"
        action="サポートに連絡してください"
      />
    );

    expect(screen.getByText("不明なエラー")).toBeInTheDocument();
    expect(screen.getByTestId("error-category")).toHaveTextContent("エラー");
  });

  it("ApiErrorResponseオブジェクトをerrorプロパティで渡せること", () => {
    render(
      <ErrorDisplay
        error={{
          code: "FEED_NOT_FOUND",
          message: "フィードが見つかりません",
          category: "feed",
          action: "URLを確認してください",
        }}
      />
    );

    expect(screen.getByText("フィードが見つかりません")).toBeInTheDocument();
    expect(screen.getByText("URLを確認してください")).toBeInTheDocument();
  });
});
