# 実装ノート: Issue #97 web（Next.js standalone）の全 interface bind

## 変更概要

Next.js standalone server（`.next/standalone/server.js`）は起動時に `process.env.HOSTNAME`
が指すアドレスへ bind する。Docker は既定でコンテナ ID を `HOSTNAME` に自動設定するため、
`internal` / `external` の 2 ネットワークに所属し複数 IP を持つ web コンテナは、コンテナ ID
が解決する IP に bind し、公開ポート（NAT 先 IP）と bind 先が食い違って接続拒否になっていた。

本変更では requirements.md の **Requirement 2.1（image 自己完結性）** を採用し、
`web/Dockerfile` の runner ステージに以下の `ENV` を追加した。

- `ENV HOSTNAME=0.0.0.0` — Docker 既定のコンテナ ID 由来 `HOSTNAME` を image 側で上書きし、
  全 interface（全 IP）へ bind させる。外部 environment 無しでも到達可能（Req 2.1）。
- `ENV PORT=3000` — 待ち受けポートを image に明示する（`EXPOSE 3000` と整合）。

配置は runner ステージ既存の `ENV NODE_ENV=production` 直後にまとめた。

### Docker env 優先順位による上書き可能性（Req 2.2 / 2.3）

Dockerfile の `ENV` は、`docker-compose.yml` の `environment` / `docker run -e` で同名の
環境変数が与えられた場合にそれらが優先される（Docker の env 解決順）。したがって:

- `HOSTNAME` を外部から指定すれば bind アドレスを上書きできる（Req 2.2）。
- `PORT` を外部から指定すれば待ち受けポートを変更できる（Req 2.3）。

### compose 側の扱い（Out of Scope と整合）

`docker-compose.yml` の web サービスには bind アドレス／ポートの environment を追加していない。
Issue 方針および requirements.md「image の自己完結性」を正とし、設定箇所を Dockerfile に一本化した。
これは Out of Scope の「web の待ち受けポート番号そのものの既定値変更（既定 `3000` を維持）」とも整合する
（`PORT=3000` は既定値の明示であり既定値の変更ではない）。

`web` 以外（api / worker / db）のコンテナ構成・起動挙動は一切変更していない（NFR 1.2 / Out of Scope）。

## テスト方針

本変更は Dockerfile の `ENV` 追加であり、`docker build` + `run` + `curl 200` のフル E2E は
CI / unit テスト環境では実行できない。代わりに **回帰を機械的にロックする近傍テスト**を追加した。

- 追加テスト: `web/dockerfile-hostname.test.ts`（Vitest / BDD スタイル）
- `web/Dockerfile` を読み込み、**最後の `FROM ... AS runner` 行以降（= runner ステージ本体）**を
  切り出して、以下を assert する:
  - runner ステージに `ENV HOSTNAME=0.0.0.0` が存在すること（Req 2.1 = 全 interface bind の image 組み込み）
  - runner ステージに `ENV PORT=3000` が存在すること（Req 2.3 の待ち受けポート明示）
  - ビルドステージ（deps / builder）には `ENV HOSTNAME` / `ENV PORT` を設定しないこと
    （runner ステージ限定の上書きであることを保証し、誤検出を防ぐ）
- Red → Green: ENV 未追加の状態で 2 件の assertion が失敗することを確認してから Dockerfile を修正して通した。

### 受入基準とテストの対応

