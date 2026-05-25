# Requirements Document

## Introduction

`go.mod` の `go` ディレクティブは現在パッチバージョンまで（`go 1.25.1`）を指定している。
これにより、当該パッチ未満の Go ツールチェインを使う CI ランナーや開発者環境が不必要に
ビルド・テストから締め出される。Go 1.21 以降は `go` ディレクティブをメジャー.マイナー
（例 `go 1.25`）で記述し、パッチ固定が必要な場合は `toolchain` ディレクティブを併用する
ことが推奨されている。本要件は、`go` ディレクティブをメジャー.マイナーへ丸めつつ、
ローカル・CI 双方でビルド／テストが従来どおり成功する状態を維持することを目的とする。

## Requirements

### Requirement 1: go ディレクティブのメジャー.マイナー丸め

**Objective:** As a 開発者, I want `go.mod` の `go` ディレクティブがメジャー.マイナー形式で指定されること, so that 不必要にパッチ未満のツールチェインを締め出さずに開発・CI を回せる

#### Acceptance Criteria

1. The ビルド設定（`go.mod`）shall `go` ディレクティブをメジャー.マイナー形式（例 `go 1.25`）で記述する
2. The ビルド設定（`go.mod`）shall `go` ディレクティブにパッチバージョン（例 `go 1.25.1`）を含めない
3. The ビルド設定（`go.mod`）shall 既存コードが依存する Go 言語機能・標準ライブラリ API をビルド可能なメジャー.マイナー系列を維持する

### Requirement 2: ローカル・CI でのビルド／テスト成功

**Objective:** As a 開発者, I want 変更後の `go.mod` でビルドとテストが従来どおり通ること, so that バージョン表記変更によるデグレを起こさず安心して merge できる

#### Acceptance Criteria

1. When `go.mod` のバージョン表記をメジャー.マイナーへ変更した後にローカルでビルドを実行したとき, the ビルドシステム shall 成功を返す
2. When `go.mod` のバージョン表記をメジャー.マイナーへ変更した後にローカルでテストを実行したとき, the テストシステム shall 既存の全テストを成功させる
3. When `go.mod` のバージョン表記をメジャー.マイナーへ変更した後に CI が `go-version-file: go.mod` を参照してツールチェインを解決したとき, the CI パイプライン shall ビルド・テストジョブを成功させる
4. If CI が解決したツールチェインのパッチバージョンが従来固定値と異なる場合でも, the CI パイプライン shall ビルド・テストジョブを成功させる

### Requirement 3: toolchain 併記時の互換性（境界値）

**Objective:** As a 開発者, I want パッチ固定が必要な場合に `toolchain` ディレクティブを併記してもビルド／テストが通ること, so that メジャー.マイナー丸めとパッチ固定の両立が必要になっても破綻しない

#### Acceptance Criteria

1. Where `toolchain` ディレクティブ（例 `toolchain go1.25.1`）が `go.mod` に併記される場合, the ビルド設定（`go.mod`）shall `go` ディレクティブをメジャー.マイナー形式のまま保持する
2. Where `toolchain` ディレクティブが `go.mod` に併記される場合, the ビルドシステム shall ローカルでのビルド・テストを成功させる
3. Where `toolchain` ディレクティブが `go.mod` に併記される場合, the CI パイプライン shall ビルド・テストジョブを成功させる

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The ビルド設定（`go.mod`）shall 変更前にビルド・実行できていた `api` / `worker` コンポーネントを、変更後も同一のメジャー.マイナー系列でビルド可能な状態に保つ
2. The CI パイプライン shall 変更前後で同一のジョブ構成（`go-version-file: go.mod` 参照）を維持し、ワークフロー定義の追加変更なしに成功する

### NFR 2: ツールチェイン許容範囲

1. The ビルド設定（`go.mod`）shall メジャー.マイナーで指定したバージョン以上の任意のパッチを持つ Go ツールチェインでビルドを許容する

## Out of Scope

- 依存ライブラリ（`require` ブロック）のバージョン更新
- Go メジャー.マイナー系列そのものの引き下げ判断の確定（必要性の検討提案に留め、引き下げの確定は本要件のスコープ外）
- `.github/workflows/ci.yml` のジョブ構成・ステップの再設計（`go-version-file` 参照方式は現状維持）
- フロントエンド（`web`）のツールチェイン・Node バージョン指定

## Open Questions

- パッチ固定（`toolchain` 併記）を実際に採用するか否か。本要件では「併記しても破綻しない」ことを境界値として保証するに留めており、採用要否の確定判断は人間レビューに委ねる（Requirement 3 は併記時の互換性を担保する条件付き要件）。
