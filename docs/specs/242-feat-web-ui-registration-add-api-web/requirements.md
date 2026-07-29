# Requirements Document

## Introduction

Feedman Web（Next.js）のアカウント設定には、認証済みユーザーが 2 つ目以降のパスキーを登録する
導線が存在しない。既に本 spec 対象のサーバ側追加登録 API（`POST /api/passkey/registration/add/begin`
と `POST /api/passkey/registration/add/finish`、excludeCredentials 付きチャレンジを含む）は
Issue #216 で main に投入済みで、iOS 側でも同 endpoint を利用予定であり、当該 endpoint は
既存の Web Cookie セッション認証で到達可能である（実装前調査で確認済み）。したがって本 spec の
スコープはバックエンド変更を含まない **Web 配線のみ** となる。

現状「パスキーのみで登録したアカウント」はパスキーを 1 つ失うとアカウントへのアクセス手段が
完全に失われる。業界の主要なロスト対策は「同一アカウントに複数のパスキーを登録しておく」
（別の端末・別の同期元でバックアップを持つ）ことであり、本 spec はこの導線を認証済みユーザーに
提供する。追加後のログイン可否そのものはサーバ側の add endpoint 契約と既存パスキー認証フローで
担保されるため、本 spec の受入基準は **Web 側から観測可能な「追加登録成功が伝達される」ことと
「壊れた状態にならないこと」**に集中する。

本 spec は Google 由来ログイン（パスキー未登録を含む）ユーザーが同じ導線からパスキーを追加できる
ことも受入基準として要件化する。パスキー credential の一覧・削除・リネーム UI、リカバリコード、
iOS 側の実装、サーバ側 API 変更は本 spec のスコープ外とする。

## Requirements

### Requirement 1: 追加パスキー登録導線の表示条件

**Objective:** As a 認証済みユーザー, I want アカウント設定画面から「パスキーを追加」導線に
アクセスしたい, so that 2 つ目以降のパスキーを自力で登録してアカウント喪失リスクを下げられる

#### Acceptance Criteria

1. While 認証済みユーザーが Account Settings UI を開いているとき, the Account Settings UI shall
   パスキー追加登録の起動要素を表示する
2. While 未認証状態でアプリを閲覧しているとき, the Web Application shall パスキー追加登録の起動
   要素を表示しない
3. While アカウント情報の取得が完了する前に Account Settings UI が開かれているとき, the Account
   Settings UI shall パスキー追加登録の起動要素を確定的に操作可能な状態としては提示しない
4. If アカウント情報の取得に失敗したとき, the Account Settings UI shall パスキー追加登録の起動
   要素を確定的に操作可能な状態としては提示しない
5. When ユーザーがパスキー追加登録の起動要素を操作したとき, the Account Settings UI shall
   パスキー追加登録フロー（Requirement 2）を開始する

### Requirement 2: 追加パスキー登録フローの完了と成功の伝達

**Objective:** As a 認証済みユーザー, I want ブラウザのパスキー作成 UI から 2 つ目以降のパスキーを
登録したい, so that 追加登録が成功したことを Web 画面上で確認できる

#### Acceptance Criteria

1. When ユーザーがパスキー追加登録を開始したとき, the Web Passkey Add Registration Flow shall
   現在ログイン中のユーザー識別子に紐付いた追加登録用チャレンジをサーバから取得する
2. When 追加登録用チャレンジを受領したとき, the Web Passkey Add Registration Flow shall
   ブラウザのパスキー作成 UI を起動する
3. When ブラウザのパスキー作成 UI が成功で完了したとき, the Web Passkey Add Registration Flow
   shall 生成された credential 情報を現在ログイン中のユーザーとして追加登録用の確定処理へ提示する
4. When サーバが追加登録応答を成功として受理したとき, the Account Settings UI shall 追加登録の
   成功をユーザーが視認できる形で提示する
5. When 追加登録が成功したとき, the Web Application shall ユーザーの認証済みセッションを維持し、
   追加登録操作の前後で他画面の表示や既存機能の利用可否を変化させない

### Requirement 3: 同一 authenticator の重複登録抑止とユーザー可読なエラー

**Objective:** As a 認証済みユーザー, I want 既に登録済みの authenticator を誤って再登録しよう
としたときに重複と分かる形でエラー表示されたい, so that 「登録に失敗したのか同じ端末を選んだの
か」を混同せず、別の端末・別の同期先で再試行できる

