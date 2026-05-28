# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-97-impl-web-next-js-standalone-hostname
- HEAD commit: 7a2e4fd4e697270bf5c4c393d5be5617308bd4f5
- Compared to: develop..HEAD

差分は `web/Dockerfile`（runner ステージへの `ENV HOSTNAME=0.0.0.0` / `ENV PORT=3000` 追加）と
新規 `web/dockerfile-hostname.test.ts`（回帰ガード）、および spec ドキュメント
（`requirements.md` / `impl-notes.md`）のみ。design なし impl のため `tasks.md` は不在で
`_Boundary:_` アノテーションは存在しない。Feature Flag Protocol は CLAUDE.md にて `opt-out`
宣言のため flag 観点は適用しない（通常の 3 カテゴリ判定のみ）。

## Verified Requirements

- 1.1 — `web/Dockerfile:38` `ENV HOSTNAME=0.0.0.0` で全 interface bind を image に組み込み。
  `dockerfile-hostname.test.ts`「runner ステージに ENV HOSTNAME=0.0.0.0」で回帰ガード
- 1.2 — 起動ログ Network 行 `http://0.0.0.0:3000` は HOSTNAME bind の結果 Next.js standalone
  server が出力する。unit 検証不能、impl-notes.md で手動 / staging 検証へ紐付け済み
- 1.3 — 公開ポート経由 `/` で HTTP 200。HOSTNAME=0.0.0.0 bind により到達可能。unit 検証不能、
  impl-notes.md で手動 / staging 検証へ紐付け済み（Issue 本文でも staging 確認済みと明記）
- 1.4 — 応答 HTML の `<title>Feedman - RSS リーダー</title>`。bind 範囲拡大のみで応答内容不変。
  unit 検証不能、impl-notes.md で手動 / staging 検証へ紐付け済み
- 1.5 — ホスト側ポート変更後の到達性は既存 compose `${WEB_PORT}` マッピングで担保（本変更非対象）。
  impl-notes.md で紐付け済み
- 2.1 — 外部 env 無しでも全 interface bind（image 自己完結）。`ENV HOSTNAME=0.0.0.0` +
  `dockerfile-hostname.test.ts` で回帰ガード
- 2.2 — 外部 env 指定時の優先は Docker の env 解決順（`docker-compose.yml` の environment /
  `docker run -e` が `ENV` を上書き）で担保。impl-notes.md に明記
- 2.3 — `web/Dockerfile:40` `ENV PORT=3000` で待ち受けポート明示。
  `dockerfile-hostname.test.ts`「runner ステージに ENV PORT=3000」で回帰ガード。上書きは env 優先順
- 3.1 — `API_INTERNAL_URL` 未設定 / 空 / 空白での fail-fast は既存
  `web/src/lib/rewrites.test.ts` の `resolveApiInternalUrl` テスト群（rewrites.test.ts:75/83/91、
  いずれも throw を assert）で担保。本変更は entrypoint の fail-fast を変更しないため既存挙動保持
- 3.2 — 有効時は起動継続し全 interface bind。HOSTNAME bind + entrypoint 通過で担保。unit 検証
  不能分は手動 / staging 検証へ紐付け済み
- 3.3 — 公開ポート未マッピング網からの到達可否はネットワーク分離（`internal` / `external`）を
  非変更とすることで従来どおり維持。`web/Dockerfile` のみ変更で担保（Out of Scope）
- NFR 1.1 — 変更前後で `/` の 200 応答・HTML 同一。bind 範囲拡大のみで応答内容不変。手動 /
  staging 検証へ紐付け済み
- NFR 1.2 — api / worker / db の起動・通信挙動を変更しない。`web/Dockerfile` のみ変更で担保
- NFR 2.1 — 待ち受けアドレスを起動ログに出力。Next.js standalone server の Network 行出力で担保。
  unit 検証不能、手動 / staging 検証へ紐付け済み

## Findings

なし

## Summary

全 numeric ID（Req 1.1〜1.5 / 2.1〜2.3 / 3.1〜3.3 / NFR 1.1〜2.1）に対し、観測可能な image 設定
（`ENV HOSTNAME=0.0.0.0` / `ENV PORT=3000`）と回帰テスト、もしくは既存テスト・手動 staging 検証への
明示的な紐付けが impl-notes.md に揃っている。新規 image 設定には `dockerfile-hostname.test.ts` の
回帰テストが追加され、Req 3.1 の fail-fast は既存 `rewrites.test.ts` でカバー済み（紐付け明記）。
変更は `web/Dockerfile` と近傍テストのみで Out of Scope への侵食はなく、Feature Flag Protocol は
opt-out のため flag 観点は適用外。AC 未カバー / missing test / boundary 逸脱のいずれも検出されない。

RESULT: approve
