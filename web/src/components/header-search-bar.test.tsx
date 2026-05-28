import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { useEffect, useState, type ReactNode } from "react";
import { HeaderSearchBar } from "./header-search-bar";
import {
  AppStateProvider,
  useAppState,
  useAppDispatch,
} from "@/contexts/app-state";

/**
 * AppStateProvider 配下に HeaderSearchBar をマウントし、AppState を
 * 観測するためのフック。`StateProbe` 経由で state を test に流す。
 */
function renderWithProvider(ui: ReactNode) {
  let observed: ReturnType<typeof useAppState> | null = null;
  let dispatchRef: ReturnType<typeof useAppDispatch> | null = null;
  function StateProbe() {
    observed = useAppState();
    dispatchRef = useAppDispatch();
    return null;
  }
  const utils = render(
    <AppStateProvider>
      <StateProbe />
      {ui}
    </AppStateProvider>
  );
  return {
    ...utils,
    getState: () => observed!,
    getDispatch: () => dispatchRef!,
  };
}

describe("HeaderSearchBar", () => {
  it("検索キーワード入力欄が ARIA ロール search 配下にレンダされること（Req 1.1）", () => {
    renderWithProvider(<HeaderSearchBar />);
    expect(screen.getByRole("search")).toBeInTheDocument();
    expect(screen.getByTestId("header-search-input")).toBeInTheDocument();
  });

  it("キーワードを入力し Enter 押下で SET_SEARCH_QUERY（scope='global'）が dispatch されること（Req 1.3）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithProvider(<HeaderSearchBar />);
    const input = screen.getByTestId("header-search-input");

    await user.type(input, "typescript");
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("typescript");
    expect(state.isSearching).toBe(true);
    expect(state.searchScope).toBe("global");
    expect(state.searchFeedId).toBeNull();
  });

  it("空入力で Enter 押下しても dispatch されず通常一覧表示が維持されること（Req 1.5）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithProvider(<HeaderSearchBar />);
    const input = screen.getByTestId("header-search-input");

    // 空のまま focus を当てて Enter
    await user.click(input);
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("");
    expect(state.isSearching).toBe(false);
  });

  it("空白のみの入力で Enter 押下しても dispatch されないこと（Req 1.5 / 境界値）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithProvider(<HeaderSearchBar />);
    const input = screen.getByTestId("header-search-input");

    await user.type(input, "   ");
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("");
    expect(state.isSearching).toBe(false);
  });

  it("入力に前後空白がある場合は trim された値が dispatch されること", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithProvider(<HeaderSearchBar />);
    const input = screen.getByTestId("header-search-input");

    await user.type(input, "  rust  ");
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("rust");
  });

  it("クリアボタンは入力が空のときは表示されず、入力後に表示されること", async () => {
    const user = userEvent.setup();
    renderWithProvider(<HeaderSearchBar />);
    const input = screen.getByTestId("header-search-input");

    expect(screen.queryByTestId("header-search-clear")).toBeNull();

    await user.type(input, "kafka");
    expect(screen.getByTestId("header-search-clear")).toBeInTheDocument();
  });

  it("クリアボタン押下で CLEAR_SEARCH が dispatch され、ローカル入力もクリアされること（Req 1.6）", async () => {
    const user = userEvent.setup();
    const { getState, getDispatch } = renderWithProvider(<HeaderSearchBar />);
    const input = screen.getByTestId("header-search-input") as HTMLInputElement;

    // 検索中状態をシミュレートするため、事前に dispatch しておく
    await act(async () => {
      getDispatch()({
        type: "SET_SEARCH_QUERY",
        query: "kubernetes",
        scope: "global",
      });
    });
    // 入力欄にも値を入れておく
    await user.type(input, "kubernetes");
    expect(getState().isSearching).toBe(true);

    // クリアボタン押下
    await user.click(screen.getByTestId("header-search-clear"));

    const state = getState();
    expect(state.isSearching).toBe(false);
    expect(state.searchQuery).toBe("");
    expect(state.searchScope).toBe("global");
    // ローカル入力欄もクリアされている
    expect(input.value).toBe("");
  });

  it("検索中（scope='global'）の状態でレンダされたとき AppState.searchQuery を初期値に反映すること", () => {
    // 検索状態を事前にセットアップしてから HeaderSearchBar を render する
    function Setup() {
      const dispatch = useAppDispatch();
      // useEffect 相当の即時 dispatch を render 内で行うため、ref で 1 回限定にする
      const didDispatch = useDispatchOnce(() => {
        dispatch({
          type: "SET_SEARCH_QUERY",
          query: "docker",
          scope: "global",
        });
      });
      return didDispatch ? <HeaderSearchBar /> : null;
    }

    render(
      <AppStateProvider>
        <Setup />
      </AppStateProvider>
    );

    const input = screen.getByTestId(
      "header-search-input"
    ) as HTMLInputElement;
    expect(input.value).toBe("docker");
  });
});

// テスト専用の "1 回だけ effect を発火する" ヘルパー（useEffect の代替）。
// AppStateProvider 配下で初回マウント時に dispatch を 1 度だけ流し、
// その完了タイミングで子コンポーネントを render するために使用する。
function useDispatchOnce(fn: () => void): boolean {
  const [done, setDone] = useState(false);
  useEffect(() => {
    fn();
    setDone(true);
    // 初回 mount でのみ発火させたいため deps は空配列を明示する
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return done;
}
