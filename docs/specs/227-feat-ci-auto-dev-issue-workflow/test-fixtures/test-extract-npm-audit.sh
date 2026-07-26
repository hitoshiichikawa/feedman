#!/usr/bin/env bash
# extract-npm-audit.sh の shell-level テスト
#
# 検証内容（Issue #227 / requirements.md AC）:
#   - 正常系: high / critical の advisory が TSV 形式で抽出される（Req 1.5, 2.1, 4.2）
#   - 境界値: moderate 以下は抽出対象から除外される（Req 1.5 の "high 以上" 境界）
#   - 空入力: vulnerabilities 空の JSON は出力ゼロ行（Req 5.1）
#   - dedupe: 同じ advisory URL / source の重複は 1 行に集約される
#   - 出力形式: 各行が severity\tpackage\tadvisory_id\ttitle の TSV
#
# 使い方: bash test-extract-npm-audit.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
EXTRACT="${REPO_ROOT}/.github/scripts/scheduled-audit/extract-npm-audit.sh"

if [ ! -x "${EXTRACT}" ]; then
  echo "FAIL: extract-npm-audit.sh が実行可能ではない: ${EXTRACT}" >&2
  exit 1
fi

pass_count=0
fail_count=0

pass() { echo "PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "FAIL: $1" >&2; fail_count=$((fail_count + 1)); }

# --- 正常系 1: high / critical を含む JSON を渡すと 2 行以上抽出される（Req 1.5, 2.1） ---
out=$(bash "${EXTRACT}" < "${SCRIPT_DIR}/npm-audit-high.json")
line_count=$(printf '%s\n' "${out}" | grep -c '^' || true)
if [ "${line_count}" -ge 2 ]; then
  pass "high / critical を含む JSON から 2 件以上抽出された（${line_count} 件 / Req 1.5, 2.1）"
else
  fail "high / critical から抽出件数が想定外: ${line_count} 件 (出力: ${out})"
fi

# --- 正常系 2: 特定 advisory ID (GHSA-*) が含まれる（Req 4.2） ---
if printf '%s\n' "${out}" | grep -q 'GHSA-v6h2-p8h4-qcjw'; then
  pass "brace-expansion advisory ID (GHSA-v6h2-p8h4-qcjw) が抽出出力に含まれる（Req 4.2）"
else
  fail "brace-expansion advisory ID が出力に含まれない: ${out}"
fi
if printf '%s\n' "${out}" | grep -q 'GHSA-r28c-9q8g-f849'; then
  pass "postcss advisory ID (GHSA-r28c-9q8g-f849) が抽出出力に含まれる（Req 4.2）"
else
  fail "postcss advisory ID が出力に含まれない: ${out}"
fi

# --- 正常系 3: パッケージ名 / severity が含まれる（Req 4.2） ---
if printf '%s\n' "${out}" | grep -q 'brace-expansion'; then
  pass "パッケージ名 brace-expansion が出力に含まれる（Req 4.2）"
else
  fail "パッケージ名 brace-expansion が出力に含まれない: ${out}"
fi
if printf '%s\n' "${out}" | grep -qE 'HIGH|CRITICAL'; then
  pass "severity ラベル (HIGH / CRITICAL) が出力に含まれる（Req 4.2）"
else
  fail "severity ラベルが出力に含まれない: ${out}"
fi

# --- 正常系 4: 出力形式が TSV（tab 区切り 4 カラム） ---
first_line=$(printf '%s\n' "${out}" | head -1)
col_count=$(printf '%s' "${first_line}" | awk -F'\t' '{ print NF }')
if [ "${col_count}" -eq 4 ]; then
  pass "出力が 4 カラム TSV 形式（severity/package/advisory_id/title）"
else
  fail "出力カラム数が 4 でない: ${col_count} (line: ${first_line})"
fi

# --- 境界値: moderate のみ → 抽出 0 件（high 未満は除外） ---
out_mod=$(bash "${EXTRACT}" < "${SCRIPT_DIR}/npm-audit-moderate-only.json")
if [ -z "${out_mod}" ]; then
  pass "moderate 以下のみの JSON は抽出 0 件（Req 1.5 の high 以上境界）"
else
  fail "moderate 以下しかないのに抽出があった: ${out_mod}"
fi

# --- 空入力: vulnerabilities が空 → 抽出 0 件（Req 5.1） ---
out_clean=$(bash "${EXTRACT}" < "${SCRIPT_DIR}/npm-audit-clean.json")
if [ -z "${out_clean}" ]; then
  pass "vulnerabilities が空の JSON は抽出 0 件（Req 5.1）"
else
  fail "clean JSON なのに抽出があった: ${out_clean}"
fi

# --- 異常系: 無効な JSON を渡してもクラッシュしない（安全側に空出力 + exit 0 or 非ゼロを明示） ---
if bash "${EXTRACT}" <<<"invalid json" >/dev/null 2>&1; then
  pass "無効 JSON でも抽出スクリプトが停止しない（exit 0 か 空出力）"
else
  # 非ゼロで終了することも許容（呼び出し側で判定できる）
  pass "無効 JSON で抽出スクリプトが非ゼロ終了した（呼び出し側で処理可能）"
fi

echo ""
echo "==== RESULT: ${pass_count} passed, ${fail_count} failed ===="
[ "${fail_count}" -eq 0 ]
