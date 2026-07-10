# Requirements Document

## Introduction

Feedman iOS（親 Issue #163）は `ASWebAuthenticationSession` で
`/auth/google/login?flow=native&code_challenge=...` を開き、OAuth 完了後に Cookie ではなく
アプリスキーム `feedman://auth/callback?auth_code=...` で一時コードを受け取る必要がある。
既存の Google OAuth callback は Web Cookie セッション発行を前提としており、native flow の
分岐が存在しない。

本 spec は **OAuth login / callback における native flow の分岐**のみを対象とする。
具体的には (1) `flow=native` 開始時の PKCE S256 パラメータ検証と flow 文脈の保持、
(2) callback 成功時の短命・単回利用 `auth_code` の発行とアプリスキームへのリダイレクト、
(3) `flow=native` を伴わない既存 Web ログインの完全な後方互換、を要件化する。
`auth_code` の永続化には Issue #164 で導入済みの永続化レイヤーを利用する。
`auth_code` を Bearer token に交換する `POST /api/auth/token` は Issue #166 の領分であり
本 spec では扱わない。

## Requirements

### Requirement 1: Native flow の開始と PKCE パラメータ検証

**Objective:** As a Feedman iOS アプリ, I want `flow=native` と PKCE S256 challenge を付けて
OAuth ログインを開始したい, so that OAuth 完了後にアプリへ安全に一時コードを受け取る
準備ができる

#### Acceptance Criteria

1. When ログイン開始エンドポイントが `flow=native` と有効な PKCE S256 パラメータ（challenge と method=S256）付きで呼ばれたとき, the Auth Login Endpoint shall OAuth プロバイダーへの認可リダイレクトを開始し、native flow の文脈（PKCE challenge を含む）を callback 処理まで保持する
2. If `flow=native` が指定され PKCE challenge が欠落しているとき, the Auth Login Endpoint shall OAuth リダイレクトを開始せず、安全なエラー（入力値を反射しない 4xx 応答）で要求を拒否する
3. If `flow=native` が指定され PKCE method が S256 以外（欠落・plain 等）のとき, the Auth Login Endpoint shall OAuth リダイレクトを開始せず、安全なエラーで要求を拒否する
4. If `flow=native` が指定され PKCE challenge が S256 の challenge として不正な形式のとき, the Auth Login Endpoint shall OAuth リダイレクトを開始せず、安全なエラーで要求を拒否する
5. When ログイン開始エンドポイントが `flow=native` 指定なしで呼ばれたとき, the Auth Login Endpoint shall 従来どおりの Web ログイン開始挙動（CSRF state の発行と OAuth リダイレクト）を維持する
6. When ログイン開始エンドポイントが `flow=native` 指定なしで呼ばれ、過去の native flow 文脈が残存しているとき, the Auth Login Endpoint shall 残存する native flow 文脈を破棄し Web flow として継続する

### Requirement 2: Native callback での auth_code 発行とアプリスキームリダイレクト

**Objective:** As a Feedman iOS アプリ, I want OAuth 完了時にアプリスキームで一時コードを
受け取りたい, so that ディープリンク経由で `auth_code` を受領し後続の token 交換に進める

#### Acceptance Criteria

1. When native flow の OAuth callback が成功したとき, the Auth Callback Endpoint shall 短命・単回利用前提の `auth_code` を発行し `feedman://auth/callback?auth_code=<one-time-code>` へリダイレクトする
2. When native flow の callback で `auth_code` を発行するとき, the Auth Callback Endpoint shall `auth_code` を発行時刻から 60 秒で失効する有効期限・PKCE challenge・解決済みユーザー識別子に紐付けて保存する
3. When native flow の callback が完了したとき, the Auth Callback Endpoint shall Web 用セッション Cookie を発行せず、Web 用セッションも作成しない
4. When native flow の callback でユーザーを解決するとき, the Auth Callback Endpoint shall 既存ユーザーのログインと新規ユーザーの自動作成を Web flow と同一の規則で行う
5. The Auth Callback Endpoint shall `auth_code` の平文をリダイレクト先 URL 以外（ログ・エラーメッセージ・永続化領域）に残さない

