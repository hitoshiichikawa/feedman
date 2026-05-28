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
 * ヘッダー領域にタイトル（左）・はてなブックマーク数表示・スター切替
 * （アイコンのみのトグル）を同一行に左右配置し、著者名と「元記事を開く」リンクを
 * 中点区切りで隣接配置する。本文はサニタイズ済み HTML を 300px で折りたたみ、
 * 「続きを読む」トグルで全文展開する。展開時に未読記事は自動的に既読にする。
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

  // スター切替コントロールのアクセシブル名。状態と用途を支援技術へ伝える（Req 2.7）。
  const starLabel = item.is_starred ? "スターを解除する" : "スターを付ける";
  // はてブ数のツールチップ。未取得時は数値ではなく取得状況を伝える（Req 3.5）。
  const hatebuTitle = item.hatebu_fetched_at
    ? `はてなブックマーク数: ${item.hatebu_count}`
    : "はてなブックマーク数未取得";

  return (
    <div className="border-t bg-background px-4 py-4 space-y-4">
      {/* ヘッダー: タイトル(左) + メタ情報(右) を同一行に左右配置（Req 1.1〜1.5） */}
      <div className="space-y-1">
        <div
          data-testid="item-detail-title-row"
          className="flex items-start gap-3"
        >
          {/* タイトルリンク（長文時は折り返し、shrink を許容するため min-w-0 を付与）。 */}
          <h3 className="flex-1 min-w-0 text-lg font-semibold leading-tight">
            <a
              href={item.link}
              target="_blank"
              rel="noopener noreferrer"
              className="hover:underline"
            >
              {item.title}
            </a>
          </h3>

          {/* タイトル右側のメタ情報グループ（はてブ数 + スター切替）。
             縮小せず常にタイトル右端に整列する（Req 1.3, 1.4, 1.5）。 */}
          <div
            data-testid="item-detail-meta-group"
            className="flex flex-shrink-0 items-center gap-1"
          >
            {/* はてなブックマーク数（アイコン + 数値、ツールチップで意味を補足） */}
            <span
              data-testid="hatebu-count"
              className="inline-flex items-center gap-1 text-sm text-muted-foreground px-1"
              title={hatebuTitle}
            >
              <Bookmark className="w-4 h-4" aria-hidden="true" />
              {hatebuDisplay}
            </span>

            {/* スター切替コントロール（アイコンのみのトグル、Req 2.1〜2.7 / NFR 2.1, 2.2）。
               size="icon-sm" で 32px の正方形ヒット領域を確保し、
               ghost variant のホバー時 bg-accent と rounded-full で丸い背景強調を提供する。 */}
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              data-testid="star-toggle"
              aria-label={starLabel}
              aria-pressed={item.is_starred}
              title={starLabel}
              className="rounded-full"
              onClick={() => onToggleStar(item.id, !item.is_starred)}
            >
              <Star
                aria-hidden="true"
                className={cn(
                  "w-4 h-4",
                  item.is_starred
                    ? "fill-yellow-400 text-yellow-400"
                    : "text-muted-foreground"
                )}
              />
            </Button>
          </div>
        </div>

        {/* 著者名 ・ 元記事を開く（Req 4.2, 4.3, 4.4, 4.5）。
           著者情報が無い場合は中点を伴わず元記事リンクのみを表示する。 */}
        <p className="flex flex-wrap items-center gap-x-2 text-sm text-muted-foreground">
          {item.author && (
            <>
              <span data-testid="item-detail-author">{item.author}</span>
              <span data-testid="item-detail-author-separator" aria-hidden="true">
                ・
              </span>
            </>
          )}
          <a
            href={item.link}
            target="_blank"
            rel="noopener noreferrer"
            data-testid="original-link"
            className="inline-flex items-center gap-1 text-primary hover:underline"
          >
            元記事を開く
            <ExternalLink className="w-3.5 h-3.5" aria-hidden="true" />
          </a>
        </p>
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
