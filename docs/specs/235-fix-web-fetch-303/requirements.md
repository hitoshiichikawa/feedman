# Requirements Document

## Introduction

Feedman Web（Next.js）のログアウトボタンが実効的に機能しない不具合を修正する。現状、
ボタンをクリックするとサーバ側はセッションを破棄しているにもかかわらず、Web 画面は認証済み
のまま留まり、ユーザーが同じボタンを繰り返し連打する事象が発生している（staging 実機で
2026-07-28 に確認、`POST /auth/logout` 303 応答が 11 連続でサーバに到達）。

背景は「ブラウザ fetch API 経由で `POST /auth/logout` を呼び、サーバが form POST 前提の
303 See Other + Location を返した結果、fetch が自動的にリダイレクトを追従してログイン画面の
HTML を取得し、JSON 解析に失敗してクライアントが失敗扱いとなる」構造で、初期実装から潜在
していたバグである。加えて、ログアウトボタン側の遷移先ハードコード `/login` は Feedman の
Web ルーティング（`/` 上で AuthGuard が未認証時にログイン画面を出す構成）に存在しないため、
仮にクライアント側の後続処理が走っても 404 に落ちる。

本 spec は Google / パスキーいずれの認証セッションに対しても、ユーザーがログアウトボタンを
1 クリックしただけでセッションが破棄されログイン画面に到達し、クライアント側キャッシュも
クリアされるという **ユーザー観察可能な挙動** を要件化する。実装方針（サーバ側の応答方式、
クライアント側の fetch オプション、遷移先ルート等）は本 spec では規定せず、実装フェーズに
委ねる。ただし既存のセッション Cookie 属性（`HttpOnly` / `SameSite=Lax` / `Secure`）と
CSRF 前提、および既存の form POST 経由ログアウト互換性の破壊は本 spec のスコープ外とする。

## Requirements

### Requirement 1: ワンクリックでのログアウト完了

**Objective:** As a Feedman Web の認証済みユーザー, I want ログアウトボタンを 1 回クリック
すれば認証が終了し、未認証のログイン画面が表示されること, so that セッション破棄が完了して
いないと誤認して連打する必要が無くなる

#### Acceptance Criteria

1. When 認証済みユーザーがログアウトボタンを 1 回クリックしたとき, the Web Logout Flow shall
   同一クリックの処理シーケンス内でサーバセッションの破棄要求を発行し、サーバ側でセッションが
   破棄される
2. When サーバ側でセッションが破棄されたとき, the Web Logout Flow shall 追加のユーザー操作を
   要求せず、未認証状態のログイン画面を Web 画面に表示する
3. When ログアウトフローが完了したとき, the Web App shall 直前まで認証済みだったブラウザから
   保護されたビュー（フィード一覧・アイテム一覧等）を再度取得できない状態にする
4. While ログアウトフローが進行中であるとき, the Logout Button shall 追加のログアウト要求を
   受け付けず、多重クリックによるサーバへの重複要求送信を発生させない

### Requirement 2: 認証方式に依存しない共通挙動

**Objective:** As a Feedman Web の全ユーザー（Google OAuth / パスキーいずれの経路でログインした
ユーザー）, I want どちらの認証方式でログインしていても同じログアウトボタンで同じ結果に到達
したい, so that 認証方式による挙動差でユーザーが混乱しない

#### Acceptance Criteria

1. When Google OAuth 経由でログインしたユーザーがログアウトボタンをクリックしたとき, the Web
   Logout Flow shall Requirement 1.1〜1.3 と同じ結果（サーバセッション破棄 + ログイン画面表示）に
   到達する
2. When パスキー認証経由でログインしたユーザーがログアウトボタンをクリックしたとき, the Web
   Logout Flow shall Requirement 1.1〜1.3 と同じ結果（サーバセッション破棄 + ログイン画面表示）に
   到達する
3. The Web Logout Flow shall 認証方式（Google / パスキー）を判別してからログアウト処理を分岐
   させない

### Requirement 3: クライアント側キャッシュのクリア

**Objective:** As a Feedman Web の運用者およびユーザー, I want ログアウト後に前ユーザーの
データがクライアント側キャッシュに残存しないこと, so that 同一ブラウザで別ユーザーがログイン
した際・またはログイン画面に戻った際に前ユーザーのフィード内容やユーザー情報が漏出しない

#### Acceptance Criteria

1. When Requirement 1.2 のログイン画面表示に到達したとき, the Web App shall 認証済みだった
   ユーザーのフィード一覧・アイテム一覧・ユーザー情報などクライアント側の取得結果キャッシュを
   クリアする
2. When 同一ブラウザで別ユーザーが後続でログインしたとき, the Web App shall 前ユーザーの
   キャッシュされたデータを画面に表示しない

### Requirement 4: 遷移先の実在性

**Objective:** As a Feedman Web のユーザー, I want ログアウト後に 404 ページや白画面ではなく
Feedman の正規ログイン画面に到達したい, so that ログアウト後の再ログイン導線を迷わず利用
できる

#### Acceptance Criteria

