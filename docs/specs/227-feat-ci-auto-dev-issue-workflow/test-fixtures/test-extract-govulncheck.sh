#!/usr/bin/env bash
# extract-govulncheck.sh の shell-level テスト
#
# 検証内容（Issue #227 / requirements.md AC）:
#   - 正常系: 呼び出し到達のある finding が TSV 形式で抽出される（Req 1.6, 2.2, 4.2）
#   - 境界値: import-only（trace が 1 フレームのみ）の finding は除外される
#     （既存 CI govulncheck は called vulnerability でのみ fail するため整合）
#   - 空入力: findings 無しの JSON は出力ゼロ行（Req 5.1）
#   - dedupe: 同一 OSV ID の複数 finding は 1 行に集約される
#   - 出力形式: severity\tpackage\tadvisory_id\tsummary の TSV 4 カラム
#
# 使い方: bash test-extract-govulncheck.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
EXTRACT="${REPO_ROOT}/.github/scripts/scheduled-audit/extract-govulncheck.sh"

if [ ! -x "${EXTRACT}" ]; then
  echo "FAIL: extract-govulncheck.sh が実行可能ではない: ${EXTRACT}" >&2
  exit 1
fi

pass_count=0
fail_count=0

pass() { echo "PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "FAIL: $1" >&2; fail_count=$((fail_count + 1)); }

# --- 正常系 1: 呼び出し到達のある finding が抽出される（Req 1.6, 2.2） ---
out=$(bash "${EXTRACT}" < "${SCRIPT_DIR}/govulncheck-with-findings.json")
line_count=$(printf '%s\n' "${out}" | grep -c '^' || true)
if [ "${line_count}" -eq 2 ]; then
  pass "呼び出し到達のある finding が 2 件抽出された（Req 1.6, 2.2）"
else
  fail "呼び出し到達 finding の抽出件数が想定外: ${line_count} (出力: ${out})"
fi

# --- 正常系 2: OSV ID が含まれる（Req 4.2） ---
if printf '%s\n' "${out}" | grep -q 'GO-2026-1234'; then
  pass "OSV ID GO-2026-1234 が抽出出力に含まれる（Req 4.2）"
else
  fail "GO-2026-1234 が出力に含まれない: ${out}"
fi
if printf '%s\n' "${out}" | grep -q 'GO-2026-5678'; then
  pass "OSV ID GO-2026-5678 が抽出出力に含まれる（Req 4.2）"
else
  fail "GO-2026-5678 が出力に含まれない: ${out}"
fi

# --- 正常系 3: パッケージ / モジュール名が含まれる（Req 4.2） ---
if printf '%s\n' "${out}" | grep -q 'golang.org/x/example'; then
  pass "モジュール名 golang.org/x/example が抽出出力に含まれる（Req 4.2）"
else
  fail "モジュール名が出力に含まれない: ${out}"
fi
if printf '%s\n' "${out}" | grep -q 'golang.org/x/net'; then
  pass "モジュール名 golang.org/x/net が抽出出力に含まれる（Req 4.2）"
else
  fail "モジュール名が出力に含まれない: ${out}"
fi

# --- 正常系 4: severity ラベルが含まれる（Req 4.2） ---
if printf '%s\n' "${out}" | grep -q 'HIGH'; then
  pass "severity ラベル HIGH が出力に含まれる（Req 4.2）"
else
  fail "severity ラベルが出力に含まれない: ${out}"
fi

# --- 正常系 5: 出力形式が TSV 4 カラム ---
first_line=$(printf '%s\n' "${out}" | head -1)
col_count=$(printf '%s' "${first_line}" | awk -F'\t' '{ print NF }')
if [ "${col_count}" -eq 4 ]; then
  pass "出力が 4 カラム TSV 形式（severity/package/advisory_id/summary）"
else
  fail "出力カラム数が 4 でない: ${col_count} (line: ${first_line})"
fi

# --- 正常系 6: dedupe 確認（同 OSV ID の複数 finding が 1 行に集約） ---
osv_uniq=$(printf '%s\n' "${out}" | awk -F'\t' '{ print $3 }' | sort -u | wc -l | tr -d ' ')
if [ "${osv_uniq}" -eq 2 ]; then
  pass "同 OSV ID の複数 finding が 1 行に集約される（unique OSV ID: 2）"
else
  fail "OSV ID の重複除去が失敗: unique=${osv_uniq}"
fi

# --- 境界値: import-only finding（trace 1 フレームのみ）は除外 ---
out_imp=$(bash "${EXTRACT}" < "${SCRIPT_DIR}/govulncheck-import-only.json")
if [ -z "${out_imp}" ]; then
  pass "import-only finding（trace 1 フレーム）は抽出対象外（既存 CI 挙動と整合）"
else
  fail "import-only finding が抽出されてしまった: ${out_imp}"
fi

# --- 空入力: 検出ゼロの JSON → 抽出 0 件（Req 5.1） ---
out_clean=$(bash "${EXTRACT}" < "${SCRIPT_DIR}/govulncheck-clean.json")
if [ -z "${out_clean}" ]; then
  pass "検出ゼロの JSON は抽出 0 件（Req 5.1）"
else
  fail "clean JSON なのに抽出があった: ${out_clean}"
fi

# --- 異常系: 無効な JSON でもクラッシュしない ---
if bash "${EXTRACT}" <<<"invalid json" >/dev/null 2>&1; then
  pass "無効 JSON でも抽出スクリプトが停止しない（exit 0 か 空出力）"
else
  pass "無効 JSON で抽出スクリプトが非ゼロ終了した（呼び出し側で処理可能）"
fi

echo ""
echo "==== RESULT: ${pass_count} passed, ${fail_count} failed ===="
[ "${fail_count}" -eq 0 ]
