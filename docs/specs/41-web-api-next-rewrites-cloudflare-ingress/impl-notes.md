# 実装メモ（Issue #41: web/api 単一オリジン化）

## Implementation Notes

### Task 1

- **採用方針**: `web/src/lib/api.ts` の `API_BASE_URL` を `process.env.NEXT_PUBLIC_API_URL` 非依存の固定空文字 `""` にし、常に同一オリジン相対パスで `fetch` する構成にした（design.md の API Client コンポーネント定義に準拠）。
- **重要な判断**:
  - `API_BASE_URL` はモジュールロード時に評価される `const` のため、`NEXT_PUBLIC_API_URL` を無視することを検証するテスト（Req 1.4）は `vi.stubEnv` + `vi.resetModules` + 動的 `import("./api")` で構成した。env を設定したうえでモジュールを再読込しても `fetch` が相対パス `/api/feeds` で呼ばれること、および `API_BASE_URL` が `""` のままであることを 2 ケースで担保している。
  - エンドポイントパス・`credentials: "include"`・`ApiError`・`createApiClient`・`request` のシグネチャは不変（NFR 1.1 / Req 6.3）。`fullUrl = ${API_BASE_URL}${url}` の組み立ても据え置きで、`API_BASE_URL=""` のため相対パスになる。
  - doc comment を「単一オリジン相対パス方針・`NEXT_PUBLIC_API_URL` 非参照・rewrites（`API_INTERNAL_URL`）が内部転送を担う」旨に更新した。
- **残存課題**: なし（task 2 以降の rewrites / fail-fast / Docker / compose 側で `API_INTERNAL_URL` 導入を行う前提。本 task の範囲では api.ts の env 撤去のみで自己完結）。

## 受入基準とテストの対応（Task 1 範囲）

| Req ID | 担保するテスト |
|---|---|
| 1.2（成果物に API 絶対 URL を含まない） | `api.test.ts > NEXT_PUBLIC_API_URL 非依存 > "...API_BASE_URL が空文字のままであること"`（`API_BASE_URL === ""` を assert） |
| 1.4（env が与えられても相対パス維持） | `api.test.ts > NEXT_PUBLIC_API_URL 非依存 > "NEXT_PUBLIC_API_URL が設定されていても相対パスで fetch されること"`（絶対 URL を env 設定後も `fetch("/api/feeds", ...)` を assert） |
| 2.3 / 5.2 / 6.3 / NFR 1.1（相対パスで各エンドポイント到達） | 既存 GET/POST/PUT/PATCH/DELETE ケース（`/api/feeds` 等で相対パス `fetch` を assert）を維持 |
| 5.3（ビルド時引数を要求しない） | `API_BASE_URL = ""` 固定により `NEXT_PUBLIC_API_URL` 参照を撤去（上記 1.2 / 1.4 テストで間接担保） |
| 6.1（既存フロントテスト成功） | `npm test` 全 135 件 green を確認 |

## 確認事項

- **ローカル実行環境の Node バージョン制約**: 本 worktree のローカル環境は Node `v22.11.0` だが、`web` の `vite@7.3.1`（vitest 経由）は `^20.19.0 || ^22.12.0 || >=24.0.0` を要求しており、`22.11.0` では `vitest run` の config ロード時に `ERR_REQUIRE_ESM` が発生する。`NODE_OPTIONS=--experimental-require-module` を付与して全テスト・lint を green 確認した。CI（`.github/workflows/ci.yml`）は Node 20 を使用するため、この環境固有の制約は CI には影響しない（本 task の実装内容とは無関係）。この点は spec の矛盾ではなく実行環境の事象として記録する。

## 是正実装（Reviewer round=1 reject 対応）

Reviewer round=1 の 5 件の Findings を是正し、Task 2〜6 を実装した。各 Finding への対応は以下。

### Finding 1（Task 4.2 一部 / Req 1.1）: Dockerfile の NEXT_PUBLIC_API_URL 撤去

- `web/Dockerfile` builder stage の `ARG NEXT_PUBLIC_API_URL` / `ENV NEXT_PUBLIC_API_URL=...`（旧 20-21 行）を削除し、ビルド成果物に API URL を焼き込まない構成にした（build-once）。
- 確認: `grep NEXT_PUBLIC_API_URL web/Dockerfile` で残存なし。

### Finding 2（Task 2.1 + Task 3 / Req 1.3, 2.1〜2.5, 5.1）: rewrites の中核実装

