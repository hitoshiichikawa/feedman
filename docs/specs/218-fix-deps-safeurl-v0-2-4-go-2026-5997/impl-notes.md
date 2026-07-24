# Implementation Notes — Issue #218

## 実施内容

- `github.com/doyensec/safeurl` を v0.2.2 → v0.2.4 に更新し、GO-2026-5997（v0.2.2 の blocklist に
  IPv6 CIDR レンジが欠けている問題）を解消した。
- 変更対象は `go.mod` と `go.sum` のみ（`internal/security/ssrf_guard.go` を含むアプリケーション
  コード・テストコードは無変更）。NFR 2.1 のスコープ限定制約を遵守。
- リポジトリ構成: `go.mod` はルート 1 ファイルのみ（api / worker で分割されておらず、
  `github.com/hitoshi/feedman` 単一モジュール構成）。他モジュール向けの対応は不要。

## 実行コマンドと結果

| ステップ | コマンド | 結果 |
|---|---|---|
| 依存更新 | `go get github.com/doyensec/safeurl@v0.2.4` | `upgraded github.com/doyensec/safeurl v0.2.2 => v0.2.4` |
| tidy | `go mod tidy` | 差分は `go.mod` の safeurl 行と `go.sum` の safeurl 2 行のみ |
| ビルド | `go build ./...` | エラーなし（exit 0） |
| 静的解析 | `go vet ./...` | 違反 0 件（exit 0） |
| テスト | `go test ./...` | 全 22 パッケージ pass（SSRF ガードを含む `internal/security` を含む） |
| 脆弱性スキャン | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | `No vulnerabilities found.` / exit 0 / GO-2026-5997 の検出行 0 件 |

### govulncheck 出力の要点

- 呼び出し到達（`=== Symbol Results ===`）で **No vulnerabilities found.** を確認
- verbose 出力で列挙されたモジュール脆弱性は `GO-2026-5970` と `GO-2026-5942` の 2 件のみで、
  いずれも本 Issue の対象 `GO-2026-5997` ではない（かつ呼び出し到達なし）。よって本 Issue の
  スコープ（GO-2026-5997 の解消）は達成されている。
- CI 側は `go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...` を使用しており、
  本ローカル実行の `go run` と実行対象は等価（`.github/workflows/ci.yml` の当該ジョブは変更なし）。

## 受入基準の達成状況

### Requirement 1: safeurl の脆弱性解消

- **1.1**（`go.mod` が safeurl v0.2.4 以上を要求）: `go.mod` の `require` ブロックで
  `github.com/doyensec/safeurl v0.2.4` を指定。`git diff go.mod` で確認済み。
- **1.2**（`go.sum` に v0.2.4 以上のエントリ）: `go.sum` に
  `github.com/doyensec/safeurl v0.2.4 h1:...` と `.../go.mod h1:...` の 2 行を含む。
- **1.3**（govulncheck が GO-2026-5997 を検出しない）: 上記 `govulncheck ./...` 出力に
  GO-2026-5997 の検出行 0 件を確認済み。
- **1.4**（govulncheck がゼロ終了）: `EXIT=0` を確認済み。

### Requirement 2: 既存ビルド・テストの後方互換

- **2.1**（`go test ./...` 全成功）: 22 パッケージすべて `ok`。SSRF ガード
  （`internal/security`）を含む。
- **2.2**（`go vet ./...` 違反 0 件）: exit 0、警告出力なし。
- **2.3**（`gofmt` 差分ゼロ）: **確認事項参照**。本 Issue の変更前後で `gofmt -l .` 出力は同一
  （差分は既存 main の状態に由来し、本 Issue の safeurl 更新起因ではない）。
- **2.4**（既存テストを弱めない）: 既存 SSRF ガードテストはすべて pass しており、テスト側を
  変更する必要は発生しなかった。

### Requirement 3: SSRF ガード挙動の維持

- **3.1–3.4**: `internal/security/ssrf_guard.go` および `internal/security/*_test.go` は
  無変更で、`internal/security` パッケージのテストが全 pass。許可 scheme / port / タイムアウト /
  プライベートアドレス遮断の挙動は退行していない。

### Non-Functional Requirements

- **NFR 1.1**（CI 所要時間）: 純粋な依存バージョン更新（v0.2.2 → v0.2.4 の patch bump）のため、
  govulncheck・テスト所要時間に有意な変化は想定されない。
- **NFR 2.1**（スコープ限定）: 変更ファイルは `go.mod` / `go.sum` のみ（`git diff --name-only`
  で確認済み）。
- **NFR 3.1**（追跡可能性）: govulncheck 出力に GO-2026-5997 の検出行を含まないことを確認済み。

## 確認事項

- **`gofmt -l .` に既存 main 由来の差分が残存**: 本 Issue の変更前（`git stash` した状態）でも
  以下 11 ファイルが `gofmt -l` に列挙される。本 Issue の safeurl 更新起因ではないため、
  スコープ限定制約（NFR 2.1）に従い**本 PR では修正しない**。別 Issue（chore: gofmt 差分の
  一括解消）で対応することを推奨。
  - `internal/crossfeed/service_test.go`
  - `internal/handler/crossfeed_handler_test.go`
  - `internal/handler/feed_handler_test.go`
  - `internal/handler/item_search_handler.go`
  - `internal/handler/router_full_test.go`
  - `internal/hatebu/batch.go`
  - `internal/itemsearch/service.go`
  - `internal/itemsearch/service_test.go`
  - `internal/middleware/ratelimit_test.go`
  - `internal/model/item.go`
  - `internal/security/content_sanitizer_test.go`

  なお、これら既存 gofmt 差分が CI で fail しているとの報告は Issue #218 の記述に無く、CI 上の
  gofmt 検査ステップは `.github/workflows/ci.yml` を確認する限り現状存在しない。本 Issue の
  完了判定には影響しない。

## その他

- `govulncheck` バイナリはこの環境に事前インストールされていなかったため、`go run
  golang.org/x/vuln/cmd/govulncheck@latest ./...` の代替経路で検証した。CI 側は
  `go install ...` 経由で同 CLI をインストールして実行する構成であり、実行対象は等価。
- 追加した依存は無し（safeurl のバージョン変更のみで indirect 依存の増減もなし）。
- 参考: 過去 Issue #212 / PR #213 / commit `cb53d40`（Go toolchain 1.25.12 更新の `fix(deps):`
  対応）。今回は toolchain ではなく単一パッケージのバージョン bump のため差分は最小。

STATUS: complete
