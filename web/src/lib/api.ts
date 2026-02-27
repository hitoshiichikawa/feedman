/**
 * API通信レイヤー
 *
 * mutation系リクエスト（POST/PUT/PATCH/DELETE）に
 * X-CSRF-Tokenヘッダーを自動付与するAPIクライアント。
 * credentials: "include" により Cookie を自動送信する。
 */

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

/** CSRFトークンを返すゲッター関数の型 */
type TokenGetter = () => string | null;

/** mutation系メソッド（CSRFトークンが必要） */
const MUTATION_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);

/**
 * APIリクエストを実行する共通関数
 */
async function request<T>(
  getToken: TokenGetter,
  method: string,
  url: string,
  body?: unknown
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  // mutation系リクエストにはCSRFトークンを付与
  if (MUTATION_METHODS.has(method)) {
    const token = getToken();
    if (token) {
      headers["X-CSRF-Token"] = token;
    }
  }

  const options: RequestInit = {
    method,
    headers,
    credentials: "include",
  };

  if (body !== undefined) {
    options.body = JSON.stringify(body);
  }

  const response = await fetch(url, options);

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
 * CSRFトークンゲッターを注入してAPIクライアントを生成する。
 * CSRFProvider の useCSRFToken と組み合わせて使用する。
 */
export function createApiClient(getToken: TokenGetter): ApiClient {
  return {
    get: <T = unknown>(url: string) => request<T>(getToken, "GET", url),
    post: <T = unknown>(url: string, body?: unknown) =>
      request<T>(getToken, "POST", url, body),
    put: <T = unknown>(url: string, body?: unknown) =>
      request<T>(getToken, "PUT", url, body),
    patch: <T = unknown>(url: string, body?: unknown) =>
      request<T>(getToken, "PATCH", url, body),
    delete: <T = unknown>(url: string) => request<T>(getToken, "DELETE", url),
  };
}
