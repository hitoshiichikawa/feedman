#!/usr/bin/env bash
#
# db-backup.sh — Feedman の PostgreSQL（docker-compose の db サービス）から
# pg_dump（カスタム形式）でバックアップを取得し、タイムスタンプ付きファイルを保存先へ出力する。
#
# 詳細な運用手順は docs/operations/backup-restore.md を参照すること。
#
# 接続情報・保存先はすべて環境変数で与える（機密値をスクリプトに直書きしない）:
#   POSTGRES_PASSWORD  必須。DB 接続パスワード（未設定ならエラー終了）。
#   POSTGRES_USER      任意。接続ユーザー（未設定時は feedman）。
#   POSTGRES_DB        任意。対象 DB 名（未設定時は feedman）。
#   BACKUP_DIR         任意。保存先ディレクトリ（未設定時は backups）。
#   COMPOSE_ENV_FILE   任意。docker compose --env-file に渡す env ファイル（未設定時は付与しない）。
#
# 終了コード: 成功 0 / 前提不備・取得失敗は非 0。
set -euo pipefail

# --- 入力（環境変数）の解決 ---
PG_USER="${POSTGRES_USER:-feedman}"
PG_DB="${POSTGRES_DB:-feedman}"
BACKUP_DIR="${BACKUP_DIR:-backups}"

if [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "error: POSTGRES_PASSWORD が未設定です。環境変数で接続パスワードを与えてください（直書き禁止）。" >&2
  exit 1
fi

# docker compose の存在確認
if ! docker compose version >/dev/null 2>&1; then
  echo "error: 'docker compose' が利用できません。Docker Compose v2 をインストールしてください。" >&2
  exit 1
fi

# --- docker compose 呼び出しの組み立て ---
# COMPOSE_ENV_FILE が指定されていれば --env-file を付与する。
compose_cmd=(docker compose)
if [ -n "${COMPOSE_ENV_FILE:-}" ]; then
  compose_cmd+=(--env-file "${COMPOSE_ENV_FILE}")
fi

# --- 保存先の準備 ---
mkdir -p "${BACKUP_DIR}"

ts="$(date +%Y%m%d-%H%M%S)"
out="${BACKUP_DIR}/feedman-${ts}.dump"

echo "backing up DB '${PG_DB}' as user '${PG_USER}' -> ${out}"

# --- バックアップ取得（pg_dump カスタム形式） ---
# PGPASSWORD はコンテナ内プロセスへ -e で引き渡す（コマンド行に平文を残さない）。
# -T は TTY 無効化（バイナリ出力をリダイレクトで壊さないため）。
if ! "${compose_cmd[@]}" exec -T \
  -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  db pg_dump -U "${PG_USER}" -d "${PG_DB}" -F c \
  > "${out}"; then
  echo "error: pg_dump に失敗しました。docs/operations/backup-restore.md のトラブルシュートを参照してください。" >&2
  # 途中まで書かれた壊れたファイルを残さない。
  rm -f "${out}"
  exit 1
fi

# --- 取得物の簡易検証（サイズ > 0） ---
if [ ! -s "${out}" ]; then
  echo "error: 取得したバックアップファイルが空です: ${out}" >&2
  rm -f "${out}"
  exit 1
fi

echo "backup completed: ${out} ($(wc -c < "${out}") bytes)"
