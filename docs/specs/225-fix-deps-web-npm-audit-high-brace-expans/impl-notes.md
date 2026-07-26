# 実装ノート #225: `web/` npm audit high（brace-expansion / postcss）解消

## 変更概要

`web/` 配下の high 以上の npm 脆弱性検出を semver 互換の範囲および既存 `overrides` 節への
追記により解消した。

### 変更ファイル

- `web/package.json` — `overrides` 節に `"brace-expansion": "^5.0.8"` を 1 行追記
- `web/package-lock.json` — 上記 override と `npm audit fix`（非 force） による semver 互換の
  依存更新を反映

アプリケーションソースコード（`web/src/**`）およびバックエンド／インフラ設定は変更なし
（Req 4.1〜4.4）。

## 対応方針の詳細

### brace-expansion GHSA-mh99-v99m-4gvg の解消

- 対応前の lockfile では `node_modules/brace-expansion` が **1.1.16** で解決されており、
  npm advisory の vulnerable range `<=5.0.7` に該当していた（GHSA-mh99-v99m-4gvg は
  brace-expansion の semver range を `<=5.0.7` として npm 側で記述しており、1.x
  maintenance-v1 の 1.1.16 も範囲に含まれる）
- semver 互換の lockfile 更新（`npm audit fix`）では 1.x 系のまま解消できず、high の
  reject 対象だった
- そこで `web/package.json` の既存 `overrides` 節に **`"brace-expansion": "^5.0.8"`** を
  追記し、全チェーンで brace-expansion 5.0.8 を強制することで解消した（Req 3.4 に基づく
  `overrides` 追記方式）
- brace-expansion 5.x は `type: "module"` だが `"main": "./dist/commonjs/index.js"` と
  CommonJS デュアルエクスポートを備えているため、`^1.1.7` を要求する minimatch 3.x など
  1.x consumer（CommonJS `require()` 呼び出し）からも読み込める。API はシンプルな
  `expand(string) → string[]` で、1.x〜5.x で互換
- 検証: `npm ci` 後の `npm run lint` が 0 error（既存 warning 5 件は据え置き）、
  `npm run build` 成功、`npm test`（vitest）407 テスト全 pass。eslint / minimatch を
  介するツールチェーンが実行時エラーなしで動作することを確認

### postcss GHSA-r28c-9q8g-f849 の解消

- `npm audit fix`（非 force） による semver 互換の lockfile 更新で解消
- `node_modules/postcss` が 8.5.23（top-level）、`node_modules/next/node_modules/postcss` が
  8.5.22（既存 override `next.postcss ^8.5.22`）に更新され、いずれも patched バージョン
  （>8.5.17）
- 併せて `@eslint/config-array`、`@eslint/eslintrc`、`@eslint/js`、`eslint`、
  `eslint-plugin-*` など eslint チェーンが patch レベルで自動更新されている（いずれも
  major 変更なし。Req 3.2）

## Audit 結果 before / after

### before（対応前 / develop HEAD）

```
14 vulnerabilities (1 low, 3 moderate, 10 high)
- brace-expansion <=5.0.7 (high) — minimatch → eslint chain で計 9 件
- postcss <=8.5.17 (high) — 1 件
- @hono/node-server <2.0.5 (moderate) — shadcn CLI chain 3 件（Out of Scope）
- esbuild 0.27.3-0.28.0 (moderate) — 1 件（Out of Scope）
- （low 1 件）
```

### after（対応後）

```
4 vulnerabilities (1 low, 3 moderate)
- @hono/node-server <2.0.5 (moderate) — 3 件（Out of Scope）
- esbuild 0.27.3-0.28.0 (moderate) — 1 件（Out of Scope）

npm audit --audit-level=high の終了コード: 0
高severity（high / critical）件数: 0
```

## 実行コマンドと結果

すべて `web/` ディレクトリで実行:

- `npm audit --audit-level=high` — exit 0, high: 0 / critical: 0 / moderate: 3 / low: 1
- `npm ci` — 793 packages 追加、4 vulnerabilities（high 0 件）
- `npm run lint` — 0 error, 5 warnings（対応前と同一の既存 warning）
- `npm test` — 42 test files, 407 tests 全 pass, duration 58.20s
- `npm run build` — 成功（Route: 2 static pages, First Load JS 197KB / 125KB）

## 受入基準の達成確認

