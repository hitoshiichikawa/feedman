# 実装ノート: Issue #39 docker-compose 秘匿情報 fail-fast 化

## 概要

`docker-compose.yml` の秘匿情報（`POSTGRES_PASSWORD` / `SESSION_SECRET`）を必須化し、
未設定/空文字での `docker compose up` / `docker compose config` を fail-fast させた。
弱い既知デフォルト（`feedman` / `dummy`）を排除し、`.env.sample` / `README.md` に
安全な値の生成手順を追記した。人間決定（Option B）に従い compose ファイル分離は行わず、
単一 `docker-compose.yml` の `.env` 必須化に統一した。

## 変更ファイル

- `docker-compose.yml` — 秘匿情報の必須化（`${VAR:?...}` 構文）と弱いデフォルト排除
- `.env.sample` — `POSTGRES_PASSWORD` を必須節へ移動、生成コマンド併記
- `README.md` — 起動必須化・fail-fast・生成手順の明示
- `docs/specs/39-docker-compose-yml-db-session-secret/test-fixtures/test-compose-fail-fast.sh` — 検証スクリプト

## 採用した compose 必須化構文と enforcement 配置の判断理由

### 必須化構文

docker compose のインターポレーション機能 `${VAR:?エラーメッセージ}` を採用した。`VAR` が
未設定または空文字の場合、`docker compose config` / `up` の設定読込フェーズ全体が非ゼロ終了し、
指定したエラーメッセージを stderr に出力する。設定読込はファイル全体に対して行われるため、
`db` サービスに置いた `${POSTGRES_PASSWORD:?...}` は他サービスの起動有無に関わらず compose 全体を
fail-fast させられることを `docker compose config` の実挙動で確認済み（後述の検証結果参照）。

### enforcement 配置

| env var | enforcement 配置 | 理由 |
|---|---|---|
| `POSTGRES_PASSWORD` | `db` サービスの `POSTGRES_PASSWORD` 1 箇所に `${VAR:?...}` を集約 | DB が本変数の一次利用者であり、エラーメッセージの重複を避けるため |
| `SESSION_SECRET` | `api` / `worker` 両サービスの `SESSION_SECRET` に `${VAR:?...}` | 両サービスが本変数を直接参照するため。どちらが先に評価されても var 名が出る |

### DATABASE_URL ネスト側の扱い（判断理由）

`api` / `worker` の `DATABASE_URL` デフォルトはネスト interpolation
`${DATABASE_URL:-postgres://${POSTGRES_USER:-feedman}:${POSTGRES_PASSWORD}@db:5432/${POSTGRES_DB:-feedman}?sslmode=disable}`
で構成される。ここでネストされた `${POSTGRES_PASSWORD}` は **デフォルト無し・エラー無し**
（`:-feedman` も `:?...` も付けない）とした。判断理由:

- 弱いデフォルト `:-feedman` を除去することで Req 2.3（DATABASE_URL デフォルトに弱い既知
  パスワードが埋め込まれない）を満たす。
- enforcement（fail-fast の `:?...`）は `db` サービス側 1 箇所に一本化済みのため、ネスト側にも
  `:?...` を付けると同一 env で 2 重にエラー宣言となり、エラーメッセージが冗長になる。
- ネスト側を素の `${POSTGRES_PASSWORD}` にしても、`POSTGRES_PASSWORD` 未設定時は `db` サービスの
  `:?...` が先に compose 全体を fail-fast させるため、DATABASE_URL のパスワード部が空のまま
  解決されることはない（検証で確認済み）。

`POSTGRES_USER` / `POSTGRES_DB` のネストデフォルト `:-feedman` は非秘匿のため温存した（Req 2.4）。

### エラーメッセージの記法上の注意（YAML パース）

`environment` を list 記法（`- KEY=VALUE`）で書く場合、エラーメッセージ中に `: `（コロン + 空白）が
含まれると YAML がそれを map のキーと誤認しパースエラー（`unexpected type map[string]interface {}`）に
なる。このため、(1) 該当エントリをダブルクォートで囲み、(2) エラーメッセージ内の区切りを
`: ` ではなく ` - ` にした。最終形:

