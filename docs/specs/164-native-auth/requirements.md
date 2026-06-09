# Requirements Document

## Introduction

Feedman iOS の native token auth（親 Issue #163）では、OAuth 完了後の一時 `auth_code`、
refresh token family、refresh token のローテーション状態をサーバー側で安全に保持する
必要がある。現在は Cookie セッション前提の保存処理しか存在せず、native auth に必要な
スキーマ・モデル・リポジトリが揃っていない。

本 spec は、後続 Issue（#165 〜 #172）が依存する **永続化レイヤー**（migration / model /
repository の interface と implementation）のみを対象とし、handler / middleware / OAuth
callback 実装は扱わない。`auth_code` および refresh token は平文保存を禁止し、hash 化と
TTL・単回利用・rotation 状態の表現を要件として明示する。

## Requirements

### Requirement 1: Native auth 用ストレージスキーマの追加

**Objective:** As a Feedman API 開発者, I want native auth code / refresh token family /
refresh token を保存するためのスキーマを追加したい, so that 後続の token endpoint 実装が
このスキーマを使って認証情報を保存・検証できる

#### Acceptance Criteria

1. When 新規追加された native auth 用 migration が適用されたとき, the Native Auth Persistence Module shall native auth code を保存する領域を作成する
2. When 新規追加された native auth 用 migration が適用されたとき, the Native Auth Persistence Module shall refresh token family を保存する領域を作成する
3. When 新規追加された native auth 用 migration が適用されたとき, the Native Auth Persistence Module shall refresh token を保存する領域を作成する
4. When 追加された migration の down 操作が適用されたとき, the Native Auth Persistence Module shall 本 spec で追加した領域を削除し、既存 Cookie session 用スキーマを変更しない状態に戻す
5. The Native Auth Persistence Module shall 既存 Cookie session 用のスキーマと挙動を変更しない

### Requirement 2: Auth code の保存と単回利用・TTL 表現

**Objective:** As a Feedman API 開発者, I want auth code を hash 保存し、60 秒の TTL と単回
利用を表現可能にしたい, so that OAuth callback で発行した一時 code を安全に保管し、後続の
token 交換で検証・無効化できる

#### Acceptance Criteria

1. When auth code が保存されるとき, the Native Auth Persistence Module shall code 本体ではなく code の hash 値のみを保存する
2. When auth code が保存されるとき, the Native Auth Persistence Module shall ユーザー識別子・PKCE S256 challenge・有効期限・使用済みフラグを併せて保存する
3. When auth code レコードが作成されるとき, the Native Auth Persistence Module shall 有効期限を発行時刻から 60 秒後に設定できるよう有効期限を呼び出し側から受け取る
4. When 保存済みの auth code を hash 一致で参照するとき, the Native Auth Persistence Module shall 該当 1 件のレコードを返し、ヒットしない場合は not-found 相当のエラーを返す
5. When auth code を「使用済み」として確定するとき, the Native Auth Persistence Module shall 当該レコードの使用済みフラグを set し、以降の参照で使用済みと判定できる状態にする
6. If 既に使用済み、もしくは有効期限を超過した auth code を使用済みとして確定しようとしたとき, the Native Auth Persistence Module shall 使用済み確定を成功させず、呼び出し側に失敗を通知する
7. The Native Auth Persistence Module shall auth code の平文を永続化領域・ログ出力・エラーメッセージのいずれにも残さない

### Requirement 3: Refresh token family と rotation 状態の表現

**Objective:** As a Feedman API 開発者, I want refresh token を hash 保存し、token family と
rotation 状態・revocation 状態を表現したい, so that refresh rotation と再利用検知
（後続 Issue #167 / #168）を実装可能な永続化レイヤーを提供できる

#### Acceptance Criteria

1. When refresh token が保存されるとき, the Native Auth Persistence Module shall token 本体ではなく token の hash 値のみを保存する
2. When refresh token が保存されるとき, the Native Auth Persistence Module shall family 識別子・ユーザー識別子・有効期限・rotation メタデータ・revocation 状態を併せて保存する
3. When 同一 family 内で refresh token が rotation されたとき, the Native Auth Persistence Module shall 直前の token を rotation 済みとして識別でき、最新の token を family 内で参照可能な状態にする
4. When family 単位で revoke が要求されたとき, the Native Auth Persistence Module shall 当該 family に属する全ての refresh token を revoke 済み状態に遷移させる
5. When 保存済みの refresh token を hash 一致で参照するとき, the Native Auth Persistence Module shall 該当 1 件のレコードを返し、ヒットしない場合は not-found 相当のエラーを返す
6. When ユーザー単位の削除（アカウント削除）に対応するため、ユーザー識別子で refresh token と family の一括削除が要求されたとき, the Native Auth Persistence Module shall 当該ユーザーに属する全ての refresh token と family を削除する
7. The Native Auth Persistence Module shall refresh token の平文を永続化領域・ログ出力・エラーメッセージのいずれにも残さない

