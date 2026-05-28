import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { useEffect, useState, type ReactNode } from "react";
import { FeedSearchBar } from "./feed-search-bar";
import {
  AppStateProvider,
  useAppState,
  useAppDispatch,
  type AppAction,
} from "@/contexts/app-state";

/**
 * AppStateProvider 配下に FeedSearchBar をマウントし、初期 dispatch（フィード選択等）を
 * 1 度だけ実行してから子をレンダするヘルパー。
 */
function renderWithInitialDispatch(
  ui: ReactNode,
  initialDispatch?: (dispatch: (action: AppAction) => void) => void
) {
  let observed: ReturnType<typeof useAppState> | null = null;
  function Probe() {
    const dispatch = useAppDispatch();
    observed = useAppState();
    const ready = useDispatchOnce(() => {
      if (initialDispatch) initialDispatch(dispatch);
    });
    return ready ? <>{ui}</> : null;
  }
  const utils = render(
    <AppStateProvider>
      <Probe />
    </AppStateProvider>
  );
  return {
    ...utils,
    getState: () => observed!,
  };
}

function useDispatchOnce(fn: () => void): boolean {
  const [done, setDone] = useState(false);
  useEffect(() => {
    fn();
    setDone(true);
    // 初回 mount のみ発火させる目的で deps を空配列に固定する
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return done;
}

describe("FeedSearchBar", () => {
  it("フィード未選択時（selectedFeedId === null）は何も描画しないこと（NFR 2.3）", () => {
    // 初期 dispatch なし = selectedFeedId が null のまま
    render(
      <AppStateProvider>
        <FeedSearchBar />
      </AppStateProvider>
    );
    expect(screen.queryByTestId("feed-search-bar")).toBeNull();
    expect(screen.queryByTestId("feed-search-input")).toBeNull();
  });

  it("フィード選択時は検索バーがレンダされること（Req 1.2）", () => {
    renderWithInitialDispatch(<FeedSearchBar />, (dispatch) => {
      dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
    expect(screen.getByTestId("feed-search-input")).toBeInTheDocument();
  });

  it("キーワード入力 + Enter で SET_SEARCH_QUERY（scope='feed', feedId）が dispatch されること（Req 1.4）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithInitialDispatch(
      <FeedSearchBar />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-99" });
      }
    );

    const input = screen.getByTestId("feed-search-input");
    await user.type(input, "kubernetes");
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("kubernetes");
    expect(state.isSearching).toBe(true);
    expect(state.searchScope).toBe("feed");
    expect(state.searchFeedId).toBe("feed-99");
  });

  it("空入力で Enter 押下しても dispatch されないこと（Req 1.5）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithInitialDispatch(
      <FeedSearchBar />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
      }
    );

    const input = screen.getByTestId("feed-search-input");
    await user.click(input);
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("");
    expect(state.isSearching).toBe(false);
  });

  it("空白のみ Enter 押下も dispatch されないこと（Req 1.5 / 境界値）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithInitialDispatch(
      <FeedSearchBar />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
      }
    );

    const input = screen.getByTestId("feed-search-input");
    await user.type(input, "   ");
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("");
    expect(state.isSearching).toBe(false);
  });

  it("入力に前後空白がある場合は trim された値で dispatch されること", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithInitialDispatch(
      <FeedSearchBar />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
      }
    );

    const input = screen.getByTestId("feed-search-input");
    await user.type(input, "  golang  ");
    await user.keyboard("{Enter}");

    const state = getState();
    expect(state.searchQuery).toBe("golang");
    expect(state.searchScope).toBe("feed");
    expect(state.searchFeedId).toBe("feed-1");
  });

  it("クリアボタンは入力後にのみ表示され、押下で CLEAR_SEARCH が dispatch されること（Req 1.6）", async () => {
    const user = userEvent.setup();
    const { getState } = renderWithInitialDispatch(
      <FeedSearchBar />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
        dispatch({
          type: "SET_SEARCH_QUERY",
          query: "docker",
          scope: "feed",
          feedId: "feed-1",
        });
      }
    );

    // 検索中状態で render されたとき、ローカル入力欄に searchQuery が反映される
    const input = screen.getByTestId(
      "feed-search-input"
    ) as HTMLInputElement;
    expect(input.value).toBe("docker");
    expect(getState().isSearching).toBe(true);

    // クリアボタン押下
    await user.click(screen.getByTestId("feed-search-clear"));

    const state = getState();
    expect(state.isSearching).toBe(false);
    expect(state.searchQuery).toBe("");
    expect(state.searchScope).toBe("global");
    expect(state.searchFeedId).toBeNull();
    expect(input.value).toBe("");
  });

  it("入力が空のときはクリアボタンが表示されないこと", () => {
    renderWithInitialDispatch(<FeedSearchBar />, (dispatch) => {
      dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    expect(screen.queryByTestId("feed-search-clear")).toBeNull();
  });
});
