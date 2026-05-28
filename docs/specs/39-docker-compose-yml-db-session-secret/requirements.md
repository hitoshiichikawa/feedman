# Requirements Document

## Introduction

`docker-compose.yml` では DB 認証情報（`POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`）と
worker の `SESSION_SECRET` に弱い既知のデフォルト値（`feedman` / `dummy`）が設定されており、
`.env` 未設定でも `docker compose up` が成功してしまう。その結果、弱いパスワード・推測可能な
セッション暗号化キーのまま本番投入される導線が残っている。本要件はこの導線を断ち、秘匿情報
（`POSTGRES_PASSWORD` / `SESSION_SECRET`）が未設定のまま起動しようとした際に fail-fast させ、
かつ開発者が安全な値を生成・設定する手順をドキュメントから辿れる状態を定義する。人間の決定
（Option B）に従い、単一の `docker-compose.yml` を `.env` 必須化に統一し、開発環境向けの別
compose ファイル分離は採用しない。

## Requirements

### Requirement 1: 秘匿情報未設定時の起動 fail-fast

**Objective:** As a 運用者, I want 秘匿情報が未設定のまま起動しようとしたときに即座に失敗してほしい, so that 弱い既知のデフォルト値のまま本番環境にデプロイされる事故を防げる

#### Acceptance Criteria

1. If `POSTGRES_PASSWORD` が未設定または空文字のまま `docker compose up` を実行した場合, the Compose 構成 shall コンテナを起動せず処理を失敗終了させる
2. If `SESSION_SECRET` が未設定または空文字のまま `docker compose up` を実行した場合, the Compose 構成 shall コンテナを起動せず処理を失敗終了させる
3. If `POSTGRES_PASSWORD` が未設定または空文字のまま `docker compose config` を実行した場合, the Compose 構成 shall 設定の解決を失敗させ非ゼロの終了コードを返す
4. If `SESSION_SECRET` が未設定または空文字のまま `docker compose config` を実行した場合, the Compose 構成 shall 設定の解決を失敗させ非ゼロの終了コードを返す
5. If 秘匿情報の未設定により起動または設定解決が失敗した場合, the Compose 構成 shall どの環境変数の設定が必要かを判別できるエラーメッセージを出力する

### Requirement 2: 弱い既知のデフォルト値の排除

**Objective:** As a 運用者, I want 弱い既知のデフォルト値で起動できる導線をなくしたい, so that 設定漏れによって推測可能な秘匿情報が本番で使われることがなくなる

#### Acceptance Criteria

1. The Compose 構成 shall `SESSION_SECRET` に対する弱い既知のデフォルト値（`dummy`）を持たない
2. The Compose 構成 shall `POSTGRES_PASSWORD` に対する弱い既知のデフォルト値（`feedman`）を持たない
3. The Compose 構成 shall DB 接続文字列（`DATABASE_URL`）のデフォルト値に弱い既知のパスワード値が埋め込まれない構成とする
4. When `POSTGRES_USER` または `POSTGRES_DB` が未設定の場合, the Compose 構成 shall 起動を妨げず従来どおりのデフォルト値（`feedman`）を適用する

### Requirement 3: 必要な秘匿情報設定時の正常起動

**Objective:** As a 開発者, I want 必要な秘匿情報を設定すれば従来どおり起動できる状態を保ちたい, so that fail-fast 化によってローカル起動の利便性が損なわれない

#### Acceptance Criteria

1. When `POSTGRES_PASSWORD` と `SESSION_SECRET` の双方に値が設定された状態で `docker compose up` を実行した場合, the Compose 構成 shall 4 つのコンテナ（web / api / worker / db）を従来どおり起動する
2. When `POSTGRES_PASSWORD` と `SESSION_SECRET` の双方に値が設定された状態で `docker compose config` を実行した場合, the Compose 構成 shall 設定を正常に解決しゼロの終了コードを返す
3. The Compose 構成 shall 既存の環境変数名（`POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` / `SESSION_SECRET` / `DATABASE_URL`）を変更せず後方互換を維持する

### Requirement 4: 安全な秘匿情報生成手順のドキュメント化

**Objective:** As a 開発者, I want 安全な秘匿情報の生成・設定手順をドキュメントから辿れるようにしたい, so that fail-fast に遭遇しても自力で正しい値を設定して起動できる

#### Acceptance Criteria

1. The `.env.sample` shall `SESSION_SECRET` の安全な値を生成するコマンド例（`openssl rand -base64 32`）を提示する
2. The `.env.sample` shall `POSTGRES_PASSWORD` の安全な値を生成するコマンド例（`openssl rand -base64 32` 相当）を提示する
3. The セットアップドキュメント（README）shall `POSTGRES_PASSWORD` と `SESSION_SECRET` が起動に必須であることを明示する
4. The セットアップドキュメント（README）shall 秘匿情報未設定時に起動が失敗する旨と、その回避手順（安全な値の生成・設定）を辿れる記述を持つ

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The 変更後の Compose 構成 shall 既存の環境変数名（`POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` / `SESSION_SECRET` / `DATABASE_URL`）を 1 つも改名しない
2. While 必要な秘匿情報がすべて設定された状態では, the 変更後の Compose 構成 shall 本変更導入前と同一のサービス構成・ネットワーク構成・ポート公開で起動する

### NFR 2: 開発者の起動容易性

1. The セットアップ手順 shall 開発者が `.env`（実運用では `.env.production`）を 1 回セットアップするだけで起動できる導線を維持する
2. The 起動失敗時のエラーメッセージ shall 設定すべき環境変数名を開発者が追加の調査なしに特定できる粒度で示す

### NFR 3: セキュリティ（機密情報の非露出）

1. The Compose 構成およびドキュメント shall 実値の秘匿情報（`POSTGRES_PASSWORD` / `SESSION_SECRET`）をリポジトリにコミットしない
2. The 起動失敗時のエラーメッセージ shall 設定済みの秘匿情報の値そのものを出力しない

## Out of Scope

- 開発環境向け compose ファイル分離（`docker-compose.dev.yml` の新規作成）。人間の決定（Option B）により単一 `docker-compose.yml` の必須化に統一する
- DB 認証情報・`SESSION_SECRET` のローテーション機構
- Secrets 管理基盤（HashiCorp Vault / AWS Secrets Manager 等）への移行・連携
- `POSTGRES_USER` / `POSTGRES_DB` の必須化（秘匿情報ではなく弱いデフォルトでも実害が小さいため、必須化対象に含めない）
- 既存の `CORS_ALLOWED_ORIGIN` 等、本 Issue と無関係な環境変数の整理（別 Issue の領分）
- アプリケーションコード（Go）側での `SESSION_SECRET` 長さ・強度バリデーションの追加（本要件は Compose 起動段階の fail-fast に限定する）

## Open Questions

- なし（compose ファイル分離 vs `.env` 必須統一の論点は人間が Option B を確定済み。必須化対象は秘匿情報である `POSTGRES_PASSWORD` / `SESSION_SECRET` の 2 つに確定）
