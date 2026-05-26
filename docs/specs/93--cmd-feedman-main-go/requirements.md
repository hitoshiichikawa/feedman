# Requirements Document

## Introduction

Feedman はアプリの起動ロジックを `internal/app`（`package app`）に実装済みだが、`package main` /
`func main()` を持つエントリポイントがリポジトリに存在しないため、バックエンドのバイナリも
api/worker のコンテナイメージもビルドできない。`Dockerfile` は `go build -o /feedman ./cmd/feedman`
を前提にしているが、`cmd/feedman` ディレクトリが一度もコミットされていないことが原因である。
本要件は、既存の起動機構（`app.Run` / `app.ParseCommand`）を変更せずに薄いエントリポイントを
追加してビルドを通し、エントリポイント不在 / root Dockerfile ビルド不能を CI で検出して再発を
防ぐことをゴールとする。新機能の追加や `internal/app` の serve/worker ロジック自体の変更は行わない。

## Requirements

### Requirement 1: バックエンドバイナリのビルド可能性

**Objective:** As a 開発者, I want `cmd/feedman` のエントリポイントから feedman バイナリをビルドできること, so that バックエンドをローカル・CI・コンテナで実行できる状態にする

#### Acceptance Criteria

1. The feedman リポジトリ shall `package main` の `func main()` を持つエントリポイントを `cmd/feedman` に含む
2. When 開発者が `go build ./cmd/feedman` を実行したとき, the Go ビルド shall エラーなく完了し実行可能バイナリを生成する
3. The エントリポイント main shall 起動処理を既存の `internal/app` 起動機構に委譲する（独自のサブコマンド解釈ロジックを重複実装しない）

### Requirement 2: サブコマンドによる起動モードの切り替え

**Objective:** As a 運用者, I want ビルドしたバイナリに serve / worker などのサブコマンドを渡して起動モードを切り替えられること, so that 同一バイナリを API サーバとワーカーの双方に使える

#### Acceptance Criteria

1. When ビルドしたバイナリに `serve` 引数を渡して起動したとき, the feedman バイナリ shall API サーバモードで起動する
2. When ビルドしたバイナリに `worker` 引数を渡して起動したとき, the feedman バイナリ shall ワーカーモードで起動する
3. When ビルドしたバイナリに引数を渡さずに起動したとき, the feedman バイナリ shall 既定モード（serve）で起動する
4. When ビルドしたバイナリに既定サブコマンド集合外の引数を渡して起動したとき, the feedman バイナリ shall 既存仕様どおり既定モード（serve）にフォールバックして起動する

### Requirement 3: 起動失敗時のエラー観測性

**Objective:** As a 運用者, I want 起動・初期化に失敗したときにエラーが標準エラー出力に表れプロセスが非ゼロで終了すること, so that コンテナオーケストレータや CI が起動失敗を検出できる

#### Acceptance Criteria

1. If 起動機構が初期化中にエラーを返したとき, the feedman バイナリ shall そのエラーメッセージを標準エラー出力に出力する
2. If 起動機構がエラーを返したとき, the feedman バイナリ shall 非ゼロの終了コードでプロセスを終了する
3. When 起動機構がエラーを返さずに正常終了したとき, the feedman バイナリ shall 終了コード 0 でプロセスを終了する

### Requirement 4: コンテナイメージのビルド可能性

**Objective:** As a 運用者, I want root / web の Dockerfile と docker compose が成功裏にビルドできること, so that 全コンポーネントをコンテナとしてデプロイできる

#### Acceptance Criteria

1. When 運用者が root の `Dockerfile` でイメージをビルドしたとき, the Docker ビルド shall 成功し `/feedman` バイナリを含むイメージを生成する
2. When 運用者が `web/Dockerfile` で web イメージをビルドしたとき, the Docker ビルド shall 成功する
3. When 運用者が docker compose の build を実行したとき, the build shall api / worker / web の全サービスについて成功する

### Requirement 5: 再発防止のための CI 検出

