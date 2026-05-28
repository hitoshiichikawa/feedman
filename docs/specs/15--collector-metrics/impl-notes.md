# 実装ノート（Issue #15: メトリクス Collector と /metrics エンドポイントの本番組み込み）

## 受入基準とテストの対応（本起動分: task 1）

| Requirement ID | 担保するテスト |
|---|---|
| 5.1（後方互換維持 / NopCollector が既定値として安全） | `internal/metrics/nop_test.go` の `TestNopCollector_ImplementsMetricsCollector` / `TestNopCollector_MethodsDoNotPanic` / `TestNopCollector_ZeroValueIsUsable` |
| NFR 1.2（既存テストスイートを失敗させない / nil 安全） | 同上 + `go test ./...` 全 green を確認 |
| 4.1（信頼 CIDR の設定読み込み） | `internal/config/config_test.go` の `TestLoad_MetricsTrustedCIDRs`（単一/複数/空白トリム/空要素除外/不正値保持/未設定）、`TestLoad_MetricsPort` |
| NFR 2.1（未設定時は空のまま保持し検証はミドルウェアに委譲） | `TestLoad_MetricsTrustedCIDRs` の「未設定（空文字）のとき空スライスを保持する」「空白のみのとき空スライスを保持する」、`TestLoad_DefaultValues`（Metrics defaults） |

> NFR 2.1 の「安全側（アクセス拒否）に倒す」挙動自体は後続 task 2.1（TrustedCIDRMiddleware）で
> 担保する。本 task では config パース段階で空スライスを保持することまでを検証する（tasks.md L13
> の方針に準拠）。

## Implementation Notes

### Task 1

- **採用方針**: design.md L263-273 のシグネチャに準拠した no-op 実装 `NopCollector` と、config への
  `TrustedCIDRs` / `MetricsPort` フィールド追加を、既存実装（`metrics.go` の interface / `config.go` の
  env ヘルパパターン）に非破壊で追加した。
- **重要な判断**:
  - `NopCollector` は value receiver（`func (NopCollector) ...`）で実装し、ゼロ値 `NopCollector{}` を
    そのまま既定値として使える（nil 安全・ポインタ不要）形にした。既存 interface の
    `RecordFetchFailure(feedID string, reason string)` と design の `(feedID, reason string)` は
    同一シグネチャのため整合する。
  - CIDR のカンマ区切りパースは config 内に `parseCommaSeparated` ヘルパとして切り出し、TrimSpace +
    空要素除外を行う。不正 CIDR の判定はパース段階では行わず（tasks.md L13 / NFR 2.1 の方針どおり）
    後続のミドルウェアに委譲するため、不正文字列もそのまま保持する（テストで明示）。
  - `MetricsPort` は既存の `getEnvString("METRICS_PORT", "9090")` ヘルパを流用し、既定 "9090" とした。
- **残存課題**: なし（task 2 以降の TrustedCIDRMiddleware / Fetcher・Upsert への WithMetrics 注入 /
  ルーター登録 / wiring は別 task で実装される。本 task の `NopCollector` と config フィールドは
  それらの前提として提供済み）。

## 是正対応（Reviewer round=1 reject を受けた追加実装: task 2〜6）

Reviewer round=1 が「`/metrics` 本体・メトリクス記録組み込み・worker listener・CIDR 制限が未実装」
（Finding 1〜5）として reject したため、tasks.md の task 2〜6 を依存順に実装した。task 1
（`NopCollector` / config フィールド）は前提として温存し変更していない。

### Reviewer Finding への対応サマリ

| Finding | 内容 | 対応 |
|---|---|---|
| Finding 1（Target 1.1/1.2/1.3） | `/metrics` エンドポイント本体未実装 | task 5.1（serve ルーター条件登録）+ task 6.1/6.2（worker listener + wiring）を実装。`internal/handler/router_metrics_test.go` / `internal/app/metrics_listener_test.go` で結合検証 |
| Finding 2（Target 2.1〜2.6） | フェッチ/UPSERT メトリクス記録組み込み未実装 | task 3.1（Fetcher.WithMetrics + 各分岐の記録挿入）+ task 4.1（ItemUpsertService.WithMetrics + 件数記録）を実装。モック Collector 注入テストで各記録呼び出しを検証 |
| Finding 3（Target 3.1/3.2/3.3） | worker 専用 registry + listener のスクレイプ経路未実装 | task 6.1/6.2 を実装。`TestStartWorkerMetricsListener_FetchSuccessReflected` でフェッチ成功記録が同一プロセスの `/metrics` 応答へ反映されることを検証 |
| Finding 4（Target 4.1〜4.3） | 信頼 CIDR アクセス制限の実挙動未実装 | task 2.1（`internal/middleware/trusted_cidr.go`）を実装。範囲内 200 / 範囲外・未設定・パース不能 403 / 不正 CIDR スキップ / 拒否時本文非包含を単体テストで検証 |
| Finding 5（Target 5.1〜5.3, NFR 1.1/2.1/2.2） | 既存ルーティング不変性・既存 timeout 維持・起動直後 200・未設定時 403・拒否時本文非包含の実装/テスト不足 | router の nil 分岐テスト（既存ルート不変）、serve timeout 不変（app.go で server timeout を変更していない）、起動直後 200 と系列出力、空 CIDR 全拒否、拒否時本文非包含を各テストで担保 |

