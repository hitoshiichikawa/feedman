# Review Notes

<!-- idd-claude:review round=2 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-41-impl-web-api-next-rewrites-cloudflare-ingress
- HEAD commit: 3be7dc0cc11ca587915c1c25c6ee9dd6d3866e91
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `web/Dockerfile`: builder stage の `ARG/ENV NEXT_PUBLIC_API_URL` を削除（build-once）。`web/next.config.ts` の `rewrites()` は throw しない `resolveApiInternalUrlForRewrites` を使うためビルドが env 未指定でも完了する。テスト `rewrites.test.ts > resolveApiInternalUrlForRewrites > "未設定のとき throw せず暫定 base を返すこと"` + impl-notes の `npm run build`（`API_INTERNAL_URL` 未指定）成功記録で担保
- 1.2 — `web/src/lib/api.ts`: `API_BASE_URL = ""` 固定で成果物に API 絶対 URL を含まない。`api.test.ts > NEXT_PUBLIC_API_URL 非依存 > "...API_BASE_URL が空文字のままであること"`
- 1.3 — `web/next.config.ts` の `rewrites()` が実行時 `API_INTERNAL_URL` を参照（`compose` の `web.environment` で注入）。`buildRewrites` の 2 ルール内容テスト
- 1.4 — `api.test.ts > NEXT_PUBLIC_API_URL 非依存 > "NEXT_PUBLIC_API_URL が設定されていても相対パスで fetch されること"`（env 設定後も `fetch("/api/feeds", ...)`）
- 2.1 — `rewrites.test.ts > buildRewrites`（`/api/:path*`→`<base>/api/:path*`、プレフィックス保持）+ `next.config.ts` `rewrites()`
- 2.2 — 同上（`/auth/:path*`→`<base>/auth/:path*`）
- 2.3 — `api.ts` の相対パス固定 + `next.config.ts` 同一オリジン proxy 化により CORS プリフライト不要（同一オリジン化の帰結）
- 2.4 / 2.5 — Next.js `rewrites()` の server-side proxy 透過挙動（design Decision）。自動化検証は deferrable Task 7 に委譲。本サイクルでは設計通り proxy 実装で担保
- 4.1 — `web/server-entrypoint.mjs`: `resolveApiInternalUrl(process.env)` を呼び未設定/空で `process.exit(1)`。`rewrites.test.ts > resolveApiInternalUrl > "変数が未設定のとき throw..." / "空文字..." / "空白のみ..."`
- 4.2 / NFR 2.1 — entrypoint が catch して `console.error(message)`（メッセージに `API_INTERNAL_URL` を含む）後に exit。`resolveApiInternalUrl` の throw メッセージテスト（`toThrow(API_INTERNAL_URL_ENV)`）
- 4.3 — entrypoint 検証通過時のみ `await import("./server.js")` で起動。`resolveApiInternalUrl > "有効な値..."` / `resolveApiInternalUrlForRewrites > "有効な値..."`
- 5.1 / 5.2 — env 非依存な 2 ルール生成（`buildRewrites`）+ `api.ts` 相対パス固定でローカル/本番同一の `/api` 規約
- 5.3 / NFR 1.2 — Dockerfile `ARG NEXT_PUBLIC_API_URL` 撤去 + `docker-compose.yml` の `web.build.args` 撤去 + `npm run build` 未指定成功
- 3.4 / 3.5 / 6.2 — Go コードは差分に含まれず不変（`git diff --name-only develop..HEAD` に `.go` ファイルなし）。Cookie 属性（`SameSite=Lax` / `HttpOnly` / `Secure`）と OAuth フローを維持
- 6.1 — 既存フロントテスト維持（impl-notes に `npx vitest run` 全 146 件 green の記録。本環境は node 未導入のため再実行できず Developer 記録に依拠）
- 6.3 / NFR 1.1 — `api.ts` のエンドポイントパス・`credentials: "include"`・`createApiClient` シグネチャ不変

## Findings

なし

## Summary

round=1 で指摘した 5 件の Findings（Dockerfile の `NEXT_PUBLIC_API_URL` 撤去 / rewrites
中核実装 / rewrites 単体テスト / fail-fast entrypoint / compose・env・docs の単一オリジン化）は
いずれも解消済み。tasks.md の必須 Task 1〜6 がすべて実装・テストされ、全 numeric AC に
対応する実装またはテストが差分・既存コードのいずれかで確認できた。`_Boundary:_`（Rewrites
Proxy / Startup Validation / API Client）の逸脱はなく、Go コードは不変で Out of Scope と整合する。
Feature Flag Protocol は opt-out 宣言のため flag 観点は適用しない。node 未導入のため
テスト再実行は不可だが、impl-notes のテスト結果記録（vitest 146 件 / go test 全件 green /
build 未指定成功）と差分内容が一致しており approve とする。

RESULT: approve
