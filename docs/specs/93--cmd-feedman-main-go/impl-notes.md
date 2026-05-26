# 実装ノート: Issue #93 プロジェクトをビルド可能にする（cmd/feedman/main.go の作成）

## 実装方針の要約

`internal/app`（`app.Run` / `app.ParseCommand`）は既に実装済みだが、`package main` /
`func main()` を持つエントリポイントがコミットされていないためバックエンドバイナリ /
コンテナイメージがビルドできなかった。本実装では既存起動機構を一切変更せず、薄いエントリ
ポイント `cmd/feedman/main.go` を追加してビルドを通し、CI でエントリポイント不在を機械的に
検出できるようにした。

### main.go の設計

- `package main` / `func main()` を持つ薄いラッパー。起動は `app.Run(os.Stdout, args)` に委譲
  し、`internal/app` は変更していない（Req 1.3 / NFR 1.1）。
- テスト容易性のため `main()` から `run(stdout, stderr io.Writer, args []string, r runner) int`
  を分離し、起動関数を `runner` 型で注入できる形にした。これによりサーバ／DB を起動せずに
  「error→stderr 出力＋終了コード 1 / 正常時 0」「args を改変せず委譲」を単体テストできる。
- `main()` は `os.Exit(run(os.Stdout, os.Stderr, os.Args[1:], app.Run))` のみ。
- exported な識別子は持たせず（`run` / `runner` は package-private）、doc comment を付与した。

### .gitignore の修正（根本原因の一部）

調査の結果、`.gitignore` の `feedman` パターン（先頭スラッシュなし）が、ビルド成果物バイナリ
だけでなく `cmd/feedman/` ディレクトリ配下のソースにもマッチしており、`git add` してもエントリ
ポイントがコミットできない状態であった。これが「`cmd/feedman` が一度もコミットされていない」
（requirements.md Introduction 記載）根本原因の一部である。ルート直下のビルド成果物のみを対象と
する `/feedman` に限定し、`cmd/feedman/` 配下のソースを追跡可能にした。ルートの `feedman`
バイナリは引き続き無視される（`git check-ignore feedman` で確認済み）。

### テスト方針

`cmd/feedman/main_test.go` に、fake runner を注入して `run` の挙動を検証する table 形式の
サブテスト（Arrange/Act/Assert・1 テスト 1 検証）を配置した。実起動（serve/worker、DB 必要・
ブロッキング）は単体テストでは起動しない。サブコマンドルーティング（serve/worker/migrate/
healthcheck・未知引数→serve フォールバック）は `internal/app` の既存テスト責務とした
（確認事項参照）。

### CI 変更内容

`.github/workflows/ci.yml` の backend ジョブに、テスト実行前へ
`CGO_ENABLED=0 go build -o /tmp/feedman ./cmd/feedman` の検証ステップを追加した。エントリ
ポイント不在でビルドできない状態を CI で機械的に失敗させる（Req 5.1 / 5.3）。既存の
`go test` / `go vet` / `govulncheck` / frontend 系ジョブは変更していない。

## 各 AC への対応箇所

### Requirement 1: バックエンドバイナリのビルド可能性

- 1.1（`package main` の `func main()` を `cmd/feedman` に含む）: `cmd/feedman/main.go` を新規
  作成。`.gitignore` 修正によりコミット可能化。
- 1.2（`go build ./cmd/feedman` がエラーなく完了し実行可能バイナリ生成）: 検証で
  `go build ./cmd/feedman` 成功・実行可能バイナリ生成を確認。CI ステップでも担保。
- 1.3（起動処理を既存 `internal/app` に委譲、独自解釈ロジックを重複実装しない）:
  `run` が `app.Run` に args をそのまま委譲。`main_test.go` の「args を改変せず委譲」テストで担保。

### Requirement 2: サブコマンドによる起動モード切り替え

