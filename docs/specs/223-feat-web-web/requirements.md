# Requirements Document

## Introduction

Feedman Web（Next.js）のログイン画面は現在 Google OAuth のみに対応しており、Issue #216 で
サーバに追加されたパスキー自社認証（ユーザー名 + WebAuthn）を利用する導線が存在しない。
このため、Google アカウントを持たない・使いたくないユーザーや、iOS #117 で作成した
パスキー由来アカウントのユーザーが Web から Feedman にアクセスできない状態が続いている。

本 spec は Web ログイン画面に「パスキーでログイン」および「パスキーでアカウント新規作成」の
2 導線を追加し、いずれの導線でも完了直後に Web の既存 Cookie セッションベース認証状態に到達し
2 ペイン UI が使えるようにする、というユーザー・運用者から観察可能な挙動を要件化する。
新規作成はユーザー名のみで開始でき（recovery email は初回フローに含めない）、ログインは
ブラウザのパスキー選択 UI で完結する。既存 Google OAuth のログイン挙動・
セッション寿命・既存機能は本 spec 導入後も一切変化させない。

Web からの既存 Google アカウントへのパスキー追加登録 UI、パスキー credential の管理 UI
（一覧・削除・リネーム）、アカウント統合、および #216 で導入済みのサーバ側パスキー API 契約の
変更は本 spec のスコープ外とする（合流方式次第で必要になる最小限のサーバ側追加を除く）。

## Requirements

### Requirement 1: Web ログイン画面のパスキー導線表示

**Objective:** As a Feedman Web の未認証訪問者, I want ログイン画面で Google に加えて
パスキーでログイン・パスキーで新規作成の選択肢を提示されたい, so that Google アカウント無しでも
Feedman の利用を開始・再開できる

#### Acceptance Criteria

1. When 未認証状態でログイン画面が表示されるとき, the Web Login Screen shall Google ログイン
   導線・パスキーログイン導線・パスキーでのアカウント新規作成導線の 3 種を同一画面上に提示する
2. When ログイン画面が表示されるとき, the Web Login Screen shall パスキー機能が本 spec 導入前に
   存在しなかった状態と識別可能な形（Google 以外の選択肢の追加）で提示する
3. When ユーザーがパスキーログイン導線を選択したとき, the Web Login Screen shall パスキー
   ログインフロー（Requirement 4）を開始する
4. When ユーザーがアカウント新規作成導線を選択したとき, the Web Login Screen shall アカウント
   新規作成フロー（Requirement 2）を開始する
5. When ユーザーが Google ログイン導線を選択したとき, the Web Login Screen shall 本 spec 導入前と
   同一の Google OAuth ログインフローを開始する

### Requirement 2: パスキーによるアカウント新規作成フロー

**Objective:** As a Feedman Web の未認証訪問者, I want ユーザー名だけを指定してブラウザの
パスキー作成 UI からアカウントを作成したい, so that Google アカウントを介さず Feedman の利用を
開始できる

#### Acceptance Criteria

1. When ユーザーがアカウント新規作成導線を選択したとき, the Web Login Screen shall ユーザー名
   入力欄と作成開始操作を提示する
2. When ユーザーがユーザー名を入力し作成開始を要求したとき, the Web Passkey Registration Flow
   shall ブラウザのパスキー作成 UI を起動する
3. When ブラウザのパスキー作成 UI が成功で完了したとき, the Web Passkey Registration Flow shall
   ユーザーに追加操作を要求せず、Requirement 3 の登録完了後合流フローに遷移する
4. If ユーザーが recovery email など任意項目を入力せずに作成開始を要求したとき, the Web Passkey
   Registration Flow shall recovery email 未指定を欠落として扱わず、ユーザー名のみで作成開始
   要求を進行させる
5. If ユーザーが入力したユーザー名がサーバの受け入れ形式（許容文字種・許容長さ）を満たさない
   ことがサーバ応答で判明したとき, the Web Passkey Registration Flow shall アカウント作成を確定
   させず、ユーザー名形式不正である旨の表示を同一画面に提示する
