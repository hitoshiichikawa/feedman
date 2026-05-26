# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-27-impl-db
- HEAD commit: 2450b562299e7b5528d5d7dc616971aeb947e013
- Compared to: develop..HEAD
- 種別: design-less impl（`design.md` / `tasks.md` なし）。`_Boundary:_` アノテーション不在のため
  boundary 逸脱判定は requirements.md の Out of Scope / NFR 3.1 を境界として代替。
- Feature Flag Protocol: CLAUDE.md `## Feature Flag Protocol` の採否は `opt-out`。flag 観点の確認は行わない。
- 変更ファイル: `README.md` / `docs/operations/backup-restore.md`（新規）/ `scripts/db-backup.sh`（新規）/
  `docs/specs/27-db/{impl-notes,requirements}.md`。`api` / `worker` / `web` / `internal` / `migrations` /
  `*.go` / `*.sql` への変更は無し（NFR 3.1 充足を `git diff --name-only` で確認）。

## Verified Requirements

- 1.1 — backup-restore.md §1-2（`docker compose exec -T db pg_dump -U ... -d ... -F c > "${OUT}"`）で実行可能なコマンド例を提示
- 1.2 — 「前提と接続情報」節（対象 = `db` サービス / PostgreSQL）+ §1-1（保存先ディレクトリの指定方法 `mkdir -p backups`）
- 1.3 — §1-2（`> "${OUT}"` で保存先に dump ファイルが生成される手順を明記）
- 1.4 — 「前提と接続情報」環境変数表（`POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`）+ 各コマンドの `PGPASSWORD` / `${PG_USER}` / `${PG_DB}` env 経由参照
- 2.1 — §2-2（`pg_restore -U ... -d ... --clean --if-exists < "${DUMP}"`）で復元コマンド例を提示
- 2.2 — §2-1（空 DB を `createdb` で作成）+ §2-2（空 DB へ最初から最後まで実行で復元される旨を明記）
- 2.3 — §2-1 前提条件（DB 接続可否 `\conninfo` / 対象 DB 存在確認・作成 / 必要権限）を明記
- 2.4 — §2-4（`\dt` / 行数 / アプリ疎通の復元後確認観点）を提示
- 3.1 — §3-1（日次取得の取得頻度方針）
- 3.2 — §3-2（直近 7 世代の保持期間方針）
- 3.3 — §3-3（別領域退避・暗号化・`.gitignore` の保存先方針）
- 4.1 — §4-1（接続不可の典型原因 = コンテナ未起動 / 接続先誤り / 認証情報誤り、確認手順）
- 4.2 — §4-2（権限不足の典型原因 = 非所有者実行、`\du` / `\l` での確認手順）
- 4.3 — §4-3（終了ステータス / ファイルサイズ / `pg_restore --list` での失敗検知、再実行・切り分け指針）
- 5.1 — §5-1（保存先 0 件状態からの初回取得手順）
- 5.2 — §5-2（複数世代からファイル名明示指定 / `ls -1t` での特定世代選択）
- 6.1 — 「このドキュメントの位置づけ」+ §6 冒頭（手動運用手順を必須範囲として明記）
- 6.2 — §6-1（自動化の実行方法 + 前提条件の env / 権限列挙）+ `scripts/db-backup.sh`（実体スクリプト）
- 6.3 — §6（自動化手段 (a) スクリプト / (b) マネージドスナップショット / (c) 併用の選択が運用者判断である旨を明記）
- NFR 1.1 — 全コマンド例・スクリプトが `${PGPASSWORD}` / `${POSTGRES_*}` env 経由参照、平文直書き無し（diff / 目視で確認）
- NFR 1.2 — 本番認証情報・本番接続情報の記載無し（diff 全体で確認）
- NFR 2.1 — §2 が前提条件 → 復元 → 確認の手順として完結しており暗黙知を要求しない
- NFR 3.1 — `git diff --name-only develop..HEAD` で `api` / `worker` / `web` / DB スキーマ / migrations 無変更を確認

## Findings

なし

## Summary

全 numeric ID（要件 1〜6 の各 AC および NFR 1〜3）が運用ドキュメント該当節 / スクリプト / README 導線で裏打ちされており、AC 未カバーは無い。design-less impl で `_Boundary:_` 不在のため境界判定は NFR 3.1 / Out of Scope を境界として代替し、`api` / `worker` / `web` / スキーマ / migrations への変更が無いことを diff で確認、逸脱なし。ドキュメント + 補助スクリプト主体の Issue であり、Developer は実コンテナでのラウンドトリップ検証（A/B/C）と `shellcheck` / `bash -n` の静的検証を impl-notes に AC 対応付きで記録しており、AC 対応挙動の未検証は無いため missing test の指摘なし。

RESULT: approve
