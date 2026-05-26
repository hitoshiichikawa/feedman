# Requirements Document

## Introduction

API サーバーモード（`serve`）の起動時、レート制限コンポーネント（RateLimiter）は
バックグラウンドで期限切れエントリのクリーンアップを行う goroutine を起動する。しかし現状の
サーバー起動シーケンスでは RateLimiter が変数参照を持たない形で構築されており、シャットダウン時に
その停止処理（`Stop()`）が呼ばれない。このため SIGINT / SIGTERM によるグレースフルシャットダウン後も
クリーンアップ goroutine が残存し、goroutine リークが発生する。本要件は、シャットダウン経路で
RateLimiter のバックグラウンド goroutine を確実に停止し、かつその過程で異常終了（panic）を
起こさないことを定義する。対象は API サーバーモードのみで、ワーカーモードは RateLimiter を
構築しないため対象外とする。

## Requirements

### Requirement 1: シャットダウン時の RateLimiter 停止

**Objective:** As a 運用者, I want API サーバーのシャットダウン時に RateLimiter のクリーンアップ goroutine が確実に停止されること, so that プロセス終了前後で goroutine リークが発生しないこと

#### Acceptance Criteria

1. When API サーバーが SIGINT または SIGTERM を受信してグレースフルシャットダウンを開始したとき, the API Server shall RateLimiter の停止処理を呼び出す
2. While API サーバーが稼働中である間, the RateLimiter shall クリーンアップのバックグラウンド goroutine を継続させる
3. When グレースフルシャットダウンが完了したとき, the API Server shall RateLimiter が起動したクリーンアップ goroutine を残存させない
4. The API Server shall RateLimiter の停止処理をシャットダウン経路で 1 回だけ実行する

### Requirement 2: 停止経路における異常終了の防止

**Objective:** As a 運用者, I want シャットダウン経路で RateLimiter を停止しても異常終了が起きないこと, so that プロセスが想定どおりに正常終了し終了コードやログが汚染されないこと

#### Acceptance Criteria

1. When シャットダウン経路で RateLimiter の停止処理が実行されたとき, the API Server shall panic を発生させずに正常終了する
2. If RateLimiter の停止処理がシャットダウン経路で重複して起動され得る状況になったとき, the API Server shall panic を発生させない
3. The RateLimiter shall 既存の停止処理の公開シグネチャ（引数・戻り値）を変更しない
4. The RateLimiter shall 停止処理が起動されていない状態で稼働しているテストおよびミドルウェアの既存挙動を変更しない

### Requirement 3: 停止タイミングの妥当性

**Objective:** As a 運用者, I want RateLimiter の停止がシャットダウン手続きの中で適切なタイミングで行われること, so that 稼働中のリクエスト処理を不当に阻害せずにクリーンアップ goroutine だけを終了できること

#### Acceptance Criteria

1. While API サーバーがリクエストを処理している通常稼働中である間, the API Server shall RateLimiter の停止処理を呼び出さない
2. When シャットダウン手続きが開始されたとき, the API Server shall 稼働中の HTTP リクエスト処理の終了を待機するシャットダウン手続きと整合する順序で RateLimiter を停止する

### Requirement 4: 適用範囲の限定

**Objective:** As a 開発者, I want 本変更の適用範囲が API サーバーモードに限定されること, so that RateLimiter を構築しない他の起動モードに不要な副作用が及ばないこと

#### Acceptance Criteria

1. Where RateLimiter を構築する起動モードである場合, the Application shall シャットダウン時に RateLimiter の停止処理を呼び出す
2. Where RateLimiter を構築しない起動モードである場合, the Application shall RateLimiter の停止処理を呼び出さない

## Non-Functional Requirements

### NFR 1: 可観測性・検証可能性

1. The Test Suite shall シャットダウン経路で RateLimiter のクリーンアップ goroutine が停止することを観察可能な形で検証できる手段（停止前後の goroutine 数の比較等）を提供する
2. While 自動テストが RateLimiter の停止挙動を検証している間, the Test Suite shall 停止処理を二重に起動するケースで panic が発生しないことを検証する

### NFR 2: 後方互換性

1. The API Server shall 本変更の前後でレート制限の応答（許可・429 応答・Retry-After ヘッダー）の挙動を変えない
2. The RateLimiter shall 既存の単体テストおよび結合テスト（停止処理を呼び出すもの・呼び出さないものの双方）を変更なしで通過させる

## Out of Scope

- レート制限アルゴリズムそのもの（トークンバケットの方式・許可判定ロジック）の変更
- レート制限の設定値（req/min・バーストサイズ）のチューニングや変更
- クリーンアップ間隔（CleanupInterval）および TTL のチューニングや変更
- ワーカーモード（RateLimiter を構築しない起動モード）のシャットダウン挙動の変更
- 429 応答のフォーマット・メッセージ・メトリクスの変更
- RateLimiter の停止処理を context ベースに作り替えるなど、停止機構の API 形状の刷新

## Open Questions

- 「停止処理を 1 回だけ実行する」ことで panic を防ぐか、停止処理側を多重起動に耐える形（idempotent 化）にするかは design / 実装の判断に委ねる。本要件では「シャットダウン経路で panic が起きない」観察可能な振る舞いのみを要求し、実装方式は指定しない。ただし「既存の停止処理の公開シグネチャ・挙動を変更しない」制約（Requirement 2.3 / 2.4）と「panic を防ぐ」目標（Requirement 2.1 / 2.2）の両立方法に複数案がある点は Architect が明示的に判断すること
- 上記以外に Issue 本文・既存コメントから読み取れなかった不足情報は現時点でなし