### Requirement 3: State 検証と native flow 文脈の安全性

**Objective:** As a Feedman 運用者, I want native flow でも既存の CSRF 対策と安全な
フォールバックが機能してほしい, so that native 分岐の追加が新たな攻撃面を作らない

#### Acceptance Criteria

1. If OAuth callback の state 検証に失敗したとき（native flow 文脈の有無に関わらず）, the Auth Callback Endpoint shall 要求を拒否し `auth_code` を発行しない
2. If OAuth callback 時に native flow 文脈が確認できない（欠落・期限切れ）とき, the Auth Callback Endpoint shall 従来の Web flow として処理する
3. When native flow の callback 処理が完了または失敗したとき, the Auth Callback Endpoint shall native flow 文脈を破棄し再利用不能にする
4. If native flow の callback 処理中にユーザー解決・`auth_code` 保存が失敗したとき, the Auth Callback Endpoint shall 安全なエラー（内部詳細を反射しない 5xx 応答）を返し `auth_code` を発行しない

### Requirement 4: 既存 Web flow の後方互換

**Objective:** As a Feedman Web ユーザー, I want 既存の Web ログイン・ログアウトが
従来どおり動作してほしい, so that native 対応の追加によって Web の利用が影響を受けない

#### Acceptance Criteria

1. When OAuth callback が native flow 文脈なしで成功したとき, the Auth Callback Endpoint shall 既存の Web Cookie ログイン挙動（セッション発行・セッション固定対策・Cookie 設定・フロントエンドへのリダイレクト）を 100% 維持する
2. The Auth Login Endpoint and Auth Callback Endpoint shall 既存のログアウト・現在ユーザー取得の挙動を変更しない

## Non-Functional Requirements

### NFR 1: セキュリティ

1. The Auth Callback Endpoint shall `auth_code` を暗号論的乱数源から十分なエントロピー（128bit 以上）で生成する
2. The Auth Login Endpoint shall PKCE method として S256 のみを受理する（plain は拒否する）
3. The Auth Login Endpoint and Auth Callback Endpoint shall エラー応答にクライアント入力値・内部詳細（SQL・スタックトレース等）を反射しない

### NFR 2: 後方互換性

1. The Auth Login Endpoint and Auth Callback Endpoint shall `flow=native` を伴わないすべての既存リクエストに対して、本 spec 導入前と同一の応答（ステータスコード・リダイレクト先・Cookie 発行）を返す

### NFR 3: テスト容易性

1. The Auth Module shall native flow 成功・PKCE 欠落/不正・state 不正・既存 Web flow 互換の各ケースを外部ネットワーク依存なし（OAuth プロバイダーはモック）で検証可能にする

## Out of Scope

- `POST /api/auth/token` での auth_code → Bearer token 交換（Issue #166）
- refresh token の発行・ローテーション・再利用検知（Issue #167 / #168）
- Bearer-or-Session middleware の導入（Issue #169）
- 退会時の native auth state クリーンアップ（Issue #170）
- native auth endpoint への未認証 IP レート制限（Issue #171）
- contract / integration テストの全体整備（Issue #172）
- 期限切れ `auth_code` レコードの定期削除（cleanup worker）— 単回利用と TTL 判定は #164 の永続化レイヤーが担保済み
- アプリスキーム（`feedman://auth/callback`）の設定変更機構 — Issue 本文の仮案どおり固定値とする

## Open Questions

- native flow の callback 失敗時（OAuth 拒否・ユーザー解決失敗）にアプリスキームへ `error` パラメータ付きでリダイレクトする UX 改善は、iOS 側仕様（SERVER.md §1.2）に規定がないため本 spec では HTTP エラー応答（既存 Web flow と同形）とする。アプリ側で必要になった場合は別 Issue で扱う

## 関連

- Parent: #163
- Depends on: #164
- Sibling: #166 #167 #168 #169 #170 #171 #172
