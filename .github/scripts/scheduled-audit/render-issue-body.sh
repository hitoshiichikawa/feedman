#!/usr/bin/env bash
# render-issue-body.sh: scheduled-audit で起票する Issue の本文を markdown で render する
#
# 使い方:
#   render-issue-body.sh \
#     --kind (npm|govulncheck) \
#     --workflow-url <URL> \
#     --marker <HTML コメントマーカー> \
#     --job-name <ジョブ名> \
#     --input <TSV ファイル>
#
# TSV フォーマット（extract-npm-audit.sh / extract-govulncheck.sh の出力）:
#   severity \t package \t advisory_id \t title
#
# 出力: stdout に markdown 本文
#   - Req 4.1: 対象領域 (frontend / backend) 判別
#   - Req 4.2: advisory ID / package / severity
#   - Req 4.3: workflow 実行 URL / ジョブ名
#   - Req 4.4: 優先度 High
#   - Req 4.5: feature.yml テンプレート見出し構成
#   - Req 4.6: 重複ガードマーカー
set -euo pipefail

KIND=""
WORKFLOW_URL=""
MARKER=""
JOB_NAME=""
INPUT=""

usage() {
  cat >&2 <<'EOF'
Usage: render-issue-body.sh --kind (npm|govulncheck) --workflow-url URL --marker MARKER --job-name NAME --input TSV_FILE
EOF
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --kind)         KIND="$2"; shift 2 ;;
    --workflow-url) WORKFLOW_URL="$2"; shift 2 ;;
    --marker)       MARKER="$2"; shift 2 ;;
    --job-name)     JOB_NAME="$2"; shift 2 ;;
    --input)        INPUT="$2"; shift 2 ;;
    -h|--help)      usage ;;
    *) echo "Unknown argument: $1" >&2; usage ;;
  esac
done

[ -z "${KIND}" ] && { echo "--kind is required" >&2; usage; }
[ -z "${WORKFLOW_URL}" ] && { echo "--workflow-url is required" >&2; usage; }
[ -z "${MARKER}" ] && { echo "--marker is required" >&2; usage; }
[ -z "${JOB_NAME}" ] && { echo "--job-name is required" >&2; usage; }
[ -z "${INPUT}" ] && { echo "--input is required" >&2; usage; }
[ ! -f "${INPUT}" ] && { echo "Input TSV file not found: ${INPUT}" >&2; exit 1; }

case "${KIND}" in
  npm)
    # shellcheck disable=SC2016  # backtick は markdown リテラルとしてそのまま出力する
    AREA_LABEL='フロントエンド (`web/` の npm 依存)'
    SCAN_TOOL="npm audit --audit-level=high"
    TABLE_HEADER_4TH="Title"
    FIX_INSTR="\`web/\` で \`npm audit --audit-level=high\` が exit 0 (クリーン) になる状態"
    AC_LINES=$'- `web/` ディレクトリで `npm audit --audit-level=high` が exit 0 で完了すること\n- 上表の advisory ID がいずれも検出されなくなること\n- 既存テスト・既存機能が壊れていないこと（`npm test` / `npm run lint` / `npm run build` が全て通る）\n- 破壊的なメジャーダウングレード（`npm audit fix --force`）は行わないこと'
    NOTES=$'- 過去の同型対応: PR #210 (2026-06) / #219→PR #221 (2026-07-24) / #225→PR #226 (2026-07-26)\n- `web/package.json` の `overrides` 節で修正版に固定する方式は許容（前例あり）'
    ;;
  govulncheck)
    AREA_LABEL="バックエンド (Go 依存)"
    SCAN_TOOL="govulncheck ./..."
    TABLE_HEADER_4TH="Summary"
    FIX_INSTR="\`govulncheck ./...\` が exit 0 (呼び出し到達 vulnerability ゼロ) になる状態"
    AC_LINES=$'- リポジトリルートで `govulncheck ./...` が exit 0 で完了すること\n- 上表の OSV ID がいずれも呼び出し到達 vulnerability として検出されなくなること\n- 既存テスト（`go test ./...`）と `go vet ./...` が引き続き pass すること'
    NOTES=$'- 呼び出し到達なし (import のみ) の finding は本 Issue のスコープ外\n- 依存モジュールの semver 互換更新で解消できない場合は代替 API 移行を検討する'
    ;;
  *)
    echo "Invalid --kind: ${KIND} (expected: npm | govulncheck)" >&2
    exit 2
    ;;
esac

DATE_UTC="$(date -u +%Y-%m-%d)"

# 検出テーブル本文（TSV から markdown table）
if [ -s "${INPUT}" ]; then
  TABLE_BODY=$(awk -F'\t' -v OFS='' '
    {
      # markdown table セル内の | / 改行を安全化
      for (i = 1; i <= 4; i++) {
        gsub(/\|/, "\\|", $i)
      }
      print "| ", $1, " | `", $2, "` | ", $3, " | ", $4, " |"
    }' "${INPUT}")
  DETECTION_COUNT=$(wc -l < "${INPUT}" | tr -d ' ')
else
  TABLE_BODY="| (検出なし) | - | - | - |"
  DETECTION_COUNT=0
fi

# ---- 本文出力 ----
# マーカーを先頭に置くことで gh issue list --search でも grep でも判定容易にする
cat <<EOF
${MARKER}

### 種別

不具合修正

### 背景・課題（なぜ必要か）

定期スキャン workflow (\`scheduled-audit\`) が ${DATE_UTC} (UTC) 実行時に、${AREA_LABEL} で
**${DETECTION_COUNT} 件の脆弱性 advisory** を検出しました（\`${SCAN_TOOL}\` 相当）。

このまま放置すると、次回 push / pull_request イベントで既存 CI (\`.github/workflows/ci.yml\`)
の依存脆弱性スキャンジョブが fail し、develop 宛ての全 open PR の CI が巻き添えで赤くなる
可能性があります（過去 3 回同型対応: #210 / #219 / #225）。

- 検出元ジョブ: \`${JOB_NAME}\`
- Workflow 実行 URL: ${WORKFLOW_URL}
- スキャン日時: ${DATE_UTC} (UTC)

### 現状の挙動

- 定期スキャン (\`${SCAN_TOOL}\`) が ${DETECTION_COUNT} 件の advisory を検出
- 既存 CI の依存脆弱性スキャンジョブが次回イベントで fail する可能性あり

#### 検出された脆弱性

| Severity | Package | Advisory ID | ${TABLE_HEADER_4TH} |
|---|---|---|---|
${TABLE_BODY}

### 期待する挙動・ゴール

- ${FIX_INSTR}
- 上記 advisory の解消により、既存 CI (\`.github/workflows/ci.yml\`) の依存脆弱性ジョブが
  引き続き green で完了する
- 定期スキャン workflow の次回実行で同じ advisory が再検出されない

### 受入基準の候補

${AC_LINES}

### スコープ外（今回は含めたくないこと）

- 検出された特定 advisory 以外の脆弱性対応（別 Issue で扱う）
- 依存ライブラリのメジャーバージョン更新（semver 互換範囲での修正を優先）
- 定期スキャン workflow (\`.github/workflows/scheduled-audit.yml\`) 自体の変更

### 影響範囲のヒント（任意）

${NOTES}

### 参考資料（任意）

- 定期スキャン workflow: \`.github/workflows/scheduled-audit.yml\`
- 検出元 workflow 実行: ${WORKFLOW_URL}

### 優先度

High（数日以内）
EOF
