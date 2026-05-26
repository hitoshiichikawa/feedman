# Requirements Document

## Introduction

Feedman の `web`（Next.js）が配信する HTML レスポンスに Content-Security-Policy（CSP）ヘッダーを付与し、XSS への多層防御（defense in depth）を実現する。記事本文は外部フィード由来の HTML をサニタイズ後に `dangerouslySetInnerHTML` で描画しており、サニタイズをすり抜けた悪性スクリプトに対する追加の安全網として CSP の価値が高い。既存のセキュリティヘッダー（`X-Content-Type-Options` / `X-Frame-Options` / `Referrer-Policy` / `Permissions-Policy`、#41 で導入済み）は温存しつつ CSP を追加する。本機能の最重要制約は、CSP 適用後も既存画面の表示・操作を一切壊さない後方互換性である。

## Requirements

### Requirement 1: CSP ヘッダーの全ルート付与

**Objective:** As a セキュリティ運用者, I want web が返す全ルートのレスポンスに CSP ヘッダーが付与されること, so that フロントエンド全体で XSS に対する多層防御を一律に効かせられる

#### Acceptance Criteria

1. When ブラウザが web の任意のルートに対する HTML レスポンスを受け取ったとき, the Web Frontend shall `Content-Security-Policy` レスポンスヘッダーを 1 つ含める
2. The Web Frontend shall CSP ヘッダーを特定の画面に限定せず全ルート（トップ画面・記事一覧・記事詳細・認証関連画面を含む）へ一律に付与する
3. The `Content-Security-Policy` ヘッダー値 shall 少なくとも `default-src` ディレクティブを含み、明示的に許可されていないリソース取得をブラウザが既定で拒否する状態にする

### Requirement 2: 厳格な許可方針（最小許可）

**Objective:** As a セキュリティ運用者, I want CSP が「できるだけ厳しく、必要な許可のみ追加する」方針で構成されること, so that 攻撃面を最小化しつつ正当なリソースのみ許可できる

#### Acceptance Criteria

1. The `default-src` ディレクティブ shall 既定の取得元を同一オリジン（`'self'`）に限定し、ワイルドカード `*` を既定値として許可しない
2. Where 同一オリジン化済みの API / 認証エンドポイント（`/api/*` / `/auth/*`）への接続が必要な場合, the Web Frontend shall それらの接続を `'self'` の範囲で許可する
3. Where アプリが利用するフォントが同一オリジンから配信される場合, the Web Frontend shall フォント取得を `'self'` の範囲で許可し、外部フォント CDN への接続を CSP で許可しない
4. The `Content-Security-Policy` ヘッダー値 shall 機能上必要と確認されていない外部オリジンを許可リストに含めない

### Requirement 3: 既存セキュリティヘッダーの温存（後方互換）

**Objective:** As a セキュリティ運用者, I want CSP 追加後も既存のセキュリティヘッダーが維持されること, so that 既存の防御（クリックジャッキング対策・MIME スニッフィング対策等）が後退しない

#### Acceptance Criteria

1. When CSP ヘッダーが付与されたレスポンスを受け取ったとき, the Web Frontend shall `X-Content-Type-Options: nosniff` ヘッダーを引き続き付与する
2. When CSP ヘッダーが付与されたレスポンスを受け取ったとき, the Web Frontend shall `X-Frame-Options: DENY` ヘッダーを引き続き付与する
3. When CSP ヘッダーが付与されたレスポンスを受け取ったとき, the Web Frontend shall `Referrer-Policy: strict-origin-when-cross-origin` ヘッダーを引き続き付与する
4. When CSP ヘッダーが付与されたレスポンスを受け取ったとき, the Web Frontend shall `Permissions-Policy: camera=(), microphone=(), geolocation=()` ヘッダーを引き続き付与する

### Requirement 4: 既存画面の表示・操作の維持（後方互換・境界）

**Objective:** As a エンドユーザー, I want CSP 適用後も既存画面が今まで通り表示・操作できること, so that セキュリティ強化によって UI が壊れたり機能が使えなくなったりしない

#### Acceptance Criteria

1. When ユーザーが CSP 適用後の画面を開いたとき, the Web Frontend shall 既存の CSS スタイル（Tailwind / shadcn UI のスタイリング）を CSP 違反でブロックされることなく適用する
2. When ユーザーが CSP 適用後の画面を操作したとき, the Web Frontend shall アプリのスクリプト（ページ初期化・クライアントサイドの操作応答）を CSP 違反でブロックされることなく実行する
3. When ユーザーが記事を展開したとき, the Web Frontend shall サニタイズ済みの記事本文 HTML を CSP 違反でブロックされることなく描画する
4. When ユーザーが画面を開いたとき, the Web Frontend shall 同一オリジンから配信されるフォントを CSP 違反でブロックされることなく表示する
5. The Web Frontend shall CSP 適用前に通っていた既存の自動テスト（表示・操作の検証）を CSP 適用後も合格させる

