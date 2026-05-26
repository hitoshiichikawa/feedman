# Requirements Document

## Introduction

`docker-compose.yml` の `api` サービスの healthcheck がリスト形式 `test: ["/feedman", "healthcheck"]` で記述されており、Compose 仕様に違反している。Compose 仕様ではリスト形式の `healthcheck.test` は先頭要素が `CMD` / `CMD-SHELL` / `NONE` のいずれかでなければならず、現状のままでは `docker compose config` / `docker compose up` が `healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` エラーで失敗し、スタックを起動できない。本要件は当該違反を解消し、Compose によるスタック起動を回復させることを目的とする。なお `db` サービスの healthcheck は既に Compose 仕様準拠（`["CMD-SHELL", ...]`）であり修正対象外である。

## Requirements

### Requirement 1: api サービス healthcheck の Compose 仕様準拠化

**Objective:** As a 運用者, I want api サービスの healthcheck 定義を Compose 仕様準拠の形式にしたい, so that スタックを `docker compose` で正常に起動できる

#### Acceptance Criteria

1. The api サービスの healthcheck.test shall リスト形式の先頭要素が `CMD` / `CMD-SHELL` / `NONE` のいずれかである Compose 仕様準拠の形式である
2. The api サービスの healthcheck shall コンテナ内で `/feedman healthcheck` を実行する内容を保持する
3. When healthcheck 定義を修正した状態で `docker compose -f docker-compose.yml config` を実行したとき, the docker compose shall `healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` エラーを出さずに正常終了する
4. When healthcheck 定義を修正した状態で `docker compose up` を実行したとき, the docker compose shall healthcheck 記法に起因するエラーなしでスタックを起動する

### Requirement 2: 既存 healthcheck パラメータの維持

**Objective:** As a 運用者, I want healthcheck の検査周期・タイムアウト・リトライ回数を従来どおり維持したい, so that ヘルスチェックの挙動が本修正前と等価に保たれる

#### Acceptance Criteria

1. The api サービスの healthcheck shall interval を `10s` に維持する
2. The api サービスの healthcheck shall timeout を `5s` に維持する
3. The api サービスの healthcheck shall retries を `3` に維持する

### Requirement 3: 同種違反の不在確認

**Objective:** As a 運用者, I want docker-compose.yml 全体で CMD 前置のないリスト形式 healthcheck が残っていないことを確認したい, so that 同種の Compose 仕様違反による起動失敗が再発しないことを保証できる

#### Acceptance Criteria

1. The docker-compose.yml shall リスト形式 `healthcheck.test` を持つすべてのサービスについて先頭要素が `CMD` / `CMD-SHELL` / `NONE` のいずれかである
2. While db サービスの healthcheck が既に Compose 仕様準拠である状態で, the 修正作業 shall db サービスの healthcheck 定義を変更しない

## Non-Functional Requirements

### NFR 1: 互換性

1. The 修正後の docker-compose.yml shall api / web / worker / db の 4 サービス構成・ネットワーク設計・依存関係（depends_on）を本修正前と同一に保つ

## Out of Scope

- `db` / `web` / `worker` サービスの healthcheck・設定変更（`db` は既に仕様準拠、`web` / `worker` は healthcheck 未定義）
- healthcheck の検査内容（`/feedman healthcheck` サブコマンドそのもの）の挙動変更
- interval / timeout / retries の値そのものの見直し（現行値を維持する）
- override ファイル（未 commit）や他環境向け compose ファイルの追加・修正
- `/feedman healthcheck` サブコマンドの実装変更

## Open Questions

- なし
