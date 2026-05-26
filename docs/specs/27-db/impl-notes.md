# 実装ノート（Issue #27: DB バックアップ・リストアの運用手順を整備する）

## 概要

本 Issue は **design-less impl**（`design.md` / `tasks.md` なし）であり、`requirements.md` を直接の
入力とした。DB（docker-compose の `db` サービス、`postgres:16-alpine`）のバックアップ取得・リストアの
運用手順を整備した。

## 実装した内容と配置ファイル

| ファイル | 種別 | 内容 |
|---|---|---|
| `docs/operations/backup-restore.md` | 新規（必須スコープの本体） | バックアップ取得手順・リストア手順・運用方針・トラブルシュート・境界ケース・自動化（任意）を網羅した運用ドキュメント |
| `scripts/db-backup.sh` | 新規（任意スコープ） | `docker compose exec db pg_dump`（カスタム形式）でタイムスタンプ付きバックアップを保存先へ出力する補助スクリプト。接続情報は全て環境変数経由 |
| `README.md` | 編集（導線追加） | 「初期デプロイ手順」に「6. DB バックアップ・リストア」節を追加し、運用ドキュメントへのリンクを 1 つ張った |

ドキュメント配置先（`docs/operations/`）・スクリプト配置先（`scripts/`）・ファイル名は requirements の
Out of Scope で design / impl 領分として委ねられているため Developer 判断で決定した。既存 `docs/specs/`
構成と並立する `docs/operations/` を新設し、運用文書の置き場とした。

## バックアップ手段の選定

- **論理バックアップ `pg_dump` を基本線**とした。`db` サービスは `internal` ネットワーク専用で
  ホストにポート公開していないため、`docker compose exec db pg_dump ...` 形式（コンテナ内実行）が
  実態に即する。
- カスタム形式（`-F c`）を基本とし、`pg_restore --clean --if-exists` で冪等に復元できる手順を主とした。
  プレーン SQL 形式（`psql` 復元）も任意手段として併記した。

## 自動化スクリプトの追加判断

- **追加した**（`scripts/db-backup.sh`）。要件 6 では自動化は任意だが、ドキュメント [6-1] で「スクリプトに
  よる定期実行」を選択肢として提示するにあたり、実体のあるスクリプトを同梱したほうが運用者が cron 等へ
  載せやすく、AC 6.2（自動化の実行方法と前提条件の提示）を具体的に満たせると判断した。
- スクリプトの前提条件（必須/任意の環境変数）・実行方法はドキュメント [6-1] とスクリプト冒頭コメントに
  列挙した。
  - 必須: `POSTGRES_PASSWORD`（未設定ならエラー終了）
  - 任意: `POSTGRES_USER`（既定 `feedman`）/ `POSTGRES_DB`（既定 `feedman`）/ `BACKUP_DIR`（既定 `backups`）/
    `COMPOSE_ENV_FILE`（既定: 付与しない）
- 機密値（パスワード）はスクリプトに直書きせず、`PGPASSWORD` をコンテナ内プロセスへ `-e` で引き渡す
  形にした（NFR 1.1）。
- 保持期間の適用（古い世代削除）は運用ポリシー依存のためスクリプトに含めず、ドキュメントで `find ... -delete`
  の例を提示するに留めた。

## 受入基準（AC）とテスト/検証のマッピング

本 Issue はドキュメント整備が主体のため、AC の担保は「ドキュメント該当箇所」と「documented コマンドの
実コンテナでのラウンドトリップ検証」で行った。

| AC | 担保箇所 | 検証 |
|---|---|---|
| 1.1 取得コマンド例の提示 | backup-restore.md §1-2 / 1-3 | 検証(A) で `pg_dump -F c` 実行成功 |
| 1.2 対象=db サービス / 保存先指定の明記 | §前提と接続情報 / §1-1 | — |
| 1.3 保存先にバックアップファイル生成 | §1-2 / §1-3 | 検証(A) で 2591 bytes の dump 生成確認 |
| 1.4 接続情報を環境変数経由 | §前提と接続情報 / §1-2（`PGPASSWORD` / `POSTGRES_*`） | 検証(A)（全コマンドが env 経由） |
| 2.1 リストアコマンド例の提示 | §2-2 / §2-3 | 検証(A) で `pg_restore --clean --if-exists` 成功 |
| 2.2 空 DB へ最初から最後まで実行で復元 | §2-1（空 DB 作成）+ §2-2 | 検証(B) で空 DB `restoredb` へ復元し 3 行確認 |
| 2.3 前提条件（接続可否・対象 DB 存在・権限）の明記 | §2-1 | 検証(C) で接続不可時の挙動確認 |
| 2.4 復元後の確認手段の提示 | §2-4 | 検証(A) で `\dt` / 行数で復元確認 |
| 3.1 取得頻度の方針 | §3-1（日次推奨） | — |
| 3.2 保持期間の方針 | §3-2（7 世代推奨） | — |
| 3.3 保存先の方針 | §3-3（別領域退避・暗号化・.gitignore） | — |
| 4.1 接続不可の原因と確認手順 | §4-1 | 検証(C) で誤パスワード→exit code 2（非 0）を確認 |
| 4.2 権限不足の原因と確認手順 | §4-2（`\du` / `\l`） | — |
| 4.3 失敗検知と再実行・切り分け | §4-3（exit code / サイズ / `--list`） | 検証(A) で `pg_restore --list` による中身検証を実演 |
| 5.1 0 件状態からの初回取得 | §5-1 | 検証(A)（保存先空状態から 1 世代生成） |
| 5.2 複数世代から特定世代を指定 | §5-2（ファイル名明示指定 / `ls -1t`） | 検証(B) で `ls -1t ... head -n1` による世代選択 |
| 6.1 手動運用手順を必須範囲として提示 | §冒頭の位置づけ / §6 | — |
| 6.2 自動化の実行方法と前提条件 | §6-1 / `scripts/db-backup.sh` | shellcheck 検証 |
| 6.3 自動化手段の選択が運用者判断である旨 | §6（(a)/(b)/(c) を明記） | — |
| NFR 1.1 機密情報の非直書き（環境変数経由） | 全コマンド例で `$PGPASSWORD` / `${POSTGRES_*}` 参照 | スクリプトに直書き無し（shellcheck / 目視） |
| NFR 1.2 本番認証情報を成果物に含めない | 平文パスワード・本番接続情報の記載なし | grep / 目視 |
| NFR 2.1 暗黙知なしで復元完了 | §2 の前提条件→復元→確認の手順化 | 検証(B) でドキュメント手順どおりに空 DB 復元成功 |
| NFR 3.1 アプリ/スキーマ/マイグレ非変更 | 変更は docs/scripts/README のみ | `git status`（api/worker/web/migrations 無変更） |

