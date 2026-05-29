"use client";

import { useState } from "react";
import { useAppState, useAppDispatch } from "@/contexts/app-state";
import { useFeeds } from "@/hooks/use-feeds";
import { FeedList } from "@/components/feed-list";
import { FeedRegisterDialog } from "@/components/feed-register-dialog";
import { HeaderSearchBar } from "@/components/header-search-bar";
import { ItemList } from "@/components/item-list";
import { FeedPaneHeader } from "@/components/feed-pane-header";
import { CrossFeedItemList } from "@/components/cross-feed-item-list";
import { StarredItemList } from "@/components/starred-item-list";
import { StarredNavItem } from "@/components/starred-nav-item";
import { LogoutButton } from "@/components/logout-button";
import { SearchResults } from "@/components/search-results";
import { SubscriptionSettingsDialog } from "@/components/subscription-settings-dialog";
import { useTheme } from "@/components/theme-provider";
import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Subscription } from "@/types/feed";

/**
 * アプリケーションシェル（2ペインレイアウト）
 *
 * 左ペイン（フィード一覧）と右ペイン（記事一覧 + 記事詳細展開）の
 * 2ペイン分割レイアウトを提供する。
 * AppStateContext を介して選択フィード、展開記事ID、フィルタ状態を
 * 一元管理する。
 */
