# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-07-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-225-impl-fix-deps-web-npm-audit-high-brace-expans
- HEAD commit: 72a51a305dea42c969e7a09bceb620250c31a847
- Compared to: develop..HEAD

（design-less impl のため design.md / tasks.md は不在。`_Boundary:_` は無く、
変更範囲制約は requirements.md の Req 4 で判定した。Feature Flag Protocol は
CLAUDE.md で opt-out 宣言のため、通常の 3 カテゴリ判定のみを適用。）

## Verified Requirements

- 1.1 — lockfile 検証で `brace-expansion` が単一 5.0.8、`postcss` が 8.5.22/8.5.23（patched）に解決。impl-notes に `npm ci` → `npm audit --audit-level=high` exit 0 の記録あり
- 1.2 — impl-notes の after 結果で high/critical 0 件（残存は Out of Scope の moderate 3 / low 1）。lockfile 上に vulnerable な brace-expansion 1.x / postcss <=8.5.17 は残存せず
- 1.3 — CI green は PR 作成後に確定する性質。impl-notes にローカル同等コマンド（audit/lint/test/build）成功の記録あり
- 1.4 — brace-expansion GHSA-mh99-v99m-4gvg（→5.0.8）と postcss GHSA-r28c-9q8g-f849（→8.5.22/8.5.23）の両系統を解消。lockfile で確認
- 2.1 — impl-notes: `npm test`（vitest）42 files / 407 tests 全 pass
- 2.2 — impl-notes: `npm run lint` 0 error（既存 warning 5 件据え置き）
- 2.3 — impl-notes: `npm run build` 成功
- 2.4 — lockfile: eslint 9.39.3→9.39.5 / @eslint/eslintrc 3.3.4→3.3.6 / @eslint/config-array 0.21.1→0.21.2 いずれも patch レベル。eslint 連鎖は major 変更なしで維持
- 2.5 — backend（`api` / `worker` / `internal/**` / `go.mod` / `go.sum`）に変更なし。CI 判定に影響なし
- 2.6 — `web/src/**` に変更なし。ユーザー可視挙動に影響なし
- 3.1 — `@eslint/eslintrc` は 3.3.6 に維持され 0.1.0 への降格なし。`--force` 相当の破壊的ダウングレードなし
- 3.2 — eslint / eslint-config-next / @eslint/eslintrc の major 変更なし（lockfile で確認）
- 3.3 — 本番 dependencies に変更なし。package-lock.json の更新エントリはすべて `dev: true`
- 3.4 — `web/package.json` の既存 overrides 節に `"brace-expansion": "^5.0.8"` を追記（diff で確認）
- 3.5 — 追加/削除された concat-map・nested balanced-match/brace-expansion は brace-expansion 5.x 化に伴う必然的な推移的更新であり、脆弱性解消に無関係な依存変更ではない
- 3.6 — Req 3.1〜3.5 の制約内で全 high を解消済み。後続 Issue への切り出しは不要
- 4.1 — diff の主対象は `web/package.json` / `web/package-lock.json`（他は spec docs のみ）
- 4.2 — `web/src/**` を機能変更目的で変更していない
- 4.3 — ソースコードの追随修正は発生せず（該当なし）
- 4.4 — backend / インフラ設定（`Dockerfile` / `docker-compose*` / `.github/workflows/**`）に変更なし
- NFR 1.1 — `npm ci` で決定論的に再現可能な lockfile を成果物として含む
- NFR 1.2 — `--package-lock-only` 方式は今回不要（許容規定であり未使用でも AC に反しない）
- NFR 2.1 — CI ステップログ出力は既存 workflow の挙動に依存（本対応の変更対象外）

## Findings

なし

## Summary

依存メタデータ（`web/package.json` overrides + `web/package-lock.json`）のみの変更で
brace-expansion / postcss の high 系を解消しており、lockfile 検証でも解決版が patched で
あることを確認した。eslint 連鎖は patch レベルの更新のみで major 変更なし、本番 deps・
`web/src/**`・backend・infra への変更なしで Req 4 の範囲制約も満たす。全 numeric AC を
カバーしており、missing test / boundary 逸脱にも該当しない。

RESULT: approve
