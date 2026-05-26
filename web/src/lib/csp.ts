/**
 * Content-Security-Policy（CSP）ヘッダー値の生成ロジック。
 *
 * `next.config.ts` の `headers()` から静的 CSP を全ルートへ付与するための純粋ロジックを提供する。
 * 環境（dev / production）を引数で受け取る純粋関数として実装し、`process.env.NODE_ENV` への
 * 直接依存を排して Vitest から単体テスト可能にする（`rewrites.ts` の env 注入パターンに準拠）。
 *
 * 方針: 「できるだけ厳しく、必要な許可のみ追加する」（最小許可）。`headers()` による静的 CSP では
 * per-request nonce が使えない（nonce は middleware を要し本 Issue のスコープ外）ため、
 * Next.js App Router のブートストラップ inline script / Tailwind・next/font の inline style 向けに
 * `'unsafe-inline'` を許容する。dev モードのみ HMR / turbopack 用に `'unsafe-eval'` と websocket を許可する。
 */

/** CSP ヘッダー名（HTTP ヘッダーの正準名）。 */
export const CSP_HEADER_NAME = "Content-Security-Policy";

/**
 * CSP 生成時に参照する環境を表す引数型。
 *
 * `NODE_ENV` のみを参照する。`process.env` をそのまま渡すこともできる
 * （余剰キーは無視される）。
 */
export interface CspEnv {
  /** Node 実行環境。`"production"` のとき本番向けの厳格ポリシーを生成する。 */
  NODE_ENV?: string;
}

/**
 * 与えられた環境に応じた CSP ディレクティブのマップを構築する純粋関数。
 *
 * production 以外（dev / test）では HMR / turbopack のために `script-src` へ `'unsafe-eval'` を、
 * `connect-src` へ websocket（`ws:` / `wss:`）を追加する。production ではこれらを含めない。
 *
 * @param env - 参照する環境（既定: `process.env`）。`NODE_ENV` のみ参照する
 * @returns ディレクティブ名 → 値配列のマップ（挿入順を保持する）
 */
export function buildCspDirectives(
  env: CspEnv = process.env
): Map<string, string[]> {
  const isProduction = env.NODE_ENV === "production";

  // script-src: Next.js App Router のブートストラップ inline script 用に 'unsafe-inline' を許容。
  // dev のみ HMR / turbopack の eval 実行のため 'unsafe-eval' を追加（本番では付けない）。
  const scriptSrc = ["'self'", "'unsafe-inline'"];
  if (!isProduction) {
    scriptSrc.push("'unsafe-eval'");
  }

  // connect-src: API / 認証は #41 で同一オリジン化済みのため 'self' で足りる。
  // dev のみ HMR の websocket（ws: / wss:）を許可する。
  const connectSrc = ["'self'"];
  if (!isProduction) {
    connectSrc.push("ws:", "wss:");
  }

  // 挿入順を保つため Map を使う（CSP 文字列の決定論的な並びを保証する）。
  const directives = new Map<string, string[]>();
  directives.set("default-src", ["'self'"]);
  directives.set("script-src", scriptSrc);
  // style-src: Tailwind / next/font が注入する inline style 用に 'unsafe-inline' を許容。
  directives.set("style-src", ["'self'", "'unsafe-inline'"]);
  // img-src: 記事本文 HTML 中の外部画像を表示するため data: と HTTPS 外部画像を許可（平文 http は不許可）。
  directives.set("img-src", ["'self'", "data:", "https:"]);
  directives.set("font-src", ["'self'"]);
  directives.set("connect-src", connectSrc);
  // object-src / base-uri / frame-ancestors / form-action: OWASP 推奨の最小堅牢化。
  // frame-ancestors 'none' は X-Frame-Options: DENY と整合する。
  directives.set("object-src", ["'none'"]);
  directives.set("base-uri", ["'self'"]);
  directives.set("frame-ancestors", ["'none'"]);
  directives.set("form-action", ["'self'"]);

  return directives;
}

/**
 * CSP ヘッダーに設定する文字列値を生成する純粋関数。
 *
 * `buildCspDirectives` の結果を `"<directive> <values...>; ..."` 形式の 1 行へ直列化する。
 *
 * @param env - 参照する環境（既定: `process.env`）。`NODE_ENV` のみ参照する
 * @returns CSP ヘッダー値（例: `"default-src 'self'; script-src 'self' 'unsafe-inline'; ..."`）
 */
export function buildContentSecurityPolicy(env: CspEnv = process.env): string {
  const directives = buildCspDirectives(env);

  return Array.from(directives.entries())
    .map(([name, values]) => `${name} ${values.join(" ")}`)
    .join("; ");
}
