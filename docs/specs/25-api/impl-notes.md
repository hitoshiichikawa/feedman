# 実装ノート（Issue #25: API クライアント共有インスタンス化）

## 変更概要

Web フロントエンドの API クライアントを、毎レンダリング（毎実行）で `createApiClient()` を
呼び出す方式から、モジュールレベルの共有シングルトン `apiClient` を参照する方式へ集約した。
API クライアントはステートレス（`request` 関数をラップするだけで固有状態を持たない）であるため、
モジュール初期化時に一度だけ生成して全消費者で共有でき、レンダリングごとの不要な生成を排除できる
（技術債返済 / 優先度 Low）。

### 変更ファイル

- `web/src/lib/api.ts`
  - `export const apiClient: ApiClient = createApiClient();` を追加（モジュール初期化時に 1 度だけ生成）
  - `createApiClient()` 関数は **削除せず温存**（テスト用途・独立インスタンス取得用途。Requirement 3.2）
- `web/src/lib/api.test.ts`
  - 共有インスタンス `apiClient` の同一参照性・`createApiClient` で生成した新規インスタンスとの独立性・
    共有インスタンス経由の fetch 挙動を検証するテストを追加
- 消費者（毎レンダリング生成を排除し、共有インスタンス参照へ変更）
  - フック: `web/src/hooks/use-feeds.ts` / `use-items.ts` / `use-auth.ts`（`useCurrentUser` / `useLogout`）/
    `use-item-state.ts`（`useMarkAsRead` / `useToggleStar`）/ `use-subscriptions.ts`
    （`useUpdateFetchInterval` / `useUnsubscribe` / `useResumeFeed`）
  - コンポーネント: `web/src/components/feed-register-dialog.tsx` / `withdraw-dialog.tsx`

`createApiClient` の参照は `api.ts`（定義）と `api.test.ts`（テスト）のみに残し、全消費者は
`apiClient` を import する形に統一した（grep で確認済み）。

## 採用した共有インスタンス公開方式

- `api.ts` に `export const apiClient: ApiClient = createApiClient();` を 1 行で公開する方式を採用。
  - ES Module の評価セマンティクス上、モジュールは初回 import 時に 1 度だけ評価され、以降は同一の
    モジュールインスタンスが返るため、`const apiClient` は自然にシングルトンとなる（Requirement 1.1/1.2/1.3）。
  - 遅延初期化（getter 関数）も検討したが、ステートレスかつ生成コストが軽微で、即時生成で副作用が
    無い（`request` をラップするのみ）ため、最もシンプルな module-level const を採用した。
- `createApiClient()` 自体の内部実装（`request` / 認証 / `API_BASE_URL`）は一切変更していない
  （スコープ外。`API_BASE_URL = ""` は #41 / PR #62 で確定済みのため不変）。

## テスト観点（AC との対応）

| Requirement | 観点 | 担保したテスト |
|---|---|---|
| 1.1 / 1.2 | モジュール初期化時に 1 度だけ生成され、複数回参照しても追加生成しない | `api.test.ts` 「モジュールから公開された共有インスタンスが定義されていること」「同一参照を複数回読み出しても同じオブジェクトであること」 |
| 1.2 / 1.3 / NFR 2.1 | 複数消費者が同一の共有インスタンスを参照する（追加生成されない） | `api.test.ts` 「複数回参照しても同一の共有インスタンス参照を返すこと」 |
| 2.1 | フィード一覧取得が従来と同一 | `use-feeds.test.tsx`（既存・全 pass） |
| 2.2 | アイテム一覧取得が従来と同一 | `use-items.test.tsx`（既存・全 pass） |
| 2.3 | 認証状態取得・認証操作が従来と同一 | `use-auth.test.tsx`（既存・全 pass） |
| 2.4 | アイテム状態更新が従来と同一 | `use-item-state.test.tsx`（既存・全 pass） |
| 2.5 | 購読の登録・解除・設定更新が従来と同一 | `use-subscriptions.test.tsx` / `feed-register-dialog.test.tsx` / `withdraw-dialog.test.tsx`（既存・全 pass） |
| 2.6 / NFR 1 | 公開インターフェース不変（利用側修正不要） | 利用側コンポーネントのテスト群が無修正で pass することで担保 |
| 3.1 | `"use client"` 前提を変更しない | 各フック・コンポーネント先頭の `"use client"` を保持（差分なし） |
| 3.2 | モック差し替え可能性の維持 | `createApiClient` を温存（`api.test.ts` 「共有インスタンスが createApiClient で生成した新規インスタンスとは別オブジェクトであること」）。既存テストは `global.fetch` 境界でモックしており、共有インスタンス化後も同一メカニズムで差し替え可能 |
| 3.3 | 既存テストを破壊しない | 全 25 ファイル / 183 テストが pass |

### Red → Green の確認

`apiClient` 未実装の状態で新規テストを実行し、「`expected undefined to be defined`」で失敗
（Red）することを観測してから、`api.ts` に `apiClient` を追加して全テスト pass（Green）させた。

## 検証結果

実行環境では `node` / `npm` が PATH 上に無かったため `/home/hitoshi/.local/node/bin` を PATH に
追加して実行した。また当環境の Node は v22.11.0 で、vitest@4 の CJS shim が vite@7（ESM-only）を
`require()` する際に `ERR_REQUIRE_ESM` が発生したため、`NODE_OPTIONS="--experimental-require-module"`
を付与して実行した（後述「確認事項」参照）。

- `cd web && npm test`（= `vitest run`）: **25 ファイル / 183 テスト 全 pass**
- `cd web && npm run lint`（ESLint）: **0 errors**（6 warnings はいずれも本変更で触れていない
  既存ファイル由来。`feed-list.tsx` の `no-img-element` 等）
- `cd web && npm run build`（Next.js build）: **成功**

## 確認事項

- **実行環境の Node バージョン差異**: CI（`.github/workflows/ci.yml`）は Node 20 を使用するが、
  当開発環境は Node 22.11.0 のみ利用可能だった。Node 22.11 では vitest@4 が vite@7 を `require()`
  する箇所で `ERR_REQUIRE_ESM` が発生するため、ローカル検証時のみ
  `NODE_OPTIONS="--experimental-require-module"` を付与した。これは実行環境固有の事情であり、
  本変更（実装）とは無関係。CI（Node 20）では当該フラグなしで従来どおり green になる想定。
  本リポジトリの依存バージョン整合（Node エンジン要件と vitest/vite の組み合わせ）については、
  必要であれば別 Issue での検討対象（本 Issue のスコープ外）。
- **要件定義との矛盾**: なし。`requirements.md` の記述（消費者全体を対象・公開 IF 不変・
  `createApiClient` 温存・`"use client"` 維持）に沿って実装した。

STATUS: complete
