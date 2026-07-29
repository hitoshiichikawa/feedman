# Requirements Document

## Introduction

パスキー（WebAuthn）で新規登録したユーザーが自分の指定した ID（username）を UI 上のどこ
でも確認できず、「どのアカウントでログインしているか」を判別できない不具合を修正する。
現状、パスキー登録処理は `users.username` と `users.username_normalized` を保存する一方で
`users.name`（表示名）を空のままとし、`GET /auth/me` の応答にも username フィールドが
含まれない。結果としてアカウント設定ダイアログ（#236 で導入）は表示名が空 / email 未設定
となり、指定 ID がどこにも表示されない。本改修では (a) 登録時に表示名を username で初期化、
(b) `GET /auth/me` に username を追加、(c) アカウント設定ダイアログでの username 表示、
の 3 点を後方互換を保ちつつ実施する。関連: #223（Web パスキー登録）／ #236（アカウント設定
ダイアログ）。

## Requirements

### Requirement 1: 新規パスキー登録時の表示名（name）初期化

**Objective:** As a パスキーで新規登録するユーザー, I want 登録直後から自分が指定した ID が
表示名として画面に表示されること, so that ログイン中のアカウントを識別できる

#### Acceptance Criteria

1. When ユーザーがパスキー新規登録の finish 段階を成功で完了したとき, the Passkey Registration Service shall 当該ユーザーの表示名（name）を保存済み username と同値で初期化する
2. When 上記登録が成功したとき, the Passkey Registration Service shall 保存済みの username（username_normalized）を変更しない
3. If パスキー新規登録の finish 段階が失敗（重複・拒否・障害）したとき, the Passkey Registration Service shall 表示名（name）を含むユーザー行を永続化しない
4. The Passkey Registration Service shall 表示名の初期化を users 行の作成と同一トランザクション内で実施する

### Requirement 2: `/auth/me` 応答への username フィールド追加

**Objective:** As Web フロントエンド, I want 現在ログイン中ユーザーの username を API から取得できること, so that 画面上に username を表示できる

#### Acceptance Criteria

1. When Web セッション Cookie を持つクライアントが `GET /auth/me` を要求したとき, the Auth API shall 応答本文に username フィールドを含める
2. When 応答対象のユーザーが username を保有しているとき, the Auth API shall username フィールドに保存済み username の文字列を返す
3. When 応答対象のユーザーが username を保有していない（Google 由来ユーザー等）とき, the Auth API shall username フィールドを null として返す
4. The Auth API shall `GET /auth/me` の既存フィールド（id / email / name）の名前・型・値の意味を変更しない
5. The Auth API shall `GET /auth/me` の HTTP ステータスコード（認証成功時 200 / 未認証時 401）を変更しない
6. If Web セッション Cookie を持たないクライアントが `GET /auth/me` を要求したとき, the Auth API shall 401 を返し username を含めた応答本文を返さない

### Requirement 3: アカウント設定ダイアログでの username 表示

**Objective:** As ログイン中のユーザー, I want アカウント設定ダイアログで自分の username と表示名を目視で確認できること, so that 複数アカウント運用時やサポート問い合わせ時に主体を明示できる

#### Acceptance Criteria

1. When ユーザーがアカウント設定ダイアログを開き `/auth/me` の応答取得に成功したとき, the Account Settings Dialog shall 表示名フィールドに応答の name を表示する
2. When 上記状態で応答の username が非 null / 非空であるとき, the Account Settings Dialog shall username を目視可能な形式で表示する
3. When 上記状態で応答の username が null または空文字であるとき, the Account Settings Dialog shall username 表示要素を描画しない
4. The Account Settings Dialog shall username 表示の追加によって既存の表示名・email・退会導線の表示条件および操作を変更しない

### Requirement 4: Google 由来ユーザーへの非破壊性

**Objective:** As Google OAuth で登録済みの既存ユーザー, I want 本修正の前後で自分のアカウント画面・認証挙動が変化しないこと, so that 既存ユーザー体験が壊れない

#### Acceptance Criteria

1. When Google 由来ユーザーが `GET /auth/me` を要求したとき, the Auth API shall 本修正導入前と同じ id / email / name を返す
2. When Google 由来ユーザーがアカウント設定ダイアログを開いたとき, the Account Settings Dialog shall 本修正導入前と同じ表示名・email 表示を維持する
3. The Passkey Registration Service shall 本修正によって Google 由来ユーザーの users 行を書き換えない

## Non-Functional Requirements

### NFR 1: API 応答の後方互換

1. The Auth API shall `GET /auth/me` の応答形式を、既存フィールド（id / email / name）が同名・同型・同意味で維持されるよう変更する（フィールドの削除・改名・型変更を伴わない追加のみで済ませる）
2. The Auth API shall 応答本文の Content-Type を既存挙動どおり application/json のまま維持する

### NFR 2: プライバシー・機密情報の非漏出

1. The Auth API shall `GET /auth/me` の応答本文にセッション ID / リフレッシュトークン / パスワードハッシュ / OAuth access token 等の秘匿値を含めない
2. The Passkey Registration Service shall 表示名初期化に伴う運用ログに username 生値を含めない（既存の challenge_id 先頭 8 文字のみを記録する運用と整合させる）

### NFR 3: 遡及影響の限定

1. The Passkey Registration Service shall 本修正の適用範囲を「本修正後に新規登録されるユーザー」に限定し、本修正前に既に作成済みのユーザーの users.name を自動更新しない

## Out of Scope

- 表示名（name）および username の編集機能（ユーザー自身による変更 UI / API）
- ヘッダー領域へのアバター表示・ユーザーメニューの新設（表示はアカウント設定ダイアログ内に留める）
- 本修正前に登録済みのパスキーユーザーの表示名を遡及的に補完するマイグレーション
- パスキー credential の管理画面（追加・削除・一覧表示など）
- iOS ネイティブアプリ側の表示改修（本修正は Web フロントエンドおよび Web が呼ぶ `/auth/me` に限定）

## Open Questions

- username の表示フォーマットについて Issue 本文では「例: @heatsea 形式」と例示されているが、`@` プレフィックスを必須とするか、装飾なしで username 文字列そのものを表示するかは明示されていない。UI 詳細は Architect / Developer フェーズで確定を要する（本要件では「目視可能な形式で表示する」に留めている）
- 本修正前に登録済みのパスキーユーザー（name が空のまま残存する既存ユーザー）の救済方針は本 Issue のスコープ外としているが、運用上救済が必要であれば別 Issue として起票するかを判断する必要がある

## 関連

- Related: #223 #236
