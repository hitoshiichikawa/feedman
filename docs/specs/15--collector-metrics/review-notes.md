# Review Notes

<!-- idd-claude:review round=2 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-15-impl--collector-metrics
- HEAD commit: 78f17940d6b7e188d55b138fdbd07392aeb2b33c
- Compared to: develop..HEAD

> 補足: `develop..HEAD` の diff には `.github/workflows/ci.yml` の削除や
> `docs/specs/51-ci-lint-npm-audit-eslint/` 配下の削除が現れるが、これらは
> 本ブランチの commit による変更ではなく（`git log develop..HEAD -- <paths>` が空）、
> base ブランチ `develop` が merge-base（3e99aea）より先行していることに起因する
> base 進行差分である（round 1 でも同様に確認済み）。本ブランチの実変更は Issue #15
> 関連ファイルに閉じており boundary 逸脱は無い。

## Verified Requirements

ROUND=1 で AC 未カバーだった Requirement 1〜5 / NFR が、task 2〜6 の追加実装で
カバーされたことを確認した。

- 1.1 — serve: `router.go` の `/metrics` 条件登録（CIDR mw 前段）+ `app.go` runServe の
  `MetricsHandler`/`MetricsMiddleware` 注入。worker: `metrics_listener.go`。
  `TestNewRouter_Metrics_NonNilHandler_InRange_Returns200` /
  `TestStartWorkerMetricsListener_StartupExposesSeries` で検証。
- 1.2 — `metrics_listener.go` / `app.go` で listener を起動完了時に公開。
  `TestStartWorkerMetricsListener_StartupExposesSeries`（起動直後 200）で検証。
- 1.3 — `app.go` の `NewCollector(registry)` で全 6 Collector を登録。
  `TestStartWorkerMetricsListener_StartupExposesSeries`（ラベルなし 5 系列）+
  `TestStartWorkerMetricsListener_HTTPStatusSeriesAppearsAfterRecord`（`feedman_http_status_total`）で
  6 系列を検証（CounterVec の出力仕様は後述）。
- 2.1 — `fetcher.go` 200/304 成功時 `RecordFetchSuccess`。
  `TestFetcher_Fetch_Metrics_Success200` / `TestFetcher_Fetch_Metrics_304Success`。
- 2.2 — `fetcher.go` SSRF/HTTP/stop/backoff/parse/upsert 失敗時 `RecordFetchFailure`。
  `TestFetcher_Fetch_Metrics_HTTPFailure` / `...ParseFailure` / `...HTTPStatusRecordedOnBackoff`。
- 2.3 — `fetcher.go` parse 失敗時 `RecordParseFailure`。`TestFetcher_Fetch_Metrics_ParseFailure`。
- 2.4 — `fetcher.go` レスポンス受信直後 `RecordHTTPStatus`。
  `TestFetcher_Fetch_Metrics_Success200`（200）/ `...HTTPStatusRecordedOnBackoff`（500）。
- 2.5 — `fetcher.go` defer `RecordFetchLatency`。
  `TestFetcher_Fetch_Metrics_Success200` / `...HTTPFailure`（失敗時も defer 記録）。
- 2.6 — `upsert.go` BulkUpsert 成功後 `RecordItemsUpserted(inserted+updated)`。
  `TestUpsertItems_Metrics_RecordsUpsertedCount` / `...NotRecordedOnError` / `...EmptyInputNotRecorded`。
- 3.1 — `metrics_listener.go` worker 独立 listener。`TestStartWorkerMetricsListener_StartupExposesSeries`。
- 3.2 — worker registry 共有（`app.go`）。`TestStartWorkerMetricsListener_FetchSuccessReflected`。
- 3.3 — serve/worker で独立 registry（`app.go`）。`TestStartWorkerMetricsListener_FetchSuccessReflected`。
- 4.1 — `trusted_cidr.go` 範囲外/未設定/パース不能 403。`TestTrustedCIDRMiddleware` /
  `TestNewRouter_Metrics_NonNilHandler_OutOfRange_Returns403` / `TestStartWorkerMetricsListener_OutOfRangeForbidden`。
