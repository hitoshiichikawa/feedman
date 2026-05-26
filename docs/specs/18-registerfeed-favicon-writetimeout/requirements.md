# Requirements Document

## Introduction

フィード登録処理（`FeedService.RegisterFeed`）は、フィード検出（最大 10 秒）と favicon 取得（最大 5 秒）を
同期実行している。両者が同時に遅延すると合計処理時間が HTTP サーバーの WriteTimeout（15 秒）を超過し、
購読登録自体は成功しているのにクライアントへはタイムアウトエラーとして見える不整合が起こり得る。
本要件は、人間が確定した方針（favicon 取得の非同期化＝Option A）に基づき、登録レスポンスを WriteTimeout
内で安定して返しつつ、favicon の取得遅延・失敗が登録結果に波及しないことを定義する。
favicon 取得失敗時に null として扱う現行のフォールバック挙動と、登録 API のレスポンス形式は維持する。

## Requirements

### Requirement 1: 登録レスポンスのタイムアウト安全性

**Objective:** As a フィードを登録するユーザー, I want 登録レスポンスが WriteTimeout 内に必ず返ること, so that 登録が成功したのにタイムアウトエラーに見える不整合を経験しない

#### Acceptance Criteria

1. When ユーザーがフィード登録を要求し favicon 取得が遅延または完了していない状態のとき, the Feed Service shall favicon 取得の完了を待たずに登録レスポンスを返す
2. When フィード検出が上限時間（10 秒）近くまで要した後に登録処理を継続するとき, the Feed Service shall favicon 取得時間を登録レスポンスの応答時間に加算しない
3. While favicon 取得がバックグラウンドで継続している状態のとき, the Feed Service shall ユーザーへの登録レスポンス送出をブロックしない
4. The Feed Service shall 登録レスポンスを WriteTimeout（15 秒）未満で返す

### Requirement 2: favicon 取得失敗・遅延時の登録成功維持

**Objective:** As a フィードを登録するユーザー, I want favicon が取得できなくても登録自体は成功扱いになること, so that favicon の有無に関わらずフィードを購読できる

#### Acceptance Criteria

1. If favicon 取得が失敗したとき, the Feed Service shall 登録を成功として扱い成功レスポンスを返す
2. If favicon 取得が遅延しタイムアウトしたとき, the Feed Service shall 登録を成功として扱い成功レスポンスを返す
3. If favicon が見つからないとき, the Feed Service shall 当該フィードの favicon を null として保持する
4. If favicon 取得が失敗したとき, the Feed Service shall 当該フィードの favicon を null として保持する
5. When favicon 取得が失敗または遅延したとき, the Feed Service shall 失敗事由を運用者が追跡できるログとして記録する

### Requirement 3: favicon 取得成功時の保存

**Objective:** As a フィードを登録するユーザー, I want favicon が取得できた場合は最終的にフィードへ反映されること, so that 後続の表示で favicon を確認できる

#### Acceptance Criteria

1. When favicon 取得がバックグラウンドで成功したとき, the Feed Service shall 取得した favicon を当該フィードに保存する
2. When favicon の保存が完了したとき, the Feed Service shall 後続のフィード取得操作で当該 favicon を参照可能にする
3. While 登録レスポンス送出後に favicon 取得が継続している状態のとき, the Feed Service shall 当該登録要求のリクエストスコープ完了によって favicon 取得を中断しない

### Requirement 4: バックグラウンド処理の有界性

**Objective:** As a システム運用者, I want バックグラウンドの favicon 取得が無制限に滞留しないこと, so that リソースリークや滞留処理の蓄積を避けられる

#### Acceptance Criteria

1. While favicon 取得がバックグラウンドで継続している状態のとき, the Feed Service shall 取得処理に上限時間（30 秒以内）を設ける
2. When favicon 取得が上限時間に達したとき, the Feed Service shall 当該取得処理を打ち切る
3. When favicon 取得処理が完了または打ち切られたとき, the Feed Service shall 当該処理に割り当てたバックグラウンドリソースを解放する

### Requirement 5: 後方互換の維持

**Objective:** As a 登録 API の利用者, I want 既存のレスポンス形式と挙動が変わらないこと, so that 既存クライアントが影響を受けない

#### Acceptance Criteria

1. When フィード登録が成功したとき, the Feed Service shall フィード情報と購読情報を含む既存のレスポンス形式で結果を返す
2. The Feed Service shall 登録成功時の HTTP ステータスを現行（201 Created）から変更しない
3. When 購読上限超過・重複購読・無効 URL・フィード未検出などフィード検出までに発生する既存のエラー条件が成立したとき, the Feed Service shall 現行と同一のエラー応答を返す

## Non-Functional Requirements

### NFR 1: 応答時間

1. The Feed Service shall フィード登録レスポンスを WriteTimeout（15 秒）未満で返す
2. While favicon 取得がバックグラウンドで継続している状態のとき, the Feed Service shall 登録レスポンスの応答時間に favicon 取得時間を含めない

### NFR 2: 可観測性

1. When favicon 取得が失敗・未検出・タイムアウトのいずれかになったとき, the Feed Service shall 対象フィードを特定できる情報を含むログを記録する

### NFR 3: 互換性

1. The Feed Service shall 登録 API のレスポンス形式（feed / subscription）を本変更前と同一に保つ

## Out of Scope

- フィード検出（パース・URL 正規化）ロジックの変更
- favicon の画像最適化・キャッシュ戦略
- WriteTimeout の延長（Option B は不採用）
- favicon 取得の非同期化に伴う UI 側の遅延反映表示（後追い表示）の追加
- 既存のフィード検出タイムアウト（10 秒）・favicon 取得タイムアウト（5 秒）値そのものの調整

## Open Questions

- なし（実装方針は Issue コメントで Option A（非同期化）に確定済み。バックグラウンド取得の上限時間は
  目安 30 秒以内とし、具体値・実装手段は design.md の領分とする）
