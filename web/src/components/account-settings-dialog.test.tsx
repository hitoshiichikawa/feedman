import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { AccountSettingsDialog } from "./account-settings-dialog";

// グローバル fetch のモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// window.location.assign のモック（退会成功時のリダイレクト検証用）
const mockAssign = vi.fn();
Object.defineProperty(window, "location", {
  value: {
    assign: mockAssign,
    href: "http://localhost:3000",
    pathname: "/",
  },
  writable: true,
});

/** テスト用ラッパー（QueryClient を fresh に生成） */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

interface AuthMockOptions {
  /**
   * GET /auth/me の応答種別
   *
   * - "ok": パスキーユーザー相当（username: "alice-id"）
   * - "ok-empty-email": Google 由来ユーザー相当（email 空 / username: null）
   * - "ok-empty-username": username が空文字（Req 3.3 の境界値検証用）
   * - "error": 500 応答
   */
  auth: "ok" | "ok-empty-email" | "ok-empty-username" | "error";
  /** DELETE /api/users/me の応答種別（省略時は成功） */
  withdraw?: "success" | "fail" | "pending";
}

/**
 * `/auth/me` と `/api/users/me` の応答を一括で組み立てる mockFetch セットアップ。
 * 呼び出し履歴（`mockFetch.mock.calls`）で URL と method の観測ができる。
 */
function setupMockFetch(options: AuthMockOptions) {
  const withdrawMode = options.withdraw ?? "success";
  mockFetch.mockImplementation((url: string, init?: RequestInit) => {
    if (url === "/auth/me") {
      if (options.auth === "ok") {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            id: "user-1",
            email: "alice@example.com",
            name: "Alice",
            username: "alice-id",
            created_at: "2026-01-01T00:00:00Z",
          }),
        });
      }
      if (options.auth === "ok-empty-email") {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            id: "user-2",
            email: "",
            name: "Passkey User",
            username: null,
            created_at: "2026-01-02T00:00:00Z",
          }),
        });
      }
      if (options.auth === "ok-empty-username") {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({
            id: "user-3",
            email: "bob@example.com",
            name: "Bob",
            username: "",
            created_at: "2026-01-03T00:00:00Z",
          }),
        });
      }
      // "error"
      return Promise.resolve({
        ok: false,
        status: 500,
        json: async () => ({ error: "internal server error" }),
      });
    }
    if (url === "/api/users/me" && init?.method === "DELETE") {
      if (withdrawMode === "success") {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({}),
        });
      }
      if (withdrawMode === "fail") {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ error: "internal server error" }),
        });
      }
      // pending: never resolve（多重送信テスト用）
      return new Promise(() => {});
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

/**
 * ダイアログを開くヘルパ。open → 内容が render されるまで待つ。
 */
async function openDialog(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByTestId("account-settings-trigger"));
  await waitFor(() => {
    expect(screen.getByText("アカウント設定")).toBeInTheDocument();
  });
}

