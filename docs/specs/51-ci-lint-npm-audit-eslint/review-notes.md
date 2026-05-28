# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-51-impl-ci-lint-npm-audit-eslint
- HEAD commit: 5073112bb8bb626e3fa2e0b2d2e98e9021d45209
- Compared to: develop..HEAD

本 Issue は design-less impl（`design.md` / `tasks.md` なし）。`_Boundary:_` アノテーションが
存在しないため boundary 逸脱判定は「Issue スコープ（CI ワークフロー変更）に閉じているか」で
代替確認した。Feature Flag Protocol は CLAUDE.md で `opt-out` 宣言のため flag 観点の確認は
行わない（通常の 3 カテゴリ判定のみ）。差分は `.github/workflows/ci.yml`（新規 2 ジョブ追加）と
spec docs（requirements.md / impl-notes.md）のみで、scope 内に閉じている。

## Verified Requirements

- 1.1 — `frontend-lint` ジョブが `on.pull_request.branches: [main, develop]` 配下で
  `defaults.run.working-directory: web` + `run: npm run lint` を実行（ci.yml:73-92）。
  `web/package.json` の `"lint": "eslint"` も確認済み。
- 1.2 — `npm run lint`（eslint）は lint 違反検出時に非ゼロ終了し、exit code 抑制（`|| true` 等）が
  無いため step が fail → ジョブ失敗 → PR チェック fail（ci.yml:91-92）。
- 1.3 — eslint がゼロ終了すればジョブ成功（同上、追加抑制なし）。
- 1.4 — `npm run lint` の stdout はジョブログに出力（出力リダイレクト・抑制なし、ci.yml:92）。
- 2.1 — `frontend-audit` ジョブが PR トリガー配下で `working-directory: web` +
  `run: npm audit --audit-level=high` を実行（ci.yml:94-113）。
- 2.2 — `--audit-level=high` を明示指定（ci.yml:113）。
- 2.3 — `npm audit --audit-level=high` は high/critical 検出時に非ゼロ終了 → ジョブ失敗
  （exit code 抑制なし、ci.yml:112-113）。
- 2.4 — `--audit-level=high` 指定時、moderate 以下のみならゼロ終了 → ジョブ成功（同上）。
- 2.5 — high 以上の検出なしならゼロ終了 → ジョブ成功（同上）。
- 2.6 — `npm audit` の検出一覧 stdout はジョブログに出力（抑制なし、ci.yml:113）。
- 3.1 — 既存 `frontend`（`npm test`）ジョブ定義は無変更（diff は新規 2 ジョブ追加のみ、ci.yml:52-71）。
- 3.2 — 既存 `backend` / `go-vet` / `govulncheck` の各ジョブ定義は無変更（diff に含まれず、
  ci.yml:9-50）。トリガー `on`（push/pull_request to main/develop）も無変更（ci.yml:3-7）。
- 3.3 — lint / audit を `frontend` とは別の独立ジョブにし、ジョブ間に `needs` 依存を張らないため
  並列・独立に成否判定される（ci.yml:73-113）。
- NFR 1.1 — 独立ジョブ + 非ゼロ終了でブロックという govulncheck と同型の構成（ci.yml:36-50 と
  73-113 が同型）。
- NFR 1.2 — `--audit-level=high` の足切りにより moderate 以下ではブロックしない（2.4 と同根拠）。
- NFR 2.1 — `frontend-lint` / `frontend-audit` を独立ジョブとし、固有の `name`
  （`Frontend Lint (eslint)` / `Frontend Dependency Vulnerability Scan (npm audit)`）を付与
  したため PR チェック一覧で他ジョブと区別可能（ci.yml:74, 95）。

## Findings

なし

## Summary

requirements.md の全 numeric ID（1.1-1.4 / 2.1-2.6 / 3.1-3.3 / NFR 1.1 / 1.2 / 2.1）が
ci.yml の新規 2 ジョブ追加と既存ジョブ無変更で裏付けられている。成果物は GitHub Actions YAML
であり Feedman の規約上 CI 定義に対する単体テストは要求されず、impl-notes.md に各 AC と
検証手段の対応が明記されているため missing test に該当しない。boundary 逸脱もなし。

RESULT: approve