- `web/src/lib/rewrites.ts` を新規作成。`API_INTERNAL_URL_ENV` 定数 / `resolveApiInternalUrl(env)`（未設定・空・空白で throw、末尾スラッシュ正規化）/ `buildRewrites(base)`（`/api/:path*`・`/auth/:path*` をプレフィックス保持で転送する 2 ルール生成）を design.md Service Interface のシグネチャ通りに実装。
- `web/next.config.ts` に `async rewrites()` を追加。`output: "standalone"` と既存 `headers()` は維持。
- **設計補正の判断（後述「確認事項（是正実装）」#1 と対）**: `buildRewrites(resolveApiInternalUrl(process.env))` を rewrites() でそのまま呼ぶと、Next.js が `rewrites()` を **ビルド時にも評価する**ため、`API_INTERNAL_URL` 未指定だとビルドが throw して失敗し **Req 1.1（未指定でビルド完了）/ Req 5.3（ビルド時引数を要求しない）に違反**することを `npm run build` で実測確認した。design.md Error Handling の Decision は「rewrites 内 throw は却下、fail-fast は起動 entrypoint へ集約」と明記しているため、rewrites() では throw しない `resolveApiInternalUrlForRewrites(env)`（未設定時は暫定 base `API_INTERNAL_URL_FALLBACK="http://api:8080"` を返す）を新設して使用した。これにより Req 1.1 を満たしつつ、未設定検出の fail-fast 責務は entrypoint（`resolveApiInternalUrl`）に集約され design の Decision と整合する。

### Finding 3（Task 2.2 / missing test）: rewrites の単体テスト

- `web/src/lib/rewrites.test.ts` を新規作成。design.md「Testing Strategy > Unit Tests」1〜4 + Finding 2 補正分をカバー。全 11 ケース green。

### Finding 4（Task 4.1 + 4.2 / Req 4.1〜4.3, NFR 2.1）: fail-fast entrypoint

- `web/server-entrypoint.mjs` を新規作成。`API_INTERNAL_URL` を検証し、未設定/空なら stderr に変数名を含むエラーを出力して `process.exit(1)`、通過時に `import("./server.js")` で standalone サーバを起動する。
- **import 解決の判断**: standalone 出力（`.next/standalone/`）には `src/lib/rewrites.ts` が **同梱されない**ことをビルド成果物の実査で確認した（standalone root は `server.js` / `node_modules` / `package.json` のみ）。design.md は「`resolveApiInternalUrl` 再利用」を意図するが、standalone runtime で確実に動かすため検証ロジック（変数名 `API_INTERNAL_URL` / throw 条件 / メッセージ）を entrypoint 内に **inline** した。design の意図（`resolveApiInternalUrl` と意味的に等価な起動前 fail-fast）と矛盾しない。
- **実機検証**: ビルド済み standalone に entrypoint をコピーし、(a) 未設定→exit=1 + メッセージ、(b) 空文字→exit=1 + メッセージ、(c) 有効値→server.js が "Ready" で起動、を実測確認。
- `web/Dockerfile` runner stage で `server-entrypoint.mjs` を `server.js` と同階層（standalone root = `/app`）へコピーし、`CMD ["node", "server-entrypoint.mjs"]` に変更した。

### Finding 5（Task 5.1 + 5.2 / Req 5.3, NFR 1.2, 他）: 構成・env・docs

- `docker-compose.yml`: `web.build.args.NEXT_PUBLIC_API_URL` を削除し、`web.environment` に `API_INTERNAL_URL=${API_INTERNAL_URL:-http://api:8080}` を追加。web から `api` への内部到達のため web を `internal` ネットワークにも接続（+ `depends_on: api`）。`api` の `GOOGLE_REDIRECT_URL` / `BASE_URL` を単一オリジン（ブラウザ可視オリジン）前提のコメントで補足し、デフォルトを web オリジン（`http://localhost:3000`）側へ更新。
- `.env.sample`: `NEXT_PUBLIC_API_URL` 削除、`API_INTERNAL_URL` 追加、`GOOGLE_REDIRECT_URL`（`http://localhost:3000/auth/google/callback`）/ `BASE_URL`（`http://localhost:3000`）を単一オリジンの説明に更新。
- `README.md`: アーキテクチャ図（2 オリジン→単一オリジン + rewrites proxy）、build-once 節、Google OAuth redirect URI 登録手順、環境変数表（所有列追加 / `API_INTERNAL_URL` 追加 / `NEXT_PUBLIC_API_URL` 廃止注記）、本番デプロイ注意事項、ネットワークセキュリティ、セキュリティ節（first-party Cookie / `SameSite=Lax` CSRF）、Docker なしローカル開発手順を更新。

### Task 6（Req 3.4, 3.5, 6.2）: バックエンド非改修の回帰確認

- Go コードは一切変更していない（`git diff --name-only <base>...HEAD` に `.go` ファイルなし）。
- `go test ./...` を実行し全パッケージ green（既存バックエンドテスト全件成功 = Req 6.2）。`go vet ./...` も exit=0。
  - 補足: `gofmt -l ./internal ./cmd` は既存 Go ファイルを複数挙げるが、これらは本ブランチで未変更の既存リポジトリ状態であり本 Issue の範囲外（spec も Go コード不変を明示）。

## 是正実装の受入基準とテストの対応

