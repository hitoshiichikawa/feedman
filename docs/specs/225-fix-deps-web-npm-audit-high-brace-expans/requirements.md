# Requirements Document

## Introduction

Feedman の CI（`.github/workflows/ci.yml`）の `Frontend Dependency Vulnerability Scan
(npm audit)` ジョブが、`web/` 配下の依存に対する新規公開アドバイザリ群により fail している
状態である（high 10 件を含む計 14 件）。high の実体は 2 系統で、(a) `brace-expansion` の
DoS 系（GHSA-mh99-v99m-4gvg。`minimatch` → `@eslint/config-array` / `@eslint/eslintrc` /
`eslint-plugin-import` / `eslint-plugin-jsx-a11y` / `eslint-plugin-react` → `eslint` →
`eslint-config-next` の devDependencies 連鎖で計上され high の大半を占める）、(b) `postcss`
の sourceMappingURL パストラバーサル系（GHSA-r28c-9q8g-f849）である。この赤は develop 宛て
の全 PR に継承され、脆弱性対応と無関係な PR（例: 設計 spec のみの PR #224）の CI までブロック
している。本 Issue は同型対応の 3 回目（#210 → #219 に続く再発）であり、`web/` の依存を
semver 互換の範囲もしくは既存 `overrides` 節への追記により更新することで `frontend-audit`
ジョブを green に戻し、Web アプリの既存挙動（テスト・lint・ビルド）と eslint ツールチェーンの
動作を維持することを目的とする。

## Requirements

### Requirement 1: npm audit high 以上検出の解消

**Objective:** As a リポジトリのメンテナ, I want `web/` 配下の high 以上の npm 脆弱性検出を解消すること, so that PR の CI が脆弱性対応と無関係な要因でブロックされない状態に戻る

#### Acceptance Criteria

1. When 対応後の `web/package-lock.json` を用いて `web/` 配下で `npm ci` に続けて `npm audit --audit-level=high` を実行したとき, the Frontend Dependency Vulnerability Scan shall 終了コード 0 で完了する
2. When 対応後の `web/package-lock.json` を用いて `web/` 配下で `npm audit --audit-level=high` を実行したとき, the Frontend Dependency Vulnerability Scan shall high 以上（high または critical）の脆弱性を 0 件として報告する
3. When 対応後のブランチに対して CI がトリガーされたとき, the `Frontend Dependency Vulnerability Scan (npm audit)` ジョブ shall 成功（green）ステータスとなる
4. The 本対応 shall Issue 本文で特定された既知の high 以上アドバイザリ（`brace-expansion` <=5.0.7 の DoS 系 GHSA-mh99-v99m-4gvg、および `postcss` <=8.5.17 の sourceMappingURL パストラバーサル系 GHSA-r28c-9q8g-f849）の両方を解消する

### Requirement 2: 既存挙動・既存テストの維持

**Objective:** As a リポジトリのメンテナ, I want 依存更新後も Web アプリの既存挙動と CI の他ジョブが従来どおり通ること, so that 脆弱性対応が回帰不具合を持ち込まない

#### Acceptance Criteria

1. When 対応後のブランチで `web/` 配下の `npm test`（vitest）が実行されたとき, the Frontend Tests ジョブ shall 全テストを pass して終了コード 0 で完了する
2. When 対応後のブランチで `web/` 配下の `npm run lint`（eslint）が実行されたとき, the Frontend Lint ジョブ shall lint 違反 0 件で終了コード 0 で完了する
3. When 対応後のブランチで `web/` 配下の `npm run build` が実行されたとき, the Web ビルド shall エラーなく成功する
4. The eslint ツールチェーン（`eslint` / `eslint-config-next` / `@eslint/eslintrc` および連鎖する eslint-plugin 群） shall 対応後も従来どおり実行可能な状態を維持する（実行時エラー・ルールセット消失を起こさない）
5. The バックエンド側 CI ジョブ（`Backend Tests` / `Go Static Analysis (go vet)` / `Go Vulnerability Scan (govulncheck)`） shall 本対応前と同一の成否判定を維持する
6. The Web アプリのユーザー可視挙動（フィード購読・記事一覧・記事詳細・認証等の既存ユースケース） shall 本対応前と同一の挙動を維持する

### Requirement 3: 更新方針の制約（破壊的変更の禁止）

**Objective:** As a リポジトリのメンテナ, I want 脆弱性解消のための依存更新が破壊的変更を含まないこと, so that lint ツールチェーンや本番挙動を壊さずに最小変更で赤を解消できる

#### Acceptance Criteria

