# Requirements Document

## Introduction

Feedman は現在、フィードの新着記事をワーカーの定期スケジューラのみで取得しており、ユーザーは
次回フェッチサイクルが回るまで待つしかない。新規登録直後のフィードや、今すぐ読みたい更新が
ある場面でユーザーが受動的に待たされる体験を解消するため、購読単位で能動的にフェッチを起こす
「手動更新」機能を追加する。手動フェッチは外部サイトへの負荷集中とワーカーとの二重実行を
避ける必要があるため、同期 API・10 分クールダウン・行ロックによる排他制御を伴った形で提供する。
本要件は対応する API、UI、観測性、セキュリティの観測可能な挙動のみを定義し、内部実装方式は
`design.md` の領分とする。

## Requirements

### Requirement 1: 手動フェッチ API（同期実行）

**Objective:** As a 認証済みユーザー, I want 任意のタイミングで購読フィードの即時フェッチを
トリガーする手段, so that 次の自動フェッチサイクルを待たずに新着記事を取得できる

#### Acceptance Criteria

1. When 認証済みユーザーが自身の購読 ID を指定して手動フェッチ API を呼び出したとき, the Manual Fetch API shall そのフィードの取得・パース・記事 UPSERT を同期的に完了させてから HTTP 2xx 系のレスポンスを返す
2. When 手動フェッチが成功したとき, the Manual Fetch API shall そのフィードの「最終成功時刻」を当該リクエスト完了時刻で更新する
3. While 手動フェッチが進行中である間, the Manual Fetch API shall フェッチ完了（成功・失敗いずれも確定）するまでクライアントへレスポンスを返さない
4. If 認証情報を持たないリクエストが手動フェッチ API に到達したとき, the Manual Fetch API shall HTTP 401 を返し、フェッチ処理を一切実行しない
5. If 指定された購読 ID が存在しないとき, the Manual Fetch API shall HTTP 404 を返し、フェッチ処理を一切実行しない
6. If 指定された購読 ID が呼び出しユーザー自身のものではないとき, the Manual Fetch API shall HTTP 404 を返し、他ユーザーの購読の存在を示唆する情報を返さない
7. The Manual Fetch API shall リクエスト本文を必要としない（パラメータは URL パスの購読 ID のみで完結する）

### Requirement 2: クールダウン制御（同一フィードの 10 分制限）

**Objective:** As a 運用者, I want 同一フィードへの手動フェッチを最後の成功から 10 分間
スキップする制御, so that 外部サイトへの DoS と無駄な帯域消費を防げる

#### Acceptance Criteria

1. While 対象フィードの最終成功時刻から 10 分が経過していない間, the Manual Fetch API shall そのフィードへの外部 HTTP リクエストを実行せず HTTP 429 を返す
2. When クールダウン中の理由で HTTP 429 を返すとき, the Manual Fetch API shall レスポンスボディに次回フェッチ可能になるまでの残り時間が分かる情報を含める
3. When 対象フィードの最終成功時刻から 10 分以上経過しているとき, the Manual Fetch API shall クールダウンを適用せず通常通り外部フェッチを実行する
4. The Manual Fetch API shall クールダウン判定の起点として、ワーカーによる自動フェッチ成功時刻と手動フェッチ成功時刻の両方を同等に扱う
5. If 対象フィードに過去の成功実績がまったく無いとき, the Manual Fetch API shall クールダウンを適用せず通常通り外部フェッチを実行する

### Requirement 3: 自動ワーカーとの排他制御（行ロック）

**Objective:** As a 運用者, I want 手動フェッチと自動ワーカーフェッチが同一フィードに対して
並行実行されないこと, so that 重複した外部リクエストと記事 UPSERT の競合を防げる

#### Acceptance Criteria

1. When 手動フェッチが開始されるとき, the Manual Fetch API shall 対象フィード行に対して非ブロッキングな排他ロックを取得してから処理を開始する
2. If 対象フィード行のロックを別プロセス（自動ワーカー含む）が保持していてロック取得に失敗したとき, the Manual Fetch API shall 待機せず即座に HTTP 4xx 系（競合を示すコード）を返し、フェッチ処理を一切実行しない
3. When ロック取得に失敗して競合エラーを返すとき, the Manual Fetch API shall レスポンスボディに「現在フェッチが進行中のため再試行してほしい」旨が分かる情報を含める
4. The Manual Fetch API shall ロックの保持期間をフェッチ・パース・UPSERT・状態更新の完了までに限定し、レスポンス返却前にロックを解放する

### Requirement 4: 外部 URL 取得時のセキュリティ

