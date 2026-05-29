"use client";

import { FeedFavicon } from "@/components/feed-favicon";
import type { ViewMode } from "@/contexts/app-state";
import { cn } from "@/lib/utils";
import type { Subscription } from "@/types/feed";
import { CircleAlert, CirclePause, Settings } from "lucide-react";

/** FeedList コンポーネントのプロパティ */
interface FeedListProps {
  /** 購読一覧データ */
  feeds: Subscription[];
  /** 現在選択中のフィードID（null = 未選択 / 横断新着一覧選択中も null） */
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
  /**
   * 現在の表示モード（Issue #121 / Req 1.4）。
   * 'cross-feed' のとき「すべての新着記事」仮想エントリを選択中スタイルで表示する。
   */
  viewMode: ViewMode;
  /** 「すべての新着記事」仮想エントリ選択時のハンドラ（Req 1.2） */
  onSelectAllNewItems: () => void;
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
 *
 * 一覧の **先頭**に「すべての新着記事」仮想エントリを常設し（Req 1.1、購読 0 件でも表示）、
 * 当該エントリを選択することで横断新着一覧表示へ切替える（Req 1.2）。
 */
export function FeedList({
  feeds,
  selectedFeedId,
  onSelectFeed,
  onOpenSettings,
  viewMode,
  onSelectAllNewItems,
}: FeedListProps) {
  const isAllNewItemsSelected = viewMode === "cross-feed";

  return (
    <nav className="flex flex-col gap-0.5" role="list">
      {/* 「すべての新着記事」仮想エントリ（Req 1.1, 1.4, 1.5）。
          購読 0 件でも常設表示する（Req 1.1）。 */}
      <button
        data-testid="all-new-items-entry"
        data-selected={isAllNewItemsSelected ? "true" : "false"}
        aria-current={isAllNewItemsSelected ? "page" : undefined}
        className={cn(
          "flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors",
          "hover:bg-accent hover:text-accent-foreground",
          isAllNewItemsSelected && "bg-accent text-accent-foreground font-medium"
        )}
        onClick={onSelectAllNewItems}
      >
        <FeedFavicon
          feedId="__all__"
          faviconURL={null}
          feedTitle="すべての新着記事"
        />

        <span className="flex-1 truncate">すべての新着記事</span>
      </button>

      {feeds.map((feed) => {
        const isSelected =
          viewMode !== "cross-feed" && feed.feed_id === selectedFeedId;

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

      {/* 購読 0 件時のメッセージ（Req 1.1: 仮想エントリ自体は常時表示。
          メッセージは仮想エントリの後ろに配置する）。 */}
      {feeds.length === 0 && (
        <div className="flex items-center justify-center p-4 text-sm text-muted-foreground">
          フィードが登録されていません
        </div>
      )}
    </nav>
  );
}