### 追加した実装・テストと対応 AC のマッピング（task 2〜6）

| Requirement ID | 実装 | 担保するテスト |
|---|---|---|
| 1.1（信頼 CIDR 内から Prometheus 形式応答） | `router.go`（serve 条件登録）/ `metrics_listener.go`（worker） | `TestNewRouter_Metrics_NonNilHandler_InRange_Returns200` / `TestStartWorkerMetricsListener_StartupExposesSeries` |
| 1.2（起動完了で即スクレイプ可能） | `metrics_listener.go` / `app.go` の listener 起動 | `TestStartWorkerMetricsListener_StartupExposesSeries`（起動直後 200） |
| 1.3（全 6 系列を応答に含める） | `app.go` の `NewCollector(registry)` 登録 | `TestStartWorkerMetricsListener_StartupExposesSeries`（ラベルなし 5 系列）+ `TestStartWorkerMetricsListener_HTTPStatusSeriesAppearsAfterRecord`（`feedman_http_status_total`、後述「確認事項」参照） |
| 2.1（フェッチ成功で成功数増加） | `fetcher.go` 200/304 成功時 `RecordFetchSuccess` | `TestFetcher_Fetch_Metrics_Success200` / `TestFetcher_Fetch_Metrics_304Success` |
| 2.2（フェッチ失敗で失敗数増加） | `fetcher.go` SSRF/HTTP/stop/backoff/parse/upsert 失敗時 `RecordFetchFailure` | `TestFetcher_Fetch_Metrics_HTTPFailure` / `TestFetcher_Fetch_Metrics_ParseFailure` / `TestFetcher_Fetch_Metrics_HTTPStatusRecordedOnBackoff` |
| 2.3（パース失敗でパース失敗数増加） | `fetcher.go` parse 失敗時 `RecordParseFailure` | `TestFetcher_Fetch_Metrics_ParseFailure` |
| 2.4（HTTP ステータス別レスポンス数増加） | `fetcher.go` レスポンス受信直後 `RecordHTTPStatus` | `TestFetcher_Fetch_Metrics_Success200`（200）/ `TestFetcher_Fetch_Metrics_HTTPStatusRecordedOnBackoff`（500） |
| 2.5（フェッチ完了で所要時間記録） | `fetcher.go` defer `RecordFetchLatency` | `TestFetcher_Fetch_Metrics_Success200` / `TestFetcher_Fetch_Metrics_HTTPFailure`（失敗時も defer で記録） |
| 2.6（UPSERT 完了で件数加算） | `upsert.go` BulkUpsert 成功後 `RecordItemsUpserted(inserted+updated)` | `TestUpsertItems_Metrics_RecordsUpsertedCount` / `TestUpsertItems_Metrics_NotRecordedOnError` / `TestUpsertItems_Metrics_EmptyInputNotRecorded` |
| 3.1（フェッチプロセスのメトリクスをスクレイプ可能に） | `metrics_listener.go` worker 独立 listener | `TestStartWorkerMetricsListener_StartupExposesSeries` |
| 3.2（成功数増加分を同一プロセス応答に反映） | worker registry 共有（`app.go`） | `TestStartWorkerMetricsListener_FetchSuccessReflected` |
| 3.3（別プロセス応答に依存せず観測可能） | serve/worker で独立 registry（`app.go`） | `TestStartWorkerMetricsListener_FetchSuccessReflected`（worker 単独 registry で反映） |
| 4.1（CIDR 範囲外は 403） | `trusted_cidr.go` | `TestTrustedCIDRMiddleware`（範囲外 403 / 空 CIDR 全拒否 / パース不能拒否） |
| 4.2（CIDR 範囲内はメトリクス応答） | `trusted_cidr.go` | `TestTrustedCIDRMiddleware`（範囲内 200） |
| 4.3（IP のみで判定・秘匿情報不要） | `trusted_cidr.go`（`RemoteAddr` のみ、X-Forwarded-For 非信頼） | `TestTrustedCIDRMiddleware`（ヘッダ要求なしで判定） |
| 5.1（既存ルーティング・レスポンス維持） | `router.go` MetricsHandler nil 分岐 | `TestNewRouter_Metrics_NilHandler_NotRegistered` / `TestNewRouter_Metrics_NilHandler_ExistingRoutesUnchanged` / `TestNewRouter_Metrics_NonNilHandler_ExistingRoutesUnchanged` + 既存 `router_full_test.go` 全 green |
| 5.2（タイムアウト設定維持） | `app.go` runServe の server timeout を変更せず | `go build ./...` + 既存 serve テスト（timeout 値の変更なし） |
| 5.3（起動直後の未記録でも正常応答） | `metrics_listener.go` / promhttp 既定 | `TestStartWorkerMetricsListener_StartupExposesSeries`（200） |
| NFR 1.1（未記録でも 2xx） | promhttp 既定の空 gather | `TestStartWorkerMetricsListener_StartupExposesSeries` |
| NFR 1.2（既存テストスイートを失敗させない） | 全 option/nil 分岐の後方互換 | `TestNewFetcher_DefaultMetricsIsNopAndNilSafe` / `TestNewItemUpsertService_DefaultMetricsIsNopAndNilSafe` + `go test ./...` 全 green |
| NFR 2.1（CIDR 未設定時は全拒否） | `trusted_cidr.go` 空 nets で deny | `TestTrustedCIDRMiddleware`（空 CIDR 全拒否） |
| NFR 2.2（拒否時に本文非包含） | `trusted_cidr.go` next 未呼び出し | `TestTrustedCIDRMiddleware`（拒否時本文にメトリクス文字列なし）/ `TestNewRouter_Metrics_NonNilHandler_OutOfRange_Returns403` / `TestStartWorkerMetricsListener_OutOfRangeForbidden` |

