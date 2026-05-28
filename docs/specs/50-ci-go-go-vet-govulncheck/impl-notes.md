# 実装メモ: Issue #50 CI に Go の静的解析・脆弱性スキャン（go vet / govulncheck）を追加

## 実装サマリ

`.github/workflows/ci.yml` に Go の静的解析と依存脆弱性スキャンを **2 つの専用ジョブ**として追加した。

- **`go-vet` ジョブ**（`Go Static Analysis (go vet)`）
  - `actions/checkout@v4` → `actions/setup-go@v5`（`go-version-file: go.mod`）→ `go vet ./...`
- **`govulncheck` ジョブ**（`Go Vulnerability Scan (govulncheck)`）
  - `actions/checkout@v4` → `actions/setup-go@v5`（`go-version-file: go.mod`）→
    `go install golang.org/x/vuln/cmd/govulncheck@latest` → `govulncheck ./...`
- 既存の `backend` / `frontend` ジョブはそのまま温存。新規 2 ジョブは独立 job として宣言したため、
  GitHub Actions の既定挙動で **既存ジョブと並列実行**される（NFR 1 充足）。
- いずれのステップも `continue-on-error` を付与しておらず、`go vet` / `govulncheck` が非ゼロ終了
  すると当該ジョブが失敗し、PR の必須チェック未達となる（人間が Issue で確定した「検出時はブロック」
  方針に従う / AC 1.3・1.4・2.3・2.4）。

### govulncheck の導入方法と選定理由

**選定: `go install golang.org/x/vuln/cmd/govulncheck@latest` 方式を採用**（`golang/govulncheck-action`
は採用しない）。

理由:

- 本リポジトリの既存 CI は `actions/setup-go@v5` + `go.mod` 参照で Go ツールチェインを統一して
  おり、`go install ...@latest` 方式はそのツールチェイン（`go.mod` 指定の 1.25 系列）で
  govulncheck をビルド・実行できるため、ツールチェイン整合（NFR 2）を保ちやすい。
- `golang/govulncheck-action` を使うと action 側が独自に Go バージョンや govulncheck バージョンを
  解決するため、本リポジトリの `go.mod` 起点のバージョン整合方針と二重管理になりやすい。
- `@latest` は最新の govulncheck 本体（脆弱性検出ロジック）を取得する。脆弱性 DB（`vuln.go.dev`）は
  実行時に常に最新を参照するため、ツール本体の固定よりも DB 追従性を優先した。なお action / install
  いずれを選んでも脆弱性 DB はネットワーク取得である点は変わらない（後述「制約」参照）。
- 既存の `backend` ジョブと同じ `setup-go` 流儀を踏襲することで、保守時の認知負荷を最小化した。

> pinning に関する補足: 現状は govulncheck 本体を `@latest` としているが、再現性を厳格にしたい
> 場合は将来的に `@vX.Y.Z` への固定も後方互換に可能。今回は「最新の検出ロジック + 最新 DB」を
> 優先した（レビュワー判断ポイントとして「確認事項」にも記載）。

## 各 AC と実装/検証のトレーサビリティ

