import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { ItemList } from "./item-list";
import {
  AppStateProvider,
  useAppDispatch,
  type AppAction,
} from "@/contexts/app-state";
import type { ItemDetail, ItemListResponse } from "@/types/item";
import type { Subscription } from "@/types/feed";
import type { ReactNode } from "react";

/** テスト用の購読一覧（手動更新ボタン用 subscription.id 解決のため） */
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

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserverのモック
const mockIntersectionObserver = vi.fn();
mockIntersectionObserver.mockImplementation((callback: IntersectionObserverCallback) => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
global.IntersectionObserver = mockIntersectionObserver;

/** テスト用ラッパー
 *
 * ItemList は FeedSearchBar（useAppState 経由で AppState を参照）を内包するため、
 * AppStateProvider 配下でレンダする必要がある。
 */
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <AppStateProvider>{children}</AppStateProvider>
      </QueryClientProvider>
    );
  };
}

/**
 * AppStateProvider に初期 dispatch（例: SELECT_FEED）を流してから ItemList をマウントする
 * ヘルパー。FeedSearchBar の selectedFeedId 連動を検証する場合に使用する。
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
  function Probe() {
    const dispatch = useAppDispatch();
    const ready = useDispatchOnce(() => {
      if (initialDispatch) initialDispatch(dispatch);
    });
    return ready ? <>{ui}</> : null;
  }
  return render(
    <QueryClientProvider client={queryClient}>
      <AppStateProvider>
        <Probe />
      </AppStateProvider>
    </QueryClientProvider>
  );
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

/** テスト用の記事一覧レスポンス */
const mockItemsResponse: ItemListResponse = {
  items: [
    {
      id: "item-1",
      feed_id: "feed-1",
      title: "最新の記事タイトル",
      link: "https://example.com/article-1",
      summary: "最新記事の概要テキストです。",
      published_at: "2026-02-27T10:00:00Z",
      is_date_estimated: false,
      is_read: false,
      is_starred: false,
      hatebu_count: 10,
      hatebu_fetched_at: "2026-02-27T09:00:00Z",
    },
    {
      id: "item-2",
      feed_id: "feed-1",
      title: "推定日付の記事",
      link: "https://example.com/article-2",
      summary: "",
      published_at: "2026-02-26T10:00:00Z",
      is_date_estimated: true,
      is_read: true,
      is_starred: true,
      hatebu_count: 0,
      hatebu_fetched_at: null,
    },
  ],
  next_cursor: null,
  has_more: false,
};

/**
 * mockFetchの設定ヘルパー
 */
function setupMockFetch(response: ItemListResponse = mockItemsResponse) {
  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url.includes("/api/feeds/feed-1/items")) {
      return Promise.resolve({
        ok: true,
        json: async () => response,
      });
    }
    if (typeof url === "string" && url === "/api/subscriptions") {
      return Promise.resolve({
        ok: true,
        json: async () => mockSubscriptions,
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({}),
    });
  });
}

/** テスト用の記事詳細レスポンス（item-1 用） */
const mockItemDetail: ItemDetail = {
  id: "item-1",
  feed_id: "feed-1",
  title: "最新の記事タイトル",
  link: "https://example.com/article-1",
  published_at: "2026-02-27T10:00:00Z",
  is_date_estimated: false,
  is_read: false,
  is_starred: false,
  hatebu_count: 10,
  hatebu_fetched_at: "2026-02-27T09:00:00Z",
  content: "<p>これは記事の本文です</p>",
  summary: "記事の要約",
  author: "著者名",
};

/**
 * 一覧・詳細・状態更新をまとめてルーティングする mockFetch 設定ヘルパー。
 *
 * @param options.detailItemId - 詳細取得のキーとなる記事ID（既定 item-1）
 * @param options.detail - 返す記事詳細（既定 mockItemDetail）
 * @param options.detailFails - true の場合、詳細取得を 500 で失敗させる
 * @param options.detailDelayMs - 詳細取得レスポンスを遅延させる（ローディング検証用）
 */
