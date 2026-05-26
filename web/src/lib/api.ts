/**
 * API通信レイヤー
 *
 * credentials: "include" により Cookie を自動送信する。
 * CSRF保護はSameSite=Lax Cookie + CORSポリシーで実現する。
 *
 * API は常に同一オリジンの相対パス経由で呼び出す（単一オリジン構成）。
 * NEXT_PUBLIC_API_URL は参照しない（build-once / ビルド時 URL 焼き込みの廃止のため）。
 * 内部 API への転送は Next.js の rewrites（実行時設定 API_INTERNAL_URL）が担う。
 */

/**
 * API のベース URL。常に空文字（同一オリジン相対パス）。
 * NEXT_PUBLIC_API_URL は参照しない（build-once / 単一オリジン化のため）。
 */
export const API_BASE_URL = "";

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
 *
 * 通常の利用側はモジュールレベルの共有インスタンス `apiClient` を参照すること。
 * 本関数はテスト時のモック差し替えや、独立したインスタンスが必要な場合のために温存している。
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

/**
 * モジュールレベルの共有 API クライアントインスタンス。
 *
 * API クライアントはステートレス（`request` をラップするだけで固有状態を持たない）であるため、
 * モジュール初期化時に一度だけ生成し全消費者で共有する。各フック・コンポーネントは
 * レンダリングごとに `createApiClient()` を呼ばず、本インスタンスを参照すること。
 */
export const apiClient: ApiClient = createApiClient();
