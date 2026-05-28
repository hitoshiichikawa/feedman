# Feedman

Web ベースの RSS/Atom フィードリーダー。Google OAuth 認証、2ペイン UI、バックグラウンドフェッチ、はてなブックマーク連携を備える。

## アーキテクチャ

ブラウザは **単一オリジン**（`web` のオリジンのみ）にアクセスします。`/api/*`・`/auth/*` への
リクエストは `web` の Next.js rewrites が **同一オリジン経由で server-side proxy** し、内部
ネットワーク経由で `api` へ転送します。ブラウザは `api` のオリジンや内部 URL（`API_INTERNAL_URL`）を
一切認識しません。

```
ブラウザ（単一オリジン https://<host> のみ認識）
  │  HTML/JS/CSS, /api/*, /auth/*
  ▼
┌──────────────────────────────────┐
│  web (Next.js standalone, :3000) │  ← Docker コンテナ
│  server.js + rewrites() proxy    │
└──────────────────────────────────┘
        │ /api/:path*  → API_INTERNAL_URL/api/:path*
        │ /auth/:path* → API_INTERNAL_URL/auth/:path*  （internal ネットワーク, Set-Cookie 透過）
        ▼
┌──────────────────┐    ┌────────────┐
│  api (Go)        │───>│ PostgreSQL │
│  (chi router)    │    │    16      │
│  :8080           │    │   :5432    │
└──────────────────┘    └────────────┘
┌──────────────────┐         │
│  worker (Go)     │─────────┘
│  (fetch/hatebu/  │───> 外部 RSS/Atom フィード
│   cleanup)       │───> はてなブックマーク API
└──────────────────┘
```

- **web**: Next.js 15 (App Router) + shadcn/ui の 2 ペイン SPA（Docker standalone ビルド）。
  `rewrites()` で `/api/*`・`/auth/*` を内部 API へ同一オリジン転送する reverse proxy も兼ねる
- **api**: HTTP ハンドラー、認証、ミドルウェア、CRUD API
- **worker**: フィード定期フェッチ、はてブ数取得、古い記事の自動削除
- **db**: PostgreSQL 16

### 単一オリジン化と build-once

ブラウザは常に **同一オリジンの相対パス**（例: `/api/feeds`）で API を呼びます。ビルド時に API の
URL を焼き込まないため（`NEXT_PUBLIC_API_URL` は廃止）、同一の `web` イメージを環境を問わず再利用
できます（**build-once**）。内部 API への転送先は **実行時**の環境変数 `API_INTERNAL_URL`（web の
server-side のみで参照、ブラウザ非公開）で決まり、未設定なら `web` は起動時に fail-fast します。

## 技術スタック

### バックエンド

| 技術 | バージョン | 用途 |
|------|-----------|------|
| Go | 1.25+ | API サーバー / ワーカー |
| chi | v5 | HTTP ルーター |
| PostgreSQL | 16 | データベース |
| golang-migrate | v4 | DB マイグレーション |
| gofeed | v1.3 | RSS/Atom パーサー |
| bluemonday | v1.0 | HTML サニタイズ (XSS 対策) |
| safeurl | v0.2 | SSRF 防止 |
| prometheus/client_golang | v1.23 | メトリクス収集 |
| golang.org/x/time/rate | - | レート制限 |

### フロントエンド

| 技術 | バージョン | 用途 |
|------|-----------|------|
| Next.js | 15 | React フレームワーク (App Router) |
| React | 19 | UI ライブラリ |
| TypeScript | 5 | 型安全 |
| Tailwind CSS | 4 | スタイリング |
| shadcn/ui | - | UI コンポーネント |
| TanStack Query | v5 | サーバー状態管理 |
| Vitest | v4 | テスト |

## 前提条件

- Go 1.25 以上
- Node.js 20 以上 / npm
- Docker / Docker Compose
- Google Cloud Console で OAuth 2.0 クライアント ID を取得済みであること

### Google OAuth の設定

