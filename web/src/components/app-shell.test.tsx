import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ThemeProvider } from "@/components/theme-provider";
import { AppStateProvider } from "@/contexts/app-state";
import { AppShell } from "./app-shell";
import type { Subscription } from "@/types/feed";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

// IntersectionObserverのモック
// new で呼ばれる前提のため class として実装する（vi.fn().mockImplementation だと
// "is not a constructor" になるため）。StarredItemList / ItemList が IntersectionObserver
// を new で生成するので両者に共通の mock を提供する。
class MockIntersectionObserver {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  constructor(_callback: IntersectionObserverCallback) {}
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}
global.IntersectionObserver =
  MockIntersectionObserver as unknown as typeof IntersectionObserver;

/** テスト用の購読データ */
const mockSubscriptions: Subscription[] = [
  {
    id: "sub-1",
    user_id: "user-1",
    feed_id: "feed-1",
    feed_title: "Tech Blog",
    feed_url: "https://example.com/feed.xml",
    favicon_url: "https://example.com/favicon.ico",
    fetch_interval_minutes: 60,
    feed_status: "active",
    unread_count: 5,
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "sub-2",
    user_id: "user-1",
    feed_id: "feed-2",
    feed_title: "News Feed",
    feed_url: "https://news.example.com/rss",
    favicon_url: null,
    fetch_interval_minutes: 30,
    feed_status: "active",
    unread_count: 0,
    created_at: "2026-01-02T00:00:00Z",
  },
];

/** テスト用ラッパー（全Provider含む） */
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
        <ThemeProvider>
          <AppStateProvider>{children}</AppStateProvider>
        </ThemeProvider>
      </QueryClientProvider>
    );
  };
}

/** Issue #121: 横断新着一覧 API の固定応答（test 用、Req 4.7 の since_time を保持） */
const CROSS_FEED_SINCE_TIME = "2026-05-26T10:00:00Z";

/** mockFetchの設定ヘルパー */
function setupMockFetch() {
  mockFetch.mockImplementation((url: string, options?: RequestInit) => {
    if (url === "/api/subscriptions") {
      return Promise.resolve({
        ok: true,
        json: async () => mockSubscriptions,
      });
    }
    // 横断新着一覧 API（GET /api/items/cross-feed）。`since=` パラメータの有無を
    // テストから観測できるよう常に固定 since_time を返す。
    if (typeof url === "string" && url.startsWith("/api/items/cross-feed")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          items: [],
          next_cursor: null,
          has_more: false,
          since_time: CROSS_FEED_SINCE_TIME,
        }),
      });
    }
    // PUT /api/users/me/cross-feed-last-seen の応答（Req 4.3）
    if (
      url === "/api/users/me/cross-feed-last-seen" &&
      options?.method === "PUT"
    ) {
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      });
    }
    // 横断スター / 単一フィード両方の記事一覧 API を空配列で返す
    if (typeof url === "string" && url.includes("/api/feeds/")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          items: [],
          next_cursor: null,
          has_more: false,
        }),
      });
    }
    // 横断検索 API（GET /api/items/search）を空結果で返す。
    // ItemSearchResponse の形（items / next_cursor / has_more）を満たさないと
    // SearchResults の allHits 構築で undefined 要素が混入しクラッシュするため、
    // 検索モードの空状態表示を検証するには正しい空レスポンス形が必要。
    if (typeof url === "string" && url.includes("/api/items/search")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          items: [],
          next_cursor: null,
          has_more: false,
        }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({}),
    });
  });
}

