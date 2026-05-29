import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { StarredNavItem } from "./starred-nav-item";
import {
  AppStateProvider,
  useAppDispatch,
  useAppState,
} from "@/contexts/app-state";
import { useEffect } from "react";

/**
 * dispatch を外部に露出させて任意のアクションをテスト中に発火させるためのヘルパー。
 * AppStateProvider 配下にレンダリングし、`onReady` callback で
 * `useAppDispatch` の返値（dispatch 関数）と現在 state を呼び出し側に渡す。
 */
function StateProbe({
  onReady,
}: {
  onReady: (
    dispatch: ReturnType<typeof useAppDispatch>,
    state: ReturnType<typeof useAppState>
  ) => void;
}) {
  const dispatch = useAppDispatch();
  const state = useAppState();
  useEffect(() => {
    onReady(dispatch, state);
  }, [dispatch, state, onReady]);
  return null;
}

describe("StarredNavItem", () => {
  it("「お気に入り」テキストと Star アイコンを表示すること", () => {
    render(
      <AppStateProvider>
        <StarredNavItem />
      </AppStateProvider>
    );

    const button = screen.getByTestId("starred-nav-item");
    expect(button).toBeInTheDocument();
    expect(button).toHaveTextContent("お気に入り");
    // Star アイコン（lucide-react は SVG として描画される）
    const svg = button.querySelector("svg");
    expect(svg).not.toBeNull();
  });

  it("初期状態（selectedView='feed'）ではアクティブクラスが付与されないこと", () => {
    render(
      <AppStateProvider>
        <StarredNavItem />
      </AppStateProvider>
    );

    const button = screen.getByTestId("starred-nav-item");
    expect(button.dataset.selected).toBe("false");
    expect(button.className).not.toMatch(/bg-accent text-accent-foreground font-medium/);
  });

  it("クリックすると SELECT_STARRED アクションが dispatch されて selectedView が 'starred' に遷移すること", async () => {
    const user = userEvent.setup();

    let latestDispatch: ReturnType<typeof useAppDispatch> | null = null;
    let latestState: ReturnType<typeof useAppState> | null = null;

    render(
      <AppStateProvider>
        <StarredNavItem />
        <StateProbe
          onReady={(dispatch, state) => {
            latestDispatch = dispatch;
            latestState = state;
          }}
        />
      </AppStateProvider>
    );

    // 初期 state は selectedView = "feed"
    expect(latestState?.selectedView).toBe("feed");

    const button = screen.getByTestId("starred-nav-item");
    await user.click(button);

    expect(latestState?.selectedView).toBe("starred");
    expect(latestState?.selectedFeedId).toBeNull();
    // dispatch 経由で再操作可能（参照確保のためのチェック）
    expect(latestDispatch).not.toBeNull();
  });

  it("selectedView='starred' のとき既存 feed-list と同一のアクティブクラスが付与されること", () => {
    let latestDispatch: ReturnType<typeof useAppDispatch> | null = null;

    render(
      <AppStateProvider>
        <StarredNavItem />
        <StateProbe
          onReady={(dispatch) => {
            latestDispatch = dispatch;
          }}
        />
      </AppStateProvider>
    );

    // SELECT_STARRED を dispatch してアクティブ状態に遷移
    act(() => {
      latestDispatch?.({ type: "SELECT_STARRED" });
    });

    const button = screen.getByTestId("starred-nav-item");
    expect(button.dataset.selected).toBe("true");
    // feed-list.tsx と同一のアクティブクラス（要件 1.2 / 1.4）
    expect(button.className).toMatch(/bg-accent/);
    expect(button.className).toMatch(/text-accent-foreground/);
    expect(button.className).toMatch(/font-medium/);
  });

  it("selectedView が 'starred' から 'feed' に戻るとアクティブクラスが解除されること", () => {
    let latestDispatch: ReturnType<typeof useAppDispatch> | null = null;

    render(
      <AppStateProvider>
        <StarredNavItem />
        <StateProbe
          onReady={(dispatch) => {
            latestDispatch = dispatch;
          }}
        />
      </AppStateProvider>
    );

    // 一度 starred にしてから SELECT_FEED で feed に戻す
    act(() => {
      latestDispatch?.({ type: "SELECT_STARRED" });
    });
    let button = screen.getByTestId("starred-nav-item");
    expect(button.dataset.selected).toBe("true");

    act(() => {
      latestDispatch?.({ type: "SELECT_FEED", feedId: "feed-1" });
    });

    button = screen.getByTestId("starred-nav-item");
    expect(button.dataset.selected).toBe("false");
    expect(button.className).not.toMatch(/bg-accent text-accent-foreground font-medium/);
  });
});