**Objective:** As a 運用者, I want 手動フェッチが既存の SSRF / サニタイズ防御と同等の保護を
受けること, so that ユーザー起点のフェッチが内部ネットワーク探索や悪意あるコンテンツの混入経路に
ならない

#### Acceptance Criteria

1. The Manual Fetch API shall 外部 URL への取得に先立ち、自動フェッチと同等の SSRF 対策（内部 IP / プライベートレンジ / メタデータエンドポイント等の拒否）を適用する
2. If SSRF 対策により対象フィードの URL が拒否されたとき, the Manual Fetch API shall 外部 HTTP リクエストを実行せず、ユーザーには中立的な失敗を示すレスポンスを返す
3. The Manual Fetch API shall 取得したコンテンツのサニタイズ・解析方針を自動フェッチと同一に保ち、手動経由でのみ緩和されるサニタイズ抜け道を作らない

### Requirement 5: 手動更新ボタン（記事一覧ヘッダー UI）

**Objective:** As a ログイン中のユーザー, I want 記事一覧ヘッダーから 1 クリックで手動更新を
起動できる UI, so that 操作対象のフィードを明示的に選んで即時更新できる

#### Acceptance Criteria

1. Where 記事一覧ペインがフィード選択状態であるとき, the Item List Header shall フィルタタブ群の右隣に手動更新ボタン（更新マーク）を表示する
2. Where 記事一覧ペインがフィード未選択状態であるとき, the Item List Header shall 手動更新ボタンを表示しないか操作不可能な状態にする
3. When ユーザーが手動更新ボタンを押下したとき, the Item List Header shall そのフィードに対する手動フェッチ要求を 1 件だけ発行する
4. While 手動フェッチ要求の応答が返るまでの間, the Item List Header shall 手動更新ボタンを操作不可（disabled）状態にし、ローディング中であることが視覚的に分かる回転アニメーションを継続する
5. When 手動フェッチ要求の応答（成功・失敗のいずれも）が返ったとき, the Item List Header shall 回転アニメーションを停止し、ボタンを操作可能な状態へ戻す
6. While 手動更新ボタンが disabled 状態である間, the Item List Header shall 同一フィードに対する重複した手動フェッチ要求の発行を防ぐ

### Requirement 6: 完了後の自動再表示

**Objective:** As a ユーザー, I want 手動更新の完了直後に最新の記事一覧と未読件数が
反映されること, so that 取得した新着記事を追加操作なしで確認できる

#### Acceptance Criteria

1. When 手動フェッチ API が成功レスポンスを返したとき, the Web UI shall 対象フィードの記事一覧表示を最新状態に再取得する
2. When 手動フェッチ API が成功レスポンスを返したとき, the Web UI shall 左ペインの未読件数バッジを最新状態に再取得する
3. While 記事一覧の再取得が完了するまでの間, the Web UI shall 既存の記事一覧表示を維持し、画面全体を空にしない

### Requirement 7: フロントエンドのエラー表示

**Objective:** As a ユーザー, I want 手動更新が失敗したときに理由が判別できる表示, so that
クールダウン中・競合中・ネットワーク不調などの状況に応じて適切に再試行できる

#### Acceptance Criteria

1. If 手動フェッチ API が HTTP 429（クールダウン中）を返したとき, the Web UI shall ユーザーへ「クールダウン中であり再試行までの残り時間がある」旨を通知する
2. If 手動フェッチ API が競合エラー（自動ワーカーや別手動更新と排他制御で競合）を返したとき, the Web UI shall ユーザーへ「現在フェッチ中のためしばらく待ってから再試行する」旨を通知する
3. If 手動フェッチ API が HTTP 401 を返したとき, the Web UI shall ユーザーへ認証切れである旨を通知し、再ログイン動線を妨げない
4. If 手動フェッチ API が HTTP 5xx またはネットワーク到達不能の応答となったとき, the Web UI shall ユーザーへ一時的失敗である旨を通知し、再試行を可能とする
5. When 手動フェッチが失敗したとき, the Web UI shall 記事一覧の表示内容を失敗前の状態のまま保持する

### Requirement 8: 観測性（メトリクス）

**Objective:** As a 運用者, I want 手動フェッチの実行状況をメトリクスから把握できること, so that
利用頻度・成功率・拒否要因の傾向を分析しキャパシティ判断に活かせる

#### Acceptance Criteria