function setupMockFetchWithDetail(options?: {
  detailItemId?: string;
  detail?: ItemDetail;
  detailFails?: boolean;
  detailDelayMs?: number;
}) {
  const detailItemId = options?.detailItemId ?? "item-1";
  const detail = options?.detail ?? mockItemDetail;

  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url.includes("/api/feeds/feed-1/items")) {
      return Promise.resolve({ ok: true, json: async () => mockItemsResponse });
    }
    if (typeof url === "string" && url === "/api/subscriptions") {
      return Promise.resolve({ ok: true, json: async () => mockSubscriptions });
    }
    if (typeof url === "string" && url === `/api/items/${detailItemId}`) {
      if (options?.detailFails) {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ message: "Internal Server Error" }),
        });
      }
      if (options?.detailDelayMs) {
        return new Promise((resolve) =>
          setTimeout(
            () => resolve({ ok: true, json: async () => detail }),
            options.detailDelayMs
          )
        );
      }
      return Promise.resolve({ ok: true, json: async () => detail });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

describe("ItemList コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("feedIdが指定されていない場合は案内メッセージを表示すること", () => {
    render(
      <ItemList
        feedId={null}
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("フィードを選択してください")).toBeInTheDocument();
  });

  it("記事一覧がpublished_at降順で表示されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    expect(screen.getByText("推定日付の記事")).toBeInTheDocument();

    // 順序確認: 最新の記事が先に表示される
    const items = screen.getAllByTestId(/^item-row-/);
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("最新の記事タイトル")).toBeInTheDocument();
    expect(within(items[1]).getByText("推定日付の記事")).toBeInTheDocument();
  });

  it("推定フラグ付き日付には推定マークが表示されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("推定日付の記事")).toBeInTheDocument();
    });

    // 推定日付の記事に推定マーカーが表示されること
    const estimatedItem = screen.getByTestId("item-row-item-2");
    expect(within(estimatedItem).getByTestId("date-estimated")).toBeInTheDocument();
  });

  it("フィルタ切替UI（全て/未読/スター）が表示されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });

    expect(screen.getByRole("tab", { name: "未読" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "スター" })).toBeInTheDocument();
  });

  it("フィルタを切り替えるとAPIにフィルタパラメータが送信されること", async () => {
    const user = userEvent.setup();

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "未読" })).toBeInTheDocument();
    });

    // 「未読」フィルタをクリック
    await user.click(screen.getByRole("tab", { name: "未読" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("filter=unread"),
        expect.any(Object)
      );
    });
  });

  it("記事をクリックするとonSelectItemが呼ばれること", async () => {
    const user = userEvent.setup();
    const onSelectItem = vi.fn();

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={onSelectItem}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("item-row-item-1"));

    expect(onSelectItem).toHaveBeenCalledWith("item-1");
  });

  it("既読記事は視覚的に区別されること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByTestId("item-row-item-2")).toBeInTheDocument();
    });

    // 既読記事にはdata-read属性がtrueであること
    const readItem = screen.getByTestId("item-row-item-2");
    expect(readItem).toHaveAttribute("data-read", "true");

    // 未読記事にはdata-read属性がfalseであること
    const unreadItem = screen.getByTestId("item-row-item-1");
    expect(unreadItem).toHaveAttribute("data-read", "false");
  });

  it("無限スクロール用のsentinelが存在すること", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    // IntersectionObserver用のsentinelが存在すること
    expect(screen.getByTestId("scroll-sentinel")).toBeInTheDocument();
  });

  it("記事が0件の場合に空の状態を表示すること", async () => {
    setupMockFetch({
      items: [],
      next_cursor: null,
      has_more: false,
    });

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("記事がありません")).toBeInTheDocument();
    });
  });

  it("タイトルが元記事への外部リンクであり、新規タブで開くこと", async () => {
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    const link = screen.getByRole("link", { name: "最新の記事タイトル" });
    expect(link).toHaveAttribute("href", "https://example.com/article-1");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("タイトルリンクをクリックした際に親行のクリックイベントが伝搬しないこと", async () => {
    const user = userEvent.setup();
    const onSelectItem = vi.fn();

    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={onSelectItem}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    const link = screen.getByRole("link", { name: "最新の記事タイトル" });
    await user.click(link);

    // e.stopPropagation()により、onSelectItemが呼び出されないこと
    expect(onSelectItem).not.toHaveBeenCalled();
  });

  // --- 概要表示 (Requirement 2) ---

  it("概要があるとき記事行のタイトル直下に概要が表示されること", async () => {
    // Arrange / Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    const row = screen.getByTestId("item-row-item-1");
    const summary = within(row).getByTestId("item-summary-item-1");
    expect(summary).toHaveTextContent("最新記事の概要テキストです。");
  });

  it("概要が空のとき概要領域を描画しないこと", async () => {
    // Arrange / Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    await waitFor(() => {
      expect(screen.getByTestId("item-row-item-2")).toBeInTheDocument();
    });

    // item-2 は summary 空のため概要要素を描画しない
    expect(screen.queryByTestId("item-summary-item-2")).not.toBeInTheDocument();
  });

  it("概要テキストがタイトルより小さく薄い配色で表示されること", async () => {
    // Arrange / Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    const summary = screen.getByTestId("item-summary-item-1");
    // フォントサイズ縮小 (text-xs) と低コントラスト配色 (text-muted-foreground)
    expect(summary.className).toContain("text-xs");
    expect(summary.className).toContain("text-muted-foreground");
  });

  // --- 長い概要の省略 (Requirement 3) ---

  it("概要が最大2行で省略されるよう line-clamp-2 が適用されること", async () => {
    // Arrange / Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    const summary = screen.getByTestId("item-summary-item-1");
    expect(summary.className).toContain("line-clamp-2");
  });

  // --- タイムスタンプのタイトル右側配置 (Requirement 4) ---

  it("公開日時がタイトルと同一行の右側に配置されること", async () => {
    // Arrange / Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    const row = screen.getByTestId("item-row-item-1");
    const titleRow = within(row).getByTestId("item-title-row-item-1");
    const time = within(row).getByRole("time");
    const summary = within(row).getByTestId("item-summary-item-1");

    // 日時はタイトル行(同一行)に含まれる
    expect(titleRow).toContainElement(time as HTMLElement);
    // 概要はタイトル行の外（下）に配置され、日時を含まない
    expect(summary).not.toContainElement(time as HTMLElement);
  });

  it("推定日付の記事では推定フラグが日時に隣接して表示されること", async () => {
    // Arrange / Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    await waitFor(() => {
      expect(screen.getByTestId("item-row-item-2")).toBeInTheDocument();
    });

    const row = screen.getByTestId("item-row-item-2");
    const titleRow = within(row).getByTestId("item-title-row-item-2");
    const estimated = within(row).getByTestId("date-estimated");
    // 推定フラグはタイトル行（=日時のある行）に維持される
    expect(titleRow).toContainElement(estimated as HTMLElement);
  });

  // --- フィード内検索バーの表示制御 (Req 1.2 / NFR 2.3) ---

  it("フィード選択時はフィルタ群と同列に FeedSearchBar が表示されること（Req 1.2）", async () => {
    // Arrange: AppState で feed-1 を選択した状態にして ItemList を render
    renderWithInitialDispatch(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      (dispatch) => {
        dispatch({ type: "SELECT_FEED", feedId: "feed-1" });
      }
    );

    // Assert: FeedSearchBar が描画される（フィルタタブと同じ領域に併設）
    await waitFor(() => {
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
    });
    expect(screen.getByTestId("feed-search-input")).toBeInTheDocument();
    // フィルタタブも引き続き表示される（同列の隣接）
    expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
  });

  it("フィード未選択時（feedId === null）は FeedSearchBar が描画されないこと（NFR 2.3）", () => {
    // Arrange / Act: feedId を null で render（AppState の selectedFeedId も初期値 null）
    render(
      <ItemList feedId={null} onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert: ItemList 自体が案内メッセージのみで FeedSearchBar に到達しない
    expect(screen.getByText("フィードを選択してください")).toBeInTheDocument();
    expect(screen.queryByTestId("feed-search-bar")).not.toBeInTheDocument();
    expect(screen.queryByTestId("feed-search-input")).not.toBeInTheDocument();
  });
});

