# Requirements Document

## Introduction

Feedman の CI（`.github/workflows/ci.yml`）に含まれる依存脆弱性スキャン（`web/` の
`npm audit --audit-level=high` / Go の `govulncheck ./...`）は push / pull_request でのみ実行
される。しかし脆弱性アドバイザリは live DB を参照するため、コード無変更でも新規公開と同時に
green → fail へ遷移し、無関係な open PR 全てが巻き添えで赤くなる事象が過去 1 ヶ月で 3 回発生
している（2026-06-23 / 2026-07-24 / 2026-07-26。それぞれ PR #210、#219 → PR #221、#225 →
PR #226 で解消）。現状は「人間が PR の赤に気づく → 手動で auto-dev Issue を起票 → 修正 →
巻き添え PR を update-branch」という後手対応となっており、その間 PR のマージ判断が停止する。

本要件は、依存脆弱性スキャンを PR / push とは独立に **schedule（cron）で毎日 1 回自動実行**
し、検出があった場合に **auto-dev ラベル付き Issue を自動起票**することで、既存 watcher
パイプライン（PM → Developer → PjM）に修正を任せる先手対応を可能にする。既存の PR / push
トリガー CI のゲート挙動、既存ジョブの成否判定、および `.github/workflows/ci.yml` 自体の
内容には手を触れない。

## Requirements

### Requirement 1: スケジュール実行と手動実行

**Objective:** As a Feedman のメンテナ, I want 依存脆弱性スキャンが毎日 1 回自動実行され必要に応じて手動でも起動できること, so that PR が赤くなる前後で新規アドバイザリを能動的に検知できる

#### Acceptance Criteria

1. The 定期スキャン workflow shall `.github/workflows/` 配下の既存 `ci.yml` と別ファイルとして追加される
2. The 定期スキャン workflow shall schedule（cron）トリガーにより毎日 1 回自動実行される
3. The 定期スキャン workflow shall `workflow_dispatch` トリガーにより GitHub UI からも手動実行できる
4. When 定期スキャン workflow が起動したとき, the 定期スキャン workflow shall リポジトリの develop ブランチの HEAD に対してスキャンを実行する
5. The 定期スキャン workflow shall フロントエンド依存の脆弱性スキャン（`npm audit --audit-level=high` 相当）を実行するジョブを含む
6. The 定期スキャン workflow shall Go 依存の脆弱性スキャン（`govulncheck` 相当）を実行するジョブを含む

### Requirement 2: 検出時の Issue 自動起票

**Objective:** As a Feedman のメンテナ, I want スキャンで脆弱性が検出されたとき自動的に auto-dev Issue が起票されること, so that 既存 idd-claude パイプラインが人間の介在なしに修正を開始できる

#### Acceptance Criteria

1. If フロントエンド依存スキャンが high 以上の脆弱性を 1 件以上検出した場合, the 定期スキャン workflow shall フロントエンド担当領域向けの新規 Issue を作成する
2. If Go 依存スキャンが 1 件以上の脆弱性を検出した場合, the 定期スキャン workflow shall バックエンド担当領域向けの新規 Issue を作成する
3. When 定期スキャン workflow が Issue を作成したとき, the 作成 Issue shall `auto-dev` ラベルを付与された状態となる
4. The 作成 Issue shall フロントエンド系（`npm audit` 起因）とバックエンド系（`govulncheck` 起因）で **別々の Issue** として起票される（担当領域が異なるため）
5. When フロントエンド依存スキャンとバックエンド依存スキャンの両方で同時に検出があったとき, the 定期スキャン workflow shall 2 件（フロントエンド用 1 件・バックエンド用 1 件）の Issue を作成する

### Requirement 3: 重複起票の抑止

**Objective:** As a Feedman のメンテナ, I want 同種の未解決 Issue が既に open のとき同じ Issue が繰り返し起票されないこと, so that 同一問題の Issue が乱立せず既存 auto-dev パイプラインの処理を妨げない

#### Acceptance Criteria

1. While 同一スキャン種別（フロントエンド用 or バックエンド用）の未解決 Issue が既に存在するあいだ, the 定期スキャン workflow shall 当該スキャン種別の新規 Issue を追加で作成しない
2. When スキャン種別ごとに独立して重複判定が行われるとき, the 定期スキャン workflow shall フロントエンド用 Issue の open 状態がバックエンド用 Issue の起票可否に影響しないよう独立に判定する
3. The 定期スキャン workflow shall 重複判定に用いるマーカー（Issue 本文に埋め込む機械可読な識別子）をスキャン種別ごとに固有な値として持つ
4. When 過去に起票された Issue が close された後の次回スキャン実行で再度検出があったとき, the 定期スキャン workflow shall 新規 Issue を起票する（close 済み Issue は重複ガードの判定対象としない）