| Req ID | 担保 |
|---|---|
| 1.1（未指定でビルド完了） | `npm run build` を `API_INTERNAL_URL` 未指定で実測し成功を確認。`resolveApiInternalUrlForRewrites` の未設定時 fallback テスト（`rewrites.test.ts > resolveApiInternalUrlForRewrites > "未設定のとき throw せず暫定 base を返すこと"`） |
| 1.3（同一イメージで各環境 API へ到達） | `buildRewrites` テスト（2 ルール内容）+ `API_INTERNAL_URL` 実行時参照（compose env） |
| 2.1（/api/* 転送） | `rewrites.test.ts > buildRewrites > "/api/:path* と /auth/:path* の 2 ルールをプレフィックス保持で返すこと"`（`/api/:path*`→`<base>/api/:path*`） |
| 2.2（/auth/* 転送） | 同上テスト（`/auth/:path*`→`<base>/auth/:path*`） |
| 2.3 / 2.4 / 2.5 / 3.1 / 3.2 / NFR 3.3（透過・Cookie・機密非露出） | next.config rewrites() による server-side proxy（Next.js の透過挙動）。自動化は deferrable Task 7（結合テスト）に委譲、本サイクルでは設計通り未実施 |
| 4.1（未設定/空で起動中断） | `server-entrypoint.mjs` 実機検証（未設定/空→exit=1）+ `rewrites.test.ts > resolveApiInternalUrl > "変数が未設定のとき throw..." / "空文字..." / "空白のみ..."` |
| 4.2（不足項目を識別できるエラー） | 同検証（メッセージに `API_INTERNAL_URL` を含む）+ `resolveApiInternalUrl` の throw メッセージテスト（`toThrow(API_INTERNAL_URL_ENV)`） |
| 4.3（有効値で起動完了） | `server-entrypoint.mjs` 実機検証（有効値→server.js "Ready"）+ `resolveApiInternalUrl > "有効な値が与えられたとき..."` |
| 5.1 / 5.2（同一ルーティング規約） | `buildRewrites` の env 非依存な 2 ルール生成 + `api.ts` 相対パス固定（Task 1）|
| 5.3 / NFR 1.2（ビルド時引数を要求しない） | Dockerfile `ARG NEXT_PUBLIC_API_URL` 撤去 + `npm run build` 未指定成功 |
| NFR 2.1（起動失敗理由をログ出力） | `server-entrypoint.mjs` 実機検証で stderr にメッセージ 1 件出力を確認 |
| 6.1（既存フロントテスト成功） | `npx vitest run` 全 146 件 green |
| 6.2（既存バックエンドテスト成功） | `go test ./...` 全パッケージ green |
| 6.3 / NFR 1.1（既存エンドポイントへ相対パス到達） | `api.test.ts` 既存ケース（`/api/feeds` 等の相対パス fetch）維持（Task 1）|

## テスト実行結果（是正実装）

- web 単体テスト: `cd web && NODE_OPTIONS=--experimental-require-module npx vitest run` → **23 files / 146 tests passed**（Task 1 完了時 135 → rewrites 11 件追加で 146）
- web lint: `npm run lint` → **0 errors / 6 warnings**（warning は全て既存テストファイルの未使用変数で本変更と無関係）
- web build: `npm run build`（`API_INTERNAL_URL` 未指定）→ **成功**（Req 1.1 実証）
- Go: `go test ./...` → **全パッケージ ok**、`go vet ./...` → **exit=0**

## 確認事項（是正実装）

> requirements/design/tasks は書き換えていない。以下は実装中に気づいた設計レビュー観点の論点。

1. **Next.js standalone の rewrites destination はビルド時に焼き込まれる（runtime 切替不可の可能性）**: design.md は「Next.js standalone の rewrites は runtime proxy」「`server.js`（standalone）が runtime で rewrites を適用」と記述しているが、実測では standalone の `server.js` に `__NEXT_PRIVATE_STANDALONE_CONFIG` として **ビルド時に評価された rewrites の destination**（`http://api:8080/...`）が JSON 文字列で焼き込まれていた。これは「実行時の `API_INTERNAL_URL` を変えても rewrites の転送先が変わらない」可能性を示唆し、Req 1.3（同一イメージで各環境 API へ到達）/ build-once の根幹に関わる。本 PR では Dockerfile / compose のデフォルト（`http://api:8080`）が本番含む想定構成と一致する前提で実装したが、**環境ごとに内部 API URL が異なる運用が必要な場合、本設計（next.config の rewrites）では runtime 切替が効かない**恐れがある。Next.js のバージョン（15.5.12）依存の挙動でもあり、Architect による設計確認（middleware proxy への切替や、ビルド時に確実に決まる内部 URL を前提とする運用方針の明文化など）を提案する。Task 7（結合テスト / PoC）でこの透過・転送挙動を実機検証することが望ましい。
2. **`API_INTERNAL_URL` のデフォルト値**: compose / `.env.sample` のデフォルトを `http://api:8080`（compose サービス名）とした。entrypoint の fail-fast は「未設定/空」で発火するため、compose 経由ではデフォルト値が常に入り fail-fast はローカルでは発火しない（意図通り）。Dockerfile 単体起動・本番オーケストレータでは env を明示設定する前提。

STATUS: complete
