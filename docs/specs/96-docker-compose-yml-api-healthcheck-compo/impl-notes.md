# 実装ノート（Issue #96）

## 概要

`docker-compose.yml` の `api` サービスの healthcheck がリスト形式 `test: ["/feedman", "healthcheck"]`
で記述されており、Compose 仕様（リスト形式 `healthcheck.test` の先頭要素は `CMD` / `CMD-SHELL` /
`NONE` のいずれか）に違反していたため、スタックが起動できない不具合を修正した。

## 修正内容

`docker-compose.yml` の `api` サービスの healthcheck.test に `CMD` を前置した（1 行のみの修正）。

```yaml
# 修正前（Compose 仕様違反）
healthcheck:
  test: ["/feedman", "healthcheck"]
  interval: 10s
  timeout: 5s
  retries: 3

# 修正後（Compose 仕様準拠）
healthcheck:
  test: ["CMD", "/feedman", "healthcheck"]
  interval: 10s
  timeout: 5s
  retries: 3
```

- `interval` / `timeout` / `retries` は変更していない（Req 2.1〜2.3）。
- `db` サービスの healthcheck（`["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-feedman}"]`）は
  既に Compose 仕様準拠のため変更していない（Req 3.2）。
- `web` / `worker` は healthcheck 未定義のため対象外。
- サービス構成（api / web / worker / db）・ネットワーク（internal / external）・`depends_on` は
  本修正前と同一（NFR 1）。

## 検証結果

本環境では `docker` / `docker compose` が利用可能だったため、実コマンドで検証した。

- 環境: Docker version 29.4.3 / Docker Compose version v5.1.3
- 実行コマンド: `SESSION_SECRET=dummy POSTGRES_PASSWORD=dummy docker compose -f docker-compose.yml config`
  - 終了コード: `RC=0`（正常終了。Req 1.3）
  - `healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` エラーは出力されなかった
  - stderr は `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` 未設定の warning のみで、healthcheck とは無関係
- `docker compose config` の解決結果（healthcheck 抜粋）:
  - api: `test: [CMD, /feedman, healthcheck]` / `interval: 10s` / `timeout: 5s` / `retries: 3`（Req 1.1, 1.2, 2.x）
  - db: `test: [CMD-SHELL, pg_isready -U feedman]` / `interval: 5s` / `timeout: 5s` / `retries: 5`（未変更 / Req 3.2）
- Req 3.1（同種違反の不在確認）: リスト形式 `healthcheck.test` を持つサービスは api / db の 2 件のみで、
  それぞれ先頭要素が `CMD` / `CMD-SHELL` であることを `config` 解決結果および元 YAML の grep で確認した。
  CMD/CMD-SHELL/NONE 前置のないリスト形式 healthcheck は残存しない。

> Req 1.4（`docker compose up` でのスタック起動）は、イメージビルド・外部依存・OAuth クレデンシャル等の
> 実行時前提が必要で本検証環境では完結しないため、healthcheck 記法の妥当性を担保する
> `docker compose config`（rc=0 / 記法エラー無し）で代替検証とした。記法起因の起動失敗が解消された
> ことは config レベルで確認済み。

## 受入基準とテスト/検証の対応

| Req ID | 内容 | 担保方法 |
|---|---|---|
| 1.1 | healthcheck.test 先頭が CMD/CMD-SHELL/NONE | `config` 解決結果で `test: [CMD, ...]` を確認 |
| 1.2 | コンテナ内で `/feedman healthcheck` 実行を保持 | `config` 解決結果で `[CMD, /feedman, healthcheck]` を確認 |
| 1.3 | `config` が記法エラーなく正常終了 | `RC=0`・該当エラー無しを確認 |
| 1.4 | `up` で記法起因エラーなく起動 | `config` rc=0 で代替検証（上記注記参照） |
| 2.1 | interval = 10s | `config` 解決結果で確認 |
| 2.2 | timeout = 5s | `config` 解決結果で確認 |
| 2.3 | retries = 3 | `config` 解決結果で確認 |
| 3.1 | 全リスト形式 healthcheck が CMD/CMD-SHELL/NONE 前置 | grep + `config` 解決結果（api / db のみ）で確認 |
| 3.2 | db の healthcheck 定義を変更しない | 差分が api の 1 行のみであることと db 解決結果が従来どおりであることを確認 |
| NFR 1 | サービス構成・ネットワーク・depends_on を同一に保つ | 差分が healthcheck.test 1 行のみで他要素に変更なし |

## 確認事項

- なし（修正は api healthcheck.test の 1 行のみで、requirements.md の Out of Scope と整合している）

## 残課題 / 派生タスク

- なし

STATUS: complete