describe("ItemList コンポーネント: 記事詳細の展開表示", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("選択中の記事行の直下に記事詳細エリアを展開表示すること", async () => {
    // Arrange
    setupMockFetchWithDetail();

    // Act: expandedItemId に item-1 を渡して展開状態でレンダリング
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    // Assert: 本文が取得され表示される（AC 1.2, 2.4）
    await waitFor(() => {
      expect(
        screen.getByText("これは記事の本文です")
      ).toBeInTheDocument();
    });

    // 詳細エリアが選択行 item-1 の直後に兄弟要素として配置されること（AC 1.1）
    const row = screen.getByTestId("item-row-item-1");
    const content = screen.getByTestId("item-content");
    expect(row.nextElementSibling).toContainElement(content);
    // ItemDetail が button の内側にネストされていないこと
    expect(row).not.toContainElement(content);
  });

  it("展開中の記事詳細にスター切替・はてブ数・元記事リンクが表示されること", async () => {
    // Arrange
    setupMockFetchWithDetail();

    // Act
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    // Assert: AC 1.3
    await waitFor(() => {
      expect(screen.getByTestId("star-toggle")).toBeInTheDocument();
    });
    expect(screen.getByTestId("hatebu-count")).toBeInTheDocument();
    expect(screen.getByTestId("original-link")).toBeInTheDocument();
  });

  it("いずれの記事も選択されていない場合は記事詳細エリアを表示しないこと", async () => {
    // Arrange
    setupMockFetchWithDetail();

    // Act: expandedItemId が null
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    // Assert: AC 1.5 / 詳細取得もしないこと
    expect(screen.queryByTestId("item-content")).not.toBeInTheDocument();
    expect(screen.queryByTestId("item-detail-loading")).not.toBeInTheDocument();
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/items/item-1",
      expect.any(Object)
    );
  });

  it("記事詳細の取得が完了していない間はローディング表示を提示すること", async () => {
    // Arrange: 詳細取得を遅延させる
    setupMockFetchWithDetail({ detailDelayMs: 1000 });

    // Act
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    // Assert: AC 2.2 / NFR 2.1（取得完了を待たず展開枠を表示）
    await waitFor(() => {
      expect(screen.getByTestId("item-detail-loading")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("item-content")).not.toBeInTheDocument();
  });

  it("記事詳細の取得に失敗した場合はエラー表示を提示すること", async () => {
    // Arrange
    setupMockFetchWithDetail({ detailFails: true });

    // Act
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    // Assert: AC 2.3
    await waitFor(() => {
      expect(screen.getByTestId("item-detail-error")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("item-content")).not.toBeInTheDocument();
  });

  it("未読記事の詳細を展開すると既読化リクエストを送信すること", async () => {
    // Arrange
    setupMockFetchWithDetail();

    // Act
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    // Assert: AC 3.1（is_read: false の item-1 を展開 → 既読化 PUT が送信される）
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/items/item-1/state",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ is_read: true }),
        })
      );
    });
  });

  it("既読記事の詳細を展開しても既読化リクエストを送信しないこと", async () => {
    // Arrange: 既読の item-2 を詳細として返す
    const readDetail: ItemDetail = {
      ...mockItemDetail,
      id: "item-2",
      is_read: true,
      content: "<p>既読記事の本文</p>",
    };
    setupMockFetchWithDetail({ detailItemId: "item-2", detail: readDetail });

    // Act
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-2"
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("既読記事の本文")).toBeInTheDocument();
    });

    // Assert: AC 3.2（既読化 PUT は送信されない）
    expect(mockFetch).not.toHaveBeenCalledWith(
      "/api/items/item-2/state",
      expect.objectContaining({ method: "PUT" })
    );
  });

  it("詳細のスター切替ボタン押下でスター反転の更新リクエストを送信すること", async () => {
    // Arrange
    const user = userEvent.setup();
    setupMockFetchWithDetail();

    // Act
    render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByTestId("star-toggle")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("star-toggle"));

    // Assert: AC 4.1（is_starred: false → true への反転を要求）
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/items/item-1/state",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ is_starred: true }),
        })
      );
    });
  });

  it("別の記事を選択すると直前の詳細を閉じて新たな記事詳細を展開すること", async () => {
    // Arrange: item-1 と item-2 の詳細を両方ルーティングする
    const detail2: ItemDetail = {
      ...mockItemDetail,
      id: "item-2",
      title: "推定日付の記事",
      is_read: true,
      content: "<p>二番目の記事本文</p>",
    };
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.includes("/api/feeds/feed-1/items")) {
        return Promise.resolve({
          ok: true,
          json: async () => mockItemsResponse,
        });
      }
      if (typeof url === "string" && url === "/api/subscriptions") {
        return Promise.resolve({ ok: true, json: async () => mockSubscriptions });
      }
      if (url === "/api/items/item-1") {
        return Promise.resolve({ ok: true, json: async () => mockItemDetail });
      }
      if (url === "/api/items/item-2") {
        return Promise.resolve({ ok: true, json: async () => detail2 });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });

    // Act: 最初は item-1 を展開
    const { rerender } = render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("これは記事の本文です")).toBeInTheDocument();
    });

    // expandedItemId を item-2 に切り替えて再レンダリング（排他トグルは props 制御）
    rerender(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-2"
      />
    );

    // Assert: AC 5.3 / 5.4（直前の詳細が閉じ、新たな詳細のみ展開・同時 2 件以上展開しない）
    await waitFor(() => {
      expect(screen.getByText("二番目の記事本文")).toBeInTheDocument();
    });
    expect(screen.queryByText("これは記事の本文です")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("item-content")).toHaveLength(1);
  });

  it("展開中の記事を閉じる（expandedItemId=null）と詳細エリアが消えること", async () => {
    // Arrange
    setupMockFetchWithDetail();

    // Act: item-1 を展開
    const { rerender } = render(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId="item-1"
      />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByTestId("item-content")).toBeInTheDocument();
    });

    // 同じ行を再クリックして閉じた状態（expandedItemId=null）に切り替える
    rerender(
      <ItemList
        feedId="feed-1"
        onSelectItem={() => {}}
        expandedItemId={null}
      />
    );

    // Assert: AC 5.1 / 5.2（詳細が閉じる。ハイライト解除は data-state で確認）
    await waitFor(() => {
      expect(screen.queryByTestId("item-content")).not.toBeInTheDocument();
    });
  });
});

