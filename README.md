# Feedman

Web ベースの RSS/Atom フィードリーダー。Google OAuth 認証、2ペイン UI、バックグラウンドフェッチ、はてなブックマーク連携を備える。

## アーキテクチャ

```
┌─────────────┐    ┌──────────────────┐    ┌────────────┐
│  Next.js    │───>│  Go API Server   │───>│ PostgreSQL │
│  Frontend   │    │  (chi router)    │    │    16      │
│  :3000      │    │  :8080           │    │   :5432    │
└─────────────┘    └──────────────────┘    └────────────┘
                   ┌──────────────────┐         │
                   │  Go Worker       │─────────┘
                   │  (fetch/hatebu/  │───> 外部 RSS/Atom フィード
                   │   cleanup)       │───> はてなブックマーク API
                   └──────────────────┘
```

- **API サーバー**: HTTP ハンドラー、認証、ミドルウェア、CRUD API
- **ワーカー**: フィード定期フェッチ、はてブ数取得、古い記事の自動削除
- **フロントエンド**: Next.js 15 (App Router) + shadcn/ui の 2 ペイン SPA

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
3. 承認済みリダイレクト URI に `http://localhost:8080/auth/google/callback` を追加
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

| 変数 | 設定内容 |
|------|---------|
| `GOOGLE_CLIENT_ID` | Google Cloud Console で取得した OAuth クライアント ID |
| `GOOGLE_CLIENT_SECRET` | Google Cloud Console で取得した OAuth クライアントシークレット |
| `SESSION_SECRET` | ランダムな文字列（下記コマンドで生成） |

```bash
# SESSION_SECRET の生成
openssl rand -base64 32
```

> `.env.production` は `.gitignore` に含まれており、リポジトリにコミットされません。
> シークレットの流出を防ぐため、**環境変数ファイルは絶対にコミットしないでください。**

### 3. Docker Compose で起動

```bash
docker compose --env-file .env.production up -d
```

3 つのコンテナが起動します:

| コンテナ | 役割 | ポート |
|---------|------|-------|
| `api` | API サーバー (`serve`) | 8080 |
| `worker` | バックグラウンドワーカー (`worker`) | - |
| `db` | PostgreSQL 16 | 5432 |

### 4. DB マイグレーション

```bash
docker compose --env-file .env.production exec api /feedman migrate
```

### 5. フロントエンドの起動（開発環境）

```bash
cd web
npm install
npm run dev
```

`http://localhost:3000` でアクセスできます。

### 6. 動作確認

- `http://localhost:3000` にアクセスし、Google ログインを実施
- フィード登録ダイアログで RSS/Atom フィードの URL を入力
- ワーカーが 5 分間隔でフィードをフェッチし、記事が表示される

## 本番デプロイ時の注意事項

- `.env.sample` を `.env.production` にコピーし、本番用の値を設定する
- **`.env.production` は絶対に Git にコミットしない**（`.gitignore` で除外済み）
- `SESSION_SECRET` には `openssl rand -base64 32` で生成した十分に長いランダム文字列を設定する
- `BASE_URL` と `GOOGLE_REDIRECT_URL` を実際のドメイン（HTTPS）に変更する
- PostgreSQL のパスワードをデフォルト（`feedman`）から変更する
- HTTPS を有効にし、リバースプロキシ（nginx 等）を前段に配置する
- `docker-compose.yml` の `db` ポートマッピングを削除し、外部からの直接接続を遮断する

## ネットワークセキュリティ

Docker Compose は 2 つのネットワークを定義:

- **internal**: API ↔ DB、Worker ↔ DB 間の内部通信専用（外部通信不可）
- **external**: Worker のみ接続し、外部フィードのフェッチを許可

API サーバーは `internal` ネットワークのみに接続し、外部への直接通信は行わない。

## API エンドポイント

### 認証（認証不要）

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/auth/google/login` | OAuth フロー開始 |
| GET | `/auth/google/callback` | OAuth コールバック |
| POST | `/auth/logout` | ログアウト |
| GET | `/auth/me` | 現在のユーザー情報 |
| GET | `/api/csrf-token` | CSRF トークン取得 |

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
SessionMiddleware → CSRFMiddleware → RateLimitMiddleware(General)
```

- **SessionMiddleware**: HTTP Only Cookie からセッションを検証し、user_id をコンテキストに注入
- **CSRFMiddleware**: SameSite=Lax + X-CSRF-Token ヘッダーによるダブルサブミット検証
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
- **CSRF**: SameSite=Lax Cookie + X-CSRF-Token ダブルサブミット
- **XSS**: bluemonday によるサニタイズ（許可タグのみ通過、script/iframe/style 除去）
- **SSRF**: safeurl によるプライベート IP・メタデータ IP・ループバック拒否
- **レート制限**: ユーザーごとのトークンバケット方式
- **ネットワーク分離**: Docker ネットワークで API の外部通信を遮断
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
export DATABASE_URL=postgres://feedman:feedman@localhost:5432/feedman?sslmode=disable
export GOOGLE_CLIENT_ID=...
export GOOGLE_CLIENT_SECRET=...
export GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback
export SESSION_SECRET=...
export BASE_URL=http://localhost:8080

# マイグレーション
go run ./cmd/feedman migrate

# API サーバー起動
go run ./cmd/feedman serve

# ワーカー起動（別ターミナル）
go run ./cmd/feedman worker

# フロントエンド起動（別ターミナル）
cd web && npm run dev
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
│   ├── middleware/        # セッション・CSRF・レート制限・ログ
│   ├── model/            # ドメインモデル
│   ├── repository/       # データアクセス層 (PostgreSQL)
│   ├── security/         # SSRF 防止・コンテンツサニタイズ
│   ├── subscription/     # 購読管理サービス
│   ├── user/             # ユーザー管理・退会サービス
│   └── worker/           # バックグラウンドジョブ
│       ├── cleanup/      # 記事自動削除
│       └── fetch/        # フェッチスケジューラ・フェッチャー・リトライ
├── web/                  # Next.js フロントエンド
│   └── src/
│       ├── app/          # App Router ページ
│       ├── components/   # React コンポーネント
│       │   └── ui/       # shadcn/ui コンポーネント
│       ├── contexts/     # React Context (AppState)
│       ├── hooks/        # カスタムフック
│       ├── lib/          # ユーティリティ (API クライアント, CSRF)
│       └── types/        # TypeScript 型定義
├── Dockerfile            # マルチステージビルド (distroless)
├── docker-compose.yml    # 3 コンテナ構成
└── go.mod
```

## ライセンス

TBD
