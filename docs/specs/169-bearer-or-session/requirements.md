# Requirements Document

## Introduction

Feedman iOS（親 Issue #163）は Issue #166 の token 交換で取得した access token を
`Authorization: Bearer` ヘッダとして既存 API（フィード・記事・購読・ユーザー等の認証必須
エンドポイント群）に送る。現状の認証必須 API は Cookie セッションの提示のみを受け付けるため、
Bearer token を Cookie セッションと同じユーザー識別に解決する受け口が無いと、iOS からの
API 呼び出しが成立しない。

本 spec は **既存認証必須 API への Bearer 認証の追加**のみを対象とする。具体的には
(1) 有効な access token の提示を Cookie セッション認証と同一のユーザー識別へ解決すること、
(2) 無効な token の提示を Cookie への fallback なしに未認証として拒否すること、
(3) Bearer 不在時は既存の Cookie セッション認証へ完全に委譲し従来挙動を保つこと、
(4) 署名鍵未設定の環境では Bearer 認証を安全に無効化すること、を要件化する。
access token の発行（#166）、refresh のローテーション（#167）、再利用検知と revoke（#168）、
未認証エンドポイントの IP レート制限（#171）はスコープ外。

## Requirements

### Requirement 1: Bearer token による認証の成立

**Objective:** As a Feedman iOS アプリ, I want 取得済みの access token を要求に付与して
既存 API を呼び出したい, so that Cookie セッションを持たずに自分のデータへアクセスできる

#### Acceptance Criteria

1. When 認証必須 API への要求が有効な access token を `Authorization: Bearer` として含むとき, the API Server shall Cookie セッション認証と同一のユーザー識別で当該要求を処理する
2. When 同一ユーザーが同一の認証必須 API を Bearer 認証で呼び出したとき, the API Server shall Cookie セッション認証時と同一の応答内容・形式を返す
3. The API Server shall Bearer 認証の成立に Cookie セッションの併送を要求しない
4. When 有効な access token と有効な Cookie セッションの両方が提示されたとき, the API Server shall access token から解決したユーザーとして当該要求を処理する

### Requirement 2: 無効な Bearer token の拒否

**Objective:** As a Feedman 運用者, I want 無効な access token の提示が確実に拒否されてほしい,
so that 失効・改ざん token の黙認や Cookie への黙示的な fallback がセキュリティの抜け穴にならない

#### Acceptance Criteria

1. If 提示された access token の署名が検証できないとき, the API Server shall 当該要求を未認証として拒否する
2. If 提示された access token が期限切れのとき, the API Server shall 当該要求を未認証として拒否する
3. If 提示された token の用途種別が access token でないとき, the API Server shall 当該要求を未認証として拒否する
4. If Bearer として提示された token が無効で、かつ有効な Cookie セッションが併送されていたとき, the API Server shall Cookie セッションでの認証成立に fallback せず当該要求を未認証として拒否する
5. If `Authorization: Bearer` の token 部が空または形式不正のとき, the API Server shall 当該要求を未認証として拒否する
6. The API Server shall Bearer 認証の未認証応答を既存 Cookie セッション認証の未認証応答と同一形式とする
7. The API Server shall 未認証応答において、署名不正・期限切れ・用途不一致・形式不正の別を区別できる情報を返さない

### Requirement 3: Bearer 不在時の委譲（既存挙動の維持）

**Objective:** As a Feedman Web ユーザー, I want Cookie セッションでの利用が従来どおり
動作してほしい, so that 本機能の導入によって既存の利用体験が一切変わらない

#### Acceptance Criteria

1. When 認証必須 API への要求が Bearer token を含まないとき, the API Server shall 既存の Cookie セッション認証で当該要求を処理する
2. When `Authorization` ヘッダが Bearer 以外の認証方式で提示されたとき, the API Server shall Bearer 提示なしとして扱い既存の Cookie セッション認証で当該要求を処理する
3. When Bearer token を含まない要求が有効な Cookie セッションを提示したとき, the API Server shall 本機能導入前と同一の応答を返す
4. If Bearer token も有効な Cookie セッションも提示されないとき, the API Server shall 本機能導入前と同一の未認証応答を返す

### Requirement 4: 検証の構成と安全な縮退

**Objective:** As a Feedman 運用者, I want access token の検証が発行側と同一の鍵設定で行われ、
鍵未設定の環境では機能が安全に無効化されてほしい, so that 設定不整合や未設定環境での事故を防げる

#### Acceptance Criteria

1. The API Server shall access token の検証に token 発行機能と同一の署名鍵設定を用い、発行された有効な access token を受理する
2. If 署名鍵が未設定のとき, the API Server shall Bearer 認証を無効化し、認証必須 API を本機能導入前と同一（Cookie セッション認証のみ）で提供する
3. While 署名鍵が未設定の間, when 認証必須 API への要求が Bearer token を含むとき, the API Server shall 当該 token を評価せず本機能導入前と同一に処理する
4. If 署名鍵が未設定のとき, the API Server shall 起動を成功させる
5. Where テストコードが対象となるとき, the Token Verification Module shall 固定の署名鍵・固定の時刻を注入して決定論的に検証可能とする

## Non-Functional Requirements

### NFR 1: セキュリティ

1. The API Server shall 提示された token の値をログ・エラー応答のいずれにも残さない
2. The API Server shall access token の検証を、保存済みセッション情報・外部サービスへの照会なしで完結する

### NFR 2: 後方互換性

1. The API Server shall 既存の Cookie セッション認証で成立しているすべての API 挙動（応答・ログ・レート制限の適用）を変更しない
2. The API Server shall 署名鍵の環境変数が存在しない既存デプロイ環境でも、本 spec 導入前と同一に起動・動作する

### NFR 3: テスト容易性

1. The Token Verification Module shall 有効 token・期限切れ・署名不正・用途不一致・形式不正・Bearer 不在の委譲・署名鍵未設定の縮退の各ケースを外部ネットワーク依存なしで検証可能にする

## Out of Scope

- access token の発行・交換（`POST /api/auth/token`、Issue #166 — 本 spec は**検証**のみ）
- refresh のローテーション（#167）、再利用検知と revoke（#168）
- token / refresh endpoint への未認証 IP レート制限（#171）
- contract / integration テストの全体整備（#172）
- 既存 API の応答形式の変更、Web frontend の認証 UI 変更
- token の即時失効（失効リスト照合）— access token は短命であることを前提に、失効は有効期限で吸収する
- 複数署名鍵の並行受理（鍵ローテーション）— 単一鍵での検証までを本 spec で行う

## Open Questions

- `Authorization` ヘッダの認証方式名の大文字小文字の扱いは、HTTP 仕様（方式名は大文字小文字を
  区別しない）に従う前提で design に委ねる

## 関連

- Parent: #163
- Depends on: #166
- Sibling: #164 #165 #167 #168 #170 #171 #172
