# Contract Notes: native auth API ↔ feedman-ios SERVER.md §1

本文書は feedman の native auth 実装（#164〜#171、すべて develop マージ済み）と
iOS 仕様の正本 `feedman-ios/design/SERVER.md` §1 の整合確認結果を記録する
（Issue #172 / Req 4.1〜4.5）。

- 確認日: 2026-06-12
- 確認対象: SERVER.md §1.2 / §1.3 / §1.4 / §1.5 / §1.6 / §1.7 / §1.8
- 結論: **承認済み逸脱 2 件（後述）を除き、実装は SERVER.md §1 と整合している。
  未承認の差分は発見されなかった。**

## §1.2 ネイティブ OAuth フロー

| 項目 | SERVER.md §1.2 | 実装 | 整合 |
|---|---|---|---|
| login 入口 | `/auth/google/login?flow=native&code_challenge=...` | `auth_handler.go`（#165。`code_challenge_method=S256` 必須） | ✓ |
| callback リダイレクト | `feedman://auth/callback?auth_code=<one-time-code>` | `handleNativeCallback`（303 / Cookie セッション非発行） | ✓ |
| PKCE | S256 必須 | `ValidatePKCES256` / `VerifyPKCES256Verifier`（RFC 7636 準拠・定数時間比較） | ✓ |
| auth_code | 一回限り・短命（60 秒）・単一交換 | `NativeAuthCodeTTL = 60s` / `MarkUsed` の atomic 単回消費 | ✓ |
| 後方互換 | `flow=native` 無しは従来どおり Cookie 発行 | web flow のコード経路は #165 で不変 | ✓ |

検証テスト: `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode` /
`TestE2E_NativeAuthFullFlow_DBBacked` step1〜2 / 既存 `TestIntegration_NativeAuthFlow_*`

## §1.3 エンドポイント契約

### POST /api/auth/token

| 項目 | SERVER.md §1.3 | 実装 | 整合 |
|---|---|---|---|
| Request | `{auth_code, code_verifier}` | `tokenRequest`（snake_case / 未知フィールド拒否） | ✓ |
| 200 Response | `{access_token, refresh_token, token_type:"Bearer", expires_in:900}` | `tokenResponse`（4 フィールド厳密） | ✓ |
| Error | 400 `INVALID_GRANT`（コード不正 / 期限切れ / 再利用） | `invalidGrantError()`（原因を区別しない uniform 拒否） | ✓ |
| 認証 | 未認証で叩ける。IP レート制限を適用 | 認証不要グループ + `unauthIPMW`（#171） | ✓ |

検証テスト: `TestContract_TokenResponse_ExactJSONShape` / `TestE2E_NativeAuthFullFlow_DBBacked`
step3・step3-reject

### POST /api/auth/refresh

| 項目 | SERVER.md §1.3 | 実装 | 整合 |
|---|---|---|---|
| Request | `{refresh_token}` | `refreshRequest` | ✓ |
| 200 Response | token と同形の 4 フィールド | `tokenResponse` 共用 | ✓ |
| Error | 401 `INVALID_REFRESH_TOKEN`（失効 / 不正 / 再利用検知） | `invalidRefreshTokenError()`（uniform 拒否） | ✓ |
| rotation | あり（旧 token 失効） | `RotateRefreshToken`（atomic `MarkRotated`） | ✓ |

検証テスト: `TestContract_RefreshResponse_ExactJSONShape` / `TestE2E_NativeAuthFullFlow_DBBacked`
step4・step4-reject

### POST /api/auth/revoke

| 項目 | SERVER.md §1.3 | 実装 | 整合 |
|---|---|---|---|
| Request | `{refresh_token}` | `refreshRequest` 共用 | ✓ |
| Response | 204 No Content | 204 + ボディなし（不明 token でも同一 / 冪等） | ✓ |
| 認証 | **「Bearer 認証下」と注記** | **未認証 + token 所持 = 権限**（#168） | **承認済み逸脱 →「承認済み逸脱」節** |

検証テスト: `TestContract_RevokeResponse_204AndEmptyBody` / `TestE2E_NativeAuthFullFlow_DBBacked`
step5 / 既存 `TestIntegration_RevokeFlow_RefreshRejectedAndIdempotent`

## §1.4 トークン設計

