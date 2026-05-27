"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, Star, Bookmark } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { sanitizeContentHtml } from "@/lib/sanitize";
import type { ItemDetail as ItemDetailType } from "@/types/item";

/**
 * 本文表示エリアの初期折りたたみ時の最大高さ（px）。
 * これを超える本文は折りたたんでクリップし「続きを読む」トグルを表示する。
 */
const COLLAPSED_MAX_HEIGHT_PX = 300;

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

  // 記事本文を DOM 挿入前にクライアント側でサニタイズする（多層防御）。
  // 同一記事本文に対する再計算を避けるため item.content をキーにメモ化する。
  const sanitizedContent = useMemo(
    () => sanitizeContentHtml(item.content),
    [item.content]
  );

  // 本文コンテナの参照。レンダリング後に実コンテンツ高さ（scrollHeight）を測定する。
  const contentRef = useRef<HTMLDivElement>(null);
  // 本文の実高さが閾値（300px）を超えるか（= 折りたたみ対象か）。
  const [isOverflowing, setIsOverflowing] = useState(false);
  // 利用者が「続きを読む」で全文表示へ展開したか。
  const [isExpanded, setIsExpanded] = useState(false);

  // 本文の実コンテンツ高さを測定し、閾値超過かどうかを判定する。
  // 文字列で切り取らず DOM ツリーを維持したまま CSS でクリップするため、
  // 高さ判定は描画後の scrollHeight を用いる（Req 1.4 / NFR 1.1）。
  // item.content が変わったら再測定し、展開状態も初期（折りたたみ）に戻す。
  useLayoutEffect(() => {
    const el = contentRef.current;
    if (el === null) {
      return;
    }
    setIsOverflowing(el.scrollHeight > COLLAPSED_MAX_HEIGHT_PX);
    setIsExpanded(false);
  }, [sanitizedContent]);

  // 折りたたみ中（閾値超過かつ未展開）はクリップとフェードアウトを表示する。
  const isCollapsed = isOverflowing && !isExpanded;

  return (
    <div className="border-t bg-background px-4 py-4 space-y-4">
      {/* ヘッダー: タイトル + メタ情報 */}
      <div className="space-y-2">
        <h3 className="text-lg font-semibold leading-tight">
          <a
            href={item.link}
            target="_blank"
            rel="noopener noreferrer"
            className="hover:underline"
          >
            {item.title}
          </a>
        </h3>
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

      {/* コンテンツ表示エリア（本文 HTML の DOM を維持したまま CSS でクリップする） */}
      <div className="relative">
        <div
          ref={contentRef}
          data-testid="item-content"
          className={cn(
            "prose prose-sm dark:prose-invert max-w-none",
            isCollapsed && "max-h-[300px] overflow-hidden"
          )}
          dangerouslySetInnerHTML={{ __html: sanitizedContent }}
        />
        {/* 折りたたみ時のフェードアウト（下から上へのグラデーション） */}
        {isCollapsed && (
          <div
            data-testid="content-fade"
            aria-hidden="true"
            className="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-background to-transparent"
          />
        )}
      </div>

      {/* 「続きを読む」/「折りたたむ」トグル（閾値超過時のみ表示） */}
      {isOverflowing && (
        <Button
          variant="ghost"
          size="sm"
          data-testid="content-toggle"
          onClick={() => setIsExpanded((prev) => !prev)}
        >
          {isExpanded ? "折りたたむ" : "続きを読む"}
        </Button>
      )}
    </div>
  );
}
