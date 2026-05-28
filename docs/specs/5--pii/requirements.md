# Requirements Document

## Introduction

新規ユーザー作成時のログ（`internal/auth/service.go` の「new user created」ログ）で、
ユーザーのメールアドレス（PII）が平文のまま出力されている。GDPR・個人情報保護法の観点から、
PII を平文でログに残すことは避けるべきである。本要件では、当該ログ出力からメールアドレスの
平文を排除し、代わりに元の値を復元できないマスク形式（例: `h***@example.com`）で出力する。
`user_id` / `provider` など PII でない識別情報は引き続き出力し、後方互換のためログのキーは
不必要に変更しない。本対応は人間の確定判断により Option B（マスク出力）を採用する。

## Requirements

### Requirement 1: 新規ユーザー作成ログからの PII 平文除去

**Objective:** As a 運用者（ログ閲覧者）, I want 新規ユーザー作成ログにメールアドレスの平文が含まれない, so that PII の平文がログに残らずプライバシー保護義務を満たせる

#### Acceptance Criteria

1. When 新規ユーザーが作成されログが出力される, the 認証サービス shall メールアドレスの平文を当該ログに含めない
2. When 新規ユーザーが作成されログが出力される, the 認証サービス shall メールアドレスをマスクした値を出力する

### Requirement 2: メールアドレスのマスク形式

**Objective:** As a 運用者, I want ログ上のメールアドレスが復元不能な形式でマスクされる, so that マスク値から元のメールアドレスを推測・復元できない

#### Acceptance Criteria

1. When 有効な形式のメールアドレスをマスク出力する, the 認証サービス shall ローカル部の先頭一部のみを残し残余を伏字に置換した値を出力する（例: `h***@example.com`）
2. When 有効な形式のメールアドレスをマスク出力する, the 認証サービス shall ドメイン部（`@` 以降）をそのまま保持する
3. The マスク出力 shall 元のメールアドレスを復元できない形式である

### Requirement 3: PII でない識別情報の継続出力と後方互換

**Objective:** As a 運用者, I want PII でない識別情報がこれまで通りログに出力され、ログのキーが不必要に変わらない, so that 既存のログ解析・監視がそのまま機能し続ける

#### Acceptance Criteria

1. When 新規ユーザーが作成されログが出力される, the 認証サービス shall `user_id` の値をこれまで通り出力する
2. When 新規ユーザーが作成されログが出力される, the 認証サービス shall `provider` の値をこれまで通り出力する
3. The 認証サービス shall ログのキー名 `user_id` および `provider` を変更しない

### Requirement 4: 空入力・不正形式メールアドレスの安全な処理

**Objective:** As a 運用者, I want 空文字や不正形式のメールアドレスでもマスク処理が安全に完了する, so that ログ出力処理が異常入力でクラッシュせず認証フローを阻害しない

#### Acceptance Criteria

1. If メールアドレスが空文字である, the 認証サービス shall パニック・エラーを発生させずマスク処理を完了する
2. If メールアドレスが `@` を含まない不正形式である, the 認証サービス shall パニック・エラーを発生させずマスク処理を完了する
3. If メールアドレスが空文字または不正形式である, the 認証サービス shall 復元可能なメールアドレスの平文を出力しない

## Non-Functional Requirements

### NFR 1: プライバシー・セキュリティ

1. The 認証サービス shall 新規ユーザー作成ログにメールアドレスの平文（`@` を含む復元可能な完全な値）を出力しない

### NFR 2: 後方互換性

1. The 認証サービス shall 本対応の前後で `user_id` / `provider` のログキー名および出力有無を変えない

## Out of Scope

- ログ出力以外の箇所（DB 保存、API レスポンス等）でのメールアドレスの扱い
- 既存（過去出力済み）ログの遡及的なマスキング・削除
- 「existing user logged in」など、もともとメールアドレスを出力していないログの変更
- メールアドレス以外の PII（氏名等）のマスク方針の見直し

## Open Questions

- なし（マスク採用方針は人間判断により Option B（マスク出力）で確定済み）