- 2.1〜2.4: 解釈ロジックは `app.ParseCommand` / `app.Run` がそのまま担当（main は委譲のみ）。
  `internal/app/cmd_test.go` が serve / worker / migrate / 未知引数→serve フォールバック
  （`TestParseCommand_UnknownDefaultsToServe`）/ 引数なし→serve（`TestParseCommand_DefaultsToServe`）
  を既にテスト済み。main 側は委譲の正しさ（args 改変なし・空 args 委譲）を `main_test.go` で担保。

### Requirement 3: 起動失敗時のエラー観測性

- 3.1（エラーメッセージを stderr に出力）: `run` で `fmt.Fprintln(stderr, err)`。
  `main_test.go` の「runner が error を返すとき stderr にエラーメッセージが出力される」で担保。
- 3.2（非ゼロ終了コード）: `run` が 1 を返す。「終了コード 1 を返す」テストで担保。
- 3.3（正常終了時は終了コード 0）: `run` が 0 を返す。「終了コード 0 で stderr に何も書かれない」
  「stderr が空である」テストで担保。

### Requirement 4: コンテナイメージのビルド可能性

- 4.1〜4.3: `cmd/feedman` 追加により `Dockerfile` の `go build -o /feedman ./cmd/feedman` が解決
  可能になる（従来は対象不在でビルド不能）。`Dockerfile` 自体は無変更。実 docker build は本 Issue
  では CI 化しない（確認事項参照）が、ローカルで `CGO_ENABLED=0 GOOS=linux go build` 相当の
  静的バイナリビルド成功を確認済み。

### Requirement 5: 再発防止のための CI 検出

- 5.1 / 5.3（エントリポイント不在で CI ビルド検証失敗 / 存在時は成功）: backend ジョブに
  `CGO_ENABLED=0 go build -o /tmp/feedman ./cmd/feedman` ステップ追加。
- 5.2（root Dockerfile ビルド不能の検出）: 実 docker build は採用せず、Dockerfile と同条件
  （`CGO_ENABLED=0` で `./cmd/feedman` をビルド）を CI で検証することで、Dockerfile のビルド対象
  が解決可能であることを近似的に担保（確認事項参照）。

### Non-Functional Requirements

- NFR 1.1〜1.4（後方互換）: `internal/app` / `Dockerfile`（`ENTRYPOINT`/`CMD`）/ docker compose
  の `command` / healthcheck はいずれも無変更。main は `app.Run` を委譲利用するのみ。
- NFR 1.5（既存 `go test ./...` 成功維持）: 全パッケージ PASS を確認。
- NFR 2.1（`CGO_ENABLED=0` 静的バイナリビルド可能）: `file` 出力で `statically linked` を確認。
- NFR 2.2（distroless 上で追加動的ライブラリ依存なし起動）: 静的リンクバイナリのため動的依存
  なし。Dockerfile の distroless ランタイム構成は無変更で維持。

## 検証コマンドの実行結果

| コマンド | 結果 |
|---|---|
| `gofmt -l cmd/` | 出力なし（未整形なし） |
| `go vet ./...` | PASS |
| `go build ./cmd/feedman` | PASS（実行可能バイナリ生成） |
| `CGO_ENABLED=0 go build -o /tmp/feedman-static ./cmd/feedman` | PASS（`statically linked` 確認） |
| `go test ./...` | 全パッケージ PASS（`cmd/feedman` 含む、既存テスト後方互換維持） |

Red→Green 確認: `run` の `return 1` を `return 0` に一時改変すると「終了コード 1 を返す」テストが
FAIL し、戻すと PASS することを確認済み（テストが観点不備でないことを検証）。

## 確認事項

### Open Question 1: 未知サブコマンドの扱いに関する要件の矛盾

