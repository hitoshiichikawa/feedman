# Requirements Document

## Introduction

Feedman の Web パスキーによる再認証（ログイン）が、実ブラウザで登録された credential
に対して常に失敗する不具合を修正する。原因は、WebAuthn credential のバックアップ資格情報
属性（BackupEligible / BackupState、以下「credential backup 属性」）を登録時に永続化して
おらず、認証時に復元される credential の同属性がゼロ値（BE=false, BS=false）で組み立てられ
ているためである。実ブラウザ由来の同期パスキー（BE=1）は WebAuthn 検証層で assertion と
credential の BE フラグ一致を要求され、常に不一致で拒否される。iOS ネイティブ経路も同一
lookup を共用しているため同様に失敗する。本 spec は、登録時の credential backup 属性
永続化と認証時の反映、それらを担保するリグレッション観測点を要件として明文化する。

## Requirements

### Requirement 1: 登録時における credential backup 属性の永続化

**Objective:** As an operator, I want パスキー登録時に credential backup 属性を永続化する,
so that 認証時に登録時と同一の credential backup 属性で assertion を検証できる

#### Acceptance Criteria

1. When 未認証クライアントの新規パスキー登録 ceremony が正常完了したとき, the Passkey Registration Service shall 当該 credential の BackupEligible / BackupState 属性を credential 永続化ストアに保存する
2. When 認証済みクライアントの追加パスキー登録 ceremony が正常完了したとき, the Passkey Registration Service shall 当該 credential の BackupEligible / BackupState 属性を credential 永続化ストアに保存する
3. When 登録された authenticator が BackupEligible=true / BackupState=true を報告したとき, the Passkey Registration Service shall 両属性を true として保存する
4. When 登録された authenticator が BackupEligible=false / BackupState=false を報告したとき, the Passkey Registration Service shall 両属性を false として保存する
5. If credential backup 属性の保存に失敗したとき, the Passkey Registration Service shall 当該 credential 行と紐付く user 行を一切永続化せず、登録全体を失敗として扱う

### Requirement 2: 認証時における credential backup 属性の復元

**Objective:** As an operator, I want 認証時 lookup で保存済みの credential backup 属性を復元する, so that WebAuthn 検証層が assertion のバックアップフラグと登録時属性の一致を正しく評価できる

#### Acceptance Criteria

1. When 認証 ceremony の credential lookup が保存済み credential を解決したとき, the Passkey Authentication Service shall 復元する WebAuthn credential 表現に保存済みの BackupEligible / BackupState 属性を反映する
2. When 保存済み credential の BackupEligible=true / BackupState=true であるとき, the Passkey Authentication Service shall 検証層に対して BE=1 / BS=1 を保持した credential を提示する
3. When 保存済み credential の BackupEligible=false / BackupState=false であるとき, the Passkey Authentication Service shall 検証層に対して BE=0 / BS=0 を保持した credential を提示する
4. If lookup で credential が解決されなかったとき, the Passkey Authentication Service shall 既存契約どおり uniform 拒否として認証を失敗させる

### Requirement 3: 実環境でのパスキー再認証成功

**Objective:** As an end user, I want 実ブラウザおよびネイティブアプリで登録したパスキー
で再認証できる, so that ログインが 400 で必ず失敗する状態から復旧しユーザーが継続利用できる

#### Acceptance Criteria

1. When 実ブラウザで登録された BackupEligible=1 同期パスキーの assertion が Web 経路から到達したとき, the Passkey Authentication Service shall assertion を検証成功として受理し auth_code を発行する
2. When 実ブラウザで登録された BackupEligible=1 同期パスキーの assertion がネイティブ経路（iOS 等）から到達したとき, the Passkey Authentication Service shall assertion を検証成功として受理し auth_code を発行する
3. When BackupEligible=0 の authenticator（synthetic 等）で登録された credential の assertion が到達したとき, the Passkey Authentication Service shall 従来どおり検証成功として受理する
4. If assertion の BackupEligible フラグと登録時に保存した BackupEligible が不一致であるとき, the Passkey Authentication Service shall 拒否理由を反射せず uniform に認証失敗として扱う
5. If assertion 検証が失敗したとき, the Passkey Authentication Service shall 平文の assertion / requestBody / challenge 生値をログ・エラー・レスポンスに一切含めない

