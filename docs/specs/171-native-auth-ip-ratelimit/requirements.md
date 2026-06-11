# Requirements Document

## Introduction

native auth エンドポイント群（親 Issue #163）のうち `POST /api/auth/token`（#166）・
`POST /api/auth/refresh`（#167）・`POST /api/auth/revoke`（#168）は、Cookie セッションも
Bearer 認証も持たない未認証状態から呼び出される。これらは token 試行・フラッディングの
入口になりうるため、SERVER.md §1.7 が要求する未認証 IP rate limit による保護が必要になる。

Feedman には Issue #38 で導入済みの未認証エンドポイント向け IP 単位レート制限
（`/auth/google/login`・`/auth/google/callback`・`/health` に適用、閾値超過時に
HTTP 429 を返す）が存在する。本 spec は **この既存保護を native auth の 3 エンドポイントへ
拡張適用すること**のみを対象とする。制限方式そのもの（クライアント IP の判定方針・閾値の
設定可能性・内部状態の資源管理）は #38 で要件化済みであり、本 spec では再定義しない。

なお Issue 本文の仮案は「`/api/auth/revoke` は Bearer 認証下想定のため対象外」としていたが、
#168 の要件定義で revoke は「未認証 + token 所持 = 権限」の境界が採用され、未認証で
公開されることが確定した（`docs/specs/168-reuse-detection-revoke/requirements.md` の
Open Questions）。このため本 spec は revoke も保護対象に含める。

## Requirements

### Requirement 1: native auth エンドポイントへの IP 単位レート制限

**Objective:** As a サービス運用者, I want native auth エンドポイントを同一 IP 単位で
レート制限したい, so that 未認証で公開される token 系エンドポイントへの総当たり・
フラッディングからサービスを保護できる

#### Acceptance Criteria

1. When 同一クライアント IP から token 交換エンドポイントへの単位時間あたりリクエスト数が閾値を超過したとき, the IP レート制限 shall token 交換処理を試行せずに HTTP 429 Too Many Requests を返す
2. When 同一クライアント IP から refresh エンドポイントへの単位時間あたりリクエスト数が閾値を超過したとき, the IP レート制限 shall refresh 処理を試行せずに HTTP 429 Too Many Requests を返す
3. When 同一クライアント IP から revoke エンドポイントへの単位時間あたりリクエスト数が閾値を超過したとき, the IP レート制限 shall 失効処理を試行せずに HTTP 429 Too Many Requests を返す
4. While 同一クライアント IP からのリクエスト数が閾値以内であるとき, the IP レート制限 shall token 交換・refresh・revoke の各要求を後続処理へ通常どおり通過させる
5. When 閾値超過により拒否したとき, the IP レート制限 shall 再試行可能になるまでの待機時間をレスポンスヘッダーで通知する
6. When 異なるクライアント IP から native auth エンドポイントへリクエストが到達したとき, the IP レート制限 shall 各 IP のリクエスト数を独立にカウントする

### Requirement 2: 既存挙動の後方互換

**Objective:** As a 既存利用者・運用者, I want 既存エンドポイントのレート制限挙動と
エラー応答契約が変わらないでほしい, so that 本変更を既存環境へ無調整で導入できる

#### Acceptance Criteria

1. The システム shall 既存未認証エンドポイント（Google ログイン入口・コールバック・ヘルスチェック）への IP 単位レート制限の適用範囲・閾値を本変更導入前と同一に維持する
2. The システム shall 認証済みエンドポイントの userID 単位レート制限挙動を本変更導入前と同一に維持する
3. The IP レート制限 shall native auth エンドポイントでの閾値超過応答を既存未認証エンドポイントの閾値超過応答と同一形式とする
4. While native auth 機能が無効な環境（native auth エンドポイントが公開されない環境）であるとき, the システム shall 本変更導入前と同一の挙動を維持する

## Non-Functional Requirements

### NFR 1: 可観測性

1. When native auth エンドポイントで IP 単位レート制限による拒否が発生したとき, the IP レート制限 shall 拒否事象を運用ログとして記録する
2. The IP レート制限 shall 拒否ログに token・セッション情報・個人を特定しうる情報を含めない

### NFR 2: 互換性・運用

1. While IP 単位レート制限の設定値が未指定であるとき, the システム shall 設定追加なしに native auth エンドポイントへの制限を既定値で有効化する
2. The システム shall 既存ルートに適用されているミドルウェアの種類・順序を変更しない

### NFR 3: テスト容易性

1. The レート制限テスト shall token 交換・refresh・revoke の各エンドポイントについて、閾値以内の通過と閾値超過の拒否を外部ネットワーク依存なしで検証可能にする

## Out of Scope

- グローバル rate limit 設計の置き換え・制限方式（IP 判定・資源管理・閾値設定機構）の再設計（#38 で要件化済み）
- 認証済み既存 API（`/api/*` の userID 単位制限）のレート制限変更
- push notification 等、v1 スコープ外エンドポイントの保護
- 分散環境で複数インスタンス間にレート制限状態を共有する外部ストアの導入
- Bearer middleware（#169）/ contract tests 全体整備（#172）

## Open Questions

- `/api/auth/revoke` の包含: Issue 仮案（Bearer 認証下想定で対象外）に対し、#168 で
  「未認証 + token 所持 = 権限」の境界が確定したため対象に含める（Introduction 参照）。
  revoke は破壊操作のみで token 奪取には使えないが、未認証で永続化層の参照を伴うため、
  資源保護として他 2 エンドポイントと同一の制限を適用する

## 関連

- Parent: #163
- Depends on: #166 #167 #168
- Related: #38
- Sibling: #164 #165 #169 #170 #172
