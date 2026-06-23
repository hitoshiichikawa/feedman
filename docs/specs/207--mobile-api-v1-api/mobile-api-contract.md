# Feedman Mobile API Contract (v1)

本文書は Feedman v1 モバイルクライアント（iOS / Android）が依存するサーバー API の
契約を、サーバー実装ソースを参照せずにクライアント側だけで実装・検証できる粒度で
明文化したものである（NFR 2.1）。

- 対象クライアント: iOS（feedman-ios） / Android
- 対象サーバー実装: 本リポジトリの当該 spec branch（`claude/issue-207-impl--mobile-api-v1-api`）
  を含む `develop` ブランチ以降
- 文書スコープ: v1 で接続確認対象とする全エンドポイントの契約（URL / 認証方式 /
  要求・応答 JSON / エラー応答）
- 文書非対象: モバイルクライアント実装内部、Web フロントエンドの UI 仕様、
  サーバー実装の内部詳細（DB スキーマ・middleware 配線等）

---

## 1. 概要

### 1.1 v1 の位置付け

v1 は「ログイン後の認証済み API を呼び出して主要動線（記事一覧・記事詳細・検索・
購読操作・横断新着）を完成させる」フェーズである。Native auth（PKCE による access /
refresh token 発行・rotation・revoke）は別 spec 群（#163 配下 #165〜#171 / #172）で
**既に実装完了済み** であり、本文書ではそのサマリと参照のみを記載する。

### 1.2 スコープ境界

- **v1 で呼び出す API**: 本文書の §3, §4, §5 に記載
- **v1 で呼び出さない API**: §6「v1 スコープ外（次フェーズ）」に記載

### 1.3 既存 Web 動線との関係

既存 Web フロントエンド（Cookie セッション）が利用する API はすべて維持され、本 spec
で URL・認証方式・応答形状の破壊的変更は一切行わない（NFR 1.1, 1.2, 1.3）。詳細は
§7「後方互換ポリシー」を参照。

---

## 2. 共通方針

### 2.1 認証方式

| クライアント種別 | 認証ヘッダ / Cookie | 取得経路 |
|---|---|---|
| モバイル（iOS / Android） | `Authorization: Bearer <access_token>` HTTP ヘッダ | §3 native auth の `POST /api/auth/token` で発行 |
| Web フロントエンド | `Cookie: session_id=<sid>` | `/auth/google/login` → `/auth/google/callback` で HttpOnly Cookie として設定 |

`/api/*` 配下の認証必須エンドポイントは Bearer / Cookie の **両経路** を統一的に受け付ける
（BearerOrSession middleware により判別）。クライアントは自分の経路のいずれか 1 つだけを
提示すればよい。両方を同時に提示した場合の挙動はサーバー側で `Authorization` ヘッダが
優先されるが、モバイルクライアントは Bearer のみ提示する想定であり Cookie の併用は不要。

未認証時（Authorization 無し + Cookie 無し / どちらも無効）は 401 `unauthorized`
が返る（応答ボディは text/plain `unauthorized`）。

`/auth/me`（Web 専用 current user 取得）は本文書のスコープ外の **既存 Web 動線** であり、
モバイルクライアントは利用しない。詳細は §7「後方互換ポリシー」を参照。

### 2.2 エラー応答形式

`/api/*` 配下のエンドポイントは、4xx / 5xx エラー時に統一フォーマット JSON を返す
（`Content-Type: application/json`）:

