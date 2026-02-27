"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";

/** テーマの型定義 */
export type Theme = "light" | "dark";

/** localStorageのキー */
const STORAGE_KEY = "feedman-theme";

/** デフォルトテーマ */
const DEFAULT_THEME: Theme = "light";

/** テーマコンテキストの値の型 */
interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

/**
 * localStorageから保存済みテーマを読み込む。
 * 不正な値やlocalStorage未対応環境ではデフォルト（ライト）を返す。
 */
function getStoredTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark") {
      return stored;
    }
  } catch {
    // localStorageにアクセスできない場合はデフォルトを返す
  }
  return DEFAULT_THEME;
}

/**
 * html要素のdarkクラスを更新する
 */
function applyThemeToDocument(theme: Theme): void {
  if (theme === "dark") {
    document.documentElement.classList.add("dark");
  } else {
    document.documentElement.classList.remove("dark");
  }
}

/**
 * テーマ切替プロバイダー
 *
 * ライト/ダークテーマの切替を管理し、Tailwindのdark:クラスを使用して
 * テーマを適用する。設定はlocalStorageに永続化される。
 * デフォルトはライトテーマ。
 */
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(getStoredTheme);

  // テーマ変更時にhtml要素とlocalStorageを更新
  useEffect(() => {
    applyThemeToDocument(theme);
  }, [theme]);

  const setTheme = useCallback((newTheme: Theme) => {
    setThemeState(newTheme);
    try {
      localStorage.setItem(STORAGE_KEY, newTheme);
    } catch {
      // localStorageにアクセスできない場合は無視
    }
  }, []);

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

/**
 * テーマコンテキストを使用するカスタムフック。
 * ThemeProvider内で使用する必要がある。
 */
export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext);
  if (context === null) {
    throw new Error("useTheme は ThemeProvider 内で使用してください");
  }
  return context;
}
