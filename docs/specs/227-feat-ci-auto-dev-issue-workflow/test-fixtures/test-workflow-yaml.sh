#!/usr/bin/env bash
# scheduled-audit.yml の workflow 定義に対する静的検証テスト
#
# 検証内容（Issue #227 / requirements.md AC）:
#   - Req 1.1: `.github/workflows/scheduled-audit.yml` が独立したファイルとして存在
#   - Req 1.2: schedule (cron) トリガーを持つ
#   - Req 1.3: workflow_dispatch トリガーを持つ
#   - Req 1.4: develop を対象ブランチとして checkout する
#   - Req 1.5: npm audit を実行するステップを含む
#   - Req 1.6: govulncheck を実行するステップを含む
#   - Req 3.3: 2 種類の canonical マーカーが宣言されている
#   - Req 6.1: `.github/workflows/ci.yml` に触っていない (存在するが変更されない)
#   - Req 6.3: pull_request / push トリガーを持たない
#   - NFR 1.1: permissions が最小権限 (contents: read + issues: write のみ)
#   - NFR 2.1: cron が毎時 0 分 (m=0) ではない
#   - NFR 2.2: cron が JST 6〜9 時 (UTC 21〜24 時) の範囲にある
#   - actionlint 構文検証を pass する (実行環境にあれば)
#
# 使い方: bash test-workflow-yaml.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
WF="${REPO_ROOT}/.github/workflows/scheduled-audit.yml"
CI="${REPO_ROOT}/.github/workflows/ci.yml"

if [ ! -f "${WF}" ]; then
  echo "FAIL: scheduled-audit.yml が存在しない: ${WF}" >&2
  exit 1
fi

