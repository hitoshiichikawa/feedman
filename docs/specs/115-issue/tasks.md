# Implementation Plan

- [x] 1. DB マイグレーション: feeds.last_successful_fetch_at カラム追加
- [x] 1.1 up / down マイグレーションファイルを新規作成
  - `internal/database/migrations/20260528120000_add_feeds_last_successful_fetch_at.up.sql` を作成し、
    `ALTER TABLE feeds ADD COLUMN last_successful_fetch_at TIMESTAMPTZ NULL` を記述（バックフィルなし）
  - `internal/database/migrations/20260528120000_add_feeds_last_successful_fetch_at.down.sql` を作成し、
    `ALTER TABLE feeds DROP COLUMN last_successful_fetch_at` を記述
  - `model.Feed` 構造体（`internal/model/feed.go`）に `LastSuccessfulFetchAt *time.Time` を追加（nil 表現用ポインタ）
  - `internal/repository/postgres_feed_repo.go` の既存 SELECT 句（`FindByID` / `FindByFeedURL` / `ListDueForFetch`）に
    `last_successful_fetch_at` を加え、`sql.NullTime` で Scan して `feed.LastSuccessfulFetchAt` に詰める
  - 既存テスト（`internal/repository/postgres_feed_repo_test.go` / `internal/repository/postgres_subscription_repo_db_test.go`）の
    SELECT 期待値・Scan 行数が変わるため整合させる
  - _Requirements: 2.4_

- [x] 2. Repository 層: 行ロック取得 + last_successful_fetch_at 更新メソッド追加
- [x] 2.1 LockFeedForUpdateNowait / UpdateLastSuccessfulFetchAt の interface 定義と PostgreSQL 実装
  - `internal/repository/interfaces.go` の `FeedRepository` interface に以下 2 メソッドを追加:
    `LockFeedForUpdateNowait(ctx, tx *sql.Tx, feedID string) (*model.Feed, error)`
    `UpdateLastSuccessfulFetchAt(ctx, feedID string, at time.Time) error`
  - `internal/repository/postgres_feed_repo.go` に上記実装を追加。`LockFeedForUpdateNowait` は
    `SELECT ... FROM feeds WHERE id = $1 FOR UPDATE NOWAIT` を発行し、PG ErrCode `55P03`（lock_not_available）を
    `errors.Is` 判定可能な sentinel `ErrFeedLocked` に正規化する（`lib/pq.Error.Code` を inspect）
  - `UpdateLastSuccessfulFetchAt` は `UPDATE feeds SET last_successful_fetch_at = $2, updated_at = now() WHERE id = $1` を発行
  - 単体テスト（`internal/repository/postgres_feed_repo_test.go`）に追加: ロック成功時の Feed 返却、競合時の
    `errors.Is(err, ErrFeedLocked)`、UpdateLastSuccessfulFetchAt の冪等性
  - _Requirements: 3.1, 3.2, 3.4, 1.2_
  - _Boundary: PostgresFeedRepo_
  - _Depends: 1.1_

- [ ] 3. Worker fetcher の成功経路に UpdateLastSuccessfulFetchAt 反映
- [x] 3.1 既存 Fetcher.Fetch の ApplySuccess 直後に成功時刻を記録 (P)
  - `internal/worker/fetch/fetcher.go` の 2 箇所（304 Not Modified パス / 200 OK 成功パス）の `ApplySuccess` 呼び出し直後に
    `f.feedRepo.UpdateLastSuccessfulFetchAt(ctx, feed.ID, time.Now())` を 1 行追加する
  - エラーが発生した場合はログ警告のみ（成功時刻の記録失敗で fetch 自体は成功扱いを維持）
  - `internal/worker/fetch/fetcher_test.go` で 200 / 304 成功時に `UpdateLastSuccessfulFetchAt` が
    呼ばれることを mock repository で検証、エラーパス（ApplyBackoff / ApplyStopFeed）では呼ばれないことも検証
  - _Requirements: 2.4_
  - _Boundary: fetch.Fetcher_
  - _Depends: 2.1_

