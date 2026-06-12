# Requirements Document

## Introduction

Feedman の native token 認証（親 Issue #163）は iOS v1 が依存するサーバー契約であり、
Issue #164〜#171 にまたがって実装された。各 spec は単体で要件カバレッジを満たしているが、
複数 Issue にまたがる主要な動線 —— ネイティブ OAuth → `auth_code` 発行 → token 交換 →
refresh rotation → revoke → Bearer 認証付き既存 API 呼び出し —— の **end-to-end な契約整合**
は、横断的な検証手段を持たない限り iOS 側仕様（feedman-ios `design/SERVER.md` §1）からの
ずれを検出できない。

本 spec は **native auth API の contract / integration テスト整備、および iOS 仕様
（SERVER.md §1）との同期確認**のみを対象とする。新規エンドポイントの追加実装、iOS
クライアント実装、v1 スコープ外機能（push 通知・OPML 等）は扱わない。実装済み spec
（#165〜#171）が定めた挙動・JSON 形状・エラー契約を「契約として何が検証されるべきか」の
観点で要件化し、検証手段（テストファイル配置・関数名・helper 構成）の選定は design の
領分とする。

## Requirements

### Requirement 1: Native OAuth フロー全体の契約検証

**Objective:** As a Feedman iOS / サーバー運用者, I want native OAuth フロー全体の主要動線が
契約として検証されてほしい, so that 複数 Issue にまたがる実装の整合が iOS 仕様からずれた
ことを早期に検出できる

#### Acceptance Criteria

1. The Native Auth Contract Test Suite shall ネイティブ OAuth callback での一時 `auth_code` 発行とアプリスキームへのリダイレクトを契約として検証する
2. The Native Auth Contract Test Suite shall `POST /api/auth/token` による `auth_code` と PKCE verifier の本トークン交換を契約として検証する
3. The Native Auth Contract Test Suite shall `POST /api/auth/refresh` による rotation 付き再発行を契約として検証する
4. The Native Auth Contract Test Suite shall `POST /api/auth/revoke` による refresh token 失効を契約として検証する
5. The Native Auth Contract Test Suite shall 発行済み access token を `Authorization: Bearer` として認証必須既存 API に提示した際に、Cookie セッション認証時と同一のユーザー識別で応答されることを契約として検証する
6. The Native Auth Contract Test Suite shall 上記 5 つの動線を、外部ネットワーク・実 Google OAuth・実 FCM への接続なしで実行可能とする

### Requirement 2: JSON 応答契約（フィールド名・型・status）の検証

**Objective:** As a Feedman iOS 実装者, I want token 系応答の JSON 形状が iOS 仕様と一致して
ほしい, so that クライアント側の decoder が壊れない

#### Acceptance Criteria

1. The Native Auth Contract Test Suite shall `POST /api/auth/token` の成功応答が JSON フィールド `access_token` / `refresh_token` / `token_type` / `expires_in` を含むことを検証する
2. The Native Auth Contract Test Suite shall `POST /api/auth/refresh` の成功応答が JSON フィールド `access_token` / `refresh_token` / `token_type` / `expires_in` を含むことを検証する
3. The Native Auth Contract Test Suite shall token 系成功応答の `token_type` フィールド値が `Bearer` であることを検証する
4. The Native Auth Contract Test Suite shall token 系成功応答の `expires_in` フィールド値が 900（秒）であることを検証する
5. The Native Auth Contract Test Suite shall `POST /api/auth/revoke` の成功応答が HTTP 204 No Content かつボディなしであることを検証する
6. The Native Auth Contract Test Suite shall native callback 成功時のリダイレクト先 URL が `feedman://auth/callback?auth_code=<one-time-code>` の形式であることを検証する

### Requirement 3: エラー契約（拒否パス）の検証

**Objective:** As a Feedman 運用者, I want 不正・期限切れ・再利用などの拒否パスが契約として
検証されてほしい, so that 攻撃面の縮退・列挙オラクル防止が後続変更でも維持される

#### Acceptance Criteria

