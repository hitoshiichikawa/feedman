# Requirements Document

## Introduction

購読のフェッチ間隔は要件 8.5 で「最短 30 分・30 分刻み・最大 12 時間（720 分）」と定められている。
しかし購読サービスの設定更新処理（`internal/subscription/service.go` の `UpdateSettings`）はこの境界値
バリデーションを実装しておらず、受け取った分数を無検証のままリポジトリ層へ渡している。現状は同等の
バリデーションがハンドラー層（`internal/handler/subscription_handler.go` の `isValidFetchInterval`）に
存在するため API 経由では弾けているが、検証の責務がハンドラーに置かれており、サービス層を直接利用する
経路や将来の呼び出し経路では不正値が素通りする。本要件では、フェッチ間隔の境界値バリデーションを
サービス層を正として集約し、既存の正当な値での更新挙動・API エラー契約を壊さないことを定義する。

## Requirements

### Requirement 1: フェッチ間隔の境界値バリデーション

**Objective:** As a 購読設定を更新するユーザー, I want 不正なフェッチ間隔を確実に拒否してほしい, so that 要件 8.5 で定められた範囲外の値で更新されず、フィード取得が想定どおりのリズムで行われる

#### Acceptance Criteria

1. When フェッチ間隔として 30 分が指定された購読設定更新が要求されたとき, the Subscription Service shall 更新を成功させる
2. When フェッチ間隔として 60 分が指定された購読設定更新が要求されたとき, the Subscription Service shall 更新を成功させる
3. When フェッチ間隔として 90 分が指定された購読設定更新が要求されたとき, the Subscription Service shall 更新を成功させる
4. When フェッチ間隔として 720 分が指定された購読設定更新が要求されたとき, the Subscription Service shall 更新を成功させる
5. If フェッチ間隔として 29 分（下限未満）が指定されたとき, the Subscription Service shall 更新を拒否しフェッチ間隔無効エラーを返す
6. If フェッチ間隔として 721 分（上限超過）が指定されたとき, the Subscription Service shall 更新を拒否しフェッチ間隔無効エラーを返す
7. If フェッチ間隔として 31 分（30 分刻み違反）が指定されたとき, the Subscription Service shall 更新を拒否しフェッチ間隔無効エラーを返す
8. If フェッチ間隔として 45 分（30 分刻み違反）が指定されたとき, the Subscription Service shall 更新を拒否しフェッチ間隔無効エラーを返す
9. If フェッチ間隔として 0 分が指定されたとき, the Subscription Service shall 更新を拒否しフェッチ間隔無効エラーを返す
10. If フェッチ間隔として負値が指定されたとき, the Subscription Service shall 更新を拒否しフェッチ間隔無効エラーを返す

### Requirement 2: 不正値拒否時のエラー契約

**Objective:** As a API クライアント, I want 不正なフェッチ間隔の拒否時に既存と同一のエラー応答を受け取りたい, so that 既存のクライアント側エラー処理を変更せずに済む

#### Acceptance Criteria

1. If フェッチ間隔が無効と判定されたとき, the Subscription Service shall コード `INVALID_FETCH_INTERVAL` のフェッチ間隔無効エラーを返す
2. When サービス層がフェッチ間隔無効エラーを返したとき, the Subscription API shall HTTP ステータス 400（Bad Request）で応答する
3. When サービス層がフェッチ間隔無効エラーを返したとき, the Subscription API shall コード `INVALID_FETCH_INTERVAL` を含むエラー本文で応答する
4. While 拒否対象の更新要求が処理されているとき, the Subscription Service shall 購読のフェッチ間隔を更新しない

### Requirement 3: バリデーション責務のサービス層集約

**Objective:** As a 本コードベースの保守担当者, I want フェッチ間隔バリデーションの正をサービス層に一元化したい, so that 検証ロジックの二重実装による挙動の不一致を防ぐ

#### Acceptance Criteria

1. The Subscription Service shall フェッチ間隔の境界値バリデーションを購読設定更新処理の中で実施する
2. While ハンドラー層に同等のフェッチ間隔バリデーションが存在するとき, the Subscription API shall その検証をサービス層へ集約し二重実装を解消する
3. When 正当なフェッチ間隔（30〜720 分・30 分刻み）で購読設定更新が要求されたとき, the Subscription Service shall 本変更導入前と同一の更新後購読情報を返す

## Non-Functional Requirements

### NFR 1: 後方互換性

1. When 本変更導入前に成功していた正当なフェッチ間隔値（30, 60, 90, 720 分など 30〜720 分の 30 分刻み）で更新が要求されたとき, the Subscription Service shall 本変更導入前と同一の成功結果を返す
2. The Subscription API shall フェッチ間隔の許容範囲（30〜720 分・30 分刻み）を本変更導入前と同一に維持する

### NFR 2: 検証可能性

1. The Subscription Service shall 各境界値（29, 30, 31, 45, 60, 90, 720, 721, 0, 負値）に対する受理／拒否を単体テストで検証可能な形で提供する

## Out of Scope

- フェッチ間隔の許容範囲そのものの仕様変更（下限・上限・刻み幅の変更）
- worker 側のスケジューリングロジックの変更
- ハンドラー層以外（例: フロントエンド）でのバリデーション追加・変更
- フェッチ間隔以外の購読設定項目に対するバリデーション

## Open Questions

- なし（Issue 本文・既存コメント・既存実装で受入基準を確定できた。Triage 自動コメント以外に人間の追加決定事項なし）