| AC | 内容（要約） | 実装 / 検証 |
|---|---|---|
| 1.1 | push / PR 契機で全パッケージに静的解析 | `go-vet` ジョブが `on: push/pull_request` で `go vet ./...` を実行（`./...` で全パッケージ） |
| 1.2 | 違反 0 件なら成功 | `go vet ./...` がゼロ終了 → ステップ成功。ローカルで EXIT=0 を確認（後述） |
| 1.3 | 違反 1 件以上で失敗（非ゼロ終了） | `continue-on-error` 不使用。`go vet` の非ゼロ終了がそのままジョブ失敗に伝播 |
| 1.4 | 失敗時 PR マージをブロック | 独立ジョブとして失敗が PR チェックに反映（required check 設定は repo 側設定の領分。確認事項参照） |
| 2.1 | push / PR 契機で全パッケージに脆弱性スキャン | `govulncheck` ジョブが `govulncheck ./...` を実行 |
| 2.2 | 脆弱性 0 件なら成功 | `govulncheck ./...` がゼロ終了 → ステップ成功 |
| 2.3 | 到達可能な脆弱性 1 件以上で失敗 | `continue-on-error` 不使用。govulncheck は到達可能脆弱性検出時に EXIT=3 を返しジョブ失敗 |
| 2.4 | 失敗時 PR マージをブロック | 独立ジョブの失敗が PR チェックに反映（required check は repo 側設定の領分） |
| 3.1 | 違反内容を CI ログに出力 | `go vet ./...` は検出内容を標準エラーに出力（Actions のステップログに表示される） |
| 3.2 | 脆弱性識別情報を CI ログに出力 | `govulncheck ./...` は GO-YYYY-NNNN 等の識別子と呼び出しトレースを出力（ローカル実行で確認） |
| 4.1 | 既存 Go テストを従来どおり実行 | `backend` ジョブ（`go test ./...`）を無変更で温存 |
| 4.2 | 既存フロントエンドテストを従来どおり実行 | `frontend` ジョブを無変更で温存 |
| 4.3 | いずれも違反なしなら全体成功 | 4 ジョブが全て成功すれば PR 全体チェック成功 |
| 4.4 | 解析失敗を既存テストと独立に全体結果へ反映 | `go-vet` / `govulncheck` を `backend` から分離した独立ジョブにしたため、テスト成否と独立に失敗が反映される |
| NFR 1 | CI 所要時間を不必要に延長しない | 既存テストジョブに直列で積まず、独立ジョブ化して並列実行（GitHub Actions 既定で job 間は並列） |
| NFR 2 | go.mod 指定系列と整合するツールチェイン | 両ジョブとも `actions/setup-go@v5` + `go-version-file: go.mod` を使用（既存 backend と同流儀） |
| NFR 3 | 既存ジョブのトリガーを維持 | `on:` ブロック・既存 2 ジョブを無変更。新規ジョブも同一 `on:` 配下で動作 |

## ローカル検証結果

ローカル環境の PATH 上デフォルト `go` は `GOTOOLCHAIN=auto` のもとで `go.mod` の `go 1.25` 表記から
存在しないリリース名 `go1.25` を取得しようとし、サンドボックス（ネットワーク遮断）で失敗する
（Issue #42 impl-notes に記載済みの環境固有事象）。このためキャッシュ済みの実ツールチェイン
（`go1.25.1` / `go1.25.10`）を `GOTOOLCHAIN` で明示指定して検証した。

| 検証 | コマンド | 結果 |
|---|---|---|
| go vet | `GOTOOLCHAIN=go1.25.1 go vet ./...` | **成功（EXIT=0、違反 0 件）** |
| govulncheck install | `GOTOOLCHAIN=go1.25.1 go install golang.org/x/vuln/cmd/govulncheck@latest` | 成功（govulncheck v1.3.0 を取得・ビルド） |
| govulncheck（go1.25.1 ベース） | `GOTOOLCHAIN=go1.25.1 govulncheck ./...` | **EXIT=3 / 24 件検出**（標準ライブラリ crypto/tls・crypto/x509 等 + `golang.org/x/net`） |
| govulncheck（go1.25.10 ベース） | `GOTOOLCHAIN=go1.25.10 govulncheck ./...` | **EXIT=3 / 5 件検出**（すべて `golang.org/x/net@v0.47.0` 由来。標準ライブラリ起因は解消） |
| YAML 構文 | `python3 -c 'yaml.safe_load(...)'` | OK（jobs: backend / go-vet / govulncheck / frontend、`on:` は push/pull_request の main, develop を維持） |

### `go vet` 結果

`go vet ./...` は go1.25.1 ツールチェインで **EXIT=0**（違反 0 件）。現状の本リポジトリコードは
vet を pass するため、AC を満たすゲートを入れても既存コードでこのジョブは成功する見込み。

### `govulncheck` 結果（重要・レビュワー判断ポイント）

govulncheck はローカル実行で**到達可能な脆弱性を検出して EXIT=3**（非ゼロ終了）した。検出内容は
ベースとした Go ツールチェインのパッチ世代によって異なる:

- **go1.25.1 ベース**: 24 件（うち多くが標準ライブラリ crypto/tls・crypto/x509 起因。これらは
  go1.25.2 / go1.25.3 等で修正済み）+ `golang.org/x/net@v0.47.0` 起因。
- **go1.25.10 ベース**: 5 件。すべて `golang.org/x/net@v0.47.0` 起因（fixed in `golang.org/x/net@v0.55.0`）。
  例: `GO-2026-5025` 等。`internal/worker/fetch/fetcher.go` の `gofeed.Parser.ParseString` →
  `html.Parse` 経由で到達可能と報告される。

つまり:

