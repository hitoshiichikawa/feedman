# Requirements Document

## Introduction

ユーザー退会（アカウント削除）時、native auth（親 Issue #163）が導入した認証状態 ——
一時認可コード・refresh token・その rotation family —— が削除されずに残ると、削除済み
ユーザーに紐づく認証情報が永続化領域に残存する。本 spec は **退会フローにおける
native auth 認証状態の cleanup** のみを対象とし、(1) 退会完了時に当該ユーザーの認可コード /
refresh token / rotation family が残存しないこと、(2) cleanup が既存退会フローと同一の
原子性境界で実行され部分的な削除状態を確定しないこと、(3) 他ユーザーの認証状態に影響
しないこと、を要件化する。永続化操作には Issue #164 の native auth 永続化レイヤーを前提と
する。token endpoint の実装（#166 / #167 / #168）、退会 UI の変更、期限切れレコードの
定期削除はスコープ外。

## Requirements

### Requirement 1: 退会時の native auth 認証状態の削除

**Objective:** As a Feedman 運用者, I want 退会したユーザーの native auth 認証状態が
残存しないこと, so that 削除済みアカウントに紐づく認証情報の残存・悪用リスクを排除できる

#### Acceptance Criteria

1. When ユーザーの退会処理が完了したとき, the User Withdrawal Service shall 当該ユーザーの native auth 一時認可コードをすべて削除する
2. When ユーザーの退会処理が完了したとき, the User Withdrawal Service shall 当該ユーザーの refresh token とその rotation family をすべて削除する

### Requirement 2: 退会フローへの統合と原子性

**Objective:** As a Feedman 運用者, I want native auth cleanup が既存退会処理と一体で
原子的に実行されてほしい, so that 途中失敗時に退会と認証状態削除の不整合が残らない

#### Acceptance Criteria

1. The User Withdrawal Service shall native auth 認証状態の削除を、既存の退会削除フローと同一の処理単位（同一の原子性境界）内で実行する
2. If 退会処理の途中で native auth 認証状態の削除が失敗したとき, the User Withdrawal Service shall 退会処理全体を失敗させ、それまでの削除を確定しない
3. If 退会処理が native auth 認証状態の削除より後の段階で失敗したとき, the User Withdrawal Service shall native auth 認証状態の削除を確定しない

### Requirement 3: 他ユーザーへの非影響

**Objective:** As a Feedman ユーザー, I want 自分の認証状態が他ユーザーの退会で消えない
こと, so that 無関係な退会によって再認証手段やログイン状態を失わない

#### Acceptance Criteria

1. When あるユーザーの退会処理が実行されたとき, the User Withdrawal Service shall 他ユーザーの認可コード・refresh token・rotation family を削除も失効もしない

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The User Withdrawal API shall 成功・失敗の応答形式（response shape）を本機能の導入前と同一とする
2. The Feedman システム shall 退会以外の native auth 操作（認可コードの発行・参照・消費、refresh token の発行）の挙動を変更しない
3. The Feedman システム shall 本機能をデータ移行・保存構造の変更なしに導入する

### NFR 2: 検証可能性

1. The Withdrawal Test Suite shall 退会完了後に当該ユーザーの認可コード・refresh token・rotation family が残存しないことを明示的に検証する（保存構造上の自動削除に依拠する経路についても、期待挙動をテストとして明文化する）

## Out of Scope

- 退会 UI の変更
- token endpoint の新規実装（#166 / #167 / #168 の領分）
- 既存ユーザー削除仕様（削除対象・順序・応答）の再設計
- 期限切れ認可コード / refresh token の定期削除（cleanup worker）
- ログアウト・revoke 操作（#168 の領分）

## Open Questions

- Issue の判断委任点「DB constraint と service-level cleanup の両方で担保するか、既存
  repository pattern に合わせて最小実装を選ぶか」については、feedman-ios `design/SERVER.md`
  §1.5 の「退会処理への明示削除の追加（自動削除でも担保されるが明示推奨）」に従い、
  **既存退会フローへの明示削除の統合を要件とし、保存構造上の自動削除は防衛線として温存する**
  前提で design に委ねる

## 関連

- Parent: #163
- Depends on: #164
- Sibling: #165 #166 #167 #168 #169 #171 #172
