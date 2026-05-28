# Requirements Document

## Introduction

Feedman の CI（`.github/workflows/ci.yml`）の `frontend` ジョブは現在 `npm ci` + `npm test` のみを実行しており、フロントエンド（`web/`）の依存パッケージ脆弱性スキャン（`npm audit`）と静的解析（eslint）が CI に組み込まれていない。このため脆弱な npm 依存や lint で検出可能な問題が PR 段階で検知されず、main に混入し得る。本要件は umbrella #40 をフロントエンド側に分割したもので、PR の作成・更新時にフロントエンドの脆弱性スキャンと lint を自動実行し、結果を CI に反映させることをゴールとする。バックエンド側には既に `govulncheck`（脆弱性検出でジョブを失敗させるブロッキング方式）が存在しており、フロントエンド側もこの既存方針との一貫性を取る。

## Requirements

### Requirement 1: フロントエンド lint（eslint）の CI 実行

**Objective:** As a リポジトリのメンテナ, I want PR 上でフロントエンドの eslint が自動実行されること, so that lint で検出可能な問題が main に混入する前に検知できる

#### Acceptance Criteria

1. When PR が `main` または `develop` 向けに作成または更新されたとき, the CI shall `web/` 配下で `npm run lint`（eslint）を実行する
2. If eslint が lint 違反を検出したとき, the CI shall 該当ジョブを失敗させ PR チェックを fail 状態にする
3. When eslint が lint 違反を検出しなかったとき, the CI shall 該当ジョブを成功させる
4. The CI shall lint の実行結果（違反内容を含むログ）を当該ジョブのログに出力する

### Requirement 2: フロントエンド依存脆弱性スキャン（npm audit）の CI 実行

**Objective:** As a リポジトリのメンテナ, I want PR 上でフロントエンドの依存脆弱性スキャンが自動実行されること, so that 脆弱な npm 依存が main に混入する前に検知できる

#### Acceptance Criteria

1. When PR が `main` または `develop` 向けに作成または更新されたとき, the CI shall `web/` 配下で `npm audit` を実行する
2. The CI shall `npm audit` の閾値を `--audit-level=high` に設定して実行する
3. If `npm audit` が `high` 以上（high または critical）の脆弱性を検出したとき, the CI shall 該当ジョブを失敗させ PR チェックを fail 状態にする
4. While 検出された脆弱性が `high` 未満（moderate / low / info）のみであるとき, the CI shall 該当ジョブを成功させる
5. When `npm audit` が `high` 以上の脆弱性を検出しなかったとき, the CI shall 該当ジョブを成功させる
6. The CI shall `npm audit` の実行結果（検出された脆弱性の一覧を含むログ）を当該ジョブのログに出力する

### Requirement 3: 既存 CI ジョブとの後方互換性

**Objective:** As a リポジトリのメンテナ, I want 新しい lint / audit の追加が既存の CI ジョブを壊さないこと, so that 既存のテスト・静的解析が従来どおり機能し続ける

#### Acceptance Criteria

1. The CI shall 既存の `npm test`（フロントエンドテスト）を引き続き実行し、変更前と同一の成否判定を維持する
2. The CI shall 既存の `backend` / `go-vet` / `govulncheck` の各ジョブを変更前と同一の挙動で維持する
3. Where lint または audit のジョブが失敗したとき, the CI shall その失敗を `npm test` の成否とは独立に判定する

## Non-Functional Requirements

### NFR 1: 一貫性と誤検知への配慮

1. The CI shall フロントエンドの脆弱性スキャンを、バックエンドの `govulncheck` と同じく「閾値以上の検出でブロッキングする」方針で運用する
2. While `npm audit` の transitive 依存由来の検出が含まれるとき, the CI shall `--audit-level=high` の閾値による足切りを適用し、moderate 以下の検出ではブロックしない（誤検知・更新待ちの多い低重大度検出での PR ブロックを回避するため）

### NFR 2: 可観測性

1. The CI shall lint ジョブと audit ジョブの成否を、GitHub の PR チェック一覧で他ジョブと区別可能な独立したステータスとして表示する

## Out of Scope

- Go 側の静的解析・脆弱性スキャン（既存 `go-vet` / `govulncheck` ジョブで対応済み。本 Issue では変更しない）。
- コンテナイメージスキャン（Trivy 等。別 Issue）。
- `npm audit fix` 等による脆弱性の自動修正（検知のみを対象とする）。
- eslint ルールセット自体の追加・変更（既存 `web/eslint.config.mjs` / `eslint-config-next` の設定をそのまま用いる）。
- 検出された脆弱性に対する例外許可リスト（allowlist）機構の導入。
- `develop` 以外のブランチや push トリガーに対する実行ポリシーの変更（既存 `ci.yml` のトリガー設定を踏襲する）。

## Open Questions

本 Issue 本文「判断を委ねたい点」（block/warn ポリシー・`--audit-level` 閾値）について人間からの追加回答コメントは存在しないため、PM が以下の defensible なデフォルトを選定し、本 requirements に明記した。残課題はなし。

- npm audit の block/warn 方針: **`high` 以上の検出でブロッキング**（Req 2.3、NFR 1.1）。
  - 根拠: 既存 `govulncheck` ジョブが脆弱性検出でジョブを失敗させるブロッキング方式であり、フロントエンド側もこれと一貫させる。
  - 却下した代替案（警告のみ）: 脆弱性を検知しても PR がマージ可能だと混入リスクが残るため不採用。
- `--audit-level` 閾値: **`high`**（Req 2.2）。
  - 根拠: transitive 依存由来の low / moderate 検出は誤検知・更新待ちが多く、これらでブロックすると PR 運用が頻繁に阻害される。`high` で足切りすることで重大な脆弱性に限定してブロックする（NFR 1.2）。
  - 却下した代替案（`critical` のみ）: high の脆弱性（実害が大きいものを含む）を見逃すため、`critical` より緩い `high` を採用。
