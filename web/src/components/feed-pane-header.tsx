"use client";

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FeedSearchBar } from "@/components/feed-search-bar";
import { ManualRefreshBanner } from "@/components/manual-refresh-banner";
import { ManualRefreshButton } from "@/components/item-list";
import { useFeeds } from "@/hooks/use-feeds";
import { useManualRefresh } from "@/hooks/use-manual-refresh";
import type { ItemFilter } from "@/types/item";

/**
 * `FeedPaneHeader` の表示モード。
 * - `"normal"`: 通常の記事一覧表示（FilterTabs + FeedSearchBar + ManualRefreshButton の 3 要素を描画）
 * - `"search-feed"`: フィード内検索結果表示中（FeedSearchBar のみ描画。FilterTabs / ManualRefresh は撤去）
 */
export type FeedPaneHeaderMode = "normal" | "search-feed";

/** FeedPaneHeader のプロパティ */
export interface FeedPaneHeaderProps {
  /** 表示モード（"normal" = 通常一覧、"search-feed" = フィード内検索結果表示中） */
  mode: FeedPaneHeaderMode;
  /**
   * 対象フィードID。
   * `"normal"` / `"search-feed"` のいずれでも非 null を期待する
   * （呼び出し側で `selectedFeedId` / `searchFeedId` を解決して渡す）。
   */
  feedId: string;
  /** `"normal"` モードでのフィルタ値（`"search-feed"` モードでは未使用） */
  filter?: ItemFilter;
  /** `"normal"` モードでのフィルタ変更ハンドラ（`"search-feed"` モードでは未使用） */
  onFilterChange?: (filter: ItemFilter) => void;
}

/**
 * フィードペイン上部のヘッダ領域レイアウトコンポーネント。
 *
 * 通常一覧と検索結果表示の双方で「フィードを開いている文脈の上部ヘッダ」という同一意味論を
 * 共有しつつ、内部要素の出し分けを行う（design.md Components and Interfaces / Issue #145）。
 *
 * - `mode === "normal"`: `FilterTabs` + `<FeedSearchBar />` + `<ManualRefreshButton />` の 3 要素を、
 *   従来 `item-list.tsx` のフィードヘッダ DOM 構造と同等のレイアウトで描画する（Req 3.4）。
 *   `ManualRefreshBanner` もヘッダ直下に描画する。
 * - `mode === "search-feed"`: 同じ外側コンテナ div を用い、内部は `<FeedSearchBar />` のみを
 *   描画する（暫定方針。design.md 確認事項 1, 2 を参照）。
 *
 * `<FeedSearchBar />` は両モードで同一の React tree 位置（同じ親 div の同じ index 兄弟要素）に
 * 配置するため、mode 切替時に unmount されず DOM 上で同一要素として保持される（Req 1.1）。
 */
export function FeedPaneHeader({
  mode,
  feedId,
  filter,
  onFilterChange,
}: FeedPaneHeaderProps) {
  // 手動更新ボタン用に subscription.id を解決する（旧 item-list.tsx L62-67 の責務を移譲）。
  const { data: feeds } = useFeeds();
  const subscriptionId = Array.isArray(feeds)
    ? feeds.find((f) => f.feed_id === feedId)?.id ?? null
    : null;
  const manualRefresh = useManualRefresh(feedId);

  const isNormal = mode === "normal";

  // React の reconciliation で `<FeedSearchBar />` を mode 切替時にも同一インスタンスとして
  // 保持する（Req 1.1 の構造的担保）。兄弟要素の有無で position が変わると key 無しでは remount
  // されうるため、左スロット（FilterTabs 用）と右スロット（FeedSearchBar + ManualRefreshButton 用）の
  // ラッパ div を常に同じ位置・同じ key で描画し、中身だけ条件分岐する。`<FeedSearchBar />` は
  // 右スロット div 内の常に最初の子として固定される。
  return (
    <>
      <div className="flex flex-shrink-0 flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
        <div key="feed-pane-header-left">
          {isNormal ? (
            <Tabs
              value={filter}
              onValueChange={(value) => onFilterChange?.(value as ItemFilter)}
            >
              <TabsList>
                <TabsTrigger value="all">全て</TabsTrigger>
                <TabsTrigger value="unread">未読</TabsTrigger>
                <TabsTrigger value="starred">スター</TabsTrigger>
              </TabsList>
            </Tabs>
          ) : null}
        </div>
        <div key="feed-pane-header-right" className="flex items-center gap-2">
          <FeedSearchBar />
          {isNormal && subscriptionId !== null ? (
            <ManualRefreshButton
              subscriptionId={subscriptionId}
              isPending={manualRefresh.isPending}
              onClick={() => manualRefresh.mutate(subscriptionId)}
            />
          ) : null}
        </div>
      </div>

      {/* 手動更新エラー表示（normal モードのみ。検索結果表示中は手動更新自体を撤去するため非表示） */}
      {isNormal && <ManualRefreshBanner error={manualRefresh.error ?? null} />}
    </>
  );
}
