# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-39-impl-docker-compose-yml-db-session-secret
- HEAD commit: bc9ddb7c5d60de31dde575e8da948ec1227f2391
- Compared to: develop..HEAD

変更ファイル: `docker-compose.yml` / `.env.sample` / `README.md` /
`docs/specs/39-.../impl-notes.md` / `docs/specs/39-.../test-fixtures/test-compose-fail-fast.sh`。
本 spec ディレクトリには `tasks.md` / `design.md` が存在しない（design-less impl）。
このため `_Boundary:_` アノテーションは不在で、boundary 判定は requirements.md の
Out of Scope と実変更ファイルの突き合わせで行った。CLAUDE.md の Feature Flag Protocol は
`opt-out` のため flag 観点の確認は行わない。

## Verified Requirements

- 1.1 — `db` の `"POSTGRES_PASSWORD=${POSTGRES_PASSWORD:?...}"`（docker-compose.yml）。空文字でも非ゼロ終了をスクリプトで確認（test-compose-fail-fast.sh「空文字でも config が非ゼロ終了」）
- 1.2 — `api`/`worker` の `"SESSION_SECRET=${SESSION_SECRET:?...}"`。同上スクリプトで空文字 fail-fast 確認
- 1.3 — POSTGRES_PASSWORD 未設定で `docker compose config` が非ゼロ終了（スクリプト異常系 1・2 で PASS）
- 1.4 — SESSION_SECRET 未設定で `docker compose config` が非ゼロ終了（スクリプト異常系 1・3 で PASS）
- 1.5 — `${VAR:?...}` のメッセージに var 名 + 生成コマンドを含む。stderr に `POSTGRES_PASSWORD`/`SESSION_SECRET` が出ることを確認
- 2.1 — `SESSION_SECRET=${SESSION_SECRET:-dummy}`（api/worker）を `:?...` に置換済み（diff で確認）
- 2.2 — `POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-feedman}`（db）を `:?...` に置換済み（diff で確認）
- 2.3 — DATABASE_URL デフォルトのネスト `${POSTGRES_PASSWORD:-feedman}` を `${POSTGRES_PASSWORD}` に変更。解決後 URL に `feedman:feedman@` が現れないことをスクリプトで確認
- 2.4 — `POSTGRES_USER:-feedman` / `POSTGRES_DB:-feedman` を温存。config 出力に `feedman` 適用をスクリプトで確認
- 3.1 — `web`/`api`/`worker`/`db` の 4 サービスが config で解決されることを確認（実起動は環境固有の既存 healthcheck 記法制約によりサンドボックスで未実行。impl-notes.md にスコープ外と明記、shim で 4 サービス解決を確認済み）
- 3.2 — 両秘匿情報設定時にインターポレーションが解決される（スクリプト正常系で PASS）
- 3.3 — 既存 env var 名（POSTGRES_USER/POSTGRES_PASSWORD/POSTGRES_DB/SESSION_SECRET/DATABASE_URL）の改名なし（diff で確認）
- 4.1 — `.env.sample` の SESSION_SECRET 節に `openssl rand -base64 32` 生成コマンドを併記
- 4.2 — `.env.sample` の必須節に POSTGRES_PASSWORD を移し `openssl rand -base64 32` を併記
- 4.3 — README 環境変数テーブルで POSTGRES_PASSWORD/SESSION_SECRET を「起動に必須」と明示
- 4.4 — README「環境変数の設定」「本番デプロイ時の注意事項」に未設定時の fail-fast と回避手順（値生成）を記載
- NFR 1.1 — env var 改名なし（diff で確認）
- NFR 1.2 — サービス/ネットワーク/ポート定義は不変（diff は environment 値のみの変更）
- NFR 2.1 — 単一 `.env` 必須化を維持（compose ファイル分離なし）
- NFR 2.2 — エラーメッセージに var 名を含む（スクリプトで確認）
- NFR 3.1 — `.env.sample` はプレースホルダのみ（実値コミットなし）
- NFR 3.2 — エラーメッセージは var 名 + 生成コマンドのみで実値を出力しない（`${VAR:?...}` の仕様）

## Findings

なし

## Summary

requirements.md の全 numeric ID（Req 1.1〜4.4 / NFR 1.1〜3.2）が docker-compose.yml の
`${VAR:?...}` 必須化・弱いデフォルト排除と .env.sample/README の手順追記でカバーされ、
検証スクリプトを reviewer 環境で再実行し 12 passed, 0 failed を確認した。変更は compose/
env/README/検証スクリプトに限られ、Out of Scope（アプリコード・compose 分離）を逸脱しない。

RESULT: approve
