import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";

// `usePasskeyRegistration` をモックし、返却する mutation state をケースごとに差し替える。
// `PasskeyRegistrationError` / `PasskeyRegistrationErrorKind` は実物を再 export し、
// エラーオブジェクトを production と同じクラスで組み立てられるようにする（`instanceof` /
// `kind` プロパティが本番同等に通る）。
vi.mock("@/hooks/use-passkey-registration", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/use-passkey-registration")
  >("@/hooks/use-passkey-registration");
  return {
    ...actual,
    usePasskeyRegistration: vi.fn(),
  };
});

import {
  PasskeyRegistrationError,
  usePasskeyRegistration,
  type PasskeyRegistrationErrorKind,
} from "@/hooks/use-passkey-registration";
import { PasskeySignupDialog } from "./passkey-signup-dialog";

/**
 * 本 Dialog が消費する `UseMutationResult` サブセットの型。
 * production の `usePasskeyRegistration` 戻り値のうち、コンポーネントが実際に参照する
 * フィールドだけを持ち、他は overrides で任意に上書きする。
 */
interface MockMutation {
  mutate: ReturnType<typeof vi.fn>;
  reset: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: PasskeyRegistrationError | null;
}

/** ダミーの mutation state を組み立てる。デフォルトは idle（成功も失敗もしていない）状態。 */
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

/** `usePasskeyRegistration` の mock 戻り値を差し替える薄いラッパ（型キャストを 1 箇所に閉じる）。 */
function mockRegistration(mutation: MockMutation) {
  vi.mocked(usePasskeyRegistration).mockReturnValue(
    mutation as unknown as ReturnType<typeof usePasskeyRegistration>,
  );
}

/** kind から実物クラスの `PasskeyRegistrationError` を組み立てる。 */
function buildError(kind: PasskeyRegistrationErrorKind): PasskeyRegistrationError {
  return new PasskeyRegistrationError(kind);
}

describe("PasskeySignupDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("open=true のとき username 入力欄と「作成」ボタンが表示され、recovery email 入力欄が存在しないこと（Req 2.1, 2.4）", () => {
    // Arrange
    mockRegistration(buildMutation());

    // Act
    render(<PasskeySignupDialog open={true} onOpenChange={vi.fn()} />);

    // Assert
    expect(screen.getByLabelText("ユーザー名")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "作成" })).toBeInTheDocument();
    // Requirement 2.4: recovery email 入力欄を UI に一切設けない
    expect(screen.queryByLabelText(/email/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/メール/)).not.toBeInTheDocument();
  });

  it("username を入力して「作成」をクリックすると mutate({username: <入力値>}) が呼ばれること（Req 2.1, 2.2）", async () => {
    // Arrange
    const mutate = vi.fn();
    mockRegistration(buildMutation({ mutate }));
    const user = userEvent.setup();
    render(<PasskeySignupDialog open={true} onOpenChange={vi.fn()} />);

    // Act
    await user.type(screen.getByLabelText("ユーザー名"), "alice");
    await user.click(screen.getByRole("button", { name: "作成" }));

    // Assert
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate).toHaveBeenCalledWith({ username: "alice" });
  });

  it("error.kind === 'invalid_username' のとき対応する形式エラー文言が alert として表示されること（Req 2.5）", () => {
    // Arrange
    mockRegistration(
      buildMutation({
        isError: true,
        error: buildError("invalid_username"),
      }),
    );

    // Act
    render(<PasskeySignupDialog open={true} onOpenChange={vi.fn()} />);

    // Assert
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(
      "ユーザー名の形式が不正です（3〜32 文字、英数字とハイフン・アンダースコアのみ）",
    );
  });

  it("error.kind === 'username_taken' のとき対応する重複エラー文言が alert として表示されること（Req 2.6）", () => {
    // Arrange
    mockRegistration(
      buildMutation({
        isError: true,
        error: buildError("username_taken"),
      }),
    );

    // Act
    render(<PasskeySignupDialog open={true} onOpenChange={vi.fn()} />);

    // Assert
    expect(screen.getByRole("alert")).toHaveTextContent(
      "このユーザー名は既に使用されています",
    );
  });

  it("error.kind === 'cancelled' のとき汎用エラー文言が表示されず mutation.reset が呼ばれること（Req 2.7）", async () => {
    // Arrange
    const reset = vi.fn();
    mockRegistration(
      buildMutation({
        isError: true,
        error: buildError("cancelled"),
        reset,
      }),
    );

    // Act
    render(<PasskeySignupDialog open={true} onOpenChange={vi.fn()} />);

    // Assert
    // どの汎用エラー文言も alert として表示されない（Req 2.7「画面を壊さずに戻す」）
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    // useEffect が mutation.reset を呼び、mutation state を初期状態に戻す
    await waitFor(() => {
      expect(reset).toHaveBeenCalled();
    });
  });

  it("isSuccess === true のとき onOpenChange(false) が呼ばれること（Req 3.1, 3.2）", async () => {
    // Arrange
    const onOpenChange = vi.fn();
    mockRegistration(buildMutation({ isSuccess: true }));

    // Act
    render(<PasskeySignupDialog open={true} onOpenChange={onOpenChange} />);

    // Assert
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});