- 4.2 — `trusted_cidr.go` 範囲内 next。`TestTrustedCIDRMiddleware`（範囲内 200, IPv6 含む）。
- 4.3 — `trusted_cidr.go` `RemoteAddr` のみで判定・秘匿情報不要。`TestTrustedCIDRMiddleware`。
- 5.1 — `router.go` `MetricsHandler` nil/非 nil 分岐。
  `TestNewRouter_Metrics_NilHandler_NotRegistered` / `...NilHandler_ExistingRoutesUnchanged` /
  `...NonNilHandler_ExistingRoutesUnchanged` + 既存 `internal/handler` 全 green。
- 5.2 — `app.go` runServe の server timeout（Read/Write/Idle）を変更していないことを diff で確認。
- 5.3 — `metrics_listener.go` / promhttp 既定。`TestStartWorkerMetricsListener_StartupExposesSeries`（200）。
- NFR 1.1 — promhttp 空 gather でも 200。`TestStartWorkerMetricsListener_StartupExposesSeries`。
- NFR 1.2 — option/nil 既定値。`TestNewFetcher_DefaultMetricsIsNopAndNilSafe` /
  `TestNewItemUpsertService_DefaultMetricsIsNopAndNilSafe` + `go test ./...` 全 green（reviewer 再実行で確認）。
- NFR 2.1 — `trusted_cidr.go` 空 nets で全拒否。`TestTrustedCIDRMiddleware`（空 CIDR 全拒否）。
- NFR 2.2 — `trusted_cidr.go` 拒否時 next 未呼び出し。`TestTrustedCIDRMiddleware`（拒否時本文非包含）/
  `TestNewRouter_..._OutOfRange_Returns403` / `TestStartWorkerMetricsListener_OutOfRangeForbidden`。

reviewer 自身が `go build ./...` / `go test ./...` / `go vet ./...`（tasks.md の verify ブロック
コマンド）を再実行し、すべて green / clean であることを確認した（NFR 1.2 の既存スイート不退行を含む）。

## Findings

なし

## ROUND=1 reject 理由の解消確認

ROUND=1 の Finding 1〜5（`/metrics` 本体・フェッチ/UPSERT 記録・worker スクレイプ経路・
CIDR 制限・後方互換）はいずれも AC 未カバーであった。task 2〜6 の追加実装により:

- Finding 1（1.1/1.2/1.3）→ router 条件登録 + worker listener + 全 6 系列登録で解消。
- Finding 2（2.1〜2.6）→ Fetcher/ItemUpsertService の WithMetrics と各分岐の記録挿入で解消。
- Finding 3（3.1/3.2/3.3）→ worker 専用 registry + listener、同一プロセス反映テストで解消。
- Finding 4（4.1〜4.3）→ TrustedCIDRMiddleware の実アクセス制御と単体テストで解消。
- Finding 5（5.x, NFR）→ router nil 分岐の不変性、serve timeout 不変、起動直後 200、
  未設定時 403、拒否時本文非包含の各テストで解消。

`feedman_http_status_total` の起動直後非出力（impl-notes.md「確認事項」）は、CounterVec が
ラベル値記録まで系列を出さない Prometheus テキスト形式の仕様に起因する。Req 1.3 は
「`/metrics` が全 6 系列を応答に含める」という ubiquitous 要件であり、6 Collector は registry に
登録済みで、ステータス記録後に 6 系列目が出力されることをテストで実証している。`metrics.go`
本体不変（Out of Scope）の制約下で観測可能な実装・テストが揃っており、AC 未カバー /
missing test / boundary 逸脱のいずれにも該当しないため reject 対象としない。

## Summary

ROUND=1 で reject の根拠だった AC 未カバー（Finding 1〜5）は task 2〜6 の是正実装で
すべて解消され、全 numeric AC / NFR に対応する実装とテストを確認した。reviewer 再実行で
`go build`/`go test ./...`/`go vet` も green。boundary 逸脱なし、missing test なし。

RESULT: approve
