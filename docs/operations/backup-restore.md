# DB バックアップ・リストア運用手順

本ドキュメントは、Feedman の PostgreSQL（docker-compose の `db` サービス、`postgres:16-alpine`）に
対するバックアップ取得手順・リストア手順・運用方針・トラブルシュートをまとめた運用者向けガイドです。
**ドキュメントどおりに辿れば、暗黙知なしでバックアップを取得・復元できること**を目的とします。

> **このドキュメントの位置づけ**
>
> - **必須スコープ**: リストアを含む**手動運用手順**の整備（[1. バックアップ取得手順](#1-バックアップ取得手順) /
>   [2. リストア手順](#2-リストア手順) / [3. バックアップ運用方針](#3-バックアップ運用方針) /
>   [4. トラブルシュート](#4-トラブルシュート) / [5. 境界ケース](#5-境界ケース)）。
> - **任意スコープ**: 定期実行の自動化（[6. 自動化（任意）](#6-自動化任意)）。スクリプトによる定期実行か、
>   マネージド DB スナップショットへの委譲か、またはその併用かの選択は**運用者の判断に委ねます**。

## 前提と接続情報

Feedman の `db` サービスは `internal` ネットワーク専用で、ホストにポートを公開していません
（`docker-compose.yml` 参照）。そのため本手順は **`docker compose exec db` 経由でコンテナ内から
`pg_dump` / `pg_restore` / `psql` を実行する**形を基本線とします。

接続情報は以下の環境変数で与えられます（`docker-compose.yml` の `db` サービス定義および
`.env.sample` 参照）。いずれもデフォルトは `feedman` です。**パスワード等の機密値は本ドキュメントに
平文で書かず、必ず環境変数経由で参照してください**。

| 環境変数 | 用途 | デフォルト |
|---|---|---|
| `POSTGRES_USER` | 接続ユーザー | `feedman` |
| `POSTGRES_PASSWORD` | 接続パスワード | `feedman`（本番では必ず変更） |
| `POSTGRES_DB` | 対象 DB 名 | `feedman` |

接続先（ホスト）はコンテナ内では `db`（サービス名で名前解決）、ポートは `5432` です。

本ドキュメントのコマンド例では、以下のように `.env` で設定済みの値を shell 変数へ展開して使うことを
想定します（パスワードは `.env.production` 等から読み込み、画面・コマンド履歴に平文を残さない運用を推奨）。

```bash
# .env.production を読み込んでいる前提の env-file を docker compose に渡す。
# 以降の例では COMPOSE="docker compose --env-file .env.production" と置く。
COMPOSE="docker compose --env-file .env.production"

# 対象 DB / 接続ユーザーは環境変数から取得する（直書きしない）。
# 未設定ならデフォルト feedman を使う。
PG_USER="${POSTGRES_USER:-feedman}"
PG_DB="${POSTGRES_DB:-feedman}"
```

---

## 1. バックアップ取得手順

> 対応要件: AC 1.1 / 1.2 / 1.3 / 1.4 / NFR 1.1

### 1-1. 保存先の準備

ホスト側にバックアップの保存先ディレクトリを用意します。**保存先はホスト側の任意のパス**で構いません
（例ではプロジェクト直下の `backups/` を使いますが、運用方針に応じて外部マウント等へ変更可能です）。

```bash
mkdir -p backups
```

### 1-2. バックアップの取得（`pg_dump` / カスタム形式）

`db` コンテナ内で `pg_dump` を実行し、その出力をホスト側のファイルへリダイレクトして保存します。
`pg_dump` のカスタム形式（`-F c`）は世代管理・部分リストアに向くため本手順の基本とします。

接続パスワードは `PGPASSWORD` 環境変数で `pg_dump` に渡します（**コマンド行に平文で書かない**）。
`docker compose exec` の `-e` でコンテナ内プロセスへ `PGPASSWORD` を引き渡します。

```bash
# タイムスタンプ付きファイル名で保存先に出力する（複数世代の区別に利用）。
TS="$(date +%Y%m%d-%H%M%S)"
OUT="backups/feedman-${TS}.dump"

# PGPASSWORD はホストの環境変数（.env から export 済み等）を参照する。直書きしない。
$COMPOSE exec -T \
  -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db pg_dump -U "${PG_USER}" -d "${PG_DB}" -F c \
  > "${OUT}"

echo "backup written to ${OUT}"
```

- `-T` は `docker compose exec` に TTY を割り当てない指定で、出力をそのままファイルへリダイレクトする
  ために必要です（TTY 付きだとバイナリ出力が壊れます）。
- `-F c` は **カスタムアーカイブ形式**です（`pg_restore` で復元します）。
- 出力先 `${OUT}` が**復元に利用可能なバックアップファイル**です。上記コマンドを手順どおり実行すると、
  指定した保存先（`backups/`）にこのファイルが生成されます。

### 1-3. プレーン SQL 形式で取得する場合（任意）

`psql` で復元したい場合や差分を `git diff` 等で確認したい場合は、プレーン SQL 形式（デフォルト）でも
取得できます。

```bash
TS="$(date +%Y%m%d-%H%M%S)"
OUT="backups/feedman-${TS}.sql"

$COMPOSE exec -T \
  -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db pg_dump -U "${PG_USER}" -d "${PG_DB}" \
  > "${OUT}"
```

プレーン SQL 形式は [2-2. プレーン SQL からのリストア](#2-2-プレーン-sql-からのリストアpsql) で復元します。

### 1-4. 取得物の確認

取得直後にファイルが空でない（サイズ > 0）こと、カスタム形式なら `pg_restore --list` で中身を
列挙できることを確認します。

```bash
ls -lh "${OUT}"

# カスタム形式の場合、アーカイブの目次を確認できる（中身が壊れていないことの簡易確認）。
pg_restore --list "${OUT}" | head
# pg_restore がホストに無い場合はコンテナ内で確認する:
#   $COMPOSE exec -T db pg_restore --list < "${OUT}" | head
```

---

## 2. リストア手順

> 対応要件: AC 2.1 / 2.2 / 2.3 / 2.4 / NFR 2.1

### 2-1. 前提条件（リストア実行前に満たすべき条件）

> 対応要件: AC 2.3

リストアを開始する前に、以下を満たしていることを確認します。

1. **DB へ接続できること**: `db` コンテナが起動しており、接続できる。
   ```bash
   $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
     db psql -U "${PG_USER}" -d postgres -c '\conninfo'
   ```
2. **対象 DB が存在すること**: リストア先 DB（`${PG_DB}`）が存在する。存在しない場合は作成する
   （初回リストア時など）。
   ```bash
   # 存在確認（1 行返れば存在する）
   $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
     db psql -U "${PG_USER}" -d postgres -tAc \
     "SELECT 1 FROM pg_database WHERE datname='${PG_DB}'"

   # 存在しない場合は作成する
   $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
     db createdb -U "${PG_USER}" "${PG_DB}"
   ```
3. **必要な権限を持つユーザーで実行すること**: 復元はオブジェクト作成（テーブル・インデックス・
   データ投入）を伴うため、対象 DB の所有者または十分な権限を持つユーザー（既定の `feedman` ユーザーは
   `POSTGRES_USER` であり対象 DB の所有者）で実行します。

> **注意**: リストアは既存データを上書き・追加します。本番 DB に対して実行する場合は、事前に現在の
> 状態のバックアップを取得しておくことを推奨します（[1. バックアップ取得手順](#1-バックアップ取得手順)）。

### 2-2. カスタム形式からのリストア（`pg_restore`）

[1-2](#1-2-バックアップの取得pg_dump--カスタム形式) で取得したカスタム形式（`.dump`）を復元します。

```bash
# 復元に用いるバックアップファイルを指定する。
DUMP="backups/feedman-20260526-120000.dump"  # 実際の世代名に置き換える

$COMPOSE exec -T \
  -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db pg_restore -U "${PG_USER}" -d "${PG_DB}" --clean --if-exists \
  < "${DUMP}"
```

- `--clean --if-exists` は復元前に既存オブジェクトを削除してから再作成します（再現性のため、
  空でない DB にも安全に適用できます。`--if-exists` により対象が無くてもエラーになりません）。
- 空の DB に対して最初から最後までこのコマンドを実行すると、取得済みバックアップの内容が復元されます。

### 2-3. プレーン SQL からのリストア（`psql`）

[1-3](#1-3-プレーン-sql-形式で取得する場合任意) で取得したプレーン SQL（`.sql`）を復元します。

```bash
SQL="backups/feedman-20260526-120000.sql"  # 実際の世代名に置き換える

$COMPOSE exec -T \
  -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db psql -U "${PG_USER}" -d "${PG_DB}" \
  < "${SQL}"
```

### 2-4. リストア完了後の確認

> 対応要件: AC 2.4

復元が完了したら、以下の観点でデータが復元されたことを確認します。

```bash
# テーブル一覧が復元されているか
$COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db psql -U "${PG_USER}" -d "${PG_DB}" -c '\dt'

# 主要テーブルの行数を確認する（例: users / subscriptions / items 等、存在するテーブルで確認）
$COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db psql -U "${PG_USER}" -d "${PG_DB}" -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'"
```

確認観点:

- バックアップ取得時に存在したテーブルがすべて復元されていること（`\dt` の一覧）。
- 主要テーブルの行数がバックアップ取得時点と整合すること。
- アプリケーション（`api` / `web`）から実際にログイン・購読一覧の表示ができること（疎通確認）。

---

## 3. バックアップ運用方針

> 対応要件: AC 3.1 / 3.2 / 3.3

本方針は推奨値です。データの重要度・更新頻度・復旧目標（RPO/RTO）に応じて運用者が調整してください。

### 3-1. 取得頻度

- **推奨**: 1 日 1 回の定期取得（日次）を基本とする。
- 更新が活発な期間や重要なデータ投入の前後では、随時手動取得を追加する。

### 3-2. 保持期間

- **推奨**: 直近 7 世代（日次なら約 1 週間分）を保持する。
- 月次の長期保管が必要な場合は、月初の世代を別途長期保存領域へ退避する。
- 保持期間を超えた古い世代は削除して保存先の肥大化を防ぐ（削除は運用者の責任で実施）。

### 3-3. 保存先

- **推奨**: バックアップファイルはアプリケーションが稼働するホストの**ローカルディスクとは別の領域**
  （外部ストレージ・別ボリューム・オブジェクトストレージ等）へ退避する。同一ディスク上のみの保持は、
  ディスク障害時にバックアップごと失う点に注意する。
- 保存先には**機密データ（ユーザー情報等）が含まれる**ため、アクセス権限を最小化し、暗号化保存を検討する。
- 本リポジトリの作業ディレクトリ直下に保存する場合は、`backups/` を **`.gitignore` に追加**し、
  バックアップファイル（機密データを含む）を誤ってコミットしないこと。

---

## 4. トラブルシュート

> 対応要件: AC 4.1 / 4.2 / 4.3

### 4-1. DB へ接続できない

> 対応要件: AC 4.1

典型的な症状: `could not connect to server` / `Connection refused` / `password authentication failed`。

典型原因と確認手順:

- **DB コンテナが起動していない**: `$COMPOSE ps` で `db` サービスの状態を確認する。停止していれば
  `$COMPOSE up -d db` で起動する。`healthcheck`（`pg_isready`）が `healthy` になるまで待つ。
- **接続先（ホスト・ポート）の誤り**: `db` は `internal` ネットワーク専用でホストに公開されていない。
  ホストから直接 `psql` で接続しようとしていないか確認する（本手順は `docker compose exec db` 経由が前提）。
- **認証情報（ユーザー・パスワード）の誤り**: `POSTGRES_USER` / `POSTGRES_PASSWORD` が `db` コンテナ
  起動時の値と一致しているか確認する。`.env.production` の値と `docker-compose.yml` のデフォルトの
  どちらが適用されているかに注意する。
  ```bash
  # 接続確認（成功すれば接続情報は正しい）
  $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    db psql -U "${PG_USER}" -d postgres -c 'SELECT 1'
  ```

### 4-2. 権限不足が発生する

> 対応要件: AC 4.2

典型的な症状: `permission denied for ...` / `must be owner of ...`。

典型原因と確認手順:

- **オブジェクト所有者でないユーザーで復元している**: 復元はテーブル作成・削除を伴うため、対象 DB の
  所有者または十分な権限を持つユーザーで実行する必要がある。既定では `POSTGRES_USER`（`feedman`）が
  対象 DB の所有者となる。
  ```bash
  # 接続ユーザーのロール属性を確認する（Superuser / CreateDB / CreateRole 等）
  $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    db psql -U "${PG_USER}" -d "${PG_DB}" -c '\du'

  # 対象 DB の所有者を確認する
  $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    db psql -U "${PG_USER}" -d postgres -c '\l'
  ```
- 権限が不足する場合は、対象 DB の所有者ユーザーで実行し直すか、必要な権限を付与する。

### 4-3. バックアップ・リストアが途中で失敗する

> 対応要件: AC 4.3

失敗の検知観点:

- **コマンドの終了ステータスを確認する**: `pg_dump` / `pg_restore` / `psql` の終了コードが `0` でない
  場合は失敗。`$COMPOSE exec` 経由でもコンテナ内コマンドの終了コードがそのまま伝播する。
  ```bash
  $COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    db pg_dump -U "${PG_USER}" -d "${PG_DB}" -F c > "${OUT}"
  echo "exit code: $?"   # 0 以外なら失敗
  ```
- **出力ファイルのサイズ**: バックアップ出力が 0 バイトの場合は失敗を疑う（`ls -lh "${OUT}"`）。
- **`pg_restore --list` で中身を検証**: カスタム形式のバックアップが壊れていないか目次で確認する。

再実行・切り分けの指針:

- バックアップ失敗時は、原因（接続不可・権限・ディスク容量不足等）を切り分けたうえで、**新しい
  ファイル名で取り直す**（途中まで書かれた壊れたファイルは復元に使わない）。
- リストアが途中で失敗した場合、`pg_restore` は部分的にオブジェクトを作成している可能性がある。
  `--clean --if-exists` を付けて**最初からやり直す**ことで、既存オブジェクトを削除してから再作成できる
  （冪等な再実行）。
- ディスク容量不足が疑われる場合は保存先の空き容量を確認する（`df -h`）。

---

## 5. 境界ケース

> 対応要件: AC 5.1 / 5.2

### 5-1. バックアップが 1 件も存在しない初期状態（初回取得）

> 対応要件: AC 5.1

保存先にバックアップが 1 件も無い状態（初回）でも、手順は通常と同じです。保存先ディレクトリを作成し、
[1. バックアップ取得手順](#1-バックアップ取得手順) を実行すれば最初の 1 世代が生成されます。

```bash
# 保存先が無ければ作成（初回）。-p により既存でもエラーにならない。
mkdir -p backups

# 初回バックアップ取得
TS="$(date +%Y%m%d-%H%M%S)"
$COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db pg_dump -U "${PG_USER}" -d "${PG_DB}" -F c \
  > "backups/feedman-${TS}.dump"
```

### 5-2. 複数世代のバックアップが存在する場合（特定世代の指定）

> 対応要件: AC 5.2

保存先に複数世代がある場合、復元に用いる**特定の世代をファイル名で明示的に指定**してリストアします。

```bash
# 保存先の世代一覧を新しい順に確認する
ls -1t backups/feedman-*.dump

# 復元したい世代を明示的に指定する（例: 特定タイムスタンプの世代）
DUMP="backups/feedman-20260526-120000.dump"

$COMPOSE exec -T -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db pg_restore -U "${PG_USER}" -d "${PG_DB}" --clean --if-exists \
  < "${DUMP}"
```

最新世代を自動選択したい場合は次のように指定できます（特定世代を意図的に選びたい場合は上記のように
明示指定すること）。

```bash
DUMP="$(ls -1t backups/feedman-*.dump | head -n1)"
```

---

## 6. 自動化（任意）

> 対応要件: AC 6.1 / 6.2 / 6.3

[1](#1-バックアップ取得手順)〜[5](#5-境界ケース) の**手動運用手順が本整備の必須範囲**です。定期実行の
自動化は**必須ではなく任意**であり、以下のいずれを採用するか（またはその併用）は**運用者の判断に
委ねます**。

- **(a) スクリプトによる定期実行**: 本リポジトリ同梱の補助スクリプト
  [`scripts/db-backup.sh`](../../scripts/db-backup.sh) を cron / systemd timer 等から定期実行する。
- **(b) マネージド DB スナップショットへの委譲**: 外部のマネージド PostgreSQL（クラウドの DB サービス）の
  自動スナップショット機能に保護を委譲する。この場合、本ドキュメントの手動 `pg_dump` / `pg_restore` は
  論理バックアップ（ポータブルなダンプ）として補助的に併用できる。
- **(c) 併用**: (a) と (b) を組み合わせる。

### 6-1. スクリプトによる定期実行の例

同梱スクリプト [`scripts/db-backup.sh`](../../scripts/db-backup.sh) は、本ドキュメント [1](#1-バックアップ取得手順)
と同じ `docker compose exec db pg_dump`（カスタム形式）を実行し、タイムスタンプ付きファイルを保存先へ
出力します。実行方法と前提条件は次のとおりです。

**前提条件**:

- 実行ホストで `docker compose` が利用可能で、`db` サービスが起動していること。
- 接続情報を環境変数で与えること（**スクリプトに機密値を直書きしない**）。
  - `POSTGRES_PASSWORD`（必須。未設定ならスクリプトはエラー終了する）
  - `POSTGRES_USER`（任意。未設定時は `feedman`）
  - `POSTGRES_DB`（任意。未設定時は `feedman`）
  - `BACKUP_DIR`（任意。未設定時は `backups`）
  - `COMPOSE_ENV_FILE`（任意。`docker compose --env-file` に渡す env ファイル。未設定時は付与しない）

**実行方法**:

```bash
# 必要な接続情報を環境変数で与えて実行する（パスワードは .env から export 等で渡す）。
POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
POSTGRES_USER=feedman \
POSTGRES_DB=feedman \
BACKUP_DIR=backups \
  ./scripts/db-backup.sh
```

cron での日次実行例（毎日 04:00、パスワードは環境ファイルから読み込む）:

```cron
# 環境変数は cron から直接渡さず、専用の env ファイルを読み込むラッパー経由を推奨。
0 4 * * * cd /path/to/feedman && set -a && . ./.env.production && set +a && ./scripts/db-backup.sh >> /var/log/feedman-backup.log 2>&1
```

保持期間（[3-2](#3-2-保持期間)）の適用（古い世代の削除）は運用ポリシーに依存するため、スクリプトには
含めていません。必要に応じて `find backups -name 'feedman-*.dump' -mtime +7 -delete` 等を別途運用してください。

---

## 関連

- リポジトリ構成・デプロイ手順: [`README.md`](../../README.md)
- DB マイグレーション: `docker compose exec api /feedman migrate`（README「初期デプロイ手順」参照）
- 補助スクリプト: [`scripts/db-backup.sh`](../../scripts/db-backup.sh)