1. The Native Auth Contract Test Suite shall 不正・期限切れ・使用済みの `auth_code` を提示した `POST /api/auth/token` 要求が token を発行せず拒否されることを検証する
2. The Native Auth Contract Test Suite shall 保存済み challenge と一致しない PKCE verifier を提示した `POST /api/auth/token` 要求が token を発行せず拒否されることを検証する
3. The Native Auth Contract Test Suite shall 不明・期限切れ・失効済み・rotation 済みの refresh token を提示した `POST /api/auth/refresh` 要求が新 token を発行せず拒否されることを検証する
4. The Native Auth Contract Test Suite shall rotation 済みの refresh token を `POST /api/auth/refresh` に再提示した際に、当該 family に属する以後の refresh 要求も拒否されることを検証する
5. The Native Auth Contract Test Suite shall 署名不正・期限切れ・用途種別不一致・形式不正の Bearer token を認証必須 API に提示した要求が、Cookie セッションへの fallback なしで未認証として拒否されることを検証する
6. The Native Auth Contract Test Suite shall 上記の拒否応答が、原因の別（auth_code の存在有無 / 期限切れ / 使用済み / verifier 不一致、refresh token の存在有無 / 期限切れ / 失効済み / rotation 済み、Bearer の署名不正 / 期限切れ / 用途不一致 / 形式不正）を区別できる情報を含まないことを検証する

### Requirement 4: iOS 仕様（SERVER.md §1）との同期確認

**Objective:** As a Feedman / iOS 双方の開発者, I want サーバー実装と iOS 仕様の差分が
明示されてほしい, so that 暗黙の差分により iOS 実装が想定外の挙動に遭遇しない

#### Acceptance Criteria

1. The Implementation Notes shall サーバー実装（#164〜#171）と feedman-ios `design/SERVER.md` §1（エンドポイント契約・トークン設計・受け入れ基準）の整合を確認した結果を記録する
2. Where 実装が SERVER.md §1 の記述と異なるとき, the Implementation Notes shall 当該差分を「承認済み逸脱」として明示し、根拠となる Issue / spec を参照する
3. The Implementation Notes shall SERVER.md §1.3 が `POST /api/auth/revoke` を「Bearer 認証下」と記述するのに対し、実装が #168 で「未認証 + token 所持 = 権限」を採用済みであることを承認済み逸脱として明文化する
4. The Implementation Notes shall SERVER.md §1.5 の DB スキーマ例と、#164 で確定した実装スキーマとの差分（テーブル分離等）を承認済み逸脱として明文化する
5. If 整合確認の過程で SERVER.md §1 と実装の間に承認されていない差分を発見したとき, the Implementation Notes shall 当該差分を本 spec で確定させず、Issue コメントによる人間判断を仰ぐ旨を記録する

## Non-Functional Requirements

### NFR 1: テスト容易性・決定性

1. The Native Auth Contract Test Suite shall 実 Google OAuth プロバイダー・実 FCM・実外部ネットワークへの接続なしで全契約検証を完了する
2. The Native Auth Contract Test Suite shall 時刻依存（access token の 900 秒有効期限・refresh token の 30 日有効期限・`auth_code` の 60 秒有効期限）の検証において、固定時刻の注入により決定論的に再現可能とする
3. The Native Auth Contract Test Suite shall 署名鍵を固定値として注入し、token 発行・検証の決定論的な再現を可能とする

### NFR 2: 後方互換性

1. The Native Auth Contract Test Suite shall 既存 Web Cookie セッション認証の動線（ログイン・ログアウト・認証必須 API への Cookie 提示）が本 spec 導入により挙動変更されないことを検証する
2. The Native Auth Contract Test Suite shall 既存テストスイートの実行結果を本 spec 導入前と同一に保つ

### NFR 3: 既存 spec との非重複

1. The Native Auth Contract Test Suite shall 各 spec（#165〜#171）固有の内部詳細を再検証せず、複数 spec を跨ぐ契約整合（end-to-end な動線・JSON 形状・横断的なエラー契約・iOS 仕様同期）のみを対象とする

## Out of Scope

- 新規エンドポイントの追加実装（既存実装の挙動を契約として固定するのみ）
- iOS クライアント実装（クライアント側の利用検証は feedman-ios 側で行う）
- push 通知 / OPML 等の v1 スコープ外機能（SERVER.md §2 以降）
- 各 spec（#165〜#171）固有の単体レベル要件の再要件化（各 spec の requirements.md が
  正本）
- 実 Google OAuth プロバイダーへの接続を伴う E2E テスト
- 鍵ローテーション（複数 kid 並行受理）の契約検証 — 単一鍵での発行・検証までを対象
- 既存 Web Cookie セッション認証フローの新規契約検証（後方互換維持の確認のみ行う）

## Open Questions

- なし（SERVER.md §1 と実装の既知差分は Requirement 4 で「承認済み逸脱」として明文化する。
  整合確認中に未承認の差分が発見された場合は AC 4.5 に従い Issue コメントで人間判断を仰ぐ）

## 関連

- Parent: #163
- Depends on: #165 #166 #167 #168 #169 #170 #171
- Sibling: #164
