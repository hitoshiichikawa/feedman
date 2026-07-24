# Requirements Document

## Introduction

Feedman iOS の認証を App Store Review Guideline 4.8 に適合させるため、Google Sign-In のみで
構成される現状に対する代替として、パスキー（WebAuthn）とユーザー名（メール登録は任意）による
自社認証を **サーバ側**に追加する。ゴールは iOS クライアントがパスキー登録・認証を完了した後、
既存 native token auth の token 交換契約（親 Issue #163 / 子 Issue #164〜#172 で確定済み）に
合流して以降の Bearer 認証 API を利用できる状態を提供することにある。

本 spec は (1) 新規ユーザーがユーザー名とパスキーだけでアカウントを作成できる導線、(2) 既存
アカウント（Google 由来を含む）にパスキーを追加登録できる導線、(3) パスキー認証成功時に既存
native auth の token 交換契約と同一形式で auth_code を発行する導線、(4) iOS プラットフォーム
パスキー API と連動するためのドメイン所有権表明ファイル配信、(5) 未認証パスキー endpoint への
IP レート制限、(6) 退会時のパスキー credential cleanup、をユーザー・運用者から観察可能な形で
要件化する。iOS クライアント側の実装・Sign in with Apple・パスワード認証・メール送信基盤・
Web フロントのパスキー UI は本 spec のスコープ外とする。

## Requirements

### Requirement 1: 新規ユーザーのパスキー登録によるアカウント作成

**Objective:** As a Feedman iOS ユーザー, I want ユーザー名（必要ならリカバリ用メールも）を指定
してパスキーを登録するだけでアカウントを作成したい, so that Google Sign-In を経由せずに Feedman
の利用を開始できる

#### Acceptance Criteria

1. When 未認証クライアントがユーザー名（および任意でリカバリ用メール）を提示してパスキー登録
   開始を要求したとき, the Passkey Registration Service shall プラットフォームパスキー API が
   利用可能な形式の登録用チャレンジ・パラメータを返す
2. When クライアントがパスキー登録開始で受け取ったチャレンジに対して有効なパスキー登録応答を
   提示したとき, the Passkey Registration Service shall 提示された公開鍵・credential 識別子等の
   検証用情報を当該ユーザーに紐付けて保存し、Google 紐付けを伴わないアカウントを新規作成する
3. When 新規アカウントが作成されたとき, the Passkey Registration Service shall 直後のログイン
   に使えるようにクライアントへ登録成功を通知し、以降のパスキー認証で当該ユーザーとして解決
   可能な状態にする
4. If 登録要求のユーザー名が既存ユーザーのユーザー名と重複するとき, the Passkey Registration
   Service shall アカウントを作成せず、ユーザー名重複であることをクライアントが判別可能な形で
   拒否する
5. If 登録要求のユーザー名が要件を満たさない形式（空・許容外文字・許容外長さ等）であるとき,
   the Passkey Registration Service shall アカウントを作成せず、入力形式不正として要求を拒否する
6. If 登録要求にリカバリ用メールが含まれないとき, the Passkey Registration Service shall メール
   未指定を欠落として扱わず、リカバリ用メールなしのアカウントとして作成を許容する
7. If 登録要求のパスキー応答が検証に失敗した（不正な attestation / 期限切れチャレンジ / 既知の
   credential 重複等）とき, the Passkey Registration Service shall アカウントを作成せず、内部
   詳細を反射しない安全なエラーで要求を拒否する

### Requirement 2: パスキー認証による auth_code 発行と既存 token 交換への合流

**Objective:** As a Feedman iOS ユーザー, I want 登録済みパスキーで Feedman にログインしたい,
so that ログイン成功を既存の Bearer 認証 API 呼び出しにそのまま接続できる

#### Acceptance Criteria

1. When 未認証クライアントがパスキー認証開始を要求したとき, the Passkey Authentication Service
   shall プラットフォームパスキー API が利用可能な形式の認証用チャレンジ・パラメータを返す
2. When クライアントがパスキー認証開始で受け取ったチャレンジに対して有効なパスキー認証応答を
   提示したとき, the Passkey Authentication Service shall 応答を検証し、当該 credential に紐付く
   ユーザーを解決したうえで、既存 native auth の token 交換契約と同一形式の一時 auth_code を
   発行する
3. When パスキー認証成功で auth_code を発行するとき, the Passkey Authentication Service shall
   auth_code を発行時刻から 60 秒で失効する有効期限・単回利用可能な状態・解決済みユーザー
   識別子に紐付けて保存する
4. When パスキー認証成功で発行された auth_code が既存 native auth の token 交換エンドポイントに
   提示されたとき, the Token Exchange Endpoint shall 当該 auth_code を既存 native auth と同じ
   単回消費規則で受理し、既存契約と同形式の access token・refresh token・token_type・expires_in
   を返す
