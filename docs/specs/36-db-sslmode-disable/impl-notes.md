# 実装ノート: DB 接続 sslmode=disable 固定の解消（Issue #36）

## スコープ

人間が Option B で確定済みのとおり、**設定ファイルとドキュメントの修正のみ**。Go アプリ
ケーションコード・DB スキーマ・マイグレーションには一切変更を加えていない（NFR 1.2）。
起動時警告ガードは別 Issue へ分離済み（Out of Scope）。

## 変更ファイル

- `.env.sample`（`DATABASE_URL` 行）
- `docker-compose.yml`（api / worker 両サービスの `DATABASE_URL`）
- `README.md`（環境変数表 / 本番デプロイ時の注意事項 / ローカル開発例）

## AC トレーサビリティ（対応表）

| AC | 対応箇所 | 対応内容 |
|---|---|---|
| 1.1 | `.env.sample` `DATABASE_URL` 行 | 固定 `?sslmode=disable` を持つ active な `DATABASE_URL` 行を撤去。本行はコメントアウト（`# DATABASE_URL=`）し、固定値を残さない |
| 1.2 | `.env.sample` `DATABASE_URL` 上のコメント | 「コンテナ内 DB 利用時のみ disable 許容」「外部 DB は require 以上を明示」をコメントで明記 |
| 1.3 | `.env.sample` `DATABASE_URL` 周辺コメント + コメントアウト構造 | コンテナ内 DB 例（disable）／外部 DB 例（require）を併記し、利用者が接続先に応じて明示選択する構造にした |
| 2.1 | `docker-compose.yml` api `environment.DATABASE_URL` | `${DATABASE_URL:-<コンテナ内DBデフォルト>}` により未指定時は `db` ホスト・`sslmode=disable` を適用（従来起動を維持） |
| 2.2 | `docker-compose.yml` worker `environment.DATABASE_URL` | api と同一形式で worker 側も未指定時コンテナ内 DB デフォルトを適用 |
| 2.3 | `docker-compose.yml` api / worker `DATABASE_URL` | `${DATABASE_URL:-...}` により `.env` 等からの環境別差し替えを許容（両サービス） |
| 2.4 | `docker-compose.yml` api / worker `DATABASE_URL` | `DATABASE_URL` 未上書き時はコンテナ内 DB 向けデフォルト接続文字列を適用 |
| 3.1 | `README.md` 本番デプロイ時の注意事項（DB 接続の TLS 設定） | 「コンテナ内 DB 利用時のみ `sslmode=disable` 許容」を明記 |
| 3.2 | `README.md` 本番デプロイ時の注意事項 + 環境変数表 `DATABASE_URL` 行 | 「外部 PostgreSQL 接続時は `require` 以上を必須」を明記 |
| 3.3 | `README.md` 本番デプロイ時の注意事項 | `DATABASE_URL` の sslmode を接続先に応じて設定する手順を本番注意事項として追加（外部 DB の `?sslmode=require` 例を含む） |
| 3.4 | `README.md` ローカル開発（Docker なし）例 / 本番デプロイ時の注意事項 | 外部 DB 接続例で `disable` を推奨せず `require` 以上を案内。ローカル例の `localhost` 接続のみ disable 許容とコメント明記 |
| NFR 1.1 | `docker-compose.yml` | `${DATABASE_URL:-...}` の `:-` により未指定時は変更前と同一のコンテナ内 DB 接続文字列を生成（後述の検証で確認） |
| NFR 1.2 | （全体） | Go コード・スキーマ・マイグレーション無変更 |
| NFR 2.1 | 3 ファイル全体 | sslmode 記述（コンテナ内=disable 許容 / 外部=require 以上）を 3 ファイルで一貫させた |

## `.env.sample` の sslmode 扱いに関する設計判断と根拠

**判断: active な `DATABASE_URL` 行をコメントアウトし、コンテナ内 DB 例（disable）と外部 DB 例
（require）を併記するコメント構造にした。**

根拠:

- AC 1.1 は「`?sslmode=disable` の固定指定を `.env.sample` の `DATABASE_URL` に残さない」ことを
  要求する。一方で、`.env.sample` をそのままコピーしてコンテナ内 DB で開発する後方互換を壊さない
  配慮も求められている。
- `lib/pq` は接続 URL に `sslmode` を **省略すると既定で `require`** を要求するため、active 行から
  単に `?sslmode=disable` を外して値を残すと、コンテナ内 DB（TLS 無効構成）への接続が壊れる。
  そのため「値を残しつつ sslmode だけ削る」案は後方互換を満たせず却下した。
