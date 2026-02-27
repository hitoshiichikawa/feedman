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