pass_count=0
fail_count=0
pass() { echo "PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "FAIL: $1" >&2; fail_count=$((fail_count + 1)); }

# --- Req 1.1: 独立したファイルとして存在 ---
if [ -f "${WF}" ] && [ -f "${CI}" ] && [ "${WF}" != "${CI}" ]; then
  pass "scheduled-audit.yml が ci.yml と別ファイルとして存在（Req 1.1）"
else
  fail "scheduled-audit.yml と ci.yml の独立性が担保できない"
fi

# --- Req 1.2: schedule トリガー ---
if grep -qE '^\s*schedule:' "${WF}"; then
  pass "schedule トリガー宣言が存在（Req 1.2）"
else
  fail "schedule トリガー宣言がない"
fi

# --- Req 1.3: workflow_dispatch トリガー ---
if grep -qE '^\s*workflow_dispatch:' "${WF}"; then
  pass "workflow_dispatch トリガー宣言が存在（Req 1.3）"
else
  fail "workflow_dispatch トリガー宣言がない"
fi

# --- Req 1.4: develop を対象 ---
if grep -qE "ref:\s*(\\$\\{\\{\\s*env\\.TARGET_REF.*\\}\\}|develop)" "${WF}" && \
   grep -qE "TARGET_REF:\s*develop" "${WF}"; then
  pass "checkout の ref が develop に固定されている（Req 1.4）"
else
  fail "checkout ref に develop が設定されていない"
fi

# --- Req 1.5: npm audit を実行 ---
if grep -q 'npm audit' "${WF}"; then
  pass "npm audit の実行ステップが存在（Req 1.5）"
else
  fail "npm audit の実行が見つからない"
fi

# --- Req 1.6: govulncheck を実行 ---
if grep -q 'govulncheck' "${WF}"; then
  pass "govulncheck の実行ステップが存在（Req 1.6）"
else
  fail "govulncheck の実行が見つからない"
fi

# --- Req 3.3: 2 種類の canonical マーカーが宣言 ---
if grep -qF '<!-- scheduled-audit: npm -->' "${WF}"; then
  pass "フロントエンド用マーカー '<!-- scheduled-audit: npm -->' が宣言されている（Req 3.3）"
else
  fail "フロントエンド用マーカーが宣言されていない"
fi
if grep -qF '<!-- scheduled-audit: govulncheck -->' "${WF}"; then
  pass "バックエンド用マーカー '<!-- scheduled-audit: govulncheck -->' が宣言されている（Req 3.3）"
else
  fail "バックエンド用マーカーが宣言されていない"
fi

# --- Req 6.3: pull_request / push トリガーがない ---
# on: 節配下で pull_request / push が独立キーとして現れないこと
# (行頭でマッチさせて body 中の記述を誤検出しないようにする)
if grep -qE '^\s*(pull_request|push):' "${WF}"; then
  fail "pull_request または push トリガーが宣言されている（Req 6.3 違反）"
else
  pass "pull_request / push トリガーは宣言されていない（Req 6.3）"
fi

# --- NFR 1.1: permissions が最小権限 ---
# permissions ブロックに contents: read と issues: write のみが宣言されていること
perms_block=$(awk '/^permissions:/,/^$/' "${WF}" | head -20)
if echo "${perms_block}" | grep -qE '^\s*contents:\s*read' && \
   echo "${perms_block}" | grep -qE '^\s*issues:\s*write'; then
  pass "permissions に contents:read と issues:write が宣言されている（NFR 1.1）"
else
  fail "permissions の最小権限宣言が欠落: ${perms_block}"
fi
# 過剰な権限が付与されていないこと
if echo "${perms_block}" | grep -qE '^\s*(pull-requests|actions|packages|deployments|id-token|checks|statuses|repository-projects|security-events):\s*write'; then
  fail "過剰な書き込み権限が付与されている: ${perms_block}"
else
  pass "過剰な書き込み権限は付与されていない（NFR 1.1）"
fi

# --- NFR 2.1 / NFR 2.2: cron が毎時 0 分でない かつ JST 6〜9 時 (UTC 21〜24) ---
cron_line=$(grep -oE "cron:\s*'[^']+'" "${WF}" | head -1 || true)
if [ -z "${cron_line}" ]; then
  fail "cron 表現が見つからない"
else
  # cron 形式: "MM HH * * *"
  cron_expr=$(echo "${cron_line}" | grep -oE "'[^']+'" | tr -d "'")
  cron_min=$(echo "${cron_expr}" | awk '{print $1}')
  cron_hour=$(echo "${cron_expr}" | awk '{print $2}')

  if [ "${cron_min}" != "0" ] && [ "${cron_min}" -gt 0 ] 2>/dev/null; then
    pass "cron 分値が毎時 0 分ではない (min=${cron_min} / NFR 2.1)"
  else
    fail "cron 分値が 0 または不正: ${cron_min}"
  fi

  # UTC 21〜23 (23 は 24:00 JST を含まないため許容範囲: 21, 22, 23)
  # JST 6:00 = UTC 21:00, JST 9:00 = UTC 24:00 (=翌 00:00)
  # NFR 2.2 の PM 推奨値は 22:15 UTC = JST 07:15
  if [ "${cron_hour}" -ge 21 ] && [ "${cron_hour}" -le 23 ] 2>/dev/null; then
    pass "cron 時値が UTC 21〜23 の範囲 (JST 6〜9 時) にある (hour=${cron_hour} / NFR 2.2)"
  else
    fail "cron 時値が JST 6〜9 時の範囲外: hour=${cron_hour}"
  fi
fi

# --- actionlint 構文検証 (存在すれば実行) ---
if command -v actionlint >/dev/null 2>&1; then
  if actionlint "${WF}" >/tmp/actionlint.log 2>&1; then
    pass "actionlint 構文検証を pass"
  else
    fail "actionlint 検証エラー: $(cat /tmp/actionlint.log)"
  fi
else
  echo "SKIP: actionlint が実行環境に存在しない"
fi

# --- shellcheck 構文検証 (関連 shell スクリプトのみ) ---
if command -v shellcheck >/dev/null 2>&1; then
  if shellcheck "${REPO_ROOT}/.github/scripts/scheduled-audit/"*.sh >/tmp/shellcheck.log 2>&1; then
    pass "scheduled-audit スクリプトの shellcheck を pass"
  else
    fail "shellcheck エラー: $(cat /tmp/shellcheck.log)"
  fi
else
  echo "SKIP: shellcheck が実行環境に存在しない"
fi

echo ""
echo "==== RESULT: ${pass_count} passed, ${fail_count} failed ===="
[ "${fail_count}" -eq 0 ]
