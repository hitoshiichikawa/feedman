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

    expect(result.current.selectedView).toBe("feed");
    expect(result.current.viewMode).toBe("none");
    expect(result.current.selectedFeedId).toBeNull();
    expect(result.current.expandedItemId).toBeNull();
    expect(result.current.filter).toBe("all");
    expect(result.current.searchQuery).toBe("");
    expect(result.current.isSearching).toBe(false);
    expect(result.current.searchScope).toBe("global");
    expect(result.current.searchFeedId).toBeNull();
    expect(result.current.crossFeedSessionSince).toBeNull();
  });

  it("SELECT_STARRED アクションで selectedView が 'starred' に遷移し、他フィールドがリセットされること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 事前にフィード選択 / 展開 / フィルタ変更を行う
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "unread" });
    });

    expect(result.current.state.selectedView).toBe("feed");
    expect(result.current.state.selectedFeedId).toBe("feed-1");
    expect(result.current.state.expandedItemId).toBe("item-1");
    expect(result.current.state.filter).toBe("unread");

    // お気に入り選択で他フィールドがリセットされる
    act(() => {
      result.current.dispatch({ type: "SELECT_STARRED" });
    });

    expect(result.current.state.selectedView).toBe("starred");
    expect(result.current.state.selectedFeedId).toBeNull();
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
  });

  it("SELECT_STARRED の後に SELECT_FEED を dispatch すると selectedView が 'feed' に戻ること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // お気に入り選択状態に遷移
    act(() => {
      result.current.dispatch({ type: "SELECT_STARRED" });
    });
    expect(result.current.state.selectedView).toBe("starred");
    expect(result.current.state.selectedFeedId).toBeNull();

    // フィード選択で selectedView が "feed" に戻る
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-2" });
    });

    expect(result.current.state.selectedView).toBe("feed");
    expect(result.current.state.selectedFeedId).toBe("feed-2");
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
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

  it("CLEAR_SELECTED_FEED アクションで selectedFeedId が null になり、expandedItemId と filter がリセットされること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 事前にフィード選択 / 記事展開 / フィルタ変更を行い、非初期状態を作る
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "unread" });
    });

    expect(result.current.state.selectedFeedId).toBe("feed-1");
    expect(result.current.state.expandedItemId).toBe("item-1");
    expect(result.current.state.filter).toBe("unread");

    // CLEAR_SELECTED_FEED で selectedFeedId が null になり、関連状態がリセットされる
    act(() => {
      result.current.dispatch({ type: "CLEAR_SELECTED_FEED" });
    });

    // (a) selectedFeedId が null になる
    expect(result.current.state.selectedFeedId).toBeNull();
    // (b) expandedItemId / filter が初期値にリセットされる
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
    // selectedView は "feed" のまま（SELECT_FEED と同等パターン）
    expect(result.current.state.selectedView).toBe("feed");
  });

  it("CLEAR_SELECTED_FEED アクションで検索状態（searchQuery / isSearching / searchScope / searchFeedId）もリセットされること", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 検索状態を非初期値にする
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    act(() => {
      result.current.dispatch({
        type: "SET_SEARCH_QUERY",
        query: "kubernetes",
        scope: "feed",
        feedId: "feed-1",
      });
    });

    expect(result.current.state.searchQuery).toBe("kubernetes");
    expect(result.current.state.isSearching).toBe(true);
    expect(result.current.state.searchScope).toBe("feed");
    expect(result.current.state.searchFeedId).toBe("feed-1");

    // CLEAR_SELECTED_FEED で検索状態もリセットされる（SELECT_FEED と同等の副作用）
    act(() => {
      result.current.dispatch({ type: "CLEAR_SELECTED_FEED" });
    });

    expect(result.current.state.selectedFeedId).toBeNull();
    expect(result.current.state.searchQuery).toBe("");
    expect(result.current.state.isSearching).toBe(false);
    expect(result.current.state.searchScope).toBe("global");
    expect(result.current.state.searchFeedId).toBeNull();
  });

  it("CLEAR_SELECTED_FEED アクションは初期状態に対しても安全に動作すること（冪等性）", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 初期状態（selectedFeedId === null）から CLEAR_SELECTED_FEED を dispatch しても
    // 例外が発生せず、全フィールドが初期値のままであること
    act(() => {
      result.current.dispatch({ type: "CLEAR_SELECTED_FEED" });
    });

    expect(result.current.state.selectedFeedId).toBeNull();
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
    expect(result.current.state.selectedView).toBe("feed");
  });

  it("CLEAR_SELECTED_FEED アクション導入後も既存 SELECT_FEED の挙動が変わらないこと（NFR 1.1 回帰）", () => {
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
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-a" });
    });
    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "starred" });
    });

    // SELECT_FEED は引き続き feedId を string で受け、関連状態をリセットする
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-x" });
    });

    expect(result.current.state.selectedFeedId).toBe("feed-x");
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
    expect(result.current.state.selectedView).toBe("feed");
  });

  it("CLEAR_SELECTED_FEED アクション導入後も既存 EXPAND_ITEM のトグル挙動が変わらないこと（NFR 1.1 回帰）", () => {
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

    // 同じ ID でトグル off
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    expect(result.current.state.expandedItemId).toBeNull();

    // 異なる ID で排他的展開
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-2" });
    });
    expect(result.current.state.expandedItemId).toBe("item-2");
  });

  it("CLEAR_SELECTED_FEED アクション導入後も既存 SET_FILTER の挙動が変わらないこと（NFR 1.1 回帰）", () => {
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

    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "unread" });
    });
    expect(result.current.state.filter).toBe("unread");
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

  // --- Issue #121 task 6: 横断新着一覧 (cross-feed timeline) reducer 追加分 ---

  it("SELECT_ALL_NEW_ITEMS アクションで viewMode が 'cross-feed' に遷移し他フィールドがリセットされること（Req 1.2, 4.7）", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 事前に個別フィード選択 / 記事展開 / フィルタ変更を行う
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    act(() => {
      result.current.dispatch({ type: "EXPAND_ITEM", itemId: "item-1" });
    });
    act(() => {
      result.current.dispatch({ type: "SET_FILTER", filter: "unread" });
    });

    expect(result.current.state.viewMode).toBe("feed");
    expect(result.current.state.selectedFeedId).toBe("feed-1");
    expect(result.current.state.expandedItemId).toBe("item-1");
    expect(result.current.state.filter).toBe("unread");

    // SELECT_ALL_NEW_ITEMS で viewMode='cross-feed' に切替、selectedFeedId / expandedItemId / filter がリセット
    act(() => {
      result.current.dispatch({ type: "SELECT_ALL_NEW_ITEMS" });
    });

    expect(result.current.state.viewMode).toBe("cross-feed");
    expect(result.current.state.selectedFeedId).toBeNull();
    expect(result.current.state.expandedItemId).toBeNull();
    expect(result.current.state.filter).toBe("all");
  });

  it("SELECT_FEED アクションで viewMode が 'feed' に遷移すること（Req 1.3）", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // 初期状態は viewMode='none'
    expect(result.current.state.viewMode).toBe("none");

    // SELECT_FEED で viewMode='feed' になる
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-99" });
    });

    expect(result.current.state.viewMode).toBe("feed");
    expect(result.current.state.selectedFeedId).toBe("feed-99");
  });

  it("SELECT_ALL_NEW_ITEMS → SELECT_FEED の遷移で crossFeedSessionSince が保持されること（Req 4.7）", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    // crossFeedSessionSince を固定
    act(() => {
      result.current.dispatch({
        type: "SET_CROSS_FEED_SESSION_SINCE",
        sinceTime: "2026-05-27T00:00:00Z",
      });
    });
    expect(result.current.state.crossFeedSessionSince).toBe(
      "2026-05-27T00:00:00Z"
    );

    // SELECT_ALL_NEW_ITEMS でも保持される
    act(() => {
      result.current.dispatch({ type: "SELECT_ALL_NEW_ITEMS" });
    });
    expect(result.current.state.crossFeedSessionSince).toBe(
      "2026-05-27T00:00:00Z"
    );
    expect(result.current.state.viewMode).toBe("cross-feed");

    // SELECT_FEED でも保持される（行き来時の baseline 固定検証）
    act(() => {
      result.current.dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
    });
    expect(result.current.state.crossFeedSessionSince).toBe(
      "2026-05-27T00:00:00Z"
    );
    expect(result.current.state.viewMode).toBe("feed");

    // 再度 SELECT_ALL_NEW_ITEMS でも保持
    act(() => {
      result.current.dispatch({ type: "SELECT_ALL_NEW_ITEMS" });
    });
    expect(result.current.state.crossFeedSessionSince).toBe(
      "2026-05-27T00:00:00Z"
    );
  });

  it("SET_CROSS_FEED_SESSION_SINCE アクションで crossFeedSessionSince が指定値に固定されること（Req 4.7）", () => {
    const wrapper = createWrapper();

    const { result } = renderHook(
      () => {
        const state = useAppState();
        const dispatch = useAppDispatch();
        return { state, dispatch };
      },
      { wrapper }
    );

    expect(result.current.state.crossFeedSessionSince).toBeNull();

    act(() => {
      result.current.dispatch({
        type: "SET_CROSS_FEED_SESSION_SINCE",
        sinceTime: "2026-05-28T12:00:00Z",
      });
    });

    expect(result.current.state.crossFeedSessionSince).toBe(
      "2026-05-28T12:00:00Z"
    );

    // 別 sinceTime で上書き
    act(() => {
      result.current.dispatch({
        type: "SET_CROSS_FEED_SESSION_SINCE",
        sinceTime: "2026-05-29T00:00:00Z",
      });
    });
    expect(result.current.state.crossFeedSessionSince).toBe(
      "2026-05-29T00:00:00Z"
    );
  });
});
