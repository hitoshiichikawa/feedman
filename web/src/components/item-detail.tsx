"use client";

import { useEffect } from "react";
import { ExternalLink, Star, Bookmark } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ItemDetail as ItemDetailType } from "@/types/item";

/** ItemDetail コンポーネントのプロパティ */
interface ItemDetailProps {
  /** 表示する記事データ */
  item: ItemDetailType;
  /** 既読にするコールバック */
  onMarkAsRead: (itemId: string) => void;
  /** スター切替コールバック */
  onToggleStar: (itemId: string, isStarred: boolean) => void;
}

/**
 * 記事展開表示コンポーネント
 *
 * サニタイズ済みHTMLコンテンツの展開表示、元記事URL遷移ボタン、
 * はてなブックマーク数の表示、スター切替を提供する。
 * 展開時に自動的に既読状態にする。
 */
export function ItemDetail({
  item,
  onMarkAsRead,
  onToggleStar,
}: ItemDetailProps) {
  // 展開時に未読なら自動的に既読にする
  useEffect(() => {
    if (!item.is_read) {
      onMarkAsRead(item.id);
    }
    // item.idが変わった時のみ実行（展開された記事が変わった時）
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id]);

  /** はてブ数の表示テキスト */
  const hatebuDisplay =
    item.hatebu_fetched_at === null ? "-" : String(item.hatebu_count);

  return (
    <div className="border-t bg-background px-4 py-4 space-y-4">
      {/* ヘッダー: タイトル + メタ情報 */}
      <div className="space-y-2">
        <h3 className="text-lg font-semibold leading-tight">{item.title}</h3>
        {item.author && (
          <p className="text-sm text-muted-foreground">{item.author}</p>
        )}
      </div>

      {/* アクションバー */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* 元記事リンク */}
        <a
          href={item.link}
          target="_blank"
          rel="noopener noreferrer"
          data-testid="original-link"
          className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
        >
          <ExternalLink className="w-4 h-4" />
          元記事を開く
        </a>

        {/* はてなブックマーク数 */}
        <span
          data-testid="hatebu-count"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground"
          title={
            item.hatebu_fetched_at
              ? `${item.hatebu_count} users`
              : "はてなブックマーク数未取得"
          }
        >
          <Bookmark className="w-4 h-4" />
          {hatebuDisplay}
        </span>

        {/* スター切替ボタン */}
        <Button
          variant="ghost"
          size="sm"
          data-testid="star-toggle"
          className="inline-flex items-center gap-1"
          onClick={() => onToggleStar(item.id, !item.is_starred)}
        >
          <Star
            className={cn(
              "w-4 h-4",
              item.is_starred
                ? "fill-yellow-400 text-yellow-400"
                : "text-muted-foreground"
            )}
          />
          {item.is_starred ? "スター解除" : "スター"}
        </Button>
      </div>

      {/* コンテンツ表示エリア */}
      <div
        data-testid="item-content"
        className="prose prose-sm dark:prose-invert max-w-none"
        dangerouslySetInnerHTML={{ __html: item.content }}
      />
    </div>
  );
}
