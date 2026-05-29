"use client";

import {
  createContext,
  useContext,
  useReducer,
  type Dispatch,
  type ReactNode,
} from "react";
import type { ItemFilter } from "@/types/item";

// --- Search scope ---

/** 検索スコープ: 'global' = 横断検索 / 'feed' = フィード内検索 */
export type SearchScope = "global" | "feed";

// --- State ---

/**
 * 右ペインの表示モード。
 * - `"feed"`: 単一フィード記事一覧（selectedFeedId が選択中フィードを示す）
 * - `"starred"`: フィード横断のお気に入り記事一覧
 */
export type SelectedView = "feed" | "starred";

/** アプリケーションUIステート */
export interface AppState {
  /** 現在の右ペイン表示モード（"feed" or "starred"） */
  selectedView: SelectedView;
  /** 現在選択中のフィードID（null = 未選択 / "starred" 選択時も null） */
  selectedFeedId: string | null;
  /** 現在展開中の記事ID（null = 未展開） */
  expandedItemId: string | null;
  /** 現在のフィルタ */
  filter: ItemFilter;
  /** 検索キーワード（空文字 = 検索オフ） */
  searchQuery: string;
  /** 検索モード中か否か（searchQuery !== '' と等価のキャッシュ） */
  isSearching: boolean;
  /** 検索スコープ（'global' = 横断検索 / 'feed' = フィード内検索） */
  searchScope: SearchScope;
  /** フィード内検索の対象フィードID（searchScope === 'feed' のときのみ非 null） */
  searchFeedId: string | null;
}

/** 初期状態 */
const initialState: AppState = {
  selectedView: "feed",
  selectedFeedId: null,
  expandedItemId: null,
  filter: "all",
  searchQuery: "",
  isSearching: false,
  searchScope: "global",
  searchFeedId: null,
};

// --- Actions ---

/** フィード選択アクション: 展開記事ID、フィルタ、および検索状態をリセットし、selectedView を "feed" に戻す */
type SelectFeedAction = { type: "SELECT_FEED"; feedId: string };

/**
 * お気に入り選択アクション:
 * selectedView を "starred" に切り替え、selectedFeedId / expandedItemId をリセットし、
 * filter を "all" に戻す。
 */
type SelectStarredAction = { type: "SELECT_STARRED" };

/** 記事展開アクション: 同じIDなら閉じる（トグル）、異なるIDなら排他的に展開 */
type ExpandItemAction = { type: "EXPAND_ITEM"; itemId: string };

/** フィルタ変更アクション */
type SetFilterAction = { type: "SET_FILTER"; filter: ItemFilter };

/**
 * 検索キーワード設定アクション。
 *
 * - scope='global' のとき: 横断検索を開始する（feedId は無視され searchFeedId は null になる）
 * - scope='feed' のとき: フィード内検索を開始する（feedId 指定値、未指定なら null）
 */
type SetSearchQueryAction = {
  type: "SET_SEARCH_QUERY";
  query: string;
  scope: SearchScope;
  feedId?: string | null;
};

/** 検索解除アクション: 検索状態のみリセットし、selectedFeedId と filter は保持する */
type ClearSearchAction = { type: "CLEAR_SEARCH" };

/**
 * 選択フィードクリアアクション:
 * `selectedFeedId` を null に戻し、`expandedItemId` と `filter` を初期値にリセットする。
 * 購読解除完了時に「選択中フィードが解除対象だった」場合に AppShell から dispatch される
 * （要件 4.2）。`SELECT_FEED` と同じ副作用パターン（展開記事・フィルタ・検索状態リセット）
 * を踏襲しつつ、`selectedFeedId` のみ null に倒す点が異なる。
 */
type ClearSelectedFeedAction = { type: "CLEAR_SELECTED_FEED" };

/** 全アクションのユニオン型 */
export type AppAction =
  | SelectFeedAction
  | SelectStarredAction
  | ExpandItemAction
  | SetFilterAction
  | SetSearchQueryAction
  | ClearSearchAction
  | ClearSelectedFeedAction;

// --- Reducer ---

/** アプリケーションステートのリデューサー */
function appReducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case "SELECT_FEED":
      return {
        ...state,
        selectedView: "feed",
        selectedFeedId: action.feedId,
        expandedItemId: null,
        filter: "all",
        searchQuery: "",
        isSearching: false,
        searchScope: "global",
        searchFeedId: null,
      };
    case "SELECT_STARRED":
      return {
        ...state,
        selectedView: "starred",
        selectedFeedId: null,
        expandedItemId: null,
        filter: "all",
        searchQuery: "",
        isSearching: false,
        searchScope: "global",
        searchFeedId: null,
      };
    case "EXPAND_ITEM":
      return {
        ...state,
        expandedItemId:
          state.expandedItemId === action.itemId ? null : action.itemId,
      };
    case "SET_FILTER":
      return {
        ...state,
        filter: action.filter,
      };
    case "SET_SEARCH_QUERY":
      return {
        ...state,
        searchQuery: action.query,
        isSearching: true,
        searchScope: action.scope,
        searchFeedId:
          action.scope === "feed" ? (action.feedId ?? null) : null,
      };
    case "CLEAR_SEARCH":
      return {
        ...state,
        searchQuery: "",
        isSearching: false,
        searchScope: "global",
        searchFeedId: null,
      };
    case "CLEAR_SELECTED_FEED":
      return {
        ...state,
        selectedView: "feed",
        selectedFeedId: null,
        expandedItemId: null,
        filter: "all",
        searchQuery: "",
        isSearching: false,
        searchScope: "global",
        searchFeedId: null,
      };
    default:
      return state;
  }
}

// --- Context ---

const AppStateContext = createContext<AppState | null>(null);
const AppDispatchContext = createContext<Dispatch<AppAction> | null>(null);

// --- Provider ---

/**
 * アプリケーションUIステートプロバイダー
 *
 * 選択フィード、展開記事ID、フィルタ状態を
 * React Context + useReducer で一元管理する。
 */
export function AppStateProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(appReducer, initialState);

  return (
    <AppStateContext.Provider value={state}>
      <AppDispatchContext.Provider value={dispatch}>
        {children}
      </AppDispatchContext.Provider>
    </AppStateContext.Provider>
  );
}

// --- Hooks ---

/**
 * アプリケーションステートを取得するフック。
 * AppStateProvider内で使用する必要がある。
 */
export function useAppState(): AppState {
  const context = useContext(AppStateContext);
  if (context === null) {
    throw new Error("useAppState は AppStateProvider 内で使用してください");
  }
  return context;
}

/**
 * アプリケーションステートのdispatch関数を取得するフック。
 * AppStateProvider内で使用する必要がある。
 */
export function useAppDispatch(): Dispatch<AppAction> {
  const context = useContext(AppDispatchContext);
  if (context === null) {
    throw new Error("useAppDispatch は AppStateProvider 内で使用してください");
  }
  return context;
}
