import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ItemDetail } from "./item-detail";
import type { ItemDetail as ItemDetailType } from "@/types/item";
import type { ReactNode } from "react";

// グローバルfetchのモック
const mockFetch = vi.fn();
global.fetch = mockFetch;

/** テスト用ラッパー */
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
        {children}
      </QueryClientProvider>
    );
  };
}

/** テスト用の記事詳細データ */
const mockItem: ItemDetailType = {
  id: "item-1",
  feed_id: "feed-1",
  title: "テスト記事のタイトル",
  link: "https://example.com/article-1",
  published_at: "2026-02-27T10:00:00Z",
  is_date_estimated: false,
  is_read: true,
  is_starred: false,
  hatebu_count: 42,
  hatebu_fetched_at: "2026-02-27T09:00:00Z",
  content: "<p>これはテスト記事の<strong>本文</strong>です。</p>",
  summary: "テスト記事のサマリー",
  author: "テスト著者",
};

/** はてブ未取得の記事データ */
const mockItemNoHatebu: ItemDetailType = {
  ...mockItem,
  id: "item-2",
  hatebu_count: 0,
  hatebu_fetched_at: null,
};

/**
 * mockFetchの設定ヘルパー
 */
function setupMockFetch() {
  mockFetch.mockImplementation((url: string) => {
    if (typeof url === "string" && url.includes("/api/items/") && url.includes("/state")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          user_id: "user-1",
          item_id: "item-1",
          is_read: true,
          is_starred: true,
        }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({}),
    });
  });
}

