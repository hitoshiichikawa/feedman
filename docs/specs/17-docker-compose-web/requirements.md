# Requirements Document

## Introduction

`docker-compose.yml` の `web` サービスにだけログローテーション設定（`logging` ブロック）が無く、
`api` / `worker` のように上限が効かないため、長期稼働で `web` のコンテナログがディスクを
際限なく消費する恐れがある。本要件は `web` サービスにも `api` / `worker` と整合する
ログローテーション設定を追加し、ログのディスク消費量に上限を設けることを目的とする。
編集対象は `docker-compose.yml` の `web` サービス定義のみで、`api` / `worker` / `db` の
既存設定や集中ログ基盤への転送は扱わない。

## Requirements

### Requirement 1: web サービスのログローテーション設定

**Objective:** As a 運用者, I want `web` サービスのコンテナログをローテーションさせたい, so that 長期稼働でもログがディスクを際限なく消費しないようにできる

#### Acceptance Criteria

1. The `web` サービス定義 shall ログローテーションを行う `logging` 設定を持つ
2. The `web` サービスのログ shall 1 ファイルあたりのサイズが `10m` を超えた時点で新しいログファイルへローテーションされる
3. The `web` サービスのログ shall 保持するログファイル数を `14` 件までに制限する

### Requirement 2: api / worker との設定値整合

**Objective:** As a 運用者, I want `web` のログ上限値を `api` / `worker` と揃えたい, so that サービス間でログ運用の挙動が一貫し、設定の認知負荷を下げられる

#### Acceptance Criteria

1. The `web` サービスのログローテーション設定 shall 1 ファイルあたりのサイズ上限を `api` / `worker` と同じ `10m` とする
2. The `web` サービスのログローテーション設定 shall 保持ファイル数の上限を `api` / `worker` と同じ `14` とする
3. While `api` / `worker` の既存ログ設定値が変更されていない状態で, the `web` サービスのログ設定 shall それらと等価な上限値で運用される

### Requirement 3: ログ上限到達時の破棄挙動

**Objective:** As a 運用者, I want 上限を超えたログを自動で破棄させたい, so that ディスク使用量を一定の範囲内に保てる

#### Acceptance Criteria

1. When `web` サービスのログ蓄積量がサイズ上限と保持ファイル数の上限（`10m` × `14`）に達したとき, the `web` サービスのログ管理 shall 最も古いログから順に破棄する
2. While ログが上限に達して破棄が継続している状態で, the `web` サービスのログ保持総量 shall サイズ上限と保持ファイル数の積で定まる上限を超えない

### Requirement 4: 後方互換（既存サービスの起動・稼働維持）

**Objective:** As a 運用者, I want ログ設定追加後も既存どおりに各サービスが起動・稼働してほしい, so that 既存環境を壊さずに設定変更を取り込める

#### Acceptance Criteria

1. When ログ設定追加後に compose 環境を起動したとき, the `web` サービス shall 従来どおり正常に起動し稼働する
2. The `api` / `worker` / `db` サービスの定義 shall 本変更によって変更されない
3. If `web` サービスへの今回のログ設定追加以外の差分が生じたとき, the 変更内容 shall 後方互換違反として扱われ拒否される

## Non-Functional Requirements

### NFR 1: ディスク消費の上限

1. The `web` サービスのコンテナログ shall 1 サービスあたり最大でも `10m` × `14`（= 約 140MB）を超えてディスクを消費しない

### NFR 2: 互換性

1. The 本変更 shall `api` / `worker` の既存ログ設定値（`max-size: 10m` / `max-file: 14`）を変更しない
2. The 本変更 shall `web` サービスの既存の `ports` / `environment` / `networks` / `depends_on` / `restart` / `deploy` 設定を変更しない

## Out of Scope

- 集中ログ基盤（ELK / Loki 等）へのログ転送機構の導入
- `api` / `worker` の既存ログ設定値の変更
- `db` サービスへのログローテーション設定の追加
- ログのサイズ上限・保持件数の閾値そのものの見直し（`api` / `worker` と整合させる方針を維持する）

## Open Questions

- なし（Issue 本文・コメント・既存 `docker-compose.yml` の確認により、設定値・整合対象・スコープが確定している）
