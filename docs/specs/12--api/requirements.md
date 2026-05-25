# Requirements Document

## Introduction

はてなブックマーク連携クライアントは、はてブ一括取得 API のレスポンスボディを全量メモリに
読み込んでから JSON パースしている。レスポンスが想定外に巨大だった場合（不正なサーバ応答・
中間プロキシによる挙動など）、読み込みサイズが無制限のためメモリを際限なく消費し、
ワーカープロセスが OOM に陥るリスクがある。本要件では、はてブ API レスポンスボディの
読み込みサイズに上限を設け、上限内の正常レスポンスは従来どおりブックマーク数マップとして
返しつつ、上限を超える異常レスポンスをエラーとして検出・記録できるようにする。後方互換
（正常レスポンスのパース結果・戻り値の型）は維持し、API エンドポイントやリクエスト方式・
JSON スキーマには手を加えない。

## Requirements

### Requirement 1: レスポンスボディ読み込みサイズの上限適用

**Objective:** As a Feedman の運用者, I want はてブ API レスポンスの読み込みサイズに明示的な上限が適用されること, so that 巨大な異常レスポンスを受信してもワーカーのメモリ消費が際限なく増大せず OOM を回避できる

#### Acceptance Criteria

1. The HatebuClient shall はてブブックマーク数取得時のレスポンスボディ読み込み量を、あらかじめ定義された上限サイズ以内に制限する
2. The HatebuClient shall 読み込みサイズの上限値を 1 MiB（1,048,576 バイト）として扱う
3. While 受信レスポンスボディが上限サイズ以内であるとき, the HatebuClient shall レスポンスボディの全量を読み込む

### Requirement 2: 上限内レスポンスの後方互換なパース

**Objective:** As a Feedman の利用者, I want 上限内の正常レスポンスが従来どおり処理されること, so that ブックマーク数表示の挙動が本変更の前後で変わらない

#### Acceptance Criteria

1. When 上限サイズ以内の正常な JSON レスポンスを受信したとき, the HatebuClient shall 各 URL のブックマーク数を対応付けたマップを呼び出し元に返す
2. When 上限サイズ以内のレスポンスにリクエストした一部の URL が含まれていないとき, the HatebuClient shall 含まれない URL のブックマーク数を 0 件として補完したマップを返す
3. When 空の URL リストを受け取ったとき, the HatebuClient shall API を呼び出さず空のブックマーク数マップを返す
4. The HatebuClient shall 上限内レスポンスに対して、本変更の前と同一の戻り値の型でブックマーク数マップを返す

### Requirement 3: 上限超過レスポンスのエラー検出と記録

**Objective:** As a Feedman の運用者, I want 上限を超える異常レスポンスがサイレントに切り詰められずエラーとして区別されること, so that 不完全なデータを誤ってパースする事故を防ぎ、異常発生を検知できる

#### Acceptance Criteria

1. If 受信レスポンスボディが上限サイズを超過したとき, the HatebuClient shall ブックマーク数マップを返さずエラーを呼び出し元に返す
2. If 受信レスポンスボディが上限サイズを超過したとき, the HatebuClient shall 上限超過を示すログを出力する
3. If 受信レスポンスボディが上限サイズを超過したとき, the HatebuClient shall 切り詰めた不完全なボディを正常結果としてパース・返却しない

### Requirement 4: 上限境界の正確な扱い

**Objective:** As a Feedman の開発者, I want 上限ちょうどのサイズのレスポンスが正しく処理されること, so that オフバイワン誤りで正常レスポンスを誤ってエラー扱いしない

#### Acceptance Criteria

1. When ボディサイズが上限値とちょうど等しい正常な JSON レスポンスを受信したとき, the HatebuClient shall そのレスポンスをエラーとせずブックマーク数マップにパースして返す
2. If ボディサイズが上限値を 1 バイトでも超過したとき, the HatebuClient shall そのレスポンスを上限超過エラーとして扱う

### Requirement 5: 既存エラー処理経路の不変性

**Objective:** As a Feedman の開発者, I want 既存の HTTP ステータス異常・通信失敗時のエラー処理が本変更の影響を受けないこと, so that 本変更が既存のエラーハンドリングを退行させない

#### Acceptance Criteria

1. If はてブ API が 200 以外の HTTP ステータスを返したとき, the HatebuClient shall ボディ読み込み上限処理に到達せずステータス異常エラーを返す
2. If はてブ API へのリクエスト送信が失敗したとき, the HatebuClient shall ボディ読み込み上限処理に到達せず通信失敗エラーを返す

## Non-Functional Requirements

### NFR 1: メモリ消費の上限

1. While 単一のはてブ API レスポンスを処理しているとき, the HatebuClient shall レスポンスボディ起因のメモリ確保量を上限サイズ（1 MiB）に概ね比例する範囲に抑える

### NFR 2: 保守性（マジックナンバー排除）

1. The HatebuClient shall 読み込み上限値を、意味のある名前を持つ単一の定数として定義し、リテラル値の散在を避ける

### NFR 3: 後方互換性

1. The HatebuClient shall 本変更の前後で、ブックマーク数取得の公開シグネチャ（引数・戻り値の型）を変更しない

## Out of Scope

- はてブ API エンドポイント URL・リクエスト方式（GET / クエリ構築・User-Agent 等）の変更
- レスポンス JSON スキーマおよびパースロジック（マップへの変換・0 件補完規則）の変更
- 1 リクエストあたりの最大 URL 数（既存の上限）の変更
- 上限値を環境変数・設定ファイルで外部から変更可能にする仕組みの追加
- リトライ・タイムアウト・レート制限など読み込みサイズ以外のエラーハンドリング全般の見直し

## Open Questions

- なし（上限値 1 MiB の採用、および上限超過時にサイレント切り詰めではなくエラーとして区別する方針は、Issue 本文の仮案と既存実装の慣習に沿って PM が確定済み。根拠は Introduction および各 Requirement の Objective に記載）
