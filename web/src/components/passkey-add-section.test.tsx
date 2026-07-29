import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";

// `usePasskeyAddRegistration` をモックし、返却する mutation state をケースごとに差し替える。
// `PasskeyAddRegistrationError` は実物を再 export し、production 同等の error オブジェクトを
// 組み立てられるようにする。
vi.mock("@/hooks/use-passkey-add-registration", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/use-passkey-add-registration")
  >("@/hooks/use-passkey-add-registration");
  return {
    ...actual,
    usePasskeyAddRegistration: vi.fn(),
  };
});

import {
  PasskeyAddRegistrationError,
  usePasskeyAddRegistration,
  type PasskeyAddRegistrationErrorKind,
} from "@/hooks/use-passkey-add-registration";
import { PasskeyAddSection } from "./passkey-add-section";

interface MockMutation {
  mutate: ReturnType<typeof vi.fn>;
  reset: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: PasskeyAddRegistrationError | null;
}

function buildMutation(overrides: Partial<MockMutation> = {}): MockMutation {
  return {
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
    isSuccess: false,
    error: null,
    ...overrides,
  };
}

function mockAddRegistration(mutation: MockMutation) {
  vi.mocked(usePasskeyAddRegistration).mockReturnValue(
    mutation as unknown as ReturnType<typeof usePasskeyAddRegistration>,
  );
}

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

function makeError(kind: PasskeyAddRegistrationErrorKind) {
  return new PasskeyAddRegistrationError(kind);
}

