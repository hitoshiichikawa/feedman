# Requirements Document

## Introduction

フィード登録（`POST /api/feeds`）は成功し DB にも購読が作成されるにもかかわらず、購読一覧
（左ペイン）に何も表示されない不具合が発生している。原因は `GET /api/subscriptions` が、
favicon を未取得または favicon を持たないフィードについて `favicon_mime` が NULL のとき、
これを非 NULL 文字列フィールドへ読み取ろうとして失敗し、一覧 API 全体が 500 を返している
ことにある。favicon を持たないフィードを 1 件でも購読していると一覧全体が表示不能になる
致命的な導線であり、本 Issue 単独で修正する。本要件はユーザーから観測可能な挙動
（NULL の favicon でも 200 で一覧が返ること・既存レスポンス形を変えないこと）を定義する。

## Requirements

### Requirement 1: favicon 未設定フィードを含む購読一覧の取得成功

**Objective:** As a ログインユーザー, I want favicon を持たないフィードを購読していても購読一覧が取得できること, so that 左ペインに購読中フィードが正しく表示される

#### Acceptance Criteria

1. When ユーザーが購読一覧を要求し購読中のいずれかのフィードが favicon mime 情報を持たないとき, the Subscriptions API shall HTTP 200 と購読一覧を返す
2. When ユーザーが購読一覧を要求し購読中のフィードがすべて favicon mime 情報を持つとき, the Subscriptions API shall HTTP 200 と購読一覧を返す
3. When ユーザーが favicon mime 情報を持たないフィードを購読しているとき, the Subscriptions API shall そのフィードの favicon mime を空文字として返す
4. When ユーザーが購読を 1 件も持たないとき, the Subscriptions API shall HTTP 200 と空の購読一覧を返す

### Requirement 2: favicon あり/なし混在時の全件返却

**Objective:** As a ログインユーザー, I want favicon を持つフィードと持たないフィードを混在して購読していても全件取得できること, so that 一部フィードの状態に依存せず一覧全体が表示される

#### Acceptance Criteria

1. When ユーザーが favicon mime 情報を持つフィードと持たないフィードの両方を購読しているとき, the Subscriptions API shall すべての購読を漏れなく返す
2. While 購読中の一部フィードが favicon mime 情報を持たない状態であるとき, the Subscriptions API shall favicon を持つ他フィードの favicon mime を従来どおり実際の値で返す

### Requirement 3: 異常系での一覧全体の保護

**Objective:** As a ログインユーザー, I want 個々のフィードの favicon 取得状態によって一覧 API 全体が失敗しないこと, so that フィード状態の差異で購読一覧が空表示になる事象が再発しない

#### Acceptance Criteria

1. If 購読中のフィードの favicon mime 情報が未設定（NULL 相当）であるとき, the Subscriptions API shall HTTP 500 を返さず一覧取得を完了する
2. If favicon を持たないフィードを購読した直後に一覧を要求したとき, the Subscriptions API shall 当該フィードを含む購読一覧を返す

### Requirement 4: 既存レスポンス形の後方互換

**Objective:** As a フロントエンド（web）開発者, I want 購読一覧 API のレスポンス構造が修正前後で変わらないこと, so that 既存の左ペイン表示ロジックを変更せずに不具合だけが解消される

#### Acceptance Criteria

1. The Subscriptions API shall 修正前と同一の JSON レスポンス構造（フィールド名・型・階層）を維持する
2. When favicon mime 情報を持つフィードを返すとき, the Subscriptions API shall 修正前と同一の favicon mime 値を返す

## Non-Functional Requirements

### NFR 1: 回帰検証可能性

1. The Subscriptions API shall favicon mime 情報が未設定のフィードを購読した状態で一覧取得が成功することを、テスト用 PostgreSQL を介した結合テストで検証可能とする
2. The Subscriptions API shall favicon あり/なしが混在する購読状態で全件返却されることを、テスト用 PostgreSQL を介した結合テストで検証可能とする

## Out of Scope

- worker のフェッチ処理の不具合（#98）の修正。本 Issue とは独立であり、#98 を修正しても
  favicon を持たないフィードでは favicon mime が未設定のまま残るため、本 Issue は単独で修正する
- `favicon_data`（バイナリ）の取り扱い変更。バイナリ列は未設定でも読み取り可能であり本不具合の原因ではない
- favicon を実際に取得・保存するロジックの追加・変更
- 購読一覧の表示順・未読数集計・ページネーション等、本不具合に関係しない一覧仕様の変更
- フロントエンド（web）側の表示ロジックの変更（レスポンス形を維持するため不要）

## Open Questions

- なし（Issue 本文が確定要件。人間による追加の決定事項・要件はコメント上に存在しない）