5. If パスキー認証応答が検証に失敗した（不正な assertion / 期限切れチャレンジ / 未知の credential /
   カウンタ値の後退等）とき, the Passkey Authentication Service shall auth_code を発行せず、
   内部詳細を反射しない安全なエラーで要求を拒否する
6. The Passkey Authentication Service shall 認証拒否応答において、ユーザーの存在有無・credential
   の存在有無・チャレンジ失効・応答不正のいずれで拒否したかを区別できる情報を返さない

### Requirement 3: 認証済みユーザーによる既存アカウントへのパスキー追加登録

**Objective:** As a Feedman 認証済みユーザー（Google 由来を含む）, I want 自分のアカウントに
新しいパスキーを追加登録したい, so that Google Sign-In だけでなくパスキーでもログインできる
状態に移行できる

#### Acceptance Criteria

1. When 認証済みクライアントが自身のアカウントへのパスキー追加登録開始を要求したとき, the
   Passkey Registration Service shall 現在のユーザー識別子に紐付いた追加登録用チャレンジ・
   パラメータを返す
2. When 認証済みクライアントが追加登録開始で受け取ったチャレンジに対して有効なパスキー登録
   応答を提示したとき, the Passkey Registration Service shall 提示された公開鍵・credential
   識別子等の検証用情報を **現在ログイン中のユーザー** に紐付けて保存する
3. When 認証済みユーザーが Google 紐付けのみを持つアカウントにパスキーを追加登録したとき, the
   Passkey Registration Service shall 既存の Google 紐付けを解除・変更せず、当該アカウントを
   両方のログイン手段で解決可能な状態にする
4. When 認証済みユーザーが同一アカウントに複数のパスキーを追加登録したとき, the Passkey
   Registration Service shall いずれのパスキーからでも当該ユーザーとしてログイン可能な状態に
   する
5. If 未認証クライアントが追加登録エンドポイントを呼び出したとき, the Passkey Registration
   Service shall 追加登録を試行せず未認証として要求を拒否する
6. If 追加登録要求のパスキー応答が別ユーザーに既に登録済みの credential 識別子を提示したとき,
   the Passkey Registration Service shall 追加登録を確定させず、内部詳細を反射しない安全な
   エラーで要求を拒否する
7. If 追加登録要求のパスキー応答が検証に失敗した（不正な attestation / 期限切れチャレンジ 等）
   とき, the Passkey Registration Service shall 追加登録を確定させず、内部詳細を反射しない
   安全なエラーで要求を拒否する

### Requirement 4: パスキーチャレンジのライフサイクル管理

**Objective:** As a Feedman 運用者, I want パスキー登録・認証チャレンジが有効期限付きで単回
利用に限定されてほしい, so that チャレンジ再利用や中間者による使い回しでの攻撃を防げる

#### Acceptance Criteria

1. When パスキー登録開始または追加登録開始でチャレンジが発行されたとき, the Passkey Challenge
   Store shall 当該チャレンジを発行時刻からの有効期限と単回利用可能な状態で保存する
2. When パスキー認証開始でチャレンジが発行されたとき, the Passkey Challenge Store shall 当該
   チャレンジを発行時刻からの有効期限と単回利用可能な状態で保存する
3. When 発行済みチャレンジが登録応答または認証応答の検証で成功裏に消費されたとき, the Passkey
   Challenge Store shall 当該チャレンジを以降の応答検証で再利用不能な状態にする
4. If 既に消費済みまたは有効期限を超過したチャレンジが登録応答または認証応答として提示された
   とき, the Passkey Registration Service and Passkey Authentication Service shall 応答検証を
   成立させず、内部詳細を反射しない安全なエラーで要求を拒否する
5. The Passkey Challenge Store shall チャレンジ発行に暗号論的乱数源から 128bit 以上のエントロピー
   を用いる

### Requirement 5: iOS プラットフォーム連携のためのドメイン所有権表明

**Objective:** As a Feedman iOS クライアント, I want サーバがプラットフォームパスキー API が
参照するドメイン所有権表明を配信してほしい, so that iOS のパスキー機構が Feedman ドメインに
対する credential を扱えるようになる

#### Acceptance Criteria

1. When 未認証クライアントが標準のドメイン所有権表明パス `/.well-known/apple-app-site-association`
   を GET したとき, the Domain Association Endpoint shall iOS プラットフォームパスキー API 用途
   （webcredentials）を含む JSON を HTTP 200 で応答する
2. When ドメイン所有権表明が応答されるとき, the Domain Association Endpoint shall Apple プラット
   フォームが要求する Content-Type ヘッダおよび `Cache-Control` ヘッダを付与して返す
3. The Domain Association Endpoint shall 応答内容にユーザー個別のトークン・秘密情報・環境依存の
   ホスト名を含めない