describe("PasskeyAddSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("パスキー追加の起動要素とロスト対策説明を表示すること (Req 1.5, Req 6.1)", () => {
    mockAddRegistration(buildMutation());
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    expect(screen.getByTestId("passkey-add-section")).toBeInTheDocument();
    expect(screen.getByTestId("passkey-add-trigger")).toBeInTheDocument();
    const lossPrevention = screen.getByTestId("passkey-add-loss-prevention");
    expect(lossPrevention).toBeInTheDocument();
    expect(lossPrevention.textContent ?? "").toMatch(
      /別の端末や同期先|アクセスできなくなる/,
    );
  });

  it("ロスト対策説明文に credential 識別子等の機密情報を含めないこと (Req 6.2)", () => {
    mockAddRegistration(buildMutation());
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    const lossPrevention = screen.getByTestId("passkey-add-loss-prevention");
    const text = lossPrevention.textContent ?? "";
    // credential ID / user id / email 等の識別子キーワードを含まない
    expect(text).not.toMatch(/credential[_-]?id/i);
    expect(text).not.toMatch(/user[_-]?id/i);
    expect(text).not.toMatch(/@/);
  });

  it("起動要素をクリックすると mutation.mutate が呼ばれること (Req 1.5, 2.1)", async () => {
    const mutation = buildMutation();
    mockAddRegistration(mutation);
    const user = userEvent.setup();
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    await user.click(screen.getByTestId("passkey-add-trigger"));

    expect(mutation.mutate).toHaveBeenCalledTimes(1);
  });

  it("mutation pending 中は起動要素が disabled になり文言が「登録中...」になること (Req 4.4)", () => {
    mockAddRegistration(buildMutation({ isPending: true }));
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    const trigger = screen.getByTestId("passkey-add-trigger") as HTMLButtonElement;
    expect(trigger.disabled).toBe(true);
    expect(trigger).toHaveTextContent("登録中...");
  });

  it("mutation 終了後は起動要素が再操作可能な状態に戻ること (Req 4.5)", () => {
    // 成功終了時: isPending は false に戻る
    mockAddRegistration(buildMutation({ isSuccess: true }));
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    const trigger = screen.getByTestId("passkey-add-trigger") as HTMLButtonElement;
    expect(trigger.disabled).toBe(false);
    expect(trigger).toHaveTextContent("パスキーを追加");
  });

  it("成功時に成功フィードバックが視認できる形で表示されること (Req 2.4)", () => {
    mockAddRegistration(buildMutation({ isSuccess: true }));
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    const success = screen.getByTestId("passkey-add-success");
    expect(success).toBeInTheDocument();
    expect(success).toHaveAttribute("role", "status");
    expect(success).toHaveTextContent("パスキーを追加しました。");
  });

  it("重複エラー時に他のエラーと区別できる文言・testid で提示されること (Req 3.1, 3.4)", () => {
    mockAddRegistration(
      buildMutation({
        isError: true,
        error: makeError("already_registered"),
      }),
    );
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    // 専用 testid で重複エラーを区別できる
    const dupError = screen.getByTestId("passkey-add-error-already-registered");
    expect(dupError).toBeInTheDocument();
    expect(dupError).toHaveAttribute("role", "alert");
    expect(dupError.textContent ?? "").toMatch(
      /すでに.*登録|既に.*登録|別の端末/,
    );
    // 汎用エラー表示は同時に出ない（キー分離を確認）
    expect(screen.queryByTestId("passkey-add-error")).not.toBeInTheDocument();
    // 起動要素は残り、再試行できる状態 (Req 3.3)
    expect(screen.getByTestId("passkey-add-trigger")).toBeInTheDocument();
  });

  it.each<PasskeyAddRegistrationErrorKind>([
    "server_rejected",
    "server_error",
    "network_error",
  ])(
    "汎用エラー %s は共通の汎用文言で提示され、専用の重複エラー枠が出ないこと (Req 4.2, 4.3, NFR 2.2)",
    (kind) => {
      mockAddRegistration(
        buildMutation({
          isError: true,
          error: makeError(kind),
        }),
      );
      render(<PasskeyAddSection />, { wrapper: createWrapper() });

      const err = screen.getByTestId("passkey-add-error");
      expect(err).toBeInTheDocument();
      expect(err).toHaveAttribute("role", "alert");
      // 内部詳細（kind 名 / status 数値等）が DOM に反射しない
      expect(err.textContent ?? "").not.toContain(kind);
      // 重複エラー枠は出ない
      expect(
        screen.queryByTestId("passkey-add-error-already-registered"),
      ).not.toBeInTheDocument();
      // 再試行可能な状態が維持される (Req 4.5 / 4.6)
      const trigger = screen.getByTestId(
        "passkey-add-trigger",
      ) as HTMLButtonElement;
      expect(trigger.disabled).toBe(false);
    },
  );

  it("cancelled 発生時はエラー表示を出さず mutation.reset() を呼ぶ (Req 4.1)", async () => {
    const mutation = buildMutation({
      isError: true,
      error: makeError("cancelled"),
    });
    mockAddRegistration(mutation);
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    // エラー枠は表示されない
    expect(screen.queryByTestId("passkey-add-error")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("passkey-add-error-already-registered"),
    ).not.toBeInTheDocument();
    // useEffect で mutation.reset が呼ばれる
    await waitFor(() => {
      expect(mutation.reset).toHaveBeenCalled();
    });
    // 起動要素は残り再試行可能
    expect(screen.getByTestId("passkey-add-trigger")).toBeInTheDocument();
  });

  it("再試行時に成功通知が消え、mutate が再度呼ばれること (Req 4.5)", async () => {
    const mutation = buildMutation({ isSuccess: true });
    mockAddRegistration(mutation);
    const user = userEvent.setup();
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    // 初期状態で成功通知が出ている
    expect(screen.getByTestId("passkey-add-success")).toBeInTheDocument();

    await user.click(screen.getByTestId("passkey-add-trigger"));

    // isSuccess 状態からの再クリックで reset と mutate が呼ばれる
    expect(mutation.reset).toHaveBeenCalled();
    expect(mutation.mutate).toHaveBeenCalledTimes(1);

    // 成功通知は隠される（dismissedSuccess = true になる）
    await waitFor(() => {
      expect(
        screen.queryByTestId("passkey-add-success"),
      ).not.toBeInTheDocument();
    });
  });

  it("エラー分類のいずれでも起動要素が残り、認証済みセッション UI が破壊されないこと (Req 4.6)", () => {
    // 代表として server_rejected を使う
    mockAddRegistration(
      buildMutation({
        isError: true,
        error: makeError("server_rejected"),
      }),
    );
    render(<PasskeyAddSection />, { wrapper: createWrapper() });

    // セクション自体が存在し（＝親の設定 UI 表示は維持される想定）、
    // 起動要素も残る
    expect(screen.getByTestId("passkey-add-section")).toBeInTheDocument();
    expect(screen.getByTestId("passkey-add-trigger")).toBeInTheDocument();
  });
});
