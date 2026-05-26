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

## 確認事項（レビュワー判断ポイント）

- requirements.md / design.md / tasks.md との矛盾は本 task の範囲では検出していない。
- 本起動は per-task ループの **task 1（子 1.1 / 1.2）のみ**を実装している。task 2〜6 は未着手で、
  orchestrator が後続起動で順次消化する。

STATUS: complete
