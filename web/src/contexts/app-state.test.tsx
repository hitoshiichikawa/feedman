import { renderHook, act } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import {
  AppStateProvider,
  useAppState,
  useAppDispatch,
} from "./app-state";
import type { ReactNode } from "react";

/** テスト用ラッパー */
function createWrapper() {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <AppStateProvider>{children}</AppStateProvider>;
  };
}

describe("AppStateContext", () => {
  it("初期状態が正しく設定されていること", () => {
    const { result } = renderHook(() => useAppState(), {
      wrapper: createWrapper(),
    });

    expect(result.current.selectedFeedId).toBeNull();
    expect(result.current.expandedItemId).toBeNull();
    expect(result.current.filter).toBe("all");
    expect(result.current.searchQuery).toBe("");
    expect(result.current.isSearching).toBe(false);
    expect(result.current.searchScope).toBe("global");
    expect(result.current.searchFeedId).toBeNull();
  });

  it("SELECT_FEED アクションでフィードが選択されること", () => {
    const { result: stateResult } = renderHook(() => useAppState(), {
      wrapper: createWrapper(),
    });
    const { result: dispatchResult } = renderHook(() => useAppDispatch(), {
      wrapper: createWrapper(),
    });

    // 注意: 同じProviderインスタンスを共有する必要があるため、別の方法で検証する
  });

  it("SELECT_FEED アクションでフィード選択時に展開記事IDとフィルタがリセットされること", () => {
    const wrapper = createWrapper();

    // useAppState と useAppDispatch を同一のProviderから取得するヘルパー
    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 記事を展開し、フィルタを変更してからフィード選択
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "unread" });
    });

    expect(result.current.state.expandedItemId).toBe("item-1");
    expect(result.current.state.filter).toBe("unread");

    // フィード選択で展開記事IDとフィルタがリセットされる
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });

    expect(result.current.state.selectedFeedId).toBe("feed-1");
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
  });

  it("EXPAND_ITEM アクションで記事が展開されること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });

    expect(result.current.state.expandedItemId).toBe("item-1");
  });

  it("EXPAND_ITEM アクションで同じ記事IDを指定すると展開が解除されること（トグル動作）", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    expect(result.current.state.expandedItemId).toBe("item-1");

    // 同じIDを再度展開すると閉じる
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    expect(result.current.state.expandedItemId).toBeNull();
  });

  it("EXPAND_ITEM アクションで異なる記事IDを指定すると排他的に展開されること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    expect(result.current.state.expandedItemId).toBe("item-1");

    // 異なるIDを展開すると前回が閉じて新しいものが展開される
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-2" });
    });
    expect(result.current.state.expandedItemId).toBe("item-2");
  });

  it("SET_FILTER アクションでフィルタが変更されること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "starred" });
    });

    expect(result.current.state.filter).toBe("starred");
  });

  it("SET_SEARCH_QUERY アクション（scope='global'）で横断検索状態に遷移すること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({
        type: "SET_SEARCH_QUERY",
        query: "typescript",
        scope: "global",
      });
    });

    expect(result.current.state.searchQuery).toBe("typescript");
    expect(result.current.state.isSearching).toBe(true);
    expect(result.current.state.searchScope).toBe("global");
    expect(result.current.state.searchFeedId).toBeNull();
  });

  it("SET_SEARCH_QUERY アクション（scope='feed'）でフィード内検索状態に遷移し searchFeedId が設定されること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({
        type: "SET_SEARCH_QUERY",
        query: "rust",
        scope: "feed",
        feedId: "feed-42",
      });
    });

    expect(result.current.state.searchQuery).toBe("rust");
    expect(result.current.state.isSearching).toBe(true);
    expect(result.current.state.searchScope).toBe("feed");
    expect(result.current.state.searchFeedId).toBe("feed-42");
  });

  it("SET_SEARCH_QUERY アクション（scope='feed'）で feedId 未指定時は searchFeedId が null になること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    act(() => {
      result.current.dispatch({
        type: "SET_SEARCH_QUERY",
        query: "golang",
        scope: "feed",
      });
    });

    expect(result.current.state.searchQuery).toBe("golang");
    expect(result.current.state.isSearching).toBe(true);
    expect(result.current.state.searchScope).toBe("feed");
    expect(result.current.state.searchFeedId).toBeNull();
  });

  it("CLEAR_SEARCH アクションで検索状態がリセットされ、selectedFeedId と filter は保持されること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 先にフィード選択と filter 変更で selectedFeedId と filter を非初期値にする
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "unread" });
    });
    // 続けて検索を開始
    act(() => {
      result.current.dispatch({
        type: "SET_SEARCH_QUERY",
        query: "kubernetes",
        scope: "feed",
        feedId: "feed-1",
      });
    });

    expect(result.current.state.selectedFeedId).toBe("feed-1");
    expect(result.current.state.filter).toBe("unread");
    expect(result.current.state.isSearching).toBe(true);

    // CLEAR_SEARCH で検索状態のみリセット、selectedFeedId と filter は保持
    act(() => {
      result.current.dispatch({ type: "CLEAR_SEARCH" });
    });

    expect(result.current.state.searchQuery).toBe("");
    expect(result.current.state.isSearching).toBe(false);
    expect(result.current.state.searchScope).toBe("global");
    expect(result.current.state.searchFeedId).toBeNull();
    expect(result.current.state.selectedFeedId).toBe("feed-1");
    expect(result.current.state.filter).toBe("unread");
  });

  it("SELECT_FEED アクションで検索状態（searchQuery / isSearching / searchScope / searchFeedId）もリセットされること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 先に検索を開始
    act(() => {
      result.current.dispatch({
        type: "SET_SEARCH_QUERY",
        query: "docker",
        scope: "feed",
        feedId: "feed-1",
      });
    });

    expect(result.current.state.searchQuery).toBe("docker");
    expect(result.current.state.isSearching).toBe(true);
    expect(result.current.state.searchScope).toBe("feed");
    expect(result.current.state.searchFeedId).toBe("feed-1");

    // 別フィードを選択すると検索状態もリセットされる
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-2" });
    });

    expect(result.current.state.selectedFeedId).toBe("feed-2");
    expect(result.current.state.searchQuery).toBe("");
    expect(result.current.state.isSearching).toBe(false);
    expect(result.current.state.searchScope).toBe("global");
    expect(result.current.state.searchFeedId).toBeNull();
  });

  it("Provider外でuseAppStateを使用するとエラーが発生すること", () => {
    expect(() => {
      renderHook(() => useAppState());
    }).toThrow("useAppState は AppStateProvider 内で使用してください");
  });

  it("Provider外でuseAppDispatchを使用するとエラーが発生すること", () => {
    expect(() => {
      renderHook(() => useAppDispatch());
    }).toThrow("useAppDispatch は AppStateProvider 内で使用してください");
  });
});