## 検証方法と結果（ラウンドトリップ検証）

`docker` が利用可能だったため、実コンテナでドキュメント記載コマンドのラウンドトリップを検証した。

> **注**: `docker compose` 全体（`docker compose up`）は、ローカルの Docker Compose v5.1.3 が
> `api` サービスの `healthcheck.test: ["/feedman", "healthcheck"]` を「CMD/CMD-SHELL/NONE で始まる
> 必要がある」としてパース拒否するため起動できなかった。これは既存 `docker-compose.yml` と新しい
> compose スキーマの差異であり、本 Issue のスコープ外（NFR 3.1 によりアプリ/compose は変更しない）。
> そのため `db` と同一 image（`postgres:16-alpine`）・同一 env 構成の単体コンテナを起動し、ドキュメントの
> `docker compose exec db ...` と等価なコンテナ内コマンド（`docker exec <c> ...`）で構文・整合性を実証した。
> ドキュメント本文のコマンドは `docker compose exec` 形式で記述している（実態に即した形）。

- **検証(A) DROP→復元ラウンドトリップ**: `users` テーブルに 3 行を投入 → `pg_dump -F c` で取得
  （2591 bytes）→ `pg_restore --list` で目次確認 → `DROP TABLE users`（消失をシミュレート）→
  `pg_restore --clean --if-exists` で復元 → `\dt` と行数で **3 行が完全復元**されたことを確認。**成功**。
- **検証(B) 空 DB への復元 + 世代選択**: 新規空 DB `restoredb` を `createdb` で作成 → `ls -1t ... head -n1` で
  最新世代を選択 → custom dump を `pg_restore` で復元 → **3 行復元**を確認。**成功**。
  プレーン SQL 形式も別の空 DB `sqlrestoredb` へ `psql` 復元し **3 行復元**を確認。**成功**。
- **検証(C) 接続失敗の検知**: 誤ったパスワードで `psql` 実行 → `docker exec` の **exit code 2（非 0）**を
  確認。トラブルシュート §4-1/4-3 の「終了ステータスで失敗を検知」が成立することを実証。**成功**。
- 検証に使った一時コンテナ・一時ファイルはすべて削除済み（リポジトリには成果物のみ残る）。

### 静的検証

- `shellcheck scripts/db-backup.sh`: 警告ゼロ（pass）。
- `bash -n scripts/db-backup.sh`: 構文 OK。

### 既存テストへの影響

- 変更は `docs/` / `scripts/` / `README.md` のみで、`api` / `worker` / `web` のコード・DB スキーマ・
  マイグレーションに変更なし（NFR 3.1）。既存 `go test ./...` / `npm test` への影響は無いと判断し、
  ドキュメント・スクリプトのみの変更のため再実行は省略した（影響範囲外）。

## 確認事項

- **既存 `docker-compose.yml` の `healthcheck.test` 形式**: ローカルの Docker Compose v5.1.3 では
  `api` の `test: ["/feedman", "healthcheck"]`（先頭が `CMD`/`CMD-SHELL`/`NONE` でない exec 形式）が
  パース拒否され `docker compose up` が起動できなかった。本 Issue のスコープ外（アプリ/compose 非変更）
  のため本 PR では触れていないが、運用環境の Compose バージョン次第で同事象が起きうる。別 Issue として
  `healthcheck.test` を `["CMD", "/feedman", "healthcheck"]` 形式へ修正する切り出しを検討されたい
  （本 Issue では検証手段の制約として記録するに留める）。
- 上記以外に requirements.md との矛盾・人間判断が必要な不明点は **なし**。

STATUS: complete