4. The Domain Association Endpoint shall 認証ミドルウェア・IP レート制限による拒否の対象外と
   なる公開エンドポイントとして到達可能である

### Requirement 6: パスキー endpoint への未認証 IP レート制限

**Objective:** As a Feedman 運用者, I want 未認証で公開されるパスキー endpoint 群を同一 IP 単位で
レート制限したい, so that パスキー登録・認証エンドポイントへの総当たり・フラッディングから
サービスを保護できる

#### Acceptance Criteria

1. When 同一クライアント IP からパスキー登録開始または登録応答検証への単位時間あたりリクエスト
   数が閾値を超過したとき, the IP レート制限 shall 登録処理を試行せずに HTTP 429 Too Many
   Requests を返す
2. When 同一クライアント IP からパスキー認証開始または認証応答検証への単位時間あたりリクエスト
   数が閾値を超過したとき, the IP レート制限 shall 認証処理を試行せずに HTTP 429 Too Many
   Requests を返す
3. While 同一クライアント IP からのリクエスト数が閾値以内であるとき, the IP レート制限 shall
   パスキー登録・認証の各要求を後続処理へ通常どおり通過させる
4. When 異なるクライアント IP からパスキー endpoint へリクエストが到達したとき, the IP レート
   制限 shall 各 IP のリクエスト数を独立にカウントする
5. When 閾値超過により拒否したとき, the IP レート制限 shall 既存の未認証エンドポイント
   （Google ログイン入口・native auth token / refresh / revoke）の閾値超過応答と同一形式で
   応答する

### Requirement 7: 退会時のパスキー credential cleanup

**Objective:** As a Feedman 運用者, I want 退会したユーザーのパスキー credential が残存しない
こと, so that 削除済みアカウントに紐づく認証情報の残存・悪用リスクを排除できる

#### Acceptance Criteria

1. When ユーザーの退会処理が完了したとき, the User Withdrawal Service shall 当該ユーザーに
   紐付くパスキー credential を全て削除する
2. The User Withdrawal Service shall パスキー credential の削除を、既存の退会削除フローと同一の
   処理単位（同一の原子性境界）内で実行する
3. If 退会処理の途中でパスキー credential の削除が失敗したとき, the User Withdrawal Service
   shall 退会処理全体を失敗させ、それまでの削除を確定しない
4. When あるユーザーの退会処理が実行されたとき, the User Withdrawal Service shall 他ユーザーの
   パスキー credential を削除も無効化もしない
5. When ユーザーの退会処理が完了したとき, the Passkey Authentication Service shall 削除された
   ユーザーのユーザー名を新規登録で再取得可能な状態にする（ユーザー名の使い回しをブロック
   する追加ルールは本 spec では要件化しない）

### Requirement 8: 既存認証フローの後方互換

**Objective:** As a Feedman Web ユーザー・iOS ユーザー・運用者, I want 本機能の追加によって
既存の Google OAuth（Web Cookie / native flow）挙動が変化しないでほしい, so that 既存ユーザーが
再ログインや設定変更を強いられずに従来通り利用できる

#### Acceptance Criteria

1. The Auth Login Endpoint and Auth Callback Endpoint shall 既存の Google OAuth Web ログイン
   （Cookie セッション発行・Cookie 設定・フロントエンドへのリダイレクト）を本機能導入前と
   同一に維持する
2. The Auth Login Endpoint and Auth Callback Endpoint shall 既存の Google OAuth native flow
   （`flow=native` + PKCE S256 → アプリスキームへの auth_code リダイレクト）を本機能導入前と
   同一に維持する
3. The Token Exchange Endpoint shall Google OAuth 由来の auth_code に対する token 交換応答の
   JSON 形式・エラー拒否契約を本機能導入前と同一に維持する
4. The Refresh Token Endpoint and Revoke Endpoint shall 本機能導入前と同一の要求・応答形式で
   動作する
5. The User Withdrawal API shall 成功・失敗の応答形式を本機能導入前と同一に維持する

## Non-Functional Requirements

### NFR 1: セキュリティ（credential 情報と秘密の取り扱い）

1. The Passkey Registration Service and Passkey Authentication Service shall 保存対象を検証に
   必要な情報（公開鍵・credential 識別子・カウンタ値等）のみに限定し、パスキー本体の秘密情報
   を保存しない
2. The Passkey Registration Service and Passkey Authentication Service shall パスキー生応答
   （attestation / assertion の生バイト列）・チャレンジの平文・auth_code の平文をログ・エラー
   メッセージ・レスポンスに含めない
3. The Passkey Registration Service and Passkey Authentication Service shall エラー応答に
   クライアント入力値・内部詳細（スタックトレース・クエリ文字列等）を反射しない
