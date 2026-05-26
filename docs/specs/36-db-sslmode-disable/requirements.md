# Requirements Document

## Introduction

Feedman の DB 接続文字列（`DATABASE_URL`）は、`.env.sample` と `docker-compose.yml`（api / worker 両サービス）で
`sslmode=disable` がハードコードされている。サンプルをそのまま本番にコピーして外部 PostgreSQL へ接続すると、
デフォルトで TLS 無効（平文通信）となり、中間者攻撃（MITM）によって DB 認証情報やユーザーデータが漏洩する導線になる。
本要件は、開発時（docker-compose のコンテナ内 DB `db` ホスト）では従来どおり無設定で起動できる後方互換を保ちつつ、
サンプル設定が本番の平文通信導線にならないよう、設定ファイルとドキュメントを通じて利用者に sslmode の明示的選択を促すことを目的とする。
本 Issue のスコープは設定ファイル（`docker-compose.yml` / `.env.sample`）とドキュメント（`README.md`）の修正に限定し、
Go アプリケーションコードの変更や起動時警告ガードは扱わない（別 Issue へ分離済み）。

## Requirements

### Requirement 1: サンプル接続文字列のデフォルト平文通信の解消

**Objective:** As a 本番運用者, I want `.env.sample` をそのまま本番にコピーしてもデフォルトで平文 DB 通信にならないこと, so that 設定のコピーミスによる DB 認証情報・データの平文漏洩リスクを避けられる

#### Acceptance Criteria

1. When 運用者が `.env.sample` をそのままコピーして外部 PostgreSQL に接続したとき, the サンプル設定 shall デフォルトで sslmode=disable による平文通信を強制しない（`?sslmode=disable` の固定指定を `.env.sample` の `DATABASE_URL` に残さない）。
2. The `.env.sample` の `DATABASE_URL` 行 shall コンテナ内 DB 利用時のみ disable が許容される旨と、外部 DB 接続時は require 以上の sslmode を選択する必要がある旨をコメントで明示する。
3. The `.env.sample` shall sslmode を接続先に応じて利用者が明示的に選択する形式であることを、コメントまたは設定値の構造から読み取れるようにする。

### Requirement 2: docker-compose のコンテナ内 DB 開発起動の後方互換維持

**Objective:** As a 開発者, I want docker-compose のコンテナ内 DB（`db` ホスト）を使う開発時に追加設定なしで従来どおり起動できること, so that 既存の開発フローを壊さずにセキュリティ修正を取り込める

#### Acceptance Criteria

1. When 開発者が追加の環境変数指定なしで docker-compose を起動したとき, the api サービス shall コンテナ内 DB（`db` ホスト）へ接続して従来どおり起動できる。
2. When 開発者が追加の環境変数指定なしで docker-compose を起動したとき, the worker サービス shall コンテナ内 DB（`db` ホスト）へ接続して従来どおり起動できる。
3. Where 外部 PostgreSQL を利用する運用構成が選択されたとき, the docker-compose 設定 shall api サービス・worker サービスの両方で `DATABASE_URL` を環境別に差し替え可能とする。
4. When 外部 DB 接続用に `DATABASE_URL` を上書きせず docker-compose を起動したとき, the docker-compose 設定 shall コンテナ内 DB 向けのデフォルト接続文字列を適用する。

### Requirement 3: 外部 DB 接続時の sslmode 要件のドキュメント化

**Objective:** As a 本番運用者, I want 外部 DB 接続時に require 以上の sslmode が必要であることがドキュメントから読み取れること, so that 本番デプロイ時に正しい TLS 設定で DB 接続を構成できる

#### Acceptance Criteria

1. The README shall コンテナ内 DB（`db` ホスト）利用時のみ sslmode=disable が許容される旨を明記する。
2. The README shall 外部 PostgreSQL 接続時は sslmode に require 以上を必須とする旨を明記する。
3. Where 本番デプロイ手順が記載されている箇所において, the README shall `DATABASE_URL` の sslmode を接続先に応じて設定する手順を本番デプロイ時の注意事項として含める。
4. If 運用者が README のサンプルコマンドや設定例を参照したとき, the README shall 外部 DB 接続例で sslmode=disable を平文接続のまま推奨するような記述を残さない。

## Non-Functional Requirements

### NFR 1: 後方互換性

1. While コンテナ内 DB（`db` ホスト）を用いた既存の開発用 docker-compose 起動を行っている間, the docker-compose 設定 shall 本変更前と同一の起動結果（コンテナ内 DB への接続成立）を維持する。
2. The 本変更 shall Go アプリケーションコード・DB スキーマ・マイグレーションに変更を加えずに完結する。

### NFR 2: 設定とドキュメントの整合性

1. The `.env.sample` / `docker-compose.yml` / `README.md` の DB 接続に関する sslmode の記述 shall 相互に矛盾しない一貫した内容とする。

## Out of Scope

- 起動時警告ガード（sslmode=disable かつ DB ホストが localhost/db 以外の場合に警告ログを出す仮案）。Issue コメントで確定した Option B に従い、別 Issue へ分離する。
- Go アプリケーションコード（`internal/config/` / `internal/database/` 等）の DB ドライバ・接続実装の変更およびそれに伴うテスト追加。
- DB マイグレーション・スキーマ変更。
- PostgreSQL サーバ側の TLS 証明書配置・TLS 終端構成・リバースプロキシ設定など、Feedman リポジトリ外のインフラ構成。

## Open Questions

- なし（Issue コメントで Option B が確定済み。スコープは設定・ドキュメント修正のみ）。