- **標準ライブラリ起因の検出は、CI の `setup-go` が解決する最新 1.25.x パッチで自動的に解消され得る**
  （CI は `go-version-file: go.mod` で 1.25 系列の最新パッチを取得するため）。
- **`golang.org/x/net@v0.47.0` 由来の脆弱性は本リポジトリの依存自体に存在**し、ツールチェイン更新
  では解消されない。`go.mod` の依存を `golang.org/x/net@v0.55.0` 以上に更新するまで残存する。

このため、本 PR のゲートを `continue-on-error: false`（ブロック）で導入すると、**現状の依存のままでは
`govulncheck` ジョブが失敗し、PR マージがブロックされる**可能性が高い。これは AC 2.3（到達可能な
脆弱性検出時は失敗させる）の **仕様どおりの正しい挙動**であり、検出された脆弱性の修正自体は本 Issue の
Out of Scope（「検出された違反・脆弱性の実際の修正作業」は別 Issue）である。詳細は「確認事項」を参照。

## 制約

- **govulncheck はネットワーク依存**: govulncheck は実行時に Go 脆弱性データベース（`vuln.go.dev`）を
  ネットワーク経由で参照する。また `go install golang.org/x/vuln/cmd/govulncheck@latest` 自体も
  module proxy へのネットワークアクセスを要する。GitHub Actions の `ubuntu-latest` ランナーは
  これらに到達可能なため CI 上では問題ないが、ネットワーク制限環境（オフライン CI 等）では失敗する。
  本リポジトリのローカルサンドボックスでは module proxy へのアクセスが許可されていたため install /
  実行ともに成功した。
- **ローカルでの `GOTOOLCHAIN=auto` 制約**: `go.mod` の `go 1.25` 表記に対し、ローカルの
  `GOTOOLCHAIN=auto` は存在しないリリース名 `go1.25` を取得しようとして失敗する（#42 で既知）。
  検証はキャッシュ済み `go1.25.1` / `go1.25.10` を明示指定して実施した。CI（`setup-go` が
  正規の 1.25.x を取得する環境）では発生しない。
- **govulncheck の検出件数はツールチェインのパッチ世代に依存**: 標準ライブラリ起因の脆弱性は
  setup-go が取得するパッチ世代で増減する。CI では最新 1.25.x パッチが取得されるため、ローカルの
  go1.25.1 ベース（24 件）より少ない検出に収束する想定（go1.25.10 ベースの 5 件が目安）。

## 確認事項（人間レビュー向け）

1. **現状の依存に既知脆弱性があり、ブロック構成では `govulncheck` ジョブが赤になる見込み**:
   `golang.org/x/net@v0.47.0` 由来の到達可能な脆弱性が検出される（fixed in `v0.55.0`）。人間が Issue で
   確定した「検出時はブロック」方針に忠実に従い `continue-on-error` を入れていないため、この PR を
   merge して以降の PR では `govulncheck` チェックが失敗し続ける可能性が高い。検出された脆弱性の修正
   （`go.mod` の `golang.org/x/net` 更新等）は本 Issue の **Out of Scope**（別 Issue）。
   - **レビュワー / 人間判断ポイント**: ゲート導入と依存更新の順序をどうするか。(a) 先に依存更新 Issue を
     消化してから本ゲートを有効化する / (b) 本ゲートを先に入れて赤を可視化し、依存更新 Issue を即時起票
     する、のいずれか。requirements の決定事項（ブロック / `continue-on-error` 不使用）には従った実装で
     あるため、本 PR では構成変更（警告化）は行っていない。依存更新は別 Issue として切り出すことを推奨。

2. **GitHub の required status checks 設定（AC 1.4 / 2.4 の「マージをブロックする状態」）**:
   ジョブの失敗が「PR マージをブロックする必須チェック未達」になるには、リポジトリの Branch
   protection rules で `Go Static Analysis (go vet)` / `Go Vulnerability Scan (govulncheck)` を
   required status check に登録する必要がある。これはワークフロー YAML 側ではなく **repo 設定の領分**
   であり、本 PR の変更範囲外。ジョブ自体は失敗（非ゼロ終了）を正しく返すため、required 登録さえ
   行えば AC 1.4 / 2.4 を満たす。required 登録の要否・タイミングは人間運用判断。

3. **govulncheck 本体のバージョン pin**: 現状 `@latest`。検出ロジックの再現性を厳格化したい場合は
   `@vX.Y.Z` 固定も可能（後方互換）。今回は最新検出ロジック + 最新 DB 追従を優先した。

STATUS: complete
