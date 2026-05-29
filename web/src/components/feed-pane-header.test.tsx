import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { FeedPaneHeader } from "./feed-pane-header";
import {
  AppStateProvider,
  useAppDispatch,
  type AppAction,
} from "@/contexts/app-state";
import type { Subscription } from "@/types/feed";

/**
 * テスト用の購読一覧。
 * `FeedPaneHeader` 内部の `useFeeds` が解決する `subscription.id` のソースとなり、
 * `ManualRefreshButton` の描画条件を満たすために用いる。
 */
const mockSubscriptions: Subscription[] = [
  {
    id: "sub-1",
    user_id: "user-1",
    feed_id: "feed-1",
    feed_title: "テストフィード",
    feed_url: "https://example.com/feed.xml",
    favicon_url: null,
    fetch_interval_minutes: 60,
    feed_status: "active",
    error_message: null,
    unread_count: 1,
    created_at: "2026-02-27T00:00:00Z",
  },
];

// グローバル fetch のモック（useFeeds の `/api/subscriptions` を満たすため）。
const mockFetch = vi.fn();
global.fetch = mockFetch;

/**
 * `/api/subscriptions` を返すだけのシンプルな fetch モックセットアップ。
 * 他の API は使わない（FeedPaneHeader は記事一覧 fetch を呼ばない）。
 */
function setupMockFetch() {
  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url === "/api/subscriptions") {
      return Promise.resolve({
        ok: true,
        json: async () => mockSubscriptions,
      });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

/**
 * `AppStateProvider` 配下で対象 UI を render し、初回 mount で 1 度だけ
 * `initialDispatch` を実行するヘルパー（`FeedSearchBar` 内部の `selectedFeedId`
 * 連動を成立させるため、フィード選択を初期 dispatch で行う）。
 */
function renderWithInitialDispatch(
  ui: ReactNode,
  initialDispatch?: (dispatch: (action: AppAction) => void) => void
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  // Probe 内で current ui を ref 経由で参照することで、render 後の rerender でも
  // initialDispatch / AppStateProvider / QueryClientProvider を再構築せず、AppState と
  // 内部マウント済みコンポーネントの identity を維持したまま children だけ差し替える。
  let currentUi: ReactNode = ui;
  function Probe() {
    const dispatch = useAppDispatch();
    const ready = useDispatchOnce(() => {
      if (initialDispatch) initialDispatch(dispatch);
    });
    return ready ? <>{currentUi}</> : null;
  }
  const Wrappers = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <AppStateProvider>{children}</AppStateProvider>
    </QueryClientProvider>
  );
  const utils = render(
    <Wrappers>
      <Probe />
    </Wrappers>
  );
  return {
    ...utils,
    /** 同じ QueryClient / AppState ツリーを維持したまま、Probe 配下の ui だけ差し替える */
    rerenderUi(nextUi: ReactNode) {
      currentUi = nextUi;
      utils.rerender(
        <Wrappers>
          <Probe />
        </Wrappers>
      );
    },
  };
}

