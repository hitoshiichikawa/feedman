"use client";

import { useState, type FormEvent } from "react";
import { Search, X } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useAppState, useAppDispatch } from "@/contexts/app-state";

/**
 * ヘッダー領域に常設する横断検索バー。
 *
 * 入力欄に検索キーワードを受け取り、Enter / submit で AppState に
 * `SET_SEARCH_QUERY({ scope: 'global' })` を dispatch する。
 *
 * - 入力欄の値はローカル状態（未確定）として保持し、submit で AppState に反映する
 * - 検索中（AppState.isSearching=true かつ scope='global'）はローカル入力を
 *   AppState.searchQuery と同期させ、ユーザーが現在の検索キーワードを再編集できる
 * - 空入力 / 空白のみで submit したときは dispatch せず通常一覧を維持（Req 1.5）
 * - クリアボタン押下時は `CLEAR_SEARCH` を dispatch して検索結果を解除（Req 1.6）
 */
export function HeaderSearchBar() {
  const state = useAppState();
  const dispatch = useAppDispatch();

  // 検索中かつ scope='global' のときはローカル入力に AppState の値を反映する。
  // それ以外（フィード内検索中 / 未検索）はローカル入力を空にする方が UX 上自然。
  const initialLocalQuery =
    state.isSearching && state.searchScope === "global" ? state.searchQuery : "";
  const [localQuery, setLocalQuery] = useState(initialLocalQuery);

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
      scope: "global",
    });
  };

  const handleClear = () => {
    setLocalQuery("");
    // Req 1.6: 検索結果表示を解除し検索実行前の記事一覧表示状態へ戻す
    dispatch({ type: "CLEAR_SEARCH" });
  };

  return (
    <form
      data-testid="header-search-bar"
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
          data-testid="header-search-input"
          type="search"
          value={localQuery}
          onChange={(e) => setLocalQuery(e.target.value)}
          placeholder="記事を検索"
          aria-label="購読中の全フィードを横断検索"
          className="w-64 pl-8 pr-8"
        />
        {localQuery.length > 0 && (
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            data-testid="header-search-clear"
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