### 重要な実装判断

- **304 を成功扱い**: `Fetch` の 304 NotModified 分岐は「変更なしで取得成功」とみなし
  `RecordFetchSuccess` を記録する（design.md L360-361 / tasks.md L33 の方針に準拠）。
- **失敗系の記録範囲**: task / design が明示した SSRF / HTTP（client.Do）/ stop / backoff / parse に加え、
  body 読み取り失敗・予期しないステータス default・UPSERT 失敗・状態更新失敗の各 return path にも
  `RecordFetchFailure` を挿入した（いずれも Req 2.2「フェッチ失敗」に該当するため。成功と失敗の二重計上が
  起きないよう、各 return は単一の終端記録に閉じている）。`RecordFetchLatency` は defer で全 return を
  通過時に 1 度だけ記録する。
- **worker listener の graceful stop**: `startWorkerMetricsListener` は ctx キャンセルで
  `server.Shutdown` する watcher goroutine と `ListenAndServe` 本体 goroutine の 2 本を起動し、
  ctx キャンセルで listener を確実に停止する（goroutine リーク防止）。起動失敗（`ListenAndServe` が
  `http.ErrServerClosed` 以外）はエラーログにとどめ worker 本体を落とさない。
- **RouterDeps の nil 分岐**: `MetricsHandler` が nil なら `/metrics` を登録せず既存ルーティングを完全に
  不変に保つ。`MetricsHandler` 非 nil かつ `MetricsMiddleware` が nil のケースは chi の `With(nil)` が
  panic するため、素通しラッパ（`func(next) http.Handler { return next }`）にフォールバックして安全側に
  倒した（実運用の app.go は常に CIDR mw を渡すため通常は通らない経路）。
- **後方互換の functional options**: `NewFetcher` / `NewItemUpsertService` とも既存の位置引数を不変に保ち、
  末尾に `opts ...Option` を追加。未指定時は `metrics.NopCollector{}` を既定値とし、既存 50+ call site は
  コンパイル・挙動とも不変（NFR 1.2）。

## 確認事項（レビュワー判断ポイント）

- **`feedman_http_status_total` の起動直後出力（Req 1.3 の解釈）**: 既存 `internal/metrics/metrics.go`
  （変更禁止）では `feedman_http_status_total` をラベル付き `CounterVec`（`status_code` ラベル）として
  定義している。Prometheus テキスト公開形式の仕様上、`CounterVec` はラベル値が 1 度も記録されるまで
  HELP/TYPE/系列のいずれも出力されない。したがって **起動直後（未記録）の `/metrics` 応答には
  ラベルなし 5 系列のみが現れ、`feedman_http_status_total` はステータス記録後に初めて現れる**。
  本実装はこれを正直に検証する 2 本のテスト（`...StartupExposesSeries` で 5 系列 / `...HTTPStatusSeriesAppearsAfterRecord`
  で 6 系列目）に分割した。Req 1.3「起動直後に全 6 系列名を含む」を厳密に満たすには `metrics.go` 側で
  起動時に `RecordHTTPStatus` 相当の初期化（ゼロ値ラベルのプリセット）が必要だが、これは
  「既存メトリクス実装本体は一切変更しない」（design.md L18-19 / Out of Scope）と矛盾するため
  **本実装では metrics.go を変更していない**。Req 1.3 の厳密充足を要する場合は、metrics.go の初期化を
  許容する設計変更（別 Issue）が必要であり、Architect / PM への差し戻しが妥当と判断する。
- 上記以外に requirements.md / design.md / tasks.md との矛盾は検出していない。
- serve プロセスは本機能ではフェッチ系記録経路を持たないため、serve の `/metrics` は初期値（0）公開と
  なる（design.md の Architecture Decision 表に明記された想定どおり）。フェッチ系メトリクスの主役は
  worker 側 listener。

STATUS: complete
