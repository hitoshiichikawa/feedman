/**
 * 同一オリジン proxy 用の rewrites ルール生成と内部 API 接続先設定の検証ロジック。
 *
 * ブラウザの `/api/*`・`/auth/*` リクエストを Next.js の rewrites（server-side proxy）で
 * 内部 API（`API_INTERNAL_URL`）へ転送するための純粋ロジックを提供する。
 * 環境変数の読取と rewrites ルール生成を分離し、Vitest から単体テスト可能にする。
 */

import type { Rewrite } from "next/dist/lib/load-custom-routes";

/** 環境変数名（fail-fast / rewrites 双方で参照する単一定義） */
export const API_INTERNAL_URL_ENV = "API_INTERNAL_URL";

/**
 * `API_INTERNAL_URL` を読み取り、未設定/空ならエラーを投げる（fail-fast 用）。
 * 末尾スラッシュを除去した base を返す。
 *
 * @param env - 参照する環境変数オブジェクト（既定: `process.env`）
 * @returns 末尾スラッシュを除去した内部 API 接続先 base
 * @throws 未設定または空文字（空白のみを含む）のとき、変数名を含むエラー
 */
export function resolveApiInternalUrl(
  env: NodeJS.ProcessEnv = process.env
): string {
  const raw = env[API_INTERNAL_URL_ENV];

  if (raw === undefined || raw.trim() === "") {
    throw new Error(
      `FATAL: ${API_INTERNAL_URL_ENV} is not set. ` +
        `内部 API 接続先（${API_INTERNAL_URL_ENV}）を実行時環境変数として設定してください。`
    );
  }

  return raw.trim().replace(/\/+$/, "");
}

/**
 * 与えられた api 接続先 base から rewrites ルール配列を生成する純粋関数。
 * `/api/:path*` と `/auth/:path*` をプレフィックス保持で転送する（strip しない）。
 *
 * @param apiInternalUrl - 正規化済み（末尾スラッシュ除去済み）の内部 API 接続先 base
 * @returns `/api/:path*`・`/auth/:path*` の 2 ルール
 */
export function buildRewrites(apiInternalUrl: string): Rewrite[] {
  const base = apiInternalUrl.replace(/\/+$/, "");

  return [
    { source: "/api/:path*", destination: `${base}/api/:path*` },
    { source: "/auth/:path*", destination: `${base}/auth/:path*` },
  ];
}
