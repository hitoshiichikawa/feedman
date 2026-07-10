# Requirements Document

## Introduction

access token は 900 秒で失効するため、Feedman iOS（親 Issue #163）は refresh token を使って
access token を更新し続ける必要がある。SERVER.md §1.4 はローテーション方式（refresh のたびに
新 refresh token を発行し旧 token を失効）を要求している。

本 spec は **`POST /api/auth/refresh` の rotation 付き再発行**のみを対象とする。具体的には
(1) 有効な refresh token の検証と新 access / refresh token の発行、(2) 旧 token の rotation
確定（以後の通常 refresh で使用不可）、(3) 並行 rotation の安全性、を要件化する。
rotated 済み token の**再利用検知による family 全体失効**は Issue #168 の領分であり、本 spec
では再利用を単に拒否するに留める。`POST /api/auth/revoke`（#168）、Bearer middleware（#169）、
IP レート制限（#171）はスコープ外。

## Requirements

### Requirement 1: Rotation 付き再発行の成功パス

**Objective:** As a Feedman iOS アプリ, I want refresh token で access token を更新したい,
so that 再ログインなしに API 呼び出しを継続できる

#### Acceptance Criteria

1. When refresh エンドポイントが有効な refresh token を受け取ったとき, the Refresh Endpoint shall 新しい `access_token`・新しい `refresh_token`・`token_type: "Bearer"`・`expires_in: 900` を JSON で返す
2. When rotation が成功したとき, the Refresh Endpoint shall 旧 refresh token を rotation 済みとして確定し、以後の通常 refresh で使用不可にする
3. When rotation が成功したとき, the Refresh Endpoint shall 新 refresh token を hash 値でのみ永続化し、旧 token と同一の rotation family に紐付ける
4. When 新 refresh token を発行するとき, the Refresh Endpoint shall 有効期限を発行時点から 30 日に再設定する（スライディング延長）
5. The Refresh Endpoint shall Cookie セッション・Bearer 認証のいずれも無しで呼び出し可能とする
6. The Refresh Endpoint shall 成功応答の JSON 形式を token 交換エンドポイント（Issue #166）と同一フィールド名（`access_token` / `refresh_token` / `token_type` / `expires_in`）とする

### Requirement 2: 再発行の拒否パス

**Objective:** As a Feedman 運用者, I want 無効な refresh token による再発行が確実に拒否されて
ほしい, so that 漏えい・失効済み token がアクセス継続につながらない

#### Acceptance Criteria

1. If 提示された refresh token が存在しないとき, the Refresh Endpoint shall 新 token を発行せず要求を拒否する
2. If 提示された refresh token が期限切れのとき, the Refresh Endpoint shall 新 token を発行せず要求を拒否する
3. If 提示された refresh token が失効（revoke）済みのとき, the Refresh Endpoint shall 新 token を発行せず要求を拒否する
4. If 提示された refresh token が既に rotation 済みのとき, the Refresh Endpoint shall 新 token を発行せず要求を拒否する
5. If リクエストボディが不正な JSON、または必須フィールド（`refresh_token`）が欠落しているとき, the Refresh Endpoint shall 入力不正として要求を拒否する
6. The Refresh Endpoint shall 拒否応答において token の存在有無・期限切れ・失効済み・rotation 済みを区別できる情報を返さない（同一のエラー応答とする）
7. If 再発行が拒否されたとき, the Refresh Endpoint shall 新 refresh token を永続化せず、既存 token の状態も変更しない

### Requirement 3: 並行 rotation の安全性

**Objective:** As a Feedman 運用者, I want 同一 refresh token への並行要求が二重発行に
つながらないでほしい, so that ネットワーク再送や攻撃による token 増殖を防げる

#### Acceptance Criteria

1. While 同一の refresh token に対する複数の rotation 要求が並行して処理されているとき, the Refresh Endpoint shall 高々 1 件の要求のみを成功させ、他の要求を拒否する

## Non-Functional Requirements

### NFR 1: セキュリティ

1. The Refresh Endpoint shall 新 refresh token を暗号論的乱数源から 256bit 以上のエントロピーで生成する
2. The Refresh Endpoint shall refresh token の平文を永続化領域・ログ・エラーメッセージのいずれにも残さない
3. The Refresh Endpoint shall エラー応答にクライアント入力値・内部詳細を反射しない

### NFR 2: 後方互換性

1. The API Server shall 既存のすべてのルート（Web ログイン・既存 API・token 交換）の挙動を変更しない
2. The API Server shall 署名鍵未設定の環境では refresh エンドポイントも公開しない（token 交換と同一の fail-closed 縮退）

### NFR 3: テスト容易性

1. The Refresh Module shall 成功・期限切れ・失効済み・不明 token・不正 JSON・rotation 後の旧 token 再利用の各ケースを外部ネットワーク依存なしで検証可能にする

## Out of Scope

- rotated 済み token 再利用検知による **family 全体失効**（Issue #168 — 本 spec は単純拒否のみ）
- `POST /api/auth/revoke`（Issue #168）
- Bearer middleware（Issue #169）/ IP レート制限（Issue #171）/ contract tests 全体整備（Issue #172）
- device_label 等の任意メタデータ
- refresh token family の上限管理・世代数制限

## Open Questions

- rotation 確定後・新 token 永続化前にインフラ障害が起きた場合、旧 token は消費済みのまま
  新 token が返らない（クライアントは再ログイン）。#166 の auth_code と同じ「安全側に倒して
  燃やす」方針を踏襲する（rollback 機構は導入しない）

## 関連

- Parent: #163
- Depends on: #164 #166
- Sibling: #165 #168 #169 #170 #171 #172
