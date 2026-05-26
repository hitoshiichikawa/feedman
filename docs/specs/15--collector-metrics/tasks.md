# Implementation Plan

- [x] 1. NopCollector と設定値（信頼 CIDR / メトリクスポート）の追加
- [x] 1.1 NopCollector を追加する (P)
  - `internal/metrics/nop.go` に `MetricsCollector` の no-op 実装 `NopCollector` を追加
  - 6 メソッドすべてを空実装。既存 `metrics.go` は変更しない（追加のみ）
  - 6 メソッドが panic せず副作用なく呼べる単体テストを追加
  - _Requirements: 5.1, NFR 1.2_
  - _Boundary: NopCollector_
- [x] 1.2 Config に信頼 CIDR とメトリクスポートを追加する (P)
  - `internal/config/config.go` の `Config` に `TrustedCIDRs []string` / `MetricsPort string` を追加
  - `Load()` で `METRICS_TRUSTED_CIDRS`（カンマ区切り、未設定時は空スライス）と `METRICS_PORT`（既定 9090）を読み込む
  - 未設定時に `TrustedCIDRs` を空のまま保持し、検証はミドルウェアに委譲する（NFR 2.1 は後続タスクで担保）
  - _Requirements: 4.1, NFR 2.1_
  - _Boundary: Config_

- [ ] 2. 信頼 CIDR ミドルウェアの実装
- [x] 2.1 TrustedCIDRMiddleware を実装する (P)
  - `internal/middleware/trusted_cidr.go` に `NewTrustedCIDRMiddleware(cidrs []string) func(next http.Handler) http.Handler` を追加（既存ミドルウェアのコンストラクタ関数パターンに準拠）
  - 起動時に `net.ParseCIDR` で `[]*net.IPNet` を構築し、不正な CIDR 文字列はスキップしてログ出力
  - リクエスト時に `r.RemoteAddr` から host を抽出して `net.ParseIP`、いずれかの CIDR に含まれれば next、なければ 403
  - 信頼 CIDR が空（未設定）の場合は全拒否（403）に倒す。`X-Forwarded-For` は信頼しない
  - 拒否時は next を呼ばず `http.Error(w, "forbidden", 403)` で即終了し、メトリクス本文を含めない
  - 単体テスト: 範囲内 200 / 範囲外 403 / 空 CIDR 全拒否 / 不正 CIDR スキップ / 拒否時にメトリクス文字列を含まない
  - _Requirements: 4.1, 4.2, 4.3, NFR 2.1, NFR 2.2_
  - _Boundary: TrustedCIDRMiddleware_

- [ ] 3. フェッチャー層へのメトリクス記録の組み込み
- [ ] 3.1 Fetcher に WithMetrics option とメトリクス記録を追加する
  - `internal/worker/fetch/fetcher.go` の `Fetcher` に `metrics metrics.MetricsCollector` フィールドを追加
  - `FetcherOption` 型と `WithMetrics(c)` を定義し、`NewFetcher(...)` の末尾に `opts ...FetcherOption` を追加（既存 7 引数 call site は不変）
  - option 未指定時は `NopCollector{}` を既定値にする（nil 安全）
  - `Fetch` 内に記録を挿入: レスポンス受信時 `RecordHTTPStatus`、200+UPSERT 成功と 304 で `RecordFetchSuccess`、SSRF/HTTP/stop/backoff 失敗で `RecordFetchFailure`、パース失敗で `RecordParseFailure`+`RecordFetchFailure`、終了時 `RecordFetchLatency`
  - 既存のフェッチ判定ロジック・フィード状態更新は変更しない
  - 結合テスト: テスト用 Collector を WithMetrics で注入し、成功/失敗/パース失敗/ステータス/レイテンシの記録呼び出しを検証
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_
  - _Boundary: Fetcher_

