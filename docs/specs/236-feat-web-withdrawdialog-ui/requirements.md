# Requirements Document

## Introduction

Web クライアントにはログイン後ユーザーの「アカウント設定」への導線が現状存在せず、実装済みの退会確認
ダイアログもどこからも呼び出されない状態にある。さらに、その退会ダイアログが送信している削除要求先と、
バックエンドが実際に受け付けている退会エンドポイントの経路に不整合があるため、単に配線するだけでは退会
操作が完了しない状態にある。本要件では、認証済み画面からアカウント情報の確認と退会操作にアクセスできる
最小限の UI を提供し、パスキーのみアカウント（email 空）でも表示・操作が破綻しないことを保証する。
ログアウトの導線集約やパスキー credential 管理などは今回のスコープ外とする。

## Requirements

### Requirement 1: アカウント設定への導線

**Objective:** As a 認証済みユーザー, I want ログイン後の画面からアカウント設定にアクセスできる導線を持ちたい, so that 自分のアカウント情報の確認や退会操作を Web UI 上で自力で開始できる

#### Acceptance Criteria

1. While 認証済みユーザーとしてアプリを閲覧しているとき, the Web Application shall アカウント設定への入口を認証済み画面から視認できる位置に表示する
2. When ユーザーがアカウント設定への入口を操作したとき, the Account Settings UI shall アカウント情報表示領域と退会導線を含む画面を開く
3. While 未認証状態でアプリを閲覧しているとき, the Web Application shall アカウント設定への入口を表示しない
4. The Account Settings UI shall ユーザー操作で明示的に閉じることができる

### Requirement 2: アカウント情報の表示

**Objective:** As a 認証済みユーザー, I want アカウント設定からログイン中の自分のアカウント情報を確認できる, so that 現在ログインしている主体を把握し、意図しないアカウントで退会などの操作をしないことを担保できる

#### Acceptance Criteria

1. While Account Settings UI が開かれているとき, the Account Settings UI shall 現在ログイン中ユーザーの表示名を表示する
2. While Account Settings UI が開かれており email が設定されているとき, the Account Settings UI shall 当該 email アドレスを表示する
3. While Account Settings UI が開かれており email が未設定（空文字）であるとき, the Account Settings UI shall 「未設定」と識別可能なプレースホルダ表示を行い、空欄のまま放置しない
4. If アカウント情報の取得に失敗したとき, the Account Settings UI shall 取得失敗である旨を利用者に通知し、アカウント設定 UI が空の状態で残らないようにする

### Requirement 3: 退会導線

**Objective:** As a 認証済みユーザー, I want アカウント設定から自分のアカウントを退会できる, so that 不要になったアカウントを自力で削除し、Feedman 側のデータを残さずサービス利用を終了できる

#### Acceptance Criteria

1. While Account Settings UI が開かれているとき, the Account Settings UI shall 退会操作の起動要素を表示する
2. When ユーザーが退会起動要素を操作したとき, the Account Settings UI shall 退会内容（データ削除・取り消し不能）を明示した確認ダイアログを表示する
3. When 確認ダイアログでユーザーがキャンセルを選択したとき, the Account Settings UI shall 退会要求を送信せず、認証状態と Account Settings UI の表示を維持する
4. When 確認ダイアログでユーザーが退会を最終確定したとき, the Web Application shall バックエンドに対して現行の退会エンドポイントと整合する削除要求を送信し、成功応答を受領する
5. When 退会が成功したとき, the Web Application shall 認証キャッシュを破棄し未認証状態へ遷移させ、ログイン画面が表示される状態にする
6. If 退会リクエストがネットワークエラー・サーバエラー等で失敗したとき, the Web Application shall ユーザーに失敗した旨を通知し、既存の認証セッションを破棄しない
7. When email 未設定（空文字）のパスキーのみアカウントで退会を最終確定したとき, the Web Application shall email 有設定アカウントと同一のフローで退会を完了させ、未認証状態へ遷移させる
8. While 退会リクエストの応答を待機している間, the Account Settings UI shall 確認ダイアログの退会確定操作を非活性化し、多重送信を防止する

## Non-Functional Requirements

### NFR 1: 既存 UI との非破壊性

1. The Web Application shall 既存の 2 ペインレイアウト（フィード一覧・記事一覧・記事詳細）の挙動を変更しない
2. The Web Application shall 既存の購読設定ダイアログの起動経路と挙動を変更しない
3. The Web Application shall ヘッダーの既存機能（テーマ切替・ログアウト等）の挙動を、本機能追加によって変更しない

### NFR 2: 検証可能性

1. The Web Application shall Requirement 2.1〜2.4 の各表示条件（表示名表示・email 有設定表示・email 空プレースホルダ表示・取得失敗通知）を自動テストで検証可能な形で実装する
2. The Web Application shall Requirement 3.4〜3.7 の退会フロー（正常成功による未認証遷移・失敗時セッション維持・パスキーのみアカウントでの成功）を自動テストで検証可能な形で実装する
3. The Web Application shall 認証済み画面での Requirement 1.1 入口表示と、未認証時の Requirement 1.3 非表示を自動テストで検証可能な形で実装する

## Out of Scope

- パスキー credential の一覧・削除・リネーム等の管理 UI
- パスキー追加登録（registration/add）を起動する Web UI
- プロフィール編集（表示名変更・email 追加・アイコン変更）
- OAuth アカウントとパスキー credential のリンク／解除操作
- ログアウトボタンのアカウント設定メニューへの集約（PM 判断: 最小スコープに絞るため既存 LogoutButton のヘッダー配置と挙動を維持し、本 Issue では退会導線とアカウント情報表示のみを配線対象とする。集約の要否は本機能リリース後の利用状況を見て別 Issue で判断する）
- 退会確認画面での追加情報入力（理由アンケート・パスワード再入力等）
- Web 以外（モバイル / API 直接利用）の退会 UX 改善

## Open Questions

- 「アカウント設定」の UI 形式（ヘッダー配下のポップオーバーメニューか、ダイアログか、独立ページか）は本要件では規定していない。design 層で選択し、Requirement 1・2・3 の観測可能な挙動を満たす形にすること
- email 未設定時の表示文言として本要件では「未設定」を採用しているが、既存 UI コピー方針との整合上より適切な文言（例: 「(メールアドレス未登録)」等）があれば design/impl 段階で調整して差し支えない
- 退会失敗時（Requirement 3.6）および取得失敗時（Requirement 2.4）の通知手段（トースト・インラインエラー等）の具体形式は design 層で決定する
