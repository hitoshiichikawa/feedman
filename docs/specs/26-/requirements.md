# Requirements Document

## Introduction

`api` のエラーレスポンス処理には、フォーマット定義（フィールド構成と JSON タグ）と書き込み処理が
2 箇所に重複して定義されており、両者はフィールド構成・JSON タグ（`code`/`message`/`category`/`action`）・
処理内容が完全に等価な二重定義となっている。一方の定義は複数のエンドポイント（フィード / アイテム /
購読 / ユーザー）の約 20 箇所から呼ばれており、保守時に両方を更新し忘れるとレスポンス整合性が崩れる
リスクがある。本件は技術債返済のリファクタとして、この二重定義を単一実装へ集約し、ユーザー可視の
振る舞いを一切変えずに重複を解消することを目的とする。

## Requirements

### Requirement 1: エラーレスポンス実装の単一化

**Objective:** As a 保守担当エンジニア, I want エラーレスポンスのフォーマット定義と書き込み処理が単一実装に集約されること, so that 二重定義による更新漏れリスクを排除し保守性を高められる

#### Acceptance Criteria

1. The API バックエンド shall エラーレスポンスのフォーマット定義（フィールド構成と JSON タグ）を単一の定義に集約する
2. The API バックエンド shall エラーレスポンスの書き込み処理を単一の実装に集約する
3. When リファクタ完了後にエラーレスポンス処理を参照したとき, the API バックエンド shall フィールド構成・JSON タグ・処理内容が等価な重複定義を 1 つも残さない
4. The 各エンドポイントのエラー応答箇所 shall 集約後の単一実装を経由してエラーレスポンスを書き込む
5. Where フィード登録時の URL 空チェックなど既存の入力分岐が存在する場合, the 各エンドポイント shall 集約後の単一実装を経由して当該分岐のエラーレスポンスを返す

### Requirement 2: 後方互換性の維持（レスポンス不変）

**Objective:** As a API 利用者（フロントエンド / 外部クライアント）, I want エラーレスポンスの構造とステータスコードが統一前後で完全に同一であること, so that 既存クライアントの挙動が一切影響を受けない

#### Acceptance Criteria

1. When 任意のエンドポイントがエラーを返したとき, the API バックエンド shall 統一前と同一の JSON フィールド（`code`/`message`/`category`/`action`）と JSON タグ名を持つレスポンスボディを返す
2. When 任意のエンドポイントがエラーを返したとき, the API バックエンド shall 統一前と同一の HTTP ステータスコードを返す
3. When 任意のエンドポイントがエラーを返したとき, the API バックエンド shall レスポンスの `Content-Type` を `application/json` とする
4. If 認証情報が無い状態でエンドポイントへアクセスされたとき, the API バックエンド shall HTTP 401 と `code` = `UNAUTHORIZED` のレスポンスを統一前と同一内容で返す
5. If リクエストボディの解析に失敗したとき, the API バックエンド shall HTTP 400 と `code` = `INVALID_REQUEST` のレスポンスを統一前と同一内容で返す
6. If 指定されたフィードが存在しないとき, the API バックエンド shall HTTP 404 と `code` = `FEED_NOT_FOUND` のレスポンスを統一前と同一内容で返す
7. If API エラーでない内部エラーが発生したとき, the API バックエンド shall HTTP 500 と `code` = `INTERNAL_ERROR`・`category` = `system` のレスポンスを統一前と同一内容で返す
8. When サービス層が API エラーを返したとき, the API バックエンド shall 統一前と同一のエラーコード→ステータスコード対応（例: `INVALID_URL`→400, `SSRF_BLOCKED`→403, `SUBSCRIPTION_LIMIT`→409, `ITEM_NOT_FOUND`→404 等）でレスポンスを返す

### Requirement 3: ビルドとテストの通過

**Objective:** As a 開発チーム, I want 統一後もビルドと全テストが通過すること, so that リファクタによる退行が混入していないことを保証できる

#### Acceptance Criteria

1. When リファクタ後にバックエンドをビルドしたとき, the ビルドプロセス shall コンパイルエラーなく完了する
2. When リファクタ後に既存テストスイートを実行したとき, the テストプロセス shall 既存の全テストを通過させる
3. The テストスイート shall 集約後の単一実装に対する書き込み処理・ステータスコード・フィールド存在の検証ケースを保持する

## Non-Functional Requirements

### NFR 1: 互換性

1. The API バックエンド shall リファクタによってモジュール間の依存関係の循環を新たに発生させない
2. The リファクタ shall エラーレスポンスに関わる外部から観測可能な振る舞いを差分等価（挙動変更なし）に保つ

### NFR 2: 保守性

1. The API バックエンド shall エラーレスポンスフォーマット変更時に 1 箇所のみの修正で全エンドポイントへ反映できる単一定義を提供する

## Out of Scope

- エラーレスポンスのフィールド構成（`code`/`message`/`category`/`action`）の追加・削除・名称変更
- `category` / `action` の文言や分類ポリシーの見直し
- エラーコード→HTTP ステータスコードのマッピング規則自体の変更
- 新規エラーケース・エンドポイントの追加
- フロントエンド（`web`）側のエラーハンドリング変更
- ログ出力フォーマット・ログ項目の変更

## Open Questions

- なし（実装判断として「どのモジュールへ集約するか」は design / 実装者に委ねる。要件としては単一実装への集約のみを要求する）
