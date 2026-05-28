# Requirements Document

## Introduction

API サーバーのセキュリティヘッダー付与ミドルウェアは現在 `X-Content-Type-Options` /
`X-Frame-Options` / `Referrer-Policy` / `Permissions-Policy` の 4 ヘッダーのみを全レスポンスに
付与しており、HTTPS 通信の強制（HSTS / Strict-Transport-Security）と、コンテンツ読み込み元の
制限（CSP / Content-Security-Policy）が欠落している。本機能は JSON のみを返す API サーバーに
適した厳格な CSP と、HTTPS 配信時の HSTS を追加し、既存 4 ヘッダーの挙動を維持したまま
セキュリティ態勢を強化する。本リポジトリは Cloudflare / リバースプロキシ ingress 配下で
TLS 終端の内側（平文 HTTP）で動作するため、HTTPS 配信かどうかの判定は運用構成に依存する点を
要件として扱う。スコープは API サーバー側ミドルウェアに限定し、フロントエンド（Next.js）側の
CSP 設定は Sibling Issue #53 で扱う。

## Requirements

### Requirement 1: CSP（Content-Security-Policy）ヘッダーの付与

**Objective:** As an API 運用者, I want すべての API レスポンスに JSON 専用 API に適した厳格な CSP が付与されること, so that XSS・クリックジャッキング・リソース読み込みを起点とした攻撃面を最小化できる

#### Acceptance Criteria

1. The セキュリティヘッダーミドルウェア shall すべてのレスポンスに `Content-Security-Policy` ヘッダーを付与する
2. The セキュリティヘッダーミドルウェア shall `Content-Security-Policy` の値に `default-src 'none'` ディレクティブを含める
3. The セキュリティヘッダーミドルウェア shall `Content-Security-Policy` の値に `frame-ancestors 'none'` ディレクティブを含める
4. When HTTP 配信時（非 TLS）であるとき, the セキュリティヘッダーミドルウェア shall HTTPS 配信時と同一の `Content-Security-Policy` を付与する

### Requirement 2: HSTS（Strict-Transport-Security）ヘッダーの条件付き付与

**Objective:** As an API 運用者, I want HTTPS 配信時のレスポンスにのみ HSTS ヘッダーが付与されること, so that 本番 HTTPS では通信の暗号化を強制しつつ開発用 HTTP 環境の動作を壊さない

#### Acceptance Criteria

1. While HTTPS 配信と判定される状態であるとき, the セキュリティヘッダーミドルウェア shall レスポンスに `Strict-Transport-Security: max-age=31536000; includeSubDomains` を付与する
2. While HTTP 配信（非 TLS）と判定される状態であるとき, the セキュリティヘッダーミドルウェア shall `Strict-Transport-Security` ヘッダーを付与しない
3. Where リバースプロキシ配下で `X-Forwarded-Proto` を信頼する設定が有効化されているとき, the セキュリティヘッダーミドルウェア shall `X-Forwarded-Proto` の値が `https` の場合に HTTPS 配信と判定する
4. Where リバースプロキシ配下で `X-Forwarded-Proto` を信頼する設定が有効化されているとき, the セキュリティヘッダーミドルウェア shall `X-Forwarded-Proto` の値が `https` 以外（`http` または欠落）の場合に HTTP 配信と判定する

### Requirement 3: HSTS 有効化フラグによる出力制御

**Objective:** As an API 運用者, I want 設定フラグで HSTS の出力有無を切り替えられること, so that デプロイ構成（HTTPS 終端の有無）に応じて HSTS を安全に有効化・無効化できる

#### Acceptance Criteria

1. While HSTS 有効化フラグが無効であるとき, the セキュリティヘッダーミドルウェア shall HTTPS 配信と判定される場合でも `Strict-Transport-Security` ヘッダーを付与しない
2. While HSTS 有効化フラグが有効かつ HTTPS 配信と判定される状態であるとき, the セキュリティヘッダーミドルウェア shall `Strict-Transport-Security` ヘッダーを付与する
3. If HSTS 有効化フラグの設定値が未指定または不正値であるとき, the 設定読み込み処理 shall 既定値を採用して起動を継続する