- [ ] 4. Service 層: subscription.Service に ManualFetch メソッド追加
- [ ] 4.1 ManualFetch のオーケストレーション実装（認可 / 行ロック / クールダウン判定 / fetcher 呼び出し / メトリクス）
  - `internal/model/errors.go` に `ErrCodeFeedFetchInProgress = "FEED_FETCH_IN_PROGRESS"` /
    `ErrCodeFeedCooldown = "FEED_COOLDOWN"` 定数と、`NewFeedFetchInProgressError()` /
    `NewFeedCooldownError(retryAfterSeconds int)` 生成関数を追加。後者は `Details["retry_after_seconds"] = int` をセット
  - `internal/model/errors.go` の `APIError` 構造体に `Details map[string]any` フィールドを追加（既存 4 フィールドは不変）
  - `internal/subscription/service.go` の `Service` 構造体に `feedFetcher fetch.FeedFetcherService` /
    `txBeginner repository.TxBeginner` / `metricsCollector metrics.MetricsCollector` 依存を追加し、
    `NewService` のシグネチャを拡張
  - `ManualFetch(ctx, userID, subscriptionID string) (*SubscriptionInfo, error)` を実装。フロー:
    (1) `subRepo.FindByID` で subID 取得・UserID 一致確認（不一致は SUBSCRIPTION_NOT_FOUND）
    (2) `txBeginner.BeginTx` で tx 開始（`defer tx.Rollback()`）
    (3) `feedRepo.LockFeedForUpdateNowait(tx, feedID)` 実行。`ErrFeedLocked` なら lock_conflict メトリクス記録後
        `FEED_FETCH_IN_PROGRESS` を返す
    (4) クールダウン判定: `feed.LastSuccessfulFetchAt != nil && time.Now().Sub(*feed.LastSuccessfulFetchAt) < 10*time.Minute`
        ならば残り秒数を算出して `FEED_COOLDOWN`、cooldown_rejected メトリクスを記録、tx は COMMIT で解放
    (5) クールダウン外: `feedFetcher.Fetch(ctx, feed)` を呼ぶ
    (6) `Fetch` が nil を返し、かつ `feed.FetchStatus == FetchStatusActive && feed.ConsecutiveErrors == 0` の場合のみ
        成功と判定し `feedRepo.UpdateLastSuccessfulFetchAt(feed.ID, time.Now())` を呼ぶ + success メトリクス記録
    (7) `Fetch` がエラーを返した場合は理由分類（fetch_error / parse_error / ssrf_blocked / upsert_error）で
        failure メトリクス記録し、対応する APIError（FETCH_FAILED / PARSE_FAILED / etc）を返す
    (8) tx COMMIT 後、`ListByUserIDWithFeedInfo` 経由で最新 SubscriptionInfo を返す（既存 ResumeFetch と同パターン）
  - `internal/subscription/service_test.go` に追加: クールダウン境界判定（9m59s / 10m0s / nil の 3 ケース）、
    認可失敗（subID 不存在 / UserID 不一致）、行ロック競合、fetch 成功 / 失敗のメトリクス記録、別フィードへの並行性
  - _Requirements: 1.1, 1.2, 1.5, 1.6, 2.1, 2.2, 2.3, 2.4, 2.5, 3.1, 3.2, 3.3, 3.4, 4.1, 4.3, NFR 1.3, NFR 3.1, NFR 3.2_
  - _Boundary: subscription.Service_
  - _Depends: 2.1, 3.1_

- [ ] 5. Metrics: feedman_manual_fetch_total カウンタ追加
- [ ] 5.1 MetricsCollector interface に 4 メソッド追加 + Collector / NopCollector 実装 (P)
  - `internal/metrics/metrics.go` の `MetricsCollector` interface に以下 4 メソッドを追加:
    `RecordManualFetchSuccess()` / `RecordManualFetchFailure(reason string)` /
    `RecordManualFetchCooldownRejected()` / `RecordManualFetchLockConflict()`
  - `Collector` 実装に `manualFetchTotal *prometheus.CounterVec`（label `result`）を新設し
    `reg.MustRegister` に追加。各メソッドが対応 label で `Inc()` する
  - `internal/metrics/nop.go` の `NopCollector` に 4 メソッドの no-op 実装を追加
  - `internal/metrics/metrics_test.go` に新規メトリクスの増分テストを追加（label 別に値が正しく加算されること）
  - `internal/handler/feed_handler_test.go` 等で使われている mock collector があれば 4 メソッドを追加
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_
  - _Boundary: metrics.Collector, metrics.NopCollector_

