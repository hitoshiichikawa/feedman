# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T15:20:00Z -->

## Reviewed Scope

- Branch: claude/issue-25-impl-api
- HEAD commit: ba35abfeab79857933f7a0ea660041ba49e35ad6
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `web/src/lib/api.ts:106` `export const apiClient: ApiClient = createApiClient();`（モジュール初期化時に 1 度だけ生成）。テスト `api.test.ts`「モジュールから公開された共有インスタンスが定義されていること」
- 1.2 — module-level const のため複数回参照しても追加生成されない。テスト `api.test.ts`「同一参照を複数回読み出しても同じオブジェクトであること」
- 1.3 — 全消費者（フック 5 + コンポーネント 2）が `apiClient` を import。テスト `api.test.ts`「複数回参照しても同一の共有インスタンス参照を返すこと」（`import("./api")` 2 回が同一参照）
- 2.1 — `use-feeds.ts` を `apiClient.get` 参照へ移行。既存 `use-feeds.test.tsx`（3 tests）pass
- 2.2 — `use-items.ts` を `apiClient.get` 参照へ移行。既存 `use-items.test.tsx`（5 tests）pass
- 2.3 — `use-auth.ts`（`useCurrentUser` / `useLogout`）を `apiClient` 参照へ移行。既存 `use-auth.test.tsx`（3 tests）pass
- 2.4 — `use-item-state.ts`（`useMarkAsRead` / `useToggleStar`）を `apiClient` 参照へ移行。既存 `use-item-state.test.tsx`（3 tests）pass
- 2.5 — `use-subscriptions.ts`（`useUpdateFetchInterval` / `useUnsubscribe` / `useResumeFeed`）+ `feed-register-dialog.tsx` / `withdraw-dialog.tsx` を `apiClient` 参照へ移行。既存テスト群 pass
- 2.6 — 各消費者の差分は `createApiClient()` 呼び出し削除 + 参照名置換のみで、戻り値・引数・呼び出し方は不変（フックシグネチャ無変更）。利用側コンポーネントの修正は不要（差分なし）
- 3.1 — 各フック・コンポーネント先頭の `"use client"` を保持（diff 上で変更なし）
- 3.2 — `createApiClient()` を削除せず温存（`api.ts:86`）。テスト `api.test.ts`「共有インスタンスが createApiClient で生成した新規インスタンスとは別オブジェクトであること」。既存テストの `global.fetch` モック境界は不変
- 3.3 — 既存テストスイートを破壊しない。reviewer 再実行で `api.test.ts`（16 tests）+ 消費者テスト 7 ファイル（30 tests）全 pass を確認
- NFR 1 — 関数シグネチャ・戻り値型に破壊的変更なし（参照名置換のみ）
- NFR 2 — module-level const により API クライアントインスタンスを 1 個に保ち、コンポーネント数に比例した追加生成を行わない

## Findings

なし

## Summary

全 numeric ID（1.1〜1.3 / 2.1〜2.6 / 3.1〜3.3 / NFR 1 / NFR 2）が実装またはテストでカバー済み。共有インスタンス化の新規挙動には `api.test.ts` に対応テストが追加され、消費者は参照名置換のみで公開 IF 不変。`createApiClient` の内部実装・`"use client"` 境界は無変更で Out of Scope を逸脱しない。reviewer 再実行でも全テスト pass を確認。Feature Flag Protocol は opt-out のため flag 観点は不適用。

RESULT: approve