```json
{
  "code": "ITEM_NOT_FOUND",
  "message": "指定された記事が見つかりません: <item-id>",
  "category": "feed",
  "action": "記事IDを確認してください。",
  "details": { "retry_after_seconds": 600 }
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `code` | string | 必須 | エラーコード（後述 §2.3 に主要コード一覧） |
| `message` | string | 必須 | エンドユーザー向けの日本語メッセージ |
| `category` | string | 必須 | 原因カテゴリ。`auth` / `validation` / `feed` / `system` / `authorization` のいずれか |
| `action` | string | 必須 | ユーザーが取るべき次の行動を示す文言 |
| `details` | object | 任意 | 追加情報。例: 429 のクールダウン応答で `retry_after_seconds`（int 秒）を載せる。値不在時はフィールド自体が JSON から省略される（`omitempty` 相当） |

例外: `BearerOrSession` middleware が拒否する 401 応答（認証情報無し / Bearer 検証失敗）は
**text/plain `unauthorized`** を返す（上記 JSON 形式ではない）。これは middleware が
handler 到達前に短絡応答するためで、本文書でも以後特に明記しない限り 401 はこの形である。

### 2.3 主要エラーコード一覧

| HTTP Status | code | 発生場面 |
|---|---|---|
| 400 | `INVALID_URL` | フィード登録時の URL 形式不正 |
| 400 | `INVALID_FILTER` | 記事一覧 `filter` パラメータが不正 |
| 400 | `INVALID_FETCH_INTERVAL` | フェッチ間隔が範囲外 |
| 400 | `INVALID_SEARCH_QUERY` | 検索 `cursor` / `feed_id` / `limit` の形式不正 |
| 400 | `INVALID_REQUEST` | クエリパラメータの一般的な形式不正（cross-feed の `since` 等） |
| 401 | （text/plain `unauthorized`） | 未認証（Authorization / Cookie 不在 or 無効） |
| 403 | `FEED_NOT_SUBSCRIBED` | 検索 `feed_id` 指定先を当該ユーザーが未購読 |
| 404 | `ITEM_NOT_FOUND` | 記事が当該ユーザーの購読範囲に存在しない（未購読時の存在秘匿を含む） |
| 404 | `SUBSCRIPTION_NOT_FOUND` | 購読が見つからない（解除・設定更新・再開・手動フェッチ時） |
| 404 | `USER_NOT_FOUND` | 認証通過後に DB 上の user が消失している例外時 |
| 409 | `FEED_FETCH_IN_PROGRESS` | 手動フェッチ時、別トランザクションがフェッチ中 |
| 422 | `FEED_NOT_DETECTED` / `PARSE_FAILED` / `SSRF_BLOCKED` / `FETCH_FAILED` / `FEED_HTTP_ERROR` | フィード登録時の検出・取得・解析失敗 |
| 429 | `FEED_COOLDOWN` | 手動フェッチが 10 分クールダウン中。`details.retry_after_seconds` に残り秒数（int） |
| 500 | `INTERNAL_ERROR` | 内部エラー（DB エラー等） |

新しいエラーコードは将来追加され得る。クライアントは未知の `code` を受け取った場合でも
`category` / `message` を用いて汎用的にハンドリングできるよう実装すること。

### 2.4 JSON 命名規約

- フィールド名は **snake_case**（例: `next_cursor` / `is_read` / `feed_title`）
- タイムスタンプは **RFC3339** 文字列（例: `2025-06-23T10:00:00Z`）
- ID は文字列（UUID v4 形式が多いが、クライアントは不透明な文字列として扱う）
- nullable な値は `string | null` または `omitempty` でフィールド自体が省略される。
  どちらの表現になるかは本文書の各エンドポイント節で個別に明記する

### 2.5 Cursor pagination 形式

記事一覧系（`/api/feeds/{id}/items`, `/api/feeds/starred/items`, `/api/items/search`,
`/api/items/cross-feed`）は共通の cursor pagination 形式を採用する:

**応答**:

```json
{
  "items": [ /* 当該ページの記事配列 */ ],
  "next_cursor": "2025-06-23T10:00:00.000Z|abcdef...",
  "has_more": true
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `items` | array | 必須 | 当該ページの記事配列 |
| `next_cursor` | string | 末尾ページでは省略（`omitempty`） | 次ページ取得用のカーソル文字列。形式 `<RFC3339Nano>\|<id>`（不透明文字列としてそのまま再送する） |
| `has_more` | bool | 必須 | 次ページが存在するか |

**次ページ取得**: `cursor=<next_cursor 文字列>` と `limit=<件数>` をクエリで送る。
`next_cursor` を発行できない末尾項目（PublishedAt がゼロ値等）でも `has_more=true` が
返ることがあるため、クライアントは `next_cursor` の空判定だけでなく `has_more` も
参照すること。

**`limit` の上限**: 各エンドポイント節で個別に記載する。共通の既定値は概ね 50 件、
最大は 200 件（cross-feed）/ 100 件（search）等エンドポイントごとに異なる。

### 2.6 リクエストボディの形式

`POST` / `PUT` は `Content-Type: application/json` で JSON ボディを送る。
ボディサイズは middleware により上限が設定されており、超過時は 413 が返る
（具体値は本文書のスコープ外）。

---

## 3. Native Auth エンドポイント（サマリ）

Native auth 系（access token 発行 / refresh / revoke / native callback）は本文書の
**契約レベルでのサマリのみ** を記載する。詳細仕様（PKCE パラメータ規約・JWT claim
構造・token rotation 規約・end-to-end 契約テスト）は以下の既存 spec を参照すること
（Req 1.2）:

- **Native Auth 全体設計**: Issue #163
- **PKCE login**: Issue #165
- **`POST /api/auth/token`**: Issue #166 / #172
- **`POST /api/auth/refresh`**: Issue #167 / #172
- **`POST /api/auth/revoke`**: Issue #168 / #172
- **`BearerOrSession` middleware**: Issue #169
- **JWT 署名鍵 rotation**: Issue #170, #171
- **End-to-end 契約テスト**: Issue #172

### 3.1 `GET /auth/google/login` — PKCE login 開始

| 項目 | 内容 |
|---|---|
| URL | `GET /auth/google/login?flow=native&code_challenge=<S256>&code_challenge_method=S256` |
| 認証 | 不要（OAuth フロー開始） |
| クエリ | `flow=native`（必須）／`code_challenge`（PKCE S256 challenge、Base64URL）／`code_challenge_method=S256`（必須） |
| 応答 | 302 redirect to Google OAuth consent screen。サーバ側 Cookie に PKCE challenge を保存（HttpOnly） |
| エラー | 400 `invalid pkce parameters`（PKCE パラメータ不正） |

詳細仕様は #165 を参照。

### 3.2 `feedman://auth/callback` — Native callback

| 項目 | 内容 |
|---|---|
| URL | `feedman://auth/callback?auth_code=<one-time code>` |
| 認証 | 不要（callback redirect の受け取り側） |
| 説明 | Google OAuth consent 完了後、サーバが上記アプリスキームにリダイレクトする。`auth_code` は短寿命の一回限り使い捨てコード |

詳細仕様は #165 を参照。

### 3.3 `POST /api/auth/token` — Access token 発行

| 項目 | 内容 |
|---|---|
| URL | `POST /api/auth/token` |
| 認証 | 不要（auth_code 交換） |
| リクエスト | `{"auth_code": "<callback で受領した auth_code>", "code_verifier": "<PKCE verifier>"}` |
| 成功応答 | 200 `{"access_token": "<JWT>", "refresh_token": "<opaque>", "token_type": "Bearer", "expires_in": <int 秒>}` |
| エラー | 400 / 401（auth_code 無効 / PKCE 検証失敗 / 期限切れ）。詳細は #166 / #172 |

### 3.4 `POST /api/auth/refresh` — Token rotation

| 項目 | 内容 |
|---|---|
| URL | `POST /api/auth/refresh` |
| 認証 | 不要（refresh_token を body で提示） |
| リクエスト | `{"refresh_token": "<現行の refresh_token>"}` |
| 成功応答 | 200 `{"access_token": "<新 JWT>", "refresh_token": "<新 opaque>", "token_type": "Bearer", "expires_in": <int 秒>}`。古い refresh_token は **無効化される**（rotation） |
| エラー | 400 / 401（refresh_token 無効 / 既に rotation 済 / 期限切れ）。詳細は #167 / #172 |

### 3.5 `POST /api/auth/revoke` — Refresh token 無効化

| 項目 | 内容 |
|---|---|
| URL | `POST /api/auth/revoke` |
| 認証 | 不要（refresh_token を body で提示） |
| リクエスト | `{"refresh_token": "<無効化したい refresh_token>"}` |
| 成功応答 | 204 No Content |
| エラー | 400 / 401（refresh_token 形式不正 / 既に無効）。詳細は #168 / #172 |

---

## 4. モバイル統一ユーザー情報取得

### 4.1 `GET /api/users/me` — Current user 取得

| 項目 | 内容 |
|---|---|
| URL | `GET /api/users/me` |
| 認証 | **必須**。Bearer（モバイル）または Cookie session_id（Web）のいずれか |
| リクエスト | （body なし） |
| 成功応答 | 200 `application/json`（後述） |
| エラー | 401（未認証）/ 404 `USER_NOT_FOUND`（認証通過後に DB 上 user が消失している例外時）/ 500 |

**成功応答スキーマ**:

```json
{
  "id": "<user id>",
  "email": "<user email>",
  "name": "<display name>",
  "avatar_url": null
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `id` | string | 必須 | ユーザー識別子 |
| `email` | string | 必須 | ログイン時のメールアドレス |
| `name` | string | 必須 | 表示名 |
| `avatar_url` | `string \| null` | 任意（v1 では常に null または省略） | アバター画像 URL。v1 時点では DB 未拡張のため常に null / 省略される。将来 OAuth プロフィール画像を保存する改修が入った場合、JSON 文字列として実値が返るようになる |

**注意事項**:

- `avatar_url` が **JSON null として返る** か **応答から省略される** かは実装側の選択であり、
  クライアントは両表現を等価として扱うこと（要件 2.4 で許容されている）
- 本エンドポイントは secret（session_id / refresh_token / hashed token 等）を一切応答に
  含めない
- 既存 Web 動線の `GET /auth/me` は本エンドポイントとは別物として **維持される**
  （§7 参照）。モバイルクライアントは `/auth/me` を呼ばない

---

## 5. v1 共通動線エンドポイント

本節に記載する全エンドポイントは `/api/*` 配下にあり、Bearer または Cookie 認証が
**必須** である。認証情報なしのアクセスはすべて 401 で拒否される。

### 5.1 `GET /api/feeds/{id}/items` — フィード内記事一覧

| 項目 | 内容 |
|---|---|
| URL | `GET /api/feeds/{id}/items` |
| 認証 | 必須 |
| パスパラメータ | `id`: 購読フィードの UUID |
| クエリ | `filter` (`all` / `unread` / `starred`、既定 `all`) ／ `cursor`（前回 next_cursor、空文字で先頭ページ） ／ `limit`（既定 50、上限はサーバ側で制限） |
| 成功応答 | 200 `application/json`（後述） |
| エラー | 400 `INVALID_FILTER` / 401 / 404（未購読 / 不在のフィード） / 500 |

**成功応答スキーマ**:

```json
{
  "items": [
    {
      "id": "<item id>",
      "feed_id": "<feed id>",
      "title": "<title>",
      "link": "<https://...>",
      "summary": "<sanitized summary>",
      "published_at": "2025-06-23T10:00:00Z",
      "is_date_estimated": false,
      "is_read": false,
      "is_starred": false,
      "hatebu_count": 42
    }
  ],
  "next_cursor": "2025-06-23T10:00:00.000Z|abcdef...",
  "has_more": true
}
```

各 item フィールドの意味:

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `id` | string | 必須 | 記事 ID |
| `feed_id` | string | 必須 | 所属フィード ID |
| `title` | string | 必須 | 記事タイトル |
| `link` | string | 必須 | 元記事の URL |
| `summary` | string | 必須 | サニタイズ済み概要（空文字列もあり得る） |
| `published_at` | string (RFC3339) | 必須 | 公開日時 |
| `is_date_estimated` | bool | 必須 | 公開日時が推定値かどうか |
| `is_read` | bool | 必須 | 既読フラグ |
| `is_starred` | bool | 必須 | スター付与フラグ |
| `hatebu_count` | int | 必須 | はてなブックマーク件数 |

**フィード表示メタデータ（`feed_title` / `feed_favicon_url`）について**: 本一覧はクライアントが
「現在選択中のフィード」のコンテキストで呼び出すため、各 item に `feed_title` /
`feed_favicon_url` は **含まれない**。クライアントは選択中フィードのメタデータ（§5.7 の
購読一覧から取得）で補うこと。

### 5.2 `GET /api/feeds/starred/items` — 全フィード横断スター記事一覧

| 項目 | 内容 |
|---|---|
| URL | `GET /api/feeds/starred/items` |
| 認証 | 必須 |
| クエリ | `cursor` / `limit`（§2.5 共通形式に従う） |
| 成功応答 | 200 `application/json`（後述） |
| エラー | 401 / 500 |

**成功応答スキーマ**: items 配列の各要素が `feed_title` を **追加** で含む（複数フィードを
横断するため、UI 側で「どのフィードの記事か」を表示する必要があるため）:

```json
{
  "items": [
    {
      "id": "<item id>",
      "feed_id": "<feed id>",
      "feed_title": "<フィード表示名>",
      "title": "<title>",
      "link": "<https://...>",
      "summary": "<sanitized summary>",
      "published_at": "2025-06-23T10:00:00Z",
      "is_date_estimated": false,
      "is_read": false,
      "is_starred": true,
      "hatebu_count": 12
    }
  ],
  "next_cursor": "...",
  "has_more": false
}
```

`feed_favicon_url` は本エンドポイントの応答には **含まれない**（v1 スコープ。将来必要に
なれば加算的に追加される）。

### 5.3 `GET /api/items/{id}` — 記事詳細

| 項目 | 内容 |
|---|---|
| URL | `GET /api/items/{id}` |
| 認証 | 必須 |
| パスパラメータ | `id`: 記事 UUID |
| クエリ | （なし） |
| 成功応答 | 200 `application/json`（後述） |
| エラー | 401 / 404 `ITEM_NOT_FOUND`（記事が当該ユーザーの購読範囲に存在しない。**未購読 / 不在の両方を同一応答にまとめ存在を秘匿する**）/ 500 |

**成功応答スキーマ**（本 spec で `feed_title` / `feed_favicon_url` を追加した拡張後の形）:

```json
{
  "id": "<item id>",
  "feed_id": "<feed id>",
  "title": "<title>",
  "link": "<https://...>",
  "summary": "<sanitized summary>",
  "published_at": "2025-06-23T10:00:00Z",
  "is_date_estimated": false,
  "is_read": true,
  "is_starred": false,
  "hatebu_count": 42,
  "content": "<sanitized HTML 本文>",
  "author": "<著者名 or 空文字>",
  "feed_title": "<フィード表示名>",
  "feed_favicon_url": "data:image/png;base64,..."
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| 既存 11 フィールド（`id` 〜 `hatebu_count`） | （§5.1 と同型） | 必須 | §5.1 のサマリと同一スキーマ |
| `content` | string | 必須 | サニタイズ済み HTML 本文 |
| `summary` | string | 必須 | サマリ（記事一覧の `summary` と重複する。v1 でも両方含める） |
| `author` | string | 必須 | 著者名（不明時は空文字） |
| `feed_title` | string | **追加 / 必須** | 所属フィードの表示タイトル（本 spec で追加） |
| `feed_favicon_url` | `string \| null` | **追加 / 任意（null 時は省略）** | 所属フィードの favicon。`data:image/...;base64,...` 形式の data URL。favicon が登録されていない場合は **JSON から省略される**（要件 3.4 で「null または省略」が許容） |

**注意事項**:

- 既存フィールド名・型は本 spec 導入により **一切変更されない**（要件 3.3 / NFR 1.2）
- `feed_favicon_url` の data URL は HTML / CSP 安全な形式に整形済（生バイトを返さない）
- 未購読記事へのアクセスは「記事が存在しない」と同じ 404 `ITEM_NOT_FOUND` を返す
  （存在秘匿 / 要件 3.5）

### 5.4 `GET /api/items/search` — 記事検索

| 項目 | 内容 |
|---|---|
| URL | `GET /api/items/search` |
| 認証 | 必須 |
| クエリ | `q`（検索キーワード、必須）／ `feed_id`（UUID、任意。指定時はフィード内検索、省略時は全購読フィード横断検索）／ `cursor` / `limit`（§2.5 共通形式） |
| 成功応答 | 200 `application/json`（後述） |
| エラー | 400 `INVALID_SEARCH_QUERY`（`cursor` / `feed_id` / `limit` の形式不正） / 401 / 403 `FEED_NOT_SUBSCRIBED`（`feed_id` 指定先が当該ユーザー未購読） / 500 |

**成功応答スキーマ**: items 配列の各要素が `feed_title` を含む（横断検索でも feed 内検索でも
共通の応答形）。また検索結果固有のオプションフィールド `favicon_url` / `hatebu_fetched_at`
が含まれる:

```json
{
  "items": [
    {
      "id": "<item id>",
      "feed_id": "<feed id>",
      "feed_title": "<フィード表示名>",
      "favicon_url": "data:image/png;base64,...",
      "title": "<title>",
      "link": "<https://...>",
      "summary": "<sanitized summary>",
      "published_at": "2025-06-23T10:00:00Z",
      "is_date_estimated": false,
      "is_read": false,
      "is_starred": false,
      "hatebu_count": 5,
      "hatebu_fetched_at": "2025-06-23T09:00:00Z"
    }
  ],
  "next_cursor": "...",
  "has_more": true
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| 既存 11 フィールド（`id` 〜 `hatebu_count`） | （§5.1 と同型） | 必須 | §5.1 のサマリと同一スキーマ |
| `feed_title` | string | 必須 | 所属フィード表示名 |
| `favicon_url` | `string \| null` | 任意（null 時は省略） | 所属フィード favicon の data URL。**`feed_favicon_url` ではなく `favicon_url`** である点に注意（記事詳細 §5.3 の命名と異なる。これは検索 API 固有の歴史的経緯による既存契約） |
| `hatebu_fetched_at` | string (RFC3339) | 任意（null 時は省略） | はてなブックマーク件数の取得時刻 |

**注意事項**:

- `scope=global|feed` のような明示的なスコープ切り替えパラメータは **契約に含まれない**。
  `feed_id` の有無で挙動が決まる（指定時 = feed 内検索 / 省略時 = 全購読横断検索）
- `feed_id` を未購読のフィード ID で送ると 403 `FEED_NOT_SUBSCRIBED` を返す
- 検索キーワード `q` の長さ・特殊文字の扱いはサーバー側で正規化される

### 5.5 `GET /api/items/cross-feed` — 横断新着記事一覧

| 項目 | 内容 |
|---|---|
| URL | `GET /api/items/cross-feed` |
| 認証 | 必須 |
| クエリ | `cursor` / `limit`（既定 50、上限 200） ／ `since`（任意、RFC3339）— **request では `since` という名前** |
| 成功応答 | 200 `application/json`（後述） |
| エラー | 400 `INVALID_REQUEST`（`limit` / `since` の形式不正） / 401 / 500 |

**成功応答スキーマ**: items 配列の各要素が `feed_title` / `feed_favicon_url` を含む。
また baseline として採用した時刻が `since_time` フィールド（**response では `since_time` という
名前**）で返る:

```json
{
  "items": [
    {
      "id": "<item id>",
      "feed_id": "<feed id>",
      "feed_title": "<フィード表示名>",
      "feed_favicon_url": "data:image/png;base64,...",
      "title": "<title>",
      "link": "<https://...>",
      "summary": "<sanitized summary>",
      "published_at": "2025-06-23T10:00:00Z",
      "is_date_estimated": false,
      "is_read": false,
      "is_starred": false,
      "hatebu_count": 0
    }
  ],
  "next_cursor": "...",
  "has_more": true,
  "since_time": "2025-06-23T09:00:00Z"
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| 既存 11 フィールド（`id` 〜 `hatebu_count`） | （§5.1 と同型） | 必須 | §5.1 のサマリと同一スキーマ |
| `feed_title` | string | 必須 | 所属フィード表示名 |
| `feed_favicon_url` | `string \| null` | 任意（null 時は応答に含まれるが `null` 値） | 所属フィード favicon の data URL |
| `since_time` | string (RFC3339) | 必須 | サーバが採用した新着判定基準時刻。クライアントは次回 `since` パラメータとしてこれを再利用できる |

**`since` ↔ `since_time` の対称性に関する注意**: リクエストパラメータ名は **`since`**、
レスポンスフィールド名は **`since_time`** で **非対称** である。これは v1 時点の既存契約で
あり、本文書でも将来的にも維持される（後方互換）。

**新着判定の動き**:

- `since` を指定しない場合、サーバ側の `user_cross_feed_views.last_seen_at`（ユーザーが
  最後に横断新着を閲覧した時刻）を baseline として用いる
- `since` を指定した場合、サーバ側 baseline を無視して当該値を用いる
- `PUT /api/users/me/cross-feed-last-seen`（§5.13）で baseline を進められる

### 5.6 `GET /api/subscriptions` — 購読一覧

| 項目 | 内容 |
|---|---|
| URL | `GET /api/subscriptions` |
| 認証 | 必須 |
| クエリ | （なし） |
| 成功応答 | 200 `application/json` — 配列を直接返す |
| エラー | 401 / 500 |

**成功応答スキーマ**:

```json
[
  {
    "id": "<subscription id>",
    "user_id": "<user id>",
    "feed_id": "<feed id>",
    "feed_title": "<フィード表示名>",
    "feed_url": "<https://example.com/feed.xml>",
    "favicon_url": "data:image/png;base64,...",
    "fetch_interval_minutes": 60,
    "feed_status": "active",
    "error_message": null,
    "unread_count": 12,
    "created_at": "2025-06-23T10:00:00Z"
  }
]
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `id` | string | 必須 | 購読 ID |
| `user_id` | string | 必須 | 当該ユーザー ID |
| `feed_id` | string | 必須 | フィード ID |
| `feed_title` | string | 必須 | フィード表示名 |
| `feed_url` | string | 必須 | フィード URL |
| `favicon_url` | `string \| null` | 任意（null 時は省略） | favicon の data URL（**`feed_favicon_url` ではなく `favicon_url`** である点に注意。§5.4 と同じ命名） |
| `fetch_interval_minutes` | int | 必須 | フェッチ間隔（30〜720 分、30 分刻み） |
| `feed_status` | string | 必須 | フィード状態（`active` / `stopped` 等） |
| `error_message` | `string \| null` | 任意（null 時は省略） | 最新のフェッチエラーメッセージ |
| `unread_count` | int | 必須 | 未読件数 |
| `created_at` | string (RFC3339) | 必須 | 購読開始時刻 |

### 5.7 `POST /api/feeds` — 新規購読登録

| 項目 | 内容 |
|---|---|
| URL | `POST /api/feeds` |
| 認証 | 必須 |
| リクエスト | `{"url": "<https://example.com/feed.xml or トップページ URL>"}` |
| 成功応答 | 201 `application/json` |
| エラー | 400 `INVALID_URL` / 422 `FEED_NOT_DETECTED` / `PARSE_FAILED` / `SSRF_BLOCKED` / `FETCH_FAILED` / `FEED_HTTP_ERROR`（`details.status_code`）／ 409 `DUPLICATE_SUBSCRIPTION` / 401 / 500 |

**成功応答スキーマ**:

```json
{
  "id": "<feed id>",
  "feed_url": "<最終検出された feed URL>",
  "site_url": "<サイトのトップ URL>",
  "title": "<フィードタイトル>",
  "fetch_status": "active"
}
```

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `id` | string | 必須 | フィード ID |
| `feed_url` | string | 必須 | サーバ側で検出した最終的なフィード URL |
| `site_url` | string | 必須 | フィードに紐付くサイト URL |
| `title` | string | 必須 | フィード表示名 |
| `fetch_status` | string | 必須 | フェッチ状態 |

### 5.8 `DELETE /api/subscriptions/{id}` — 購読解除

| 項目 | 内容 |
|---|---|
| URL | `DELETE /api/subscriptions/{id}` |
| 認証 | 必須 |
| パスパラメータ | `id`: 購読 ID |
| 成功応答 | 204 No Content |
| エラー | 401 / 404 `SUBSCRIPTION_NOT_FOUND` / 500 |

### 5.9 `PUT /api/subscriptions/{id}/settings` — フェッチ間隔設定

| 項目 | 内容 |
|---|---|
| URL | `PUT /api/subscriptions/{id}/settings` |
| 認証 | 必須 |
| パスパラメータ | `id`: 購読 ID |
| リクエスト | `{"fetch_interval_minutes": 60}`（30〜720 の 30 分刻み） |
| 成功応答 | 200（更新後の購読情報。§5.6 の項目と同形） |
| エラー | 400 `INVALID_FETCH_INTERVAL` / 401 / 404 `SUBSCRIPTION_NOT_FOUND` / 500 |

### 5.10 `POST /api/subscriptions/{id}/resume` — 停止フィード再開

| 項目 | 内容 |
|---|---|
| URL | `POST /api/subscriptions/{id}/resume` |
| 認証 | 必須 |
| パスパラメータ | `id`: 購読 ID |
| リクエスト | （body なし） |
| 成功応答 | 200（更新後の購読情報。§5.6 の項目と同形） |
| エラー | 401 / 404 `SUBSCRIPTION_NOT_FOUND` / 409 `FEED_NOT_STOPPED`（停止中でないフィード）/ 500 |

### 5.11 `POST /api/subscriptions/{id}/fetch` — 手動フェッチ

| 項目 | 内容 |
|---|---|
| URL | `POST /api/subscriptions/{id}/fetch` |
| 認証 | 必須 |
| パスパラメータ | `id`: 購読 ID |
| リクエスト | （body なし） |
| 成功応答 | 200（更新後の購読情報。§5.6 の項目と同形） |
| エラー | 401 / 404 `SUBSCRIPTION_NOT_FOUND` / 409 `FEED_FETCH_IN_PROGRESS` / 429 `FEED_COOLDOWN`（`details.retry_after_seconds` に残り秒数 int）/ 500 |

**注意事項**: 手動フェッチは最終成功時刻から 10 分のクールダウンが課される。
429 応答の `details.retry_after_seconds` に従って UI 側で次回フェッチ可能時刻を案内すること。

### 5.12 `PUT /api/items/{id}/state` — 既読 / スター状態更新

| 項目 | 内容 |
|---|---|
| URL | `PUT /api/items/{id}/state` |
| 認証 | 必須 |
| パスパラメータ | `id`: 記事 ID |
| リクエスト | `{"is_read": true, "is_starred": false}`（両フィールド任意、nil は変更なし。部分更新） |
| 成功応答 | 200 `application/json` |
| エラー | 401 / 404 `ITEM_NOT_FOUND` / 500 |

**成功応答スキーマ**:

```json
{
  "item_id": "<item id>",
  "is_read": true,
  "is_starred": false
}
```

### 5.13 `PUT /api/users/me/cross-feed-last-seen` — 横断一覧の最終閲覧時刻更新

| 項目 | 内容 |
|---|---|
| URL | `PUT /api/users/me/cross-feed-last-seen` |
| 認証 | 必須 |
| リクエスト | （body なし。サーバ側で `now()` を採用） |
| 成功応答 | 204 No Content |
| エラー | 401 / 500 |

**注意事項**: 本エンドポイントを呼ぶと、次回の `GET /api/items/cross-feed`（§5.5）で
`since` パラメータを指定しなかった場合の baseline（`user_cross_feed_views.last_seen_at`）が
`now()` に更新される。

---

## 6. v1 スコープ外（次フェーズ）

以下のエンドポイントは v1 接続確認の **必須契約に含めない**。モバイルクライアントは
v1 では呼び出さないこと。これらはキーワードプッシュ通知の **次フェーズ** に該当する
（要件 1.5 / NFR 2.2）。

| エンドポイント | 用途 | 位置付け |
|---|---|---|
| `/api/devices` | デバイス登録（push 通知トークン管理） | 次フェーズ。v1 ではサーバ側に未実装 / クライアントから呼び出さない |
| `/api/keywords` | キーワード通知（指定キーワード一致時の push） | 次フェーズ。v1 ではサーバ側に未実装 / クライアントから呼び出さない |

これらは将来の別 Issue で起票・実装される。v1 リリース時点では契約自体が存在しない。

---

## 7. 後方互換ポリシー

### 7.1 既存フィールドの不変性

本 spec 導入により、以下を **一切変更しない**:

- 既存エンドポイントの URL / HTTP method
- 既存エンドポイントの認証方式（Cookie 専用エンドポイントは Cookie のままで Bearer
  対応を強制しない）
- 既存応答 JSON のフィールド名・型・null 表現
- 既存エラーコード / HTTP status の対応

### 7.2 進化のルール

新規機能の追加は **加算的変更（adding-only）** のみで行う:

- 新しいエンドポイント / 新フィールドの追加は可
- 既存フィールドの削除・改名・型変更は **不可**
- nullable な追加フィールドは `omitempty` または `null` のいずれかで表現してよい

本文書は v1 契約として **追加フィールド** を 2 つ含む（記事詳細応答の `feed_title` /
`feed_favicon_url`）。これらは既存クライアントの応答パースを破壊しない（既存フィールドが
すべて保持されているため）。

### 7.3 `/auth/me` の維持

既存 Web 専用エンドポイント `GET /auth/me` は本 spec で **一切変更しない**（要件 2.6 /
NFR 1.1）:

- URL: `GET /auth/me`（不変）
- 認証: Cookie `session_id` 専用（Bearer 経路へ拡張しない）
- 応答: `{"id":"<>", "email":"<>", "name":"<>"}` の 3 フィールド（不変。`avatar_url` を
  追加しない）

モバイルクライアントは `/auth/me` を呼ばず、`§4.1 GET /api/users/me` を利用すること。
両エンドポイントは並行提供される。

### 7.4 命名の歴史的非対称性

以下は v1 時点で既存契約として存在する命名の非対称性であり、本文書および将来も
**そのまま維持される**:

| エンドポイント | favicon フィールド名 | 備考 |
|---|---|---|
| `GET /api/items/{id}` | `feed_favicon_url` | 本 spec で追加 |
| `GET /api/items/cross-feed` | `feed_favicon_url` | 既存 |
| `GET /api/items/search` | `favicon_url` | 既存（歴史的経緯） |
| `GET /api/subscriptions` | `favicon_url` | 既存（歴史的経緯） |

`/api/items/cross-feed` の request `since` ↔ response `since_time` も既存非対称（§5.5 参照）。

クライアントは各エンドポイントごとに正しいフィールド名でパースすること。

---

## 関連 spec / 関連 Issue

- **本 spec**: Issue #207 — v1 モバイル API 契約の明文化と統一ユーザー情報 / 記事詳細
  フィードメタデータの追加
- **Native auth 全体**: Issue #163
- **PKCE login**: Issue #165
- **`POST /api/auth/token`**: Issue #166
- **`POST /api/auth/refresh`**: Issue #167
- **`POST /api/auth/revoke`**: Issue #168
- **`BearerOrSession` middleware**: Issue #169
- **JWT 署名鍵 rotation**: Issue #170, #171
- **Native auth end-to-end 契約テスト**: Issue #172

---

## 改訂履歴

| 日付 | 改訂内容 | 関連 Issue |
|---|---|---|
| 2026-06-23 | 初版作成（v1 モバイル API 契約の明文化、`GET /api/users/me` 新設、記事詳細応答への `feed_title` / `feed_favicon_url` 追加） | #207 |