1. When 手動フェッチが成功したとき, the Metrics Collector shall 手動フェッチ成功件数を 1 増加させる
2. When 手動フェッチが外部 HTTP 取得・パース・UPSERT のいずれかで失敗したとき, the Metrics Collector shall 失敗理由のカテゴリを区別したうえで手動フェッチ失敗件数を 1 増加させる
3. When 手動フェッチがクールダウンにより拒否されたとき, the Metrics Collector shall クールダウン拒否件数を 1 増加させる
4. When 手動フェッチが行ロック競合により拒否されたとき, the Metrics Collector shall 競合拒否件数を 1 増加させる
5. The Metrics Collector shall 手動フェッチに関するカウンタを自動フェッチと区別可能な形で公開する

## Non-Functional Requirements

### NFR 1: 性能・タイムアウト

1. The Manual Fetch API shall 外部フィード取得の単一リクエストにつき自動フェッチと同一のタイムアウト上限（既存設定値）を超えない範囲で完了させる
2. The Manual Fetch API shall クールダウン拒否・行ロック競合拒否のレスポンスを 500ms 以内に返す（外部 HTTP を発行しないため）
3. While 手動フェッチが進行中である間, the Manual Fetch API shall 同一ユーザーが別フィードに対して手動フェッチを並行発行することを妨げない

### NFR 2: 認証・認可

1. The Manual Fetch API shall 既存の認証ミドルウェア（セッション必須ルート群）と同一の認証要件を適用する
2. The Manual Fetch API shall 既存の認証必須ルートと同一のレート制限ミドルウェアの対象とする

### NFR 3: 既存挙動の維持（後方互換）

1. The Manual Fetch API shall 自動ワーカーのスケジューリングロジック（`next_fetch_at`、連続エラー数、停止状態判定）を手動フェッチ成功・失敗のいずれにおいても破壊しない
2. The Manual Fetch API shall 既存のフィード状態（active / stopped / error）遷移ルールを自動フェッチと同一に保つ

## Out of Scope

- 複数フィードを一括で手動更新する API・UI（1 リクエスト = 1 購読 ID のみ対象）
- 手動更新の進捗（取得バイト数・パース中件数など）のリアルタイム表示・ストリーミング
- 手動更新の履歴・監査ログ画面
- 自動フェッチ周期や次回フェッチ時刻の手動再スケジュール
- クールダウン時間（10 分）のユーザー個別カスタマイズ
- 手動更新トリガーのキーボードショートカット
- 外部 Webhook やプッシュ通知による手動更新トリガー
- LaunchDarkly 等の外部 Feature Flag 連携（本リポジトリは Feature Flag Protocol opt-out）

## 確認事項

以下は Issue 本文・既存ドキュメント・コードベースで一意に確定できなかった事項であり、
設計（`design.md`）または人間判断で確定したうえで実装に進むこと。

- **クールダウン残り時間のレスポンス表現**: HTTP 429 レスポンスボディに残り時間を含めるフォーマット（残り秒数 / ISO 8601 期間表現 / 次回可能時刻 RFC3339 のいずれか）を決定する必要がある。既存の `model.APIError`（`Code` / `Message` / `Category` / `Action`）の枠組みに収めるのか、追加フィールドを定義するのかも要確定。
- **行ロック競合時の HTTP ステータスコード**: 409 Conflict / 423 Locked / 503 Service Unavailable のいずれを採用するか。既存実装では `FEED_NOT_STOPPED` 等のドメイン衝突に 409 を採用している（`internal/handler/feed_handler_test.go` 参照）ため、それに揃えるのが自然だが最終確定は design 側。
- **最終成功時刻の記録カラム**: 現行 `feeds` テーブル（`internal/database/migrations/20260227120000_initial_schema.up.sql`）には `last_successful_fetch_at` 相当のカラムが存在しない。新規カラム追加（マイグレーション要）か、既存の `next_fetch_at` から逆算する方式か、フェッチ間隔と組み合わせて推定するかを design で決定する必要がある。
- **フロントエンドのエラー通知 UI 形式**: 現状の Web 実装には toast / sonner 等の通知基盤が導入されておらず、既存 hooks は `isError` フラグでのインライン表示パターンが主流（`web/src/hooks/use-item-state.ts` 等）。手動更新の失敗通知を toast で実装するか、ヘッダー近傍のインラインバナーで表示するかを design 側で決定する必要がある（必要なら toast ライブラリ導入の判断も含む）。
- **手動更新ボタンのアクセシビリティ要件**: aria-label・aria-busy・スクリーンリーダー向けライブリージョンによる「更新完了」通知の要否（明示要件が Issue 本文にないため、既存 UI の慣習に合わせて design 側で確定）。
