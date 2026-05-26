# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-52-impl-ci-trivy
- HEAD commit: 3be283a32f5500736fdc3e544c9d1313dab24cb5
- Compared to: develop..HEAD
- 変更ファイル: `.github/workflows/ci.yml`（trivy job 追加のみ）+ spec docs（requirements.md / impl-notes.md）
- Feature Flag Protocol: CLAUDE.md `**採否**: opt-out` のため flag 観点は適用せず、通常の 3 カテゴリ判定のみ実施
- 本 Issue は design-less impl（`design.md` / `tasks.md` 不在）。`_Boundary:_` アノテーションが無いため、boundary 判定は requirements の Out of Scope と変更ファイルの妥当性で確認した

## Verified Requirements

- 1.1 — `.github/workflows/ci.yml` の trivy job「Build api/worker image」（`docker build -t feedman-api:scan -f Dockerfile .`）+「Scan api/worker image」（`aquasecurity/trivy-action@0.28.0`, scan-type: image, image-ref: feedman-api:scan）。ルート Dockerfile を image スキャン
- 1.2 — 同 job「Build web image」（`docker build -t feedman-web:scan -f web/Dockerfile web`）+「Scan web image」。`web/Dockerfile` を image スキャン
- 1.3 — 両スキャンステップに `severity: CRITICAL,HIGH`。image スキャン採用によりベースイメージ（distroless-debian12 / node:20-alpine）の OS パッケージ層 CVE を検出対象に含める
- 1.4 — `exit-code: '0'` により脆弱性無し時も job 正常終了
- 2.1 — 既存 `on.pull_request.branches: [main, develop]` をそのまま利用し trivy job も同トリガーで起動（無変更）
- 2.2 — 既存 `on.push.branches: [main, develop]` 無変更。トリガー定義に変更なし
- 2.3 — 独立 job `trivy`（name: `Container Image Vulnerability Scan (Trivy)`）として定義され CI checks に表示される
- 3.1 — job レベル `continue-on-error: true` + ステップの `exit-code: '0'` の二重担保で検出時も fail させない
- 3.2 — 「Publish scan results to job summary」（`if: always()`）で table 出力を `$GITHUB_STEP_SUMMARY` に掲載。CI ログにも table 出力が残る
- 3.3 — job レベル `continue-on-error: true` により docker build 失敗を含め job が常に成功扱いとなり PR をブロックしない
- 4.1 — 既存 6 job（backend / go-vet / govulncheck / frontend / frontend-lint / frontend-audit）の定義に変更なし（develop..HEAD diff で trivy 追加のみを確認）
- 4.2 — trivy は `needs` 無しの独立 job。他 job の成否に影響しない
- 4.3 — Dockerfile / コンテナイメージ（ベースイメージ・OS パッケージ）が対象で、govulncheck（Go 依存）/ npm audit（npm 依存）とスコープが重複しない
- NFR 1.1 — 軽量ベースイメージ（distroless / alpine）で目安 5 分以内に収まる見込み（impl-notes に根拠記載）
- NFR 1.2 — image スキャン + severity 絞り込みで抑制可能。schedule 化等の余地を確保
- NFR 2.1 — 既存 job 定義は無変更（diff で確認）
- NFR 2.2 — `needs` 無し独立 job + `continue-on-error: true` で他 job・CI 全体の結果に影響しない
- NFR 3.1 — `severity: CRITICAL,HIGH` の table 出力を CI ログ + Step Summary の双方に残し重大度・件数を確認可能

## Findings

なし

## Summary

全 numeric AC（1.1〜4.3）および NFR が trivy job 追加により実装でカバーされている。変更は `.github/workflows/ci.yml` への独立 job 追加のみで既存ジョブ・トリガー定義は無変更、Out of Scope の逸脱も無く boundary 逸脱は検出されなかった。本変更は GitHub Actions ワークフロー YAML であり単体テストフレームワークの対象外で、impl-notes に YAML 構文検証 + actionlint + diff 確認の検証結果と AC トレーサビリティが明記されているため missing test には該当しない。

RESULT: approve