describe("ItemDetail コンポーネント", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMockFetch();
  });

  it("サニタイズ済みHTMLコンテンツが展開表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // コンテンツ表示エリアが存在すること
    const contentArea = screen.getByTestId("item-content");
    expect(contentArea).toBeInTheDocument();
    // HTMLコンテンツが表示されていること（dangerouslySetInnerHTMLなのでテキストで確認）
    expect(contentArea.innerHTML).toContain("これはテスト記事の");
    expect(contentArea.innerHTML).toContain("<strong>本文</strong>");
  });

  it("元記事URLへの遷移ボタンが表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const linkButton = screen.getByTestId("original-link");
    expect(linkButton).toBeInTheDocument();
    expect(linkButton).toHaveAttribute("href", "https://example.com/article-1");
    expect(linkButton).toHaveAttribute("target", "_blank");
    expect(linkButton).toHaveAttribute("rel", "noopener noreferrer");
    // Req 4.3: 「元記事を開く」テキストとアイコンを伴うリンクであること
    expect(linkButton).toHaveTextContent("元記事を開く");
  });

  it("はてなブックマーク数が表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const hatebuCount = screen.getByTestId("hatebu-count");
    expect(hatebuCount).toBeInTheDocument();
    expect(hatebuCount).toHaveTextContent("42");
  });

  it("はてブ未取得時は「-」が表示されること", () => {
    render(
      <ItemDetail
        item={mockItemNoHatebu}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const hatebuCount = screen.getByTestId("hatebu-count");
    expect(hatebuCount).toHaveTextContent("-");
  });

  // Req 3.3: 取得済みかつ 0 件のとき「0」を表示する（境界値）
  it("はてブ取得済みかつ0件のとき「0」が表示されること", () => {
    const zeroItem: ItemDetailType = {
      ...mockItem,
      hatebu_count: 0,
      hatebu_fetched_at: "2026-02-27T09:00:00Z",
    };

    render(
      <ItemDetail
        item={zeroItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const hatebuCount = screen.getByTestId("hatebu-count");
    expect(hatebuCount).toHaveTextContent("0");
  });

  it("スター切替ボタンをクリックするとonToggleStarが呼ばれること", async () => {
    const user = userEvent.setup();
    const onToggleStar = vi.fn();

    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={onToggleStar}
      />,
      { wrapper: createWrapper() }
    );

    const starButton = screen.getByTestId("star-toggle");
    await user.click(starButton);

    // 現在is_starred=falseなので、trueに切り替えるリクエストが呼ばれること
    expect(onToggleStar).toHaveBeenCalledWith("item-1", true);
  });

  it("スター付き記事のスターボタンをクリックするとfalseで呼ばれること", async () => {
    const user = userEvent.setup();
    const onToggleStar = vi.fn();
    const starredItem = { ...mockItem, is_starred: true };

    render(
      <ItemDetail
        item={starredItem}
        onMarkAsRead={() => {}}
        onToggleStar={onToggleStar}
      />,
      { wrapper: createWrapper() }
    );

    const starButton = screen.getByTestId("star-toggle");
    await user.click(starButton);

    // 現在is_starred=trueなので、falseに切り替えるリクエストが呼ばれること
    expect(onToggleStar).toHaveBeenCalledWith("item-1", false);
  });

  it("展開時にonMarkAsReadが呼ばれること", () => {
    const onMarkAsRead = vi.fn();

    render(
      <ItemDetail
        item={{ ...mockItem, is_read: false }}
        onMarkAsRead={onMarkAsRead}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // コンポーネントのマウント時（展開時）にonMarkAsReadが呼ばれること
    expect(onMarkAsRead).toHaveBeenCalledWith("item-1");
  });

  it("既読記事ではonMarkAsReadが呼ばれないこと", () => {
    const onMarkAsRead = vi.fn();

    render(
      <ItemDetail
        item={mockItem} // is_read: true
        onMarkAsRead={onMarkAsRead}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    // 既に既読なのでonMarkAsReadは呼ばれない
    expect(onMarkAsRead).not.toHaveBeenCalled();
  });

  it("記事タイトルが表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("テスト記事のタイトル")).toBeInTheDocument();
  });

  it("著者名が表示されること", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("テスト著者")).toBeInTheDocument();
  });

  // Req 1.1 / 2.1: 危険な要素を含む記事本文はサニタイズ後の結果が描画されること
  it("記事本文にscript要素が含まれるときサニタイズされて描画されること", () => {
    const dangerousItem: ItemDetailType = {
      ...mockItem,
      content:
        "<p>安全な本文</p><script>window.__xss = true;</script>",
    };

    render(
      <ItemDetail
        item={dangerousItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const contentArea = screen.getByTestId("item-content");
    // 許可タグは保持される
    expect(contentArea.innerHTML).toContain("安全な本文");
    // script 要素は除去される
    expect(contentArea.innerHTML).not.toContain("<script");
    expect(contentArea.innerHTML).not.toContain("window.__xss");
  });

  // Req 1.1 / 2.3: on* インラインイベントハンドラ属性が除去されること
  it("記事本文にonerror属性が含まれるときサニタイズされて描画されること", () => {
    const dangerousItem: ItemDetailType = {
      ...mockItem,
      content:
        '<img src="https://example.com/a.png" alt="画像" onerror="window.__xss = true;">',
    };

    render(
      <ItemDetail
        item={dangerousItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const contentArea = screen.getByTestId("item-content");
    expect(contentArea.innerHTML).not.toContain("onerror");
    // 許可された画像属性は保持される
    expect(contentArea.innerHTML).toContain("https://example.com/a.png");
  });

  // Req 1.3: 空文字列の記事本文は空のコンテンツ領域を表示すること
  it("記事本文が空文字列のとき空のコンテンツ領域を表示すること", () => {
    const emptyItem: ItemDetailType = { ...mockItem, content: "" };

    render(
      <ItemDetail
        item={emptyItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const contentArea = screen.getByTestId("item-content");
    expect(contentArea).toBeInTheDocument();
    expect(contentArea.innerHTML).toBe("");
  });

  it("タイトルが元記事への外部リンクであり、新規タブで開くこと", () => {
    render(
      <ItemDetail
        item={mockItem}
        onMarkAsRead={() => {}}
        onToggleStar={() => {}}
      />,
      { wrapper: createWrapper() }
    );

    const titleLink = screen.getByRole("link", { name: "テスト記事のタイトル" });
    expect(titleLink).toHaveAttribute("href", "https://example.com/article-1");
    expect(titleLink).toHaveAttribute("target", "_blank");
    expect(titleLink).toHaveAttribute("rel", "noopener noreferrer");
  });

  // 本文の高さクリップ / 「続きを読む」トグル（Req 1〜4）
  describe("本文の高さ制限と続きを読むトグル", () => {
    /**
     * コンテンツ表示エリアの scrollHeight をモックするヘルパー。
     * jsdom はレイアウトを計算しないため scrollHeight は常に 0 を返す。
     * data-testid="item-content" 要素に対して固定の scrollHeight を返すよう
     * HTMLElement.prototype.scrollHeight を差し替える。
     */
    function mockContentScrollHeight(value: number): () => void {
      const original = Object.getOwnPropertyDescriptor(
        HTMLElement.prototype,
        "scrollHeight"
      );
      Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
        configurable: true,
        get(this: HTMLElement) {
          if (this.getAttribute("data-testid") === "item-content") {
            return value;
          }
          return 0;
        },
      });
      return () => {
        if (original) {
          Object.defineProperty(
            HTMLElement.prototype,
            "scrollHeight",
            original
          );
        } else {
          // 元々プロパティが存在しなかった場合は削除する
          delete (HTMLElement.prototype as unknown as Record<string, unknown>)
            .scrollHeight;
        }
      };
    }

    // Req 1.1 / 1.2 / 1.3 / 2.1: 本文が 300px を超えるとき折りたたみ・フェードアウト・「続きを読む」を表示する
    it("本文の高さが300pxを超えるとき折りたたまれ「続きを読む」ボタンとフェードアウトが表示されること", async () => {
      // Arrange
      const restore = mockContentScrollHeight(500);
      try {
        render(
          <ItemDetail
            item={mockItem}
            onMarkAsRead={() => {}}
            onToggleStar={() => {}}
          />,
          { wrapper: createWrapper() }
        );

        // Act / Assert
        const toggle = await screen.findByTestId("content-toggle");
        expect(toggle).toHaveTextContent("続きを読む");
        expect(screen.getByTestId("content-fade")).toBeInTheDocument();
        // 折りたたみ時はコンテナに高さ制限クラスが付与される
        expect(screen.getByTestId("item-content").className).toContain(
          "max-h-[300px]"
        );
      } finally {
        restore();
      }
    });

    // Req 2.2 / 2.3 / 2.4: 「続きを読む」押下で全文表示・フェードアウト除去・文言が「折りたたむ」に変わる
    it("「続きを読む」ボタンを押下すると全文表示に切り替わりフェードアウトが消え文言が「折りたたむ」になること", async () => {
      // Arrange
      const restore = mockContentScrollHeight(500);
      try {
        const user = userEvent.setup();
        render(
          <ItemDetail
            item={mockItem}
            onMarkAsRead={() => {}}
            onToggleStar={() => {}}
          />,
          { wrapper: createWrapper() }
        );
        const toggle = await screen.findByTestId("content-toggle");

        // Act
        await user.click(toggle);

        // Assert
        expect(screen.getByTestId("content-toggle")).toHaveTextContent(
          "折りたたむ"
        );
        expect(screen.queryByTestId("content-fade")).not.toBeInTheDocument();
        expect(screen.getByTestId("item-content").className).not.toContain(
          "max-h-[300px]"
        );
      } finally {
        restore();
      }
    });

    // Req 3.1 / 3.2 / 3.3 / 3.4: 「折りたたむ」押下で 300px 再制限・フェードアウト再表示・文言が「続きを読む」に戻る
    it("全文表示中に「折りたたむ」ボタンを押下すると300px再制限とフェードアウト再表示が行われ文言が「続きを読む」に戻ること", async () => {
      // Arrange
      const restore = mockContentScrollHeight(500);
      try {
        const user = userEvent.setup();
        render(
          <ItemDetail
            item={mockItem}
            onMarkAsRead={() => {}}
            onToggleStar={() => {}}
          />,
          { wrapper: createWrapper() }
        );
        const toggle = await screen.findByTestId("content-toggle");
        await user.click(toggle); // 全文表示へ

        // Act
        await user.click(screen.getByTestId("content-toggle")); // 折りたたみへ復帰

        // Assert
        expect(screen.getByTestId("content-toggle")).toHaveTextContent(
          "続きを読む"
        );
        expect(screen.getByTestId("content-fade")).toBeInTheDocument();
        expect(screen.getByTestId("item-content").className).toContain(
          "max-h-[300px]"
        );
      } finally {
        restore();
      }
    });

    // Req 4.1 / 4.2 / 4.3: 本文が 300px 未満なら折りたたまず全文表示・ボタンとフェードアウトを出さない
    it("本文の高さが300px未満のとき折りたたまず「続きを読む」ボタンもフェードアウトも表示しないこと", async () => {
      // Arrange
      const restore = mockContentScrollHeight(200);
      try {
        render(
          <ItemDetail
            item={mockItem}
            onMarkAsRead={() => {}}
            onToggleStar={() => {}}
          />,
          { wrapper: createWrapper() }
        );

        // Act
        await waitFor(() => {
          // 測定後にもボタンが現れないことを確認するため一旦描画完了を待つ
          expect(screen.getByTestId("item-content")).toBeInTheDocument();
        });

        // Assert
        expect(screen.queryByTestId("content-toggle")).not.toBeInTheDocument();
        expect(screen.queryByTestId("content-fade")).not.toBeInTheDocument();
        expect(screen.getByTestId("item-content").className).not.toContain(
          "max-h-[300px]"
        );
      } finally {
        restore();
      }
    });

    // Req 4.1 境界値: 本文の高さがちょうど 300px のとき折りたたまない（超過のみ折りたたむ）
    it("本文の高さがちょうど300pxのとき折りたたまず「続きを読む」ボタンを表示しないこと", async () => {
      // Arrange
      const restore = mockContentScrollHeight(300);
      try {
        render(
          <ItemDetail
            item={mockItem}
            onMarkAsRead={() => {}}
            onToggleStar={() => {}}
          />,
          { wrapper: createWrapper() }
        );

        // Act
        await waitFor(() => {
          expect(screen.getByTestId("item-content")).toBeInTheDocument();
        });

        // Assert
        expect(screen.queryByTestId("content-toggle")).not.toBeInTheDocument();
        expect(screen.queryByTestId("content-fade")).not.toBeInTheDocument();
      } finally {
        restore();
      }
    });
  });

  // Issue #116: ヘッダーのレイアウト統合・スター切替のアイコン化・元記事リンクの再配置
  describe("ヘッダーレイアウトと操作系の集約（#116）", () => {
    // Req 1.1 / 1.2: タイトル要素・はてブ数表示・スター切替コントロールが同一のヘッダー領域に左右配置される
    it("タイトル・はてブ数・スター切替が同一のヘッダー行に配置されること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const titleRow = screen.getByTestId("item-detail-title-row");
      const hatebu = screen.getByTestId("hatebu-count");
      const star = screen.getByTestId("star-toggle");

      // Assert: いずれもタイトル行コンテナの子孫として存在する
      expect(titleRow).toContainElement(hatebu);
      expect(titleRow).toContainElement(star);
      // タイトルテキストも同コンテナ内に存在する
      expect(titleRow).toHaveTextContent("テスト記事のタイトル");
    });

    // Req 1.5: はてブ数表示とスター切替がメタ情報グループとして隣接配置される
    it("はてブ数とスター切替が同一のメタ情報グループに隣接して配置されること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const metaGroup = screen.getByTestId("item-detail-meta-group");

      // Assert
      expect(metaGroup).toContainElement(screen.getByTestId("hatebu-count"));
      expect(metaGroup).toContainElement(screen.getByTestId("star-toggle"));
    });

    // Req 2.1: スター切替コントロールに文字列ラベル（「スター」「スター解除」）を表示しない
    it("スター切替コントロールにテキストラベルが表示されないこと（未スター時）", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const starButton = screen.getByTestId("star-toggle");

      // Assert: 旧文言「スター」「スター解除」は本文として描画されない
      // textContent はアイコンのみで空、もしくは空白のみであること
      expect(starButton.textContent?.trim()).toBe("");
    });

    it("スター切替コントロールにテキストラベルが表示されないこと（スター付き時）", () => {
      // Arrange
      const starredItem = { ...mockItem, is_starred: true };
      render(
        <ItemDetail
          item={starredItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const starButton = screen.getByTestId("star-toggle");

      // Assert
      expect(starButton.textContent?.trim()).toBe("");
    });

    // Req 2.7 / NFR 2.1: 支援技術が状態と用途を識別できる ARIA 属性を持つ
    it("スター切替コントロールに状態を示す aria-label と aria-pressed が付与されること（未スター時）", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const starButton = screen.getByTestId("star-toggle");

      // Assert
      expect(starButton).toHaveAttribute("aria-pressed", "false");
      expect(starButton.getAttribute("aria-label")).toMatch(/スター/);
    });

    it("スター切替コントロールに状態を示す aria-pressed が付与されること（スター付き時）", () => {
      // Arrange
      const starredItem = { ...mockItem, is_starred: true };
      render(
        <ItemDetail
          item={starredItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const starButton = screen.getByTestId("star-toggle");

      // Assert
      expect(starButton).toHaveAttribute("aria-pressed", "true");
    });

    // Req 2.2 / 2.3: 未スター時は塗りなしアイコン / スター付き時は塗りありアイコン
    it("未スター時はアイコンが塗りなし（text-muted-foreground）であること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const starButton = screen.getByTestId("star-toggle");
      const icon = starButton.querySelector("svg");

      // Assert
      expect(icon).not.toBeNull();
      expect(icon?.getAttribute("class") ?? "").toContain("text-muted-foreground");
      expect(icon?.getAttribute("class") ?? "").not.toContain("fill-yellow-400");
    });

    it("スター付き時はアイコンが塗りあり（fill-yellow-400）であること", () => {
      // Arrange
      const starredItem = { ...mockItem, is_starred: true };
      render(
        <ItemDetail
          item={starredItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const starButton = screen.getByTestId("star-toggle");
      const icon = starButton.querySelector("svg");

      // Assert
      expect(icon).not.toBeNull();
      expect(icon?.getAttribute("class") ?? "").toContain("fill-yellow-400");
    });

    // Req 3.4 / 3.5: はてブ数表示はアイコン + 数値、ツールチップで意味を補足
    it("はてブ数表示にブックマークアイコンとツールチップが付与されること（取得済み）", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const hatebu = screen.getByTestId("hatebu-count");

      // Assert: アイコン要素と数値テキストの両方が存在する
      expect(hatebu.querySelector("svg")).not.toBeNull();
      expect(hatebu).toHaveTextContent("42");
      // ツールチップ（title 属性）が "はてなブックマーク数" を含むこと
      expect(hatebu.getAttribute("title") ?? "").toMatch(/はてなブックマーク数/);
    });

    it("はてブ数表示のツールチップが未取得時に「未取得」を含むこと", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItemNoHatebu}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const hatebu = screen.getByTestId("hatebu-count");

      // Assert
      expect(hatebu.getAttribute("title") ?? "").toMatch(/未取得/);
    });

    // Req 4.1: 旧アクションバー（独立した横並び帯）が廃止されること
    // 旧実装の「元記事を開く」「はてブ数」「テキスト付きスター」の 3 要素を 1 つの兄弟要素内に
    // 並べる帯が存在していた。新実装ではメタ情報グループにはてブ数とスターのみが含まれ、
    // 元記事リンクは別行（著者行）に再配置される。
    it("元記事リンクがメタ情報グループの外側（著者行）に再配置されていること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const metaGroup = screen.getByTestId("item-detail-meta-group");
      const originalLink = screen.getByTestId("original-link");

      // Assert: 元記事リンクはメタ情報グループの子孫ではない
      expect(metaGroup).not.toContainElement(originalLink);
    });

    // Req 4.2 / 4.4: 著者名と元記事を開くが同一行に中点区切りで配置される
    it("著者名と元記事リンクが同一行に中点区切りで配置されること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const author = screen.getByTestId("item-detail-author");
      const separator = screen.getByTestId("item-detail-author-separator");
      const originalLink = screen.getByTestId("original-link");

      // Assert: 著者名と元記事リンクが同じ親要素を共有していること
      expect(author.parentElement).toBe(originalLink.parentElement);
      // 区切り記号も同じ親要素内に存在し、中点であること
      expect(separator.parentElement).toBe(author.parentElement);
      expect(separator).toHaveTextContent("・");
    });

    // Req 4.4: 元記事を開くリンクに外部リンクアイコンが付随表示される
    it("元記事リンクに外部リンクアイコンが付随表示されること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act
      const originalLink = screen.getByTestId("original-link");

      // Assert: SVG アイコン要素が含まれること
      expect(originalLink.querySelector("svg")).not.toBeNull();
    });

    // Req 4.5: 著者情報が存在しない場合は区切り記号を伴わず元記事リンクのみ表示
    it("著者情報が存在しない場合は区切り記号を表示せず元記事リンクのみ表示すること", () => {
      // Arrange
      const noAuthorItem: ItemDetailType = { ...mockItem, author: "" };
      render(
        <ItemDetail
          item={noAuthorItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act / Assert
      expect(
        screen.queryByTestId("item-detail-author")
      ).not.toBeInTheDocument();
      expect(
        screen.queryByTestId("item-detail-author-separator")
      ).not.toBeInTheDocument();
      // 元記事リンクは引き続き表示される
      expect(screen.getByTestId("original-link")).toBeInTheDocument();
    });

    // Req 5.2 / 5.3: スター切替直後にコントロールのアイコン表示が反転後の状態を反映する
    // 楽観的 UI 更新は親側（useToggleStar フック）の責務だが、押下後に親から渡される
    // 新しい is_starred 値で再レンダリングしたときアイコンが反転することを担保する。
    it("スター付き状態で再レンダリングするとアイコンが塗りありに切り替わること", () => {
      // Arrange
      const { rerender } = render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Act: 親が is_starred=true で再レンダリング
      rerender(
        <ItemDetail
          item={{ ...mockItem, is_starred: true }}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />
      );

      // Assert
      const icon = screen.getByTestId("star-toggle").querySelector("svg");
      expect(icon?.getAttribute("class") ?? "").toContain("fill-yellow-400");
      expect(screen.getByTestId("star-toggle")).toHaveAttribute(
        "aria-pressed",
        "true"
      );
    });

    // Req 5.3: スター切替の更新リクエストが失敗したときアイコン表示を操作前の状態に戻す
    // 楽観的 UI 更新 → 失敗ロールバックは親側（useToggleStar）の責務。ItemDetail は
    // 親から渡された is_starred の値を素直に反映することを検証する（rerender でロールバックを模倣）。
    it("更新失敗で is_starred が元の値に戻されたときアイコン表示が操作前の状態に戻ること", () => {
      // Arrange: スター付きで開始
      const starredItem = { ...mockItem, is_starred: true };
      const { rerender } = render(
        <ItemDetail
          item={starredItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // 楽観的に未スターへ反転（押下直後）
      rerender(
        <ItemDetail
          item={{ ...starredItem, is_starred: false }}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />
      );

      // Act: 更新失敗で親がロールバック → is_starred=true に戻る
      rerender(
        <ItemDetail
          item={{ ...starredItem, is_starred: true }}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />
      );

      // Assert: 操作前と同じ「塗りあり / aria-pressed=true」状態
      const icon = screen.getByTestId("star-toggle").querySelector("svg");
      expect(icon?.getAttribute("class") ?? "").toContain("fill-yellow-400");
      expect(screen.getByTestId("star-toggle")).toHaveAttribute(
        "aria-pressed",
        "true"
      );
    });

    // NFR 2.1: キーボード（Tab フォーカス・Enter 押下）でスター反転が呼び出せる
    it("スター切替コントロールがキーボード（Enter）で操作できること", async () => {
      // Arrange
      const user = userEvent.setup();
      const onToggleStar = vi.fn();
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={onToggleStar}
        />,
        { wrapper: createWrapper() }
      );

      // Act: フォーカス遷移は環境依存があるためボタンに直接フォーカスを当てる
      const starButton = screen.getByTestId("star-toggle");
      starButton.focus();
      expect(starButton).toHaveFocus();
      await user.keyboard("{Enter}");

      // Assert
      expect(onToggleStar).toHaveBeenCalledWith("item-1", true);
    });
  });
});