/** 初回 mount でのみ effect を 1 度発火するテスト専用ヘルパー */
function useDispatchOnce(fn: () => void): boolean {
  const [done, setDone] = useState(false);
  useEffect(() => {
    fn();
    setDone(true);
    // 初回 mount のみ発火 / deps は意図的に空配列
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return done;
}

describe("FeedPaneHeader コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  // --- mode="normal" (Req 3.4) ---

  describe("mode='normal'", () => {
    it("FilterTabs / FeedSearchBar / ManualRefreshButton の 3 要素が描画されること", async () => {
      // Arrange / Act: selectedFeedId を feed-1 にして、FeedPaneHeader を mode='normal' で render
      renderWithInitialDispatch(
        <FeedPaneHeader mode="normal" feedId="feed-1" filter="all" onFilterChange={() => {}} />,
        (dispatch) => {
          dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
        }
      );

      // Assert: FilterTabs (全て / 未読 / スター)
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });
      expect(screen.getByRole("tab", { name: "未読" })).toBeInTheDocument();
      expect(screen.getByRole("tab", { name: "スター" })).toBeInTheDocument();

      // Assert: FeedSearchBar
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
      expect(screen.getByTestId("feed-search-input")).toBeInTheDocument();

      // Assert: ManualRefreshButton（subscription.id 解決後に描画される）
      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: "フィードを更新" })
        ).toBeInTheDocument();
      });
    });

    it("タブ切替で onFilterChange が選択値とともに呼ばれること", async () => {
      // Arrange
      const onFilterChange = vi.fn();
      const user = userEvent.setup();
      renderWithInitialDispatch(
        <FeedPaneHeader
          mode="normal"
          feedId="feed-1"
          filter="all"
          onFilterChange={onFilterChange}
        />,
        (dispatch) => {
          dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
        }
      );

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "未読" })).toBeInTheDocument();
      });

      // Act: 「未読」タブをクリック
      await user.click(screen.getByRole("tab", { name: "未読" }));

      // Assert
      expect(onFilterChange).toHaveBeenCalledWith("unread");
    });
  });

  // --- mode="search-feed" (Req 1.1, 2.3) ---

  describe("mode='search-feed'", () => {
    it("FeedSearchBar のみ描画され、FilterTabs / ManualRefreshButton は描画されないこと", async () => {
      // Arrange / Act
      renderWithInitialDispatch(
        <FeedPaneHeader mode="search-feed" feedId="feed-1" />,
        (dispatch) => {
          dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
        }
      );

      // Assert: FeedSearchBar は描画される
      await waitFor(() => {
        expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
      });
      expect(screen.getByTestId("feed-search-input")).toBeInTheDocument();

      // Assert: FilterTabs は描画されない
      expect(screen.queryByRole("tab", { name: "全て" })).not.toBeInTheDocument();
      expect(screen.queryByRole("tab", { name: "未読" })).not.toBeInTheDocument();
      expect(screen.queryByRole("tab", { name: "スター" })).not.toBeInTheDocument();

      // Assert: ManualRefreshButton は描画されない
      // useFeeds（/api/subscriptions）の解決が走り終わるまで waitFor で安定状態を待ち、
      // その上で ManualRefreshButton が一切現れていないことを確認する。
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith(
          "/api/subscriptions",
          expect.any(Object)
        );
      });
      expect(
        screen.queryByRole("button", { name: "フィードを更新" })
      ).not.toBeInTheDocument();
    });
  });

  // --- mode 切替時の FeedSearchBar mount 維持 (Req 1.1) ---

  it("mode が 'normal' → 'search-feed' に切り替わっても feed-search-input が同一 DOM 要素として保持されること", async () => {
    // Arrange: 最初は mode='normal' で render
    const { rerenderUi } = renderWithInitialDispatch(
      <FeedPaneHeader
        mode="normal"
        feedId="feed-1"
        filter="all"
        onFilterChange={() => {}}
      />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
      }
    );

    await waitFor(() => {
      expect(screen.getByTestId("feed-search-input")).toBeInTheDocument();
    });
    const inputBefore = screen.getByTestId("feed-search-input");

    // Act: mode を 'search-feed' に切り替える
    rerenderUi(<FeedPaneHeader mode="search-feed" feedId="feed-1" />);

    // Assert: 同じ testid の input 要素が DOM 上に存在し、かつ
    // Element identity が切替前後で一致する（= React reconciliation で同一インスタンスとして
    // 保持されている。Req 1.1 の構造的担保）
    const inputAfter = screen.getByTestId("feed-search-input");
    expect(inputAfter).toBe(inputBefore);

    // 副次確認: 切替後は FilterTabs / ManualRefreshButton が消えていること
    expect(screen.queryByRole("tab", { name: "全て" })).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "フィードを更新" })
    ).not.toBeInTheDocument();
  });
});
