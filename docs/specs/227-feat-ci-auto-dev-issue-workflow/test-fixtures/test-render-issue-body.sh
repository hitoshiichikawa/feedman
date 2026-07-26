#!/usr/bin/env bash
# render-issue-body.sh の shell-level テスト
#
# 検証内容（Issue #227 / requirements.md AC）:
#   - Req 4.1: 対象領域（frontend/backend）が本文から判別できる
#   - Req 4.2: advisory ID / package / severity が本文に含まれる
#   - Req 4.3: workflow 実行 URL / ジョブ名が本文に含まれる
#   - Req 4.4: 優先度 High が本文に含まれる
#   - Req 4.5: feature.yml テンプレートの見出し構成に沿う
#     （### 種別 / ### 背景・課題 / ### 期待する挙動・ゴール / ### 受入基準の候補 / ### 優先度）
#   - Req 4.6: 重複ガード用マーカー（HTML コメント）が本文に含まれる
#
# 使い方: bash test-render-issue-body.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
RENDER="${REPO_ROOT}/.github/scripts/scheduled-audit/render-issue-body.sh"
EXTRACT_NPM="${REPO_ROOT}/.github/scripts/scheduled-audit/extract-npm-audit.sh"
EXTRACT_GO="${REPO_ROOT}/.github/scripts/scheduled-audit/extract-govulncheck.sh"

for f in "${RENDER}" "${EXTRACT_NPM}" "${EXTRACT_GO}"; do
  if [ ! -x "${f}" ]; then
    echo "FAIL: 必要なスクリプトが見つからない: ${f}" >&2
    exit 1
  fi
done

pass_count=0
fail_count=0

