# Requirements Document

## Introduction

`FeedDetector` と `FaviconFetcher` は HTTP リクエストのたびに新しい HTTP クライアントを生成しており、
クライアント単位で独立したコネクションプールが割り当てられるため、TCP/TLS コネクションが再利用されず
リクエスト都度にハンドシェイクが発生している。フィード検出・favicon 取得を多数行う場面ではこの
コネクション確立コストが無駄に積み上がる。本要件は、各コンポーネントが HTTP クライアントを再利用して
コネクションプールを共有しつつ、フィード検出・favicon 取得の外部から見た挙動（検出結果・取得結果・
SSRF 防止・タイムアウト・サイズ上限）を一切変えないことを定義するリファクタ要件である。

## Requirements

### Requirement 1: FeedDetector の HTTP クライアント再利用

**Objective:** As a Feedman の運用者, I want FeedDetector が HTTP クライアントを再利用してコネクションプールを共有すること, so that 多数のフィード検出時に無駄な TCP/TLS ハンドシェイクが発生せず確立コストを削減できる

#### Acceptance Criteria

1. While 同一の FeedDetector インスタンスが複数回のフィード検出に使われている, the FeedDetector shall リクエスト間で同一の HTTP クライアントを使い回す
2. When 同一の FeedDetector インスタンスで連続して複数回のリクエストを実行したとき, the FeedDetector shall リクエストごとに新しい HTTP クライアントを追加生成しない
3. The FeedDetector shall フィード検出ごとに新規コネクションを確立せず再利用可能なコネクションプールを共有する

### Requirement 2: FeedDetector の検出結果不変性

**Objective:** As a フィードを登録するユーザー, I want クライアント再利用後もフィード検出結果が従来と同一であること, so that リファクタによって検出の振る舞いが退行しない

#### Acceptance Criteria

1. When 同一の FeedDetector インスタンスから同一の入力 URL に対して複数回フィード検出を実行したとき, the FeedDetector shall 各回で本変更前と同一の検出結果（検出された feed URL もしくは未検出エラー）を返す
2. If 入力 URL からフィードが検出できないとき, the FeedDetector shall 本変更前と同一の未検出エラーを返す

### Requirement 3: FaviconFetcher の HTTP クライアント再利用

**Objective:** As a Feedman の運用者, I want FaviconFetcher が HTTP クライアントを再利用してコネクションプールを共有すること, so that 多数の favicon 取得時に無駄な TCP/TLS ハンドシェイクが発生せず確立コストを削減できる

#### Acceptance Criteria

1. While 同一の FaviconFetcher インスタンスが複数回の favicon 取得に使われている, the FaviconFetcher shall リクエスト間で同一の HTTP クライアントを使い回す
2. When 同一の FaviconFetcher インスタンスで連続して複数回のリクエストを実行したとき, the FaviconFetcher shall リクエストごとに新しい HTTP クライアントを追加生成しない
3. The FaviconFetcher shall favicon 取得ごとに新規コネクションを確立せず再利用可能なコネクションプールを共有する

### Requirement 4: FaviconFetcher の取得結果不変性

**Objective:** As a フィードを購読するユーザー, I want クライアント再利用後も favicon 取得結果が従来と同一であること, so that リファクタによって favicon 取得の振る舞いが退行しない

#### Acceptance Criteria

1. When 同一の FaviconFetcher インスタンスから複数回 favicon を取得したとき, the FaviconFetcher shall 各回で本変更前と同一の取得結果（取得データと MIME タイプ、もしくは取得失敗時の nil データと空 MIME）を返す
2. If favicon の取得に失敗したとき, the FaviconFetcher shall 本変更前と同一の挙動（nil データ・空 MIME・エラーなし）を返す

### Requirement 5: SSRF 防止の非退行

**Objective:** As a セキュリティ責任者, I want クライアント再利用後も SSRF 防止が退行しないこと, so that 内部 IP や禁止先への到達がこれまでどおりブロックされる

#### Acceptance Criteria

1. Where SSRF ガードが有効になっている, the FeedDetector shall SSRF 防止挙動を維持したまま HTTP クライアントを再利用する
2. Where SSRF ガードが有効になっている, the FaviconFetcher shall SSRF 防止挙動を維持したまま HTTP クライアントを再利用する
3. If SSRF ガード有効時に内部 IP・ループバック・リンクローカル・禁止スキーム等の禁止先への到達が試みられたとき, the FeedDetector shall 本変更前と同一にその到達をブロックする
4. If SSRF ガード有効時に内部 IP・ループバック・リンクローカル・禁止スキーム等の禁止先への到達が試みられたとき, the FaviconFetcher shall 本変更前と同一にその到達をブロックする
5. While 再利用される HTTP クライアントがリクエストを処理している, the FeedDetector and FaviconFetcher shall DNS 解決後の IP 検証（DNS リバインディング対策を含む SSRF 検証）を本変更前と同一に適用する

### Requirement 6: 既存設定値の維持

**Objective:** As a Feedman の運用者, I want タイムアウトや最大レスポンスサイズ等の既存設定値が維持されること, so that 再利用化によってリクエストの制限挙動が変わらない

#### Acceptance Criteria

1. The FeedDetector shall 本変更前と同一のタイムアウト値・最大レスポンスサイズ上限で HTTP リクエストを処理する
2. The FaviconFetcher shall 本変更前と同一のタイムアウト値・最大 favicon サイズ上限で HTTP リクエストを処理する

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The FeedDetector shall 本変更前と同一の公開メソッドシグネチャ・戻り値の型を維持する
2. The FaviconFetcher shall 本変更前と同一の公開メソッドシグネチャ・戻り値の型を維持する

### NFR 2: 並行安全性

1. While 複数の goroutine が同一インスタンスを介して同時にリクエストを実行している, the FeedDetector and FaviconFetcher shall データ競合を発生させずに再利用される HTTP クライアントへ安全にアクセスする

## Out of Scope

- SSRF ガード（`NewSafeClient`）の防御ロジック自体の変更
- タイムアウト値・最大レスポンスサイズ・最大 favicon サイズ等のチューニング・値変更
- HTTP/2 やプロキシ対応などの新機能追加
- フィード検出・favicon 取得のアルゴリズムや判定ロジックの変更
- `internal/worker/fetch` 等、本 Issue の対象外コンポーネントの HTTP クライアント生成箇所の変更

## Open Questions

- なし
