# 実装ノート（Issue #20: config パース失敗時の警告ログ）

## 実装概要

`internal/config/config.go` の数値・期間系ヘルパー `getEnvInt` / `getEnvInt64` /
`getEnvDuration` について、環境変数のパースに失敗した分岐（`err != nil`）でデフォルト値へ
フォールバックする直前に `slog.Warn` を 1 件出力するようにした。

- 環境変数が未設定（`v == ""`）の分岐ではログを出さない（正常系の挙動を維持）
- 正常パース時もログを出さない
- 各ヘルパーのシグネチャ・フォールバック挙動・デフォルト値は変更していない（NFR 1）
- パッケージ関数 `slog.Warn(...)`（グローバルデフォルトロガー）を使用。logger 未初期化でも
  stdlib のデフォルトハンドラが存在するため panic しない（NFR 2）

## 採用したログフィールド名（構造化フィールド / NFR 3）

メッセージ: `環境変数のパースに失敗したためデフォルト値を採用します`（日本語、既存 slog 慣習に準拠）

| フィールド key | 型 | 内容 |
|---|---|---|
| `key` | string | 対象の環境変数キー名 |
| `value` | string | 設定されていた不正値（生の文字列） |
| `default` | int / int64 / Duration | 採用したデフォルト値（`slog.Int` / `slog.Int64` / `slog.Duration` で型に応じて出力） |

既存コード（`internal/app/app.go` 等）が `slog.String` / `slog.Int` / `slog.Duration` の型付き
ヘルパーで構造化フィールドを出力している慣習に合わせ、フィールド key は英語とした。

## テスト方針

`internal/config/config_test.go` に近傍配置。`slog.Record` を収集する `captureHandler`
（独自 `slog.Handler` 実装）を `slog.SetDefault` で差し込み、`t.Cleanup` で元の default logger を
復元する。各ヘルパーごとに以下のサブテストを `t.Run("<条件>のとき<期待結果>", ...)` 形式で用意:

- 不正値のときデフォルト値を採用し Warn を 1 件出力する（異常系）
- 不正値のとき Warn ログに `key` / `value` / `default` を構造化フィールドで含める（異常系・構造化検証）
- 正常値のとき値を採用し Warn を出力しない（正常系）
- 未設定（空文字）のときデフォルト値を採用し Warn を出力しない（境界値）

## 受入基準とテストの対応

| Requirement ID | 内容 | 担保するテスト |
|---|---|---|
| 1.1 | int 不正値でデフォルト採用 | `TestGetEnvInt/不正値のときデフォルト値を採用しWarnを1件出力する` |
| 1.2 | int 不正値で Warn 1 件 | 同上 |
| 1.3 | int Warn にキー名・不正値・デフォルト値を構造化フィールドで含む | `TestGetEnvInt/不正値のときWarnログにキー名・不正値・デフォルト値を構造化フィールドで含める` |
| 2.1 | int64 不正値でデフォルト採用 | `TestGetEnvInt64/不正値のときデフォルト値を採用しWarnを1件出力する` |
| 2.2 | int64 不正値で Warn 1 件 | 同上 |
| 2.3 | int64 Warn に構造化フィールド | `TestGetEnvInt64/不正値のときWarnログにキー名・不正値・デフォルト値を構造化フィールドで含める` |
| 3.1 | duration 不正値でデフォルト採用 | `TestGetEnvDuration/不正値のときデフォルト値を採用しWarnを1件出力する` |
| 3.2 | duration 不正値で Warn 1 件 | 同上 |
| 3.3 | duration Warn に構造化フィールド | `TestGetEnvDuration/不正値のときWarnログにキー名・不正値・デフォルト値を構造化フィールドで含める` |
| 4.1 | 正常値を採用 | 各 `Test*/正常値のとき値を採用しWarnを出力しない` |
| 4.2 | 正常値で Warn を出さない | 各 `Test*/正常値のとき値を採用しWarnを出力しない` |
| 4.3 | 未設定でデフォルト採用 | 各 `Test*/未設定（空文字）のときデフォルト値を採用しWarnを出力しない` |
| 4.4 | 未設定で Warn を出さない | 各 `Test*/未設定（空文字）のときデフォルト値を採用しWarnを出力しない` |
| NFR 1 | 既存挙動維持（シグネチャ・フォールバック・デフォルト値不変） | 既存 `TestLoad_DefaultValues` / `TestLoad_CustomValues` が引き続き green |
| NFR 2 | ログ初期化前後で panic しない | グローバル `slog.Warn` を使用（stdlib デフォルトハンドラで panic 回避）。テストは default logger 差し替えで安全に検証 |
| NFR 3 | 機械検索可能な構造化フィールド | `key` / `value` / `default` を独立フィールドで出力（構造化検証テストで担保） |

## 検証結果

- `gofmt -l internal/config/`: 未整形なし
- `go vet ./internal/config/...`: pass
- `go test ./internal/config/...`: pass
- `go build ./...`: pass
- `go test ./...`: 全パッケージ pass（既存テストの破壊なし）

## 確認事項

- なし（要件・スコープが Issue 本文で確定しており、設計上の曖昧点は発生しなかった）

STATUS: complete