requirements.md の決定（後方互換優先で「未知引数→serve フォールバック」を Req 2.4 とし、異常系は
Req 3 の起動機構エラー観測性で定義）に従い、main 側で独自の引数検証は行わず `app.ParseCommand` /
`app.Run` の既存挙動をそのまま委譲した。`internal/app/cmd_test.go` に未知引数→serve フォールバック
（`TestParseCommand_UnknownDefaultsToServe`）を含む `ParseCommand` のテストが既に整備済みであるため、
Req 2.4 は無テストではなく既存テストでカバーされている（よって `internal/app` のテスト追加は不要と
判断）。もし「未知サブコマンドを明示的にエラーにする」挙動が必須要件であれば Issue のスコープ再定義
（`internal/app` のフォールバック挙動変更 or main 側のロジック二重化）が必要であり、人間の判断を仰ぎたい。

### Open Question 2: CI 検出手段の選択（実 docker build を CI に入れるか）

オーケストレーター決定に従い、最小かつ堅牢な `CGO_ENABLED=0 go build ./cmd/feedman` ステップを CI に
追加した。実 docker build ジョブは採用していない。

- 採用しなかった理由: 実 docker build は Docker daemon 依存と実行時間増を招くため。`go build` ステップは
  Dockerfile が前提とするビルド対象（`./cmd/feedman`）が解決可能かつ静的ビルドできることを軽量に検証でき、
  エントリポイント欠落（Req 5.1）を機械的に検出できる。
- 残課題（人間判断を仰ぎたい点）: Req 5.2（root Dockerfile が前提とするエントリポイントを解決できず
  ビルド不能になった場合の検出）を「実 docker build」で厳密に担保するか、現状の `go build` 近似で
  十分とするか。Dockerfile のステージ構成・COPY パス・distroless ランタイム起動可否までを CI で
  保証したい場合は、別途 `docker build` ジョブ（または `docker_test.go` の実ビルド化）を追加する
  派生 Issue が考えられる。本 Issue では実行時間・依存の trade-off を踏まえ `go build` 検証に留めた。

### 追加で実施した spec 外の変更（.gitignore）

`.gitignore` の `feedman` → `/feedman` 修正は requirements.md の明示要件ではないが、Req 1.1
（エントリポイントを `cmd/feedman` に含む = コミット済み状態にする）を満たすために不可欠であった
（パターンが `cmd/feedman/` 配下にマッチしてソースがコミットできなかった）。`internal/app` /
`requirements.md` のいずれにも該当しないため制約に抵触しないと判断した。レビュワーは本変更が
ビルド成果物バイナリ（ルート `feedman`）の無視を維持しつつソース追跡を可能にしている点を確認されたい。

## 受入基準とテストの対応表

| Req ID | 担保するテスト / 検証 |
|---|---|
| 1.1 | `cmd/feedman/main.go` の存在 + `.gitignore` 修正によるコミット可能化 |
| 1.2 | `go build ./cmd/feedman` 成功（検証 + CI ステップ） |
| 1.3 | `main_test.go`「受け取った args を改変せず runner にそのまま委譲する」「stdout を runner に委譲する」 |
| 2.1〜2.4 | `internal/app/cmd_test.go`（既存）+ main 側委譲テスト（`main_test.go` 委譲系） |
| 3.1 | `main_test.go`「runner が error を返すとき stderr にエラーメッセージが出力される」 |
| 3.2 | `main_test.go`「runner が error を返すとき終了コード 1 を返す」 |
| 3.3 | `main_test.go`「runner が nil を返すとき終了コード 0 で stderr に何も書かれない」「stderr が空である」 |
| 4.1〜4.3 | `CGO_ENABLED=0` 静的ビルド成功（Dockerfile 同条件）+ Dockerfile/compose 無変更 |
| 5.1 / 5.3 | CI backend ジョブの `go build ./cmd/feedman` ステップ |
| 5.2 | `go build` 近似検証（実 docker build は確認事項で trade-off 明記） |
| NFR 1.1〜1.4 | `internal/app`/`Dockerfile`/compose/healthcheck 無変更 |
| NFR 1.5 | `go test ./...` 全 PASS |
| NFR 2.1 | `file` 出力で `statically linked` 確認 |
| NFR 2.2 | 静的リンクバイナリ（動的依存なし）+ distroless 構成無変更 |

STATUS: complete