| 項目 | SERVER.md §1.4 | 実装 | 整合 |
|---|---|---|---|
| access token 形式 | JWT（HS256 or RS256）。claims: `sub`, `exp`, `iat`, `jti`, `token_use:"access"` | HS256 JWT（`JWTIssuer`）。claims 同一 | ✓（HS256 を選択） |
| access token 寿命 | 15 分（`expires_in: 900`） | `AccessTokenTTL = 15min` / 応答 `expires_in: 900` | ✓ |
| refresh token 形式 | 不透明乱数（256bit）・sha256 hash 保存 | `crypto/rand` 32 byte → base64url / `HashNativeSecret`（sha256） | ✓ |
| refresh token 寿命 | 30 日（スライディング） | `RefreshTokenTTL = 30d`（rotation 毎に再設定） | ✓ |
| rotation + 再利用検知 | 再利用検知時は当該ファミリ全失効 | #167 rotation + #168 family 失効昇格（strict 方針） | ✓ |
| access token 検証 | ステートレス（DB 不要） | `JWTVerifier`（署名鍵と時刻のみで完結） | ✓ |
| 署名鍵 / kid | 環境変数管理。`kid` を JWT ヘッダに含める | `NATIVE_AUTH_JWT_SECRET` / `NATIVE_AUTH_JWT_KID`（ヘッダ kid 出力。検証側は単一鍵 v1 で kid 不参照） | ✓ |

検証テスト: `internal/auth/jwt_issuer_test.go` / `jwt_verifier_test.go`（既存 unit）/
`TestE2E_NativeAuthFullFlow_DBBacked`（実物 issuer↔verifier の通し）

## §1.5 DB スキーマ

| 項目 | SERVER.md §1.5（例示） | 実装（#164 マイグレーション） | 整合 |
|---|---|---|---|
| refresh_tokens | 単一テーブル（`family_id` カラム + `revoked_at`） | **`refresh_token_families` / `refresh_tokens` の 2 テーブル分離**（family 単位 revoke を families.revoked_at で正本化、配下 token は FK CASCADE + revoked_at 同期） | **承認済み逸脱 →「承認済み逸脱」節** |
| `device_label` / `last_used_at` | 任意カラムとして例示 | 未実装（device_label 等の任意メタデータは #166 で Out of Scope 確定） | **承認済み逸脱（スキーマ分離の一部として記録）** |
| auth_codes | `code_hash` PK / `consumed_at` | `id` UUID PK + `code_hash` UNIQUE / `used` boolean | **承認済み逸脱（同上。意味論は同一: 単回消費の atomic 判定）** |
| 退会連動 | `DELETE BY user_id` を明示追加（CASCADE でも担保） | #170 で退会 tx に明示削除 2 段を統合 + FK CASCADE 防衛線 | ✓ |

検証テスト: 既存 `internal/repository/*_db_test.go` / `postgres_withdraw_integration_db_test.go`（#170）

## §1.6 ミドルウェア統合

| 項目 | SERVER.md §1.6 | 実装 | 整合 |
|---|---|---|---|
| 複合認証 | `NewBearerOrSessionMiddleware`（Bearer 先行 / 無ければ Cookie 委譲） | `middleware.NewBearerOrSessionMiddleware`（#169。概念コードと同名・同構造） | ✓ |
| 既存 SessionMiddleware | 置き換えない | 無変更・削除なし（nil verifier 時はそのまま返す縮退） | ✓ |
| context 注入 | 既存 `ContextWithUserID` → 下流変更不要 | 同一 context key を共用。下流 handler 変更ゼロ | ✓ |
| middleware 順序 | レート制限・logging の順序は不変 | 認証必須グループの 1 行差し替えのみ | ✓ |

検証テスト: `TestContract_BearerAccessToken_ReachesProtectedAPI_SameUserAsCookie` /
既存 `internal/middleware/bearer_or_session_test.go` / `TestNewRouter_BearerAuth_*`

## §1.7 セキュリティ要件

| 項目 | SERVER.md §1.7 | 実装 | 整合 |
|---|---|---|---|
| refresh token 平文保存禁止 | hash のみ | sha256 hash のみ永続化。ログは hash 先頭 8 文字 | ✓ |
| auth_code 60 秒・単回・PKCE 紐付け | 同左 | `NativeAuthCodeTTL` / `MarkUsed` / `pkce_challenge` カラム | ✓ |
| IP レート制限 | `/api/auth/token`・`/refresh` は IP 単位（既存 `unauthIPMW` 流用） | #171 で token / refresh / **revoke** の 3 ルートに適用（revoke は #168 の未認証化に伴い対象拡大。#171 requirements で確定） | ✓（仕様より広い保護） |
| 再利用検知 family 全失効 | 同左 | #168 strict 方針（並行 race 敗者も昇格） | ✓ |

