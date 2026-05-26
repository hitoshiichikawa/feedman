# 実装ノート: Issue #52 CI にコンテナイメージ脆弱性スキャン（Trivy）を追加

## 実装サマリー

`.github/workflows/ci.yml` に新規 job `trivy`（job 名表示: `Container Image Vulnerability
Scan (Trivy)`）を追加した。既存ジョブ（backend / go-vet / govulncheck / frontend /
frontend-lint / frontend-audit）には一切手を加えず、独立した新規 job として並列実行される。

job のステップ構成:

1. `actions/checkout@v4` でソースを取得（既存ジョブのピン留め流儀に合わせて `@v4`）
2. `docker build` でルート `Dockerfile`（api/worker 用）を `feedman-api:scan` としてビルド
3. `docker build` で `web/Dockerfile`（web 用）を `feedman-web:scan` としてビルド
4. `aquasecurity/trivy-action@0.28.0` で `feedman-api:scan` を image スキャン（CRITICAL/HIGH）
5. 同アクションで `feedman-web:scan` を image スキャン（CRITICAL/HIGH）
6. スキャン結果（table 形式テキスト）を GitHub Step Summary に出力

### 警告のみ（CI を fail させない）にした方法

- trivy-action の `exit-code: '0'` を指定し、CRITICAL/HIGH を検出しても Trivy が非ゼロ
  exit code を返さないようにした（Req 3.1 / 3.3）。
- 念のための二重防御として、job レベルのコメントに記載のとおり `exit-code: '0'` を主たる
  手段とし、検出時もステップが成功扱いになるようにした。`output:` でファイル出力した結果を
  後続ステップ（`if: always()`）が job summary に出すため、スキャン結果は必ず可視化される。

## スキャン方式の選択理由（config / image / fs）

**採用: image スキャン（イメージビルド後にビルド済みイメージをスキャン）**

要件の主目的（Req 1.3）は「ベースイメージ・OS パッケージ由来の CRITICAL/HIGH CVE 検出」で
ある。本リポジトリのスキャン対象は以下:

- ルート `Dockerfile`: 実行ステージが `gcr.io/distroless/static-debian12:nonroot`
  （Debian 12 ベースの OS パッケージを含む）
- `web/Dockerfile`: 実行ステージが `node:20-alpine`（Alpine OS パッケージ + Node ランタイム）

3 方式のトレードオフ:

| 方式 | OS パッケージ CVE 検出 | 実行時間 | 判断 |
|---|---|---|---|
| config スキャン（Dockerfile） | **不可**（Dockerfile の記述ミスのみ検出） | 速い | Req 1.3 を満たせない |
| fs スキャン（リポジトリ FS） | 不可（ビルド前のソース/lockfile のみ。ベースイメージ層を見ない） | 速い | Req 1.3 を満たせない |
| image スキャン（ビルド済みイメージ） | **可**（ベースイメージ層・OS パッケージを解析） | ビルド分の時間増 | **採用** |

config / fs スキャンでは distroless-debian12 や node:20-alpine の OS パッケージ層に含まれる
CVE を検出できないため、Req 1.3 を満たすには image スキャンが必須と判断した。実行時間は
`docker build`（マルチステージビルド）分が増えるが、両イメージとも軽量（distroless /
alpine ベース）であり、NFR 1.1 の目安（1 ジョブ 5 分以内）に収まる見込み。万一実行時間が
許容範囲を超える場合は、NFR 1.2 のとおりスキャンモードの軽量化や実行頻度の調整
（schedule 化等）で抑制できる構成余地を残している。

## アクションのバージョンピン留め

- `aquasecurity/trivy-action@0.28.0` を採用。既存 ci.yml の他アクション
  （`actions/checkout@v4` / `actions/setup-go@v5` / `actions/setup-node@v4`）はメジャー版
  タグでピン留めしているが、trivy-action は `vMAJOR` 形式のタグを提供していないため、
  公式が推奨する semver リリースタグ（`0.28.0`）でピン留めした。これは「リリースタグで
  バージョンを固定する」という既存流儀と整合する。

## 各 AC と実装の対応（トレーサビリティ）