### Requirement 4: Repository interface の境界

**Objective:** As a後続 Issue（#165 〜 #168, #170）の実装者, I want native auth 用 repository が
小さく分離された interface として提供されることを期待する, so that 各 handler / service が
必要最小限の interface に依存し、テストでも差し替え可能になる

#### Acceptance Criteria

1. The Native Auth Persistence Module shall auth code 永続化操作と refresh token 永続化操作を別個の interface として公開する
2. The Native Auth Persistence Module shall 各 interface に、保存・hash 一致での参照・状態遷移（使用済み化 / rotation / revoke）・ユーザー単位削除のうち、本 spec の他要件で定めた操作のみを含める
3. Where テストコードが対象となるとき, the Native Auth Persistence Module shall interface 単位で差し替え可能であり、実 DB を使わずに依存側のテストを記述できる
4. When repository implementation のテストが実行されるとき, the Native Auth Persistence Module shall 作成・参照・使用済み化・rotation メタデータ更新・family 単位 revoke・ユーザー単位削除の各挙動を検証する

## Non-Functional Requirements

### NFR 1: セキュリティ（機密値の取り扱い）

1. The Native Auth Persistence Module shall auth code と refresh token の本体（平文）を 0 件、永続化領域に書き込まない
2. The Native Auth Persistence Module shall auth code と refresh token の本体（平文）を 0 件、構造化ログ・標準出力・エラーメッセージに含めない
3. The Native Auth Persistence Module shall hash 値の比較を一致確認のみに用い、復号可能な暗号化処理に置き換えない

### NFR 2: 後方互換性

1. The Native Auth Persistence Module shall 既存 Cookie session 用のテーブル・カラム・index を追加・変更・削除しない
2. When 本 spec の migration が適用されたとき, the Native Auth Persistence Module shall 既存の Cookie session を利用した Web ログインフローを 100% 維持する

### NFR 3: テスト容易性

1. The Native Auth Persistence Module shall repository implementation のテストを、外部ネットワーク依存（Google OAuth・はてなブックマーク API 等）なしで完結させる
2. The Native Auth Persistence Module shall repository implementation のテストにおいて、本 spec の各 AC に対して正常系と異常系（not-found / 使用済み / 期限切れ / 二重 revoke）の少なくとも一方が境界値ケースを含む形でカバーされている状態にする

## Out of Scope

- OAuth callback における `flow=native` の redirect 実装（Issue #165）
- `POST /api/auth/token` の code-to-token 交換 handler 実装（Issue #166）
- `POST /api/auth/refresh` の rotation handler 実装（Issue #167）
- `POST /api/auth/revoke` および refresh token 再利用検知 handler 実装（Issue #168）
- Bearer-or-Session middleware の導入（Issue #169）
- アカウント削除時の native auth クリーンアップ呼び出し（Issue #170 — 本 spec はユーザー単位削除の repository 操作のみ提供）
- Native auth endpoint のレート制限（Issue #171）
- Contract / integration テストの全体整備（Issue #172）
- access token（Bearer 本体）の保存。access token は短命・stateless 想定で永続化対象外
- 既存 Cookie session 用スキーマのリファクタや改修

## Open Questions

- auth code の TTL は Issue 本文記載どおり 60 秒とするが、テスト容易性のため有効期限は呼び出し側から渡す方針で問題ないか（本 spec は呼び出し側から有効期限を受け取る前提で記述）
- refresh token の rotation メタデータの最小集合（直前 token への参照 / rotation 時刻 / rotation 回数 など）については design.md（Architect）の領分とし、本 spec では「rotation 状態を識別可能であること」までを要件として規定
- ユーザー単位削除はアカウント削除（Issue #170）に備えた repository 操作のみ提供する。アカウント削除フロー本体での呼び出し統合は Issue #170 で扱う

## 関連

- Parent: #163
- Sibling: #165 #166 #167 #168 #169 #170 #171 #172