/**
 * 手動更新ボタン / バナー専用の mockFetch セットアップヘルパー。
 *
 * @param options.manualFetch - POST /api/subscriptions/sub-1/fetch のレスポンス制御
 *   - `success`: 200 OK
 *   - `cooldown`: 429 + FEED_COOLDOWN（details.retry_after_seconds=480）
 *   - `inProgress`: 409 + FEED_FETCH_IN_PROGRESS
 *   - `unauthorized`: 401 + UNAUTHORIZED
 *   - `serverError`: 503 + INTERNAL_ERROR
 *   - `networkError`: fetch reject（ネットワーク到達不能）
 *   - `pending`: 永久 pending（isPending 検証用）
 */
function setupMockFetchForManualRefresh(options?: {
  manualFetch?:
    | "success"
    | "cooldown"
    | "inProgress"
    | "unauthorized"
    | "serverError"
    | "networkError"
    | "pending";
}) {
  const manualFetchKind = options?.manualFetch ?? "success";

  mockFetch.mockImplementation((url: string, init?: RequestInit) => {
    if (
      typeof url === "string" &&
      url === "/api/subscriptions/sub-1/fetch" &&
      init?.method === "POST"
    ) {
      switch (manualFetchKind) {
        case "success":
          return Promise.resolve({ ok: true, json: async () => ({ id: "sub-1" }) });
        case "cooldown":
          return Promise.resolve({
            ok: false,
            status: 429,
            json: async () => ({
              error: {
                code: "FEED_COOLDOWN",
                message: "cooldown",
                category: "feed",
                action: "wait",
                details: { retry_after_seconds: 480 },
              },
            }),
          });
        case "inProgress":
          return Promise.resolve({
            ok: false,
            status: 409,
            json: async () => ({
              error: {
                code: "FEED_FETCH_IN_PROGRESS",
                message: "in progress",
                category: "feed",
                action: "wait",
              },
            }),
          });
        case "unauthorized":
          return Promise.resolve({
            ok: false,
            status: 401,
            json: async () => ({
              error: {
                code: "UNAUTHORIZED",
                message: "session expired",
                category: "auth",
                action: "login",
              },
            }),
          });
        case "serverError":
          return Promise.resolve({
            ok: false,
            status: 503,
            json: async () => ({
              error: {
                code: "INTERNAL_ERROR",
                message: "internal",
                category: "system",
                action: "retry",
              },
            }),
          });
        case "networkError":
          return Promise.reject(new TypeError("Failed to fetch"));
        case "pending":
          return new Promise(() => {
            /* never resolve */
          });
      }
    }
    if (typeof url === "string" && url.includes("/api/feeds/feed-1/items")) {
      return Promise.resolve({ ok: true, json: async () => mockItemsResponse });
    }
    if (typeof url === "string" && url === "/api/subscriptions") {
      return Promise.resolve({ ok: true, json: async () => mockSubscriptions });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

describe("ItemList コンポーネント: 手動更新ボタン", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("フィード選択時に「フィードを更新」ボタンが表示されること（Req 5.1）", async () => {
    // Arrange
    setupMockFetchForManualRefresh();

    // Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert: subscription 解決待ちの後、ボタンが描画される
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "フィードを更新" })
      ).toBeInTheDocument();
    });
  });

  it("ボタンクリックで POST /api/subscriptions/:id/fetch が発火すること（Req 5.3）", async () => {
    // Arrange
    setupMockFetchForManualRefresh();
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "フィードを更新" })
      ).toBeInTheDocument();
    });

    // Act
    await user.click(screen.getByRole("button", { name: "フィードを更新" }));

    // Assert
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/subscriptions/sub-1/fetch",
        expect.objectContaining({ method: "POST" })
      );
    });
  });

  it("mutation 進行中はボタンが disabled + animate-spin になること（Req 5.4 / 5.6）", async () => {
    // Arrange: 永久 pending で進行中状態を維持する
    setupMockFetchForManualRefresh({ manualFetch: "pending" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    // Act
    await user.click(button);

    // Assert: disabled になり、内部の RotateCw アイコンに animate-spin が付く
    await waitFor(() => {
      expect(button).toBeDisabled();
    });
    expect(button).toHaveAttribute("aria-busy", "true");
    const icon = button.querySelector("svg");
    expect(icon).not.toBeNull();
    expect(icon?.getAttribute("class") ?? "").toContain("animate-spin");
  });

  it("初期状態（mutation 未発火）ではバナーが表示されないこと", async () => {
    // Arrange
    setupMockFetchForManualRefresh();

    // Act
    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    await waitFor(() => {
      expect(screen.getByText("最新の記事タイトル")).toBeInTheDocument();
    });

    // Assert
    expect(screen.queryByTestId("manual-refresh-banner")).not.toBeInTheDocument();
  });

  it("成功時にバナーが表示されないこと", async () => {
    // Arrange
    setupMockFetchForManualRefresh({ manualFetch: "success" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    // Act
    await user.click(button);
    await waitFor(() => expect(button).not.toBeDisabled());

    // Assert
    expect(screen.queryByTestId("manual-refresh-banner")).not.toBeInTheDocument();
  });

  it("429 エラー時にクールダウン残り秒数を含むバナーが表示されること（Req 7.1）", async () => {
    // Arrange
    setupMockFetchForManualRefresh({ manualFetch: "cooldown" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    // Act
    await user.click(button);

    // Assert
    await waitFor(() => {
      expect(screen.getByTestId("manual-refresh-banner")).toBeInTheDocument();
    });
    expect(screen.getByTestId("manual-refresh-banner")).toHaveTextContent(
      /480 秒以内のため再試行できません/
    );
  });

  it("409 エラー時に「現在フェッチ中です」バナーが表示されること（Req 7.2）", async () => {
    setupMockFetchForManualRefresh({ manualFetch: "inProgress" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    await user.click(button);

    await waitFor(() => {
      expect(screen.getByTestId("manual-refresh-banner")).toHaveTextContent(
        /現在フェッチ中です/
      );
    });
  });

  it("401 エラー時にセッション切れバナーが表示されること（Req 7.3）", async () => {
    setupMockFetchForManualRefresh({ manualFetch: "unauthorized" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    await user.click(button);

    await waitFor(() => {
      expect(screen.getByTestId("manual-refresh-banner")).toHaveTextContent(
        /セッションが切れました/
      );
    });
  });

  it("5xx エラー時に一時的失敗バナーが表示されること（Req 7.4）", async () => {
    setupMockFetchForManualRefresh({ manualFetch: "serverError" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    await user.click(button);

    await waitFor(() => {
      expect(screen.getByTestId("manual-refresh-banner")).toHaveTextContent(
        /一時的なエラーが発生しました/
      );
    });
  });

  it("ネットワークエラー時に一時的失敗バナーが表示されること（Req 7.4）", async () => {
    setupMockFetchForManualRefresh({ manualFetch: "networkError" });
    const user = userEvent.setup();

    render(
      <ItemList feedId="feed-1" onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    const button = await waitFor(() =>
      screen.getByRole("button", { name: "フィードを更新" })
    );

    await user.click(button);

    await waitFor(() => {
      expect(screen.getByTestId("manual-refresh-banner")).toHaveTextContent(
        /一時的なエラーが発生しました/
      );
    });
  });

  it("feedId=null のときはボタンを描画しないこと（Req 5.2）", () => {
    // Arrange / Act: ItemList は feedId=null で早期 return するため、ボタンは構造的に存在しない
    setupMockFetchForManualRefresh();
    render(
      <ItemList feedId={null} onSelectItem={() => {}} expandedItemId={null} />,
      { wrapper: createWrapper() }
    );

    // Assert
    expect(
      screen.queryByRole("button", { name: "フィードを更新" })
    ).not.toBeInTheDocument();
  });
});