6. If ユーザーが入力したユーザー名が既に別ユーザーで使用されていることがサーバ応答で判明した
   とき, the Web Passkey Registration Flow shall アカウント作成を確定させず、ユーザー名重複で
   ある旨の表示を同一画面に提示する
7. If ユーザーがブラウザのパスキー作成 UI 上でキャンセルまたは拒否したとき, the Web Passkey
   Registration Flow shall アカウント作成を確定させず、画面表示を壊さずに作成開始前の状態へ
   復帰させる
8. If アカウント作成中にサーバがエラー応答（内部エラー・レート制限超過等）を返したとき, the Web
   Passkey Registration Flow shall アカウント作成を確定させず、汎用エラー表示を同一画面に提示し、
   サーバ応答の内部詳細（スタックトレース・エラー原因のクエリ文字列等）をユーザー画面に反射しない

### Requirement 3: 新規作成完了後のセッション合流と 2 ペイン UI 到達

**Objective:** As a Feedman Web でパスキー新規作成を完了したユーザー, I want 登録完了直後に
追加のログイン操作なしで 2 ペイン UI に到達したい, so that 新規作成体験が Google OAuth 新規と
同等にシームレスになる

#### Acceptance Criteria

1. When Requirement 2 で新規作成が成功したとき, the Web Passkey Registration Flow shall ユーザーに
   別途ログインボタンを再度押させることなく、既存の Cookie セッションベース認証状態に到達する
2. When 新規作成後にセッションベース認証状態に到達したとき, the Web App shall Google OAuth 経由で
   ログインしたときと同一の 2 ペイン UI（フィード一覧 / アイテム一覧）を初期表示する
3. When 新規作成後にセッションベース認証状態に到達したとき, the Web App shall Google OAuth 経由で
   ログインしたときと同一の既存機能一式（フィード登録・スター・検索・ログアウト・退会）を利用
   可能にする
4. If セッションベース認証状態への到達がサーバ側の合流失敗で成立しなかったとき, the Web Passkey
   Registration Flow shall 2 ペイン UI に遷移させず、汎用エラー表示を提示してログイン画面に復帰
   させる

### Requirement 4: パスキーによるログインフロー

**Objective:** As a Feedman Web にパスキー登録済みのユーザー（Web で新規作成・iOS で作成のいずれも
含む）, I want ブラウザのパスキー選択 UI からログインを完了したい, so that Google に依存せず
Web から Feedman を再開できる

#### Acceptance Criteria

1. When ユーザーがパスキーログイン導線を選択したとき, the Web Passkey Authentication Flow shall
   ブラウザのパスキー選択 UI を起動し、ユーザーにユーザー名の入力を要求しない
2. When ブラウザのパスキー選択 UI が成功で完了したとき, the Web Passkey Authentication Flow shall
   ユーザーに追加操作を要求せず、既存の Cookie セッションベース認証状態に到達する
3. When 認証成功で Cookie セッションベース認証状態に到達したとき, the Web App shall Google OAuth
   経由でログインしたときと同一の 2 ペイン UI と既存機能一式を利用可能にする
4. When Web のユーザーが iOS #216 で作成したパスキー由来アカウントのパスキーを使ってログイン
   したとき, the Web Passkey Authentication Flow shall そのアカウントとして Requirement 4.2 の
   認証状態に到達する
5. If ユーザーがブラウザのパスキー選択 UI 上でキャンセルまたは拒否したとき, the Web Passkey
   Authentication Flow shall 認証状態に到達させず、画面表示を壊さずにログイン画面へ復帰させる
6. If サーバがチャレンジ期限切れ・credential 未解決・assertion 不正のいずれかで拒否応答を
   返したとき, the Web Passkey Authentication Flow shall 認証状態に到達させず、拒否理由の内部
   区別をユーザー画面に反射しない汎用エラー表示を提示する
7. If ログイン中にサーバがサーバエラー応答（内部エラー・レート制限超過等）を返したとき, the Web
   Passkey Authentication Flow shall 認証状態に到達させず、汎用エラー表示を同一画面に提示する