- **Req 1.1**: `npm ci` → `npm audit --audit-level=high` で終了コード 0 を確認 ✓
- **Req 1.2**: high / critical 0 件を確認 ✓
- **Req 1.3**: CI での実行は PR 作成後に確認予定（本 impl でローカル同等コマンドの成功を確認済み）
- **Req 1.4**: brace-expansion GHSA-mh99-v99m-4gvg / postcss GHSA-r28c-9q8g-f849 の両方を解消 ✓
- **Req 2.1**: `npm test`（vitest）407 テスト全 pass ✓
- **Req 2.2**: `npm run lint`（eslint）0 error ✓
- **Req 2.3**: `npm run build` 成功 ✓
- **Req 2.4**: eslint / eslint-config-next / @eslint/eslintrc / eslint-plugin-* の major 変更なし。
  lint 実行時エラーなし、ルールセット消失なし ✓
- **Req 2.5**: バックエンド側 CI ジョブは変更対象外（`api` / `worker` / `internal/**` /
  `go.mod` / `go.sum` に変更なし） ✓
- **Req 2.6**: `web/src/**` 変更なしのため既存ユーザー可視挙動に影響なし ✓
- **Req 3.1**: `npm audit fix --force` 未使用（非 force の `npm audit fix` および `overrides`
  追記のみ） ✓
- **Req 3.2**: eslint 9.x、eslint-config-next 15.5.12、@eslint/eslintrc 3.x（対応前と同一
  major） ✓
- **Req 3.3**: 本番 dependencies（`web/package.json` の `dependencies` セクション）に
  変更なし ✓
- **Req 3.4**: brace-expansion は `overrides` への `"brace-expansion": "^5.0.8"` 追記で対応 ✓
- **Req 3.5**: 依存パッケージの追加・削除・脆弱性解消と無関係なバージョン更新は含まれない
  （semver 互換の patch 更新および override 追記のみ） ✓
- **Req 3.6**: Req 3.1〜3.5 の制約範囲内で全 high 脆弱性を解消できたため、後続 Issue への
  切り出しなし ✓
- **Req 4.1**: 変更は `web/package.json` / `web/package-lock.json` のみ ✓
- **Req 4.2**: `web/src/**` 変更なし ✓
- **Req 4.3**: ソースコードの追随修正なし（該当せず） ✓
- **Req 4.4**: バックエンド／インフラ設定変更なし ✓
- **NFR 1.1**: `npm ci` で決定論的に再現可能な lockfile を成果物として含む ✓
- **NFR 1.2**: 今回は `--package-lock-only` は不要だった（`npm ci` 相当が通常実行できたため）。
  `npm install --package-lock-only` で override 反映後 `npm ci` で clean install も動作確認済み ✓
- **NFR 2.1**: CI ステップログへの出力は既存 workflow の挙動に依存（本 impl での変更対象外）

## 判断事項

### なぜ brace-expansion を 5.0.8 に全チェーン強制したか

Issue 本文にある通り、brace-expansion 1.x（1.1.16 含む）は npm advisory の vulnerable
range `<=5.0.7` に含まれ、`npm audit fix`（非 force） では 1.x のまま解消できない。
options:

- (A) 全チェーンで 5.0.8 に override → 1 行追記で完結、5.x は CommonJS 互換なので既存 1.x
  consumer（minimatch 3.x など）から `require()` 可能
- (B) 特定パスのみ override（`"minimatch": {"brace-expansion": "^5.0.8"}` 等） → 記述が
  冗長になり、将来別の consumer が増えたときの追随忘れリスク
- (C) `npm audit fix --force` → `@eslint/eslintrc@0.1.0` への降格が発生し Req 3.1 / 3.2 に
  違反

(A) を採用。lint / test / build がすべて通過し、eslint ツールチェーンの実行時挙動に影響が
ないことを確認済み。

### なぜ postcss は `npm audit fix` のみで解消したか

既存 `overrides.next.postcss ^8.5.22` が PR #221 で設定済み。`npm audit fix`（非 force）を
実行すると `@eslint/eslintrc` の間接依存として拾われる postcss および依存する eslint 系
package が semver 互換で patch 更新され、結果的に postcss が 8.5.22 / 8.5.23 に更新された。
既存 override 側で明示的なバージョン変更は不要だった。

## 確認事項（レビュワー向け）

- brace-expansion を全チェーンで 5.x に強制する override は将来的な eslint / minimatch の
  更新で回帰する可能性があります。次回 npm audit で新規 GHSA が公開されたら、
  overrides を見直して 5.x が引き続き最新の non-vulnerable range に含まれることを確認して
  ください
- Out of Scope の残存 moderate（`@hono/node-server` / `esbuild`）は本 Issue のスコープ外
  ですが、shadcn CLI 側の major 更新（`shadcn 3.8.3` への降格を含む breaking change）が
  必要になるため、別 Issue として起票済みか要確認です

## Feature Flag Protocol

対象 repo `CLAUDE.md` の `## Feature Flag Protocol` は `**採否**: opt-out` 宣言。通常フローで
実装した（flag 導入なし）。

STATUS: complete
