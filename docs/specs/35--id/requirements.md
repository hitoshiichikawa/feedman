# Requirements Document

## Introduction

Feedman の OAuth ログインフロー（認可コールバック処理）では、認可成功後にセッション Cookie を
発行する際、ログイン前から存在する既存セッションを無効化・再発行（rotate）する処理がない。
このため、攻撃者が事前に被害者のブラウザへセッション識別子を植え込めると、被害者のログイン完了後も
その植え込まれた識別子が有効なまま残り得る（セッション固定攻撃 / Session Fixation の余地）。
認証境界をまたぐ際にセッション識別子を必ず旋回させるのは OWASP（ASVS V3.2.1 Session Management）の
基本対策である。本要件は、ログイン成功時に旧セッションを確実に無効化し、新規発行された識別子のみが
有効になることを、観測可能な挙動として規定する。既存の Cookie 名・属性・リダイレクト挙動の後方互換は
維持する。

## Requirements

### Requirement 1: ログイン成功時の旧セッション無効化

**Objective:** As a Feedman を利用するエンドユーザー, I want ログイン成功時にログイン前から存在していたセッションが無効化されること, so that 攻撃者が事前に植え込んだセッション識別子がログイン後に使えなくなり、セッション固定攻撃の被害を受けない

#### Acceptance Criteria

1. When 既存の `session_id` Cookie を保持した状態でログインフローが認可に成功したとき, the Auth サービス shall その旧 `session_id` に対応する保存済みセッションを無効化する
2. When ログインが完了した後に旧 `session_id` を用いて認証付きリクエストが送られたとき, the Auth サービス shall そのリクエストを認証済みとして扱わず拒否する
3. While ログイン前から有効な `session_id` Cookie が存在する状態, when 同じユーザーがログインに成功したとき, the Auth サービス shall ログイン前の識別子とログイン後に有効な識別子が同一にならないようにする

### Requirement 2: 新規セッションの発行と整合性

**Objective:** As a Feedman を利用するエンドユーザー, I want ログイン成功時に新しいセッション識別子のみが有効になること, so that ログイン後の認証状態が攻撃者の関与しない新しい識別子のみに紐づく

#### Acceptance Criteria

1. When ログインフローが認可に成功したとき, the Auth サービス shall 新しい `session_id` を払い出してセッション Cookie に設定する
2. When ログインに成功して新しい `session_id` が払い出されたとき, the Auth サービス shall その新しい識別子をログイン前から保持していた `session_id` と異なる値とする
3. When ログイン完了後に新しい `session_id` を用いて認証付きリクエストが送られたとき, the Auth サービス shall そのリクエストを認証済みとして受け付ける

### Requirement 3: 正常系・境界系でのログイン継続性

**Objective:** As a Feedman を利用するエンドユーザー, I want セッション識別子の有無や状態に関わらずログインが正常に完了すること, so that セッション旋回の追加によって既存の正常なログイン体験が損なわれない

#### Acceptance Criteria

1. When `session_id` Cookie が存在しない状態でログインフローが認可に成功したとき, the Auth サービス shall 新規セッションを発行してログインを正常に完了させる
2. If ログイン時に提示された旧 `session_id` に対応する保存済みセッションが存在しない（既に期限切れ・削除済み等）とき, the Auth サービス shall ログインをエラーにせず新規セッションの発行を伴って完了させる
3. If ログイン時の旧セッション無効化が失敗したとき, the Auth サービス shall その事象を運用者が追跡可能な形で記録する

## Non-Functional Requirements

### NFR 1: セッション識別子の旋回（セキュリティ）

1. When 未認証から認証済みへ状態が遷移するログイン成功時, the Auth サービス shall ログイン前のセッション識別子をログイン後に有効な識別子へ必ず旋回し、ログイン前の識別子をログイン後に無効化する

### NFR 2: Cookie 属性の後方互換

1. The セッション Cookie shall ログイン成功時の発行において Cookie 名・`HttpOnly` / `Secure` / `SameSite` / `Domain` / `MaxAge` の各属性を本機能導入前と同一に保つ
2. When ログインフローが認可に成功してリダイレクトするとき, the Auth サービス shall 本機能導入前と同一のリダイレクト先・リダイレクト挙動を維持する

## Out of Scope

- 通常のログアウトフロー（Logout）の挙動変更
- セッションストアの実装方式（DB スキーマ等）の変更
- 多要素認証など認証方式そのものの拡張
- 旧セッション無効化処理をどの層（コールバックハンドラー / 認証サービス）で行うかの実装配置判断（design.md / 実装フェーズの領分）
- セッション識別子の生成方式（バイト長・乱数源等）の変更

## Open Questions

- なし（Issue 本文に必要情報が揃っており、人間による決定事項コメントもないため）