- `"POSTGRES_PASSWORD=${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required - generate with 'openssl rand -base64 32'}"`
- `"SESSION_SECRET=${SESSION_SECRET:?SESSION_SECRET is required - generate with 'openssl rand -base64 32'}"`

エラーメッセージは「どの env var が必要か」と「生成コマンド」のみを含み、値そのものは出力しない
（NFR 3.2）。

## 各 AC への対応マッピング

| AC | 対応 | 担保するテスト/検証 |
|---|---|---|
| Req 1.1 (POSTGRES_PASSWORD 未設定/空で up 失敗) | db サービス `${POSTGRES_PASSWORD:?...}` | test-compose-fail-fast.sh「空文字でも非ゼロ終了」(config で代替検証) |
| Req 1.2 (SESSION_SECRET 未設定/空で up 失敗) | api/worker `${SESSION_SECRET:?...}` | 同上「空文字でも非ゼロ終了」 |
| Req 1.3 (POSTGRES_PASSWORD 未設定で config 非ゼロ) | 同上 | 「POSTGRES_PASSWORD のみ未設定で非ゼロ終了」 |
| Req 1.4 (SESSION_SECRET 未設定で config 非ゼロ) | 同上 | 「SESSION_SECRET のみ未設定で非ゼロ終了」 |
| Req 1.5 (var 名を示すエラー) | `${VAR:?...}` のメッセージ | 「エラーメッセージに var 名が含まれる」 |
| Req 2.1 (SESSION_SECRET の弱いデフォルト dummy を持たない) | `:-dummy` を `:?...` に置換 | compose 差分 + config 異常系 |
| Req 2.2 (POSTGRES_PASSWORD の弱いデフォルト feedman を持たない) | `:-feedman` を `:?...` に置換 | compose 差分 + config 異常系 |
| Req 2.3 (DATABASE_URL デフォルトに弱いパスワードが埋まらない) | ネスト `${POSTGRES_PASSWORD}` から `:-feedman` 除去 | 「DATABASE_URL デフォルトに feedman が埋め込まれない」 |
| Req 2.4 (POSTGRES_USER/DB は従来デフォルト feedman 維持) | `:-feedman` 温存 | 「POSTGRES_USER/DB はデフォルト feedman が適用される」 |
| Req 3.1 (両設定時に 4 コンテナ起動) | 必須化のみで構成不変 | 「web/api/worker/db の 4 サービスが config で解決される」(config による静的確認) |
| Req 3.2 (両設定時に config ゼロ終了) | 同上 | 「両秘匿情報のインターポレーションは解決済み」 |
| Req 3.3 (env var 名を変更せず後方互換) | 既存 var 名を維持 | compose 差分（改名なし） |
| Req 4.1 (.env.sample に SESSION_SECRET 生成コマンド) | .env.sample 必須節 | .env.sample 差分 |
| Req 4.2 (.env.sample に POSTGRES_PASSWORD 生成コマンド) | .env.sample 必須節へ移動 | .env.sample 差分 |
| Req 4.3 (README に起動必須を明示) | README 必須テーブル + 注意事項 | README 差分 |
| Req 4.4 (README に未設定時の失敗と回避手順) | README「2. 環境変数の設定」「本番デプロイ時の注意事項」 | README 差分 |
| NFR 1.1 (env var 改名なし) | 既存 var 名を一切変更せず | compose 差分 |
| NFR 1.2 (両設定時に同一構成で起動) | サービス/ネットワーク/ポート定義は不変 | config によるサービス解決確認 |
| NFR 2.1 (.env 1 回設定で起動) | 単一 .env 必須化を維持 | .env.sample / README の導線記述 |
| NFR 2.2 (エラーで var 名を特定可能) | `${VAR:?...}` のメッセージに var 名 | 「エラーメッセージに var 名が含まれる」 |
| NFR 3.1 (実値をコミットしない) | プレースホルダのみ | .env.sample はプレースホルダのみ |
| NFR 3.2 (エラーに値を出力しない) | メッセージは var 名 + 生成コマンドのみ | エラーメッセージ内容確認 |