4. The Passkey Authentication Service shall パスキー認証応答のカウンタ値後退（既保存カウンタ値
   より小さい値の再提示）を認証成功として受理しない

### NFR 2: 後方互換性

1. The Feedman システム shall 本機能をデータ移行・既存テーブル構造の破壊的変更なしに導入する
2. The Feedman システム shall 本機能に関する環境変数が未設定の既存デプロイ環境において、既存の
   Google OAuth（Web Cookie / native flow）を本機能導入前と同一に動作させる
3. The Existing Native Auth Contract Tests shall 本機能導入後も無変更で green を維持する

### NFR 3: 可観測性

1. When パスキー登録または認証で拒否が発生したとき, the Passkey Registration Service and
   Passkey Authentication Service shall 拒否事象を運用ログとして記録する
2. The Passkey Registration Service and Passkey Authentication Service shall 拒否ログにパスキー
   生応答・チャレンジ平文・auth_code 平文・ユーザーのリカバリ用メール本文を含めない

### NFR 4: テスト容易性

1. The Passkey Registration Test Suite and Passkey Authentication Test Suite shall 登録成功・
   認証成功・チャレンジ失効・チャレンジ再利用・不正応答・ユーザー名重複・入力形式不正・
   カウンタ後退・追加登録での credential 重複の各ケースを外部ネットワーク依存なしで検証可能に
   する
2. The Withdrawal Test Suite shall 退会完了後に当該ユーザーのパスキー credential が残存しない
   ことを明示的に検証する

## Out of Scope

- iOS クライアント側の実装（別 Issue で扱う）
- Sign in with Apple の導入（本 Issue の方針として不採用）
- パスワード認証（ユーザー名 + パスワードでのログイン）
- メール送信基盤の追加（パスワードリセット・確認メール・通知メール等）
- リカバリ用メールを使ったアカウント復旧フロー（メール送信基盤に依存するため）
- Web フロントエンドのパスキーログイン UI（Web は当面 Google OAuth を継続）
- パスキー credential の削除・リネーム等のセルフサービス管理 UI（本 spec では登録・認証・退会
  cleanup のみ）
- Android プラットフォーム連携のためのドメイン所有権表明（`assetlinks.json` 等）
- 期限切れチャレンジ・期限切れ auth_code の定期削除ワーカー（有効期限・単回利用判定で機能上
  無効化されるため）
- ユーザー名の使い回し防止・ユーザー名変更フロー
- WebAuthn の user verification 要件の細分化（platform authenticator 必須・biometric 必須等の
  ポリシー化は design で最低限を決めれば十分）

## Open Questions

- **RP ID / origin の環境設定**: WebAuthn の Relying Party 識別子・許容 origin の env / 設定
  方式（単一値 / 環境別配列 / iOS 側 associated domains との紐付け方）は design（Architect）の
  領分として委ねる。本 spec は「iOS プラットフォームパスキー API と連動する」という user-observable
  な結果のみ要件化している
- **ユーザー名の文字種・長さ・正規化ルール**: 「一意性が判定できる」「空・許容外形式は拒否」
  までを本 spec で要件化し、具体的な文字種（英数字のみ / Unicode 許容 / 大文字小文字の正規化 /
  最大長）は design で確定する
- **チャレンジ有効期限の具体値**: Requirement 4 は「有効期限」を持って保存することまでを要件
  化し、具体的な秒数（60 秒 / 120 秒 / 300 秒等）は WebAuthn 実装ライブラリの推奨値と iOS の
  ユーザー体感を踏まえて design で確定する
- **パスキー endpoint への Web フロント公開の要否**: 本 spec は iOS からの利用に必要な公開のみを
  要件化。Web フロント（Next.js）から同 endpoint を利用させるか（CORS・rewrites 経路整備）は
  design 判断とし、本 spec の Out of Scope に「Web フロントの UI」を明記した
- **既存 Google 由来ユーザーのユーザー名初期値**: 既存 Google ユーザーが Requirement 3 の
  追加登録を行う際に、内部的にユーザー名が必要かどうか（Google 由来ユーザーはユーザー名が
  未設定でもよいか、初回追加登録時にユーザー名確定を要求するか）は design で確定する。
  本 spec は「既存 Google 紐付けを解除せずパスキーが追加できる」までを要件化している
- **AASA の cache 戦略**: `Cache-Control` の具体値・CDN でのキャッシュ TTL は運用側の判断
  事項。本 spec は「Apple プラットフォームが要求するヘッダを付与して配信する」までを要件化する
- **IP レート制限の閾値**: 既存 native auth と同一閾値・制限方式を流用する前提だが、パスキー
  登録の初回導線が総当たり耐性上どの程度の頻度で許容されるべきかは design / 運用判断とする

## 関連

- Parent: #163
- Related: #164 #165 #166 #167 #168 #169 #170 #171 #172 #207
