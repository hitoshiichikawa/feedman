# Requirements Document

## Introduction

ユーザー管理サービスの退会処理（`Withdraw`）は、記事状態・購読・セッション・ユーザー本体を
個別の削除呼び出しで item_states → subscriptions → sessions → user の順に順次削除している。
途中のいずれかのステップで失敗すると、それまでに削除済みのデータだけが消えた部分削除状態が
残り、退会という不可逆操作で関連データの整合性が壊れるリスクがある。本要件は、一連の削除を
原子的に扱い、全成功時のみ確定・途中失敗時は全ロールバックして退会前の状態へ戻すことで
部分削除を残さない挙動を定義する不具合修正である。共有キャッシュである feeds / items の削除や
GC、アカウント復旧・論理削除化は本件のスコープ外とする。

## Requirements

### Requirement 1: 退会処理の原子的な完了

**Objective:** As a 退会するユーザー, I want 退会時に自分の関連データが全削除されること, so that 退会後に自分のデータが部分的に残存しない

#### Acceptance Criteria

1. When 既存ユーザーに対して退会処理が実行され全ての削除ステップが成功したとき, the User Service shall 当該ユーザーの item_states・subscriptions・sessions・user を全て削除する
2. When 既存ユーザーに対して退会処理が成功したとき, the User Service shall user の削除に連動して CASCADE 対象（identities・user_settings）を削除する
3. When 既存ユーザーに対して退会処理が成功したとき, the User Service shall 共有キャッシュである feeds と items を削除せず残存させる
4. When 既存ユーザーに対して退会処理が成功したとき, the User Service shall エラーを返さず正常終了する

### Requirement 2: 途中失敗時のロールバック

**Objective:** As a 退会するユーザー, I want 退会処理が途中で失敗したら何も削除されないこと, so that 中途半端に一部データだけ消えた不整合状態に陥らない

#### Acceptance Criteria

1. If 退会処理の削除ステップ（item_states・subscriptions・sessions・user のいずれか）でエラーが発生したとき, the User Service shall 同一退会処理内で実施した全ての削除を取り消す
2. If 退会処理の削除ステップでエラーが発生したとき, the User Service shall 当該ユーザーおよびその関連データ（item_states・subscriptions・sessions・identities・user_settings）を退会処理開始前と同一の状態のまま残す
3. If 退会処理の削除ステップでエラーが発生したとき, the User Service shall 発生したエラーを呼び出し元へ返す

### Requirement 3: 存在しないユーザーの扱い

**Objective:** As a 開発者, I want 存在しないユーザー ID での退会要求を従来どおり扱えること, so that 無効な入力で予期しないデータ操作が起きない

#### Acceptance Criteria

1. If 存在しないユーザー ID を指定して退会処理が実行されたとき, the User Service shall `UserNotFound` を返す
2. If 存在しないユーザー ID を指定して退会処理が実行されたとき, the User Service shall いずれのテーブルに対しても削除を確定しない

### Requirement 4: 関連データが無いユーザーの退会

**Objective:** As a 退会するユーザー, I want 購読やセッションが 1 件も無くても退会できること, so that 利用実績の少ないアカウントでも問題なく退会できる

#### Acceptance Criteria

1. When item_states・subscriptions・sessions のいずれにも関連データを持たない既存ユーザーに対して退会処理が実行されたとき, the User Service shall エラーを返さず退会を完了する
2. When 関連データを持たない既存ユーザーの退会が完了したとき, the User Service shall 当該 user（および CASCADE 対象）を削除する

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The User Service shall `Withdraw` の呼び出しシグネチャ（引数・返り値）を本変更前と同一に保つ
2. The User Service shall 退会処理が削除する対象テーブルと CASCADE 対象（identities・user_settings）の集合を本変更前と同一に保つ
3. When 退会処理が成功するとき, the User Service shall 本変更前と同一の最終削除結果（同一テーブル群が削除済みであること）を返す

### NFR 2: 整合性（削除順序）

1. The User Service shall 退会処理における削除を外部キー制約を満たす順序（子テーブル item_states・subscriptions・sessions を先に、親テーブル user を後に削除する順序）で実施する

### NFR 3: 可観測性

1. When 退会処理が開始されるとき, the User Service shall 対象ユーザー識別子を含む退会開始のログを 1 件出力する
2. When 退会処理が成功し確定したとき, the User Service shall 対象ユーザー識別子を含む退会完了のログを 1 件出力する

## Out of Scope

- 共有キャッシュである feeds / items の削除・参照カウント・GC ロジックの導入や変更
- 退会後のアカウント復旧・取り消し機能
- 物理削除から論理削除（ソフトデリート）への切り替え（物理削除を維持する）
- `Withdraw` の削除順序・CASCADE 対象・シグネチャの変更（後方互換として維持する）
- 退会処理の原子化を実現する具体的な実装手段（トランザクション境界の張り方・リポジトリ層の IF 変更など。design / impl の領分）
- 退会 API エンドポイント（`DELETE /api/users/me`）のレスポンス仕様・HTTP ステータスの変更

## Open Questions

- なし（Issue 本文・受入基準候補・既存実装で挙動が確定しており、人間による追加決定事項コメントも存在しないため、現時点で要件レベルの曖昧点はない）
