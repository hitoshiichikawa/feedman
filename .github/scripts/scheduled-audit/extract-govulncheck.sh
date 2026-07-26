#!/usr/bin/env bash
# extract-govulncheck.sh: `govulncheck -json ./...` の出力から呼び出し到達 vulnerability を抽出する
#
# 入力: stdin に `govulncheck -json` の NDJSON 形式出力
# 出力: stdout に 1 行 1 advisory の TSV
#   カラム: severity \t package \t advisory_id \t summary
#
#   - severity: HIGH（govulncheck は severity を返さないが、呼び出し到達 vuln は
#     既存 CI で fail 扱い = 実質 HIGH 相当のため統一）
#   - package: module 名（trace[0].module）
#   - advisory_id: OSV ID（GO-YYYY-NNNN 形式）
#   - summary: OSV 側の summary
#
# 抽出条件:
#   - `finding` エントリのうち、`trace` フレーム数が 2 以上（呼び出し到達性あり）のもの
#     * trace フレーム 1 のみ = import はしているが未呼び出し（既存 CI 挙動と揃えて除外）
#   - 同一 OSV ID の複数 finding は 1 行に集約
#
# 参照: https://pkg.go.dev/golang.org/x/vuln/scan#Handler
#       govulncheck の NDJSON stream 仕様
set -euo pipefail

# 入力を一時ファイルに保存（2 パス: osv メタデータ抽出 + finding 抽出）
input_file=$(mktemp)
tmp_out=$(mktemp)
trap 'rm -f "${input_file}" "${tmp_out}"' EXIT

cat > "${input_file}"

# 無効な JSON でも安全側に倒す: jq のエラーは stderr / 空出力 + exit 0
# jq は NDJSON stream を自然に処理する（複数 JSON object を空白区切りで受け取る）
if jq -sr '
  # -s: 全 JSON object を配列として slurp
  # osv-id → summary のマップを構築
  (reduce .[] as $obj ({};
    if $obj.osv != null and $obj.osv.id != null then
      . + { ($obj.osv.id): ($obj.osv.summary // "(no summary)") }
    else . end
  )) as $osv_map |
  # trace 2 フレーム以上（呼び出し到達）の finding のみ抽出
  [.[] | select(.finding != null and (.finding.trace | type) == "array" and (.finding.trace | length) >= 2) | .finding] as $callable |
  # OSV ID で group_by し先頭のみ残す（dedupe）
  ($callable | group_by(.osv) | map(.[0]))[] |
  [
    "HIGH",
    (.trace[0].module // "unknown"),
    (.osv // "unknown"),
    ($osv_map[.osv] // "(no summary)")
  ] | @tsv
' "${input_file}" > "${tmp_out}" 2>/dev/null; then
  sort -u "${tmp_out}"
fi
# jq 失敗時は何も出力せず exit 0
exit 0
