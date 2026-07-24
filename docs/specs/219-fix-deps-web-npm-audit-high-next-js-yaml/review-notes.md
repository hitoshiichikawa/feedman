# Review Notes

<!-- idd-claude:review round=2 model=claude-opus-4-7 timestamp=2026-07-24T02:41:25Z -->

## Reviewed Scope

- Branch: claude/issue-219-impl-fix-deps-web-npm-audit-high-next-js-yaml
- HEAD commit: 910b7af09762b437a89ee890182bf01db67f00ed
- Compared to: develop..HEAD

commit 構成:

- `96548c1` fix(deps): web の npm audit high 検出（next / js-yaml / postcss / sharp）を解消
- `910b7af` docs(specs): #219 の impl-notes.md を追加

変更ファイル（`git diff --name-only develop..HEAD`）: `web/package.json` /
`web/package-lock.json` / `docs/specs/219-.../impl-notes.md` の 3 ファイルのみ。
本 Issue は design を経由しない design-less impl（`design.md` / `tasks.md` 不在）のため
`_Boundary:_` アノテーションは存在せず、変更範囲の境界は requirements.md の Req 4 で規定される。

## Verified Requirements

- 1.1 — reviewer が `web/` で `npm ci`（exit 0, 798 packages）→ `npm audit --audit-level=high`
  を独立再実行し **終了コード 0** を確認。
- 1.2 — 上記 `npm audit --audit-level=high` サマリが `4 vulnerabilities (1 low, 3 moderate)` で
  **high 0 件 / critical 0 件** を確認。残存は moderate（`@hono/node-server` 系 3 件）/ low
  （`esbuild` 1 件）のみで、いずれも `--audit-level=high` 閾値未満（Out of Scope）。
- 1.3 — CI の `Frontend Dependency Vulnerability Scan (npm audit)` ジョブと同一契約
  （`npm audit --audit-level=high`）がローカルで exit 0 を返すため、push 後 green に転じる見込み。
- 1.4 — lockfile 上で next 15.5.19→**15.5.21** / js-yaml 4.2.0→**4.3.0** /
  next 内 postcss→**8.5.22** / sharp 0.34.5→**0.35.3** への bump を確認（Issue 本文の
  Next.js / js-yaml / postcss / sharp のアドバイザリに対応）。
- 2.1 — reviewer が `npm test`（vitest run）を独立再実行し **Test Files 42 passed / Tests 407
  passed / exit 0** を確認。
- 2.2 — `npm run lint` は 0 error（impl-notes §4。既存 warning 5 件のみ、exit 0）。
- 2.3 — `npm run build` はエラーなく成功（impl-notes §5、`Compiled successfully`）。
- 2.4 — バックエンド（`api` / `worker` / `internal/**` / `go.mod` / `go.sum`）および
  `.github/workflows/**` に変更が無い（`git diff --name-only` は web 依存メタと impl-notes のみ）
  ため CI 他ジョブの成否判定は不変。
- 2.5 — 依存更新は semver 互換範囲に限定され、`npm test`（407 pass）で回帰不具合が観測されない。
- 3.1 — 全更新パッケージが現行 major 維持（next 15.x / js-yaml 4.x / postcss 8.x / sharp 0.x）。
- 3.2 — next は 15.5.21 で 15.x 維持（16.x への更新なし）。
- 3.3 — `web/src/app/` 配下の変更なし（diff にソース変更なし）。
- 3.4 — 該当なし（semver 互換範囲で全 high 解消。impl-notes に明示）。
- 3.5 — 変更は `overrides`（next.postcss ^8.5.22 / sharp ^0.35.3）追加と、それに伴う
  resolver 由来の transitive 追随のみ。無関係な依存追加・削除・機能更新は含まない。
- 4.1 — 主たる変更対象が `web/package.json` / `web/package-lock.json`。
- 4.2 — `web/src/**` の機能変更なし。
- 4.3 — 該当なし（ソース追随修正は発生せず）。
- 4.4 — バックエンド / インフラ設定（`Dockerfile` / `docker-compose*.yml` / `.github/workflows/**`）
  の変更なし。
- NFR 1.1 — `npm ci` で lockfile から fresh install が成功（exit 0）。決定論的に再現可能。
- NFR 1.2 — 該当なし（`npm install` の reify が成功、`--package-lock-only` 未使用）。
- NFR 2.1 — 既存 workflow を変更せず、`npm audit` サマリ出力の既存挙動を維持。

## Findings

なし（approve）。

補足: round=1 の reject 理由（依存更新が未コミットの作業ツリー変更に留まり `develop..HEAD` の
diff が空で Requirement 1 の観測可能な実装を確認できなかった）は、本 round では commit
`96548c1`（依存更新）+ `910b7af`（impl-notes 追加）として HEAD に積まれ、`develop..HEAD` の
diff として観測可能になったことで **解消済み**。reviewer が `npm ci` / `npm audit
--audit-level=high` / `npm test` を独立再実行し、いずれも exit 0（high 0 件）を確認した。

## Summary

round=1 の reject 理由（未コミットで diff 空）は commit 済みとなり解消。web 依存を semver 互換
範囲で更新（next 15.5.21 / js-yaml 4.3.0 / postcss 8.5.22 / sharp 0.35.3）し、reviewer 独立再実行で
`npm audit --audit-level=high` exit 0（high/critical 0 件）・`npm test` 407 pass を確認。全 numeric
ID をカバーし、変更範囲は web 依存メタに限定、missing test / boundary 逸脱なし。

RESULT: approve
