# Requirements Document

## Introduction

Feedman の v1 モバイルクライアント（Android / iOS）は、Web フロントエンドと共通のサーバー
API を利用してログイン後の主要機能（ユーザー情報取得・記事一覧・記事詳細・検索・購読操作）
を実現する。しかし現状はサーバー実装と Web フロント実装の暗黙の合意で動いており、
モバイル側が独立して参照できる「v1 共通 API 契約ドキュメント」が存在しない。また現行
サーバー応答にはモバイルクライアントが期待する一部情報（モバイル向け統一ユーザー情報
エンドポイント・記事詳細のフィード表示メタデータ）が欠けている。

本 spec は、(1) v1 モバイル API 契約を独立した文書として明文化し、(2) モバイルクライアントが
依存する 2 点（統一ユーザー情報エンドポイント・記事詳細のフィードメタデータ）をサーバー応答
として追加し、(3) 既存 Web Cookie 動線および既存契約フィールドの後方互換を維持することを
対象とする。Native auth の token 発行・rotation・revoke 契約は別 spec（#172 / #163 配下）で
扱い済みであり、本 spec では発行済み access token を用いて認証済み API を呼び出すクライアント
共通動線のみを扱う。

## Requirements

### Requirement 1: v1 モバイル API 契約ドキュメントの明文化

**Objective:** As a Feedman モバイルクライアント実装者, I want v1 で利用するサーバー API
契約を独立した文書として参照したい, so that Android / iOS の各クライアントがサーバー実装
コードを読まずに独立して実装・検証できる

#### Acceptance Criteria

1. The Mobile API Contract Document shall v1 モバイルクライアントが利用する共通方針（認証ヘッダ・エラー応答形式・JSON フィールド命名規約）を記載する
2. The Mobile API Contract Document shall native auth 系エンドポイント（token 発行・refresh・revoke・native callback）の契約サマリを記載し、詳細は既存 spec を参照する形で明示する
3. The Mobile API Contract Document shall 統一ユーザー情報取得エンドポイントの URL・認証方式・成功応答 JSON フィールド・エラー応答を記載する
4. The Mobile API Contract Document shall 記事一覧・記事詳細・検索・購読操作・クロスフィード閲覧の各エンドポイントについて、URL・認証方式・主要クエリ／パスパラメータ・成功応答 JSON フィールド・エラー応答を記載する
5. The Mobile API Contract Document shall v1 スコープ外として除外するエンドポイント（デバイス登録・キーワード通知）を「次フェーズ」として明示し、v1 では呼び出さない旨を記録する
6. The README shall モバイル統一ユーザー情報取得エンドポイント・native auth 系エンドポイント・クロスフィード閲覧・検索の各 API を一覧に追記し、契約文書への参照を含める

### Requirement 2: 統一ユーザー情報取得エンドポイントの新設

**Objective:** As a Feedman モバイルクライアント, I want Bearer token と Web Cookie の
いずれでも同一の URL で current user 情報を取得したい, so that モバイルと Web が同じ
クライアントコードパスで認証済みユーザー情報を扱える

#### Acceptance Criteria

1. When 有効な Bearer access token を Authorization ヘッダで提示した状態でモバイル統一ユーザー情報取得エンドポイントが要求されたとき, the Feedman API shall 当該 token の主体である current user の情報を JSON で応答する
2. When 有効な Web Cookie セッションを提示した状態でモバイル統一ユーザー情報取得エンドポイントが要求されたとき, the Feedman API shall 当該セッションの主体である current user の情報を JSON で応答する
3. The モバイル統一ユーザー情報取得エンドポイント shall 成功応答に `id`, `email`, `name`, `avatar_url` のフィールドを含める
4. Where current user に `avatar_url` 値が存在しないとき, the モバイル統一ユーザー情報取得エンドポイント shall `avatar_url` を JSON null または応答からの省略として表現する
5. If 認証ヘッダもセッションも提示されない、または提示された認証情報が無効である状態でモバイル統一ユーザー情報取得エンドポイントが要求されたとき, the Feedman API shall 未認証として要求を拒否する
6. The 既存 Web 専用ユーザー情報取得エンドポイント `/auth/me` shall 本 spec 導入前と同一の URL・同一の Web Cookie 認証方式・同一の応答形状で動作する

### Requirement 3: 記事詳細応答へのフィード表示メタデータ追加

**Objective:** As a Feedman モバイルクライアント, I want 記事詳細応答に当該記事が属する
フィードの表示用メタデータ（タイトル・favicon）を含めて受け取りたい, so that 記事詳細
画面でフィードのコンテキストを表示する際に追加 API 呼び出しを必要としない

#### Acceptance Criteria

