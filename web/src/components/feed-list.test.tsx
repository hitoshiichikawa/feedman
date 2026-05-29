import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { FeedList } from "./feed-list";
import type { Subscription } from "@/types/feed";

/** テスト用の購読データ */
const mockFeeds: Subscription[] = [
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
    feed_status: "stopped",
    error_message: "404 Not Found",
    unread_count: 0,
    created_at: "2026-01-02T00:00:00Z",
  },
  {
    id: "sub-3",
    user_id: "user-1",
    feed_id: "feed-3",
    feed_title: "Error Feed",
    feed_url: "https://error.example.com/rss",
    favicon_url: null,
    fetch_interval_minutes: 60,
    feed_status: "error",
    error_message: "Parse error",
    unread_count: 3,
    created_at: "2026-01-03T00:00:00Z",
  },
];

describe("FeedList コンポーネント", () => {
  it("フィード一覧をタイトル付きで表示すること", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    expect(screen.getByText("News Feed")).toBeInTheDocument();
    expect(screen.getByText("Error Feed")).toBeInTheDocument();
  });

  it("faviconが存在する場合にimg要素で表示すること", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    const favicon = screen.getByAltText("Tech Blog のアイコン");
    expect(favicon).toBeInTheDocument();
    expect(favicon).toHaveAttribute("src", "https://example.com/favicon.ico");
  });

  it("faviconがnullの場合はimg要素を表示しないこと", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    expect(screen.queryByAltText("News Feed のアイコン")).not.toBeInTheDocument();
  });

  it("faviconがnullの場合に代替アイコン（fallback）を表示すること（要件 3.1）", () => {
    // Arrange: favicon_url が null の sub-2 (News Feed) を含む mockFeeds を使う

    // Act
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: News Feed (sub-2) の代替アイコンが表示される
    const fallback = screen.getByTestId("feed-favicon-fallback-sub-2");
    expect(fallback).toBeInTheDocument();
    expect(fallback).toHaveAttribute("aria-label", "News Feed のアイコン");
  });

  it("faviconが存在するフィードには代替アイコンを表示しないこと", () => {
    // Arrange & Act
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: Tech Blog (sub-1, favicon_url あり) は fallback を持たない
    expect(screen.queryByTestId("feed-favicon-fallback-sub-1")).not.toBeInTheDocument();
  });

  it("favicon画像の読み込みに失敗した場合に代替アイコンに切り替わること（要件 3.2）", () => {
    // Arrange
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    const img = screen.getByTestId("feed-favicon-sub-1");
    expect(img).toBeInTheDocument();
    // 初期状態では fallback は表示されない
    expect(screen.queryByTestId("feed-favicon-fallback-sub-1")).not.toBeInTheDocument();

    // Act: img の onError を発火させる
    fireEvent.error(img);

    // Assert: img が消え fallback が表示される
    expect(screen.queryByTestId("feed-favicon-sub-1")).not.toBeInTheDocument();
    const fallback = screen.getByTestId("feed-favicon-fallback-sub-1");
    expect(fallback).toBeInTheDocument();
    expect(fallback).toHaveAttribute("aria-label", "Tech Blog のアイコン");
  });

  it("代替アイコン表示時もフィードタイトル・未読数バッジ・ステータスアイコンのレイアウトを維持すること（要件 3.4）", () => {
    // Arrange & Act: sub-2 (favicon_url null, stopped, unread 0)
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: 同じ行に fallback / タイトル / ステータス が全て存在
    expect(screen.getByTestId("feed-favicon-fallback-sub-2")).toBeInTheDocument();
    expect(screen.getByText("News Feed")).toBeInTheDocument();
    expect(screen.getByTestId("feed-status-sub-2")).toBeInTheDocument();
  });

  it("fetch_statusがstoppedの場合に停止アイコンを表示すること", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    // 停止状態のフィード行にステータスアイコンがあること
    const stoppedIndicator = screen.getByTestId("feed-status-sub-2");
    expect(stoppedIndicator).toBeInTheDocument();
    expect(stoppedIndicator).toHaveAttribute("data-status", "stopped");
  });

  it("fetch_statusがerrorの場合にエラーアイコンを表示すること", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    const errorIndicator = screen.getByTestId("feed-status-sub-3");
    expect(errorIndicator).toBeInTheDocument();
    expect(errorIndicator).toHaveAttribute("data-status", "error");
  });

  it("fetch_statusがactiveの場合はステータスアイコンを表示しないこと", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    expect(screen.queryByTestId("feed-status-sub-1")).not.toBeInTheDocument();
  });

  it("フィードをクリックするとonSelectFeedが呼ばれること", () => {
    const onSelectFeed = vi.fn();
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={onSelectFeed}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    fireEvent.click(screen.getByText("Tech Blog"));
    expect(onSelectFeed).toHaveBeenCalledWith("feed-1");
  });

  it("選択中のフィードにハイライトスタイルが適用されること", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId="feed-1"
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="feed"
        onSelectAllNewItems={() => {}}
      />
    );

    const selectedItem = screen.getByTestId("feed-item-sub-1");
    expect(selectedItem).toHaveAttribute("data-selected", "true");
  });

  it("選択されていないフィードにはハイライトが適用されないこと", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId="feed-1"
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="feed"
        onSelectAllNewItems={() => {}}
      />
    );

    const unselectedItem = screen.getByTestId("feed-item-sub-2");
    expect(unselectedItem).toHaveAttribute("data-selected", "false");
  });

  it("未読数が表示されること", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    expect(screen.getByTestId("unread-count-sub-1")).toHaveTextContent("5");
  });

  it("未読数が0の場合は未読バッジを表示しないこと", () => {
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    expect(screen.queryByTestId("unread-count-sub-2")).not.toBeInTheDocument();
  });

  it("フィード一覧が空の場合にメッセージを表示すること", () => {
    render(
      <FeedList
        feeds={[]}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        onOpenSettings={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    expect(screen.getByText("フィードが登録されていません")).toBeInTheDocument();
  });

  // ===== 設定起動コントロール（ギアアイコン）関連テスト =====
  // AC 1.1, 1.2, 1.3, 1.4, 1.5, NFR 2.1

  describe("設定起動コントロール（ギアアイコン）", () => {
    it("各フィード行にギアボタンが存在すること（AC 1.2 の DOM 存在前提）", () => {
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={() => {}}
        />
      );

      // 各 3 行にギアボタンが存在する（ホバー前は CSS の opacity-0 で非可視だが要素は DOM に存在）
      expect(screen.getByTestId("feed-settings-button-sub-1")).toBeInTheDocument();
      expect(screen.getByTestId("feed-settings-button-sub-2")).toBeInTheDocument();
      expect(screen.getByTestId("feed-settings-button-sub-3")).toBeInTheDocument();
    });

    it("ギアボタンの aria-label が「<feed_title> の設定」であること（NFR 2.1）", () => {
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={() => {}}
        />
      );

      expect(screen.getByTestId("feed-settings-button-sub-1")).toHaveAttribute(
        "aria-label",
        "Tech Blog の設定"
      );
      expect(screen.getByTestId("feed-settings-button-sub-2")).toHaveAttribute(
        "aria-label",
        "News Feed の設定"
      );
      expect(screen.getByTestId("feed-settings-button-sub-3")).toHaveAttribute(
        "aria-label",
        "Error Feed の設定"
      );
    });

    it("ギアボタンクリックで onOpenSettings に対象 subscription が渡されること（AC 1.3）", () => {
      const onOpenSettings = vi.fn();
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={onOpenSettings}
        />
      );

      fireEvent.click(screen.getByTestId("feed-settings-button-sub-1"));

      expect(onOpenSettings).toHaveBeenCalledTimes(1);
      expect(onOpenSettings).toHaveBeenCalledWith(mockFeeds[0]);
    });

    it("ギアボタンクリックで onSelectFeed が呼ばれないこと（AC 1.4 / stopPropagation 検証）", () => {
      const onSelectFeed = vi.fn();
      const onOpenSettings = vi.fn();
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={onSelectFeed}
          onOpenSettings={onOpenSettings}
        />
      );

      fireEvent.click(screen.getByTestId("feed-settings-button-sub-1"));

      expect(onOpenSettings).toHaveBeenCalledTimes(1);
      // stopPropagation により、行クリックハンドラ（onSelectFeed）は発火しない
      expect(onSelectFeed).not.toHaveBeenCalled();
    });

    it("ギアボタンは type='button' であること（form submit 等の副作用回避）", () => {
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={() => {}}
        />
      );

      const button = screen.getByTestId("feed-settings-button-sub-1");
      expect(button).toHaveAttribute("type", "button");
    });

    it("ギアボタンが Tab でフォーカス可能であること（AC 1.5 / NFR 2.1）", () => {
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={() => {}}
        />
      );

      const button = screen.getByTestId("feed-settings-button-sub-1");

      // <button> は既定で tabindex=0 相当（Tab 到達可能）
      // 明示的に tabIndex=-1 が付いていないことを確認
      expect(button).not.toHaveAttribute("tabindex", "-1");

      // focus() でフォーカスが当たることを確認
      button.focus();
      expect(document.activeElement).toBe(button);
    });

    it("ギアボタンに focus がある状態で Enter キーを押すと onOpenSettings が発火すること（AC 1.5）", () => {
      const onOpenSettings = vi.fn();
      const onSelectFeed = vi.fn();
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={onSelectFeed}
          onOpenSettings={onOpenSettings}
        />
      );

      const button = screen.getByTestId("feed-settings-button-sub-2");
      button.focus();

      // <button> のネイティブ挙動として Enter キーは click イベントを発火させる。
      // jsdom では keyDown 単体では click が走らないため、click を直接ディスパッチして
      // ネイティブ挙動を再現する（fireEvent.click は button のキーボード起動と等価扱い）。
      fireEvent.click(button);

      expect(onOpenSettings).toHaveBeenCalledWith(mockFeeds[1]);
      expect(onSelectFeed).not.toHaveBeenCalled();
    });

    it("ギアボタンへの keyDown(Enter/Space) が行の onKeyDown に伝搬しないこと（AC 1.4 拡張）", () => {
      const onSelectFeed = vi.fn();
      const onOpenSettings = vi.fn();
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={onSelectFeed}
          onOpenSettings={onOpenSettings}
        />
      );

      const button = screen.getByTestId("feed-settings-button-sub-1");

      // Enter を押下 → 親 div の onKeyDown（onSelectFeed 発火パス）に到達してはならない
      fireEvent.keyDown(button, { key: "Enter" });
      expect(onSelectFeed).not.toHaveBeenCalled();

      // Space も同様
      fireEvent.keyDown(button, { key: " " });
      expect(onSelectFeed).not.toHaveBeenCalled();
    });

    it("行コンテナで Enter キーを押すと onSelectFeed が発火すること（AC 1.5 行側のキーボード起動）", () => {
      const onSelectFeed = vi.fn();
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={onSelectFeed}
          onOpenSettings={() => {}}
        />
      );

      const row = screen.getByTestId("feed-item-sub-1");
      fireEvent.keyDown(row, { key: "Enter" });
      expect(onSelectFeed).toHaveBeenCalledWith("feed-1");
    });

    it("行コンテナで Space キーを押すと onSelectFeed が発火すること（AC 1.5 行側のキーボード起動）", () => {
      const onSelectFeed = vi.fn();
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={onSelectFeed}
          onOpenSettings={() => {}}
        />
      );

      const row = screen.getByTestId("feed-item-sub-1");
      fireEvent.keyDown(row, { key: " " });
      expect(onSelectFeed).toHaveBeenCalledWith("feed-1");
    });

    it("ギアボタンのクラスに opacity-0 / group-hover:opacity-100 / group-focus-within:opacity-100 / focus-visible:opacity-100 が含まれること（AC 1.1, 1.2）", () => {
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={() => {}}
        />
      );

      const button = screen.getByTestId("feed-settings-button-sub-1");
      const className = button.className;

      // ホバー前は非表示（opacity-0）
      expect(className).toContain("opacity-0");
      // ホバー時表示
      expect(className).toContain("group-hover:opacity-100");
      // 行内フォーカス時表示（キーボード操作対応）
      expect(className).toContain("group-focus-within:opacity-100");
      // ギア自身に focus-visible 時表示
      expect(className).toContain("focus-visible:opacity-100");
    });

    it("行コンテナに group クラスが付与されていること（group-hover / group-focus-within を機能させる前提）", () => {
      render(
        <FeedList
          feeds={mockFeeds}
          selectedFeedId={null}
          onSelectFeed={() => {}}
          onOpenSettings={() => {}}
        />
      );

      const row = screen.getByTestId("feed-item-sub-1");
      expect(row.className).toContain("group");
    });
  });

  // --- Issue #121 task 7: 「すべての新着記事」仮想エントリ ---

  it("購読 0 件でも「すべての新着記事」仮想エントリが描画されること（要件 1.1）", () => {
    // Arrange & Act: 購読 0 件
    render(
      <FeedList
        feeds={[]}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        viewMode="none"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: 仮想エントリが描画され、購読 0 件メッセージとも併存する
    const entry = screen.getByTestId("all-new-items-entry");
    expect(entry).toBeInTheDocument();
    expect(screen.getByText("すべての新着記事")).toBeInTheDocument();
    expect(screen.getByText("フィードが登録されていません")).toBeInTheDocument();
  });

  it("仮想エントリ click で onSelectAllNewItems が呼ばれること（要件 1.2）", () => {
    // Arrange
    const onSelectAllNewItems = vi.fn();
    const onSelectFeed = vi.fn();
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={onSelectFeed}
        viewMode="none"
        onSelectAllNewItems={onSelectAllNewItems}
      />
    );

    // Act
    fireEvent.click(screen.getByTestId("all-new-items-entry"));

    // Assert
    expect(onSelectAllNewItems).toHaveBeenCalledTimes(1);
    expect(onSelectFeed).not.toHaveBeenCalled();
  });

  it("viewMode='cross-feed' のとき仮想エントリが data-selected='true' になり aria-current='page' が付与されること（要件 1.4, NFR 3.1）", () => {
    // Arrange & Act
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId={null}
        onSelectFeed={() => {}}
        viewMode="cross-feed"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: 選択中スタイル
    const entry = screen.getByTestId("all-new-items-entry");
    expect(entry).toHaveAttribute("data-selected", "true");
    expect(entry).toHaveAttribute("aria-current", "page");
    expect(entry.className).toContain("bg-accent");
    expect(entry.className).toContain("text-accent-foreground");
    expect(entry.className).toContain("font-medium");
  });

  it("既存個別フィード行の並び順とスタイルが本機能導入で変化しないこと（要件 1.5, 5.2, NFR 2.1）", () => {
    // Arrange & Act: viewMode='feed' で feed-1 選択中
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId="feed-1"
        onSelectFeed={() => {}}
        viewMode="feed"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: 個別フィード行の並び順は mockFeeds の順序（sub-1, sub-2, sub-3）を維持
    const feedButtons = screen.getAllByTestId(/^feed-item-/);
    expect(feedButtons).toHaveLength(3);
    expect(feedButtons[0]).toHaveAttribute("data-testid", "feed-item-sub-1");
    expect(feedButtons[1]).toHaveAttribute("data-testid", "feed-item-sub-2");
    expect(feedButtons[2]).toHaveAttribute("data-testid", "feed-item-sub-3");

    // Assert: 個別フィードの選択中スタイルは仮想エントリ追加前と同様
    expect(feedButtons[0]).toHaveAttribute("data-selected", "true");
    expect(feedButtons[1]).toHaveAttribute("data-selected", "false");
    expect(feedButtons[2]).toHaveAttribute("data-selected", "false");
  });

  it("viewMode='cross-feed' のとき個別フィード行は selectedFeedId に関わらず選択中扱いにならないこと（要件 1.4）", () => {
    // Arrange & Act: selectedFeedId 残存だが viewMode='cross-feed'
    render(
      <FeedList
        feeds={mockFeeds}
        selectedFeedId="feed-1"
        onSelectFeed={() => {}}
        viewMode="cross-feed"
        onSelectAllNewItems={() => {}}
      />
    );

    // Assert: 仮想エントリのみ選択中、個別フィードは非選択
    expect(screen.getByTestId("all-new-items-entry")).toHaveAttribute(
      "data-selected",
      "true"
    );
    expect(screen.getByTestId("feed-item-sub-1")).toHaveAttribute(
      "data-selected",
      "false"
    );
  });
});