### Requirement 4: 起票 Issue の本文要件

**Objective:** As a Feedman のメンテナと PM エージェント, I want 起票された Issue から検出内容と対象領域が判別できること, so that Triage および Developer が推測なく修正着手できる

#### Acceptance Criteria

1. The 作成 Issue shall タイトルまたは本文からスキャン対象（フロントエンド `web` / バックエンド Go）が判別できる記述を含む
2. The 作成 Issue shall 検出された脆弱性のサマリーとして少なくとも「アドバイザリ ID（例: GHSA-xxxx-xxxx-xxxx / Go の VULN ID）」「対象パッケージ名」「severity」を本文に含む
3. The 作成 Issue shall 検出元ジョブ名または対象 workflow 実行 URL を本文に含み、CI ログへ辿れる導線を提供する
4. The 作成 Issue shall 優先度が High 相当であることが本文の記述から判別できる（既存 feature.yml テンプレの「優先度」相当のセクションで High を明示する）
5. The 作成 Issue shall 既存 auto-dev Issue テンプレ（`.github/ISSUE_TEMPLATE/feature.yml`）の見出し構成（`### 種別` / `### 背景・課題` / `### 期待する挙動・ゴール` / `### 受入基準の候補` / `### 優先度` 等）に沿った形式で記述され、PM エージェントが同一パーサで解釈できる
6. The 作成 Issue shall Req 3 の重複ガード判定に用いる機械可読なマーカー（HTML コメント形式）を本文に含む

### Requirement 5: 検出ゼロ時のノイズ抑止

**Objective:** As a Feedman のメンテナ, I want 脆弱性が検出されなかったスキャンでは何も起票・通知されないこと, so that green 状態でメンテナの受信箱がノイズで埋まらない

#### Acceptance Criteria

1. When フロントエンド依存スキャンとバックエンド依存スキャンのいずれも脆弱性を検出せずに終了したとき, the 定期スキャン workflow shall 新規 Issue を作成しない
2. When 定期スキャン workflow が脆弱性を検出せず終了したとき, the 定期スキャン workflow shall 通知目的の commit / comment / PR / GitHub 通知を追加で生成しない
3. When 定期スキャン workflow が脆弱性検出ゼロで終了したとき, the 定期スキャン workflow 全体 shall 成功（green）ステータスで完了する

### Requirement 6: 既存 CI・既存挙動への非影響

**Objective:** As a Feedman のメンテナ, I want 本 workflow 追加が既存の PR / push トリガー CI のゲート挙動を変更しないこと, so that 現状の PR マージ判定・既存ジョブの成否が本変更で影響を受けない

#### Acceptance Criteria

1. The 本対応 shall 既存 `.github/workflows/ci.yml` の内容（ジョブ・トリガー・ゲート判定・permissions 等）を変更しない
2. The 定期スキャン workflow shall 既存 CI ジョブ（`backend` / `go-vet` / `govulncheck` / `frontend` / `frontend-lint` / `frontend-audit` / `trivy`）の成否判定に影響を与えない
3. The 定期スキャン workflow shall pull_request イベント・push イベントを自身のトリガーとしない（schedule と workflow_dispatch のみ）
4. When 定期スキャン workflow が Issue を起票したとき, the 本対応 shall 既存 open PR の CI ステータスに直接影響を与えない（間接的に生成された修正 PR の merge により update-branch が必要になる場合を除く）

## Non-Functional Requirements

### NFR 1: 権限最小化

1. The 定期スキャン workflow shall `GITHUB_TOKEN` の permissions を最小権限として明示宣言する（Issue 起票のため書き込みが必要な `issues: write` およびソース参照に必要な `contents: read` に限定し、それ以外の権限は宣言しない）
2. The 定期スキャン workflow shall API キー・OAuth シークレット等の外部認証情報を必要としない範囲で実装される（GitHub Actions 標準の `GITHUB_TOKEN` のみを利用する）

### NFR 2: 実行タイミング

1. The 定期スキャン workflow shall cron 実行時刻を「毎時 0 分」を避けた分単位（GitHub Actions 混雑時間帯を回避する分）に設定する
2. The 定期スキャン workflow shall 1 日 1 回の頻度で実行され、実行時刻は日本標準時（JST, UTC+9）で午前 6 時〜午前 9 時のあいだにメンテナが結果を確認できる時刻とする（PM 判断: 22:15 UTC = JST 07:15 を推奨とし、実装者が同要件を満たす範囲で微調整することを許容する）

### NFR 3: 後方互換性