1. When 認証済み要求で記事詳細エンドポイントが特定の記事 ID に対して呼び出されたとき, the Feedman API shall 応答 JSON に当該記事が属するフィードの表示タイトルを示すフィールドを含める
2. When 認証済み要求で記事詳細エンドポイントが特定の記事 ID に対して呼び出されたとき, the Feedman API shall 応答 JSON に当該記事が属するフィードの favicon を示すフィールドを含める
3. The 記事詳細エンドポイント shall 本 spec 導入前から応答に含まれていた既存フィールド（記事 ID・フィード ID・タイトル・リンク・サマリ・本文・著者・公開日時・日時推定フラグ・既読フラグ・スター付与フラグ・はてなブックマーク件数）を引き続き同一フィールド名で含める
4. Where フィードに favicon が登録されていないとき, the 記事詳細エンドポイント shall favicon メタデータフィールドを JSON null または応答からの省略として表現する
5. If 要求された記事 ID が要求元ユーザーの購読範囲に存在しないとき, the 記事詳細エンドポイント shall 本 spec 導入前と同一の拒否応答を返す

### Requirement 4: v1 クライアント共通動線の契約テスト

**Objective:** As a Feedman 運用者, I want v1 モバイルクライアント共通動線の主要契約が
自動テストで検証されてほしい, so that 後続変更で契約から外れた応答が混入しても CI で
検出できる

#### Acceptance Criteria

1. The Mobile API Contract Test Suite shall Bearer access token 提示時にモバイル統一ユーザー情報取得エンドポイントが current user を返すことを検証する
2. The Mobile API Contract Test Suite shall Web Cookie セッション提示時にモバイル統一ユーザー情報取得エンドポイントが current user を返すことを検証する
3. The Mobile API Contract Test Suite shall 既存 Web 専用ユーザー情報取得エンドポイント `/auth/me` の Web Cookie 動線が本 spec 導入前と同一に動作することを検証する
4. The Mobile API Contract Test Suite shall 記事詳細エンドポイントの応答にフィード表示タイトルとフィード favicon メタデータの両フィールドが含まれることを検証する
5. The Mobile API Contract Test Suite shall 記事詳細エンドポイントの応答に既存フィールド一式が引き続き含まれることを検証する

## Non-Functional Requirements

### NFR 1: 後方互換性

1. The Feedman API shall 既存 Web Cookie セッション認証で利用されている全エンドポイントの URL・認証方式・応答形状を本 spec 導入により変更しない
2. The Feedman API shall 既存記事詳細エンドポイントの既存応答フィールドのフィールド名・型・null 表現を本 spec 導入により変更しない
3. The Feedman API shall 本 spec の変更により既存の自動テストスイートが追加の修正なしに通過する状態を維持する

### NFR 2: 契約文書の独立可読性

1. The Mobile API Contract Document shall モバイルクライアント実装者がサーバー実装ソースコードを参照せずに、文書のみで v1 で呼び出す全エンドポイントの URL・認証方式・要求／応答 JSON 形状を特定できる粒度で記載する
2. The Mobile API Contract Document shall v1 スコープに含まれるエンドポイントと v1 スコープ外として次フェーズに延期するエンドポイントを明示的に区別して記載する

### NFR 3: テストの自動実行性

1. The Mobile API Contract Test Suite shall 標準のリポジトリテスト実行コマンドで他テストと併せて実行可能な配置とし、外部ネットワーク・実 OAuth プロバイダ・実フィード取得への接続を必要としない

## Out of Scope

- Native auth 系エンドポイント（token 発行・refresh・revoke・native callback）の実装変更
  および契約テストの新規整備（#163 配下の #164〜#171 で実装済み、#172 spec で end-to-end
  契約テストを整備済み。本 spec では契約文書サマリ記載のみ）
- `/api/devices`（デバイス登録）および `/api/keywords`（キーワード通知）の実装。これらは
  キーワードプッシュ通知の次フェーズに該当し、v1 接続確認の必須契約に含めない
- モバイルクライアント（Android / iOS）側の実装変更
- 既存記事一覧 / 検索 / 購読操作 / クロスフィード閲覧エンドポイントの新規フィールド追加。
  本 spec では既存挙動を契約文書として固定するに留め、応答拡張は行わない（記事詳細応答への
  フィードメタデータ追加のみ例外として本 spec で扱う）
- Web フロントエンドの実装変更（モバイル統一ユーザー情報取得エンドポイントを Web からも
  呼び出すかは本 spec では決定しない）

## Open Questions

- なし（v1 スコープ外項目・後方互換維持方針・既存 `/auth/me` の保持方針は Issue #207 本文で
  決着済み。`avatar_url` の null 表現可否および favicon メタデータの null 表現可否は
  Requirement 2.4 / Requirement 3.4 で「JSON null または応答からの省略」を許容として明示）

## 関連

- Related: #163 #169 #172