1. The 本対応 shall `npm audit fix --force` による破壊的ダウングレード（例: `@eslint/eslintrc@0.1.0` への降格）を行わない
2. The 本対応 shall `eslint` / `eslint-config-next` / `@eslint/eslintrc` の major バージョンをアップグレードもダウングレードもしない
3. The 本対応 shall 本番 dependencies（`web/package.json` の `dependencies` セクション） の各パッケージに破壊的変更（既存 API 挙動の変化を伴う更新）を持ち込まない
4. Where `brace-expansion` の semver 互換 fix が lockfile のみの更新で取れない場合, the 本対応 shall `web/package.json` の既存 `overrides` 節に `brace-expansion` の修正版を追記する方式を許容する（`overrides` 追記後は `npm run lint` が実行可能であることを確認する）
5. The 本対応 shall 脆弱性解消に不要な依存パッケージの追加、削除、またはバージョン更新を含まない
6. If ある脆弱性の解消が上記制約（Req 3.1〜3.5）の範囲内で達成できないと判明した場合, the 本対応 shall 当該脆弱性を本 Issue のスコープから除外し、`Open Questions` または後続 Issue への切り出しとして明示する

### Requirement 4: 変更ファイル範囲の限定

**Objective:** As a リポジトリのメンテナ, I want 本対応の変更範囲がフロントエンド依存メタデータに限定されること, so that レビュー対象が最小化され副作用の混入リスクが下がる

#### Acceptance Criteria

1. The 本対応の diff shall `web/package.json` および `web/package-lock.json` を主たる変更対象とする
2. The 本対応 shall アプリケーションソースコード（`web/src/**`） を機能変更目的で変更しない
3. If 依存更新に伴い型定義変更や API 互換性の都合でソースコードの軽微な追随修正が必要となった場合, the 本対応 shall 当該修正の範囲と理由を PR 本文で明示する
4. The 本対応 shall バックエンド（`api` / `worker` / `internal/**` / `go.mod` / `go.sum`）およびインフラ設定（`Dockerfile` / `docker-compose*.yml` / `.github/workflows/**`） を変更しない

## Non-Functional Requirements

### NFR 1: 対応の再現性

1. The 本対応 shall CI 環境（`npm ci` による fresh install）で決定論的に再現可能な `web/package-lock.json` を成果物として含める（同一 lockfile に対して `npm ci` すれば同一ツリーが得られる状態）
2. Where ローカル環境の `web/node_modules/` に Docker 由来の root 所有ファイルなどが混在し `npm audit fix` の reify が失敗する制約に遭遇した場合, the 本対応 shall `--package-lock-only` 方式など lockfile のみを更新する手段を許容する（CI 側は `npm ci` で fresh install するため十分と扱う）

### NFR 2: 検出の可視性

1. When 本対応後に CI の `Frontend Dependency Vulnerability Scan (npm audit)` ジョブが実行されたとき, the 当該ジョブ shall `npm audit` の実行結果（検出件数のサマリー）を CI ステップログに出力する

## Out of Scope

- moderate / low / info レベルの脆弱性のみの解消（`@hono/node-server`（shadcn CLI 経由）や `esbuild` GHSA-g7r4-m6w7-qqqr など。本対応の合否判定は `--audit-level=high` に一致させる）
- eslint 体系の major 更新・設定刷新（`eslint` / `eslint-config-next` / `@eslint/eslintrc`）
- shadcn CLI の major 更新
- Next.js の major 更新および App Router 構成の変更
- バックエンド側の脆弱性対応（`govulncheck` の対応）
- 脆弱性解消に不要な依存の追加・削除・更新（機能拡張や UI ライブラリの入れ替え等）
- npm audit の allowlist（`.audit-ignore` 等）機構の導入
- CI ワークフロー（`.github/workflows/ci.yml`）のジョブ構成・閾値・トリガーの変更
- Docker イメージスキャン（Trivy）や eslint ルールセット自体の変更
- 「Req 3 の制約範囲では解消不能」と判明した個別脆弱性の破壊的手段による解消（Req 3.6 に従い後続 Issue へ切り出す）

## Open Questions

- なし（Issue 本文にて「high 実体の 2 系統特定」「`npm audit fix --force` 禁止」「eslint 系 major 変更禁止」「本番 deps に breaking change 禁止」「変更範囲は `web/package.json` / `web/package-lock.json` のみ」「`overrides` 追記方式の許容」「`--package-lock-only` 方式の許容」がすべて明示済み。人間による追加の決定事項コメントもなし）

## 関連

- Related: #219
- Related: #210
