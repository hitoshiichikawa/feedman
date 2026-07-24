# Requirements Document

## Introduction

CI の `Go Vulnerability Scan (govulncheck)` ジョブが 2026-07-23 以降、新規公開されたアドバイザリ
**GO-2026-5997**（`github.com/doyensec/safeurl` v0.2.2 の blocklist に IPv6 CIDR レンジが欠けており、
v0.2.4 で修正済み）を検出して fail しており、develop 宛て全 PR の CI が赤くなって idd-claude
ワークフローのマージ判断を阻害している。到達可能な symbol も `internal/security/ssrf_guard.go` 等で
実測されているため単なる依存検出ではない。本 Issue は `safeurl` を v0.2.4 以上へ更新して当該
advisory を解消し、CI ゲートを green に戻すことを目的とする。同時に SSRF ガードの既存挙動
（許可 scheme / port / timeout、プライベートアドレス遮断）が upgrade によって退行しないことを
保証する。

## Requirements

### Requirement 1: safeurl の脆弱性解消

**Objective:** As a 開発者, I want `github.com/doyensec/safeurl` を GO-2026-5997 が修正された
バージョン以上に更新すること, so that CI の依存脆弱性スキャンが当該 advisory を検出せずに済む

#### Acceptance Criteria

1. The Go モジュール定義 shall `github.com/doyensec/safeurl` の要求バージョンを v0.2.4 以上に指定する
2. The Go モジュールのロックファイル shall 実際に解決された `github.com/doyensec/safeurl` の
   バージョンとして v0.2.4 以上のエントリを含む
3. When 依存脆弱性スキャンをリポジトリの全パッケージに対して実行したとき, the 依存脆弱性スキャン
   shall GO-2026-5997 を検出しない
4. When 依存脆弱性スキャンをリポジトリの全パッケージに対して実行したとき, the 依存脆弱性スキャン
   shall ゼロ終了で完了する

### Requirement 2: 既存ビルド・テストの後方互換

**Objective:** As a 開発者, I want safeurl の更新後も既存のテスト・静的解析・整形チェックがすべて
従来どおり通ること, so that 依存更新に伴う退行を検知できる

#### Acceptance Criteria

1. When リポジトリの全パッケージに対してユニットテストを実行したとき, the テストランナー shall
   全テストを成功として完了する
2. When リポジトリの全パッケージに対して Go 静的解析を実行したとき, the 静的解析 shall 違反を
   0 件で完了する
3. When リポジトリの Go ソースに対して整形チェックを実行したとき, the 整形チェック shall 差分を
   検出しない
4. If 既存の SSRF ガード関連テストが safeurl 更新後に失敗した場合, the 対応作業 shall テスト側を
   弱めず、失敗内容を確認事項として PR 本文または Issue コメントで報告する

### Requirement 3: SSRF ガード挙動の維持

**Objective:** As a 運用者, I want safeurl 更新後も SSRF ガードの外部から観測可能な既存挙動が
維持されること, so that 外向き HTTP 取得のセキュリティ境界が退行しない

#### Acceptance Criteria

1. The SSRF ガード shall 更新前と同じ許可 scheme（http / https）以外の URL に対する外向き取得を
   拒否する
2. The SSRF ガード shall 更新前と同じ許可ポート範囲以外への外向き取得を拒否する
3. The SSRF ガード shall プライベート／ループバック／リンクローカル等の内部向けアドレスに対する
   外向き取得を拒否する
4. The SSRF ガード shall 更新前と同じタイムアウト境界で外向き取得を打ち切る

## Non-Functional Requirements

### NFR 1: CI 所要時間

1. The CI パイプライン shall 依存更新後も `Go Vulnerability Scan (govulncheck)` ジョブおよび
   バックエンドテストジョブの完了までの所要時間を、更新前の直近成功時と比較して有意に増やさない
   （目安として 20% 以内の増加に収める）

### NFR 2: スコープ限定性

1. The 変更対象 shall Go モジュール定義（`go.mod`）と Go モジュールのロックファイル（`go.sum`）
   に限定し、`internal/security/ssrf_guard.go` を含むアプリケーションコードおよびテストコードを
   変更しない

### NFR 3: 検出結果の追跡可能性

1. When `Go Vulnerability Scan (govulncheck)` ジョブが更新後に成功したとき, the CI ログ shall
   GO-2026-5997 に関する検出行を含まない状態で終了する

## Out of Scope

- npm audit 側の赤の解消（別 Issue で扱う）
- `internal/security/ssrf_guard.go` のロジック変更・設定変更（許可 scheme / port / timeout /
  プライベートアドレス遮断範囲の変更を含む）
- `safeurl` 以外の Go 依存パッケージのバージョン更新
- Go toolchain バージョンの変更
- CI ワークフロー（`.github/workflows/ci.yml`）の構成変更・追加ステップ導入
- 新規テストの追加（既存 SSRF ガードテストの実行のみで退行検知する）

## Open Questions

- なし

## 関連

- Related: #212
- Related: #213
