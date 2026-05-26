# Requirements Document

## Introduction

Feedman の CI（`.github/workflows/ci.yml`）には Go 依存（`govulncheck`）と
フロントエンド依存（`npm audit`）の脆弱性スキャンは存在するが、Dockerfile や
コンテナイメージ（ベースイメージ・OS パッケージ）由来の既知脆弱性を検知する仕組みが無い。
そのため `distroless` や `node:20-alpine` などのベースイメージ・OS パッケージに既知 CVE が
混入しても CI では検知されず見過ごされる。本要件では、ルート `Dockerfile`（api/worker）と
`web/Dockerfile`（web）を対象に Trivy による脆弱性スキャンジョブを CI へ追加し、PR 上で
スキャン結果を可視化する。検出時の挙動は「警告のみ（CI を fail させない）」、実行タイミングは
「PR トリガー」とすることが人間判断により確定済みである。

## Requirements

### Requirement 1: Trivy 脆弱性スキャンの CI 実行

**Objective:** As a Feedman のメンテナ, I want Dockerfile / コンテナイメージの脆弱性スキャンが CI で自動実行されること, so that ベースイメージや OS パッケージ由来の既知脆弱性を PR の段階で把握できる

#### Acceptance Criteria

1. The CI ワークフロー shall ルート `Dockerfile` を対象とする Trivy 脆弱性スキャンを実行する
2. The CI ワークフロー shall `web/Dockerfile` を対象とする Trivy 脆弱性スキャンを実行する
3. When スキャン対象に既知の CRITICAL または HIGH の脆弱性が含まれるとき, the CI ワークフロー shall 当該脆弱性を検出結果に含める
4. While スキャン対象に既知の重大脆弱性が存在しないとき, the CI ワークフロー shall スキャンジョブを正常終了として完了する

### Requirement 2: スキャン実行タイミング（PR トリガー）

**Objective:** As a Feedman のメンテナ, I want スキャンが PR の CI チェックに組み込まれること, so that マージ前に脆弱性の有無を PR 上で確認できる

#### Acceptance Criteria

1. When `main` または `develop` を対象ブランチとする pull_request イベントが発生したとき, the CI ワークフロー shall Trivy スキャンジョブを起動する
2. When `main` または `develop` への push イベントが発生したとき, the CI ワークフロー shall 既存ジョブと整合する形でワークフローを起動する
3. The Trivy スキャンジョブ shall PR の CI チェック一覧に表示されるジョブとして実行される

### Requirement 3: 検出時の挙動（警告のみ・PR をブロックしない）

**Objective:** As a Feedman のメンテナ, I want 脆弱性検出時に CI を fail させず結果を残すだけにすること, so that 段階導入の初期段階で開発フローを止めずに脆弱性を可視化できる

#### Acceptance Criteria

1. If スキャンで CRITICAL または HIGH の脆弱性が検出されたとき, the Trivy スキャンジョブ shall ジョブを失敗（fail）させずに正常終了する
2. When スキャンが完了したとき, the Trivy スキャンジョブ shall 検出結果を CI 上で参照可能な形（アノテーションまたはジョブサマリー）として残す
3. If スキャンで脆弱性が検出されたとき, the Trivy スキャンジョブ shall その検出によって PR のマージをブロックしない

### Requirement 4: 既存 CI ジョブへの非影響

**Objective:** As a Feedman のメンテナ, I want Trivy ジョブ追加が既存ジョブの結果に影響しないこと, so that 既存の Backend / Frontend / 依存脆弱性スキャンの判定が変わらない

#### Acceptance Criteria

1. The CI ワークフロー shall 既存の backend / go-vet / govulncheck / frontend / frontend-lint / frontend-audit 各ジョブを従来どおり実行する
2. When Trivy スキャンジョブが脆弱性を検出したとき, the CI ワークフロー shall 既存の各ジョブの成否判定を変化させない
3. The Trivy スキャンジョブ shall 既存の依存脆弱性スキャン（govulncheck / npm audit）が対象とするスコープと重複しない対象（Dockerfile / コンテナイメージ）を扱う

## Non-Functional Requirements

### NFR 1: 実行コスト・性能

1. The Trivy スキャンジョブ shall pull_request での CI 実行時間が既存ジョブの最長実行時間を大幅に超えない範囲で完了する（目安として 1 ジョブあたり 5 分以内）
2. Where スキャン所要時間が pull_request 実行の許容範囲を超える場合, the スキャン構成 shall 軽量なスキャンモードまたは実行頻度の調整で実行時間を抑制できる選択肢を持つ

### NFR 2: 後方互換性

1. The CI ワークフロー shall 本変更導入前から存在する全ジョブの成否判定ロジックを変更しない
2. The Trivy スキャンジョブ shall 既存ジョブの実行可否や成否に依存せず独立して実行される

### NFR 3: 可観測性

1. When Trivy スキャンジョブが完了したとき, the スキャンジョブ shall メンテナが検出された脆弱性の重大度（CRITICAL / HIGH 等）と件数を CI 上で確認できる出力を残す

## Out of Scope

- Go の静的解析・依存脆弱性スキャン（`go vet` / `govulncheck`。別 Issue / 既存ジョブ）
- フロントエンドの静的解析・依存脆弱性スキャン（ESLint / `npm audit`。別 Issue / 既存ジョブ）
- 検出された脆弱性の自動修復・ベースイメージの自動更新
- CRITICAL のみを対象とした PR ブロック（fail-on-critical）への段階移行（将来 Issue で別途検討）
- 定期実行（schedule トリガー）を本 Issue の主トリガーとすること（PR トリガーが確定方針。schedule は NFR 1.2 の選択肢にとどめ、本 Issue では追加要件としない）
- Dockerfile / コンテナイメージ以外のスキャン対象（IaC / シークレットスキャン等）

## Open Questions

- なし（検出時挙動「警告のみ」・実行タイミング「PR トリガー」はいずれも Issue コメントで人間判断により確定済み）