### Requirement 4: 認証成功時の credential 属性最新化ポリシー

**Objective:** As an operator, I want 認証成功時に更新される credential 属性の扱いを明文化する, so that sign_count 更新と credential backup 属性の扱いの差分が実装間で揺れない

#### Acceptance Criteria

1. When 認証 ceremony が成功したとき, the Passkey Authentication Service shall sign_count と last_used_at を既存契約どおり更新する
2. When 認証 ceremony 成功時に検証層から更新後 credential 表現が返却されたとき, the Passkey Authentication Service shall BackupState について再認証時点の観測値へ更新する（初回登録時の値を不変には保持しない）
3. The Passkey Authentication Service shall BackupEligible を再認証時に上書き更新しない（初回登録時の値を不変として保持する）

### Requirement 5: BackupEligible=1 リグレッション観測点

**Objective:** As a maintainer, I want BackupEligible=1 assertion に対する認証成功をテストで検証する, so that 同種の regression が今後 CI で自動検出される

#### Acceptance Criteria

1. When リポジトリの自動テストが実行されたとき, the Test Suite shall BackupEligible=1 で保存された credential に対し BackupEligible=1 assertion で認証が成功するケースを 1 件以上含む
2. When リポジトリの自動テストが実行されたとき, the Test Suite shall BackupEligible=0 で保存された credential に対し BackupEligible=0 assertion で認証が成功するケースを 1 件以上含む
3. When リポジトリの自動テストが実行されたとき, the Test Suite shall 保存済み BackupEligible フラグと assertion の BackupEligible フラグが不一致な場合に uniform 拒否となるケースを 1 件以上含む
4. When 既存の E2E テスト（synthetic authenticator ベース）が実行されたとき, the Test Suite shall 本修正導入後も引き続き成功する

## Non-Functional Requirements

### NFR 1: 既存データおよびスキーマとの互換性

1. The Migration shall 既存 credential 永続化ストアに対して列追加のみを行い、既存列の削除・型変更・rename を行わない
2. The Migration shall 追加する credential backup 属性列に対し、既定値を安全側（BackupEligible=false, BackupState=false）で設定する
3. When 本修正導入前に登録された credential 行（credential backup 属性列が既定値のみを持つ行）が読み出されたとき, the Passkey Authentication Service shall アプリケーションを異常終了させず、既定値のまま lookup を成立させる（結果として不一致で認証失敗となることは許容し、Out of Scope に定めるとおり削除・再登録で復旧する）

### NFR 2: セキュリティおよび可観測性

1. The Passkey Registration Service shall credential backup 属性の生値（BE/BS の boolean 以外の中間表現・raw authenticatorData 等）をログ・エラー・レスポンスに含めない
2. The Passkey Authentication Service shall 認証失敗時に「credential backup 属性の不一致」等の内部拒否理由を HTTP レスポンス本文に反射しない（既存の uniform 拒否契約を維持する）
3. When 認証拒否が発生したとき, the Passkey Authentication Service shall 運用ログとして拒否事象を記録し、challenge_id は先頭 8 文字までの prefix のみを載せる（既存規約と同一）

## Out of Scope

- ログアウト・アカウント設定導線に起因する既知の不具合（別 Issue で扱う）
- 本修正前に永続化された既存 credential 行（credential backup 属性がゼロ値のまま保存された行）の backfill・遡及補完（staging テストデータのみに限定されるため、削除および再登録で復旧する運用とする）
- WebAuthn 検証層の library バージョンアップ（v0.17.4 前提を維持する）
- credential backup 属性を運用者が閲覧・監査するための UI・API の追加

## Open Questions

- なし
