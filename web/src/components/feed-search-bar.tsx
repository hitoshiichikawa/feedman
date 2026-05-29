"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Search, X } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useAppState, useAppDispatch } from "@/contexts/app-state";

/**
 * フィード内検索バー。
 *
 * フィードを開いた際の記事一覧上部に配置し、当該フィード内のみを対象とした検索を提供する。
 * 入力欄に検索キーワードを受け取り、Enter / submit で AppState に
 * `SET_SEARCH_QUERY({ scope: 'feed', feedId })` を dispatch する。
 *
 * 表示制御:
 * - フィード未選択時（`selectedFeedId === null`）は `null` を返して描画しない（NFR 2.3）
 *
 * 動作:
 * - 入力欄の値はローカル状態として保持し、submit で AppState に反映する
 * - 検索中（AppState.isSearching=true かつ scope='feed' かつ同一フィード）はローカル入力を
 *   AppState.searchQuery と同期させ、現在のキーワードを再編集できる
 * - 空入力 / 空白のみ submit は dispatch せず通常一覧を維持（Req 1.5）
 * - クリアボタン押下時は `CLEAR_SEARCH` を dispatch（Req 1.6）
 */
export function FeedSearchBar() {
  const state = useAppState();
  const dispatch = useAppDispatch();

  // NFR 2.3: フィード未選択時は描画しない（早期 return が AppState を含む全 hook 評価後である
  // ことに注意。React hooks は条件付き呼び出しを許さないため、useState 等は return より前で
  // 呼んでおく必要がある）
  const selectedFeedId = state.selectedFeedId;

  // フィード内検索中かつ対象フィードが一致するときのみローカル入力に AppState を反映
  const initialLocalQuery =
    state.isSearching &&
    state.searchScope === "feed" &&
    state.searchFeedId === selectedFeedId
      ? state.searchQuery
      : "";
  const [localQuery, setLocalQuery] = useState(initialLocalQuery);

  // Req 1.2 一般化: 検索結果表示中に AppState の searchQuery が外部変更（例: ProgrammaticallyDispatched
  // からの再検索キーワード変更）されたとき、localQuery にも反映して入力欄表示を同期する。
  // useState の直後・early return の前に配置することで、React の hooks 順序制約を満たす。
  // 注: setLocalQuery は安定参照のため deps には含めない（react-hooks/exhaustive-deps は満たす）。
  useEffect(() => {
    if (
      state.isSearching &&
      state.searchScope === "feed" &&
      state.searchFeedId === selectedFeedId
    ) {
      setLocalQuery(state.searchQuery);
    }
  }, [
    state.isSearching,
    state.searchScope,
    state.searchFeedId,
    state.searchQuery,
    selectedFeedId,
  ]);

  if (selectedFeedId === null) {
    return null;
  }

  const handleSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const trimmed = localQuery.trim();
    if (trimmed.length === 0) {
      // Req 1.5: 空入力時は検索を実行せず通常の記事一覧表示を維持する
      return;
    }
    dispatch({
      type: "SET_SEARCH_QUERY",
      query: trimmed,
      scope: "feed",
      feedId: selectedFeedId,
    });
  };

  const handleClear = () => {
    setLocalQuery("");
    // Req 1.6: 検索結果表示を解除し検索実行前の記事一覧表示状態へ戻す
    dispatch({ type: "CLEAR_SEARCH" });
  };

  return (
    <form
      data-testid="feed-search-bar"
      role="search"
      onSubmit={handleSubmit}
      className="flex items-center gap-1"
    >
      <div className="relative">
        <Search
          aria-hidden="true"
          className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground"
        />
        <Input
          data-testid="feed-search-input"
          type="search"
          value={localQuery}
          onChange={(e) => setLocalQuery(e.target.value)}
          placeholder="このフィード内を検索"
          aria-label="現在のフィード内を検索"
          className="w-56 pl-8 pr-8"
        />
        {localQuery.length > 0 && (
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            data-testid="feed-search-clear"
            aria-label="検索をクリア"
            onClick={handleClear}
            className="absolute right-1 top-1/2 -translate-y-1/2 rounded-full"
          >
            <X aria-hidden="true" className="w-3 h-3" />
          </Button>
        )}
      </div>
    </form>
  );
}