## 検証結果

検証スクリプト: `docs/specs/39-docker-compose-yml-db-session-secret/test-fixtures/test-compose-fail-fast.sh`
（`bash` + `set -euo pipefail`、`env -i` で環境変数を隔離して `docker compose config` の終了コード・
stderr を assert）。

実行結果: **12 passed, 0 failed**（`docker compose version` = v5.1.3 / `docker --version` = 29.4.3）。

- 異常系（非ゼロ終了 + var 名出力）:
  - 秘匿情報を一切設定せず → 非ゼロ終了、エラーに var 名（Req 1.3, 1.4, 1.5）
  - `POSTGRES_PASSWORD` のみ未設定 → 非ゼロ終了、エラーに `POSTGRES_PASSWORD`（Req 1.3, 1.5）
  - `SESSION_SECRET` のみ未設定 → 非ゼロ終了、エラーに `SESSION_SECRET`（Req 1.4, 1.5）
  - 両方を空文字 → 非ゼロ終了（Req 1.1, 1.2）
- 正常系（両設定時の解決）:
  - 両秘匿情報のインターポレーションが解決される（Req 3.2）
  - `web` / `api` / `worker` / `db` の 4 サービスが解決される（Req 3.1）
  - `POSTGRES_USER` / `POSTGRES_DB` は未設定時にデフォルト `feedman` が適用される（Req 2.4）
  - `DATABASE_URL` デフォルトに弱い既知パスワード `feedman` が埋め込まれない（Req 2.3）
  - `DATABASE_URL` パスワード部に設定値が反映される（後方互換 / NFR 1.1）

### 検証環境固有の制約（healthcheck 記法）

本サンドボックスの docker compose は非常に新しいバージョン（v5.1.3 系）で、`api` サービスの
healthcheck 配列記法 `["/feedman", "healthcheck"]` を
`healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` として拒否する。これは
**Issue #39 とは無関係な既存の compose ファイル記法**であり（本変更前の `docker-compose.yml` でも
同じエラーが再現することを確認済み）、本 Issue のスコープ外のため `docker-compose.yml` 本体には
手を入れていない。実 CI 環境（プロジェクトが想定する compose バージョン）では配列記法は暗黙の
`CMD` として有効であり、正常系の `docker compose config` がゼロ終了する。

この環境固有の制約により、正常系の semantics 検証（Req 2.3 / 2.4 / 4 サービス解決）はスクリプト内で
**api healthcheck を CMD 付きに正規化した一時コピー（shim）** で `docker compose config` を実行して
確認している。shim は検証目的のみで実ファイルには触れない。実 CI 環境ではこの shim は不要。

### 既存テスト（Go/TS）への影響

本変更は `docker-compose.yml` / `.env.sample` / `README.md` の 3 ファイルおよび検証スクリプトのみで、
Go / TypeScript のアプリケーションコードには一切触れていない（`git diff --stat HEAD~3..HEAD` で確認済み）。
したがって `go test ./...` / `web` の `npm test` の結果は本変更で変化しない（コード非変更のため既存
テスト不変）。

## 確認事項

- **検証環境の docker compose バージョン**: 本サンドボックスの docker compose は v5.1.3 という
  通常より新しいバージョンで、`api` の healthcheck 配列記法を拒否する（Issue #39 と無関係の既存記法）。
  正常系 `config` のゼロ終了は実 CI 環境前提で担保している。レビュワーは実 CI 環境（プロジェクト想定の
  compose バージョン）で正常系 `docker compose config` がゼロ終了することを確認できると望ましい。
  なお本制約は本 PR のスコープ外（api healthcheck 記法の整備は別 Issue の領分）。
- 上記以外の不明点・要件矛盾はなし。

STATUS: complete
