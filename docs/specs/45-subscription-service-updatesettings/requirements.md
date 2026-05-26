# Requirements Document

## Introduction

`internal/subscription/service.go` の `Service.UpdateSettings` は購読のフェッチ間隔を更新する
サービス層メソッドだが、単体テストカバレッジが 0.0% であり回帰試験が存在しない。特に
認可チェック（要求ユーザーと購読所有ユーザーの不一致時に `SUBSCRIPTION_NOT_FOUND` を返す経路）を
守るテストが無く、#34 と同型の IDOR / Broken Access Control が再発しても検知できない。本機能は
`UpdateSettings` の各分岐を網羅する単体テストを追加し、回帰検知の安全網を整備することを目的とする。
実装コード（`service.go`）の挙動変更・リファクタは行わず、テスト追加のみをスコープとする。

## Requirements

### Requirement 1: UpdateSettings の正常系・異常系分岐の単体テスト網羅

**Objective:** As a Feedman の保守担当者, I want `Service.UpdateSettings` の各実行分岐が単体テストで検証されること, so that フェッチ間隔更新ロジックの回帰や認可チェックの退行を CI で早期に検知できる

`UpdateSettings` の処理フローは「フェッチ間隔バリデーション → 購読フェッチ（FindByID）→
nil チェック → 所有ユーザー一致チェック → フェッチ間隔更新 → 更新後の購読一覧再取得 →
該当 ID 探索 → 結果返却」で構成される。本要件はバリデーション分岐
（既存 `TestService_UpdateSettings_BoundaryValues` でカバー済み）を除く各分岐を補完する。

#### Acceptance Criteria

1. When 有効なフェッチ間隔と所有者一致の購読 ID が与えられ全ての依存処理が成功する場合, the Subscription Service shall フェッチ間隔を更新し、更新後の購読情報を返すことが単体テストで検証される
2. If 指定された購読 ID の所有ユーザーが要求ユーザーと一致しない場合, the Subscription Service shall `SUBSCRIPTION_NOT_FOUND` のエラーコードを返すことが単体テストで検証される
3. If 指定された購読 ID に対応する購読が存在しない（購読フェッチが該当なしを返す）場合, the Subscription Service shall `SUBSCRIPTION_NOT_FOUND` のエラーコードを返すことが単体テストで検証される
4. If 購読フェッチ処理が永続層エラーを返す場合, the Subscription Service shall 当該エラーを wrap して伝播することが単体テストで検証される
5. If フェッチ間隔更新処理がエラーを返す場合, the Subscription Service shall 当該エラーを wrap して伝播することが単体テストで検証される
6. If 更新後の購読一覧再取得処理がエラーを返す場合, the Subscription Service shall 当該エラーを wrap して伝播することが単体テストで検証される
7. If 更新後の購読一覧再取得結果に対象購読 ID が含まれない場合, the Subscription Service shall `SUBSCRIPTION_NOT_FOUND` のエラーコードを返すことが単体テストで検証される

### Requirement 2: 認可チェック回帰防止の明示

**Objective:** As a Feedman のセキュリティ責任者, I want 他ユーザーの購読 ID 指定が情報漏洩なく拒否されることを単体テストで固定すること, so that #34 と同型の IDOR / Broken Access Control の再発を回帰試験で検知できる

#### Acceptance Criteria

1. If 他ユーザーが所有する購読 ID が要求された場合, the Subscription Service shall 購読の存在を示唆せず `SUBSCRIPTION_NOT_FOUND`（存在しない場合と同一）のエラーコードを返すことが単体テストで検証される
2. When 認可チェックで拒否される場合, the Subscription Service shall フェッチ間隔更新処理を呼び出さないことが単体テストで検証される

## Non-Functional Requirements

### NFR 1: 後方互換（実装挙動の不変性）

1. The Subscription Service shall 本テスト追加の前後で `UpdateSettings` の入出力挙動を変えない（テスト追加に伴う実装コードの挙動変更・リファクタを行わない）

### NFR 2: 既存テストとの非干渉

1. The 単体テストスイート shall 既存テスト（`TestService_UpdateSettings_BoundaryValues` を含む `internal/subscription` パッケージの全テスト）を破壊せず、追加後も全て pass する
2. The 単体テストスイート shall 1 テスト = 1 検証対象の粒度を保ち、正常系・異常系・境界経路を個別のテストケースとして分離する

### NFR 3: テストの自己完結性

1. The 単体テスト shall 外部の永続層・ネットワークに依存せず、依存処理を差し替えた状態で決定論的に実行できる

## Out of Scope

- `UpdateSettings` 以外のメソッド（`ListSubscriptions` / `Unsubscribe` / `ResumeFetch` 等）のテスト追加（#47 で別途扱う）
- 実装コード（`internal/subscription/service.go`）の挙動変更・リファクタ・バグ修正
- フェッチ間隔バリデーション境界値テスト（既存 `TestService_UpdateSettings_BoundaryValues` でカバー済み）
- 結合テスト・E2E テスト（実 PostgreSQL を介したユースケース検証）

## Open Questions

- なし（Issue 本文・既存実装・既存テストで分岐と期待結果が確定しており、人間判断が必要な決定事項は無い。Issue コメントは triage 自動コメントのみで追加の人間判断は記載されていない）