### Requirement 5: インライン script / style の取り扱い

**Objective:** As a エンドユーザー, I want Next.js が注入するインライン script / style が CSP 下でも正しく機能すること, so that フレームワークが生成するページが CSP 違反で白画面・無操作にならない

#### Acceptance Criteria

1. When フレームワークがページのブートストラップのためにインライン script を注入したとき, the Web Frontend shall そのインライン script を CSP 下で実行可能な状態にする
2. When フレームワークまたはスタイリング機構がインライン style を注入したとき, the Web Frontend shall そのインライン style を CSP 下で適用可能な状態にする
3. If 想定外のインライン script / style が CSP によってブロックされたとき, the Web Frontend shall 画面の表示・操作が破綻しない状態を維持する（許可は最小限に留め、無条件な全許可に依存しない）

### Requirement 6: 記事本文 HTML 中の外部リソースの取り扱い

**Objective:** As a エンドユーザー, I want 記事本文に含まれる外部リソース（画像等）の表示可否が一貫した方針で決まること, so that 記事閲覧体験と安全性のトレードオフが予測可能になる

#### Acceptance Criteria

1. The `Content-Security-Policy` ヘッダー値 shall 記事本文 HTML 中の外部画像の取得可否を `img-src` ディレクティブで明示的に定義する
2. Where 記事本文 HTML 中に外部画像が含まれる場合, the Web Frontend shall その外部画像の表示可否を `img-src` の許可範囲に従って一貫して扱う（許可範囲外のオリジンはブロックする）
3. The `Content-Security-Policy` ヘッダー値 shall 記事本文 HTML 中のインライン script / 外部 script を `img-src` 以外のディレクティブで個別に制御し、画像の許可が script 実行の許可へ波及しないようにする

## Non-Functional Requirements

### NFR 1: 互換性

1. The Web Frontend shall 本変更の前後で既存の `headers()` 由来レスポンスヘッダー（#41 導入分）の値を変更せず保持する
2. The Web Frontend shall CSP 適用後も既存の自動テストスイート（`npm test`）を 0 件の新規失敗で完了させる

### NFR 2: セキュリティ可観測性

1. While 開発者がブラウザの開発者ツールでレスポンスヘッダーを確認しているとき, the Web Frontend shall `Content-Security-Policy` ヘッダー値を平文の HTTP ヘッダーとして可読な形で露出し、適用中のポリシーを目視確認可能にする

### NFR 3: 運用・配信環境互換

1. While アプリが本番ビルドとして配信されているとき, the Web Frontend shall CSP ヘッダーを付与した状態でページを正常に表示・操作可能にする

## Out of Scope

- API サーバー（`api`）側の HSTS / CSP 等のセキュリティヘッダー設定（#37 で扱う）
- バックエンドでの HTML サニタイズ（`bluemonday` で実施済み。本 Issue では再実装・変更しない）
- CSP 違反レポートの収集・集計基盤（`report-uri` / `report-to` によるレポーティング運用）の構築。本 Issue では CSP の付与と互換性維持に限定する
- CSP のレポートオンリーモード（`Content-Security-Policy-Report-Only`）での段階導入運用の設計。採用是非は実装判断に委ねる
- 外部フォント CDN・外部解析スクリプト・外部広告等、現状フロントで利用していない外部オリジンの許可設計（将来利用時に別途要件化）

## Open Questions

- インライン script / style の許可手段として nonce 方式（per-request nonce、ミドルウェアが必要）と `'unsafe-inline'` の一時許容のいずれを採るかは、本要件では「観測可能な振る舞い（インラインが機能し、かつ許可は最小限）」のみを定義し、具体的な実装手段は design / developer に委ねる。dev モード（HMR が eval / websocket を多用）と本番モードで CSP の厳しさを変える余地があるため、その方針も設計判断とする。
- 記事本文 HTML 中の外部画像を許可するか否か（`img-src` を `'self'` のみに絞るか、外部画像オリジンを広く許可するか）は、安全性と記事閲覧体験のトレードオフであり人間判断が必要。Issue 本文「仮案・判断を委ねたい点」が未回答のため、デフォルト方針（厳格寄り / 体験寄り）の確定を Issue コメントで仰ぐことを推奨する。
- `connect-src`（API / 認証エンドポイントへの fetch）・`style-src` / `script-src` / `font-src` 等の個別ディレクティブの最終的な許可値は、実際のフロント実装が要求するリソースの洗い出し（実機での CSP 違反観測）を経て確定する必要がある。現時点で外部オリジン接続は確認されていないが、漏れがないことの最終確認は実装フェーズで行う。
