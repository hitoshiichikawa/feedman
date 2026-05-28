# 実装ノート（Issue #14: ログレベルを環境変数で変更可能にする）

## 採用した設計判断

- **環境変数の読み取り箇所は logger パッケージ側**: `internal/app/app.go` の `Init` で
  `logger.SetupDefault(w)` が `config.Load()` より前に呼ばれており（「設定読み込み前に
  ログを使えるようにする」制約）、ログレベル決定は `config.Config` 構造体を経由できない。
  そのため logger パッケージ内に `resolveLevel`（`os.Getenv("LOG_LEVEL")` を読み、
  case-insensitive で `slog.Level` にマッピングする関数）を新設し、`Setup` の
  `HandlerOptions.Level` に渡す方式を採用した（requirements.md Introduction / Open Questions
  と整合）。
- **起動時 1 回のみ反映（Req 1.5 / Out of Scope のランタイム変更非対応）**: `Setup` 呼び出し時に
  `os.Getenv` を 1 回読み取って `slog.Level` を固定し、以後はプロセス実行中に再評価しない。
- **不正値時の警告ログを「フォールバック後のロガー自身」で出力（Req 3.2 / NFR 2.1）**:
  不正値検出時はまず INFO レベルでハンドラ/ロガーを生成し、その生成済みロガー自身で
  `logger.Warn(...)` を出力する。フォールバック先が INFO のため WARN レベルの本警告は必ず
  出力され、出力先 writer（本番は `os.Stdout`、テストは `bytes.Buffer`）と整合する形で
  確実に観測できる。
- **既存 config パッケージの警告パターンに揃えた（Req 3.2 / NFR 2.2）**: メッセージ文言
  `"環境変数のパースに失敗したためデフォルト値を採用します"` と属性 `key` / `value` /
  `default` を `internal/config/config.go` の `getEnvBool` / `getEnvInt` 等と同一形に統一。
  `default` 属性は `slog.Level.String()`（"INFO"）を文字列で格納する。
- **マジックストリングの定数化（コード規約）**: `envLogLevel`（"LOG_LEVEL"）/ `defaultLevel`
  （`slog.LevelInfo`）/ `invalidLevelWarnMsg` を定数化。exported 関数 `Setup` / `SetupDefault`
  の doc comment を更新した。
- **シグネチャ互換維持**: `Setup(io.Writer) *slog.Logger` / `SetupDefault(io.Writer)` の
  シグネチャは変更せず、既存テスト・既存呼び出し（`internal/app/app.go`）に影響を与えない。

## Feature Flag

- 本リポジトリの `CLAUDE.md` `## Feature Flag Protocol` は **採否: opt-out** のため、
  Feature Flag は導入していない（通常の単一実装パス）。

## テスト観点（AC との対応）

`internal/logger/logger_test.go` に table-driven テストおよび個別テストを追加した。

| Requirement / AC | 検証テスト |
|---|---|
| 1.1（DEBUG で全レベル出力） | `TestSetup_RespectsLogLevelEnv/DEBUGのとき全レベル出力` |
| 1.2（INFO で DEBUG 抑制） | `TestSetup_RespectsLogLevelEnv/INFOのときDEBUG抑制` |
| 1.3（WARN で DEBUG/INFO 抑制） | `TestSetup_RespectsLogLevelEnv/WARNのときDEBUGとINFO抑制` |
| 1.4（ERROR で ERROR のみ） | `TestSetup_RespectsLogLevelEnv/ERRORのときERRORのみ出力` |
| 1.5（起動時 1 回のみ反映） | `Setup` が `os.Getenv` を 1 回読むのみである実装で担保。境界は 1.1〜1.4 の出力境界テストでカバー |
| 2.1（未設定は INFO） | `TestSetup_RespectsLogLevelEnv/未設定のときINFO相当`（既存 `TestSetup_*` も env 未設定で INFO 動作を担保） |
| 2.2（空文字は未設定と同一） | `TestSetup_RespectsLogLevelEnv/空文字のときINFO相当` |
| 3.1（不正値は INFO フォールバック） | `TestSetup_RespectsLogLevelEnv/不正値VERBOSEのときINFOフォールバックと警告` |
| 3.2（警告ログにキー・値・デフォルト） | `TestSetup_InvalidLevelWarnIncludesContext` |
| 3.3（異常終了せず継続） | `TestSetup_InvalidLevelDoesNotFail`（nil を返さず継続） + 不正値ケースが正常に後続ログを出力する点で担保 |
| 4.1（小文字を同一視） | `TestSetup_RespectsLogLevelEnv/小文字debugのときDEBUG扱い` |
| 4.2（大文字小文字混在を同一視） | `TestSetup_RespectsLogLevelEnv/混在Warnのときwarn扱い` |
| NFR 1.1（後方互換 INFO 維持） | 未設定/空文字ケース + 既存 `TestSetup_*`（変更なしで pass） |
| NFR 2.1（一貫したフォールバック手順） | `TestSetup_RespectsLogLevelEnv` 不正値ケース + `TestSetup_InvalidLevelWarnIncludesContext` |
| NFR 2.2（他環境変数と一貫した警告形式） | `TestSetup_InvalidLevelWarnIncludesContext`（`key`/`value`/`default` 属性を config パターンと同形で検証） |

- 環境変数を使うテストは `t.Setenv` を使用（テストスコープで自動復元）。未設定ケースは
  `t.Setenv` で復元対象に登録した上で `os.Unsetenv` で実際に未設定状態を作る。
- 異常系・境界値: 不正値（VERBOSE / NOPE）、空文字、各レベルの出力境界（DEBUG/INFO/WARN/ERROR）
  をカバー。

## テスト結果

- `go test ./...`: 全パッケージ pass（`internal/logger` 含む）。
- `go vet ./...`: 指摘なし。
- `gofmt -l internal/logger/`: 差分なし（clean）。

## 確認事項（レビュワー判断ポイント）

- 不正値時の警告ログは「INFO ハンドラ生成後にそのロガー自身で出力」する方式とした。
  仮にデフォルトレベルが将来 WARN より上（ERROR）に変更されると本警告が抑制される懸念が
  あるが、本要件のデフォルトは INFO 固定（後方互換）であり現状は問題ない。デフォルトを
  変更する将来要件が出た場合は警告出力経路の再設計が必要になる旨を申し送る。
- ログレベルの読み取りは `Setup` 呼び出し時に行うため、`Setup` を複数回呼ぶと都度
  `os.Getenv` を再評価する。アプリ起動時は `SetupDefault` が `Init` で 1 回だけ呼ばれる
  ため Req 1.5（起動時 1 回反映）は満たすが、テスト等で `Setup` を複数回呼ぶ場合は呼び出し
  時点の env が反映される点に留意（テストはこの性質を利用して各ケースを検証している）。

STATUS: complete
