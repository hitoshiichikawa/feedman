# 実装ノート: docker-compose 全サービスへのリソース上限設定（Issue #16）

## 変更概要

`docker-compose.yml` の全 4 サービスに `deploy.resources.limits`（`memory` / `cpus`）を追加した。
追加したのは `deploy.resources.limits` ブロックのみで、既存のサービス定義・ネットワーク
（internal / external）・依存関係（depends_on）・公開ポート・環境変数・ボリューム・
ヘルスチェック・再起動ポリシー（`restart: unless-stopped`）には一切変更を加えていない。

| サービス | memory limit | cpus limit |
|---|---|---|
| db     | 512M | "0.5" |
| api    | 256M | "0.5" |
| worker | 256M | "0.5" |
| web    | 256M | "0.5" |

各 `deploy` ブロックの直前に、上限値の根拠（Option A: 保守的初期値・将来調整前提）を示す
コメントを付した（NFR 1.2）。`cpus` は Compose Spec の慣習に従いクォート文字列 `"0.5"` で記述。

## 受入基準のカバレッジ確認

| Requirement | AC | 担保方法 |
|---|---|---|
| 1.1 | 全サービスにメモリ上限 | 4 サービスすべてに `limits.memory` を設定。後述の YAML パース確認で全サービスに存在を検証 |
| 1.2 | 全サービスに CPU 上限 | 4 サービスすべてに `limits.cpus` を設定。後述の YAML パース確認で検証 |
| 1.3 | 上限未定義を未充足として検出可能 | `deploy.resources.limits` は各サービス配下に明示記述されており、欠落時は YAML パース / grep / `docker compose config` 出力差分で検出可能 |
| 2.1 / 2.2 / 2.3 | 暴走サービスの隔離・他サービス稼働継続・再起動ポリシー踏襲 | コンテナランタイム（Compose V2）が `deploy.resources.limits` を honor し、上限超過コンテナのみを cgroup 制限・OOM kill 対象とする。既存 `restart: unless-stopped` を温存したため、上限到達停止時は当該サービスのみが従来ポリシーで再起動される（YAML 構成上の挙動。実ランタイム挙動の動的検証は本変更スコープ外） |
| 3.1 / 3.2 / 3.3 | 通常負荷での正常稼働 | Option A の保守的初期値（db 512M、その他 256M / 各 0.5 cpu）を採用。通常負荷で上限到達による異常終了を起こさない想定値 |
| 4.1 / 4.2 | 既存ネットワーク・依存・ポート・環境変数・ボリューム・ヘルスチェック・再起動ポリシー不変 | `git diff --stat` で変更が docker-compose.yml の 24 行追加のみであり、追加は `deploy` ブロックに閉じていることを確認 |
| 4.3 | 構成が構文として妥当 | 後述の検証参照（YAML パース成功） |
| NFR 1.1 / 1.2 | 将来調整可能・根拠の参照可能性 | 上限値はインラインで記述され環境ごとに編集可能。Option A の根拠を各 deploy ブロック近傍のコメント及び本ノートに記録 |
| NFR 2.1 | 障害隔離の可観測性 | 既存の再起動ポリシー・logging 設定を温存。再起動回数・終了理由はコンテナランタイム標準手段（`docker ps` / `docker inspect` 等）で識別可能 |

## 検証方法と結果

### 1. `docker compose config`

実行したところ、`healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` という
エラーで exit 1 となった。これは `api` サービスの既存 healthcheck
（`test: ["/feedman", "healthcheck"]` という exec-form で `CMD`/`CMD-SHELL`/`NONE` で始まらない）
に対する、環境にインストールされた `docker compose` のバージョン固有のパース挙動である。

**本変更導入前（`git stash` で退避した base 状態）でも同一エラーで exit 1** となることを確認済みで、
本 Issue の `deploy.resources.limits` 追加とは無関係な既存事象である（本 Issue のスコープ外 /
Requirement 4.2 により healthcheck は変更していない）。`docker-compose`（V1）はこの環境に存在しない。

### 2. YAML パーサによる代替検証（Requirement 4.3）

`docker compose config` が上記の既存 healthcheck 制約で完走しないため、YAML パーサで構文妥当性と
全サービスへの limits 付与を代替検証した。

```
python3 -c 'import yaml; d=yaml.safe_load(open("docker-compose.yml")); ...'
```

結果:

```
YAML OK
web    {'memory': '256M', 'cpus': '0.5'}
api    {'memory': '256M', 'cpus': '0.5'}
worker {'memory': '256M', 'cpus': '0.5'}
db     {'memory': '512M', 'cpus': '0.5'}
```

→ YAML 構文は妥当（パース成功）であり、全 4 サービスに `deploy.resources.limits.memory` /
`deploy.resources.limits.cpus` が存在することを確認した。

### 3. 変更スコープの確認

```
$ git diff --stat
 docker-compose.yml | 24 ++++++++++++++++++++++++
 1 file changed, 24 insertions(+)
```

→ 変更は docker-compose.yml の 24 行追加のみ。既存行の削除・改変はなく、後方互換（Requirement 4）を満たす。
`go test ./...` / `npm test` は YAML のみの変更であり影響しないため実行不要（CLAUDE.md 禁止事項にも非抵触）。

## 設定値の根拠

人間が Issue コメントで確定した **Option A（保守的な初期値から開始し、将来の実測に基づいて調整する
段階的アプローチ）** に従う。実負荷プロファイリングは本対応では行わず、通常負荷で正常稼働を阻害しない
保守的初期値を付与する（NFR 1.2 / Requirement 3.3）。

- db: PostgreSQL は他サービスより多めの 512M を割り当て（共有バッファ・接続プール・キャッシュ余地）。
- api / worker / web: 256M / 0.5 cpu。いずれも将来の実測に応じて環境ごとに調整する前提。

## 確認事項

- `docker compose config` が `api` サービスの既存 healthcheck（exec-form `["/feedman", "healthcheck"]`）を
  `CMD`/`CMD-SHELL`/`NONE` で始まらないとしてエラー扱いする件は、本 Issue 導入前から存在する既存事象であり、
  本変更とは独立している。healthcheck の修正は Requirement 4.2（既存設定を変更しない）に抵触するため本 Issue では
  行わず、必要なら別 Issue として切り出すことを推奨する（派生タスク候補）。

## 派生タスク候補（次 Issue 提案）

- `api` サービスの healthcheck を `docker compose config` が完走できる記法（`CMD` プレフィックス付与等）へ
  整備する Issue。本 Issue では後方互換維持のためスコープ外とした。
- 実負荷プロファイリングに基づくリソース上限値のチューニング（Option A の将来対応フェーズ）。

STATUS: complete
