# 実装ノート: #53 web (Next.js) に CSP セキュリティヘッダーを設定

## 実装サマリ

`web`（Next.js）が返す全ルートの HTTP レスポンスに `Content-Security-Policy`（CSP）ヘッダーを
付与した。XSS への多層防御（defense in depth）が目的で、特に記事本文 HTML を
`dangerouslySetInnerHTML` で描画する箇所に対する追加の安全網となる。

### 変更ファイル

- `web/src/lib/csp.ts`（新規）— CSP 文字列を生成する純粋モジュール。`#41` の `rewrites.ts`
  パターン（env を引数注入する純粋関数）に倣い、`process.env.NODE_ENV` への直接依存を排して
  Vitest で単体テスト可能にした。
  - `CSP_HEADER_NAME`: HTTP ヘッダー名定数（`"Content-Security-Policy"`）
  - `buildCspDirectives(env)`: 環境に応じたディレクティブの `Map`（挿入順保持）を構築する純粋関数
  - `buildContentSecurityPolicy(env)`: 上記を `"<directive> <values>; ..."` の 1 行へ直列化する純粋関数
- `web/src/lib/csp.test.ts`（新規）— Vitest による単体テスト（18 ケース）。
- `web/next.config.ts`（変更）— 既存の `securityHeaders` 配列に CSP エントリを 1 件追加。
  既存の 4 ヘッダー（#41 由来）と `rewrites()` は値・構造とも温存。CSP 値は
  `buildContentSecurityPolicy(process.env)` で生成する。

### 設計判断

- `headers()` による**静的 CSP** を採用（per-request nonce は middleware を要し本 Issue の
  スコープ＝`next.config.ts` の `headers()` から外れるため）。
- dev / production の厳格さは `NODE_ENV` で切り替える。`process.env.NODE_ENV !== "production"`
  を dev 相当（緩め）とし、`NODE_ENV` 未設定も dev 相当に倒した（HMR 環境での機能維持を優先）。

## 採用したディレクティブと根拠

production の生成例:
```
default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'
```

| ディレクティブ | production 値 | dev 追加 | 根拠 / 対応 AC |
|---|---|---|---|
| `default-src` | `'self'` | — | 既定取得元を同一オリジンに限定、ワイルドカード不許可（Req 1.3 / 2.1） |
| `script-src` | `'self' 'unsafe-inline'` | `'unsafe-eval'` | Next.js App Router のブートストラップ inline script 用に `'unsafe-inline'`。dev は HMR/turbopack の eval 用に `'unsafe-eval'`（本番では付けない）（Req 5.1 / 5.3 / NFR 3） |
| `style-src` | `'self' 'unsafe-inline'` | — | Tailwind / next/font が注入する inline style 用（Req 5.2 / 4.1） |
| `img-src` | `'self' data: https:` | — | 記事本文 HTML 中の外部画像表示のため data URI と HTTPS 外部画像を許可（平文 http は不許可）。script-src と独立のため画像許可が script 実行へ波及しない（Req 6.1 / 6.2 / 6.3 / 4.3） |
| `font-src` | `'self'` | — | next/font はビルド時セルフホスト。外部フォント CDN を許可しない（Req 2.3 / 4.4） |
| `connect-src` | `'self'` | `ws: wss:` | API/認証は #41 で同一オリジン化済み。dev は HMR の websocket を許可（Req 2.2） |
| `object-src` | `'none'` | — | OWASP 推奨の最小堅牢化（プラグイン埋め込みの全面禁止） |
| `base-uri` | `'self'` | — | `<base>` 乗っ取りによる相対 URL リダイレクトの防止 |
| `frame-ancestors` | `'none'` | — | クリックジャッキング対策。`X-Frame-Options: DENY`（#41）と整合 |
| `form-action` | `'self'` | — | 外部オリジンへの form 送信を防止 |

既存セキュリティヘッダー（`X-Content-Type-Options` / `X-Frame-Options` / `Referrer-Policy` /
`Permissions-Policy`）は `securityHeaders` 配列内でそのまま温存しており、値は変更していない
（Req 3.1〜3.4 / NFR 1.1）。

## 受入基準 (AC) とテスト対応

