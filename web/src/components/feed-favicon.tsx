"use client";

import { Rss } from "lucide-react";
import { useState } from "react";

/** FeedFavicon のプロパティ */
export interface FeedFaviconProps {
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
 *
 * 本コンポーネントは元 `feed-list.tsx` 内 private function だった同名関数を
 * 横断新着一覧（cross-feed timeline）でも再利用できるよう挙動不変で抽出したもの
 * （Issue #121 task 6 / design.md "FeedFavicon" 節）。
 */
export function FeedFavicon({ feedId, faviconURL, feedTitle }: FeedFaviconProps) {
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
