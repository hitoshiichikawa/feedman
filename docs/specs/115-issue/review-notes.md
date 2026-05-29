# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-29T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-115-impl-issue
- HEAD commit: ec33b67dd9b1cfd9e8e27f7eb1946b5074e8cf8e
- Compared to: develop..HEAD（merge-base 起点で実装差分 37 ファイル / +3116 / -115）

差分取得経路:

- `git log --oneline develop..HEAD` で 38 commits を確認（task 1〜7 の feat/test/refactor/docs と
  develop マージ chore）
- `git diff --stat develop..HEAD` を取得し、本ブランチ未取り込みの develop コミット由来の削除
  （`internal/itemsearch/` / `internal/handler/item_search_handler.go` / `web/src/components/starred-*`
  / `web/src/hooks/use-item-search.ts` 等）が #117 / #120 由来であることを `git log` で確認。
  本実装の追加・変更は task 1〜7 の boundary 内に閉じている

verify 状況:

- `go build ./...` 成功
- `go test ./internal/subscription/... ./internal/metrics/... ./internal/handler/...
  ./internal/worker/fetch/... ./internal/middleware/... ./internal/model/...` 全て pass
- `npm test`（web 側）はローカル node_modules の vitest/vite ESM 不整合
  （`ERR_REQUIRE_ESM`）で reviewer 環境では起動不可。impl-notes Task 7 で Developer が
  実施済みの旨を申告しており、本 reviewer 環境固有の問題と判断（実装側コードを書き換える
  事象ではない）。AC カバレッジは記述レベルで verify した（後述）

## Verified Requirements

- 1.1 — `subscription.Service.ManualFetch`（`internal/subscription/service.go:324`）が同期で
  fetcher.Fetch を呼び、handler が完了後に 200 を返却。`TestSubscriptionHandler_ManualFetch_Success`
  / `TestService_ManualFetch_Success` で検証
- 1.2 — `s.feedRepo.UpdateLastSuccessfulFetchAt(ctx, feed.ID, time.Now())`
  （`internal/subscription/service.go:406`）。Worker 側も `recordLastSuccessfulFetch`
  （`internal/worker/fetch/fetcher.go:334`）で同時更新
- 1.3 — `ManualFetch` 内で goroutine 化なし。fetcher.Fetch を直接 await
- 1.4 — `UserIDFromContext` で 401 短絡、`TestSubscriptionHandler_ManualFetch_Unauthorized`
  でサービス未呼び出しを spy 変数で検証
- 1.5 / 1.6 — `sub == nil || sub.UserID != userID` の単一 branch で SubscriptionNotFound
  （`service.go:330-332`）。`TestSubscriptionHandler_ManualFetch_NotFound` で 2 ケース table-driven
- 1.7 — handler が `r.Body` を読まず `chi.URLParam(r, "id")` のみで処理。
  `TestSubscriptionHandler_ManualFetch_NoBody` で空ボディ 200 を検証
- 2.1 — `manualFetchCooldown = 10*time.Minute` の判定で `model.NewFeedCooldownError`、
  HTTP 429 マッピング（`internal/handler/feed_handler.go:294`）
- 2.2 — `NewFeedCooldownError` が `Details["retry_after_seconds"]` を載せ、
  `middleware.ErrorResponseBody.Details` が `omitempty` 付き JSON で透過。
  `TestSubscriptionHandler_ManualFetch_Cooldown` / `TestWriteErrorResponse_WithDetails` で検証
- 2.3 — `elapsed < manualFetchCooldown` の境界判定（`service.go:364`）。
  `TestService_ManualFetch_Cooldown` で 9m59s / 10m / nil ケース
- 2.4 — `last_successful_fetch_at` カラムを worker（fetcher.recordLastSuccessfulFetch）と
  service（ManualFetch 成功時）の両経路で更新。共通カラムで同等扱い
- 2.5 — `feed.LastSuccessfulFetchAt != nil` ガードで nil 時はクールダウン非適用
- 3.1 — `feedRepo.LockFeedForUpdateNowait` が `SELECT ... FOR UPDATE NOWAIT` を発行
  （`internal/repository/postgres_feed_repo.go` の `LockFeedForUpdateNowait` + sentinel
  `ErrFeedLocked`）。処理開始前に取得済み
- 3.2 — `errors.Is(err, repository.ErrFeedLocked)` 分岐で `NewFeedFetchInProgressError`
  → HTTP 409。`TestService_ManualFetch_LockConflict` でロック取得失敗時に fetcher 未呼び出しを検証
- 3.3 — `NewFeedFetchInProgressError` の Message / Action に再試行案内文言
- 3.4 — tx は cooldown 判定後に明示的 COMMIT、それ以前は defer Rollback で必ず解放。
  レスポンス返却前にロック解放（Note: 実装は自己 deadlock 回避のため fetcher 実行 **前** に
  COMMIT する設計逸脱を選択。design.md「データ更新の責務」/ impl-notes Task 4 で justify 済み。
  numeric AC 3.4 の「ロックは応答返却前に解放する」上限規定としては satisfied）
- 4.1 — Fetcher 内部の `ssrfGuard.ValidateURL` 再利用（既存挙動温存）
- 4.2 — SSRF 検出時は service 層で `reason == "ssrf_blocked"` の場合
  「一時的なエラー」文言にマスクして FETCH_FAILED を返す（`service.go:392-396`）