- [ ] 6. Handler / Router: POST /api/subscriptions/{id}/fetch 追加
- [ ] 6.1 SubscriptionServiceInterface 拡張 + Handler.ManualFetch + Router 配線 + Error マッピング
  - `internal/handler/subscription_handler.go` の `SubscriptionServiceInterface` に
    `ManualFetch(ctx, userID, subscriptionID string) (*subscriptionResponse, error)` を追加
  - 同ファイルに `(*SubscriptionHandler).ManualFetch` ハンドラを実装。`ResumeFetch` と同パターンで
    `UserIDFromContext` → `chi.URLParam(r, "id")` → `h.service.ManualFetch(ctx, userID, subID)` → `handleServiceError`
  - `internal/handler/service_adapter.go` の `SubscriptionServiceAdapter` に `ManualFetch` アダプタを追加
    （`subscription.Service.ManualFetch` を呼んで `*SubscriptionInfo` → `*subscriptionResponse` 変換、既存 `toSubscriptionResponse` を利用）
  - `internal/handler/router.go` の `r.Route("/api/subscriptions", ...)` 内 `r.Route("/{id}", ...)` に
    `r.Post("/fetch", subHandler.ManualFetch)` を 1 行追加
  - `internal/handler/feed_handler.go` の `mapAPIErrorToHTTPStatus` switch 文に
    `case ErrCodeFeedFetchInProgress: return http.StatusConflict`
    `case ErrCodeFeedCooldown: return http.StatusTooManyRequests` を追加
  - `internal/middleware` の `WriteErrorResponse`（既存）が JSON 出力で `APIError.Details` を含むよう拡張する
    （`Details == nil` のときは出力しない omitempty 相当）
  - `internal/handler/subscription_handler_test.go` に追加: ManualFetch ハンドラの 200 / 401 / 404 / 409 / 429 / 502 ケース
  - `internal/handler/feed_handler_test.go` の `mapAPIErrorToHTTPStatus` table テストに新規 2 ケースを追加
  - `internal/app/app.go` の `runServe` に worker と同様の依存配線を追加: `ssrfGuard` / `sanitizer` /
    `upsertSvc` / `fetcher`（`fetchpkg.NewFetcher`）/ `serveCollector := metrics.NewCollector(serveRegistry)` の戻り値受け取り。
    `subscription.NewService` 呼び出しの引数を拡張（fetcher / txBeginner / serveCollector を追加）
  - `internal/app/app_test.go` / `internal/app/withdraw_wiring_test.go` で依存配線シグネチャ変更の整合
  - _Requirements: 1.1, 1.3, 1.4, 1.5, 1.6, 1.7, 2.1, 2.2, 3.2, 3.3, 7.1, 7.2, 7.3, 7.4, NFR 1.1, NFR 2.1, NFR 2.2_
  - _Boundary: SubscriptionHandler, SubscriptionServiceAdapter, RouterDeps, app.runServe_
  - _Depends: 4.1, 5.1_

- [ ] 7. Frontend: useManualRefresh フック + ManualRefreshButton / ManualRefreshBanner UI 追加
- [ ] 7.1 useManualRefresh フック実装 (P)
  - `web/src/hooks/use-manual-refresh.ts` を新規作成。`useMutation<void, ApiError, string>` で
    `apiClient.post(\`/api/subscriptions/${subscriptionId}/fetch\`)` を呼ぶ
  - `onSuccess`: `queryClient.invalidateQueries({ queryKey: ["items", feedId] })` および
    `queryClient.invalidateQueries({ queryKey: ["feeds"] })` を発行
  - `onError`: invalidate しない（Req 7.5）。エラーは consumer 側で `mutation.error` 経由で読む
  - `web/src/hooks/use-manual-refresh.test.tsx` を新規作成し、成功時 invalidate 2 種、エラー時非 invalidate を検証
  - `web/src/types/feed.ts` に `ManualFetchErrorBody` 型を追加（`{ error: { code, message, category, action, details?: { retry_after_seconds?: number } } }`）
  - _Requirements: 5.3, 6.1, 6.2, 6.3, 7.5_
  - _Boundary: web hooks_

- [ ] 7.2 ManualRefreshButton + ManualRefreshBanner を ItemList に統合
  - `web/src/components/manual-refresh-banner.tsx` を新規作成。`error: ApiError | null` を受け取り、
    status / code 別の表示メッセージを返す純粋表示コンポーネント。429 のとき `details.retry_after_seconds` を本文に埋め込む
  - `web/src/components/item-list.tsx` を修正:
    - `ItemList` 内部に `ManualRefreshButton` を追加し、`<Tabs>` を `<div className="flex items-center justify-between">` でラップして filter tabs 右に配置
    - ボタンは `aria-label="フィードを更新"` / `aria-busy={isPending}` / `disabled={isPending}` を持ち、
      Lucide `RotateCw` アイコンを `cn("w-4 h-4", isPending && "animate-spin")` で表示（回転アニメーション）
    - subscriptionId は `useFeeds()` から `feedId === f.feed_id` で `find` して取得（既存 `Subscription.id`）
    - フィルタタブ群直下に `ManualRefreshBanner` を配置し、`error` プロップで表示制御（成功時は null）
  - `web/src/components/item-list.test.tsx` を修正・追加:
    - フィード選択時にボタンが描画されること
    - pending 時 disabled + animate-spin クラスが付くこと
    - 429 / 409 / 401 / 5xx / ネットワークエラー時にバナーが各メッセージで表示されること
    - 成功時はバナーが表示されないこと
  - 未選択時（`feedId === null`）はそもそも `ItemList` が早期 return するため、ボタンの非表示は構造的に保証される（Req 5.2）
  - _Requirements: 5.1, 5.2, 5.4, 5.5, 5.6, 6.3, 7.1, 7.2, 7.3, 7.4_
  - _Boundary: web components_
  - _Depends: 7.1_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の構造化ブロックで宣言する。

<!-- stage-a-verify -->
```sh
go test ./... && cd web && npm test && npm run lint
```
