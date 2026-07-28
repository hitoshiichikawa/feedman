"use client";

import { useQuery } from "@tanstack/react-query";
import { apiClient, ApiError } from "@/lib/api";
import { isPasskeyBrowserSupported } from "@/lib/passkey-capability";
import type { CapabilityResponse } from "@/types/passkey";

/**
 * `usePasskeyCapability` の返り値。
 *
 * - `isLoading`: サーバ側 capability の初回問い合わせが完了していない間 true
 * - `available`: サーバ側 capability + ブラウザ WebAuthn 対応の AND
 */
export interface PasskeyCapability {
  isLoading: boolean;
  available: boolean;
}

/**
 * サーバとブラウザ双方のパスキー対応可否を合成した boolean を提供するフック。
 *
 * サーバ側 `GET /api/passkey/capability` は fail-closed で、ハンドラ未登録
 * （`WEBAUTHN_RP_ID` 等未設定 / task 3）のとき 404 を返す。Web はこれを
 * 「サーバがパスキー機能を提供していない」と解釈し `available: false` に縮退する。
 *
 * queryFn は 200 成功時に server=true、それ以外（`ApiError` 4xx/5xx / fetch reject の
 * `TypeError` 等）は catch して server=false を返す形にする。これにより query 自体は
 * 常に success 扱いとなり、React Query 側で不要な isError 分岐が発生しない
 * （Requirement 5.2 / 5.3: サーバ非提供時も Google 単体構成へ静かに縮退する）。
 *
 * @remarks
 * - `retry: false` — 404 の retry は無意味。ネットワークエラーも retry しない
 * - `staleTime: Infinity` / `gcTime: Infinity` — capability は session を跨いで安定
 *   （env で決まる不変値。同一 tab 内で再問い合わせしない）
 * - NFR 1.4: `apiClient` は同一オリジン相対パス（`API_BASE_URL=""`）を通す
 */
export function usePasskeyCapability(): PasskeyCapability {
  const query = useQuery<boolean>({
    queryKey: ["passkey", "capability"],
    queryFn: async () => {
      try {
        await apiClient.get<CapabilityResponse>("/api/passkey/capability");
        return true;
      } catch (err) {
        // 404 (未登録 = 未提供構成) / ネットワーク reject を server=false に集約。
        // ApiError と TypeError（fetch 失敗）以外の予期しない例外もサーバ非提供として
        // 扱い、Google 単体構成へ縮退させる（NFR 2.1: 縮退時挙動の維持を優先）。
        if (err instanceof ApiError || err instanceof TypeError) {
          return false;
        }
        return false;
      }
    },
    retry: false,
    staleTime: Infinity,
    gcTime: Infinity,
  });

  const serverAvailable = query.data ?? false;
  const browserSupported = isPasskeyBrowserSupported();
  return {
    isLoading: query.isLoading,
    available: serverAvailable && browserSupported,
  };
}