describe("AccountSettingsDialog コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAssign.mockClear();
  });

  it("アカウント設定入口ボタンが表示されること (Req 1.1)", () => {
    setupMockFetch({ auth: "ok" });
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    expect(screen.getByTestId("account-settings-trigger")).toBeInTheDocument();
  });

  it("入口ボタンをクリックするとアカウント情報表示領域と退会導線を含むダイアログが開くこと (Req 1.2)", async () => {
    setupMockFetch({ auth: "ok" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // アカウント情報表示領域
    await waitFor(() => {
      expect(
        screen.getByTestId("account-info-section")
      ).toBeInTheDocument();
    });
    // 退会導線
    expect(
      screen.getByTestId("account-withdraw-section")
    ).toBeInTheDocument();
    expect(screen.getByTestId("withdraw-trigger")).toBeInTheDocument();
  });

  it("ダイアログはユーザー操作 (Esc) で明示的に閉じられること (Req 1.4)", async () => {
    setupMockFetch({ auth: "ok" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // ヘッダーが描画されるのを待つ
    await waitFor(() => {
      expect(
        screen.getByTestId("account-info-section")
      ).toBeInTheDocument();
    });

    // Esc で閉じられる（radix-ui Dialog の既定挙動）
    await user.keyboard("{Escape}");

    await waitFor(() => {
      expect(screen.queryByText("アカウント設定")).not.toBeInTheDocument();
    });
  });

  it("表示名と email が表示されること (Req 2.1, 2.2)", async () => {
    setupMockFetch({ auth: "ok" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    await waitFor(() => {
      expect(screen.getByTestId("account-info-name")).toHaveTextContent(
        "Alice"
      );
    });
    expect(screen.getByTestId("account-info-email")).toHaveTextContent(
      "alice@example.com"
    );
    // email 空プレースホルダは出ていない
    expect(
      screen.queryByTestId("account-info-email-unset")
    ).not.toBeInTheDocument();
  });

  it("email が未設定（空文字）のとき「未設定」プレースホルダが表示され、空欄放置されないこと (Req 2.3)", async () => {
    setupMockFetch({ auth: "ok-empty-email" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    await waitFor(() => {
      expect(screen.getByTestId("account-info-name")).toHaveTextContent(
        "Passkey User"
      );
    });
    // 未設定プレースホルダが表示され、通常 email 表示は出ない
    expect(
      screen.getByTestId("account-info-email-unset")
    ).toHaveTextContent("未設定");
    expect(screen.queryByTestId("account-info-email")).not.toBeInTheDocument();
  });

  it("username が非 null / 非空のとき username が「ユーザー名」ラベル付きで表示されること (Req 3.2)", async () => {
    setupMockFetch({ auth: "ok" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // username 表示要素が生値そのままで表示される（`@` プレフィックス付加はしない設計）
    await waitFor(() => {
      expect(screen.getByTestId("account-info-username")).toHaveTextContent(
        "alice-id"
      );
    });
    // 対応する「ユーザー名」ラベルも表示される
    expect(screen.getByText("ユーザー名")).toBeInTheDocument();
    // 既存の表示名 / email 表示は非破壊で維持される（Req 3.4）
    expect(screen.getByTestId("account-info-name")).toHaveTextContent("Alice");
    expect(screen.getByTestId("account-info-email")).toHaveTextContent(
      "alice@example.com"
    );
  });

  it("username が null のとき username 表示要素が描画されず、既存の表示名 / email 表示に影響しないこと (Req 3.3 / Req 4.2)", async () => {
    // Google 由来ユーザー相当（email 空 / username: null）
    setupMockFetch({ auth: "ok-empty-email" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // 表示名（既存）は先に描画されるのを待つ
    await waitFor(() => {
      expect(screen.getByTestId("account-info-name")).toHaveTextContent(
        "Passkey User"
      );
    });
    // username 表示要素そのものが DOM に出ない（代替ラベルも出さない / Req 3.3）
    expect(
      screen.queryByTestId("account-info-username")
    ).not.toBeInTheDocument();
    expect(screen.queryByText("ユーザー名")).not.toBeInTheDocument();
    // 既存の email 未設定プレースホルダは維持される（Req 4.2 の非破壊性）
    expect(
      screen.getByTestId("account-info-email-unset")
    ).toHaveTextContent("未設定");
  });

  it("username が空文字のとき username 表示要素が描画されないこと (Req 3.3)", async () => {
    setupMockFetch({ auth: "ok-empty-username" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // 表示名（既存）が描画されるのを待つ
    await waitFor(() => {
      expect(screen.getByTestId("account-info-name")).toHaveTextContent("Bob");
    });
    // username 表示要素が DOM に存在しない（null と同様の扱い / Req 3.3）
    expect(
      screen.queryByTestId("account-info-username")
    ).not.toBeInTheDocument();
    expect(screen.queryByText("ユーザー名")).not.toBeInTheDocument();
    // 既存の email 表示は非破壊で維持される
    expect(screen.getByTestId("account-info-email")).toHaveTextContent(
      "bob@example.com"
    );
  });

  it("アカウント情報の取得に失敗したときエラー通知が表示され、空 UI にならないこと (Req 2.4)", async () => {
    setupMockFetch({ auth: "error" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    await waitFor(() => {
      expect(
        screen.getByTestId("account-settings-error")
      ).toBeInTheDocument();
    });
    expect(screen.getByTestId("account-settings-error")).toHaveTextContent(
      "アカウント情報の取得に失敗しました"
    );
    // 表示名/退会セクションはレンダリングされない（データ不在時の空 UI 防止 = alert に置換）
    expect(screen.queryByTestId("account-info-section")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("account-withdraw-section")
    ).not.toBeInTheDocument();
  });

  it("退会確認ダイアログでキャンセルすると DELETE を送信せず認証状態と設定 UI が維持されること (Req 3.3)", async () => {
    setupMockFetch({ auth: "ok" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);
    await waitFor(() => {
      expect(screen.getByTestId("withdraw-trigger")).toBeInTheDocument();
    });

    // 退会入口 → 確認ダイアログ
    await user.click(screen.getByTestId("withdraw-trigger"));
    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    // キャンセル
    await user.click(screen.getByRole("button", { name: "キャンセル" }));

    // DELETE は呼ばれない
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/users/me",
      expect.objectContaining({ method: "DELETE" })
    );
    // window.location.assign によるリダイレクトも起きない
    expect(mockAssign).not.toHaveBeenCalled();
    // アカウント設定 UI は残る
    expect(
      screen.getByTestId("account-info-section")
    ).toBeInTheDocument();
  });

  it("退会確定で DELETE /api/users/me が発行され、成功時に /login へリダイレクトされること (Req 3.4, 3.5)", async () => {
    setupMockFetch({ auth: "ok", withdraw: "success" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);
    await waitFor(() => {
      expect(screen.getByTestId("withdraw-trigger")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-trigger"));
    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-confirm"));

    // DELETE /api/users/me が発行される
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/users/me",
        expect.objectContaining({ method: "DELETE" })
      );
    });

    // リダイレクト先が /login
    await waitFor(() => {
      expect(mockAssign).toHaveBeenCalledWith("/login");
    });
  });

  it("退会失敗時に /login へリダイレクトせず、ダイアログ内に失敗通知が表示されること (Req 3.6)", async () => {
    setupMockFetch({ auth: "ok", withdraw: "fail" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);
    await waitFor(() => {
      expect(screen.getByTestId("withdraw-trigger")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-trigger"));
    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("withdraw-confirm"));

    // DELETE は発行されるが失敗
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/users/me",
        expect.objectContaining({ method: "DELETE" })
      );
    });

    // 失敗通知
    await waitFor(() => {
      expect(screen.getByTestId("withdraw-error")).toBeInTheDocument();
    });

    // /login へのリダイレクトは起きない（セッション維持 / Req 3.6）
    expect(mockAssign).not.toHaveBeenCalled();
    // アカウント情報セクションはダイアログの背後で維持される
    expect(
      screen.getByTestId("account-info-section")
    ).toBeInTheDocument();
  });

  it("認証済みユーザーがダイアログを開くとパスキー追加登録セクションが表示されること (Issue #242 Req 1.1)", async () => {
    setupMockFetch({ auth: "ok" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // data 取得成功後に本セクションが描画される
    await waitFor(() => {
      expect(screen.getByTestId("passkey-add-section")).toBeInTheDocument();
    });
    expect(screen.getByTestId("passkey-add-trigger")).toBeInTheDocument();
  });

  it("Google 由来（email 未設定含む）ユーザーでも同一位置にパスキー追加導線が表示されること (Issue #242 Req 5.1)", async () => {
    // email 空文字（Google 由来でパスキー未登録相当）を再現
    setupMockFetch({ auth: "ok-empty-email" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    await waitFor(() => {
      expect(screen.getByTestId("passkey-add-section")).toBeInTheDocument();
    });
    // 起動要素は同一操作性で存在
    expect(screen.getByTestId("passkey-add-trigger")).toBeInTheDocument();
  });

  it("アカウント情報取得中はパスキー追加登録セクションが表示されないこと (Issue #242 Req 1.3)", async () => {
    // /auth/me を pending 状態にする
    mockFetch.mockImplementation((url: string) => {
      if (url === "/auth/me") {
        return new Promise(() => {}); // never resolve
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // loading プレースホルダは出るが、追加登録セクションは描画されない
    await waitFor(() => {
      expect(
        screen.getByTestId("account-settings-loading"),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByTestId("passkey-add-section"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("passkey-add-trigger"),
    ).not.toBeInTheDocument();
  });

  it("アカウント情報取得に失敗したときはパスキー追加登録セクションが表示されないこと (Issue #242 Req 1.4)", async () => {
    setupMockFetch({ auth: "error" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);

    // エラー通知は出るが、追加登録セクションは描画されない
    await waitFor(() => {
      expect(
        screen.getByTestId("account-settings-error"),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByTestId("passkey-add-section"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("passkey-add-trigger"),
    ).not.toBeInTheDocument();
  });

  it("入口ボタンを押さない（ダイアログを開かない）と追加登録起動要素は表示されないこと (Issue #242 Req 1.2 の未認証相当 UI 未描画)", () => {
    // 未認証状態でも、ダイアログを開かなければ本セクションは DOM に存在しない
    setupMockFetch({ auth: "error" });
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    // トリガーは表示されるが、追加登録セクション自体はダイアログを開かない限り DOM 外
    expect(screen.getByTestId("account-settings-trigger")).toBeInTheDocument();
    expect(
      screen.queryByTestId("passkey-add-section"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("passkey-add-trigger"),
    ).not.toBeInTheDocument();
  });

  it("email 未設定（空文字）のパスキーのみアカウントでも退会が同一フローで完了すること (Req 3.7)", async () => {
    setupMockFetch({ auth: "ok-empty-email", withdraw: "success" });
    const user = userEvent.setup();
    render(<AccountSettingsDialog />, { wrapper: createWrapper() });

    await openDialog(user);
    await waitFor(() => {
      expect(
        screen.getByTestId("account-info-email-unset")
      ).toBeInTheDocument();
    });

    // 退会フローを実行
    await user.click(screen.getByTestId("withdraw-trigger"));
    await waitFor(() => {
      expect(screen.getByText("退会しますか？")).toBeInTheDocument();
    });
    await user.click(screen.getByTestId("withdraw-confirm"));

    // 同一エンドポイント / 同一フローで DELETE 発行
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/users/me",
        expect.objectContaining({ method: "DELETE" })
      );
    });
    // 未認証遷移も成功する
    await waitFor(() => {
      expect(mockAssign).toHaveBeenCalledWith("/login");
    });
  });
});
