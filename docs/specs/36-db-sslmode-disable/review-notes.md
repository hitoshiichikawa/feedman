# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-36-impl-db-sslmode-disable
- HEAD commit: 69620a27173c1f3bad52c1534bf31d6aa2abc40d
- Compared to: develop..HEAD
- 変更ファイル: `.env.sample` / `docker-compose.yml` / `README.md`（+ spec ドキュメント）
- 補足: 本 spec は `design.md` / `tasks.md` を持たない design-less impl。`_Boundary:_`
  アノテーションは存在しないため、boundary 判定は requirements の Out of Scope（Go コード /
  スキーマ / マイグレーション無変更）との突き合わせで実施した。Feature Flag Protocol は
  CLAUDE.md で `opt-out` 宣言のため flag 観点は適用しない。

## Verified Requirements

- 1.1 — `.env.sample` の固定 `?sslmode=disable` を持つ active な `DATABASE_URL` 行を撤去し
  `# DATABASE_URL=`（コメントアウト）に変更。固定 disable の active 値は残っていない。
- 1.2 — `.env.sample` のコメントに「コンテナ内 DB（`db` ホスト）利用時のみ disable 許容」
  「外部 PostgreSQL は require 以上を明示」を記載（`.env.sample` 16-20 行）。
- 1.3 — コンテナ内 DB 例（disable）／外部 DB 例（require）を併記し、接続先に応じて利用者が
  明示選択する構造にした（`.env.sample` 26-30 行）。
- 2.1 — `docker-compose.yml` api `environment.DATABASE_URL` を
  `${DATABASE_URL:-postgres://...@db:5432/...?sslmode=disable}` とし、未指定時はコンテナ内 DB へ接続。
- 2.2 — worker 側も api と同一形式で未指定時コンテナ内 DB デフォルトを適用。
- 2.3 — `${DATABASE_URL:-...}` により api / worker 両サービスで環境別差し替えを許容。
- 2.4 — `DATABASE_URL` 未上書き時はコンテナ内 DB 向けデフォルト接続文字列を適用。
- 3.1 — README 本番デプロイ注意事項に「コンテナ内 DB 利用時のみ `sslmode=disable` 許容」を明記。
- 3.2 — README 環境変数表 `DATABASE_URL` 行 + 本番注意事項に「外部 PostgreSQL は `require` 以上必須」を明記。
- 3.3 — README 本番デプロイ注意事項に sslmode 設定手順（外部 DB の `?sslmode=require` 例含む）を追加。
- 3.4 — 外部 DB 例で disable を推奨せず require 以上を案内。ローカル開発例の `localhost` 接続のみ
  disable 許容とコメント明記（README 337-339 行付近）。
- NFR 1.1 — `${DATABASE_URL:-...}` の `:-` により未指定時は変更前と同一のコンテナ内 DB 接続文字列を
  生成。Developer が最小 compose ファイルで `docker compose config` 検証済み（impl-notes 検証 1）。
- NFR 1.2 — Go コード・DB スキーマ・マイグレーション無変更（差分は設定/ドキュメント 3 ファイルのみ）。
- NFR 2.1 — `.env.sample` / `docker-compose.yml` / `README.md` の sslmode 記述
  （コンテナ内=disable 許容 / 外部=require 以上）が相互に一貫。

## Findings

なし

## Summary

設定ファイルとドキュメントのスコープに閉じた変更で、Requirement 1〜3 と NFR 1〜2 の全 numeric ID
が差分内でカバーされている。Out of Scope（Go コード / スキーマ / マイグレーション）には触れておらず
boundary 逸脱なし。本件は実行可能コードを伴わない設定・ドキュメント変更であり、検証は
`docker compose config` による接続文字列解決確認で担保されているため missing test には当たらない。

RESULT: approve