export function AppShell() {
  const state = useAppState();
  const dispatch = useAppDispatch();

  const { data: feeds, isLoading: isFeedsLoading } = useFeeds();

  /**
   * 設定ダイアログの対象 subscription を保持するローカル state。
   * `null` のときはダイアログを閉じている状態を意味する（AC 1.3 の起動制御）。
   * グローバル AppState には載せず、AppShell ローカル state とする（design.md 代替案 C 却下理由）。
   */
  const [settingsTarget, setSettingsTarget] = useState<Subscription | null>(
    null
  );

  /** フィード選択ハンドラ */
  const handleSelectFeed = (feedId: string) => {
    dispatch({ type: "SELECT_FEED", feedId });
  };

  /** 記事選択ハンドラ（排他的展開） */
  const handleSelectItem = (itemId: string) => {
    dispatch({ type: "EXPAND_ITEM", itemId });
  };

  /** フィード登録完了ハンドラ */
  const handleFeedRegistered = () => {
    // フィード一覧は useFeeds のキャッシュ無効化で自動更新される
  };

  /**
   * 設定起動ハンドラ。FeedList のギアアイコンクリックから対象 subscription を受け取り、
   * 設定ダイアログを開く（AC 1.3）。
   */
  const handleOpenSettings = (feed: Subscription) => {
    setSettingsTarget(feed);
  };

  /**
   * 購読解除成功時のハンドラ。
   *
   * 解除されたフィードが現在右ペインに選択されているフィードと一致する場合のみ、
   * `CLEAR_SELECTED_FEED` を dispatch して右ペインを初期状態に戻す（AC 4.2）。
   * 一致しない場合は右ペインの選択状態を維持する（AC 4.3）。
   * 最後にダイアログ自身を閉じる（`settingsTarget` を null に戻す）。
   *
   * AC 5.3（失敗時に右ペインを触らない）の構造的保証:
   *   本ハンドラは `SubscriptionSettingsDialog` 経由で `SubscriptionSettings` の
   *   mutation `onSuccess` 内でのみ発火する。`useUnsubscribe` の mutation が
   *   エラー（ネットワーク / 5xx 等）で終了した場合は `onSuccess` が呼ばれず、
   *   よって本ハンドラにも到達しないため、右ペインの選択状態は変化しない。
   *   AppShell 側で明示的なエラー分岐を持つ必要はない。
   */
  const handleUnsubscribed = (unsubscribedFeedId: string) => {
    if (unsubscribedFeedId === state.selectedFeedId) {
      dispatch({ type: "CLEAR_SELECTED_FEED" });
    }
    setSettingsTarget(null);
  };

  return (
    <div data-testid="app-shell" className="flex flex-col h-screen">
      {/* ヘッダー */}
      <header className="flex items-center justify-between gap-3 border-b px-4 py-2 flex-shrink-0">
        <h1 className="text-lg font-bold">Feedman</h1>
        {/* Req 1.1: 横断検索バーをヘッダー領域に常設する */}
        <HeaderSearchBar />
        <div className="flex items-center gap-2">
          <ThemeToggle />
          <LogoutButton />
        </div>
      </header>

      {/* メインコンテンツ: 2ペインレイアウト */}
      <div className="flex flex-1 overflow-hidden">
        {/* 左ペイン: フィード一覧 */}
        <aside
          data-testid="left-pane"
          className="w-64 flex-shrink-0 border-r overflow-y-auto"
        >
          {/* お気に入り（横断スター記事一覧）固定ナビ項目（Req 1.1, 1.3）。
              フィード一覧の先頭に常時 1 件表示する。 */}
          <div className="px-2 pt-2">
            <StarredNavItem />
          </div>

          <div className="flex items-center justify-between px-3 py-2 border-b mt-2">
            <span className="text-sm font-medium text-muted-foreground">
              フィード
            </span>
            <FeedRegisterDialog onRegistered={handleFeedRegistered} />
          </div>

          {isFeedsLoading ? (
            <div className="flex items-center justify-center p-4 text-sm text-muted-foreground">
              読み込み中...
            </div>
          ) : (
            <FeedList
              feeds={feeds ?? []}
              selectedFeedId={state.selectedFeedId}
              onSelectFeed={handleSelectFeed}
              onOpenSettings={handleOpenSettings}
              viewMode={state.viewMode}
              onSelectAllNewItems={() =>
                dispatch({ type: "SELECT_ALL_NEW_ITEMS" })
              }
            />
          )}
        </aside>

        {/* 右ペイン: 記事一覧 + 記事詳細（検索モード時は SearchResults に切り替わり、通常時は selectedView で切替 / Req 1.3 / 2.x / 4.7） */}
        <main data-testid="right-pane" className="flex-1 overflow-hidden">
          <div className="flex flex-col h-full">
            {/* 右ペイン分岐の優先順位（Issue #145 / design.md「`app-shell.tsx`（修正）」節）:
                isSearching > selectedView==='starred' > viewMode==='cross-feed' > 既存 ItemList
                viewMode==='cross-feed' は SELECT_ALL_NEW_ITEMS によって selectedView='feed' に
                倒されているため、本分岐は selectedView==='feed' 文脈で評価される。

                Issue #145 追加: isSearching && searchScope === 'feed' && searchFeedId !== null
                の枝で <FeedPaneHeader mode="search-feed" ...> を <SearchResults /> の上に挿入し、
                フィード内検索結果表示中もフィード内検索バーを画面に残す（Req 1.1）。
                isSearching && searchScope === 'global' は従来どおり <SearchResults /> のみ（Req 2.1）。
                selectedFeedId !== null の通常一覧枝では <FeedPaneHeader mode="normal" ...> を
                <ItemList /> の上に挿入し、従来 ItemList 内部にあったフィードヘッダ要素群を
                FeedPaneHeader に移譲した責務に対応する（Req 3.4 / NFR 1.2）。 */}
            {state.isSearching ? (
              state.searchScope === "feed" && state.searchFeedId !== null ? (
                <>
                  <FeedPaneHeader
                    mode="search-feed"
                    feedId={state.searchFeedId}
                  />
                  <SearchResults />
                </>
              ) : (
                <SearchResults />
              )
            ) : state.selectedView === "starred" ? (
              <StarredItemList />
            ) : state.viewMode === "cross-feed" ? (
              <CrossFeedItemList />
            ) : state.selectedFeedId !== null ? (
              <>
                <FeedPaneHeader
                  mode="normal"
                  feedId={state.selectedFeedId}
                  filter={state.filter}
                  onFilterChange={(filter) =>
                    dispatch({ type: "SET_FILTER", filter })
                  }
                />
                <ItemList
                  feedId={state.selectedFeedId}
                  onSelectItem={handleSelectItem}
                  expandedItemId={state.expandedItemId}
                  filter={state.filter}
                />
              </>
            ) : (
              <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
                フィードを選択してください
              </div>
            )}
            {/* 記事詳細は ItemList / StarredItemList / SearchResults 内の展開で表示する。
                展開記事の詳細表示はItemDetailコンポーネントとして
                各リスト配下で統合されるため、ここでは別途表示しない */}
          </div>
        </main>
      </div>

      {/* 設定ダイアログ（2 ペインの外、AppShell 直下に配置）。
          radix-ui Dialog は Portal で body 直下に render するため位置は副次的だが、
          コンポーネントツリー上は 2 ペインの外側に置くことで責務分離を明示する。 */}
      <SubscriptionSettingsDialog
        open={settingsTarget !== null}
        subscription={settingsTarget}
        onOpenChange={(open) => {
          if (!open) {
            setSettingsTarget(null);
          }
        }}
        onUnsubscribed={handleUnsubscribed}
      />
    </div>
  );
}

/**
 * テーマ切替ボタン
 */
function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => setTheme(theme === "light" ? "dark" : "light")}
      data-testid="theme-toggle"
    >
      {theme === "light" ? (
        <Moon className="w-4 h-4" />
      ) : (
        <Sun className="w-4 h-4" />
      )}
    </Button>
  );
}