## §1.8 受け入れ基準 ↔ 検証テスト ID

| 受け入れ基準（SERVER.md §1.8） | 対応テスト |
|---|---|
| 既存 Web（Cookie）が一切の変更なく従来通り動作する | 既存 Cookie 系統合テスト群（`TestIntegration_*` / `internal/middleware/session_test.go`）が無変更 green（NFR 2.1 / 2.2） |
| `flow=native` の OAuth がアプリスキームへ `auth_code` を返す | `TestContract_NativeCallbackLocation_AppSchemeAndAuthCode` / `TestE2E_NativeAuthFullFlow_DBBacked` step1〜2 |
| `POST /api/auth/token` が PKCE 検証の上でトークンを発行する | `TestE2E_NativeAuthFullFlow_DBBacked` step3（wrong verifier 拒否を含む）/ 既存 `internal/auth/pkce_test.go` |
| `POST /api/auth/refresh` がローテーションし、旧トークン再利用を family 失効で検知する | `TestE2E_NativeAuthFullFlow_DBBacked` step4・step4-reject / 既存 `TestIntegration_ReuseDetection_FamilyRevoked` |
| Bearer 付き既存 API（subscriptions/items/...）が Cookie と同じ結果を返す | `TestContract_BearerAccessToken_ReachesProtectedAPI_SameUserAsCookie`（応答 body 等価まで検証）/ `TestE2E_NativeAuthFullFlow_DBBacked` step6 |
| 退会で当該ユーザーの refresh_tokens / auth_codes が削除される | 既存 `TestWithdrawIntegration_NativeAuthCleanup`（#170） |

## 承認済み逸脱

### 1. revoke の認証境界（Req 4.3）

- **SERVER.md §1.3**: `POST /api/auth/revoke` を「Bearer 認証下」と注記
- **実装**: **未認証 + refresh token 所持 = 失効権限**（常に 204 / 冪等 / 列挙オラクルなし）
- **根拠**: #168 requirements.md の Open Questions で確定。Bearer middleware（#169）が
  後続 Issue であったこと、および RFC 7009 §2.1（OAuth 2.0 Token Revocation）が public
  client の revocation を token 所持ベースで許す慣行であることによる。失効は破壊のみで
  奪取に使えず、セキュリティ上の劣化はない。なお #171 で revoke にも IP レート制限を適用
  済み（未認証公開に伴う資源保護）
- **iOS への影響**: revoke 呼び出し時に `Authorization` ヘッダは不要（付与しても無害）

### 2. DB スキーマの分離（Req 4.4）

- **SERVER.md §1.5**: `refresh_tokens` 単一テーブル（`family_id` カラム）+ `auth_codes`
  （`code_hash` PK / `consumed_at`）の例示
- **実装**: #164 で `refresh_token_families` / `refresh_tokens` の 2 テーブルに分離
  （family 単位 revoke の正本を families.revoked_at に集約し、再利用検知の family 全失効
  を 1 UPDATE で原子化）。`auth_codes` は `id` UUID PK + `used` boolean（単回消費の
  atomic UPDATE 判定）。`device_label` / `last_used_at` は未実装（#166 で任意メタデータを
  Out of Scope と確定。導入時は spec 改訂を伴う）
- **iOS への影響**: なし（DB スキーマは API 契約に現れない。SERVER.md 自体が「Redis 等
  でも可」とし保存構造を実装裁量としている）

## 未承認差分エスカレーション方針（Req 4.5）

本確認（2026-06-12）では上記 2 件以外の差分は発見されなかった。今後、SERVER.md §1 と
実装の間に**承認されていない差分**を発見した場合は、本文書や実装側で勝手に確定させず、
以下の手順でエスカレーションする:

1. 差分の内容（SERVER.md の記述 / 実装の挙動 / 影響範囲）を Issue #163（umbrella）または
   該当する子 Issue のコメントに日本語で記録する
2. 「仕様側を直すか・実装側を直すか・承認済み逸脱として本文書に追記するか」の判断を
   人間に仰ぐ（推測で変更しない）
3. 判断確定後、本文書の対照表・承認済み逸脱節を更新する
