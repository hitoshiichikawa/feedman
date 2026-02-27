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
