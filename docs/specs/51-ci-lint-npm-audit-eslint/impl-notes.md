# 実装ノート（Issue #51: CI フロントエンド lint / npm audit 追加）

## 概要

CI（`.github/workflows/ci.yml`）にフロントエンド（`web/`）の lint（eslint）と依存脆弱性
スキャン（`npm audit --audit-level=high`）を追加した。既存 `govulncheck` の独立ジョブ方式に
合わせて、`frontend-lint` / `frontend-audit` の 2 つの独立ジョブを追加している。

本 Issue は requirements.md のみを入力とする design-less impl（design.md / tasks.md なし）で
あり、tasks.md が存在しないため stage-a-verify gate の対象外。番号順タスク消化ではなく、
requirements.md の AC を起点に実装した。

## 実装方針と job 構成の判断理由

### 独立ジョブ化 vs frontend ジョブへの step 追加

NFR 2.1（lint / audit の成否を PR チェック一覧で他ジョブと区別可能な独立ステータスとして
表示）および Req 3.3（lint / audit の失敗を `npm test` の成否とは独立に判定）を確実に満たす
ため、`frontend` ジョブへの step 追加ではなく **独立ジョブ化** を採用した。

- **採用理由**:
  - 既存 `govulncheck` が独立ジョブ（脆弱性検出でジョブを失敗させるブロッキング方式）で
    あり、フロントエンド側もこの既存方針と一貫させられる（NFR 1.1）。
  - 独立ジョブにすると GitHub PR チェック一覧に `Frontend Lint (eslint)` /
    `Frontend Dependency Vulnerability Scan (npm audit)` という独立ステータスが並ぶため、
    どのチェックが失敗したかが一目で分かる（NFR 2.1）。
  - 同一ジョブ内の step 追加だと、`npm test` の前段 step が失敗するとジョブ全体が `Frontend
    Tests` の 1 ステータスにまとまってしまい、lint / audit / test のどれが原因かが
    チェック一覧から判別しづらく、また step が直列実行されるため早期 step の失敗で
    後続 step が走らない（Req 3.3 の「独立判定」を満たしにくい）。

- **トレードオフ（記録）**:
  - 独立ジョブ化により、`checkout` / `setup-node` / `npm ci` の setup が `frontend` /
    `frontend-lint` / `frontend-audit` の 3 ジョブで重複実行される。`npm ci` が 3 回走る
    分の実行時間・CI リソースの増加がデメリット。
  - ただし各ジョブは並列実行されるため wall-clock 時間の増加は限定的であり、`setup-node`
    の npm キャッシュ（`cache: npm` / `cache-dependency-path: web/package-lock.json`）が
    効くため依存ダウンロードコストは緩和される。可観測性（NFR 2.1）と独立判定（Req 3.3）の
    明確さを優先し、setup 重複のコストを許容する判断とした。

### lint の実行

- `npm run lint`（= `web/package.json` の `"lint": "eslint"`）をそのまま実行する。eslint は
  lint 違反検出時に非ゼロ終了するため、ジョブが失敗し PR チェックが fail になる（Req 1.2）。
  違反なしなら成功（Req 1.3）。eslint は違反内容を stdout に出力するためジョブログに残る
  （Req 1.4）。
- Out of Scope に従い eslint ルールセット（`web/eslint.config.mjs` / `eslint-config-next`）は
  一切変更していない。

### npm audit の実行

- `npm audit --audit-level=high` を実行する（Req 2.1 / 2.2）。`npm audit` は閾値（`--audit-level`）
  以上の脆弱性が見つかった場合にのみ非ゼロ終了する。
  - high / critical 検出時 → 非ゼロ終了 → ジョブ失敗（Req 2.3、NFR 1.1）。
  - moderate / low / info のみ → ゼロ終了 → ジョブ成功（Req 2.4）。
  - high 以上の検出なし → ゼロ終了 → ジョブ成功（Req 2.5）。
- `npm audit` は `package-lock.json` を参照して依存ツリーを評価する。確実性のため audit ジョブ
  でも先に `npm ci`（lockfile に基づくインストール）を実行してから `npm audit` を走らせる
  構成にした。検出された脆弱性一覧は stdout に出力されジョブログに残る（Req 2.6）。

### 既存ジョブの後方互換

- `backend` / `go-vet` / `govulncheck` / `frontend`（`npm test`）の各ジョブ定義は一切変更して
  いない（Req 3.1 / 3.2）。トリガー（`push` / `pull_request` to `main`/`develop`）も無変更で
  あり、Out of Scope の「トリガー設定踏襲」を満たす。

## 受入基準とテストの対応

本 Issue の成果物は GitHub Actions ワークフロー定義（YAML）であり、アプリケーションコードの
単体テストを書く対象ではない。AC の担保は (a) ワークフロー定義の静的検証と (b) ローカルでの
コマンド挙動確認の 2 段で行った。各 AC と検証手段の対応は以下のとおり。

