"use client";

import { cn } from "@/lib/utils";
import type { Subscription } from "@/types/feed";
import { CircleAlert, CirclePause, Rss, Settings } from "lucide-react";
import { useState } from "react";

/** FeedList コンポーネントのプロパティ */
interface FeedListProps {
  /** 購読一覧データ */
  feeds: Subscription[];
  /** 現在選択中のフィードID（null = 未選択） */
  selectedFeedId: string | null;
  /** フィード選択イベントハンドラ */
  onSelectFeed: (feedId: string) => void;
  /**
   * 設定起動コントロール（ギアアイコン）クリック時のハンドラ。
   * 対象 subscription を引数に呼ばれる。
   *
   * AC 1.3: クリックで購読設定パネルを開く。
   * AC 1.4: フィード選択イベント（onSelectFeed）は発火しない。
   */
  onOpenSettings: (subscription: Subscription) => void;
}

/**
 * フィード一覧パネル（左ペイン）
 *
 * フィード一覧をタイトルとfavicon付きで表示する。
 * フィードのfetch_statusが停止/エラーの場合にアイコンで状態を表示する。
 * フィード選択イベントを親コンポーネントに通知する。
 *
 * 各行末尾にはホバー / 行内フォーカス時にギアアイコン（設定起動コントロール）を表示し、
 * クリック / Enter / Space で onOpenSettings(subscription) を発火する（AC 1.1, 1.2, 1.3, 1.5）。
 * ギアクリック時は e.stopPropagation() で onSelectFeed の発火を抑止する（AC 1.4）。
 */
export function FeedList({
  feeds,
  selectedFeedId,
  onSelectFeed,
  onOpenSettings,
}: FeedListProps) {
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

        // 行クリック / キーボード起動で onSelectFeed を発火する共通ハンドラ。
        // 行コンテナは <button> ネスト回避のため <div role="button" tabIndex={0}> 化した。
        const handleRowKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onSelectFeed(feed.feed_id);
          }
        };

        return (
          <div
            key={feed.id}
            role="button"
            tabIndex={0}
            data-testid={`feed-item-${feed.id}`}
            data-selected={isSelected ? "true" : "false"}
            className={cn(
              "group flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors cursor-pointer",
              "hover:bg-accent hover:text-accent-foreground",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              isSelected && "bg-accent text-accent-foreground font-medium"
            )}
            onClick={() => onSelectFeed(feed.feed_id)}
            onKeyDown={handleRowKeyDown}
          >
            <FeedFavicon
              feedId={feed.id}
              faviconURL={feed.favicon_url ?? null}
              feedTitle={feed.feed_title}
            />

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

            {/* 設定起動コントロール（ギアアイコン）
                AC 1.1 / 1.2: ホバー前は非表示、ホバー / 行内フォーカス時に表示。
                AC 1.4: stopPropagation で行クリックの onSelectFeed を発火させない。
                AC 1.5 / NFR 2.1: type="button" + aria-label でキーボード到達と読み上げ対応。 */}
            <button
              type="button"
              data-testid={`feed-settings-button-${feed.id}`}
              aria-label={`${feed.feed_title} の設定`}
              className={cn(
                "flex-shrink-0 rounded p-1 text-muted-foreground",
                "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100",
                "hover:bg-accent-foreground/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              )}
              onClick={(e) => {
                e.stopPropagation();
                onOpenSettings(feed);
              }}
              onKeyDown={(e) => {
                // Enter / Space は button のデフォルト挙動でクリック発火するが、
                // 親 onKeyDown へ伝搬すると行の onSelectFeed も二重発火するため止める。
                if (e.key === "Enter" || e.key === " ") {
                  e.stopPropagation();
                }
              }}
            >
              <Settings className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        );
      })}
    </nav>
  );
}

/** FeedFavicon のプロパティ */
interface FeedFaviconProps {
  /** 購読 ID（test-id 用） */
  feedId: string;
  /** favicon の URL（null/空文字なら代替アイコン表示） */
  faviconURL: string | null;
  /** フィードタイトル（alt 属性用） */
  feedTitle: string;
}

/**
 * フィード行先頭の favicon 表示コンポーネント。
 *
 * favicon URL が設定されていない場合、および <img> の onError が発火した場合は
 * lucide-react の Rss アイコンをデフォルトアイコンとして表示する。
 * 表示サイズは w-4 h-4 で実 favicon と同じ。レイアウトは変化しない（要件 3.3, 3.4）。
 */
function FeedFavicon({ feedId, faviconURL, feedTitle }: FeedFaviconProps) {
  const [imgFailed, setImgFailed] = useState(false);

  const hasURL = faviconURL !== null && faviconURL !== "";
  const showImage = hasURL && !imgFailed;

  return (
    <div className="flex-shrink-0 w-4 h-4">
      {showImage ? (
        <img
          data-testid={`feed-favicon-${feedId}`}
          src={faviconURL}
          alt={`${feedTitle} のアイコン`}
          className="w-4 h-4 rounded-sm object-contain"
          onError={() => setImgFailed(true)}
        />
      ) : (
        <span
          data-testid={`feed-favicon-fallback-${feedId}`}
          aria-label={`${feedTitle} のアイコン`}
          role="img"
          className="inline-flex items-center justify-center w-4 h-4 text-muted-foreground"
        >
          <Rss className="w-4 h-4" aria-hidden="true" />
        </span>
      )}
    </div>
  );
}
