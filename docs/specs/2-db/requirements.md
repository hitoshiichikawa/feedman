# Requirements Document

## Introduction

`internal/database/db.go` の `Open` 関数は `database/sql` の接続プールパラメータを一切設定して
おらず、Go のデフォルト（`MaxOpenConns = 0` = 無制限）のまま動作している。Feedman は api / worker
の 2 プロセスがそれぞれ独立して同一の `Open` を呼び出してプールを保持するため、高負荷時には
両プロセス合算の同時接続数が PostgreSQL の `max_connections`（デフォルト 100）を超過し、新規接続の
確立に失敗する恐れがある。本要件では、接続プールに上限・アイドル数・接続寿命の上限を与え、
2 プロセス合算でも PostgreSQL の上限を超えない範囲に収めることを目的とする。既存の `Open` 呼び出し側
（api / worker）の後方互換を保つことを前提とする。

## Requirements

### Requirement 1: 接続プールの上限設定

**Objective:** As a Feedman の運用者, I want DB 接続プールに同時接続数の上限が設定されていること, so that 高負荷時でも PostgreSQL への接続が無制限に増加して接続枯渇を引き起こさない

#### Acceptance Criteria

1. When Database Open 関数がデータベース接続を開いたとき, the 接続プール shall 同時接続数（MaxOpenConns）に有限の上限値を設定する
2. The 接続プール shall 同時接続数の上限を無制限（0）以外の正の有限値とする
3. When api プロセスと worker プロセスが同時稼働しているとき, the 接続プール shall 両プロセス合算の同時接続数上限が PostgreSQL の `max_connections`（既定 100）を超えない値とする

### Requirement 2: アイドル接続の上限設定

**Objective:** As a Feedman の運用者, I want アイドル接続の保持数に上限があること, so that 利用されていない接続が際限なく保持されてリソースを浪費しない

#### Acceptance Criteria

1. When Database Open 関数がデータベース接続を開いたとき, the 接続プール shall アイドル接続数（MaxIdleConns）に有限の上限値を設定する
2. The 接続プール shall アイドル接続数の上限を同時接続数の上限以下の値とする

### Requirement 3: 接続寿命の上限設定

**Objective:** As a Feedman の運用者, I want 接続に寿命の上限が設定されていること, so that 長寿命接続が一定時間で再確立され、ネットワーク機器や DB 側のタイムアウトによる断線が顕在化しない

#### Acceptance Criteria

1. When Database Open 関数がデータベース接続を開いたとき, the 接続プール shall 接続の最大寿命（ConnMaxLifetime）に有限の時間上限を設定する
2. The 接続プール shall 接続の最大寿命を正の有限の時間値とする

### Requirement 4: 設定値の定数化と後方互換

**Objective:** As a Feedman の開発者, I want プール設定値が定数として明示され既存の呼び出し側を壊さないこと, so that 設定値の意図が読み取れ、api / worker の既存コードを変更せずに改善を取り込める

#### Acceptance Criteria

1. The 接続プール設定値 shall 直書きのマジックナンバーではなく名前付き定数として定義される
2. The Database Open 関数 shall 既存の呼び出し側が依存する関数シグネチャ（引数・返り値の型）を変更しない
3. When 既存の api / worker から Database Open 関数が呼び出されたとき, the Database Open 関数 shall 呼び出し側のコード変更を要さずに接続プール設定が適用された `*sql.DB` を返す

## Non-Functional Requirements

### NFR 1: 接続枯渇耐性

1. While api と worker の両プロセスが稼働しているとき, the 接続プール shall 両プロセス合算の同時接続数を PostgreSQL `max_connections`（既定 100）以下に保ち、上限超過に起因する接続確立失敗を発生させない

### NFR 2: 後方互換性

1. The 接続プール設定の追加 shall 既存の api / worker 起動フロー（`Open` → `Ping` → 依存ワイヤリング）の観測可能な挙動を維持し、設定追加以前に成功していた起動が引き続き成功する

## Out of Scope

- PostgreSQL 側 `max_connections` のチューニングやインフラ設定変更
- 接続プールのメトリクス収集・監視基盤の追加（Prometheus 等への露出）
- `Open` の引数追加によるプール設定の呼び出し側からの注入（シグネチャ変更を伴うため本 Issue では扱わない）
- 接続リトライ／バックオフ等の接続確立失敗時のリカバリ戦略

## Open Questions

- 同時接続数の上限値の確定: 仮案は api 25 + worker 25 = 50（≤ 100）。本要件では「2 プロセス合算が
  `max_connections` を超えない有限値」とのみ規定し、具体値（25 など）は設計／実装フェーズで確定する
  余地を残す。25 で確定してよいか、将来のスケール（複数 worker インスタンス等）を見込んでより低い値に
  すべきかは Architect / 運用者の判断を仰ぐ。
- 設定値を環境変数で上書き可能にするか: 既存 `internal/config` は多くの設定値を環境変数 + デフォルト値で
  管理している（例: `FETCH_MAX_CONCURRENT`）。プール設定値も同様に環境変数化するか、`db.go` 内の定数固定と
  するかは、シグネチャ後方互換（Requirement 4.2）の制約下での実現方式を含め設計判断とする。本要件では
  「定数として明示される」までを必須とし、環境変数化は必須要件に含めない。
- api と worker でプール設定値を変える必要があるか: 現状は共通の `Open` を両者が呼ぶため同一値となる。
  プロセスごとに負荷特性が異なる場合に値を分けるべきかは Open Question として残す（分ける場合は
  シグネチャ後方互換との整合を要検討）。
