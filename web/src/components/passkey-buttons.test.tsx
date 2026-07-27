import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";

// `usePasskeyCapability` をモックし、テストごとに `isLoading` / `available` を差し替える。
vi.mock("@/hooks/use-passkey-capability", () => ({
  usePasskeyCapability: vi.fn(),
}));

// `usePasskeyAuthentication` をモックし、テストごとに mutation state を差し替える。
// `PasskeyAuthError` / `PasskeyAuthErrorKind` は実物を再 export し、
// エラーオブジェクトを production と同じクラスで組み立てられるようにする
// （既存 `passkey-signup-dialog.test.tsx` と同 idiom）。
vi.mock("@/hooks/use-passkey-authentication", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/use-passkey-authentication")
  >("@/hooks/use-passkey-authentication");
  return {
    ...actual,
    usePasskeyAuthentication: vi.fn(),
  };
});

import { usePasskeyCapability } from "@/hooks/use-passkey-capability";
import {
  PasskeyAuthError,
  usePasskeyAuthentication,
  type PasskeyAuthErrorKind,
} from "@/hooks/use-passkey-authentication";
import { PasskeyButtons } from "./passkey-buttons";

/**
 * 本コンポーネントが消費する `UseMutationResult` サブセットの型。
 * production の `usePasskeyAuthentication` 戻り値のうち、コンポーネントが実際に参照する
 * フィールドだけを持ち、他は overrides で任意に上書きする。
 */
interface MockMutation {
  mutate: ReturnType<typeof vi.fn>;
  reset: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  error: PasskeyAuthError | null;
}

/** ダミーの mutation state を組み立てる。デフォルトは idle（成功も失敗もしていない）状態。 */
function buildMutation(overrides: Partial<MockMutation> = {}): MockMutation {
  return {
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
    ...overrides,
  };
}

/** `usePasskeyCapability` の mock 戻り値を差し替える薄いラッパ（型キャストを 1 箇所に閉じる）。 */
function mockCapability(state: { isLoading: boolean; available: boolean }) {
  vi.mocked(usePasskeyCapability).mockReturnValue(state);
}

/** `usePasskeyAuthentication` の mock 戻り値を差し替える薄いラッパ。 */
function mockAuthentication(mutation: MockMutation) {
  vi.mocked(usePasskeyAuthentication).mockReturnValue(
    mutation as unknown as ReturnType<typeof usePasskeyAuthentication>,
  );
}

/** kind から実物クラスの `PasskeyAuthError` を組み立てる。 */
function buildError(kind: PasskeyAuthErrorKind): PasskeyAuthError {
  return new PasskeyAuthError(kind);
}

describe("PasskeyButtons", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthentication(buildMutation());
  });

  it("capability が available: true のとき「パスキーでログイン」「アカウント新規作成」の 2 ボタンが表示されること（Req 1.1, 1.3, 1.4, 5.1）", () => {
    // Arrange
    mockCapability({ isLoading: false, available: true });

    // Act
    render(<PasskeyButtons onSignupClick={vi.fn()} />);

    // Assert
    expect(
      screen.getByRole("button", { name: "パスキーでログイン" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "アカウント新規作成" }),
    ).toBeInTheDocument();
  });

  it("capability が available: false のとき null 返却され DOM に何も出ないこと（Req 5.1, 5.2, 5.3）", () => {
    // Arrange
    mockCapability({ isLoading: false, available: false });

    // Act
    const { container } = render(<PasskeyButtons onSignupClick={vi.fn()} />);

    // Assert
    // 「パスキーでログイン」「アカウント新規作成」いずれも DOM に存在しない
    expect(
      screen.queryByRole("button", { name: "パスキーでログイン" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "アカウント新規作成" }),
    ).not.toBeInTheDocument();
    // ラッパも含めて何も render されない
    expect(container.firstChild).toBeNull();
  });

  it("capability が isLoading: true の間は null 返却され初期表示のちらつきが起きないこと（Req 5.1, 5.2）", () => {
    // Arrange
    mockCapability({ isLoading: true, available: false });

    // Act
    const { container } = render(<PasskeyButtons onSignupClick={vi.fn()} />);

    // Assert
    expect(container.firstChild).toBeNull();
  });

  it("「パスキーでログイン」クリックで usePasskeyAuthentication().mutate が呼ばれること（Req 1.3, 4.1）", async () => {
    // Arrange
    const mutate = vi.fn();
    mockCapability({ isLoading: false, available: true });
    mockAuthentication(buildMutation({ mutate }));
    const user = userEvent.setup();
    render(<PasskeyButtons onSignupClick={vi.fn()} />);

    // Act
    await user.click(screen.getByRole("button", { name: "パスキーでログイン" }));

    // Assert
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("「アカウント新規作成」クリックで onSignupClick callback が呼ばれること（Req 1.4, 2.1）", async () => {
    // Arrange
    const onSignupClick = vi.fn();
    mockCapability({ isLoading: false, available: true });
    const user = userEvent.setup();
    render(<PasskeyButtons onSignupClick={onSignupClick} />);

    // Act
    await user.click(screen.getByRole("button", { name: "アカウント新規作成" }));

    // Assert
    expect(onSignupClick).toHaveBeenCalledTimes(1);
  });

  it("mutation の error.kind === 'cancelled' ではエラー文言が表示されず mutation.reset が呼ばれること（Req 4.5, NFR 1.2）", async () => {
    // Arrange
    const reset = vi.fn();
    mockCapability({ isLoading: false, available: true });
    mockAuthentication(
      buildMutation({
        isError: true,
        error: buildError("cancelled"),
        reset,
      }),
    );

    // Act
    render(<PasskeyButtons onSignupClick={vi.fn()} />);

    // Assert
    // どの汎用エラー文言も alert として表示されない（Req 4.5「画面を壊さずに戻す」）
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    // useEffect が mutation.reset を呼び、mutation state を初期状態に戻す
    await waitFor(() => {
      expect(reset).toHaveBeenCalled();
    });
  });
});