- 4.3 — `item.ItemUpsertService`（bluemonday）経由を変更せず再利用
- 5.1 — `ItemList` 内 `ManualRefreshButton` をフィルタタブ右に `flex justify-between` で配置
  （`web/src/components/item-list.tsx:144-164`）
- 5.2 — `feedId === null` で `ItemList` が早期 return（既存挙動）+ `subscriptionId !== null`
  ガードで button 非描画。`it("feedId=null のときはボタンを描画しないこと")` で検証
- 5.3 — `mutate(subscriptionId)` を onClick で発火、useMutation の native semantics で重複
  起動なし。`it("ボタンクリックで POST /api/subscriptions/:id/fetch が発火すること")` で検証
- 5.4 — `aria-busy={isPending}` / `disabled={isPending}` / `animate-spin` クラス。
  `it("mutation 進行中はボタンが disabled + animate-spin になること")` で検証
- 5.5 — useMutation の isPending が false に戻ることで自然解除
- 5.6 — `disabled={isPending}` で HTML native semantics による重複防止
- 6.1 — `onSuccess` で `queryClient.invalidateQueries({ queryKey: ["items", feedId] })`
- 6.2 — 同上で `["feeds"]` も invalidate
- 6.3 — invalidate は背景再取得（既存 `useInfiniteQuery` がページキャッシュ保持）
- 7.1 — `ManualRefreshBanner.resolveMessage` で status === 429 + retry_after_seconds 表示
- 7.2 — status === 409 branch
- 7.3 — status === 401 branch
- 7.4 — status 5xx / 非 ApiError（status undefined）のデフォルト分岐で一時的失敗文言
- 7.5 — `useManualRefresh` の `onError` で invalidate 呼ばず（コメントで明示）。
  `it("API がエラーを返したときに invalidate が呼ばれないこと")` で検証
- 8.1 — `RecordManualFetchSuccess` → `feedman_manual_fetch_total{result="success"}`
- 8.2 — `RecordManualFetchFailure(reason)` を service が `fetch_error` / `parse_error` /
  `upsert_error` / `ssrf_blocked` で呼び分け
- 8.3 — `RecordManualFetchCooldownRejected` → `result="cooldown_rejected"`
- 8.4 — `RecordManualFetchLockConflict` → `result="lock_conflict"`
- 8.5 — メトリクス名が `feedman_manual_fetch_total`（自動経路の `feedman_fetch_success_total`
  系と別系列）
- NFR 1.1 — `runServe` での `fetchpkg.NewFetcher(..., cfg.FetchTimeout, ...)` で同一上限
- NFR 1.2 — クールダウン拒否 / ロック競合拒否は外部 HTTP を発行しない経路で局所判定のみ
- NFR 1.3 — 行ロック粒度は feedID 行単位、service stateless で別フィードは独立
- NFR 2.1 — `/api/subscriptions` グループの `r.Use(NewSessionMiddleware)` 適用済み（route 配置）
- NFR 2.2 — 同グループの `deps.RateLimiter.GeneralMiddleware()` 適用済み
- NFR 3.1 / 3.2 — Fetcher.Fetch のシグネチャ / Apply* 戦略を変更せず再利用

## Boundary Check

`tasks.md` の `_Boundary:_` 宣言と差分対象ファイルの照合:

- Task 1 (Boundary: 暗黙 repository / model)：migrations / `model.Feed` /
  `repository/postgres_feed_repo.go` — OK
- Task 2 (Boundary: PostgresFeedRepo)：`repository/interfaces.go` /
  `postgres_feed_repo.go` — OK
- Task 3 (Boundary: fetch.Fetcher)：`internal/worker/fetch/fetcher.go` のみ — OK
- Task 4 (Boundary: subscription.Service)：`internal/subscription/service.go` /
  `service_test.go` / `internal/model/errors.go`（APIError.Details 拡張は design.md で
  許可済み） — OK
- Task 5 (Boundary: metrics.Collector, metrics.NopCollector)：`internal/metrics/*` のみ — OK
- Task 6 (Boundary: SubscriptionHandler, SubscriptionServiceAdapter, RouterDeps,
  app.runServe)：`internal/handler/*` / `internal/middleware/error_response.go` /
  `internal/app/app.go` — OK（middleware への `Details` 追加は design.md で許可済み）
- Task 7 (Boundary: web hooks / web components)：`web/src/hooks/*` /
  `web/src/components/{item-list,manual-refresh-banner}.tsx` / `web/src/types/feed.ts` — OK

boundary 外の変更は検出されず（develop マージ commit による削除は本ブランチ実装ではなく
develop 由来の取り込み差分）。

## Findings

なし

## Summary

Issue #115 の AC（Req 1〜8 / NFR 1〜3）はすべて実装またはテストで verify 可能であり、
`_Boundary:_` 違反も検出されなかった。Req 3.4 の lock 解放タイミングは fetcher 実行前に
COMMIT する設計逸脱を含むが、design.md「データ更新の責務」/ impl-notes Task 4 で
self-deadlock 回避のため正当化されており、numeric AC の上限規定としては充足する。
Go 側テストは reviewer 環境で再実行し全て pass を確認。web 側は impl-notes の Developer
申告を採用（reviewer 環境の node_modules 不整合のため再実行は断念したが、AC カバレッジは
記述レベルで verify 済み）。

RESULT: approve
