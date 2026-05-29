# Requirements Document

## Introduction

Feedman のフィード一覧（画面左ペイン）には、各フィードの favicon を表示してユーザーが
視覚的に識別できるようにする UI 要件がある。現状はフィード配信 URL（RSS / Atom URL）を
基準に favicon を取得しているため、配信ドメインとサイト本体ドメインが異なるフィード
（例: 別ドメインで配信される RSS、CDN ホスト配信、ブログサービス独自ドメイン等）で
favicon が取得できず、フィード一覧で空白や壊れた画像が表示されるケースが発生している。

本機能では (1) フィード配信 URL での取得に失敗した場合にサイト本体のドメインを起点に
favicon を再探索し、(2) すべての取得経路で失敗した場合はフロントエンド側で壊れた画像を
出さずに代替アイコンを表示することで、フィード一覧の視認性を回復させる。既に favicon が
正しく取得・表示できている既存フィードに対して挙動を後退させないことを前提とする。

## Requirements

### Requirement 1: サイト本体ドメインからの favicon フォールバック取得

**Objective:** As a Feedman ユーザー, I want 配信 URL とサイト本体ドメインが異なるフィードでも favicon が表示されること, so that フィード一覧で視覚的にフィードを識別できる

#### Acceptance Criteria

1. When フィード登録時にフィード配信 URL を起点とする favicon 取得が失敗したとき, the Feed Service shall フィード内の記事リンクから得られるサイト本体ドメインを起点に favicon の再取得を試行する
2. When サイト本体ドメインを起点にした favicon 取得が成功したとき, the Feed Service shall 取得した favicon を当該フィードの favicon として永続化する
3. When フィードに記事リンクが 1 件も含まれていないとき, the Feed Service shall サイト本体ドメインからの再取得は試行せずに従来どおりフィード配信 URL を起点とした結果のみを採用する
4. When サイト本体ドメインからの favicon 取得においても画像を取得できなかったとき, the Feed Service shall 当該フィードの favicon を未取得（null）として永続化する
5. The Feed Service shall favicon 取得処理がフィード登録 API のレスポンス時間に影響しないよう、登録応答返却後のバックグラウンド処理として実行する

### Requirement 2: favicon 探索経路の段階化

**Objective:** As a Feedman 運用者, I want favicon 探索が決定論的な順序で段階実行されること, so that 取得成功率を高めつつ取得経路を観測・再現できる

#### Acceptance Criteria

1. The Feed Service shall favicon 取得を「フィード配信 URL を起点とする取得」「サイト本体ドメインを起点とする取得」の順で段階的に試行する
2. When いずれかの段階で画像として有効な favicon を取得できたとき, the Feed Service shall 後続段階の試行をスキップして取得済み favicon を採用する
3. While 各取得段階を実行している間, the Feed Service shall 取得対象 URL・成功可否・採用された段階を運用ログに記録する
4. The Feed Service shall サイト本体ドメインを起点とする探索において、ドメイントップの favicon 既定パスと、サイト HTML 内で明示宣言された favicon 参照の両方を探索対象とする

### Requirement 3: フロントエンドのデフォルトアイコン表示

**Objective:** As a Feedman ユーザー, I want favicon が無い／取得できないフィードでも壊れた画像ではなく代替アイコンが表示されること, so that フィード一覧の視認性が損なわれない

#### Acceptance Criteria

1. When フィードの favicon が未設定（null または空）であるとき, the Feed List UI shall 当該フィード行に既定の代替アイコンを表示する
2. If favicon の参照先画像の読み込みに失敗したとき, the Feed List UI shall 壊れた画像表示に代えて既定の代替アイコンを表示する
3. The Feed List UI shall 代替アイコンを実際の favicon と同じ表示サイズ・同じ配置領域で描画する
4. The Feed List UI shall 代替アイコン表示時もフィードタイトル・未読数バッジ・ステータスアイコンのレイアウトを変化させない

### Requirement 4: 既存正常ケースの非リグレッション

**Objective:** As a Feedman 既存利用ユーザー, I want 現在 favicon が正しく表示されているフィードの挙動が変わらないこと, so that 本修正によって表示崩れや再取得負荷が発生しない

#### Acceptance Criteria

1. When フィード配信 URL を起点とする従来経路で favicon が取得できる既存フィードを登録するとき, the Feed Service shall 従来と同一の favicon を採用する
2. While 既に favicon が永続化されているフィードについて, the Feed Service shall 本要件によるフォールバック取得を理由とした再取得を行わない
3. The Feed List UI shall 既存の favicon 表示済みフィードの表示位置・サイズ・代替表示ロジック以外の見た目を変更しない

## Non-Functional Requirements

### NFR 1: 外部 URL 取得の安全性

1. The Feed Service shall サイト本体ドメインを起点とした favicon 取得においても、フィード配信 URL を起点とする既存経路と同等の SSRF 対策・ホスト検証を適用する
2. The Feed Service shall サイト本体ドメインを起点とした favicon 取得においても、フィード配信 URL を起点とする既存経路と同じタイムアウト上限と同じレスポンスサイズ上限を適用する
3. If サイト本体ドメインの HTML を解析する必要があるとき, the Feed Service shall 既存実装と同等の HTML サニタイズ方針および画像 MIME 判定基準を適用する

### NFR 2: 可観測性

1. The Feed Service shall favicon 取得結果（採用段階・成功失敗）を構造化ログとして出力し、運用者が後追いでフィード単位に取得経路を特定できるようにする

### NFR 3: 後方互換性

1. The Feed Service shall 本要件導入前に永続化されたフィードの favicon データ・スキーマ・公開 API レスポンス形状を変更しない
2. The Feed List UI shall 本要件導入前に登録された favicon を持つフィードの表示挙動を変更しない

## Out of Scope

- ユーザーが手動で favicon 画像をアップロードしてカスタマイズする機能
- 既に favicon が永続化されているフィードに対するバッチ的な再取得・再評価
- favicon キャッシュ TTL の見直し、CDN 配信、解像度別バリアント生成
- フィード一覧以外の UI（記事詳細・設定画面 等）における favicon 表示挙動の変更
- フィード配信 URL とサイト本体ドメインの対応関係をユーザーに編集させる UI

## Open Questions

- なし（Issue 本文・既存実装・コメントから挙動仕様は確定可能。実装手段の選定（HTML 解析ライブラリ、探索パターンの細部、ログ項目名 等）は design.md の領分）
