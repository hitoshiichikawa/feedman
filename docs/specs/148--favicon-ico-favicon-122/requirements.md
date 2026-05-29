# Requirements Document

## Introduction

#122 で実装したフィード favicon 取得は、配信ドメインの `/favicon.ico` が
「HTTP 2xx・`image/*` MIME・サイズ上限内」であれば取得成功と判定し、その時点で
後続のフォールバック段階（サイト本体ドメインの HTML `<link rel="icon">` 解析等）を
打ち切る仕様になっている。しかし実環境では、ドメインによっては `/favicon.ico` が
**全ピクセル alpha=0 の透明 ICO** を返すケースがあり（例: rocketnews24.com の
16x16 透明 ICO）、現状仕様では「成功」と判定されてしまい、フィード一覧で
描画されない（見えない）favicon が永続化される問題が発生している。

本 Issue では favicon 取得処理における「成功判定」に画像内容の検証を加え、
**全ピクセル透明な画像は取得失敗と判定**してフォールバック探索および UI 側の
既定アイコン表示が正しく機能するように仕様を補強する。判定基準は人間決定により
**「全面透明（全ピクセルの alpha チャネルが 0）」のみ**を取得失敗扱いとし、
極小サイズ・高透明率・部分透明などのケースは本 Issue のスコープ外とする。

## Requirements

### Requirement 1: 全面透明 favicon の取得失敗判定

**Objective:** As a Feedman ユーザー, I want 描画されない透明な favicon が採用されずに後続のフォールバックが機能すること, so that フィード一覧で当該フィードに既定の代替アイコンが表示される

#### Acceptance Criteria

1. When favicon 取得処理が画像データを受領したとき, the Feed Service shall 受領した画像をデコードし全ピクセルの alpha チャネルが 0 であるかを検証する
2. If デコード結果として全ピクセルの alpha チャネルが 0 であると判定されたとき, the Feed Service shall 当該段階の取得を失敗として扱い favicon データを採用しない
3. When 画像形式が alpha チャネルを持たない形式（JPEG 等の不透明前提形式）であるとき, the Feed Service shall 透明判定の対象外として扱い従来どおり取得成功として採用する
4. If 受領した画像データがデコードできない（形式不明・破損等）とき, the Feed Service shall 当該段階の取得を失敗として扱い favicon データを採用しない
5. When 全面透明と判定され当該段階を失敗扱いにしたとき, the Feed Service shall #122 で規定された後続のフォールバック段階を継続実行する

### Requirement 2: フォールバック完走後の挙動

**Objective:** As a Feedman ユーザー, I want 全段階で有効な favicon が得られない場合に既定アイコンが表示されること, so that 透明 favicon を返すドメインのフィードでも視認性が損なわれない

#### Acceptance Criteria

1. When フォールバックの全段階が透明判定または取得失敗で打ち切られたとき, the Feed Service shall 当該フィードの favicon を未取得（null）として永続化する
2. While いずれかのフォールバック段階で有効（全面透明でない）な favicon が取得できた場合, the Feed Service shall 当該段階で取得した favicon を採用し以降の段階を試行しない
3. When favicon が未取得（null）として永続化されたフィードを表示するとき, the Feed List UI shall #122 Requirement 3 に準拠して既定の代替アイコンを表示する

### Requirement 3: 透明判定の可観測性

**Objective:** As a Feedman 運用者, I want 透明判定により段階失敗扱いになったケースを後追いできること, so that 透明 favicon を返すドメインの分布や影響範囲を運用判断できる

#### Acceptance Criteria

1. When 受領画像が全面透明と判定され段階失敗扱いになったとき, the Feed Service shall 取得対象 URL・該当段階ラベル・透明判定により失敗扱いとした旨を構造化ログに記録する
2. When 受領画像がデコード失敗により段階失敗扱いになったとき, the Feed Service shall 取得対象 URL・該当段階ラベル・デコード失敗である旨を構造化ログに記録する

### Requirement 4: 既存正常ケースの非リグレッション

**Objective:** As a Feedman 既存利用ユーザー, I want 透明 favicon を返さない既存フィードの取得挙動が変わらないこと, so that 本修正によって既存フィードの favicon 表示が後退しない

#### Acceptance Criteria

1. When alpha チャネルを持つ画像形式で 1 ピクセル以上が alpha 非 0 の favicon を受領したとき, the Feed Service shall 従来どおり当該画像を取得成功として採用する
2. When alpha チャネルを持たない画像形式（JPEG 等）の favicon を受領したとき, the Feed Service shall 従来どおり当該画像を取得成功として採用する
3. While 既に favicon が永続化されているフィードについて, the Feed Service shall 本要件導入を理由とした再評価・再取得を行わない

## Non-Functional Requirements

### NFR 1: 安全性

1. The Feed Service shall 本要件で追加する画像内容検証においても、#122 NFR 1 と同等の SSRF 対策・ホスト検証・タイムアウト上限・レスポンスサイズ上限を維持する
2. The Feed Service shall 透明判定のためのデコード対象を favicon 取得経路のサイズ上限（2MB）以内の画像に限定し、上限超過時はデコードを行わず段階失敗として扱う

### NFR 2: 性能

1. The Feed Service shall 透明判定処理を 1 画像あたり 100ms 以内で完了させる（フィード登録のバックグラウンド処理時間を実質的に増大させないため）
2. The Feed Service shall 透明判定のためのデコードを取得成功候補の画像（HTTP 2xx・`image/*` MIME・サイズ上限内）に限定し、それ以前のチェックで失格となった画像にはデコードを行わない

### NFR 3: 後方互換性

1. The Feed Service shall 本要件導入前に永続化された favicon データ・スキーマ・公開 API レスポンス形状を変更しない

## Out of Scope

- フォールバック順序の全面見直し（HTML `<link rel="icon">` を `/favicon.ico` より優先する方針変更）
- 既存 staging / 本番 DB に永続化されている透明 favicon データの修復（再フェッチ・`favicon_data` の NULL 化等の運用作業）
- 全面透明以外の「描画されない favicon」ケース（1x1 等の極小サイズ・部分透明・高透明率（alpha が極端に低いがゼロではない）・単色塗りつぶし等）の判定。本 Issue では扱わず、必要であれば別 Issue で検討
- 透明判定基準の動的設定化（環境変数・設定ファイルからの閾値変更機構）
- フィード登録 API のレスポンス契約変更（透明判定のため登録時間が変動する場合の API 側挙動変更）

## Open Questions

- なし（判定基準は人間決定により「全面透明のみ（全ピクセル alpha=0）」で確定。実装手段の選定（採用する画像デコードライブラリ、ICO / PNG / WebP 等の対応形式範囲、デコード失敗時のログ項目名 等）は design.md の領分）
