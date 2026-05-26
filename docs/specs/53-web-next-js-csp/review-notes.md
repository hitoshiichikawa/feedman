# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T12:25:00Z -->

## Reviewed Scope

- Branch: claude/issue-53-impl-web-next-js-csp
- HEAD commit: fdf51878710d1284d134a4fc6fa2d3f2d1e1da0d
- Compared to: develop..HEAD

差分は 5 ファイル / +566 行（実装は `web/src/lib/csp.ts` 新規 / `web/src/lib/csp.test.ts` 新規 /
`web/next.config.ts` 変更の 3 ファイル、残り 2 ファイルは spec ドキュメント）。本 spec ディレクトリに
`design.md` / `tasks.md` は存在せず、`_Boundary:_` アノテーションが無い design-less impl のため、
boundary 判定は requirements.md の scope と既存コードの突き合わせで実施した。Feature Flag Protocol は
CLAUDE.md で `**採否**: opt-out` のため flag 観点の確認は行わない（通常の 3 カテゴリ判定）。

## Verified Requirements

- 1.1 — `web/next.config.ts:19` `{ key: CSP_HEADER_NAME, value: buildContentSecurityPolicy(process.env) }` を securityHeaders に追加。CSP ヘッダーを 1 件付与
- 1.2 — `web/next.config.ts:27` `source: "/(.*)"` で全ルートへ一律付与
- 1.3 — `csp.ts` `default-src 'self'` を生成 / `csp.test.ts:31` `default-src 'self' を含むこと`
- 2.1 — `csp.test.ts:42` `default-src にワイルドカード * を既定値として許可しないこと`
- 2.2 — `csp.ts` `connect-src 'self'`（dev のみ ws:/wss: 追加）/ `csp.test.ts:53` `connect-src を production では 'self' のみに限定`
- 2.3 — `csp.test.ts:64` `font-src を 'self' に限定し外部フォント CDN を許可しないこと`
- 2.4 — 各ディレクティブ値に機能上不要な外部オリジンを含めない（`csp.test.ts` 各 not.toContain アサート群）
- 3.1 — `web/next.config.ts:10` `X-Content-Type-Options: nosniff` を温存（diff で値変更なし）
- 3.2 — `web/next.config.ts:11` `X-Frame-Options: DENY` を温存
- 3.3 — `web/next.config.ts:12` `Referrer-Policy: strict-origin-when-cross-origin` を温存
- 3.4 — `web/next.config.ts:14-16` `Permissions-Policy: camera=(), microphone=(), geolocation=()` を温存
- 4.1 — `style-src 'self' 'unsafe-inline'` / `csp.test.ts:86` `style-src に 'unsafe-inline'`
- 4.2 — `script-src 'self' 'unsafe-inline'` / `csp.test.ts:75` `script-src に 'unsafe-inline'`
- 4.3 — `img-src 'self' data: https:` / `csp.test.ts:152` `HTTPS 外部画像と data URI を許可`
- 4.4 — `font-src 'self'` / `csp.test.ts:64`
- 4.5 — `npm test`（vitest run）全 164 件 green（reviewer 側でも再実行確認）。新規失敗 0 件
- 5.1 — `csp.test.ts:75` `script-src に 'unsafe-inline' を含めブートストラップ inline script を許可`
- 5.2 — `csp.test.ts:86` `style-src に 'unsafe-inline' を含め inline style を許可`
- 5.3 — `csp.test.ts:97` `production では 'unsafe-eval' を含めない` / `:108` `ws: を含めない` / dev 追加 2 ケース（`:119` `:130`）
- 6.1 — `csp.test.ts:141` `img-src ディレクティブを明示的に定義すること`
- 6.2 — `csp.test.ts:152` `HTTPS 外部画像と data URI を許可しつつ * は許可しないこと`
- 6.3 — `csp.test.ts:164` `img-src の画像許可が script-src へ波及しないこと`（script-src に https:/data: が混入しない）
- NFR 1.1 — 既存 4 ヘッダーの値を diff 上で変更せず保持（追加のみ）
- NFR 1.2 — reviewer 再実行で 24 ファイル / 164 テスト全 pass、新規失敗 0 件
- NFR 2.1 — `csp.test.ts:191` セミコロン区切り 1 行文字列として直列化（平文ヘッダーで可読）
- NFR 3.1 — production 系テスト群が production 値（`'unsafe-eval'` / ws: を含まない）を検証

## Findings

なし

## Summary

requirements.md の全 numeric AC（Req 1〜6 / NFR 1〜3）に対応する実装とテストが diff 内に確認できた。
既存 4 セキュリティヘッダーは値を変更せず温存され、CSP は全ルートへ付与される。reviewer 側でも
vitest 全 164 件 green を再確認し、新規失敗 0 件。design-less impl のため boundary 逸脱は対象外で、
変更は web frontend の CSP scope に閉じている。

RESULT: approve
