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
    // Req 3.3: 「元記事を開く」テキストとアイコンを伴うリンクであること
    expect(linkButton).toHaveTextContent("元記事を開く");
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

    // コンポーネントのマウント時（展開時）にonMarkAsReadが呼ばれること（Req 3.4）
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

    // 既に既読なのでonMarkAsReadは呼ばれない（Req 3.4 既存挙動維持）
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

    // Req 3.3: タイトル維持
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

    // Req 3.3: 著者維持
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

  // Issue #154 / Task 7: 詳細ヘッダーからのメタ撤去（Req 3.1 / 3.2 / NFR 3.3）
  describe("詳細ヘッダーからのはてブ数・スター撤去（#154）", () => {
    // Req 3.1: 詳細ヘッダーにはてなブックマーク数表示を表示しない
    it("ヘッダー領域にはてブ数表示（hatebu-count）が存在しないこと", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Assert: 旧 testid が DOM から完全に撤去されていること
      expect(screen.queryByTestId("hatebu-count")).not.toBeInTheDocument();
    });

    // Req 3.1: はてブ未取得記事でも hatebu-count は出現しない（境界）
    it("はてブ未取得記事でもヘッダー領域に hatebu-count が出現しないこと", () => {
      // Arrange
      const noHatebuItem: ItemDetailType = {
        ...mockItem,
        hatebu_count: 0,
        hatebu_fetched_at: null,
      };

      render(
        <ItemDetail
          item={noHatebuItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Assert
      expect(screen.queryByTestId("hatebu-count")).not.toBeInTheDocument();
    });

    // Req 3.2: 詳細ヘッダーにスター切替トグルを表示しない（未スター時）
    it("ヘッダー領域にスター切替トグル（star-toggle）が存在しないこと（未スター時）", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Assert
      expect(screen.queryByTestId("star-toggle")).not.toBeInTheDocument();
    });

    // Req 3.2: スター付き記事でもスター切替トグルは存在しない
    it("ヘッダー領域にスター切替トグル（star-toggle）が存在しないこと（スター付き時）", () => {
      // Arrange
      const starredItem: ItemDetailType = { ...mockItem, is_starred: true };

      render(
        <ItemDetail
          item={starredItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Assert
      expect(screen.queryByTestId("star-toggle")).not.toBeInTheDocument();
    });

    // NFR 3.3 / Req 3.1 / 3.2: メタ情報グループ自体が DOM から撤去されている
    it("ヘッダーのメタ情報グループ（item-detail-meta-group）が存在しないこと", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Assert
      expect(
        screen.queryByTestId("item-detail-meta-group")
      ).not.toBeInTheDocument();
    });

    // Req 3.2: onToggleStar prop は型互換維持のため残置されているが、本体内で呼び出されない
    it("詳細ヘッダー内のクリックで onToggleStar が呼ばれないこと（ヘッダーに star トグルが存在しないため）", async () => {
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

      // Act: タイトル行をクリックしても callback は発火しない
      const titleRow = screen.getByTestId("item-detail-title-row");
      await user.click(titleRow);

      // Assert
      expect(onToggleStar).not.toHaveBeenCalled();
    });

    // Req 3.3: タイトル / 著者 / 元記事リンク / タイトル行コンテナは維持される
    it("タイトル・著者・元記事リンク・タイトル行コンテナが維持されること", () => {
      // Arrange
      render(
        <ItemDetail
          item={mockItem}
          onMarkAsRead={() => {}}
          onToggleStar={() => {}}
        />,
        { wrapper: createWrapper() }
      );

      // Assert: 既存 testid が引き続き存在
      expect(screen.getByTestId("item-detail-title-row")).toBeInTheDocument();
      expect(screen.getByTestId("item-detail-author")).toBeInTheDocument();
      expect(
        screen.getByTestId("item-detail-author-separator")
      ).toBeInTheDocument();
      expect(screen.getByTestId("original-link")).toBeInTheDocument();
      // タイトル文字列も表示されている
      expect(screen.getByText("テスト記事のタイトル")).toBeInTheDocument();
    });

    // Req 4.5（既存）: 著者情報が無い場合は区切り記号と著者を表示せず元記事リンクのみ
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
      expect(screen.getByTestId("original-link")).toBeInTheDocument();
    });

    // Req 3.3: 元記事リンクに外部リンクアイコンが付随表示される（既存挙動維持）
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
  });
});
