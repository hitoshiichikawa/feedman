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