- [ ] 4. UPSERT サービス層へのメトリクス記録の組み込み
- [ ] 4.1 ItemUpsertService に WithMetrics option とアップサート件数記録を追加する
  - `internal/item/upsert.go` の `ItemUpsertService` に `metrics metrics.MetricsCollector` フィールドを追加
  - `UpsertOption` 型と `WithMetrics(c)` を定義し、`NewItemUpsertService(...)` の末尾に `opts ...UpsertOption` を追加（既存 2 引数 call site は不変）
  - option 未指定時は `NopCollector{}` を既定値にする
  - `UpsertItems` の `BulkUpsert` 成功後に `RecordItemsUpserted(inserted + updated)` を記録。エラー時（ロールバック）は記録しない
  - 既存の同一性判定・dedup・bulk upsert ロジックは変更しない
  - 結合テスト: テスト用 Collector を注入し、成功時の件数加算とエラー時の非記録を検証
  - _Requirements: 2.6_
  - _Boundary: ItemUpsertService_

- [ ] 5. serve プロセスへの /metrics 条件登録
- [ ] 5.1 RouterDeps に MetricsHandler/MetricsMiddleware を追加し /metrics を条件登録する
  - `internal/handler/router.go` の `RouterDeps` に `MetricsHandler http.Handler`（nil 可）と `MetricsMiddleware func(http.Handler) http.Handler`（nil 可）を追加
  - `MetricsHandler` が非 nil のとき、認証不要グループに `r.With(deps.MetricsMiddleware).Handle("/metrics", deps.MetricsHandler)` を登録
  - `MetricsHandler` が nil の場合は登録せず既存ルーティングを完全に不変に保つ（後方互換）
  - 結合テスト: 非 nil で範囲内 200 / 範囲外 403、nil で `/health` `/auth/*` `/api/*` のルーティング・レスポンスが不変
  - _Requirements: 1.1, 5.1_
  - _Boundary: Router, TrustedCIDRMiddleware_
  - _Depends: 2.1_

- [ ] 6. worker プロセスへの metrics listener と全レイヤ wiring
- [ ] 6.1 worker 用 metrics listener ヘルパを追加する
  - `internal/app/metrics_listener.go` に `startWorkerMetricsListener(ctx, addr, gatherer, cidrs)` を追加
  - worker registry を gatherer として `metrics.SetupMetricsRoute` を取得し、`NewTrustedCIDRMiddleware` でラップした `http.Server` を goroutine で起動
  - ctx キャンセルで `server.Shutdown` による graceful stop（goroutine リーク防止）、起動失敗はエラーログにとどめ worker 本体を落とさない
  - 結合テスト: フェッチ成功記録後に `/metrics` 応答へ `feedman_fetch_success_total` 増加が反映され、起動直後の未記録でも 200 と全 6 系列名を含む
  - _Requirements: 1.2, 1.3, 3.1, 3.2, 3.3, 5.3, NFR 1.1_
  - _Boundary: Worker MetricsListener, TrustedCIDRMiddleware_
  - _Depends: 1.1, 2.1_
- [ ] 6.2 runServe / runWorker で registry・Collector を生成し全レイヤを wiring する
  - `internal/app/app.go` の `runServe` で serve 専用 `prometheus.NewRegistry()` + `NewCollector` を生成し、CIDR mw と `SetupMetricsRoute` を `RouterDeps` に注入
  - `runWorker` で worker 専用 registry + Collector を生成し、`fetchpkg.NewFetcher` と `item.NewItemUpsertService` に `WithMetrics` で注入、`startWorkerMetricsListener` を ctx 付きで起動
  - serve の既存 server timeout（Read/Write/Idle）は変更しない（Requirement 5.2）
  - _Requirements: 1.1, 1.2, 2.6, 3.1, 5.1, 5.2_
  - _Boundary: App Wiring, Worker MetricsListener, Fetcher, ItemUpsertService, Router_
  - _Depends: 1.1, 1.2, 2.1, 3.1, 4.1, 5.1, 6.1_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の構造化ブロックで宣言する。

<!-- stage-a-verify -->
```sh
go build ./... && go test ./... && go vet ./...
```
