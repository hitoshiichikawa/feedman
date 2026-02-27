"use client";

import { useAppState, useAppDispatch } from "@/contexts/app-state";
import { useFeeds } from "@/hooks/use-feeds";
import { FeedList } from "@/components/feed-list";
import { FeedRegisterDialog } from "@/components/feed-register-dialog";
import { ItemList } from "@/components/item-list";
import { LogoutButton } from "@/components/logout-button";
import { useTheme } from "@/components/theme-provider";
import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";

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

  return (
    <div data-testid="app-shell" className="flex flex-col h-screen">
      {/* ヘッダー */}
      <header className="flex items-center justify-between border-b px-4 py-2 flex-shrink-0">
        <h1 className="text-lg font-bold">Feedman</h1>
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
          <div className="flex items-center justify-between px-3 py-2 border-b">
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
            />
          )}
        </aside>

        {/* 右ペイン: 記事一覧 + 記事詳細 */}
        <main data-testid="right-pane" className="flex-1 overflow-hidden">
          <div className="flex flex-col h-full">
            <ItemList
              feedId={state.selectedFeedId}
              onSelectItem={handleSelectItem}
              expandedItemId={state.expandedItemId}
            />
            {/* 記事詳細は ItemList 内の展開で表示する。
                展開記事の詳細表示はItemDetailコンポーネントとして
                ItemList配下で統合されるため、ここでは別途表示しない */}
          </div>
        </main>
      </div>
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