- 代わりに active 行をコメントアウト（`# DATABASE_URL=`）した。これにより `.env.sample` を
  `.env.production` にコピーして `docker compose --env-file` で起動しても、`DATABASE_URL` が
  env に設定されないため `docker-compose.yml` 側の `${DATABASE_URL:-<コンテナ内DBデフォルト>}`
  が適用され、従来どおりコンテナ内 DB（`sslmode=disable`）で起動できる（AC 2.4 と整合）。
- 固定 disable 値を active 行として残さないことで AC 1.1 を満たしつつ、コメントで接続先別の
  明示選択（AC 1.2 / 1.3）を促す形にした。

## 実行した検証コマンドと結果

### 1. `docker-compose.yml` の `DATABASE_URL` 解決の妥当性（AC 2.1〜2.4 / NFR 1.1）

`docker compose config`（compose v2）でフル `docker-compose.yml` を検証したところ、本変更とは
**無関係な既存の healthcheck 定義**（`test: ["/feedman", "healthcheck"]` の exec-form）が compose v2
の strict 検証で `healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"` と弾かれ、
フル config のレンダリングが完了しなかった。当該 healthcheck 行は本 PR で変更しておらず
（`git diff docker-compose.yml` に healthcheck 行は含まれない）、本 Issue のスコープ外の既存事項
である。

そのため、本変更で実際に編集した `DATABASE_URL` のインターポレーションロジックのみを
最小 compose ファイルに切り出して検証した:

```
=== default (unset) ===
  DATABASE_URL: postgres://feedman:feedman@db:5432/feedman?sslmode=disable   # api / worker 両方
=== override require ===
  DATABASE_URL: postgres://u:p@ext:5432/d?sslmode=require                     # api / worker 両方
=== POSTGRES_* override ===
  DATABASE_URL: postgres://feeduser:secret@db:5432/feeddb?sslmode=disable     # api / worker 両方
```

- 未指定時はコンテナ内 DB（`db` ホスト, `sslmode=disable`）向けデフォルトを適用 → AC 2.1 / 2.2 / 2.4 / NFR 1.1 を満たす
- `DATABASE_URL` 上書き時は外部 DB（`sslmode=require`）にそのまま差し替わる → AC 2.3 を満たす
- ネストした `POSTGRES_USER/PASSWORD/DB` のデフォルトも従来どおり解決される（後方互換）

### 2. `go vet ./...`

実行を試みたが、本サンドボックス環境では Go 1.25 ツールチェーンが download できず実行不可
（`go: download go1.25 for linux/amd64: toolchain not available`）。本変更は Go コードを 1 行も
変更していない（`git diff --name-only` は `.env.sample` / `README.md` / `docker-compose.yml` のみ）
ため、Go ビルド・テストへの影響はない。

### 3. `.env.sample` / `README.md` の目視確認

- `.env.sample`: 固定 disable 値の active 行が無いこと、コンテナ内/外部の例とコメントが AC 1.1〜1.3 を満たすことを確認。
- `README.md`: 環境変数表に `DATABASE_URL` 行を追加（AC 3.1/3.2）、本番デプロイ注意事項に sslmode 手順を追加（AC 3.1〜3.3）、外部 DB 例で disable を推奨しないこと（AC 3.4）を確認。
- 3 ファイルの sslmode 記述（コンテナ内=disable 許容 / 外部=require 以上）が相互に矛盾しないこと（NFR 2.1）を確認。

## 確認事項

- **既存 healthcheck 定義（`test: ["/feedman", "healthcheck"]`）が compose v2 の strict 検証で
  invalid と判定される**: 本 Issue のスコープ外（DATABASE_URL とは無関係 / 本 PR では未変更）の
  ため修正していないが、`docker compose config` のフルレンダリングが当該行で失敗する。compose v2
  運用時に支障となりうるため、別 Issue での修正（`test: ["CMD", "/feedman", "healthcheck"]` への
  変更等）を検討する価値がある。本 PR では touch しない（spec 書き換え禁止・スコープ厳守のため）。
- `.env.sample` の active `DATABASE_URL` 行をコメントアウトする方針は requirements の AC を満たす
  範囲で Developer 判断として確定したもの（上記「設計判断」参照）。別案（コンテナ内 DB のみ
  active で残し外部は別キー）も理論上可能だが、AC 1.1 の「固定 disable を残さない」を最も素直に
  満たす本案を採用した。

## 派生タスク候補

- compose v2 strict 検証に適合するよう healthcheck の exec-form を `CMD` prefix 付きに修正する Issue。
- （別 Issue として分離済み）起動時に `sslmode=disable` かつ外部ホスト接続を検知した場合の警告
  ログガードの実装。

STATUS: complete