| Requirement ID | 担保方法 |
|---|---|
| 1.1（全 interface 待ち受け） | `dockerfile-hostname.test.ts`「runner ステージに ENV HOSTNAME=0.0.0.0」で image 設定を回帰ガード。実 bind は手動 / staging 検証で担保 |
| 1.2（起動ログに `http://0.0.0.0:3000`） | 手動 / staging 検証で担保（unit 検証不能。Next.js standalone server が出力） |
| 1.3（公開ポート経由 `/` で HTTP 200） | 手動 / staging 検証で担保（unit 検証不能） |
| 1.4（応答 HTML に `<title>Feedman - RSS リーダー</title>`） | 手動 / staging 検証で担保（unit 検証不能） |
| 1.5（ホスト側ポート変更後も到達可能） | 手動 / staging 検証で担保（compose `${WEB_PORT}` マッピングは既存。本変更非対象） |
| 2.1（外部 env 無しでも全 interface bind = image 自己完結） | `dockerfile-hostname.test.ts`「ENV HOSTNAME=0.0.0.0」で回帰ガード |
| 2.2（外部 env 指定時は優先） | Docker の env 優先順位で担保（仕様）。手動 / staging で `HOSTNAME` 上書き確認 |
| 2.3（待ち受けポート変更可能） | `dockerfile-hostname.test.ts`「ENV PORT=3000」で既定明示を回帰ガード。上書きは Docker env 優先順位（仕様） |
| 3.1（`API_INTERNAL_URL` 未設定で fail-fast） | 既存 `web/src/lib/rewrites.test.ts` の `resolveApiInternalUrl` テスト群（未設定 / 空 / 空白で throw）で担保。本変更は entrypoint の fail-fast を変更しないため既存挙動を保持 |
| 3.2（有効時は起動継続し全 interface bind） | 手動 / staging 検証で担保（unit 検証不能）+ ENV HOSTNAME 回帰ガード |
| 3.3（公開ポート未マッピング網からの到達可否を従来どおり維持） | compose のネットワーク分離（`internal` / `external`）を非変更とすることで担保（Out of Scope） |
| NFR 1.1（変更前後で `/` の 200 応答・HTML 同一） | 手動 / staging 検証で担保。bind 範囲拡大のみで応答内容は不変 |
| NFR 1.2（api / worker / db を変更しない） | `web/Dockerfile` のみ変更。他コンポーネント未変更で担保 |
| NFR 2.1（待ち受けアドレスを起動ログに出力） | 手動 / staging 検証で担保（Next.js standalone server が Network 行を出力） |

## 手動 / staging 検証で担保する AC

`docker build` + `curl` を要する以下の AC は unit 環境で検証不能なため、手動 / staging 検証で担保する:

- Req 1.2 / 1.3 / 1.4 / 1.5、Req 2.2、Req 3.2、NFR 1.1、NFR 2.1
- Issue にて staging で `HOSTNAME=0.0.0.0` 付与 → 公開ポート経由 `/` で 200 を確認済みである旨が示されている。
  本実装はその設定を image 側 `ENV` に恒久化したものである。

## 確認事項（レビュワー判断ポイント）

- **設定箇所の一本化**: requirements.md Open Questions は最終的な設定箇所（Dockerfile / compose の
  双方か片方か）を design.md の領分としているが、本 Issue は design なし impl のため、Issue 方針
  「image の自己完結性のため Dockerfile への追加が望ましい」に従い **Dockerfile に一本化**した。
  compose 側に environment を重複追加していない。この判断で問題ないか確認されたい。
- **`ENV PORT=3000` の追加是非**: Issue 方針で `EXPOSE 3000` と整合する待ち受けポート明示として
  追加した。Out of Scope の「既定ポート番号の変更」には当たらない（既定値の明示であり変更ではない）が、
  スコープ判断の確認をお願いしたい。
- **テスト方式**: 本変更は Dockerfile の宣言的設定であり、フル E2E は CI で実行できないため、
  Dockerfile を文字列解析する回帰ガードテストで担保している。実 bind 挙動は手動 / staging 検証に依存する。

## 補足（実行環境メモ）

- 当 worktree のローカル node は v22.11.0 で、vite 7 / vitest 4 の ESM ローダ要件
  （Node 22.12+）をわずかに下回るため、`npm test` / `npm run lint` を
  `NODE_OPTIONS=--experimental-require-module` 付きで実行した（テスト結果には影響しない）。
  CI 環境では Node バージョンが要件を満たす想定。

## 検証結果

- `npm test`（= `vitest run`）: 26 ファイル / 187 テスト すべて green（新規 `dockerfile-hostname.test.ts` の 4 件を含む）
- `npm run lint`（ESLint）: 0 errors（warning 6 件はすべて本変更非対象の既存ファイル。新規テストファイルは warning なし）

STATUS: complete
