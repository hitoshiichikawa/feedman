#!/usr/bin/env bash
# extract-npm-audit.sh: `npm audit --json` の出力から high / critical の advisory を抽出する
#
# 入力: stdin に `npm audit --json` の出力 JSON
# 出力: stdout に 1 行 1 advisory の TSV
#   カラム: severity \t package \t advisory_id \t title
#
#   - severity: HIGH / CRITICAL（大文字。high 以上のみ抽出）
#   - package: 影響を受けるパッケージ名
#   - advisory_id: GHSA-xxxx-xxxx-xxxx 形式（url から抽出）、取れない場合は source ID 数値
#   - title: advisory のタイトル
#
# 重複除去: severity + package + advisory_id のキーで uniq
#
# npm audit --json は non-zero exit code を返す場合があるが、本スクリプトの呼び出し側
# （workflow yaml）が捕捉するため、本スクリプト内では stdin の JSON パースのみを担当する。
#
# 参照: https://docs.npmjs.com/cli/v10/commands/npm-audit#json-output
set -euo pipefail

# 無効な JSON でも安全側に倒す: jq のエラーは stderr に流し、stdout は空のまま exit 0
# 一時ファイルに書き出し、jq が成功したら sort -u で uniq / 失敗したら空出力で exit 0
tmp_out=$(mktemp)
trap 'rm -f "${tmp_out}"' EXIT

if jq -r '
  # vulnerabilities は package name をキーとするオブジェクト
  .vulnerabilities // {}
  | to_entries[]
  | .value as $vuln
  # high 以上のみ抽出（Req 1.5 / 2.1 の境界）
  | select($vuln.severity == "high" or $vuln.severity == "critical")
  # via[] は string（他 vuln への参照）と object（実 advisory）が混在するため object のみ
  | ($vuln.via // [] | map(select(type == "object")))[]
  | . as $adv
  | [
      # severity は advisory 側優先、なければ vuln 側から
      (($adv.severity // $vuln.severity // "unknown") | ascii_upcase),
      # package 名は advisory 側優先
      ($adv.name // $vuln.name // "unknown"),
      # advisory_id は URL から GHSA-xxx 形式を抽出。取れなければ source 数値 ID を使う
      (
        ($adv.url // "") as $url
        | if ($url | test("GHSA-[a-z0-9-]+"; "i")) then
            ($url | capture("(?<id>GHSA-[a-z0-9-]+)"; "i").id)
          else
            ($adv.source // "unknown" | tostring)
          end
      ),
      # title は advisory 側に基本存在する
      ($adv.title // "(no title)")
    ]
  | @tsv
' > "${tmp_out}" 2>/dev/null; then
  sort -u "${tmp_out}"
fi
# jq が失敗した場合は何も出力せず exit 0（呼び出し側は detected=0 として扱う）
exit 0
