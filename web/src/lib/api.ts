/**
 * API通信レイヤー
 *
 * credentials: "include" により Cookie を自動送信する。
 * CSRF保護はSameSite=Lax Cookie + CORSポリシーで実現する。
 *
 * NEXT_PUBLIC_API_URL 環境変数でAPIサーバーのベースURLを指定する。
 * 未設定の場合は同一オリジン（相対パス）にフォールバックする。
 */

/**
 * APIサーバーのベースURL。
 * 末尾スラッシュは除去して保持する。
 */
export const API_BASE_URL = (
  process.env.NEXT_PUBLIC_API_URL || ""
).replace(/\/+$/, "");

/** APIエラークラス */
export class ApiError extends Error {
  status: number;
  body: unknown;

  constructor(status: number, body: unknown) {
    super(`API Error: ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

/**
 * APIリクエストを実行する共通関数
 */
async function request<T>(
  method: string,
  url: string,
  body?: unknown
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  const options: RequestInit = {
    method,
    headers,
    credentials: "include",
  };

  if (body !== undefined) {
    options.body = JSON.stringify(body);
  }

  const fullUrl = `${API_BASE_URL}${url}`;
  const response = await fetch(fullUrl, options);

  if (!response.ok) {
    let errorBody: unknown;
    try {
      errorBody = await response.json();
    } catch {
      errorBody = null;
    }
    throw new ApiError(response.status, errorBody);
  }

  return response.json();
}

/**
 * APIクライアントのインターフェース
 */
export interface ApiClient {
  get: <T = unknown>(url: string) => Promise<T>;
  post: <T = unknown>(url: string, body?: unknown) => Promise<T>;
  put: <T = unknown>(url: string, body?: unknown) => Promise<T>;
  patch: <T = unknown>(url: string, body?: unknown) => Promise<T>;
  delete: <T = unknown>(url: string) => Promise<T>;
}

/**
 * APIクライアントを生成する。
 */
export function createApiClient(): ApiClient {
  return {
    get: <T = unknown>(url: string) => request<T>("GET", url),
    post: <T = unknown>(url: string, body?: unknown) =>
      request<T>("POST", url, body),
    put: <T = unknown>(url: string, body?: unknown) =>
      request<T>("PUT", url, body),
    patch: <T = unknown>(url: string, body?: unknown) =>
      request<T>("PATCH", url, body),
    delete: <T = unknown>(url: string) => request<T>("DELETE", url),
  };
}