describe("AppShell コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("2ペインレイアウト（左ペイン + 右ペイン）がレンダリングされること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("app-shell")).toBeInTheDocument();
    });

    expect(screen.getByTestId("left-pane")).toBeInTheDocument();
    expect(screen.getByTestId("right-pane")).toBeInTheDocument();
  });

  it("左ペインにフィード一覧が表示されること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    });

    expect(screen.getByText("News Feed")).toBeInTheDocument();
  });

  it("フィード未選択時に右ペインに案内メッセージが表示されること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });
  });

  it("フィードをクリックすると右ペインに記事一覧が表示されること", async () => {
    const user = userEvent.setup();

    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    });

    // フィードをクリック
    await user.click(screen.getByText("Tech Blog"));

    // 記事一覧APIが呼ばれること（フィルタタブが表示されることで確認）
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });
  });

  it("アプリケーションヘッダーが表示されること", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Feedman")).toBeInTheDocument();
    });
  });

  it("左ペイン先頭に StarredNavItem「お気に入り」項目が表示されること（Req 1.1 / 1.3）", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("starred-nav-item")).toBeInTheDocument();
    });
    expect(screen.getByTestId("starred-nav-item")).toHaveTextContent(
      "お気に入り"
    );
  });

  it("「お気に入り」項目クリック → 右ペインが StarredItemList に切替 → フィード行クリック → 右ペインが ItemList に戻ること（Req 1.3 / 1.4 / 2.1 / 5.3）", async () => {
    const user = userEvent.setup();

    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("starred-nav-item")).toBeInTheDocument();
    });

    // 初期状態: 右ペインは「フィードを選択してください」（ItemList の feedId=null パス）
    expect(
      screen.getByText("フィードを選択してください")
    ).toBeInTheDocument();

    // 「お気に入り」項目をクリック → 右ペインが StarredItemList に切替
    await user.click(screen.getByTestId("starred-nav-item"));

    await waitFor(() => {
      expect(screen.getByTestId("starred-item-list")).toBeInTheDocument();
    });
    // コンテキストタイトル「お気に入り」が右ペインに表示される（Req 2.1）
    expect(screen.getByTestId("starred-item-list-title")).toHaveTextContent(
      "お気に入り"
    );
    // 「フィードを選択してください」の案内（ItemList feedId=null）は消える
    expect(
      screen.queryByText("フィードを選択してください")
    ).not.toBeInTheDocument();

    // フィード行（Tech Blog）をクリック → 右ペインが ItemList に戻る（Req 1.4 / 5.3）
    await user.click(screen.getByText("Tech Blog"));

    await waitFor(() => {
      // フィルタタブ（全て）が表示される = ItemList が描画されている
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });
    // StarredItemList は描画されていない
    expect(screen.queryByTestId("starred-item-list")).not.toBeInTheDocument();
  });

  it("ヘッダー領域に横断検索バー（HeaderSearchBar）が常設されること（Req 1.1）", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    // ヘッダー直下に検索バー（role="search"）が常設されている
    await waitFor(() => {
      expect(screen.getByTestId("header-search-bar")).toBeInTheDocument();
    });
    expect(screen.getByTestId("header-search-input")).toBeInTheDocument();
  });

  it("検索バーで Enter を押下すると右ペインの ItemList が SearchResults に切り替わること（Req 4.7）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    // 初期状態: ItemList の「フィードを選択してください」が表示される（フィード未選択時）
    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });

    // ヘッダー検索バーで検索を実行
    const input = screen.getByTestId("header-search-input");
    await user.type(input, "typescript");
    await user.keyboard("{Enter}");

    // 右ペインが SearchResults に切り替わる（検索結果は空 → 空状態表示）
    await waitFor(() => {
      expect(
        screen.getByTestId("search-results-empty")
      ).toBeInTheDocument();
    });

    // ItemList の「フィードを選択してください」案内は表示されなくなる
    expect(
      screen.queryByText("フィードを選択してください")
    ).not.toBeInTheDocument();
  });

  it("CLEAR_SEARCH（クリアボタン押下）で右ペインが ItemList に戻ること（Req 1.6 / NFR 2.2）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("header-search-input")).toBeInTheDocument();
    });

    // 検索を実行 → SearchResults に切り替わる
    const input = screen.getByTestId("header-search-input");
    await user.type(input, "kubernetes");
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(
        screen.getByTestId("search-results-empty")
      ).toBeInTheDocument();
    });

    // クリアボタンを押下 → ItemList に戻る
    await user.click(screen.getByTestId("header-search-clear"));

    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });

    // SearchResults は表示されなくなる
    expect(screen.queryByTestId("search-results-empty")).not.toBeInTheDocument();
    expect(screen.queryByTestId("search-results-loading")).not.toBeInTheDocument();
    expect(screen.queryByTestId("search-results")).not.toBeInTheDocument();
  });

  it("検索モード中はフィード選択メッセージが消え、検索結果コンテナが描画されること（Req 4.7）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    });

    // フィードを選択 → ItemList が表示される
    await user.click(screen.getByText("Tech Blog"));
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });

    // ヘッダー検索バーで横断検索を実行 → 右ペインは SearchResults に切り替わる
    const input = screen.getByTestId("header-search-input");
    await user.type(input, "anything");
    await user.keyboard("{Enter}");

    // ItemList のフィルタタブは消え、SearchResults の空状態が表示される
    await waitFor(() => {
      expect(screen.getByTestId("search-results-empty")).toBeInTheDocument();
    });
    expect(screen.queryByRole("tab", { name: "全て" })).not.toBeInTheDocument();
  });

  // -----------------------------------------------------------------------
  // 購読解除フロー統合テスト（task 6 / Issue #130）
  //
  // 既存 mockFetch（setupMockFetch）は GET /api/subscriptions を mockSubscriptions
  // 固定で返すため、購読解除後の一覧変化（[feeds] invalidate → refetch）を
  // 検証するシナリオでは、解除済み subscription を逐次除外する動的 mockFetch を
  // 各テスト内で setup する。
  // -----------------------------------------------------------------------
  describe("購読解除フロー（task 6）", () => {
    /**
     * DELETE /api/subscriptions/:id を成功させ、以降の GET /api/subscriptions が
     * 解除済み id を除いた一覧を返すように mockFetch を動的に切替える共通ヘルパ。
     *
     * @param deleteOk  DELETE のレスポンス ok（false 時は 500 で失敗扱い）
     */
    function setupUnsubscribeMockFetch(deleteOk: boolean) {
      const deletedIds = new Set<string>();
      mockFetch.mockImplementation((url: string, options?: RequestInit) => {
        // DELETE /api/subscriptions/:id の処理
        if (
          typeof url === "string" &&
          url.startsWith("/api/subscriptions/") &&
          options?.method === "DELETE"
        ) {
          const id = url.substring("/api/subscriptions/".length);
          if (deleteOk) {
            deletedIds.add(id);
            return Promise.resolve({
              ok: true,
              status: 200,
              json: async () => ({}),
            });
          }
          // 失敗系: 500 を返す
          return Promise.resolve({
            ok: false,
            status: 500,
            json: async () => ({ error: "internal server error" }),
          });
        }
        // GET /api/subscriptions（一覧）。DELETE 後の refetch では除外されたものを返す
        if (url === "/api/subscriptions") {
          const filtered = mockSubscriptions.filter(
            (s) => !deletedIds.has(s.id)
          );
          return Promise.resolve({
            ok: true,
            json: async () => filtered,
          });
        }
        // 記事一覧 API
        if (typeof url === "string" && url.includes("/api/feeds/")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              items: [],
              next_cursor: null,
              has_more: false,
            }),
          });
        }
        if (typeof url === "string" && url.includes("/api/items/search")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              items: [],
              next_cursor: null,
              has_more: false,
            }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({}),
        });
      });
    }

    it("(a) フィード行ホバー → ギアアイコンクリックで「フィードの設定」ダイアログが表示されること（AC 1.3, 2.1）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });

      // フィード行にホバー（jsdom では CSS hover 擬似クラスが評価されないため、
      // ホバーイベント自体は発火させた上で、ギアボタン自体は DOM に存在することを
      // 検証する。表示制御は task 2 で class 文字列レベルで担保済み）
      const feedRow = screen.getByTestId("feed-item-sub-1");
      await user.hover(feedRow);

      // ギアボタンが存在する（hover 関係なく DOM 上には常に存在し、CSS で表示制御）
      const gearButton = screen.getByTestId("feed-settings-button-sub-1");
      expect(gearButton).toBeInTheDocument();

      // ギアアイコンクリック → ダイアログタイトル「フィードの設定」が表示される
      await user.click(gearButton);

      await waitFor(() => {
        expect(screen.getByText("フィードの設定")).toBeInTheDocument();
      });

      // ダイアログ内の「購読解除」ボタンが render される（AC 2.1: 対象フィードの状態反映）
      expect(screen.getByTestId("unsubscribe-button")).toBeInTheDocument();
      expect(screen.getByTestId("fetch-interval-select")).toBeInTheDocument();
    });

    it("(b) ダイアログ表示中に Esc キーで閉じられること（AC 2.5）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });

      // ギアボタンをクリックしてダイアログを開く
      await user.click(screen.getByTestId("feed-settings-button-sub-1"));

      await waitFor(() => {
        expect(screen.getByText("フィードの設定")).toBeInTheDocument();
      });

      // Esc キーで閉じる（radix-ui Dialog の既定挙動）
      await user.keyboard("{Escape}");

      await waitFor(() => {
        expect(screen.queryByText("フィードの設定")).not.toBeInTheDocument();
      });

      // ダイアログ内コンテンツも DOM から除去されている
      expect(screen.queryByTestId("unsubscribe-button")).not.toBeInTheDocument();
    });

    it("(c) 選択中フィードを購読解除 → ダイアログ閉鎖・一覧から除外・右ペインが初期表示に戻ること（AC 4.1, 4.2, 4.4, 4.5）", async () => {
      const user = userEvent.setup();
      setupUnsubscribeMockFetch(true);

      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });

      // 事前に Tech Blog を選択（右ペインに ItemList が描画される）
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // ギアボタンクリック → ダイアログ表示
      await user.click(screen.getByTestId("feed-settings-button-sub-1"));

      await waitFor(() => {
        expect(screen.getByText("フィードの設定")).toBeInTheDocument();
      });

      // 「購読解除」ボタンクリック → 確認 AlertDialog 表示
      await user.click(screen.getByTestId("unsubscribe-button"));

      await waitFor(() => {
        expect(screen.getByText("購読を解除しますか？")).toBeInTheDocument();
      });

      // 確認ダイアログの「購読解除」ボタンを押下（AlertDialog の確定）
      await user.click(screen.getByRole("button", { name: "購読解除" }));

      // DELETE が呼ばれたことを確認
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith(
          "/api/subscriptions/sub-1",
          expect.objectContaining({ method: "DELETE" })
        );
      });

      // ダイアログが閉じる（AC 4.4）
      await waitFor(() => {
        expect(screen.queryByText("フィードの設定")).not.toBeInTheDocument();
      });

      // フィード一覧から Tech Blog が消える（AC 4.1: [feeds] invalidate → refetch）
      await waitFor(() => {
        expect(screen.queryByText("Tech Blog")).not.toBeInTheDocument();
      });
      // News Feed は引き続き表示される
      expect(screen.getByText("News Feed")).toBeInTheDocument();

      // 右ペインが初期状態（ItemList feedId=null の案内）に戻る（AC 4.2）
      await waitFor(() => {
        expect(
          screen.getByText("フィードを選択してください")
        ).toBeInTheDocument();
      });
      // ItemList のフィルタタブは消えている（selectedFeedId=null のため）
      expect(screen.queryByRole("tab", { name: "全て" })).not.toBeInTheDocument();
    });

    it("(d) 非選択フィードを購読解除 → 右ペインの選択状態は維持され、対象のみ一覧から消えること（AC 4.1, 4.3）", async () => {
      const user = userEvent.setup();
      setupUnsubscribeMockFetch(true);

      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
        expect(screen.getByText("News Feed")).toBeInTheDocument();
      });

      // フィード A（Tech Blog / sub-1 / feed-1）を選択
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // 別フィード B（News Feed / sub-2 / feed-2）のギアをクリック
      await user.click(screen.getByTestId("feed-settings-button-sub-2"));

      await waitFor(() => {
        expect(screen.getByText("フィードの設定")).toBeInTheDocument();
      });

      // 確認ダイアログを開いて確定
      await user.click(screen.getByTestId("unsubscribe-button"));

      await waitFor(() => {
        expect(screen.getByText("購読を解除しますか？")).toBeInTheDocument();
      });

      await user.click(screen.getByRole("button", { name: "購読解除" }));

      // DELETE が News Feed（sub-2）に対して発行されたことを確認
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith(
          "/api/subscriptions/sub-2",
          expect.objectContaining({ method: "DELETE" })
        );
      });

      // ダイアログ閉鎖
      await waitFor(() => {
        expect(screen.queryByText("フィードの設定")).not.toBeInTheDocument();
      });

      // News Feed が一覧から消える（AC 4.1）
      await waitFor(() => {
        expect(screen.queryByText("News Feed")).not.toBeInTheDocument();
      });
      // Tech Blog は引き続き表示される
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();

      // 右ペインは Tech Blog の ItemList のまま（AC 4.3: 選択状態維持）
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      // 「フィードを選択してください」案内は表示されていない（クリアされていないことの証左）
      expect(
        screen.queryByText("フィードを選択してください")
      ).not.toBeInTheDocument();
    });

    it("(e) DELETE 失敗時（500）→ ダイアログ残存・一覧不変・右ペイン不変（AC 5.1, 5.2, 5.3）", async () => {
      const user = userEvent.setup();
      setupUnsubscribeMockFetch(false); // DELETE は 500 で失敗

      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
        expect(screen.getByText("News Feed")).toBeInTheDocument();
      });

      // フィード A（Tech Blog）を選択し、右ペインに ItemList を描画
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // フィード B（News Feed）のギア → 設定ダイアログ
      await user.click(screen.getByTestId("feed-settings-button-sub-2"));

      await waitFor(() => {
        expect(screen.getByText("フィードの設定")).toBeInTheDocument();
      });

      // 確認 AlertDialog を開いて確定
      await user.click(screen.getByTestId("unsubscribe-button"));

      await waitFor(() => {
        expect(screen.getByText("購読を解除しますか？")).toBeInTheDocument();
      });

      await user.click(screen.getByRole("button", { name: "購読解除" }));

      // DELETE は発行される（が 500 で失敗）
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith(
          "/api/subscriptions/sub-2",
          expect.objectContaining({ method: "DELETE" })
        );
      });

      // mutation の Promise が settle するまで待つ余地（onSuccess は呼ばれない）
      await waitFor(() => {
        // 失敗時は AppShell 側の onUnsubscribed が呼ばれないため、設定ダイアログは
        // 開いたまま残存する（AC 5.2）。SubscriptionSettings 内部の confirm AlertDialog
        // も同様に閉じない（onSuccess 内で setShowUnsubscribeDialog(false) なため）
        expect(screen.getByText("フィードの設定")).toBeInTheDocument();
      });

      // 一覧の News Feed は引き続き存在（AC 5.1: [feeds] invalidate されない / refetch されない）
      // 設定 Dialog が開いている間 radix-ui がメインコンテンツに aria-hidden を付与するため
      // accessibility tree 経由の getByRole は使えないが、getByText は textContent ベースで
      // DOM ノードを探すため引き続き機能する
      expect(screen.getByText("News Feed")).toBeInTheDocument();
      expect(screen.getByText("Tech Blog")).toBeInTheDocument();

      // 右ペインは A（Tech Blog / feed-1）のまま不変（AC 5.3）。
      // ダイアログ open 中は ItemList のタブが accessibility tree から隠れる（radix-ui の
      // モーダル既定挙動）ため、selectedFeedId の維持は左ペインのフィード行の
      // data-selected="true" 属性で確認する（DOM 属性なので aria-hidden に影響されない）
      expect(screen.getByTestId("feed-item-sub-1")).toHaveAttribute(
        "data-selected",
        "true"
      );
      // 右ペインが ItemList feedId=null パスに切り替わっていないこと（AC 5.3 補強）。
      // 「フィードを選択してください」案内は ItemList の feedId=null 描画時のみ表示される
      expect(
        screen.queryByText("フィードを選択してください")
      ).not.toBeInTheDocument();
    });
  });

  // --- Issue #121 task 9: viewMode 切替配線テスト ---

  it("「すべての新着記事」エントリ click で viewMode='cross-feed' に切替わり、右ペインに CrossFeedItemList が描画されること（Req 1.2）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("all-new-items-entry")).toBeInTheDocument();
    });

    // 初期状態: 「フィードを選択してください」(viewMode='none' + ItemList feedId=null)
    expect(
      screen.getByText("フィードを選択してください")
    ).toBeInTheDocument();

    // 「すべての新着記事」エントリ click → CrossFeedItemList に切替
    await user.click(screen.getByTestId("all-new-items-entry"));

    await waitFor(() => {
      expect(screen.getByTestId("cross-feed-item-list")).toBeInTheDocument();
    });
    // タイトル「すべての新着記事」が右ペインに表示される
    expect(
      screen.getByTestId("cross-feed-item-list-title")
    ).toHaveTextContent("すべての新着記事");
    // 「フィードを選択してください」案内は消える
    expect(
      screen.queryByText("フィードを選択してください")
    ).not.toBeInTheDocument();
  });

  it("viewMode='none' のとき既存「フィードを選択してください」表示が維持されること（Req 5.1 非リグレッション）", async () => {
    render(<AppShell />, { wrapper: createWrapper() });

    // 初期状態は viewMode='none' / selectedView='feed' / selectedFeedId=null
    await waitFor(() => {
      expect(
        screen.getByText("フィードを選択してください")
      ).toBeInTheDocument();
    });

    // CrossFeedItemList は描画されていない
    expect(
      screen.queryByTestId("cross-feed-item-list")
    ).not.toBeInTheDocument();
  });

  // -----------------------------------------------------------------------
  // フィード内検索の連続操作統合テスト（Task 5 / Issue #145）
  //
  // フィード内検索の結果表示中もフィード内検索バーが画面に残り、再編集 → 再検索の
  // 連続操作が単一画面で完結することを 9 ケースで検証する。fetch モックは既存
  // `setupMockFetch` を踏襲し、`/api/feeds/:id/items` / `/api/items/search` は空応答を
  // 返す前提（描画されるリスト本体は空状態 / 空 SearchResults）。
  //
  // テスト戦略:
  // - `feed-search-bar` testid の有無で「フィード内検索バーが画面上にあるか」を判定
  //   （Req 1.1 / 2.1 / 2.3）
  // - `feed-search-input` の DOM value で「現在の検索キーワード反映」を判定（Req 1.2）
  // - mockFetch.mock.calls の URL 引数の `q=<キーワード>` 有無で「新しい検索が発火されたか」
  //   を判定（Req 1.3, 1.4）
  // - クリアボタン押下後に `search-results-empty` が消えて「全て」タブが復活することで
  //   「通常一覧に戻った」を判定（Req 1.6, NFR 1.1）
  // -----------------------------------------------------------------------
  describe("フィード内検索の連続操作（Task 5 / Issue #145）", () => {
    /**
     * 直近の `/api/items/search` リクエスト URL から `q=` パラメータを抽出する。
     * fetch モックの呼び出し履歴から検索キーワード遷移を観測するヘルパ。
     */
    function lastSearchQuery(): string | null {
      const calls = mockFetch.mock.calls
        .map(([url]) => url as string)
        .filter(
          (url) =>
            typeof url === "string" && url.startsWith("/api/items/search")
        );
      if (calls.length === 0) return null;
      const last = calls[calls.length - 1];
      const queryString = last.substring(last.indexOf("?") + 1);
      const params = new URLSearchParams(queryString);
      return params.get("q");
    }

    it("(1) フィード選択 → キーワード入力 → Enter で SearchResults と feed-search-bar が同時に表示されること（Req 1.1）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      // フィード Tech Blog を選択
      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });
      await user.click(screen.getByText("Tech Blog"));

      // 通常一覧（FeedPaneHeader normal モード）が描画される
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // フィード内検索バーで「typescript」を入力 → Enter で検索実行
      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "typescript");
      await user.keyboard("{Enter}");

      // SearchResults が表示される（空結果なので空状態 testid）
      await waitFor(() => {
        expect(
          screen.getByTestId("search-results-empty")
        ).toBeInTheDocument();
      });

      // 同時に feed-search-bar testid も画面上に残っている（Req 1.1）
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
    });

    it("(2) 検索結果表示中に feed-search-input の value が現在の検索キーワードを反映していること（Req 1.2）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "kubernetes");
      await user.keyboard("{Enter}");

      await waitFor(() => {
        expect(
          screen.getByTestId("search-results-empty")
        ).toBeInTheDocument();
      });

      // 検索結果表示中に再度 input を取得し、value が "kubernetes" を反映している
      const inputAfter = screen.getByTestId(
        "feed-search-input"
      ) as HTMLInputElement;
      expect(inputAfter.value).toBe("kubernetes");
    });

    it("(3) 検索結果表示中に input を編集 → Enter で useItemSearch の queryKey が更新され新キーワードで fetch される（Req 1.3, 1.4）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // 初回検索: "typescript"
      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "typescript");
      await user.keyboard("{Enter}");

      await waitFor(() => {
        expect(lastSearchQuery()).toBe("typescript");
      });

      // 検索結果表示中に input を編集（既存値を一旦クリアしてから上書き）
      const inputDuringSearch = screen.getByTestId(
        "feed-search-input"
      ) as HTMLInputElement;
      await user.clear(inputDuringSearch);
      await user.type(inputDuringSearch, "kubernetes");

      // onChange が追随していること（Req 1.3）
      expect(inputDuringSearch.value).toBe("kubernetes");

      // Enter で新しい検索を実行
      await user.keyboard("{Enter}");

      // mockFetch の最新 search 呼び出しが新キーワード "kubernetes" を含む（Req 1.4）
      await waitFor(() => {
        expect(lastSearchQuery()).toBe("kubernetes");
      });
    });

    it("(4) 検索結果表示中に input を空にして Enter → 検索結果表示が維持されること（Req 1.5）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // 検索を実行 → SearchResults 表示
      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "typescript");
      await user.keyboard("{Enter}");

      await waitFor(() => {
        expect(
          screen.getByTestId("search-results-empty")
        ).toBeInTheDocument();
      });

      // 入力欄を空にして Enter（空入力 submit は dispatch されない / Req 1.5）
      const inputDuringSearch = screen.getByTestId(
        "feed-search-input"
      ) as HTMLInputElement;
      await user.clear(inputDuringSearch);
      expect(inputDuringSearch.value).toBe("");

      await user.keyboard("{Enter}");

      // 検索結果表示は維持される（SearchResults 空状態が依然として表示されている）
      // 通常一覧のフィルタタブは再表示されない（モードが切り替わっていないことの裏付け）
      expect(screen.getByTestId("search-results-empty")).toBeInTheDocument();
      expect(screen.queryByRole("tab", { name: "全て" })).not.toBeInTheDocument();
      // feed-search-bar 自体は引き続き存在
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
    });

    it("(5) 検索結果表示中にクリアボタン押下 → 通常一覧（ItemList）に戻り選択フィードと filter が保持されること（Req 1.6, NFR 1.1）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // 通常一覧で filter を "未読" に切替（filter 保持の検証準備）
      await user.click(screen.getByRole("tab", { name: "未読" }));
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "未読" })).toHaveAttribute(
          "data-state",
          "active"
        );
      });

      // フィード内検索を実行
      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "typescript");
      await user.keyboard("{Enter}");

      await waitFor(() => {
        expect(
          screen.getByTestId("search-results-empty")
        ).toBeInTheDocument();
      });

      // クリアボタン押下（× アイコン）→ 通常一覧へ戻る（Req 1.6）
      await user.click(screen.getByTestId("feed-search-clear"));

      await waitFor(() => {
        // フィルタタブが再表示される = ItemList（通常一覧）に戻った
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // SearchResults は撤去された
      expect(
        screen.queryByTestId("search-results-empty")
      ).not.toBeInTheDocument();

      // 選択フィードが保持されている（feed-1 = Tech Blog / sub-1）
      expect(screen.getByTestId("feed-item-sub-1")).toHaveAttribute(
        "data-selected",
        "true"
      );

      // filter "未読" が保持されている（NFR 1.1 の通常利用挙動の非回帰）
      expect(screen.getByRole("tab", { name: "未読" })).toHaveAttribute(
        "data-state",
        "active"
      );
    });

    it("(6) 横断検索結果表示中（HeaderSearchBar から submit）に feed-search-bar testid が画面上に存在しないこと（Req 2.1）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      // フィード未選択のままヘッダー横断検索バーから submit
      await waitFor(() => {
        expect(screen.getByTestId("header-search-input")).toBeInTheDocument();
      });

      const headerInput = screen.getByTestId("header-search-input");
      await user.type(headerInput, "typescript");
      await user.keyboard("{Enter}");

      await waitFor(() => {
        expect(
          screen.getByTestId("search-results-empty")
        ).toBeInTheDocument();
      });

      // フィード内検索バーは表示されない（横断検索結果中なので Req 2.1）
      expect(
        screen.queryByTestId("feed-search-bar")
      ).not.toBeInTheDocument();
    });

    it("(7) フィード内検索結果表示中に左ペインで別フィードを選択 → 検索解除 → 選択先フィードの通常一覧が表示されること（Req 2.2）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
        expect(screen.getByText("News Feed")).toBeInTheDocument();
      });

      // フィード A (Tech Blog) を選択し、フィード内検索を実行
      await user.click(screen.getByText("Tech Blog"));
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "typescript");
      await user.keyboard("{Enter}");

      await waitFor(() => {
        expect(
          screen.getByTestId("search-results-empty")
        ).toBeInTheDocument();
      });
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();

      // 左ペインで別フィード (News Feed / feed-2) を選択 → SELECT_FEED で検索状態は解除される
      await user.click(screen.getByText("News Feed"));

      // 選択先フィードの通常一覧（フィルタタブ + feed-search-bar）が再表示される
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();

      // 検索結果表示は解除される
      expect(
        screen.queryByTestId("search-results-empty")
      ).not.toBeInTheDocument();

      // 選択先フィード（sub-2）が選択中になっている
      expect(screen.getByTestId("feed-item-sub-2")).toHaveAttribute(
        "data-selected",
        "true"
      );
    });

    it("(8) 初期状態（フィード未選択）で feed-search-bar testid が存在しないこと（Req 2.3）", async () => {
      render(<AppShell />, { wrapper: createWrapper() });

      // 初期状態の右ペインが描画されるのを待つ
      await waitFor(() => {
        expect(
          screen.getByText("フィードを選択してください")
        ).toBeInTheDocument();
      });

      // フィード未選択時はフィード内検索バーが画面に存在しない（Req 2.3）
      expect(
        screen.queryByTestId("feed-search-bar")
      ).not.toBeInTheDocument();

      // ヘッダー横断検索バーは別物なので引き続き存在する（既存 Req 3.1 の非回帰確認）
      expect(screen.getByTestId("header-search-bar")).toBeInTheDocument();
    });

    it("(9) フィード内検索結果表示中の SearchResults 空状態 DOM 構造が本変更導入前と同一であること（NFR 1.2）", async () => {
      const user = userEvent.setup();
      render(<AppShell />, { wrapper: createWrapper() });

      await waitFor(() => {
        expect(screen.getByText("Tech Blog")).toBeInTheDocument();
      });
      await user.click(screen.getByText("Tech Blog"));

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
      });

      // フィード内検索を実行
      const feedSearchInput = screen.getByTestId("feed-search-input");
      await user.type(feedSearchInput, "typescript");
      await user.keyboard("{Enter}");

      // SearchResults 空状態の DOM 構造を確認（NFR 1.2: 検索結果画面の本変更導入前との同一性）
      const empty = await screen.findByTestId("search-results-empty");

      // 1. 空状態のメッセージ文言が変更されていない（search-results.tsx L135 と一致）
      expect(empty).toHaveTextContent("検索結果はありません");

      // 2. 空状態は SearchResults の代替表示として div ノードで描画される
      //    （カード一覧の data-testid="search-results" は出現しない）
      expect(empty.tagName).toBe("DIV");
      expect(screen.queryByTestId("search-results")).not.toBeInTheDocument();

      // 3. ローディング / エラー表示の testid は同時に出現しない（出し分けが排他）
      expect(
        screen.queryByTestId("search-results-loading")
      ).not.toBeInTheDocument();
      expect(
        screen.queryByTestId("search-results-error")
      ).not.toBeInTheDocument();

      // 4. SearchResults の親 DOM 構造（FeedPaneHeader + SearchResults が右ペイン内に同居）は
      //    feed-search-bar testid と search-results-empty testid が同時に存在することで担保
      expect(screen.getByTestId("feed-search-bar")).toBeInTheDocument();
    });
  });

  it("横断 → 個別 → 横断 の遷移後に crossFeedSessionSince が保持され同一 baseline で fetch されること（Req 4.7）", async () => {
    const user = userEvent.setup();
    render(<AppShell />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("all-new-items-entry")).toBeInTheDocument();
    });

    // 1. 横断一覧を開く → 初回 fetch で since_time が baseline として固定される
    await user.click(screen.getByTestId("all-new-items-entry"));

    await waitFor(() => {
      // 初回 fetch（cross-feed GET）が呼ばれている
      const crossFeedCalls = mockFetch.mock.calls.filter(
        ([url]) => typeof url === "string" && url.startsWith("/api/items/cross-feed")
      );
      expect(crossFeedCalls.length).toBeGreaterThanOrEqual(1);
    });

    // 初回 PUT （Req 4.3）が完了するのを待つ
    await waitFor(() => {
      const putCalls = mockFetch.mock.calls.filter(
        ([url, options]) =>
          url === "/api/users/me/cross-feed-last-seen" &&
          (options as RequestInit | undefined)?.method === "PUT"
      );
      expect(putCalls).toHaveLength(1);
    });

    // 2. 個別フィードへ遷移
    await user.click(screen.getByText("Tech Blog"));
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "全て" })).toBeInTheDocument();
    });

    // 3. 横断一覧に戻る
    await user.click(screen.getByTestId("all-new-items-entry"));
    await waitFor(() => {
      expect(screen.getByTestId("cross-feed-item-list")).toBeInTheDocument();
    });

    // baseline が保持されているため、戻り時の cross-feed fetch には
    // 固定された since_time が since= として渡されている
    await waitFor(() => {
      const sinceCalls = mockFetch.mock.calls
        .map(([url]) => url as string)
        .filter(
          (url) =>
            typeof url === "string" &&
            url.startsWith("/api/items/cross-feed") &&
            url.includes(
              `since=${encodeURIComponent(CROSS_FEED_SINCE_TIME)}`
            )
        );
      expect(sinceCalls.length).toBeGreaterThanOrEqual(1);
    });

    // 再度 PUT が呼ばれていない（同一セッション内では touch は 1 回のみ、Req 4.3）
    const putCallsAfter = mockFetch.mock.calls.filter(
      ([url, options]) =>
        url === "/api/users/me/cross-feed-last-seen" &&
        (options as RequestInit | undefined)?.method === "PUT"
    );
    expect(putCallsAfter).toHaveLength(1);
  });
});
