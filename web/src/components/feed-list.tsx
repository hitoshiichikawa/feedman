"use client";

import { cn } from "@/lib/utils";
import type { Subscription } from "@/types/feed";
import { CircleAlert, CirclePause } from "lucide-react";

/** FeedList コンポーネントのプロパティ */
interface FeedListProps {
  /** 購読一覧データ */
  feeds: Subscription[];
  /** 現在選択中のフィードID（null = 未選択） */
  selectedFeedId: string | null;
  /** フィード選択イベントハンドラ */
  onSelectFeed: (feedId: string) => void;
}

/**
 * フィード一覧パネル（左ペイン）
 *
 * フィード一覧をタイトルとfavicon付きで表示する。
 * フィードのfetch_statusが停止/エラーの場合にアイコンで状態を表示する。
 * フィード選択イベントを親コンポーネントに通知する。
 */
export function FeedList({ feeds, selectedFeedId, onSelectFeed }: FeedListProps) {
  if (feeds.length === 0) {
    return (
      <div className="flex items-center justify-center p-4 text-sm text-muted-foreground">
        フィードが登録されていません
      </div>
    );
  }

  return (
    <nav className="flex flex-col gap-0.5" role="list">
      {feeds.map((feed) => {
        const isSelected = feed.feed_id === selectedFeedId;

        return (
          <button
            key={feed.id}
            data-testid={`feed-item-${feed.id}`}
            data-selected={isSelected ? "true" : "false"}
            className={cn(
              "flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors",
              "hover:bg-accent hover:text-accent-foreground",
              isSelected && "bg-accent text-accent-foreground font-medium"
            )}
            onClick={() => onSelectFeed(feed.feed_id)}
          >
            {/* favicon */}
            <div className="flex-shrink-0 w-4 h-4">
              {feed.favicon_url ? (
                <img
                  src={feed.favicon_url}
                  alt={`${feed.feed_title} のアイコン`}
                  className="w-4 h-4 rounded-sm object-contain"
                />
              ) : (
                <div className="w-4 h-4" />
              )}
            </div>

            {/* タイトル */}
            <span className="flex-1 truncate">{feed.feed_title}</span>

            {/* ステータスアイコン（停止/エラー時のみ） */}
            {feed.feed_status === "stopped" && (
              <span
                data-testid={`feed-status-${feed.id}`}
                data-status="stopped"
                title={feed.error_message ?? "フェッチ停止中"}
                className="flex-shrink-0 text-muted-foreground"
              >
                <CirclePause className="w-4 h-4" />
              </span>
            )}
            {feed.feed_status === "error" && (
              <span
                data-testid={`feed-status-${feed.id}`}
                data-status="error"
                title={feed.error_message ?? "エラー"}
                className="flex-shrink-0 text-destructive"
              >
                <CircleAlert className="w-4 h-4" />
              </span>
            )}

            {/* 未読数バッジ */}
            {feed.unread_count > 0 && (
              <span
                data-testid={`unread-count-${feed.id}`}
                className="flex-shrink-0 min-w-5 rounded-full bg-primary px-1.5 py-0.5 text-center text-xs font-medium text-primary-foreground"
              >
                {feed.unread_count}
              </span>
            )}
          </button>
        );
      })}
    </nav>
  );
}
