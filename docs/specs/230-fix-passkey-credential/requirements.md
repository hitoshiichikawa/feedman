# Requirements Document

## Introduction

Feedman iOS の新規パスキー登録の finish 段階で、ユーザーの永続化と最初のパスキー credential の
永続化が別段階で確定される実装になっており、credential 側の失敗（既登録 credential との重複・
インフラ障害等）が発生すると credential を持たない孤立ユーザーだけが Feedman に残り得る。
この状態は Issue #216 で確定済みの要件「不正な attestation / credential 重複等ではアカウントを
作成しない」（#216 Req 1.7）および同 design が定めた「登録 finish のユーザー永続化と最初の
パスキー credential 永続化の合成成功を単一の原子性境界で保証する」トランザクション境界に反する
実装バグである。

本 spec は #216 の既存要件・設計を正としたまま、新規パスキー登録 finish の永続化を
「両方保存 or 両方永続化なし」の原子性を持って実行する期待挙動へ復旧させることを目的とする。
#216 で公開済みの API 契約・拒否時 uniform エラー応答・iOS クライアント互換性は本修正で
変更しない。追加登録（認証済みユーザーが既存アカウントに新たなパスキーを追加する経路）は
credential 単体の永続化で本 Issue の対象外とする。

## Requirements

### Requirement 1: 新規パスキー登録 finish におけるユーザーと最初の credential の原子性保証

**Objective:** As a Feedman iOS 新規ユーザー, I want パスキー新規登録が成功したときにだけ自分の
アカウントと最初のパスキー credential の両方が Feedman に登録されていてほしい, so that credential
保存の失敗によって「credential を持たずログイン手段のない孤立アカウント」が Feedman に残る
状態を避けられる

#### Acceptance Criteria

1. When 新規パスキー登録 finish が成功したとき, the Passkey Registration Service shall 対応する
   ユーザーと最初のパスキー credential の両方を永続化する
2. When 新規パスキー登録 finish が成功したとき, the Passkey Authentication Service shall 直後の
   パスキー認証において当該ユーザーを当該 credential 経由で解決可能な状態にする
3. If 新規パスキー登録 finish において最初のパスキー credential の永続化が既登録 credential との
   重複により失敗したとき, the Passkey Registration Service shall 当該登録要求に対応する
   ユーザーを永続化しない
4. If 新規パスキー登録 finish において最初のパスキー credential の永続化がインフラ障害により
   失敗したとき, the Passkey Registration Service shall 当該登録要求に対応するユーザーを
   永続化しない
5. If 新規パスキー登録 finish においてユーザーの永続化がユーザー名重複により失敗したとき,
   the Passkey Registration Service shall 当該登録要求に対応するパスキー credential を
   永続化しない
6. If 新規パスキー登録 finish の永続化が失敗したとき, the Passkey Registration Service shall
   部分的に永続化されたユーザーまたはパスキー credential を残さず、以降のパスキー認証および
   同一ユーザー名での新規パスキー登録開始要求から見て「未登録」状態のみを観察可能にする

### Requirement 2: 既存 #216 API 契約およびエラー秘匿の維持

**Objective:** As a Feedman iOS クライアントおよび #216 で公開済み API を利用する既存
クライアント, I want 本修正によって新規パスキー登録 API の外部契約が変化しないでほしい,
so that クライアント側実装や #216 で確定した iOS 互換性を再度検証し直さずに済む

#### Acceptance Criteria

1. When 新規パスキー登録 finish が成功したとき, the Passkey Registration Endpoint shall #216 で
   公開済みの成功時 HTTP status および応答本文の形式を本修正導入前と同一に維持する
2. If 新規パスキー登録 finish が拒否されたとき, the Passkey Registration Endpoint shall #216 で
   公開済みの拒否時 HTTP status および error code（uniform 拒否契約）を本修正導入前と同一に
   維持する
3. If 新規パスキー登録 finish が拒否されたとき, the Passkey Registration Service shall 拒否理由
   の内部詳細（credential 重複 / インフラ障害 / ユーザー名 race 等の区別）をクライアント応答に
   反射しない
4. The Passkey Registration Service and Passkey Registration Endpoint shall #216 で公開済みの
   iOS クライアント向け API 契約（登録開始・登録応答検証の要求形式と応答形式）を破壊的に
   変更しない

## Non-Functional Requirements

### NFR 1: データ整合性（部分 commit の禁止）

1. The Passkey Registration Service shall 新規パスキー登録 finish において、ユーザーと最初の
   パスキー credential のいずれか一方のみが永続化された状態を許容しない
2. If 新規パスキー登録 finish の永続化が中断・障害・重複衝突のいずれかで失敗したとき,
   the Passkey Registration Service shall 事前チェックだけでなく永続化本体においても部分
   commit を許容しない原子性境界内で処理する

### NFR 2: 可観測性（拒否事象のログ記録）

1. When 新規パスキー登録 finish が拒否されたとき, the Passkey Registration Service shall
   #216 NFR 3.1 に準じて拒否事象を運用ログとして記録する
2. The Passkey Registration Service shall 拒否ログに #216 NFR 3.2 で禁止された情報（パスキー生
   応答・チャレンジ平文・auth_code 平文・ユーザーのリカバリ用メール本文）を含めない

### NFR 3: 後方互換性

1. The Feedman システム shall 本修正をデータ移行・既存テーブル構造の破壊的変更なしに導入する
2. The Existing Native Auth Contract Tests and #216 のパスキー登録・認証・退会 cleanup に関する
   既存テストスイート shall 本修正導入後も無変更で green を維持する

### NFR 4: テスト容易性

1. The Passkey Registration Test Suite shall 最初のパスキー credential の永続化が既登録
   credential 重複で拒否された場合に、対応するユーザーが永続化されていないことを外部ネットワーク
   依存なしで検証可能にする
2. The Passkey Registration Test Suite shall 最初のパスキー credential の永続化がインフラ障害で
   失敗した場合に、対応するユーザーが永続化されていないことを外部ネットワーク依存なしで
   検証可能にする
3. The Passkey Registration Test Suite shall ユーザーの永続化がユーザー名重複で失敗した場合に、
   対応するパスキー credential が永続化されていないことを外部ネットワーク依存なしで検証可能に
   する
4. The Passkey Registration Test Suite shall 新規パスキー登録 finish の成功時にユーザーと最初の
   パスキー credential の両方が永続化されており、以降のパスキー認証で当該ユーザーが解決される
   ことを外部ネットワーク依存なしで検証可能にする

## Out of Scope

- 追加登録（認証済みユーザーが既存アカウントに新しいパスキーを追加する経路）の挙動変更。
  追加登録は credential 単体の永続化で本 Issue の対象外とし、原子性境界の変更対象としない
- Web 登録直後の Cookie セッション合流方式の再設計
- PR #229 の UI 文言および「登録完了不明」状態の最終設計
- #216 で公開済みの外部 API を破壊的に変更すること（本修正は API 契約を維持したまま実装
  バグを直すスコープに閉じる）
- #216 の既存要件・設計そのものの変更（本修正は #216 design.md が定めるトランザクション境界
  どおりに実装バグを直すのみ）
- Passkey Authentication Service（パスキー認証）・Passkey Challenge Store（チャレンジ
  ライフサイクル）・User Withdrawal Service（退会 cleanup）・IP レート制限・ドメイン所有権
  表明配信の挙動変更

## Open Questions

- なし（本 Issue は #216 の既存要件・設計を正として実装バグを修正するスコープに閉じており、
  新規の設計判断は含まない）

## 関連

- Parent: #216
- Related: #229
