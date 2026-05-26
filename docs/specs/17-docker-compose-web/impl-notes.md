# 実装ノート: Issue #17 docker-compose の web サービスにログローテーション設定を追加

## 概要

`docker-compose.yml` の `web` サービス定義に、`api` / `worker` と整合する `logging` ブロックを
追加した。これにより `web` のコンテナログにもサイズ上限（`max-size: 10m`）と保持ファイル数
上限（`max-file: 14`）が効くようになり、長期稼働でのディスク際限消費を防ぐ。

本 Issue には `design.md` / `tasks.md` が存在しない design-less impl のため、`requirements.md` の
各 AC を直接 1:1 で検証して実装した。

## 実施した変更

- `docker-compose.yml` の `web` サービス定義に以下の `logging` ブロックを追加（`environment`
  ブロックの直後、`networks` の直前。`api` サービスでの配置順と整合）:

```yaml
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "14"
```

- `web` の既存設定（`build` / `ports` / `environment` / `networks` / `depends_on` / `restart`
  / `deploy`）は変更していない。
- `api` / `worker` / `db` サービス定義は変更していない。
- diff は `web` サービスへの `logging` ブロック 5 行追加のみ（後述の検証 4 参照）。

## 検証コマンドと結果

### 1. compose ファイルの構文妥当性

`docker compose config`（Docker Compose v5.1.3）で構文検証を試みたが、以下のエラーで停止した:

```
healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"
```

これは `api` サービスの既存 healthcheck 定義（`test: ["/feedman", "healthcheck"]`）に対する
Compose v2 系の厳格バリデーションによるもので、**本変更とは無関係の既存事象**である。
変更前の `docker-compose.yml`（git stash で一時退避した状態）でも同一エラーが再現することを
確認済み（`git stash` → `docker compose config` → 同エラー → `git stash pop`）。

要件タスクの代替手段の指示に従い、YAML パーサ（PyYAML）で YAML 妥当性および `web` の解決後
設定を検証した:

```python
import yaml
doc = yaml.safe_load(open('docker-compose.yml'))
svc = doc['services']
# web/api/worker の logging
# web -> {'driver': 'json-file', 'options': {'max-size': '10m', 'max-file': '14'}}
# api -> {'driver': 'json-file', 'options': {'max-size': '10m', 'max-file': '14'}}
# worker -> {'driver': 'json-file', 'options': {'max-size': '10m', 'max-file': '14'}}
assert svc['web']['logging'] == svc['api']['logging'] == svc['worker']['logging']
```

結果: YAML は構文的に valid。`web` サービスの解決後 `logging` 設定に
`max-size: "10m"` / `max-file: "14"` が含まれることを確認した（AC 1.1 / 1.2 / 1.3）。

### 2. api / worker と web の設定値一致

PyYAML パース結果で `web` / `api` / `worker` の `logging` ブロックが完全に等価
（`driver: json-file`、`max-size: 10m`、`max-file: 14`）であることをアサートで確認した
（AC 2.1 / 2.2 / 2.3、NFR 2.1）。`db` サービスは `logging` を持たない（`None`）ことも確認し、
スコープ外の `db` に変更が及んでいないことを担保した。

### 3. web の既存設定保持

PyYAML パース結果で `web` サービスのキー集合が
`['build', 'ports', 'environment', 'logging', 'networks', 'depends_on', 'restart', 'deploy']`
であることを確認。`logging` 追加以外の既存キーがすべて保持されていることを担保した
（AC 4.1 / 4.2 / 4.3、NFR 2.2）。

### 4. 差分スコープの確認

`git diff docker-compose.yml` により、変更が `web` サービスへの `logging` ブロック 5 行追加
のみであることを確認（AC 4.2 / 4.3）。`api` / `worker` / `db` への差分は無い。

### 5. 既存テストへの影響

本変更は `docker-compose.yml`（YAML 設定）のみで、Go / TypeScript のユニットテスト対象コードを
含まない。CI（`.github/workflows/ci.yml` の `go test ./...` / `npm test`）が参照するソースには
触れていないため、既存テストを壊さない。compose 設定ファイル自体は YAML として valid である
ことを上記で確認済み。

## AC とテスト（検証）の対応表

| AC ID | 検証方法 |
|---|---|
| 1.1 | 検証 1（PyYAML パースで `web.logging` の存在確認） |
| 1.2 | 検証 1（`max-size: "10m"` の確認） |
| 1.3 | 検証 1（`max-file: "14"` の確認） |
| 2.1 | 検証 2（`web` と `api`/`worker` の `max-size` 一致） |
| 2.2 | 検証 2（`web` と `api`/`worker` の `max-file` 一致） |
| 2.3 | 検証 2（`web.logging == api.logging == worker.logging` のアサート） |
| 3.1 | `json-file` ドライバ + `max-size`/`max-file` の組み合わせにより Docker が最古ログから破棄（設定値で担保。検証 1/2） |
| 3.2 | 同上（保持総量は `max-size` × `max-file` = 10m × 14 で上限化。検証 1/2） |
| 4.1 | 検証 3（`web` の既存キー保持により従来どおり起動可能） |
| 4.2 | 検証 2/4（`api`/`worker`/`db` 定義に差分なし） |
| 4.3 | 検証 4（`git diff` で `logging` 追加以外の差分が無いことを確認） |
| NFR 1.1 | `max-size` × `max-file` = 10m × 14 ≈ 140MB の上限（検証 1） |
| NFR 2.1 | 検証 2（`api`/`worker` の既存ログ設定値を変更しない） |
| NFR 2.2 | 検証 3（`web` の `ports`/`environment`/`networks`/`depends_on`/`restart`/`deploy` を変更しない） |

> AC 3.1 / 3.2 は Docker の `json-file` ログドライバの実行時挙動（上限到達時に最古ファイルから
> 破棄）に依存するランタイム挙動であり、設定ファイル上は `max-size` / `max-file` の付与で担保
> される。コンテナ実行を伴う実挙動検証は本ステージ（設定ファイル変更のみ）のスコープ外。

## 確認事項（レビュワー判断ポイント）

- `docker compose config`（Compose v2 系）は `api` サービスの既存 healthcheck 定義
  `test: ["/feedman", "healthcheck"]` を `must start either by "CMD", "CMD-SHELL" or "NONE"` として
  reject する。これは本 Issue のスコープ外（`api` サービス定義は変更不可）であり、変更前の
  ファイルでも再現する**既存の事象**である。本ステージでは要件タスクの代替手段に従い PyYAML で
  YAML 妥当性と `web` の解決後設定を検証した。`api` の healthcheck 形式を `["CMD", ...]` に
  揃えるべきかは別 Issue として検討する余地がある（本 Issue では対応しない）。
- AC 3.1 / 3.2 のランタイム破棄挙動は Docker `json-file` ドライバの標準挙動に依拠しており、実際の
  コンテナ起動・ログ蓄積を伴う検証は本ステージでは未実施（設定値の付与で担保）。

## 派生タスク候補（次 Issue 検討）

- `api` サービスの healthcheck 定義（`test: ["/feedman", "healthcheck"]`）を Compose v2 系の
  厳格バリデーションに適合する `["CMD", "/feedman", "healthcheck"]` 形式へ修正する Issue。
  本 Issue のスコープ外（`api` 定義変更不可）のため別途起票が望ましい。

STATUS: complete