| AC | 担保したテスト / 実装 |
|---|---|
| Req 1.1 / 1.2（全ルート付与） | `next.config.ts` の `headers()` が `source: "/(.*)"` で全ルートへ付与（既存構造を踏襲）。CSP エントリ追加で担保 |
| Req 1.3（default-src 含む） | csp.test.ts `default-src 'self' を含むこと` |
| Req 2.1（self 限定 / `*` 不許可） | csp.test.ts `default-src 'self'`、`ワイルドカード * を許可しないこと` |
| Req 2.2（connect-src self） | csp.test.ts `connect-src を production では 'self' のみに限定` |
| Req 2.3 / 4.4（font-src self / 外部CDN不許可） | csp.test.ts `font-src を 'self' に限定し外部フォント CDN を許可しないこと` |
| Req 2.4（不要な外部オリジン不許可） | 上記各テストで外部オリジンを許可していないことを担保 |
| Req 3.1〜3.4（既存ヘッダー温存） | `next.config.ts` で `securityHeaders` の既存 4 件を温存。全 164 テスト green で回帰なし（NFR 1.2） |
| Req 4.1（CSS 適用） | `style-src 'self' 'unsafe-inline'`（csp.test.ts `style-src に 'unsafe-inline'`） |
| Req 4.2（スクリプト実行） | `script-src 'self' 'unsafe-inline'`（csp.test.ts `script-src に 'unsafe-inline'`） |
| Req 4.3（記事本文 HTML 描画） | `img-src 'self' data: https:`（csp.test.ts img-src 系） |
| Req 4.5 / NFR 1.2（既存テスト合格） | `npm test`（vitest run）全 164 件 green、新規失敗 0 件 |
| Req 5.1（inline script） | csp.test.ts `script-src に 'unsafe-inline' を含め...` |
| Req 5.2（inline style） | csp.test.ts `style-src に 'unsafe-inline' を含め...` |
| Req 5.3（最小許可 / 本番厳格） | csp.test.ts `production のとき 'unsafe-eval' を含めないこと`、`production のとき ws: を含めないこと`、dev のみ追加する 2 ケース |
| Req 6.1（img-src 明示定義） | csp.test.ts `img-src ディレクティブを明示的に定義すること` |
| Req 6.2（img-src 一貫した許可範囲） | csp.test.ts `HTTPS 外部画像と data URI を許可しつつ * は許可しないこと` |
| Req 6.3（画像許可が script へ波及しない） | csp.test.ts `img-src の画像許可が script-src へ波及しないこと` |
| NFR 2.1（平文ヘッダーで可読） | csp.test.ts `セミコロン区切りの 1 行文字列として直列化` + `next.config.ts` が平文ヘッダーとして付与 |
| NFR 3.1（本番ビルドで正常） | production 系テスト群で production 値を検証。`script-src` から `'unsafe-eval'` を除外し正常表示可能な値を生成 |

境界・異常系: `NODE_ENV` 未設定（空入力）時に dev 相当を返すこと（csp.test.ts 境界ケース）、
production / development の分岐両方をカバー。

## テスト・lint 実行結果

- `npm test`（= `vitest run`）: **24 ファイル / 164 テスト 全 pass、新規失敗 0 件**
  （新規 `csp.test.ts` は 18 テスト pass）
- `npm run lint`（ESLint）: **0 errors**（既存の警告 6 件のみ。いずれも本変更と無関係の既存箇所）

### 実行環境に関する注記（確認事項ではなく環境メモ）

本ワークステーションの Node は v22.11.0 で、`vite@7.3.1` が要求する `^22.12.0` を僅かに
下回るため、`vitest run` 起動時に `require()` で vite の ESM をロードできず `ERR_REQUIRE_ESM`
となる。これは実行環境固有の制約であり、コードの問題ではない。本ステージでは
`NODE_OPTIONS="--experimental-require-module"` を付与して全テストの green を確認した
（CI（GitHub Actions）は規定の Node バージョンで `npm test` を実行するため本フラグは不要）。

## 確認事項（人間の最終確認を要する点）

本 Issue の Open Questions に対し、オーケストレーター提示の妥当デフォルトを採用した。
以下は人間判断を仰ぎたい点。

1. **インライン script / style の許可手段（`'unsafe-inline'` 採用）**:
   - 現状は `headers()` 静的 CSP の制約上 nonce を使えないため `script-src`/`style-src` に
     `'unsafe-inline'` を許容した。これは CSP による script injection 防御を弱める。
   - 将来 middleware を導入して per-request nonce 方式へ寄せれば `'unsafe-inline'` を外して
     より厳格化できる（別 Issue 化を推奨）。nonce 方式採否は人間判断としたい。

2. **記事本文 HTML 中の外部画像（`img-src 'self' data: https:`）**:
   - RSS リーダーの記事本文は外部オリジンの画像を多数含むため、外部画像を全ブロックすると
     既存の閲覧体験（Req 4.3 の後方互換）を壊す。体験寄りに HTTPS 外部画像を許可した。
   - 安全性を最優先して `img-src 'self' data:` に絞る選択肢もある（外部画像はブロック）。
     体験寄り / 厳格寄りのいずれを最終方針とするかは人間判断としたい。

3. **dev モードでの `'unsafe-eval'` / websocket 許可**:
   - HMR / turbopack の動作維持のため dev のみ `script-src 'unsafe-eval'` と
     `connect-src ws: wss:` を追加した。production には含めない。
   - dev/production の分岐は `NODE_ENV` 判定であり、`NODE_ENV` 未設定時は dev 相当（緩め）に
     倒している。本番配信は `NODE_ENV=production` 前提である点を運用で担保する必要がある。

4. **CSP Report-Only / report-uri は未導入**:
   - Out of Scope の通り、レポーティング運用や Report-Only 段階導入は本 Issue で扱っていない。
     本番投入後に実機で CSP 違反が観測された場合のディレクティブ調整は別途必要になりうる。

5. **既存テストの tsc 型エラー（本変更とは無関係 / 既存問題）**:
   - `npx tsc --noEmit` は既存の `src/lib/rewrites.test.ts` で `ProcessEnv` の `NODE_ENV` 必須化
     由来の型エラーを 8 件報告するが、これは本変更前から存在する既存問題であり、本 Issue の
     spec 書き換え禁止 / scope 外修正回避の方針から修正していない（CI は `vitest run` と
     `eslint` を用い、`tsc --noEmit` をゲートにしていないため CI は通る）。新規追加した
     `csp.ts` / `csp.test.ts` / `next.config.ts` の変更箇所は tsc エラーを生まない。
     必要なら別 Issue で `rewrites.test.ts` の型を是正することを推奨する。

STATUS: complete