| AC | 実装箇所 / 担保内容 |
|---|---|
| Req 1.1（ルート Dockerfile スキャン） | `feedman-api:scan` を image スキャンするステップ |
| Req 1.2（web/Dockerfile スキャン） | `feedman-web:scan` を image スキャンするステップ |
| Req 1.3（CRITICAL/HIGH OS CVE を検出結果に含める） | image スキャン採用 + `severity: CRITICAL,HIGH`。OS パッケージ層を解析できる方式を選択 |
| Req 1.4（重大脆弱性なしなら正常終了） | `exit-code: '0'` により常に正常終了。脆弱性なしの場合も当然 job 成功 |
| Req 2.1（PR トリガーで起動） | 既存 `on: pull_request: branches: [main, develop]` をそのまま利用。trivy job も同トリガーで起動 |
| Req 2.2（push でも整合起動） | 既存 `on: push: branches: [main, develop]` をそのまま利用。トリガー定義は無変更 |
| Req 2.3（CI チェック一覧に表示） | `jobs.trivy` として独立 job 定義。GitHub の checks に名前 `Container Image Vulnerability Scan (Trivy)` で表示される |
| Req 3.1（検出でも fail させない） | `exit-code: '0'` で Trivy が非ゼロ exit しない |
| Req 3.2（結果を CI 上で参照可能に残す） | `output:` でファイル出力し、`Publish scan results to job summary` ステップで Step Summary に table を掲載。加えて CI ログにも table 出力が残る |
| Req 3.3（マージをブロックしない） | job が常に成功扱いになるため required check でもブロックしない |
| Req 4.1（既存各 job を従来どおり実行） | 既存 job 定義を一切変更せず、trivy を新規追加のみ（diff で確認済み） |
| Req 4.2（検出が既存 job 成否を変えない） | trivy は独立 job で `needs` 無し。他 job の成否に影響しない |
| Req 4.3（依存脆弱性スキャンとスコープ非重複） | trivy は Dockerfile/コンテナイメージ（ベースイメージ・OS パッケージ）が対象。govulncheck（Go 依存）/ npm audit（npm 依存）とスコープが重複しない |
| NFR 1.1（実行時間） | 軽量ベースイメージ（distroless / alpine）のため目安 5 分以内に収まる見込み |
| NFR 1.2（軽量化・頻度調整の選択肢） | image スキャン + severity 絞り込みで抑制可能。将来 schedule 化や fs/config 併用の余地あり |
| NFR 2.1（既存 job 成否判定ロジック不変） | 既存 job は無変更（diff で確認済み） |
| NFR 2.2（独立実行） | `needs` 無しで独立 job として実行 |
| NFR 3.1（重大度・件数を CI 上で確認可能） | `severity: CRITICAL,HIGH` の table 出力を CI ログ + Step Summary の双方に残す |

## 検証手順と結果

1. **YAML 構文検証**

   ```sh
   python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('YAML OK')"
   ```

   結果: `YAML OK`（構文妥当）

2. **actionlint によるワークフロー lint**

   `which actionlint` では未インストールだったため、
   `go install github.com/rhysd/actionlint/cmd/actionlint@latest`（v1.7.12 / Go 1.25.10）で
   インストールし、以下を実行:

   ```sh
   "$(go env GOPATH)/bin/actionlint" .github/workflows/ci.yml
   ```

   結果: exit code 0（指摘なし）

3. **既存ジョブ非影響の diff 確認**

   ```sh
   git diff HEAD -- .github/workflows/ci.yml
   ```

   結果: 追加分は `trivy` job のみ。既存 job（backend / go-vet / govulncheck / frontend /
   frontend-lint / frontend-audit）および `on:` トリガー定義に変更なし。

   注記: `git diff main` では go-vet / govulncheck / frontend-lint / frontend-audit /
   develop トリガー等の差分も出るが、これらは本ブランチの base（直近マージ済みの
   #87/#88/#89）に既に含まれる変更で、`main` がそれらより手前にあることに起因する。
   本 Issue で実際に加えた変更は `git diff HEAD` のとおり trivy job 追加のみ。

   Trivy が実際に脆弱性を検出する/しないの実挙動はランナー上での docker build + スキャン
   実行が必要なため、ローカルでは検証していない（docker / trivy バイナリ未配置のローカル
   環境のため）。ワークフロー定義としての妥当性は YAML 検証 + actionlint で担保している。

## 確認事項

- なし。検出時挙動「警告のみ」・実行タイミング「PR トリガー」はいずれも requirements.md で
  確定済みであり、実装は確定方針に沿っている。スキャン方式（image スキャン）は設計裁量と
  して Req 1.3 を満たす観点から選択した（上記「スキャン方式の選択理由」参照）。

## 受入基準カバレッジの補足（テスト観点）

本変更は GitHub Actions ワークフロー YAML であり、アプリケーションコードのような単体テストは
存在しない。AC の担保は以下の検証で代替する:

- YAML 構文検証（全 AC の前提となるワークフロー妥当性）
- actionlint（ステップ定義・アクション参照の妥当性）
- diff 確認（Req 4 / NFR 2 の既存ジョブ非影響）
- requirements.md の各 AC とワークフロー定義の対応（上記トレーサビリティ表）

STATUS: complete