**Objective:** As a 開発者, I want エントリポイント不在 / root Dockerfile ビルド不能が CI で機械的に検出されること, so that ビルド不能の状態が再びマージされるのを防ぐ

#### Acceptance Criteria

1. If `cmd/feedman` のエントリポイントが欠落しバックエンドがビルドできない状態になったとき, the CI shall ビルド検証ステップを失敗させる
2. If root の `Dockerfile` が前提とするエントリポイントを解決できずビルド不能になったとき, the CI shall 検証ステップを失敗させる
3. While エントリポイントが存在し root Dockerfile がビルド可能であるとき, the CI shall 当該検証ステップを成功させる

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The エントリポイント main shall 起動機構の既存サブコマンド解析・実行ロジック（`app.ParseCommand` / `app.Run`）を変更せずに利用する
2. The 変更 shall root `Dockerfile` の `ENTRYPOINT ["/feedman"]` および `CMD ["serve"]` を変更せずに動作させる
3. The 変更 shall docker compose の api / worker サービスの `command: ["serve"]` / `command: ["worker"]` を変更せずに動作させる
4. The 変更 shall 既存の docker healthcheck（`["/feedman", "healthcheck"]`）が引き続き機能する状態を維持する
5. When 既存の `go test ./...` を実行したとき, the テストスイート shall 引き続き成功する

### NFR 2: ランタイム制約

1. The ビルドされる feedman バイナリ shall CGO を無効化（`CGO_ENABLED=0`）した静的バイナリとしてビルド可能である
2. While distroless ランタイム（`gcr.io/distroless/static-debian12:nonroot`）上で実行されているとき, the feedman バイナリ shall 追加の動的ライブラリ依存なしで起動できる

## Out of Scope

- 新機能の追加
- `internal/app` の serve / worker / migrate / healthcheck ロジック自体の挙動変更（既存 `app.Run` / `app.ParseCommand` をそのまま利用する）
- `app.ParseCommand` の未知サブコマンド時のフォールバック挙動の変更（後述「確認事項」参照）
- デプロイ基盤（Cloudflare Tunnel 等）の設定
- CI でエントリポイント / Dockerfile ビルド不能を検出する具体的手段（`go build` ステップ追加か実 docker build ジョブ追加かなど）の確定 — observable な検出要件のみを定義し、手段は design / 実装に委ねる
- `web/Dockerfile` の内部実装の変更（ビルドが成功することのみを要件とする）

## Open Questions（確認事項）

- **未知サブコマンドの扱いに関する要件の矛盾**: Issue の AC 候補「未知サブコマンド／不正引数でエラー＋非ゼロ終了」と、スコープ外「`internal/app` の serve/worker ロジック変更」が両立しない。既存 `app.ParseCommand` は未知引数を **エラーにせず serve にフォールバック**する実装である。本 requirements は後方互換を優先し「未知引数は既存仕様どおり serve にフォールバック（Requirement 2.4）」を正とし、異常系は「起動機構が返したエラーの観測性（Requirement 3）」として定義した。これで意図に合致するか人間の判断を仰ぎたい。もし「未知サブコマンドを明示的にエラーにする」挙動が必須要件であれば、(a) `internal/app` 側のフォールバック挙動を変更する（スコープ外の解除）か、(b) main 側で独自に引数検証を行う（`ParseCommand` のロジック二重化となり「薄いラッパー」仮案と緊張する）かの trade-off の選択が必要であり、いずれも Issue 現スコープの再定義を要する。
- **CI 検出手段の選択**: Requirement 5 は「エントリポイント不在 / Dockerfile ビルド不能が CI で検出される」を observable な受入基準として定義し、具体手段（最小の `go build ./cmd/feedman` ステップ追加 / `docker_test.go` の実ビルド化 / 実 docker build ジョブの追加）は確定していない。手段によって CI 実行時間・依存（Docker daemon の要否）が変わるため、どこまでを必須とするか（最小の go build ステップで足りるのか、実 docker build まで CI で回すのか）について人間の方針判断を仰ぎたい。
