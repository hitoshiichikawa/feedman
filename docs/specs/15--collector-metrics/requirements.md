# Requirements Document

## Introduction

Feedman には Prometheus 形式のメトリクスを収集する Collector と、それをスクレイプ用に公開する `/metrics`
エンドポイントの実装が既に存在するが、アプリケーション起動エントリから一切参照されておらずデッドコード状態に
なっている。本機能では、この Collector を起動時に生成してフィードフェッチ処理および記事 UPSERT 処理に組み込み、
`/metrics` をルーターに登録して本番で観測可能にする。フェッチを担うプロセスとリクエストを受ける HTTP プロセスは
分離して動作するため、フェッチ系メトリクスが実際にスクレイプ可能となる観測経路を運用者が把握できる形で要件化する。
無制限公開を避けるため、`/metrics` は信頼ネットワーク範囲（信頼 CIDR）からのアクセスのみに制限する。

## Requirements

### Requirement 1: メトリクス公開エンドポイントの提供

**Objective:** As a 運用者, I want Prometheus 形式のメトリクスを HTTP 経由で取得できること, so that フィード取得処理の稼働状況を外部監視基盤からスクレイプして可視化できる

#### Acceptance Criteria

1. When 運用者が稼働中のアプリケーションの `/metrics` エンドポイントへ信頼 CIDR 内からアクセスしたとき, the Metrics Endpoint shall Prometheus テキスト公開形式でメトリクスを応答する
2. When アプリケーションが起動を完了したとき, the Metrics Endpoint shall 追加のメトリクス記録を待たずにスクレイプ可能な状態になる
3. The Metrics Endpoint shall フィード取得処理が公開する全 6 種のメトリクス系列（フェッチ成功数・フェッチ失敗数・パース失敗数・HTTP ステータス別レスポンス数・フェッチレイテンシ・記事アップサート数）を応答に含める

### Requirement 2: フィード取得処理へのメトリクス記録の組み込み

**Objective:** As a 運用者, I want フィード取得処理の実行結果がメトリクスに反映されること, so that フェッチの成功率・失敗傾向・処理量を時系列で監視できる

#### Acceptance Criteria

1. When フィードフェッチが成功したとき, the Fetch Process shall フェッチ成功数メトリクスを増加させる
2. When フィードフェッチが失敗したとき, the Fetch Process shall フェッチ失敗数メトリクスを増加させる
3. When フィードのパースに失敗したとき, the Fetch Process shall パース失敗数メトリクスを増加させる
4. When フィード取得の HTTP レスポンスを受信したとき, the Fetch Process shall 当該 HTTP ステータスコード別レスポンス数メトリクスを増加させる
5. When フィードフェッチが完了したとき, the Fetch Process shall その所要時間をフェッチレイテンシメトリクスとして記録する
6. When 記事の UPSERT 処理が完了したとき, the Upsert Process shall アップサートした記事件数をアップサート数メトリクスに加算する

### Requirement 3: メトリクスのスクレイプ経路の保証

**Objective:** As a 運用者, I want どのプロセスのメトリクスがどこで観測できるかが定まっていること, so that フェッチ処理を行うプロセスとリクエストを受けるプロセスが分離していてもフェッチ系メトリクスを取りこぼさずスクレイプできる

#### Acceptance Criteria

1. While フィード取得処理を担うプロセスが稼働しているとき, the Fetch Process shall 当該プロセスで記録されたフェッチ系メトリクスを `/metrics` 経由でスクレイプ可能にする
2. When フィードフェッチが成功して成功数メトリクスが増加したとき, the Metrics Endpoint shall その増加分を同一プロセスのスクレイプ応答に反映する
3. The Fetch Process shall 自プロセスで記録したメトリクスを、別プロセスのスクレイプ応答に依存せず観測可能にする

### Requirement 4: 信頼 CIDR によるアクセス制限

**Objective:** As a 運用者, I want `/metrics` を信頼ネットワーク範囲からのアクセスのみに制限すること, so that 内部運用情報を含むメトリクスが不特定のクライアントへ無制限公開されない

#### Acceptance Criteria

1. If 信頼 CIDR 範囲外の送信元からのアクセスを受け取ったとき, the Metrics Endpoint shall メトリクスを応答せず 403 Forbidden を返す
2. When 信頼 CIDR 範囲内の送信元からのアクセスを受け取ったとき, the Metrics Endpoint shall アクセス制限による拒否を行わずメトリクスを応答する
3. The Metrics Endpoint shall アクセス制限の判定に送信元 IP アドレスのみを用い、追加の秘匿情報（共有シークレット等）をクライアントに要求しない

### Requirement 5: 既存挙動の後方互換維持

**Objective:** As a 既存ユーザー, I want 既存のエンドポイントと挙動が変わらないこと, so that メトリクス組み込みによって現行機能の動作やレスポンスが退行しない

#### Acceptance Criteria

1. When メトリクス組み込み後に既存エンドポイントへアクセスしたとき, the API Service shall 組み込み前と同一のルーティングおよびレスポンス挙動を維持する
2. The API Service shall 既存リクエストに対するタイムアウト設定を組み込み前と同一に維持する
3. While メトリクスが 1 件も記録されていない起動直後の状態であるとき, the Metrics Endpoint shall エラーを返さず空または初期値のメトリクス応答を返す

## Non-Functional Requirements

### NFR 1: 信頼性・後方互換性

1. The Metrics Endpoint shall メトリクスが未記録の状態でも応答を 5xx エラーにせず正常応答（2xx）を返す
2. The API Service shall メトリクス組み込みの導入によって既存の自動テストスイートを失敗させない

### NFR 2: セキュリティ

1. If 信頼 CIDR の設定値が与えられていないとき, the Metrics Endpoint shall メトリクスを無制限公開せず安全側（アクセス拒否）に倒す
2. The Metrics Endpoint shall アクセス拒否時にメトリクス本文を一切応答に含めない

## Out of Scope

- 新規メトリクス項目の追加（既存 Collector が提供する 6 メソッドの範囲を超える指標の定義は行わない）
- Grafana ダッシュボードやアラートルールの整備
- メトリクスの永続化、および外部監視基盤への push 型送信（本機能は pull/スクレイプ前提）
- メトリクス値に基づく自動スケーリング・自動復旧などの運用自動化
- 信頼 CIDR 以外の保護方式（共有シークレット・OAuth・mTLS 等）の導入

## Open Questions

- なし（`/metrics` の保護方式は信頼 CIDR による IP 制限、Collector の注入範囲はフィード取得処理層と記事 UPSERT 処理層の両方とすることで人間の判断が確定済み）
