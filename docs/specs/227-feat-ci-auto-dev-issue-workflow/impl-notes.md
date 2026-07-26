# Implementation Notes — Issue #227 定期依存脆弱性スキャン + auto-dev Issue 自動起票

## 実装した内容の要約

`.github/workflows/scheduled-audit.yml` を新規追加し、`.github/scripts/scheduled-audit/`
配下の 3 スクリプトから構成する。既存 `ci.yml` は一切変更していない。

### 追加ファイル

| ファイル | 種別 | 役割 |
|---|---|---|
| `.github/workflows/scheduled-audit.yml` | workflow yaml | 定期スキャン workflow（本体） |
| `.github/scripts/scheduled-audit/extract-npm-audit.sh` | shell | `npm audit --json` から high / critical advisory を TSV 抽出 |
| `.github/scripts/scheduled-audit/extract-govulncheck.sh` | shell | `govulncheck -json` から呼び出し到達 vulnerability を TSV 抽出 |
| `.github/scripts/scheduled-audit/render-issue-body.sh` | shell | TSV から feature.yml 準拠の markdown Issue 本文を組み立て |
| `docs/specs/227-feat-ci-auto-dev-issue-workflow/test-fixtures/` | 各種 | 上記スクリプトと workflow yaml の shell-level テスト + fixture JSON |

### 設計判断

1. **抽出・render をスクリプトに分離**: workflow yaml 内 shell を最小化して単体テスト可能に。
   fixture JSON（`npm-audit-*.json` / `govulncheck-*.json`）で shell-level テストを実装
2. **重複ガードは local jq contains() で判定**: GitHub 検索 index の遅延を回避するため、
   `gh issue list --state open --limit 200 --search "scheduled-audit in:body"` で候補を絞り
   込んだうえで、fetch した body に対する `jq contains($marker)` で最終判定する
3. **govulncheck は呼び出し到達 (trace 2 フレーム以上) のみ抽出**: 既存 `ci.yml` の
   govulncheck ジョブが called vulnerability でのみ fail する挙動と揃える（import-only は除外）
4. **1 スキャン = 1 Issue に集約** (PM 決定 2): 個別 advisory 単位に分割せず、npm audit /
   govulncheck それぞれ 1 件の Issue に集約する
5. **cron `15 22 * * *`**: PM 推奨値 22:15 UTC = JST 07:15 を採用（NFR 2.1 / 2.2）
6. **並列 2 ジョブ**: `frontend-audit` と `backend-govulncheck` は `needs` を持たず独立実行
   （Req 3.2 の独立判定を担保）
7. **`concurrency` group を宣言**: schedule と workflow_dispatch の同時発火時の重複起票 race
   を防ぐため（scheduled-audit-refs/heads/<default> でグルーピング）

## テスト方法と実施結果

### shell-level テスト (fixture JSON ベース)

各 fixture ディレクトリ配下の test-*.sh を bash で実行:

```
$ for t in docs/specs/227-feat-ci-auto-dev-issue-workflow/test-fixtures/test-*.sh; do
    bash "$t" 2>&1 | tail -1
  done
==== RESULT: 11 passed, 0 failed ====   # test-extract-govulncheck.sh
==== RESULT: 9 passed, 0 failed ====    # test-extract-npm-audit.sh
==== RESULT: 26 passed, 0 failed ====   # test-render-issue-body.sh
==== RESULT: 15 passed, 0 failed ====   # test-workflow-yaml.sh
```

合計 **61 ケース pass, 0 failed**（境界値・異常系・空入力・dedupe を含む）。

### 静的検証

```
$ actionlint .github/workflows/scheduled-audit.yml
(exit 0)
$ actionlint .github/workflows/*.yml
(exit 0, ci.yml も pass)
$ shellcheck .github/scripts/scheduled-audit/*.sh
(exit 0)
$ shellcheck docs/specs/227-feat-ci-auto-dev-issue-workflow/test-fixtures/*.sh
(exit 0)
$ go vet ./...
(exit 0)
```

### 既存テスト・既存 CI への非影響 (Req 6.1, 6.2)

- `.github/workflows/ci.yml` は本 PR で変更していない (`git diff` で空を確認済み)
- Go / TypeScript のソースコード・依存ファイル (`*.go` / `*.ts` / `*.tsx` /
  `package.json` / `package-lock.json` / `go.mod` / `go.sum`) は変更なし
- 追加は `.github/scripts/scheduled-audit/` / `.github/workflows/scheduled-audit.yml` /
  `docs/specs/227-feat-ci-auto-dev-issue-workflow/` の 3 領域のみ

### 実 CI 上での動作確認について

