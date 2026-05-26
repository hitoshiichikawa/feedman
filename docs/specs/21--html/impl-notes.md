# 実装ノート（Issue #21: 記事本文のクライアント側 HTML サニタイズ）

## 実装概要

記事展開表示（`web/src/components/item-detail.tsx`）が記事本文 HTML を
`dangerouslySetInnerHTML` で DOM 挿入する際、描画直前にフロントエンド側でも
サニタイズする多層防御（defense in depth）を追加した。バックエンドの bluemonday
サニタイズ（`internal/security/content_sanitizer.go`）は変更していない（スコープ外）。

サニタイズ処理は単一責務のユーティリティ `web/src/lib/sanitize.ts` の
`sanitizeContentHtml(rawHtml: string): string` に切り出し、`item-detail.tsx` から
`useMemo` でメモ化して呼び出す形にした（コンポーネント肥大化の回避・テスト容易性）。

## 追加・変更ファイル

| ファイル | 種別 | 内容 |
|---|---|---|
| `web/src/lib/sanitize.ts` | 追加 | DOMPurify を用いた記事本文サニタイズユーティリティ |
| `web/src/lib/sanitize.test.ts` | 追加 | サニタイズユーティリティの単体テスト（11 ケース） |
| `web/src/components/item-detail.tsx` | 変更 | `dangerouslySetInnerHTML` をサニタイズ済み HTML に差し替え |
| `web/src/components/item-detail.test.tsx` | 変更 | サニタイズ結合テスト 3 ケースを追加（既存 10 ケースは不変） |
| `web/package.json` / `web/package-lock.json` | 変更 | `dompurify` 依存追加 |

## 採用したサニタイズポリシーとバックエンド整合

バックエンド bluemonday ポリシー（`content_sanitizer.go`）の許可タグ集合と整合させた:

- **許可タグ** (`ALLOWED_TAGS`): `p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img`
  （バックエンドの許可タグと一致）
- **許可属性** (`ALLOWED_ATTR`): `href`（a）, `src` / `alt`（img）
- **許可スキーム** (`ALLOWED_URI_REGEXP`): `https` / `http` / `mailto` / `tel` と相対 URL を許可し、
  `javascript:` / `data:` などの危険スキームを無効化する
- `script` / `iframe` / `style` 要素は許可タグに含めないことで除去される
- `on*` インラインイベントハンドラ属性は許可属性に含めないことで除去される

### バックエンドとの差分（意図的・要件の「範囲維持」に準拠）

- バックエンドは a タグに `target="_blank"` / `rel="noopener noreferrer"` を自動付与し、
  相対 URL を不許可、img src を https 限定とする。フロント層では本要件の主目的
  （危険スキーム・危険要素の実行防止）を満たすことを優先し、リンクの http や相対 URL の
  描画維持を許容している。要件 3.3 は「整合する範囲に維持」であり「ポリシー単一化」は
  Out of Scope（requirements.md の Out of Scope 第 4 項）のため、許可集合を一致させた上で
  URL スキームの厳密一致までは行っていない。詳細は「確認事項」を参照。

## SSR / standalone ビルドへの配慮

DOMPurify はブラウザ DOM（`window.document`）を前提とするため、`sanitizeContentHtml` は
`window` が無い環境（SSR）では空文字列を返すガードを設けた。`item-detail.tsx` は
`"use client"` でありクライアント描画前提だが、import 経路で SSR 時に評価されても
`window` 参照で例外を出さない。`npm run build`（Next.js standalone / turbopack）が
import エラーなく成功することを確認済み。

## 追加した依存とバージョン

- `dompurify` `^3.4.5`（dependencies）
  - 追加理由: クライアント側で記事本文 HTML をサニタイズする XSS 多層防御に必要。
  - 型定義: dompurify v3 系は型定義（`dist/purify.cjs.d.ts` / `.d.mts`）を同梱するため、
    `@types/dompurify` の追加は不要（package.json には追加していない）。

## テスト結果

実行環境の Node バージョン（22.11.0）と CI 想定の Node 20 の差異により、ローカルでは
vitest / jsdom 28 の起動時に Node 22 固有の `ERR_REQUIRE_ESM`（`vite` および
`html-encoding-sniffer` → `@exodus/bytes` の ESM を CJS `require()` する経路）が発生した。
これは本 Issue の変更とは無関係の既存環境制約で、原状（クリーン `npm ci`）でも再現する。
CI（`.github/workflows/ci.yml`）は Node 20 で実行されるため影響しない。ローカル検証では
`NODE_OPTIONS=--experimental-require-module`（Node 22.12+ では既定 ON）で ESM interop を
有効化して実行した。