#### Acceptance Criteria

1. When ブラウザが追加登録要求を「同一アカウントに既に登録済みの authenticator」であることを
   理由に拒否したとき, the Account Settings UI shall 追加登録を確定させず、重複登録である旨を
   ユーザーが認識できる形で提示する
2. If 重複登録が発生したとき, the Web Passkey Add Registration Flow shall サーバ側の追加登録
   確定処理を呼び出さない
3. While 重複登録エラーが表示されているとき, the Account Settings UI shall ユーザーが別の
   authenticator を用いて追加登録を再試行できる状態を維持する
4. The Account Settings UI shall 重複登録エラーを、他のキャンセル・拒否理由（Requirement 4）と
   ユーザーが区別できる形で提示する

### Requirement 4: キャンセル・失敗時の UI 復旧と再試行

**Objective:** As a 認証済みユーザー, I want 追加登録のキャンセル・失敗時に画面が壊れず再試行
できる状態に戻ってほしい, so that 中途状態・誤操作・ネットワーク断で詰まらずに次の操作へ進める

#### Acceptance Criteria

1. If ユーザーがブラウザのパスキー作成 UI 上でキャンセルまたは拒否したとき, the Account Settings
   UI shall 追加登録を確定させず、Account Settings UI の表示を維持したまま再試行操作を提示する
2. If サーバが追加登録応答を拒否した（チャレンジ期限切れ・attestation 不正・レート制限超過等）
   とき, the Account Settings UI shall 追加登録を確定させず、拒否理由の内部区別をユーザー画面
   に反射しない汎用エラー表示を提示する
3. If 追加登録処理中にネットワーク断・サーバ内部エラーが発生したとき, the Account Settings UI
   shall 追加登録を確定させず、ユーザーが再試行操作を選択できる状態を提示する
4. While 追加登録要求の応答を待機している間, the Account Settings UI shall 追加登録の起動要素を
   非活性化し、多重送信を防止する
5. When 追加登録処理が終了（成功・失敗・キャンセルのいずれか）したとき, the Account Settings UI
   shall 追加登録の起動要素を再操作可能な状態に戻す
6. When キャンセル・失敗が発生したとき, the Web Application shall ユーザーの認証済みセッション
   を維持し、既存機能の利用可否を変化させない

### Requirement 5: Google 由来アカウントからの追加登録

**Objective:** As a Google OAuth 由来でログインしパスキー未登録を含む認証済みユーザー, I want
同じ「パスキーを追加」導線から自分のアカウントにパスキーを登録したい, so that Google Sign-In
だけでなくパスキーでもログインできる状態に移行できる

#### Acceptance Criteria

1. While Google OAuth 由来でログインした認証済みユーザーが Account Settings UI を開いていて、
   当該ユーザーにまだパスキーが登録されていないとき, the Account Settings UI shall パスキー
   追加登録の起動要素を Requirement 1.1 と同一の位置・同一の操作性で表示する
2. When Google 由来ユーザーが初回のパスキー登録を成功で完了したとき, the Web Application shall
   既存の Google 紐付けを解除・変更せず、当該アカウントを両方のログイン手段で利用可能な状態に
   する
3. When Google 由来ユーザーの初回パスキー登録を実行するとき, the Web Passkey Add Registration
   Flow shall 既にパスキーを持つユーザーが追加登録する場合と同じ導線・同じ操作手順で処理を進行
   させる

### Requirement 6: ロスト対策コンテキストの提示

**Objective:** As a 認証済みユーザー, I want パスキー追加登録がなぜ推奨されるのかを Web 画面上で
読める, so that 「別の端末や同期先を追加しておくことがアカウント喪失リスクを下げる」ことを理解
して意思決定できる

#### Acceptance Criteria

1. While Account Settings UI にパスキー追加登録の起動要素が表示されているとき, the Account
   Settings UI shall 別の端末や同期先を追加しておくとアカウントを失いにくくなる旨の説明文を
   同一画面上に表示する
2. The Account Settings UI shall ロスト対策の説明文にユーザーの個人情報・credential 識別子等の
   機密情報を含めない

## Non-Functional Requirements

### NFR 1: 既存機能の非破壊性

1. The Account Settings UI shall 既存のアカウント情報表示・退会導線の表示と挙動を、パスキー
   追加登録導線の追加によって変更しない
2. The Web Application shall 既存の 2 ペインレイアウト（フィード一覧・記事一覧・記事詳細）の
   挙動を、本機能追加によって変更しない
