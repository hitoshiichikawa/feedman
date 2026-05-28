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
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    expect(screen.getByText("Tech Blog")).toBeInTheDocument();
    expect(screen.getByText("News Feed")).toBeInTheDocument();
    expect(screen.getByText("Error Feed")).toBeInTheDocument();
  });

  it("faviconが存在する場合にimg要素で表示すること", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    const favicon = screen.getByAltText("Tech Blog のアイコン");
    expect(favicon).toBeInTheDocument();
    expect(favicon).toHaveAttribute("src", "https://example.com/favicon.ico");
  });

  it("faviconがnullの場合はimg要素を表示しないこと", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    expect(screen.queryByAltText("News Feed のアイコン")).not.toBeInTheDocument();
  });

  it("faviconがnullの場合に代替アイコン（fallback）を表示すること（要件 3.1）", () => {
    // Arrange: favicon_url が null の sub-2 (News Feed) を含む mockFeeds を使う

    // Act
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    // Assert: News Feed (sub-2) の代替アイコンが表示される
    const fallback = screen.getByTestId("feed-favicon-fallback-sub-2");
    expect(fallback).toBeInTheDocument();
    expect(fallback).toHaveAttribute("aria-label", "News Feed のアイコン");
  });

  it("faviconが存在するフィードには代替アイコンを表示しないこと", () => {
    // Arrange & Act
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    // Assert: Tech Blog (sub-1, favicon_url あり) は fallback を持たない
    expect(screen.queryByTestId("feed-favicon-fallback-sub-1")).not.toBeInTheDocument();
  });

  it("favicon画像の読み込みに失敗した場合に代替アイコンに切り替わること（要件 3.2）", () => {
    // Arrange
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
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
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    // Assert: 同じ行に fallback / タイトル / ステータス が全て存在
    expect(screen.getByTestId("feed-favicon-fallback-sub-2")).toBeInTheDocument();
    expect(screen.getByText("News Feed")).toBeInTheDocument();
    expect(screen.getByTestId("feed-status-sub-2")).toBeInTheDocument();
  });

  it("fetch_statusがstoppedの場合に停止アイコンを表示すること", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    // 停止状態のフィード行にステータスアイコンがあること
    const stoppedIndicator = screen.getByTestId("feed-status-sub-2");
    expect(stoppedIndicator).toBeInTheDocument();
    expect(stoppedIndicator).toHaveAttribute("data-status", "stopped");
  });

  it("fetch_statusがerrorの場合にエラーアイコンを表示すること", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    const errorIndicator = screen.getByTestId("feed-status-sub-3");
    expect(errorIndicator).toBeInTheDocument();
    expect(errorIndicator).toHaveAttribute("data-status", "error");
  });

  it("fetch_statusがactiveの場合はステータスアイコンを表示しないこと", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
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
      />
    );

    const unselectedItem = screen.getByTestId("feed-item-sub-2");
    expect(unselectedItem).toHaveAttribute("data-selected", "false");
  });

  it("未読数が表示されること", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    expect(screen.getByTestId("unread-count-sub-1")).toHaveTextContent("5");
  });

  it("未読数が0の場合は未読バッジを表示しないこと", () => {
    render(
      <FeedList feeds={mockFeeds} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    expect(screen.queryByTestId("unread-count-sub-2")).not.toBeInTheDocument();
  });

  it("フィード一覧が空の場合にメッセージを表示すること", () => {
    render(
      <FeedList feeds={[]} selectedFeedId={null} onSelectFeed={() => {}} />
    );

    expect(screen.getByText("フィードが登録されていません")).toBeInTheDocument();
  });
});