### Requirement 5: ブラウザ・サーバのケーパビリティに応じた導線縮退

**Objective:** As a Feedman Web 訪問者（パスキー非対応ブラウザ利用者およびパスキー機能未提供
デプロイ環境の利用者）, I want パスキーが利用できない環境で壊れたパスキー導線を提示されずに
Google のみで利用開始できたい, so that パスキーが動かない環境でも Feedman を通常どおり利用できる

#### Acceptance Criteria

1. When 訪問者のブラウザがパスキー登録・認証操作の実行機能を提供していないとき, the Web Login
   Screen shall パスキーログイン導線・アカウント新規作成導線を非表示または操作不能な状態として
   提示する
2. When サーバがパスキー機能を提供していない構成で稼働しているとき, the Web Login Screen shall
   パスキーログイン導線・アカウント新規作成導線を非表示または操作不能な状態として提示する
3. While パスキー導線が上記いずれかの理由で無効化されているとき, the Web Login Screen shall
   Google ログイン導線を本 spec 導入前と同一に利用可能な状態で提示する
4. If パスキー導線が無効化された環境で、ユーザーがパスキー機能を要求する何らかの操作を試みた
   とき, the Web Login Screen shall パスキー処理を開始せずに Google 経由でのログインを案内する
   表示に留まる

### Requirement 6: 既存 Google OAuth ログイン挙動の後方互換

**Objective:** As a Feedman Web の既存 Google ユーザー・運用者, I want 本 spec 追加によって
既存の Google OAuth ログイン挙動が変化しないでほしい, so that 既存ユーザーが再ログイン・
設定変更を強いられずに従来どおり利用できる

#### Acceptance Criteria

1. The Web Login Screen shall Google ログイン導線の表示位置・遷移先・ログイン後の Cookie
   セッション挙動を本 spec 導入前と同一に維持する
2. The Web App shall Google OAuth 経由でログイン済みのユーザーに対して、本 spec 導入前と同一の
   2 ペイン UI と既存機能一式を提示する
3. The Web App shall 本 spec 導入前から存在する認証・UI・機能のいずれかを、パスキー導線の追加を
   理由に破壊的に変更しない
4. The Web Test Suite shall 本 spec 追加前から存在する既存テストを、パスキー機能非提供環境の
   条件下でパスキー機能追加前と同一の green 状態のまま実行できる

## Non-Functional Requirements

### NFR 1: セキュリティと機密情報の非漏出

1. The Web Passkey Registration Flow and Web Passkey Authentication Flow shall パスキー登録・
   認証処理でブラウザから受け取る attestation / assertion の生バイト列・サーバから受け取る
   チャレンジ識別子および auth_code の平文を、ブラウザのコンソールログ・localStorage /
   sessionStorage・URL クエリ文字列・エラー表示テキストに残さない
2. The Web Passkey Registration Flow and Web Passkey Authentication Flow shall 拒否・エラー時に
   サーバの内部詳細（スタックトレース・内部エラー原因・SQL / DB 名等）をユーザー可視の画面や
   コンソールに反射しない
3. The Web App shall 既存の Content Security Policy およびクライアント側 HTML sanitize 方針を
   パスキー導線導入によって緩和しない
4. The Web Passkey Registration Flow and Web Passkey Authentication Flow shall パスキー登録・
   認証処理を本 spec 導入前から Web が信頼している同一オリジンのサーバに対してのみ実行する

### NFR 2: 後方互換性

1. The Web App shall パスキー機能を提供しないデプロイ環境（サーバがパスキー機能を公開していない
   構成）において、Google OAuth ログイン・既存 UI・既存機能・既存テストのいずれをも本 spec
   導入前と同一に動作させる
2. The Web App shall 既存ユーザーに対して、本 spec 導入後の初回アクセスで再ログイン・再認証・
   設定変更を要求しない

### NFR 3: テスト容易性