1. The 本対応 shall 本 workflow 導入前から存在する全 CI ジョブの成否判定ロジックを変更しない
2. The 定期スキャン workflow shall 既存 auto-dev / idd-claude パイプライン（PM / Architect / Developer / Reviewer / PjM）の既存挙動を変更しない（起票された Issue は既存 watcher が既存ラベル解釈で処理する）

### NFR 4: 検出結果の可観測性

1. When 定期スキャン workflow が実行完了したとき, the 定期スキャン workflow shall スキャン結果（検出件数・severity サマリー）を GitHub Actions のジョブログまたはジョブサマリーから確認できる形で残す
2. When Issue が起票されたとき, the 作成 Issue shall 対応する GitHub Actions 実行 URL からログを追跡できる導線を持つ

## PM 判断事項（Issue「仮案・判断を委ねたい点」への確定回答）

Issue 本文「仮案・判断を委ねたい点」で人間判断が委ねられた 3 点について、PM として以下の
デフォルトを確定した。設計・実装はこの確定値を前提としてよい。

### 決定 1: 実行時刻（NFR 2.2 に対応）

- **確定値**: cron 実行時刻は **22:15 UTC（= JST 07:15）** を推奨とする。実装時に schedule
  の cron 表現に別の分値を採用する場合も「毎時 0 分を避ける」「JST 午前 6〜9 時のあいだ」の
  2 条件を同時に満たす範囲で許容する
- **根拠**: Issue 本文の制約「毎時 0 分（GitHub Actions 混雑時間帯）を避ける」および
  「JST 朝に人間が結果を見られる時間帯」を両立する。JST 07:15 はメンテナが業務開始前に
  受信箱で確認しやすい時間帯であり、かつ 07:00 / 08:00 ちょうどの混雑帯を避けられる

### 決定 2: govulncheck 側の起票粒度（Req 2 に対応）

- **確定値**: govulncheck 側は **1 回のスキャン実行で検出された全アドバイザリを 1 Issue に
  集約**する（advisory ごとに個別 Issue を分割しない）
- **根拠**: フロントエンド側（npm audit）と粒度を揃えることで重複ガード（Req 3）のマーカー
  設計が対称になり実装・保守が単純化する。過去 3 回の同型対応（#210 / #219 / #225）はいずれも
  「同時期に判明したアドバイザリ群を 1 PR で解消」というパターンで運用されており、Issue を
  1 件に集約しても既存 auto-dev パイプラインの処理粒度と整合する。個別 advisory 単位に
  分割したい要求が将来生じた場合は本 Issue のスコープ外として別途検討する

### 決定 3: 重複ガード方式（Req 3 に対応）

- **確定値**: 重複ガードは **Issue 本文中の HTML コメント形式マーカー**（フロントエンド用
  `<!-- scheduled-audit: npm -->` / バックエンド用 `<!-- scheduled-audit: govulncheck -->`）
  を open Issue から検索することで判定する
- **根拠**: Issue 本文の仮案どおり、HTML コメントマーカーは Issue 表示時に不可視で運用者の
  可読性を損なわず、GitHub 検索 API（`gh issue list --search` 相当）で機械的に判定できる。
  外部データストア（DB / ファイル）を持たずに GitHub 上のみで完結し、実装が単純になる。
  マーカー名の canonical 表現・検索キー・埋め込み位置の詳細は design.md の領分とする

## Out of Scope

- 既存 `.github/workflows/ci.yml`（PR / push トリガー CI）のゲート挙動変更（blocking のまま維持）
- Dependabot / Renovate の導入（依存更新の自動 PR 化）
- Trivy（コンテナイメージスキャン）の schedule 化（本 Issue はコード依存の脆弱性のみ対象）
- 検出脆弱性を解消する修正 PR の自動作成（起票までを本 Issue のスコープとし、修正は既存
  auto-dev パイプラインに委ねる）
- 起票 Issue に対する Slack / メール等の外部通知連携
- 個別 advisory 単位での Issue 分割（決定 2 の通り 1 スキャン = 1 Issue に集約する）
- 過去に close された同種 Issue の再オープン（Req 3.4 の通り新規 Issue を起票する）
- 実行対象ブランチを develop 以外に拡張すること（本 Issue は develop HEAD に限定）
- 検出時挙動を「fail-fast で workflow を失敗させる」形式にすること（本 Issue は「検出時に
  Issue 起票 = 成功終了」を想定し、workflow 自体は起票が成功すれば green で終える）

## Open Questions

- なし（Issue 本文で明示された「仮案・判断を委ねたい点」3 点は上記「PM 判断事項」で確定
  値と根拠を明記した。人間による追加コメントでの明示回答がまだ無いが、Issue 本文の制約と
  過去 3 回の同型対応の運用実績から論理的に導ける範囲であり、PM 責務として確定した）

## 関連

- Related: #225 #219 #210
- Related: #52 #50