### Requirement 4: 既存セキュリティヘッダーの後方互換維持

**Objective:** As an API 運用者, I want 既存 4 ヘッダーの値と付与挙動が変わらないこと, so that 本変更によって既存のセキュリティ保証やクライアント挙動が退行しない

#### Acceptance Criteria

1. The セキュリティヘッダーミドルウェア shall すべてのレスポンスに `X-Content-Type-Options: nosniff` を付与する
2. The セキュリティヘッダーミドルウェア shall すべてのレスポンスに `X-Frame-Options: DENY` を付与する
3. The セキュリティヘッダーミドルウェア shall すべてのレスポンスに `Referrer-Policy: strict-origin-when-cross-origin` を付与する
4. The セキュリティヘッダーミドルウェア shall すべてのレスポンスに `Permissions-Policy: camera=(), microphone=(), geolocation=()` を付与する
5. The セキュリティヘッダーミドルウェア shall 既存 4 ヘッダーを全ルート（認証要否を問わない全エンドポイント）に対して付与する

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The セキュリティヘッダーミドルウェア shall 本機能導入前から付与されていた 4 ヘッダーの値を変更しない
2. Where HSTS 有効化フラグが未設定であるとき, the セキュリティヘッダーミドルウェア shall 本機能導入前と等価な HSTS 非出力挙動を保つ

### NFR 2: 観測可能性

1. The API サーバー shall いずれのレスポンスに対しても `Content-Security-Policy` ヘッダーを欠落させない（HTTP / HTTPS いずれの配信でも常時付与する）
2. When 同一リクエストに対して複数のセキュリティヘッダーを付与するとき, the セキュリティヘッダーミドルウェア shall 各ヘッダー名につき 1 つの値のみを設定する（重複ヘッダーを生成しない）

## Out of Scope

- フロントエンド（Next.js）側の CSP 設定（`web/next.config.ts`）— Sibling Issue #53 で対応
- インラインスクリプトの nonce ベース移行 — Sibling Issue #53 で対応
- CSP の `report-uri` / `report-to` による違反レポート収集機構
- `Strict-Transport-Security` の `preload` 付与（確認事項 1 参照。本要件では付与しない方針）
- セキュリティヘッダー以外のミドルウェア（CORS / レート制限 / ロギング等）の挙動変更
- HSTS の `max-age` 値や CSP ディレクティブを実行時に動的変更する仕組み

## Open Questions

- 確認事項 1: `Strict-Transport-Security` に `preload` を付与するか。`preload` は HSTS preload list
  への登録を前提とし、一度登録すると当該ドメインおよびサブドメインが恒久的に HTTPS 強制となる
  （取り消しに時間を要する）。本要件では `preload` を **付与しない**方針（Requirement 2.1 の値）
  とし、将来 preload list 登録を運用判断する場合は別途要件化する。この方針で問題ないか人間に確認したい。
- 確認事項 2: HTTPS 配信判定について。本リポジトリは Cloudflare / リバースプロキシ ingress 配下で
  TLS 終端の内側（平文 HTTP）で Go サーバーが動作するため、Go ランタイムの TLS 接続情報のみでは
  本番でも HTTPS と判定できない。このため `X-Forwarded-Proto` を信頼する判定経路（Requirement 2.3 /
  2.4）を前提としている。`X-Forwarded-Proto` を無条件に信頼すると、信頼できないネットワークから
  直接到達できる構成ではヘッダー偽装により判定を欺かれ得る。実運用で Go サーバーへの到達経路が
  信頼できるプロキシ経由に限定されている前提でよいか、人間に確認したい（信頼境界の確定はデプロイ
  構成依存のため PM では確定しない）。

## 関連

- Sibling: #53
