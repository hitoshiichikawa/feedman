/**
 * web（Next.js standalone）の起動 entrypoint。
 *
 * standalone の `server.js` を起動する前に内部 API 接続先設定（`API_INTERNAL_URL`）を
 * 検証し、未設定/空なら stderr にエラーメッセージを出力して非ゼロ終了する（fail-fast）。
 * これにより誤設定コンテナが ready にならず、誤設定の本番投入を防ぐ（Req 4.1/4.2/NFR 2.1）。
 *
 * 注意: Next.js standalone 出力には `src/lib/rewrites.ts` が同梱されないため、
 * 当該モジュールを import せず、検証ロジック（変数名・throw 条件・メッセージ）を
 * 本ファイル内に inline で保持する。`rewrites.ts` の `resolveApiInternalUrl` と
 * 意味的に等価な fail-fast を standalone runtime で確実に実行するための判断
 * （design.md Error Handling の Decision: 起動 entrypoint に fail-fast を集約）。
 */

/** 環境変数名（rewrites.ts の API_INTERNAL_URL_ENV と同一） */
const API_INTERNAL_URL_ENV = "API_INTERNAL_URL";

/**
 * `API_INTERNAL_URL` を検証する。未設定/空（空白のみ含む）なら Error を投げる。
 * rewrites.ts の `resolveApiInternalUrl` と等価な検証ロジック。
 *
 * @param {NodeJS.ProcessEnv} env - 参照する環境変数オブジェクト
 * @returns {string} 末尾スラッシュを除去した内部 API 接続先 base
 */
function resolveApiInternalUrl(env) {
  const raw = env[API_INTERNAL_URL_ENV];

  if (raw === undefined || raw.trim() === "") {
    throw new Error(
      `FATAL: ${API_INTERNAL_URL_ENV} is not set. ` +
        `内部 API 接続先（${API_INTERNAL_URL_ENV}）を実行時環境変数として設定してください。`
    );
  }

  return raw.trim().replace(/\/+$/, "");
}

try {
  resolveApiInternalUrl(process.env);
} catch (err) {
  const message = err instanceof Error ? err.message : String(err);
  console.error(message);
  process.exit(1);
}

// 検証通過時のみ standalone サーバを起動する（Req 4.3）。
await import("./server.js");