3. The Web Application shall 既存の Google OAuth ログイン・パスキー新規作成・パスキーログイン・
   ログアウトの各既存導線の挙動を、本機能追加によって変更しない
4. The Web Application shall 認証済みでないユーザーが体感する挙動を、本機能追加によって変更しない
5. The Web Test Suite shall 本機能追加前から存在する既存テストを green 状態のまま維持する

### NFR 2: セキュリティ・機密情報の非漏出

1. The Web Passkey Add Registration Flow shall 追加登録処理でブラウザから受け取る attestation
   の生バイト列・サーバから受け取るチャレンジ識別子の平文を、ブラウザのコンソールログ・
   localStorage / sessionStorage・URL クエリ文字列・エラー表示テキストに残さない
2. The Web Passkey Add Registration Flow shall サーバの拒否・エラー応答の内部詳細（スタック
   トレース・内部エラー原因・クエリ文字列等）をユーザー可視の画面やコンソールに反射しない
3. The Web Passkey Add Registration Flow shall 追加登録処理を本 spec 導入前から Web が信頼して
   いる同一オリジンのサーバに対してのみ実行する
4. The Web Application shall 既存の Content Security Policy およびクライアント側 HTML sanitize
   方針を本機能追加によって緩和しない

### NFR 3: 実装共有・重複回避

1. The Web Passkey Add Registration Flow shall 既存パスキー登録フローが依拠する WebAuthn の
   encode/decode utility を再利用し、同等機能の重複実装を作らない

### NFR 4: テスト容易性

1. The Web Passkey Add Registration Test Suite shall 追加登録成功・ブラウザキャンセル・重複
   authenticator による拒否・サーバ拒否・ネットワーク断・認証済み状態での導線表示・未認証状態
   での導線非表示 の各ケースを、ブラウザおよび外部ネットワーク依存なしに検証可能にする

## Out of Scope

- パスキー credential のセルフサービス管理 UI（一覧表示・削除・リネーム・命名）
- リカバリコード・リカバリメールなど、パスキー以外のロスト対策手段の追加
- iOS クライアント側の実装（feedman-ios#117 スコープ）
- サーバ側追加登録 API（`POST /api/passkey/registration/add/begin|finish`）の request /
  response 契約の変更（実装前調査で Cookie session 認証にて到達可能と確認済みのため本 spec は
  Web 配線のみに集中する）
- Web ログイン画面（`/login`）自体の変更（本 spec はログイン後の認証済み画面のみを対象とする）
- パスキー登録済み件数・登録日時など credential 一覧的な情報の表示
- 既存 `usePasskeyRegistration` の新規作成用エラー分類（`PasskeyRegistrationErrorKind`）を書き
  換えることによる新規作成フローの挙動変更（本 spec は追加登録専用のエラー区別のみを扱う）

## Open Questions

- **追加登録導線の UI 配置**: Account Settings UI 内の独立セクションとして提示するか、既存
  セクション（アカウント情報 / 退会）の周辺に配置するかの UI 判断は design で確定する
- **成功時のフィードバック手段**: Requirement 2.4 の「成功をユーザーが視認できる形」の具体的
  表示手段（トースト / ダイアログ内インライン表示 / セクション内の状態表示等）は design で
  確定する
- **重複登録時のエラー表示手段と文言**: Requirement 3.1 / 3.4 の「重複と分かる」表示の具体的
  な文言・強調度・提示位置は design / UI 判断とする
- **キャンセル・汎用エラー時のフィードバック手段**: Requirement 4.1〜4.3 の再試行導線・エラー
  表示の具体的な UI（トースト / インラインエラー / モーダル内表示等）は design で確定する
- **ロスト対策説明文の具体コピー・強調度**: Requirement 6.1 の「アカウントを失いにくくなる旨」
  の具体的な文言と表示位置は design / UI 判断とする
- **重複エラーを Web からどのように識別するか**: excludeCredentials 由来のブラウザ拒否を
  他のキャンセル系エラーと区別するための実装手段（DOMException の name 判別 / エラー種別
  enum の拡張等）は design の実装詳細
- **追加登録用チャレンジ取得時のリクエスト内容**: サーバ側 begin endpoint が受け付ける
  リクエストボディの詳細（空ボディでの起動可否等）はサーバ既存契約に従い design で確定する

## 関連

- Parent: #216
- Depends on: #216
- Related: #223 #236 #241
