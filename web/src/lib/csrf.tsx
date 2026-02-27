"use client";

import {
  createContext,
  useContext,
  useEffect,
  useState,
} from "react";
import { API_BASE_URL } from "@/lib/api";

/** CSRFトークンコンテキスト */
const CSRFContext = createContext<string | null | undefined>(undefined);

/**
 * CSRFトークンプロバイダー
 *
 * ページ初回読み込み時に GET /api/csrf-token からCSRFトークンを取得し、
 * React Contextで保持する。mutation系リクエストで使用される。
 */
export function CSRFProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);

  useEffect(() => {
    const fetchToken = async () => {
      try {
        const response = await fetch(`${API_BASE_URL}/api/csrf-token`, {
          credentials: "include",
        });
        if (!response.ok) {
          console.error(
            `CSRFトークンの取得に失敗しました: ${response.status}`
          );
          return;
        }
        const data = await response.json();
        setToken(data.token);
      } catch (error) {
        console.error("CSRFトークンの取得中にエラーが発生しました:", error);
      }
    };

    fetchToken();
  }, []);

  return (
    <CSRFContext.Provider value={token}>{children}</CSRFContext.Provider>
  );
}

/**
 * CSRFトークンを取得するカスタムフック。
 * CSRFProvider内で使用する必要がある。
 * トークン未取得時はnullを返す。
 */
export function useCSRFToken(): string | null {
  const context = useContext(CSRFContext);
  if (context === undefined) {
    throw new Error("useCSRFToken は CSRFProvider 内で使用してください");
  }
  return context;
}