1. [Google Cloud Console](https://console.cloud.google.com/) でプロジェクトを作成
2. 「APIとサービス」>「認証情報」> OAuth 2.0 クライアント ID を作成
3. 承認済みリダイレクト URI に **ブラウザ可視オリジン配下の callback URL** を追加する
   （単一オリジン化により `web` のオリジン配下になる。`api` の `:8080` ではない）:
   - ローカル: `http://localhost:3000/auth/google/callback`
   - 本番: `https://<host>/auth/google/callback`
   - ※ ここで登録する URL は `GOOGLE_REDIRECT_URL` の値と一致させること
4. クライアント ID とクライアントシークレットを控える

## 初期デプロイ手順

### 1. リポジトリのクローン

```bash
git clone https://github.com/hitoshiichikawa/feedman.git
cd feedman
```

### 2. 環境変数の設定

`.env.sample` をコピーし、値を環境に合わせて編集します:

```bash
# ローカル開発の場合
cp .env.sample .env.production
vi .env.production
```

最低限、以下の値を設定してください:

| 変数 | 所有 | 設定内容 |
|------|------|---------|
| `GOOGLE_CLIENT_ID` | api | Google Cloud Console で取得した OAuth クライアント ID |
| `GOOGLE_CLIENT_SECRET` | api | Google Cloud Console で取得した OAuth クライアントシークレット |
| `GOOGLE_REDIRECT_URL` | api | ブラウザ可視オリジン配下の callback URL（例: `https://<host>/auth/google/callback`）。Google Cloud Console の登録値と一致させる |
| `SESSION_SECRET` | api / worker | **起動に必須**。未設定/空のまま `docker compose up` / `config` すると fail-fast で停止する。ランダムな文字列を下記コマンドで生成して設定する |
| `BASE_URL` | api | ブラウザ可視オリジン（例: `https://<host>`）。callback 後のリダイレクト先・Cookie の Secure 自動判定に利用 |
| `API_INTERNAL_URL` | web | 内部 API 接続先（例: `http://api:8080`）。**実行時**に web が rewrites の転送先として参照。ブラウザ非公開。未設定なら web は起動時に fail-fast |
| `POSTGRES_PASSWORD` | db | **起動に必須**。未設定/空のまま `docker compose up` / `config` すると fail-fast で停止する（弱い既知のデフォルトは廃止済み）。下記コマンドで生成した値を設定する |
| `DATABASE_URL` | api / worker | DB 接続 URL。未設定ならコンテナ内 DB（`db` ホスト, `sslmode=disable`）向けデフォルトが適用される。**外部 PostgreSQL 接続時は `sslmode` に `require` 以上を明示すること**（[本番デプロイ時の注意事項](#本番デプロイ時の注意事項)参照） |
| `CORS_ALLOWED_ORIGIN` | api | CORS 許可オリジン（単一オリジン化で CORS プリフライトは発生しなくなるが、設定撤去は #23 の領分のため残置） |

> **`NEXT_PUBLIC_API_URL` は廃止しました。** 単一オリジン化によりブラウザは常に同一オリジンの
> 相対パスで API を呼ぶため、ビルド時に API の URL を焼き込みません（build-once）。内部 API への
> 転送先は実行時の `API_INTERNAL_URL` で指定します。

`SESSION_SECRET` と `POSTGRES_PASSWORD` は起動に必須です。**未設定/空のまま `docker compose up`
（や `docker compose config`）を実行すると、Compose 構成が fail-fast し、どの環境変数が必要かを
示すエラーメッセージを出力して停止します**（弱い既知のデフォルト値での起動導線は廃止済み）。
以下のコマンドで安全なランダム値を生成し、`.env.production` に設定してください:

```bash
# SESSION_SECRET の生成
openssl rand -base64 32

# POSTGRES_PASSWORD の生成
openssl rand -base64 32
```

> `.env.production` は `.gitignore` に含まれており、リポジトリにコミットされません。
> シークレットの流出を防ぐため、**環境変数ファイルは絶対にコミットしないでください。**

### 3. Docker Compose で起動

```bash
docker compose --env-file .env.production up -d
```

4 つのコンテナが起動します:

| コンテナ | 役割 | ポート |
|---------|------|-------|
| `web` | Next.js フロントエンド | 3000 |
| `api` | API サーバー (`serve`) | 8080 |
| `worker` | バックグラウンドワーカー (`worker`) | - |
| `db` | PostgreSQL 16 | 5432 |

### 4. DB マイグレーション

```bash
docker compose --env-file .env.production exec api /feedman migrate
```

### 5. 動作確認

- `http://localhost:3000` にアクセスし、Google ログインを実施
- フィード登録ダイアログで RSS/Atom フィードの URL を入力
- ワーカーが 5 分間隔でフィードをフェッチし、記事が表示される

### 6. DB バックアップ・リストア

本番運用では DB のバックアップ取得・リストア手順を整備しておくこと。手順・運用方針・トラブルシュートは
[`docs/operations/backup-restore.md`](docs/operations/backup-restore.md) を参照。

## 本番デプロイ時の注意事項

- `.env.sample` を `.env.production` にコピーし、本番用の値を設定する
- **`.env.production` は絶対に Git にコミットしない**（`.gitignore` で除外済み）
- `SESSION_SECRET` と `POSTGRES_PASSWORD` は **起動に必須**。`openssl rand -base64 32` で生成した
  十分に長いランダム値を設定する。**未設定/空のまま起動すると Compose 構成が fail-fast し、
  どの環境変数が必要かを示すエラーで停止する**（弱い既知のデフォルト値での起動導線は廃止済み）
- **単一オリジン化に伴う設定**:
  - `API_INTERNAL_URL` を内部 API の接続先（例: `http://api:8080`）に設定する。これは web の
    **実行時**環境変数であり、ビルド時引数ではない（同一イメージを環境横断で再利用できる = build-once）。
    未設定/空のまま起動すると web は fail-fast して停止する
  - `BASE_URL` を **ブラウザ可視オリジン**（`https://<host>`）に変更する。callback 後のリダイレクト先と
    Cookie の `Secure` 自動判定（`https://` で true）に利用される
  - `GOOGLE_REDIRECT_URL` を `https://<host>/auth/google/callback`（ブラウザ可視オリジン配下）に変更し、
    **Google Cloud Console の承認済みリダイレクト URI にも同じ値を登録する**
  - `NEXT_PUBLIC_API_URL` は廃止済み。設定しても無視され、ブラウザは常に同一オリジン相対パスで API を呼ぶ
- 単一 ingress（Cloudflare 等のリバースプロキシ）を前段に置き、TLS 終端のうえ **すべてのリクエストを
  `web`（:3000）へルーティング**する。`api`（:8080）はブラウザに公開せず内部ネットワークに留める
- `POSTGRES_PASSWORD` は必須化済みで、弱い既知のデフォルト（`feedman`）は廃止された。未設定だと
  起動できないため、上記のとおり `openssl rand -base64 32` で生成した値を設定する
- DB ポートはデフォルトで非公開。開発時に直接接続が必要な場合のみ `DB_PORT=5432` を設定する
- **DB 接続の TLS（`sslmode`）設定**:
  - コンテナ内 DB（`db` ホスト）を利用する場合のみ `sslmode=disable` が許容される（コンテナ間の
    `internal` ネットワークは外部公開されず、PostgreSQL コンテナも TLS 無効構成のため）
  - **外部 PostgreSQL へ接続する場合は `DATABASE_URL` の `sslmode` に `require` 以上
    （`require` / `verify-ca` / `verify-full`）を必須とする**。`sslmode=disable` のまま外部 DB へ
    接続すると平文通信となり、DB 認証情報・ユーザーデータが中間者攻撃（MITM）で漏洩する導線になる
  - 接続先に応じて `DATABASE_URL` を環境別に差し替える。外部 DB の例:
    `DATABASE_URL=postgres://<user>:<password>@<external-host>:5432/<dbname>?sslmode=require`
  - `.env.sample` の `DATABASE_URL` はデフォルトでコメントアウトしてあり、未設定時は
    docker-compose がコンテナ内 DB 向けデフォルト（`sslmode=disable`）を適用する。外部 DB を
    使う場合は `.env.production` で `DATABASE_URL` を `require` 以上の `sslmode` 付きで明示すること

## ネットワークセキュリティ

Docker Compose は 2 つのネットワークを定義:

- **internal**: API ↔ DB、Worker ↔ DB、Web → API（rewrites proxy 転送）間の内部通信専用（外部通信不可）
- **external**: Web（ポート公開用）、API、Worker（外部フィードフェッチ用）が接続

`web` は `internal` ネットワーク経由で `api`（`API_INTERNAL_URL=http://api:8080`）へ rewrites proxy
転送を行うため、両ネットワークに接続する。API サーバーも両ネットワークに接続し、DB へのアクセスは
`internal` 経由で行う。API 自身の SSRF 防止はアプリケーション層（safeurl）で実施。単一オリジン構成では
ブラウザは `web` のオリジンのみにアクセスし、`api` のポートをブラウザへ直接公開する必要はない。

## API エンドポイント

### 認証（認証不要）

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/auth/google/login` | OAuth フロー開始 |
| GET | `/auth/google/callback` | OAuth コールバック |
| POST | `/auth/logout` | ログアウト |
| GET | `/auth/me` | 現在のユーザー情報 |
### フィード管理（認証必須）

| メソッド | パス | 説明 |
|---------|------|------|
| POST | `/api/feeds` | フィード登録（自動検出） |
| GET | `/api/feeds/{id}` | フィード詳細 |
| PATCH | `/api/feeds/{id}` | フィード URL 変更 |
| DELETE | `/api/feeds/{id}` | フィード削除 |
| GET | `/api/feeds/{id}/items` | 記事一覧（カーソルページネーション） |

### 記事管理（認証必須）

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/api/items/{id}` | 記事詳細 |
| PUT | `/api/items/{id}/state` | 既読/スター状態更新 |

### 購読管理（認証必須）

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/api/subscriptions` | 購読一覧（未読数付き） |
| DELETE | `/api/subscriptions/{id}` | 購読解除 |
| PUT | `/api/subscriptions/{id}/settings` | フェッチ間隔設定 |
| POST | `/api/subscriptions/{id}/resume` | 停止フィードの再開 |

### ユーザー管理（認証必須）

| メソッド | パス | 説明 |
|---------|------|------|
| DELETE | `/api/users/me` | 退会（アカウント削除） |

### 監視

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/metrics` | Prometheus メトリクス |

## ミドルウェアスタック

認証が必要なルートには以下の順序でミドルウェアが適用される:

```
CORSMiddleware → SessionMiddleware → RateLimitMiddleware(General)
```

- **CORSMiddleware**: `CORS_ALLOWED_ORIGIN` で指定されたオリジンからのクロスオリジンリクエストを許可（`credentials: true`）
- **SessionMiddleware**: HTTP Only Cookie からセッションを検証し、user_id をコンテキストに注入
- **RateLimitMiddleware**: トークンバケット方式（120 req/分/ユーザー、フィード登録は 10 req/分）

## データベーススキーマ

| テーブル | 説明 |
|---------|------|
| `users` | ユーザーアカウント |
| `identities` | 外部 IdP アカウント紐付け（Google 等） |
| `feeds` | フィードのメタ情報とフェッチ状態 |
| `items` | フィードから取得した記事 |
| `subscriptions` | ユーザーとフィードの購読関係 |
| `item_states` | ユーザーごとの記事状態（既読/スター） |
| `user_settings` | ユーザー設定（テーマ等） |
| `sessions` | サーバーサイドセッション |

マイグレーションファイルは `internal/database/migrations/` に配置。

## ワーカージョブ

| ジョブ | 間隔 | 説明 |
|-------|------|------|
| フェッチスケジューラ | 5 分 | `next_fetch_at` に基づきフィードを取得（最大 10 並列） |
| はてブバッチ | 10 分 | 記事のはてなブックマーク数を一括取得（最大 50 URL/リクエスト） |
| 記事クリーンアップ | 日次 | 作成から 180 日超過した記事を自動削除 |

### フェッチリトライ戦略

| 条件 | 動作 |
|------|------|
| HTTP 404/410 | 即座にフェッチ停止 |
| HTTP 401/403 | 即座にフェッチ停止 |
| HTTP 429/5xx | 指数バックオフ（30 分〜最大 12 時間） |
| パース失敗 10 回連続 | フェッチ停止 |

停止したフィードは UI から手動で再開できる。

## セキュリティ

- **認証**: Google OAuth 2.0 + HTTP Only Cookie セッション
- **first-party Cookie**: 単一オリジン化により、セッション Cookie・OAuth `state` Cookie はブラウザの
  アクセス先（`web` のオリジン）に対する **first-party Cookie** となる。third-party Cookie ブロックの
  影響を受けず、`SameSite=None` を要求しない（`SameSite=Lax` を維持）
- **Cookie Secure 自動判定**: `BASE_URL` が `https://` の場合、Cookie の `Secure` フラグが自動的に有効化される
- **CSRF対策**: `SameSite=Lax` Cookie + `HttpOnly` による防御。`Lax` はトップレベル GET ナビゲーション
  （OAuth callback リダイレクト）で Cookie を送るため OAuth フローと整合し、クロスサイトの副作用リクエストには
  Cookie を送らない
- **XSS**: bluemonday によるサニタイズ（許可タグのみ通過、script/iframe/style 除去）
- **SSRF**: safeurl によるプライベート IP・メタデータ IP・ループバック拒否
- **レート制限**: ユーザーごとのトークンバケット方式
- **ネットワーク分離**: Docker internal ネットワークで DB への外部通信を遮断、API の SSRF 防止はアプリケーション層で実施
- **データ分離**: 全クエリで user_id 条件を強制

## 開発

### バックエンドのテスト

```bash
go test ./...
```

### フロントエンドのテスト

```bash
cd web
npm test
```

### ローカル開発（Docker なし）

```bash
# PostgreSQL を別途起動し、DATABASE_URL を設定
# ローカル PostgreSQL（localhost, TLS 無効）への接続のみ sslmode=disable を許容する。
# 外部 PostgreSQL へ接続する場合は sslmode に require 以上（require / verify-ca / verify-full）を
# 明示すること（平文通信防止）。
export DATABASE_URL=postgres://feedman:feedman@localhost:5432/feedman?sslmode=disable
export GOOGLE_CLIENT_ID=...
export GOOGLE_CLIENT_SECRET=...
# 単一オリジン化後はブラウザ可視オリジン（web の :3000）配下の callback URL
export GOOGLE_REDIRECT_URL=http://localhost:3000/auth/google/callback
export SESSION_SECRET=...
# 単一オリジン化後はブラウザ可視オリジン（web の :3000）
export BASE_URL=http://localhost:3000

# マイグレーション
go run ./cmd/feedman migrate

# API サーバー起動
go run ./cmd/feedman serve

# ワーカー起動（別ターミナル）
go run ./cmd/feedman worker

# フロントエンド起動（別ターミナル）
# next dev も rewrites を適用するため API_INTERNAL_URL を渡す。
# ブラウザは http://localhost:3000 のみにアクセスし /api/* /auth/* は :8080 へ転送される。
cd web && API_INTERNAL_URL=http://localhost:8080 npm run dev
```

## プロジェクト構成

```
feedman/
├── cmd/feedman/          # エントリーポイント
│   └── main.go
├── internal/
│   ├── app/              # アプリケーション初期化・CLI
│   ├── auth/             # OAuth 認証サービス
│   ├── config/           # 環境変数ベースの設定
│   ├── database/         # DB 接続・マイグレーション
│   │   └── migrations/   # SQL マイグレーションファイル
│   ├── feed/             # フィード検出・登録サービス
│   ├── handler/          # HTTP ハンドラー・ルーター
│   ├── hatebu/           # はてなブックマーク連携
│   ├── item/             # 記事 UPSERT・状態管理サービス
│   ├── logger/           # 構造化ログ (slog)
│   ├── metrics/          # Prometheus メトリクス
│   ├── middleware/        # CORS・セッション・レート制限・ログ
│   ├── model/            # ドメインモデル
│   ├── repository/       # データアクセス層 (PostgreSQL)
│   ├── security/         # SSRF 防止・コンテンツサニタイズ
│   ├── subscription/     # 購読管理サービス
│   ├── user/             # ユーザー管理・退会サービス
│   └── worker/           # バックグラウンドジョブ
│       ├── cleanup/      # 記事自動削除
│       └── fetch/        # フェッチスケジューラ・フェッチャー・リトライ
├── web/                  # Next.js フロントエンド
│   ├── Dockerfile        # Next.js マルチステージビルド (standalone)
│   └── src/
│       ├── app/          # App Router ページ
│       ├── components/   # React コンポーネント
│       │   └── ui/       # shadcn/ui コンポーネント
│       ├── contexts/     # React Context (AppState)
│       ├── hooks/        # カスタムフック
│       ├── lib/          # ユーティリティ (API クライアント)
│       └── types/        # TypeScript 型定義
├── Dockerfile            # Go バックエンド マルチステージビルド (distroless)
├── docker-compose.yml    # 4 コンテナ構成 (web, api, worker, db)
└── go.mod
```

## ライセンス

TBD