本 workflow の end-to-end 動作 (実際に schedule で起動 → 実 npm audit 出力を jq で抽出 →
Issue 起票) は、実 CI 環境でのみ観測可能です。以下のいずれかで運用開始後に確認できます:

1. `workflow_dispatch` トリガーで手動実行 (GitHub Actions UI から 1 クリック)
2. 22:15 UTC (JST 07:15) の schedule 自動実行を待つ
3. 検出時の Issue 本文・重複ガード動作・green で完了する動作を目視確認

## 受入基準の達成確認 (Requirements Traceability)

| Requirement | AC | 担保テスト / 実装箇所 |
|---|---|---|
| **Req 1: schedule と手動実行** | 1.1: 別ファイル追加 | `test-workflow-yaml.sh` (ci.yml と独立性判定) |
| | 1.2: schedule (cron) | `test-workflow-yaml.sh` (schedule 節検出) + yaml 目視 |
| | 1.3: workflow_dispatch | `test-workflow-yaml.sh` (workflow_dispatch 節検出) |
| | 1.4: develop HEAD | `test-workflow-yaml.sh` (TARGET_REF=develop + ref: 検出) |
| | 1.5: npm audit ジョブ | `test-workflow-yaml.sh` (npm audit 実行検出) + `test-extract-npm-audit.sh` |
| | 1.6: govulncheck ジョブ | `test-workflow-yaml.sh` (govulncheck 実行検出) + `test-extract-govulncheck.sh` |
| **Req 2: 検出時 Issue 起票** | 2.1: frontend 検出 → Issue | `test-render-issue-body.sh` (npm 版本文が完備) + workflow yaml の `if: steps.audit.outputs.detected != '0'` |
| | 2.2: backend 検出 → Issue | `test-render-issue-body.sh` (go 版本文が完備) + workflow yaml の `if: steps.scan.outputs.detected != '0'` |
| | 2.3: auto-dev ラベル | workflow yaml `gh issue create --label auto-dev` (目視) |
| | 2.4: 別々の Issue | workflow yaml で `frontend-audit` / `backend-govulncheck` の 2 ジョブが独立に `gh issue create` を持つ (目視) |
| | 2.5: 両方検出 → 2 件 | 2 ジョブが `needs` を持たず並列実行 (目視) |
| **Req 3: 重複起票の抑止** | 3.1: 同種 open Issue 存在時は追加起票しない | workflow yaml の `Check duplicate open issue` step + 検出時のみ create する if 条件 |
| | 3.2: 独立判定 | 2 ジョブが独立の `MARKER` env を持ち別々の dup 判定を行う |
| | 3.3: 種別固有マーカー | `test-workflow-yaml.sh` (2 種類のマーカー宣言検証) |
| | 3.4: close 済み Issue は対象外 | workflow yaml の `gh issue list --state open` フィルタ (目視) |
| **Req 4: 起票 Issue 本文** | 4.1: 対象領域判別 | `test-render-issue-body.sh` (npm / go 双方で領域判別記述を検証) |
| | 4.2: advisory ID / package / severity | `test-render-issue-body.sh` + `test-extract-*.sh` (各種項目の抽出・埋め込み検証) |
| | 4.3: ジョブ名 / workflow 実行 URL | `test-render-issue-body.sh` (URL / job-name 埋め込み検証) |
| | 4.4: 優先度 High | `test-render-issue-body.sh` (High 記述検証) |
| | 4.5: feature.yml 見出し構成 | `test-render-issue-body.sh` (5 見出しの存在検証: ### 種別 / ### 背景・課題 / ### 期待する挙動・ゴール / ### 受入基準の候補 / ### 優先度) |
| | 4.6: 重複ガードマーカー本文含有 | `test-render-issue-body.sh` (マーカー本文含有検証) |
| **Req 5: ゼロ検出のノイズ抑止** | 5.1: 検出ゼロ時は Issue 未作成 | `test-extract-*.sh` (clean fixture で 0 行出力) + workflow yaml の `if: detected != '0'` |
| | 5.2: 通知系副作用なし | workflow yaml 内に comment / commit / PR 作成コード無し (目視) |
| | 5.3: ゼロ検出時 green | workflow yaml の各ステップが例外を投げない構造 (目視) + `if: always()` の summary step は必ず成功 |
| **Req 6: 既存 CI 非影響** | 6.1: ci.yml 未変更 | `git diff HEAD~4..HEAD -- .github/workflows/ci.yml` が空 |
| | 6.2: 既存ジョブ成否無影響 | 既存 Go / TS ソース未変更 (`git diff` 確認) |
| | 6.3: pull_request / push 非トリガー | `test-workflow-yaml.sh` (禁止トリガー検出) |
| | 6.4: open PR CI 非影響 | 独立 workflow file / 独立 trigger (目視) |
| **NFR 1: 権限最小化** | 1.1: contents:read + issues:write のみ | `test-workflow-yaml.sh` (permissions ブロック検証) |
| | 1.2: GITHUB_TOKEN のみ | workflow yaml の `env: GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}` (目視。外部 secret 参照なし) |
| **NFR 2: 実行タイミング** | 2.1: 毎時 0 分回避 | `test-workflow-yaml.sh` (cron 分値 != 0 検証。実装値 min=15) |
| | 2.2: JST 6〜9 時 | `test-workflow-yaml.sh` (cron 時値 UTC 21〜23 検証。実装値 hour=22 = JST 07:15) |
| **NFR 3: 後方互換性** | 3.1: 既存ジョブ成否ロジック不変 | Req 6.1 と同じ (ci.yml 未変更) |
| | 3.2: 既存 auto-dev パイプライン不変 | 既存の watcher / issue-to-pr.yml は無変更 (目視) |
| **NFR 4: 可観測性** | 4.1: 検出結果を job summary に残す | workflow yaml の `Publish scan summary` step (`GITHUB_STEP_SUMMARY` 書き込み) |
| | 4.2: 起票 Issue から実行 URL 追跡可能 | `test-render-issue-body.sh` (URL 本文含有検証) |

## 不明点・確認事項

### 確認事項 1: 実 CI 上での初回動作観察

本 workflow が実 CI 上で意図通り動作するかは、以下のいずれかの契機で必ず観察してください:

- **推奨**: PR 承認・merge 直後に `workflow_dispatch` で手動起動 → GitHub Actions UI で
  ログ・Job Summary を確認。検出があれば Issue #NNN が起票されるので、本文が feature.yml
  テンプレの見出し構成通りか目視確認する
- **schedule 実行**: merge から 24h 以内 (直近の 22:15 UTC 到達時) に自動発火する

### 確認事項 2: `--search "scheduled-audit in:body"` の検索遅延

重複判定で候補絞り込みに `gh issue list --search "scheduled-audit in:body"` を使用しています。
GitHub 検索 index の反映遅延（通常数秒〜数分）が原因で、直前サイクルで作成した Issue が
検索結果に含まれない場合、二重起票が発生する **可能性** があります。ただし本 workflow は
schedule で 1 日 1 回・concurrency group で並列起動を抑止しているため、実運用で二重起票が
発生する確率は低いと判断しました。もし発生すれば `gh issue list --limit 200` で候補を
絞らない実装への切り替えで対応可能です。

### 確認事項 3: npm audit の advisory ID 取得ロジック

`extract-npm-audit.sh` は `via[].url` から `GHSA-xxxx-xxxx-xxxx` 形式を正規表現で抽出し、
取れない場合は `source` 数値 ID を fallback として使用します。npm audit の JSON スキーマは
npm バージョンによって変化する可能性があるため、実 CI 上で fallback が多発する場合は
schema 追随の追加対応が必要になる可能性があります（現時点の Node 20 + npm 10 系では OK と
想定）。

### 確認事項 4: govulncheck の severity

`govulncheck` は severity フィールドを OSV に含まないため、本実装では抽出結果を全て
`HIGH` として扱っています。これは既存 `ci.yml` の govulncheck ジョブが called vulnerability
でのみ fail する = 実質 HIGH 相当という運用と揃えたものです。個別 severity を扱う必要が
生じた場合は、OSV DB 側の `database_specific.severity` を参照する拡張が可能です（現時点
不要）。

### 確認事項 5: 依存 (jq / gh / actionlint / shellcheck)

- **jq**: ubuntu-latest 標準搭載。追加インストール不要
- **gh**: ubuntu-latest 標準搭載。GH_TOKEN 経由で認証
- **actionlint / shellcheck**: shell-level テストの実行環境依存（本 workflow 実行時には不要）

追加依存ライブラリは無し。

## 派生 / 次に切り出すべきタスク（本 Issue 外）

- **cleanup**: 起票された Issue が close された後、`docs/specs/227-.../test-fixtures/`
  配下の fixture JSON は将来的に Feedman 実運用データと乖離する可能性があるため、
  半年〜1 年後にリフレッシュを検討
- **拡張**: cron 頻度を Feedman 運用実態 (advisory 頻度) に応じて調整する可能性 (現状 1 日 1 回)
- **拡張**: 検出時に Slack / メール通知を追加する要望が出た場合は本 Issue のスコープ外として
  別 Issue で扱う（Out of Scope に明記）

STATUS: complete