1. When ログアウトフローが遷移先の URL を決定するとき, the Web Logout Flow shall Feedman Web の
   ルーティング上で実在するパスのみを遷移先として指定する
2. When ログアウトフローが完了したとき, the Web App shall ログイン画面（未認証訪問者向けに
   Google ログイン導線およびパスキー導線が表示される画面）を Web 画面に提示する
3. If ログアウトフローの遷移先候補が Web ルーティング上に存在しないパスであるとき, the Web
   Logout Flow shall そのパスへの遷移を発生させない

### Requirement 5: 失敗時のユーザーフィードバック

**Objective:** As a Feedman Web のユーザー, I want ネットワーク断絶やサーバエラーで
ログアウトが完了しなかった場合にそれをユーザーが認識できるようにしたい, so that ボタンが
「反応していないだけなのか」「サーバが受け付けていないのか」を切り分けられる

#### Acceptance Criteria

1. If ログアウト要求がネットワーク到達失敗（サーバに要求が到達しなかった場合）となったとき,
   the Web Logout Flow shall ユーザーに失敗した旨を認識可能な表示を提示し、認証済み状態のまま
   ログアウトボタンを再度操作可能にする
2. If ログアウト要求に対してサーバがエラー応答（5xx 相当）を返したとき, the Web Logout Flow
   shall ユーザーに失敗した旨を認識可能な表示を提示する
3. If ログアウト要求時に既にセッションが期限切れ・未存在であることをサーバが示したとき,
   the Web Logout Flow shall Requirement 1.2 と同じ結果（ログイン画面表示）に到達する
4. If ログアウトフローがエラー表示を提示するとき, the Web Logout Flow shall サーバ応答の
   内部詳細（スタックトレース・内部エラー原因）をユーザー可視のテキストに反射しない

### Requirement 6: 既存挙動・互換性の維持

**Objective:** As a Feedman Web / API の運用者, I want 本修正で既存の認証・セッション基盤の
セキュリティ前提および form POST 経由ログアウト互換性が壊れないこと, so that 修正がリグレッ
ションや別経路のログアウト機能停止を引き起こさない

#### Acceptance Criteria

1. The Web Logout Flow shall セッション Cookie の属性（`HttpOnly` / `SameSite=Lax` / `Secure`
   相当）および CSRF 前提を本修正の前後で同一に維持する
2. Where 従来 form POST 経由でログアウトを利用している経路が存在する場合, the Logout Endpoint
   shall その経路のログアウト完了挙動を本修正の前後で同一に維持する
3. The Web App shall 本修正の対象外の機能（フィード購読・アイテム閲覧・スター・検索・
   はてブ連携・アカウント設定等）の既存挙動を本修正によって変化させない
4. The Web Login Screen shall 本修正の前後で表示要素・導線（Google / パスキー）の一覧に
   変化を生じさせない

## Non-Functional Requirements

### NFR 1: 機密情報の非漏出

1. The Web Logout Flow shall セッション識別子・Cookie 値・ユーザー識別情報を、ブラウザの
   コンソールログ・エラー表示テキスト・URL クエリ文字列に残さない
2. The Logout Endpoint shall ログアウト応答のレスポンスボディにセッション識別子およびユーザー
   個人情報（メールアドレス等）を含めない

### NFR 2: テスト容易性

1. The Web Logout Test Suite shall 正常系（1 クリックでログイン画面到達 + クライアント側
   キャッシュクリア）を、ブラウザおよびサーバ実物への到達なしに検証可能にする
2. The Web Logout Test Suite shall 異常系（サーバエラー応答時のユーザーフィードバック提示）を、
   ブラウザおよびサーバ実物への到達なしに検証可能にする
3. The Web Logout Test Suite shall ログアウトフロー完了後にクライアント側キャッシュがクリア
   されていることを、キャッシュ状態を観測する形で検証可能にする

## Out of Scope

- パスキー再認証失敗（Issue #234）の修正
- アカウント設定導線・アカウント削除フローの追加・変更
- セッション管理方式そのものの変更（Cookie セッション → Bearer token 化等）
- Cookie 属性（`HttpOnly` / `SameSite` / `Secure`）および CSRF 前提の変更
- Google OAuth / パスキー登録・認証フロー本体（ログイン側）の挙動変更
- ログアウト後の別ユーザー切替 UI（アカウント切替器）の追加
- ログアウト完了トースト・確認モーダル等の追加 UX 拡張（本修正はワンクリック完了までを対象と
  し、UI 上の演出は含まない）

## Open Questions

- なし（Issue 本文と現状コード（`web/src/hooks/use-auth.ts` / `web/src/components/logout-button.tsx` /
  `web/src/lib/api.ts` / `internal/handler/auth_handler.go` の `Logout` ハンドラ）から要件は
  一意に特定できる。実装方針（サーバ応答を 204 化する / クライアント側で `redirect: "manual"`
  にする / 遷移先を `/` に変える 等）は本 spec では規定せず、Developer が Requirement 1〜6 の
  AC を満たす形で判断する）

## 関連

- Related: #223 #234