1. The Web Passkey Registration Test Suite and Web Passkey Authentication Test Suite shall 正常系
   （新規作成成功・ログイン成功）・異常系（ユーザー名形式不正・ユーザー名重複・パスキー登録
   キャンセル・パスキー認証キャンセル・チャレンジ期限切れ・サーバエラー・パスキー機能非対応環境）の
   各ケースを、ブラウザおよび外部ネットワーク依存なしに検証可能にする

## Out of Scope

- Web からの既存 Google アカウントへのパスキー追加登録 UI（Google 由来ユーザーが Web から自身の
  アカウントへパスキーを追加する導線）
- パスキー credential のセルフサービス管理 UI（一覧表示・削除・リネーム・命名）
- パスキー由来アカウントへの Google 後付けリンク（アカウント統合・identity マージ）
- パスワード認証・パスワードリセットフロー・メール送信基盤の追加
- Sign in with Apple の Web 導入
- Web のログイン画面デザイン全面刷新（本 spec は既存レイアウトへのパスキー導線追加に留める）
- パスキー credential の期限管理・再登録リマインダ UI
- 既に main に投入済みのサーバ側パスキー API（Issue #216）の request / response 契約の破壊的変更
  （合流に必要な最小限のサーバ追加は許容する）
- iOS クライアント側の実装・iOS 側 UI の変更（#216 / #117 の別 Issue で扱う）
- Android クライアント連携（AASA 相当のドメイン所有権表明 `assetlinks.json` を含む）

## Open Questions

- **auth_code → Web セッション合流方式**: パスキー認証 finish で得られる `auth_code` を Web の
  既存 Cookie セッション状態に到達させる具体的な合流方式（既存 Cookie session を発行する新規
  経路の追加・auth_code を Bearer token に交換して Web で保持する方式・その他）は `design.md` の
  領分として委ねる。本 spec は「登録・ログイン完了後は既存 Cookie セッションベース認証状態に到達し、
  既存機能一式が使える」までを user-observable に要件化している
- **PKCE `code_verifier` の生成・保管方式**: サーバ側の begin 経路が `code_challenge` を必須と
  しているため、Web からも PKCE の code_verifier / code_challenge をクライアント側で用意する
  必要がある。code_verifier をブラウザセッション内でどのように保管するか（in-memory /
  sessionStorage のいずれか）、および NFR 1.1 との整合は design で確定する
- **RP ID / origins の環境設定**: WebAuthn Relying Party ID と許容 origin の env / 設定方式
  （staging / 本番 / 開発の domain 割り当てと Next.js `rewrites` との整合）は運用制約として
  言及可能だが、具体値は design 判断とする
- **パスキー非対応ブラウザの判定手段**: Requirement 5.1 で「ブラウザがパスキー登録・認証操作の
  実行機能を提供していない」ことを検出する具体的手段（`window.PublicKeyCredential` 存在判定 /
  `isConditionalMediationAvailable` 等）は design の実装詳細
- **サーバがパスキー機能を提供していないことの検出手段**: Requirement 5.2 で「サーバがパスキー
  機能を提供していない」ことを Web が検出する手段（起動時 capability endpoint の追加 vs
  ログイン導線の実試行での 404 判定 vs env 経由の build-time / runtime フラグ）は design で
  確定する
- **アカウント新規作成 UI の同居／分離**: ログイン画面上に username 入力欄をインライン展開するか、
  別モーダル・別ページに分離するかの UI 判断は design の領分
- **ユーザー名の入力バリデーション文言と表示位置**: Requirement 2.5 / 2.6 で提示する「形式不正」
  「重複」の表示文言・強調度・入力欄との位置関係は design / UI 判断とする
- **登録・認証処理の起動失敗・timeout の扱い**: WebAuthn 実行中のブラウザ側 timeout・
  authenticator 未接続などをどこまで固有エラーとして扱うか（すべて Requirement 2.7 / 4.5 の
  「キャンセル相当」に集約するか、細分化するか）は design 判断とする

## 関連

- Parent: #216
- Depends on: #216
- Related: #117