| AC | 内容 | 担保した検証 |
|---|---|---|
| 1.1 | PR 時に `web/` で `npm run lint` を実行 | `frontend-lint` ジョブが `working-directory: web` / `run: npm run lint` を持ち、`pull_request: [main, develop]` トリガー配下にあることを YAML parse で確認 |
| 1.2 | lint 違反でジョブ失敗 | eslint の非ゼロ終了で GitHub Actions の step が fail する仕様に依拠。`npm run lint` を直接実行（exit code 抑制なし） |
| 1.3 | lint 違反なしでジョブ成功 | 同上（eslint ゼロ終了でジョブ成功） |
| 1.4 | lint 結果をログ出力 | `npm run lint` の stdout がジョブログに出力される（追加抑制なし） |
| 2.1 | PR 時に `web/` で `npm audit` を実行 | `frontend-audit` ジョブが `working-directory: web` / `run: npm audit ...` を持つことを YAML parse で確認 |
| 2.2 | `--audit-level=high` で実行 | `run: npm audit --audit-level=high` を確認 |
| 2.3 | high 以上でジョブ失敗 | `npm audit --audit-level=high` が high/critical 検出時に非ゼロ終了する仕様に依拠 |
| 2.4 | high 未満のみなら成功 | 同上（moderate 以下では `--audit-level=high` 指定時にゼロ終了） |
| 2.5 | high 以上の検出なしで成功 | 同上（ゼロ終了） |
| 2.6 | audit 結果をログ出力 | `npm audit` の stdout がジョブログに出力される（追加抑制なし） |
| 3.1 | 既存 `npm test` の成否維持 | `frontend` ジョブを無変更とし diff が新規ジョブ追加のみであることを確認 |
| 3.2 | 既存 backend / go-vet / govulncheck 維持 | 同上（既存 4 ジョブ定義に変更なし） |
| 3.3 | lint / audit を `npm test` と独立判定 | lint / audit を `frontend` とは別の独立ジョブにし、ジョブ間に `needs` 依存を張らず並列・独立に成否判定されることを確認 |
| NFR 1.1 | govulncheck と同じブロッキング方針 | 独立ジョブ + 非ゼロ終了でジョブ失敗、という govulncheck と同型の構成を採用 |
| NFR 1.2 | moderate 以下でブロックしない | `--audit-level=high` の足切りに依拠（2.4 と同根拠） |
| NFR 2.1 | PR チェック一覧で独立ステータス表示 | `frontend-lint` / `frontend-audit` を独立ジョブにし、各々固有の `name` を付与（`Frontend Lint (eslint)` / `Frontend Dependency Vulnerability Scan (npm audit)`）したことを確認 |

## 検証結果

### 1. ワークフロー YAML の構文・構成検証（実施・成功）

`python3` の `yaml.safe_load` で `.github/workflows/ci.yml` をパースし、以下を確認した
（`actionlint` はこの環境に未インストールのため Python yaml で代替検証）:

- YAML として正しくパースできる（構文崩れなし）。
- jobs キーが `backend` / `go-vet` / `govulncheck` / `frontend` / `frontend-lint` /
  `frontend-audit` の 6 件であること（既存 4 件は無変更、新規 2 件追加）。
- トリガー `on` が `push: [main, develop]` / `pull_request: [main, develop]` のまま変化が
  ないこと。
- `frontend-lint`: `working-directory: web`、steps が
  checkout → setup-node → `npm ci` → `npm run lint`。
- `frontend-audit`: `working-directory: web`、steps が
  checkout → setup-node → `npm ci` → `npm audit --audit-level=high`。

### 2. `web/` でのローカル lint / audit 実行（未実施 — 環境制約）

本実行環境（worktree のサンドボックス）に **node / npm が存在しない**（`command -v node` /
`command -v npm` がいずれも未検出、`node_modules` も未生成）。このため
`npm ci` / `npm run lint` / `npm audit --audit-level=high` のローカル実行による
「現状の web/ で lint がクリーンに通るか」「audit が high 以上を検出しないか」の事前確認は
**実施できなかった**。下記「確認事項」に明記する。

### 3. 既存 `npm test` の確認（未実施 — 同上の環境制約）

`npm test` も node/npm 不在のため実行不可。ただし `frontend` ジョブ定義は無変更であり、
CI 上の `npm test` の挙動は本変更の影響を受けない。

## 確認事項（レビュワー / 人間の判断ポイント）

1. **ローカルでの lint / audit 未検証（環境制約）**: 本実装環境に node / npm が無いため、
   現状の `web/` で `npm run lint` がクリーンに通るか、`npm audit --audit-level=high` が
   high 以上の脆弱性を検出しないかを **ローカルで事前確認できていない**。新ジョブが追加
   された瞬間に、もし既存コードベースに lint 違反または high 以上の脆弱性が存在すれば
   `frontend-lint` / `frontend-audit` ジョブが即座に fail する。CI 上での初回実行結果
   （PR チェック）で green になることを確認いただきたい。これはテストを弱める / 握り潰す
   ことを避け、実態（ブロッキング方式）をそのまま反映した結果である。
2. **setup 重複のトレードオフ**: 独立ジョブ化により `npm ci` が `frontend` /
   `frontend-lint` / `frontend-audit` の 3 ジョブで重複実行される（上記「判断理由」参照）。
   可観測性・独立判定を優先した判断だが、CI 時間 / リソースをよりタイトに抑えたい場合は
   `frontend` ジョブ内の step 集約も選択肢となる。その場合 NFR 2.1（独立ステータス）/
   Req 3.3（独立判定）の満たし方を別途検討する必要がある。

## 派生タスク候補

- `actionlint` を CI に組み込む（ワークフロー定義自体の静的検証）案。本 Issue のスコープ外
  だが、今後ワークフローを増やす際の品質担保として別 Issue 化を検討可能。

STATUS: complete
