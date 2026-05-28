# 実装ノート: Issue #2 DB コネクションプール設定を追加

## 採用値と根拠

`internal/database/db.go` にパッケージレベルの名前付き定数として以下を定義し、`Open` 内で適用した。

| 定数 | 値 | 根拠 |
|---|---|---|
| `maxOpenConns` | 25 | api 25 + worker 25 = 50 ≤ PostgreSQL `max_connections`（既定 100）。マイグレーション/管理接続の余裕も確保 |
| `maxIdleConns` | 10 | アイドル接続を際限なく保持しないため `maxOpenConns` 以下に抑制（Requirement 2.2） |
| `connMaxLifetime` | 5 * time.Minute | 長寿命接続を一定時間で再確立し、NW 機器/DB 側タイムアウトによる断線を顕在化させない（Requirement 3） |

オーケストレーター確定済みの設計判断（Open Questions への回答）に従い:

- **環境変数化はしない**: `internal/database` パッケージ内のパッケージレベル定数として固定。`internal/config` への追加・env 読み込みはしない（Requirement 4.1 「名前付き定数として明示」/ 4.2 シグネチャ後方互換）。
- **api/worker 共通値**: 共通の `Open` を両プロセスが呼ぶ現状を維持。プロセス別の差別化はしない。
- `Open` のシグネチャ `func Open(databaseURL string) (*sql.DB, error)` は変更せず、`internal/app/app.go:90`（runServe）/ `:215`（runWorker）の呼び出し側は無変更で動作する（Requirement 4.3 / NFR 2.1）。

## テスト方針

`internal/database/db_test.go` を拡張。既存テスト（`TestOpen_ReturnsDBForAnyURL` / `TestOpen_WithValidURL_ReturnsDB`）は無変更で維持。実 DB 接続は行わず、`sql.Open` が接続を試行しない前提を保持。

### `db.Stats()` の getter 制約

`*sql.DB` には設定値の公開 getter が一部しか存在しない:

- `db.Stats().MaxOpenConnections` は**公開取得可能** → `Open` 後の `*sql.DB` で `MaxOpenConns = 25` が設定されていることを実物に最も近い形で検証（`TestOpen_SetsMaxOpenConns`）。
- `MaxIdleConns` / `ConnMaxLifetime` には**公開 getter が無い** → パッケージ内テストから**定数の不変条件**として検証する（`maxIdleConns > 0` かつ `maxIdleConns <= maxOpenConns`、`connMaxLifetime > 0`）。
- 2 プロセス合算 ≤ 100 の AC は定数不変条件（`2 * maxOpenConns <= 100`）として検証する。

Red→Green を確認済み（実装前は定数未定義でコンパイルエラー、実装後に全テスト pass）。

## 受入基準とテストの対応

| Requirement ID | 担保するテスト |
|---|---|
| 1.1 / 1.2 | `TestOpen_SetsMaxOpenConns`（実物 `db.Stats().MaxOpenConnections == 25`）/ `TestPoolConstants_AreFinitePositive`（maxOpenConns > 0） |
| 1.3 / NFR 1.1 | `TestPoolConstants_TwoProcessSumWithinMaxConnections`（`2 * maxOpenConns <= 100`） |
| 2.1 / 2.2 | `TestPoolConstants_AreFinitePositive`（maxIdleConns > 0 かつ <= maxOpenConns）|
| 3.1 / 3.2 | `TestPoolConstants_AreFinitePositive`（connMaxLifetime > 0）|
| 4.1 | パッケージレベル名前付き定数として定義（コード上で担保）|
| 4.2 / 4.3 / NFR 2.1 | `Open` シグネチャ無変更。`go test ./internal/app/...` を含む全体テストが green（呼び出し側無変更で動作）|

## 検証結果

- `gofmt -l internal/database/`: 差分なし
- `go vet ./internal/database/...`: 通過
- `go test ./internal/database/...`: ok
- `go test ./...`: 全パッケージ ok（既存テストの破壊なし）

## 確認事項

- 同時接続数上限は確定済み設計判断に従い 25 とした。将来のスケール（複数 worker インスタンス等）で 2 プロセスを超える同時稼働が発生する場合、合算上限が 100 を超え得るため、その際は値の再調整または環境変数化（要シグネチャ後方互換の再検討）を別 Issue として検討する余地がある（requirements.md の Open Questions に対応）。

STATUS: complete
