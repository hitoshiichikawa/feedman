# Requirements Document

## Introduction

Feedman iOS（親 Issue #163）は OAuth callback（Issue #165）で受け取った一時 `auth_code` と
PKCE `code_verifier` を `POST /api/auth/token` に送り、API 呼び出しに使う access token と
refresh token を取得する。この交換エンドポイントが無いと native login が完了できない。

本 spec は **`POST /api/auth/token` の token 交換**のみを対象とする。具体的には
(1) `auth_code` の単回消費と PKCE S256 verifier 検証、(2) access token（短命・自己完結型）と
refresh token（不透明・hash 保存）の発行、(3) 署名鍵の環境変数管理と未設定時の安全な縮退、
を要件化する。永続化には Issue #164 の `AuthCodeRepository` / `RefreshTokenRepository` を、
`auth_code` の発行には Issue #165 の callback 実装を前提とする。refresh のローテーション
（#167）、再利用検知と revoke（#168）、Bearer middleware（#169）、IP レート制限（#171）は
スコープ外。

## Requirements

### Requirement 1: Token 交換の成功パス

**Objective:** As a Feedman iOS アプリ, I want `auth_code` と `code_verifier` を本トークンに
交換したい, so that 以降の API 呼び出しを Bearer 認証で行える

#### Acceptance Criteria

1. When token 交換エンドポイントが有効な `auth_code` と保存済み challenge に一致する PKCE verifier を受け取ったとき, the Token Exchange Endpoint shall `access_token`・`refresh_token`・`token_type: "Bearer"`・`expires_in: 900` を JSON で返す
2. When token 交換が成功したとき, the Token Exchange Endpoint shall 当該 `auth_code` を使用済みとして確定し、同一 code の再交換を不可能にする
3. When token 交換が成功したとき, the Token Exchange Endpoint shall refresh token を hash 値でのみ永続化し、新規の rotation family に紐付ける
4. The Token Exchange Endpoint shall 発行する access token を、発行から 900 秒で失効し、ユーザー識別子を含み、サーバー側に保存せず検証可能な自己完結形式とする
5. The Token Exchange Endpoint shall Cookie セッション・Bearer 認証のいずれも無しで呼び出し可能とする
6. The Token Exchange Endpoint shall 成功応答の JSON フィールド名を `access_token` / `refresh_token` / `token_type` / `expires_in` とする

### Requirement 2: 交換の拒否パス

**Objective:** As a Feedman 運用者, I want 不正・期限切れ・再利用の交換要求が確実に
拒否されてほしい, so that auth_code の漏えいや総当たりがトークン奪取につながらない

#### Acceptance Criteria

1. If PKCE verifier から導出した challenge が保存済み challenge と一致しないとき, the Token Exchange Endpoint shall token を発行せず交換を拒否する
2. If 提示された `auth_code` が存在しないとき, the Token Exchange Endpoint shall token を発行せず交換を拒否する
3. If 提示された `auth_code` が期限切れまたは使用済みのとき, the Token Exchange Endpoint shall token を発行せず交換を拒否する
4. If `code_verifier` が PKCE verifier として不正な形式のとき, the Token Exchange Endpoint shall token を発行せず交換を拒否する
5. If リクエストボディが不正な JSON、または必須フィールド（`auth_code` / `code_verifier`）が欠落しているとき, the Token Exchange Endpoint shall 入力不正として要求を拒否する
6. The Token Exchange Endpoint shall 拒否応答において、`auth_code` の存在有無・期限切れ・使用済み・verifier 不一致を区別できる情報を返さない（同一のエラー応答とする）
7. If 交換が拒否されたとき, the Token Exchange Endpoint shall refresh token・rotation family のいずれも永続化しない

### Requirement 3: 署名鍵の管理と安全な縮退

**Objective:** As a Feedman 運用者, I want access token の署名鍵を環境変数で管理し、
未設定の環境では機能が安全に無効化されてほしい, so that 鍵の混入事故や無防備な公開を防げる

#### Acceptance Criteria

1. The Token Exchange Endpoint shall access token の署名鍵を環境変数から読み込む
2. If 署名鍵が未設定のとき, the API Server shall token 交換エンドポイントを公開せず（未登録 = 404）、起動自体は成功させる
3. If 署名鍵が未設定のとき, the API Server shall 起動時に運用者向けの警告を 1 回記録する
4. Where テストコードが対象となるとき, the Token Issuance Module shall 固定の署名鍵・固定の時刻を注入して決定論的に検証可能とする
5. The Token Issuance Module shall 発行する access token に鍵識別子（kid）を含め、将来の鍵ローテーションに備える

## Non-Functional Requirements

### NFR 1: セキュリティ

1. The Token Exchange Endpoint shall refresh token を暗号論的乱数源から 256bit 以上のエントロピーで生成する
2. The Token Exchange Endpoint shall refresh token の平文を永続化領域・ログ・エラーメッセージのいずれにも残さない
3. The Token Exchange Endpoint shall `auth_code`・`code_verifier` の平文をログ・エラーメッセージに残さない
4. The Token Exchange Endpoint shall PKCE challenge の比較を定数時間比較で行う
5. The Token Exchange Endpoint shall エラー応答にクライアント入力値・内部詳細（SQL・スタックトレース等）を反射しない

### NFR 2: 後方互換性

1. The API Server shall 既存のすべてのルート（Web ログイン・既存 API）の挙動を変更しない
2. The API Server shall 署名鍵の環境変数が存在しない既存デプロイ環境でも、本 spec 導入前と同一に起動・動作する

### NFR 3: テスト容易性

1. The Token Exchange Module shall 交換成功・verifier 不一致・期限切れ code・使用済み code・不正 JSON の各ケースを外部ネットワーク依存なしで検証可能にする

## Out of Scope

- `POST /api/auth/refresh`（rotation、Issue #167）
- `POST /api/auth/revoke` と refresh token 再利用検知（Issue #168）
- Bearer middleware による既存 API の Bearer 対応（Issue #169 — access token の**検証**は
  #169 の領分。本 spec は**発行**のみ）
- token / refresh endpoint への未認証 IP レート制限（Issue #171）
- contract / integration テストの全体整備（Issue #172）
- 鍵ローテーションの実装（複数鍵の並行受理）— kid の付与までを本 spec で行う
- device_label 等の任意メタデータの受け付け

## Open Questions

- access token の形式は iOS 側仕様（SERVER.md §1.4）の指定どおり JWT とし、署名方式は
  単一サービス構成のため対称鍵（HS256）を採用する前提で design に委ねる（RS256 への変更が
  必要になった場合は別 Issue）

## 関連

- Parent: #163
- Depends on: #164 #165
- Sibling: #167 #168 #169 #170 #171 #172
