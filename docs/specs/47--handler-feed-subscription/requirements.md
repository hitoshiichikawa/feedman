# Requirements Document

## Introduction

Feedman の `internal/handler` / `internal/feed` / `internal/subscription` には、低優先度ながら
ユニットテストで明示的に検証されていない分岐が残存している。これらは主にエラー経路・nil 分岐・
SSRF ガードの有効/無効切替・状態前提違反のケースであり、未検証のまま実装が変更されると静かに
リグレッションが入り込む余地がある。本仕様は、実装挙動を一切変更せず、これら指定分岐に対する
ユニットテストを追加して回帰防止網を補強することを目的とする。SSRF 対策（要件 12）の有効/無効
両経路は、本仕様で明示的に検証対象とする。

## Requirements

### Requirement 1: handler のエラーステータスマッピング default 分岐の検証

**Objective:** As a 開発者, I want `mapAPIErrorToHTTPStatus` の未マップエラーコードに対する default 挙動がテストで固定されること, so that マッピング表の改変時に未知コードのフォールバックが静かに壊れないことを保証できる

#### Acceptance Criteria

1. When 未マップの APIError コードを `mapAPIErrorToHTTPStatus` 相当の経路に渡したとき, the Handler テストスイート shall HTTP 500（Internal Server Error）が返ることを検証する
2. The Handler テストスイート shall 既知のエラーコードが個別に対応する HTTP ステータスへマッピングされる正常系と、未知コードの default 経路（500）の両方を区別して検証する

### Requirement 2: feed パッケージの SSRF ガード有効/無効経路の検証

**Objective:** As a 開発者, I want フィード検出と favicon 取得の HTTP クライアント生成が SSRF ガードの有効時と無効時の両経路でテストされること, so that 要件 12（SSRF 対策）のガード分岐が将来の変更で意図せず無効化されないことを保証できる

#### Acceptance Criteria

1. Where SSRF ガードが有効化されたとき, the FeedDetector テスト shall SSRF 対策付き HTTP クライアントが選択される経路を検証する
2. Where SSRF ガードが無効（未設定）であるとき, the FeedDetector テスト shall SSRF 対策なしの HTTP クライアントが選択される経路を検証する
3. Where SSRF ガードが有効化されたとき, the FaviconFetcher テスト shall SSRF 対策付き HTTP クライアントが選択される経路を検証する
4. Where SSRF ガードが無効（未設定）であるとき, the FaviconFetcher テスト shall SSRF 対策なしの HTTP クライアントが選択される経路を検証する

### Requirement 3: subscription の購読解除 nil 分岐とエラー経路の検証

**Objective:** As a 開発者, I want 購読解除処理の itemStateRepo nil 分岐と記事状態削除エラー経路がテストされること, so that 依存リポジトリ未設定時の安全動作と削除失敗時のエラー伝播が回帰しないことを保証できる

#### Acceptance Criteria

1. Where 記事状態リポジトリが未設定（nil）であるとき, the Subscription Service テスト shall 購読解除が記事状態削除をスキップしたうえで正常に完了する経路を検証する
2. If 記事状態リポジトリの削除処理がエラーを返したとき, the Subscription Service テスト shall 購読解除がエラーを呼び出し元へ伝播することを検証する

### Requirement 4: subscription のフェッチ再開における状態前提違反経路の検証

**Objective:** As a 開発者, I want 停止中ではないフィードに対するフェッチ再開呼び出しがテストされること, so that 状態前提の検証ロジックが回帰した際に検出できる

#### Acceptance Criteria

1. If 停止中ではないフィードに対してフェッチ再開が呼び出されたとき, the Subscription Service テスト shall 状態前提違反として専用エラーが返り、フィード状態が更新されないことを検証する

## Non-Functional Requirements

### NFR 1: 後方互換性（実装挙動の不変性）

1. The 本仕様の変更 shall `internal/handler` / `internal/feed` / `internal/subscription` の実装コードの挙動を変更せず、テストファイルの追加・拡張のみで完結する
2. While 既存テストスイートが実行されている間, the 追加テスト shall 既存テストの結果に影響を与えず、`go test ./...` 全体が成功する状態を維持する

### NFR 2: テストの観点充足

1. The 追加テスト shall 各検証対象につき期待挙動が読み取れる名前と Arrange / Act / Assert 構造を持つ
2. The 追加テスト shall 外部副作用（HTTP / DB）が必要な分岐ではモック実装を用い、検証対象の純粋ロジック・nil 分岐は実物のまま検証する

## Out of Scope

- `subscription.Service.UpdateSettings` のユニットテスト（#45 で別途対応）
- 実装コードの挙動変更・リファクタリング（本仕様はテスト追加のみが目的）
- カバレッジ率を KPI として目標値化すること（目的は指定分岐の回帰防止であり、率の達成ではない）
- 結合テスト・E2E テストの追加（本仕様はユニットテスト層に限定する）
- 上記以外のパッケージ・分岐のカバレッジ補完

## Open Questions

- なし（Issue 本文に検証対象分岐が列挙済みで、既存コメントに人間の追加判断はない）
