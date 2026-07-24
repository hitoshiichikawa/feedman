# Requirements Document

## Introduction

Feedman の CI（`.github/workflows/ci.yml`）の `Frontend Dependency Vulnerability Scan
(npm audit)` ジョブが、`web/` 配下の依存に対する新規公開アドバイザリ群（high 6 件を含む
計 12 件）の検出により fail している状態である。主な検出は Next.js（Server Actions DoS /
SSRF / レスポンスキャッシュ混同ほか）、js-yaml（merge-key チェーンによる quadratic CPU）、
および Next.js の間接依存として連鎖検出される postcss / sharp。この赤は develop 宛ての
全 PR に継承され、脆弱性対応と無関係な PR の CI までブロックしている。本要件は、`web/` の
依存を semver 互換の範囲で更新することにより `frontend-audit` ジョブを green に戻し、
Web アプリの既存挙動（テスト・lint・ビルド）を維持することを目的とする。バックエンド側
（govulncheck）の脆弱性対応は本 Issue のスコープ外（別 Issue #218 で対応）。

## Requirements

### Requirement 1: npm audit high 以上検出の解消

**Objective:** As a リポジトリのメンテナ, I want `web/` 配下の high 以上の npm 脆弱性検出を解消すること, so that PR の CI が脆弱性対応と無関係な要因でブロックされない状態に戻る

#### Acceptance Criteria

1. When 対応後の `web/package-lock.json` を用いて `web/` 配下で `npm ci` に続けて `npm audit --audit-level=high` を実行したとき, the Frontend Dependency Vulnerability Scan shall 終了コード 0 で完了する
2. When 対応後の `web/package-lock.json` を用いて `web/` 配下で `npm audit --audit-level=high` を実行したとき, the Frontend Dependency Vulnerability Scan shall high 以上（high または critical）の脆弱性を 0 件として報告する
3. When 対応後のブランチに対して CI がトリガーされたとき, the `Frontend Dependency Vulnerability Scan (npm audit)` ジョブ shall 成功（green）ステータスとなる
4. The 本対応 shall Issue 本文で特定された既知の high 以上アドバイザリ（Next.js の Server Actions DoS / SSRF / レスポンスキャッシュ混同、js-yaml の merge-key quadratic CPU、および Next.js 間接依存として連鎖検出される postcss / sharp のうち high 以上のもの）を解消する

### Requirement 2: 既存挙動・既存テストの維持

**Objective:** As a リポジトリのメンテナ, I want 依存更新後も Web アプリの既存挙動と CI の他ジョブが従来どおり通ること, so that 脆弱性対応が回帰不具合を持ち込まない

#### Acceptance Criteria

1. When 対応後のブランチで `web/` 配下の `npm test`（vitest）が実行されたとき, the Frontend Tests ジョブ shall 全テストを pass して終了コード 0 で完了する
2. When 対応後のブランチで `web/` 配下の `npm run lint`（eslint）が実行されたとき, the Frontend Lint ジョブ shall lint 違反 0 件で終了コード 0 で完了する
3. When 対応後のブランチで `web/` 配下の `npm run build` が実行されたとき, the Web ビルド shall エラーなく成功する
4. The バックエンド側 CI ジョブ（`Backend Tests` / `Go Static Analysis (go vet)` / `Go Vulnerability Scan (govulncheck)`） shall 本対応前と同一の成否判定を維持する
5. The Web アプリのユーザー可視挙動（フィード購読・記事一覧・記事詳細・認証等の既存ユースケース） shall 本対応前と同一の挙動を維持する

### Requirement 3: 更新方針の制約（semver 互換の範囲に限定）

**Objective:** As a リポジトリのメンテナ, I want 脆弱性解消のための依存更新が semver 互換の範囲に限定されること, so that major 更新に伴う予期せぬ破壊的変更を回避しつつ最小変更で赤を解消できる

#### Acceptance Criteria

1. The 本対応での依存バージョン更新 shall 各パッケージの現行 major バージョンを維持する（現行 major と異なる major への更新を含まない）
2. The 本対応 shall Next.js の major バージョンを現行の `15.x` から他 major（例: `16.x` など）へ更新しない
3. The 本対応 shall Next.js の App Router 構成（`web/src/app/` 配下のルーティング構造）を変更しない
4. If ある脆弱性の解消が現行 major 内のパッチ／マイナー更新のみでは達成できず major 更新を要すると判明した場合, the 本対応 shall 当該脆弱性を本 Issue のスコープから除外し、`Open Questions` または後続 Issue への切り出しとして明示する
5. The 本対応 shall 脆弱性解消に不要な依存パッケージの追加、削除、またはバージョン更新を含まない

### Requirement 4: 変更ファイル範囲の限定

**Objective:** As a リポジトリのメンテナ, I want 本対応の変更範囲がフロントエンド依存メタデータに限定されること, so that レビュー対象が最小化され副作用の混入リスクが下がる

#### Acceptance Criteria

1. The 本対応の diff shall `web/package.json` および `web/package-lock.json` を主たる変更対象とする
2. The 本対応 shall アプリケーションソースコード（`web/src/**`）を機能変更目的で変更しない
3. If 依存更新に伴い型定義変更や API 互換性の都合でソースコードの軽微な追随修正が必要となった場合, the 本対応 shall 当該修正の範囲と理由を PR 本文で明示する
4. The 本対応 shall バックエンド（`api` / `worker` / `internal/**` / `go.mod` / `go.sum`）およびインフラ設定（`Dockerfile` / `docker-compose*.yml` / `.github/workflows/**`）を変更しない

## Non-Functional Requirements

### NFR 1: 対応の再現性

1. The 本対応 shall CI 環境（`npm ci` による fresh install）で決定論的に再現可能な `web/package-lock.json` を成果物として含める（同一 lockfile に対して `npm ci` すれば同一ツリーが得られる状態）
2. Where ローカル環境の `web/node_modules/` に Docker 由来の root 所有ファイルなどが混在し `npm audit fix` の reify が失敗する制約に遭遇した場合, the 本対応 shall `--package-lock-only` 方式など lockfile のみを更新する手段を許容する（CI 側は `npm ci` で fresh install するため十分と扱う）

### NFR 2: 検出の可視性

1. When 本対応後に CI の `Frontend Dependency Vulnerability Scan (npm audit)` ジョブが実行されたとき, the 当該ジョブ shall `npm audit` の実行結果（検出件数のサマリー）を CI ステップログに出力する

## Out of Scope

- バックエンド側の脆弱性対応（`govulncheck` の赤の解消。Issue #218 で対応）
- Next.js の major バージョンアップ（例: `15.x` → `16.x`）および App Router 構成の変更
- 脆弱性解消に不要な依存の追加・削除・更新（機能拡張や UI ライブラリの入れ替え等）
- moderate / low / info レベルの脆弱性の解消（本対応の合否判定は `--audit-level=high` に一致させる）
- npm audit の allowlist（`.audit-ignore` 等）機構の導入
- CI ワークフロー（`.github/workflows/ci.yml`）のジョブ構成・閾値・トリガーの変更
- Docker イメージスキャン（Trivy）や eslint ルールセット自体の変更
- 「semver 互換範囲では解消不能」と判明した個別脆弱性の major 更新による解消（Req 3.4 に従い後続 Issue へ切り出す）

## Open Questions

- なし（Issue 本文にて「semver 互換 bump のみ」「Next.js major 更新禁止」「変更範囲は `web/package.json` / `web/package-lock.json` のみ」「`--package-lock-only` 方式の許容」が明示済み。追加コメントは存在しない）

## 関連

- Sibling: #218
