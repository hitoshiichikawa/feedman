"use client";

import {
  createContext,
  useContext,
  useReducer,
  type Dispatch,
  type ReactNode,
} from "react";
import type { ItemFilter } from "@/types/item";

// --- State ---

/** アプリケーションUIステート */
export interface AppState {
  /** 現在選択中のフィードID（null = 未選択） */
  selectedFeedId: string | null;
  /** 現在展開中の記事ID（null = 未展開） */
  expandedItemId: string | null;
  /** 現在のフィルタ */
  filter: ItemFilter;
}

/** 初期状態 */
const initialState: AppState = {
  selectedFeedId: null,
  expandedItemId: null,
  filter: "all",
};

// --- Actions ---

/** フィード選択アクション: 展開記事IDとフィルタをリセットする */
type SelectFeedAction = { type: "SELECT_FEED"; feedId: string };

/** 記事展開アクション: 同じIDなら閉じる（トグル）、異なるIDなら排他的に展開 */
type ExpandItemAction = { type: "EXPAND_ITEM"; itemId: string };

/** フィルタ変更アクション */
type SetFilterAction = { type: "SET_FILTER"; filter: ItemFilter };

/** 全アクションのユニオン型 */
export type AppAction = SelectFeedAction | ExpandItemAction | SetFilterAction;

// --- Reducer ---

/** アプリケーションステートのリデューサー */
function appReducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case "SELECT_FEED":
      return {
        ...state,
        selectedFeedId: action.feedId,
        expandedItemId: null,
        filter: "all",
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