- **`npm test`（vitest run、全 25 ファイル）**: 178 passed / 0 failed
  - 新規 `src/lib/sanitize.test.ts`: 11 passed
  - 変更 `src/components/item-detail.test.tsx`: 13 passed（既存 10 + 新規 3。既存は不変）
- **`npm run lint`（ESLint）**: 0 errors（warning 6 件はいずれも本変更外の既存ファイル）
- **`npm run build`（next build --turbopack）**: 成功（型・import エラーなし。SSR ガード有効）

## 各 AC とテストの対応

| AC | 内容 | テスト |
|---|---|---|
| 1.1 | 描画前にクライアント側サニタイズ結果を DOM 挿入 | `item-detail.test.tsx`「記事本文にscript要素が含まれるときサニタイズされて描画されること」/ 既存「サニタイズ済みHTMLコンテンツが展開表示されること」 |
| 1.2 | 生の記事本文 HTML を直接挿入しない | `item-detail.tsx` 実装（`dangerouslySetInnerHTML` を `sanitizedContent` に差し替え）+ 上記 script 除去テスト |
| 1.3 | 空文字列のとき空のコンテンツ領域 | `sanitize.test.ts`「空文字列のとき空文字列を返すこと」/ `item-detail.test.tsx`「記事本文が空文字列のとき空のコンテンツ領域を表示すること」 |
| 1.4 | 同一入力に同一サニタイズ結果（冪等） | `sanitize.test.ts`「同一入力に対して常に同一のサニタイズ結果を返すこと」 |
| 2.1 | `<script>` 除去 | `sanitize.test.ts`「script要素が含まれるとき当該要素を除去すること」/ `item-detail.test.tsx` script 除去テスト |
| 2.2 | `<iframe>` / `<style>` 除去 | `sanitize.test.ts`「iframe要素…除去」「style要素…除去」 |
| 2.3 | `on*` 属性除去 | `sanitize.test.ts`「onerror属性…除去」「onclick属性…除去」/ `item-detail.test.tsx`「onerror属性が含まれるとき…」 |
| 2.4 | `javascript:` スキーム無効化 | `sanitize.test.ts`「aタグのhrefにjavascriptスキームが含まれるとき…無効化または除去」 |
| 3.1 | 許可タグの保持 | `sanitize.test.ts`「許可タグのみで構成されるとき…保持」 |
| 3.2 | 許可 URL のリンク・画像の保持 | `sanitize.test.ts`「https URLを持つリンク…保持」「https srcを持つ画像…保持」 |
| 3.3 | 許可タグ・属性集合をバックエンドと整合範囲で維持 | `sanitize.ts` の `ALLOWED_TAGS` / `ALLOWED_ATTR`（バックエンド許可タグと一致）+ 上記 3.1 / 3.2 テスト |
| NFR 1.1 | バックエンドサニタイズを緩和・無効化しない | `content_sanitizer.go` は未変更（差分なし） |
| NFR 1.2 | フロント側サニタイズを必須経路として常に適用 | `item-detail.tsx` の描画経路を `sanitizeContentHtml` 通過に統一 |
| NFR 2.1 | 許可タグのみのとき導入前と視覚的に等価 | 既存「サニタイズ済みHTMLコンテンツが展開表示されること」が `<strong>本文</strong>` 等の保持を検証（不変で pass） |

## 確認事項（レビュワー判断ポイント）

1. **フロントとバックエンドの URL スキーム/相対 URL ポリシーの差**:
   バックエンド bluemonday は a タグの相対 URL を不許可・img src を https 限定とするが、
   フロント層（DOMPurify）の `ALLOWED_URI_REGEXP` は http / 相対 URL も許容している。
   要件 3.3 は「整合する**範囲**に維持」、Out of Scope はポリシー単一化を除外しているため、
   許可タグ・属性集合は一致させつつ URL スキームの完全一致までは行っていない。
   バックエンドと完全一致（img https 限定・相対 URL 不許可）まで揃えるべきかは要判断。
   揃える場合はフロント側で img src の https 限定・相対 URL 排除を追加する。

2. **ローカルテスト実行環境の Node バージョン**:
   本ワークツリーで利用可能な Node は 22.11.0 のみで、vitest/jsdom 28 が Node 22 固有の
   `ERR_REQUIRE_ESM` で起動失敗する（本変更前の状態でも再現する既存制約）。CI は Node 20
   で動作するため CI green は阻害されない見込みだが、ローカルでは
   `NODE_OPTIONS=--experimental-require-module` を付与して全 178 テストの green を確認した。
   この Node バージョン差は本 Issue のスコープ外（toolchain 設定変更は行っていない）。

STATUS: complete