pass() { echo "PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "FAIL: $1" >&2; fail_count=$((fail_count + 1)); }

WORKFLOW_URL="https://github.com/hitoshi/feedman/actions/runs/1234567890"
NPM_MARKER='<!-- scheduled-audit: npm -->'
GO_MARKER='<!-- scheduled-audit: govulncheck -->'

# TSV を生成
tsv_npm=$(mktemp)
tsv_go=$(mktemp)
trap 'rm -f "${tsv_npm}" "${tsv_go}"' EXIT
bash "${EXTRACT_NPM}" < "${SCRIPT_DIR}/npm-audit-high.json" > "${tsv_npm}"
bash "${EXTRACT_GO}" < "${SCRIPT_DIR}/govulncheck-with-findings.json" > "${tsv_go}"

# ==============================================================================
# フロントエンド (npm) 用 Issue 本文
# ==============================================================================
body_npm=$(bash "${RENDER}" \
  --kind npm \
  --workflow-url "${WORKFLOW_URL}" \
  --marker "${NPM_MARKER}" \
  --job-name "frontend-audit" \
  --input "${tsv_npm}")

# Req 4.6: マーカーが含まれる
if printf '%s' "${body_npm}" | grep -qF "${NPM_MARKER}"; then
  pass "npm 版本文に重複ガードマーカー ${NPM_MARKER} が含まれる（Req 4.6）"
else
  fail "npm 版本文にマーカーが含まれない"
fi

# Req 4.1: 対象領域 (frontend / web) が判別できる
if printf '%s' "${body_npm}" | grep -qiE 'frontend|web/|npm audit'; then
  pass "npm 版本文からフロントエンド (web/) 対象が判別できる（Req 4.1）"
else
  fail "npm 版本文にフロントエンド判別記述がない"
fi

# Req 4.2: advisory ID / package / severity
if printf '%s' "${body_npm}" | grep -q 'GHSA-v6h2-p8h4-qcjw'; then
  pass "npm 版本文に advisory ID GHSA-v6h2-p8h4-qcjw が含まれる（Req 4.2）"
else
  fail "npm 版本文に advisory ID が含まれない"
fi
if printf '%s' "${body_npm}" | grep -q 'brace-expansion'; then
  pass "npm 版本文にパッケージ名 brace-expansion が含まれる（Req 4.2）"
else
  fail "npm 版本文にパッケージ名が含まれない"
fi
if printf '%s' "${body_npm}" | grep -qE 'HIGH|CRITICAL'; then
  pass "npm 版本文に severity ラベル (HIGH/CRITICAL) が含まれる（Req 4.2）"
else
  fail "npm 版本文に severity が含まれない"
fi

# Req 4.3: workflow 実行 URL / ジョブ名
if printf '%s' "${body_npm}" | grep -qF "${WORKFLOW_URL}"; then
  pass "npm 版本文に workflow 実行 URL が含まれる（Req 4.3）"
else
  fail "npm 版本文に workflow URL がない"
fi
if printf '%s' "${body_npm}" | grep -q 'frontend-audit'; then
  pass "npm 版本文にジョブ名 frontend-audit が含まれる（Req 4.3）"
else
  fail "npm 版本文にジョブ名がない"
fi

# Req 4.4: 優先度 High
if printf '%s' "${body_npm}" | grep -q 'High'; then
  pass "npm 版本文に優先度 High の記述が含まれる（Req 4.4）"
else
  fail "npm 版本文に優先度 High がない"
fi

# Req 4.5: feature.yml テンプレの見出し構成
for heading in '### 種別' '### 背景・課題' '### 期待する挙動・ゴール' '### 受入基準の候補' '### 優先度'; do
  if printf '%s' "${body_npm}" | grep -qF "${heading}"; then
    pass "npm 版本文に見出し '${heading}' が含まれる（Req 4.5）"
  else
    fail "npm 版本文に見出し '${heading}' がない"
  fi
done

# ==============================================================================
# バックエンド (govulncheck) 用 Issue 本文
# ==============================================================================
body_go=$(bash "${RENDER}" \
  --kind govulncheck \
  --workflow-url "${WORKFLOW_URL}" \
  --marker "${GO_MARKER}" \
  --job-name "backend-govulncheck" \
  --input "${tsv_go}")

# Req 4.6: マーカーが含まれる
if printf '%s' "${body_go}" | grep -qF "${GO_MARKER}"; then
  pass "go 版本文に重複ガードマーカー ${GO_MARKER} が含まれる（Req 4.6）"
else
  fail "go 版本文にマーカーが含まれない"
fi

# Req 4.1: 対象領域 (backend / Go) が判別できる
if printf '%s' "${body_go}" | grep -qiE 'backend|Go |govulncheck'; then
  pass "go 版本文からバックエンド (Go) 対象が判別できる（Req 4.1）"
else
  fail "go 版本文にバックエンド判別記述がない"
fi

# Req 4.2: OSV ID / モジュール名 / severity
if printf '%s' "${body_go}" | grep -q 'GO-2026-1234'; then
  pass "go 版本文に OSV ID GO-2026-1234 が含まれる（Req 4.2）"
else
  fail "go 版本文に OSV ID が含まれない"
fi
if printf '%s' "${body_go}" | grep -q 'golang.org/x/example'; then
  pass "go 版本文にモジュール名 golang.org/x/example が含まれる（Req 4.2）"
else
  fail "go 版本文にモジュール名が含まれない"
fi

# Req 4.3: workflow URL / ジョブ名
if printf '%s' "${body_go}" | grep -qF "${WORKFLOW_URL}"; then
  pass "go 版本文に workflow URL が含まれる（Req 4.3）"
else
  fail "go 版本文に workflow URL がない"
fi
if printf '%s' "${body_go}" | grep -q 'backend-govulncheck'; then
  pass "go 版本文にジョブ名 backend-govulncheck が含まれる（Req 4.3）"
else
  fail "go 版本文にジョブ名がない"
fi

# Req 4.4: 優先度 High
if printf '%s' "${body_go}" | grep -q 'High'; then
  pass "go 版本文に優先度 High の記述が含まれる（Req 4.4）"
else
  fail "go 版本文に優先度 High がない"
fi

# Req 4.5: 見出し構成
for heading in '### 種別' '### 背景・課題' '### 期待する挙動・ゴール' '### 受入基準の候補' '### 優先度'; do
  if printf '%s' "${body_go}" | grep -qF "${heading}"; then
    pass "go 版本文に見出し '${heading}' が含まれる（Req 4.5）"
  else
    fail "go 版本文に見出し '${heading}' がない"
  fi
done

# --- 異常系: 空の TSV 入力 → 検出ゼロ扱いだが render は空エラーで死なない ---
# （render は本来検出時のみ呼ばれるが、gate 漏れの safety として空でも動作する）
empty_tsv=$(mktemp)
trap 'rm -f "${tsv_npm}" "${tsv_go}" "${empty_tsv}"' EXIT
: > "${empty_tsv}"
if body_empty=$(bash "${RENDER}" \
  --kind npm \
  --workflow-url "${WORKFLOW_URL}" \
  --marker "${NPM_MARKER}" \
  --job-name "frontend-audit" \
  --input "${empty_tsv}" 2>&1); then
  if printf '%s' "${body_empty}" | grep -qF "${NPM_MARKER}"; then
    pass "空 TSV 入力でも render は動作し、マーカーは含まれる（safety fallback）"
  else
    fail "空 TSV でマーカーが欠落: ${body_empty}"
  fi
else
  fail "空 TSV で render が非ゼロ終了: ${body_empty}"
fi

echo ""
echo "==== RESULT: ${pass_count} passed, ${fail_count} failed ===="
[ "${fail_count}" -eq 0 ]
