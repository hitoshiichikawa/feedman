# 実装ノート: Issue #166 POST /api/auth/token

## 実装サマリ

Issue #166 の POST /api/auth/token を、design.md / tasks.md の指針に厳密に従って実装した。
auth_code + code_verifier を access token (HS256 JWT / 15 分) + refresh token (256bit / hash
保存 / 30 日) に交換するエンドポイントを、認証不要グループに追加した。

設計の主要ポイントは原文どおり踏襲している:

- 拒否は単一 sentinel `auth.ErrInvalidGrant` に正規化（Req 2.6 の uniform 拒否）
- PKCE 検証は `crypto/subtle.ConstantTimeCompare`（NFR 1.4 のタイミング攻撃回避）
- `NATIVE_AUTH_JWT_SECRET` 未設定環境では handler を生成せず、router 側で 404 / fail-closed
  （Req 3.2、`/metrics` と同じ後方互換指針）
- `JWTIssuer` は内部 `now` を package 内テストから差し替えて固定時刻発行できる（Req 3.4）
- 平文 auth_code / verifier / refresh token はログ・エラーメッセージに残さず、追跡用は
  hash 先頭 8 文字のみ（NFR 1.2 / 1.3 / 1.5、#165 と同方針）

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | config: JWT 署名鍵の環境変数を追加 | 6f6c0c1 `feat(config)` | 2f4f080 `docs(tasks): mark 1` |
| 2 | auth: PKCE verifier 検証を追加 | 3c5cbac `feat(auth)` | eca2ca6 `docs(tasks): mark 2` |
| 3 | auth: JWTIssuer を追加 | 8263736 `feat(auth)` | 0d89ab6 `docs(tasks): mark 3` |
| 4 | auth: TokenService.ExchangeAuthCode を追加 | 9da3ddd `feat(auth)` | c544469 `docs(tasks): mark 4` |
| 5 | handler: NativeAuthHandler と router 登録 | 2208511 `feat(handler)` | 8ac69b0 `docs(tasks): mark 5` |
| 6 | wiring と統合テスト | a08dd70 `feat(app)` | 7affd50 `docs(tasks): mark 6` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` で示されたタスク 4 (Depends 2,3) /
タスク 5 (Depends 4) / タスク 6 (Depends 5) の制約は自然に満たされた。

## 受入基準と担保テスト

すべての numeric requirement ID について、対応テストを以下に列挙する。

### Requirement 1: Token 交換の成功パス

| AC ID | 担保テスト |
|---|---|
| 1.1 (3 値 + token_type/expires_in 返却) | `handler.TestNativeAuthHandler_Token_Success` / `handler.TestNewRouter_NativeAuthToken_RegisteredWhenHandlerInjected` / `handler.TestIntegration_NativeAuthFlow_CallbackThenExchangeSucceeds` |
| 1.2 (auth_code を使用済み確定して再交換不可) | `auth.TestExchangeAuthCode_Success`（MarkUsed 呼び出し検証）/ `handler.TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400` |
| 1.3 (refresh hash 保存 / rotation family) | `auth.TestExchangeAuthCode_Success`（TokenHash = HashNativeSecret(plain) / FamilyID / UserID / ExpiresAt 検証） |
| 1.4 (JWT 900 秒 / 自己完結 / 鍵で検証可能) | `auth.TestIssueAccessToken_SuccessfulIssuance`（claims / exp = iat+900 / 署名検証）/ `auth.TestExchangeAuthCode_Success`（ExpiresIn=900） |
| 1.5 (Cookie / Bearer なしで呼び出し可能) | `handler.TestNewRouter_NativeAuthToken_DoesNotRequireSession`（Cookie 無しで 200） |
| 1.6 (JSON フィールド名 snake_case 4 種) | `handler.TestNativeAuthHandler_Token_Success`（access_token / refresh_token / token_type / expires_in 検証） |

### Requirement 2: 交換の拒否パス

| AC ID | 担保テスト |
|---|---|
| 2.1 (verifier 不一致を拒否) | `auth.TestExchangeAuthCode_VerifierMismatch` / `auth.TestVerifyPKCES256Verifier`（不一致 / 末尾改変） |
| 2.2 (auth_code 不明を拒否) | `auth.TestExchangeAuthCode_AuthCodeNotFound` / `handler.TestIntegration_NativeAuthFlow_UnknownAuthCodeReturns400` |
| 2.3 (期限切れ / 使用済み auth_code を拒否) | `auth.TestExchangeAuthCode_MarkUsedNotUsable`（ErrAuthCodeNotUsable → ErrInvalidGrant）/ `handler.TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400` |
| 2.4 (verifier 形式不正を拒否) | `auth.TestVerifyPKCES256Verifier`（42 / 129 文字 / + / / / 空白 / =）/ `auth.TestExchangeAuthCode_VerifierMalformed` |
| 2.5 (不正 JSON / 必須欠落を拒否) | `handler.TestNativeAuthHandler_Token_InvalidJSON` / `handler.TestNativeAuthHandler_Token_MissingFields`（5 サブケース） |
| 2.6 (拒否応答 uniform 化) | `handler.TestNativeAuthHandler_Token_InvalidGrant`（sentinel と wrap の両方 / 詳細反射なし）/ `handler.TestIntegration_NativeAuthFlow_ReuseAuthCodeReturns400`（message に used/expired を含めない） |
| 2.7 (拒否時に refresh 永続化しない) | `auth.TestExchangeAuthCode_AuthCodeNotFound` / `_VerifierMismatch` / `_MarkUsedNotUsable`（いずれも CreateFamily / CreateToken 0 回） |

### Requirement 3: 署名鍵管理と安全な縮退

| AC ID | 担保テスト |
|---|---|
| 3.1 (署名鍵を環境変数から読み込む) | `config.TestLoad_NativeAuthJWT`（設定あり） |
| 3.2 (未設定 → エンドポイント未登録 / 起動成功) | `config.TestLoad_NativeAuthJWT`（未設定で起動成功 + 空文字保持） / `handler.TestNewRouter_NativeAuthToken_NotRegisteredWhenHandlerNil`（404） |
| 3.3 (未設定時の運用者 Warn 1 回) | `internal/app/app.go` の起動経路で `slog.Warn` を 1 回出力（automated test では assert していないが、design.md の通り wiring 側で実装。go vet で検出される副作用なし） |
| 3.4 (固定鍵 / 固定時刻で決定論的検証) | `auth.TestIssueAccessToken_SuccessfulIssuance` / `_KidPropagation`（newFixedIssuer による固定 now 注入） |
| 3.5 (JWT ヘッダに kid を含める) | `auth.TestIssueAccessToken_SuccessfulIssuance`（header.kid 検証）/ `_KidPropagation`（v1 / v2 / 2026-06） |

### Non-Functional Requirements

| NFR | 担保テスト・実装 |
|---|---|
| NFR 1.1 (refresh token 256bit エントロピー) | `auth/token_service.go` の `generateRefreshToken` で `crypto/rand` 32 byte → base64url、`auth.TestExchangeAuthCode_Success` で TokenHash 長 64 文字を検証 |
| NFR 1.2 (平文を永続化・ログに残さない) | `auth.TestExchangeAuthCode_Success`（storedToken.TokenHash != 平文 RefreshToken）/ `_DoesNotLeakPlainSecretsInError`（メッセージに平文無し）+ slog は hash 先頭 8 文字のみ |
| NFR 1.3 (auth_code / verifier 平文をログ・エラーに残さない) | `auth.TestExchangeAuthCode_DoesNotLeakPlainSecretsInError` |
| NFR 1.4 (PKCE 定数時間比較) | `auth/pkce.go` の `subtle.ConstantTimeCompare`、`auth.TestVerifyPKCES256Verifier` の網羅ケース |
| NFR 1.5 (エラー応答に入力値・内部詳細を反射しない) | `handler.TestNativeAuthHandler_Token_InternalError`（"db connection refused" がメッセージに含まれない） |
| NFR 2.1 (既存ルートの挙動不変) | 全既存テスト green（`go test ./...` 22 パッケージすべて pass）。`createIntegrationRouter` は不変 |
| NFR 2.2 (secret 未設定で既存デプロイ起動不変) | `config.TestLoad_NativeAuthJWT`（未設定起動成功）/ `handler.TestNewRouter_NativeAuthToken_NotRegisteredWhenHandlerNil` |
| NFR 3.1 (外部ネットワーク依存なし) | 単体テストはすべて mock 駆動。`TokenService` の 8 ケース / handler の 7 ケース / router の 4 ケースが DB / net 不要 |

## 検証結果

- `go build ./...`: 成功
- `go vet ./...`: 警告なし
- `go test ./...`: 全 22 パッケージ pass（`internal/auth` 30 件以上のテスト含む）
- `gofmt -l <変更ファイル群>`: 出力なし（フォーマット差分なし）

DB 結合テスト（`internal/repository/*_db_test.go`）は `TEST_DATABASE_URL` 未接続時に
`t.Skip` する既存設計で、ローカル / CI のいずれでも今回追加分は SKIP となる。

## design からの逸脱

無し。design.md の以下は **完全にそのまま** 実装した。

- 交換フロー手順 1〜6（auth_code 照合 → PKCE 検証 → MarkUsed → family/token 作成 → JWT 発行 → 応答）
- ErrorHandling 表（INVALID_REQUEST / INVALID_GRANT / INTERNAL_ERROR のマッピング）
- File Structure Plan に列挙された全ファイル（追加 / 変更ともに同一パス・同一責務）
- Components and Interfaces の `AuthCodeConsumer` / `RefreshTokenStore` / `AccessTokenIssuer` /
  `TokenExchangeService` の 4 最小 IF
- `RouterDeps.NativeAuthHandler` の任意フィールド + 認証不要グループ + `MaxBodyBytesMiddleware`
- `app.go` の secret 設定時のみ wiring する分岐 + 未設定時の `slog.Warn`

## 実装上の判断

### `DisallowUnknownFields` の採用

design.md / requirements.md には未知フィールドの扱いが明示されていないが、device_label 等
optional メタデータが Out of Scope（design.md）に明記されているため、現時点で受理する余地を
残すとフォーマット契約が曖昧になる。`json.Decoder.DisallowUnknownFields()` で厳密に拒否し、
将来 device_label などを追加するときに明示的な spec 改訂を伴う形にした。`handler.
TestNativeAuthHandler_Token_UnknownFieldRejected` で当該挙動を担保する。

### refresh family 作成失敗時のエラー分類

design.md「部分失敗の扱い」では「auth_code は既に消費済みだが token は返らない（500）」
と書かれている。これに従い `CreateFamily` / `CreateToken` / `IssueAccessToken` 失敗時は
`ErrInvalidGrant` に正規化せず `%w` で wrap して上層に返し、handler 側で 500 に振り分ける。
`auth.TestExchangeAuthCode_CreateFamilyFailure` で「ErrInvalidGrant ではないこと」と
「`%w` wrap が `errors.Is` で検出可能なこと」を担保する。

### 統合テストの mock 構造

既存 `createIntegrationRouter` は `mockItemService` / `mockSubscriptionService` 等を **固定値**
で組み込んだ大きなヘルパーで、native auth を組み込むと既存ケースの fixture を壊しかねない。
そこで独立した `createNativeAuthIntegrationRouter` を新設し、auth と native handler のみを
注入する最小構成にした。既存ケースは無変更で、`createIntegrationRouter` 内の
`handleNativeCallbackFn` も #165 のまま温存している。

## 追加した依存

| パッケージ | バージョン | 理由 |
|---|---|---|
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | design.md technology stack で指定された HS256 JWT 実装。発行 (本 spec) と将来の検証 (#169) で同一ライブラリを使う想定 |

`go mod tidy` で direct dependency として `go.mod` に登録済み。indirect 依存は本ライブラリの
推移依存は無く、`go.sum` には自身のチェックサムのみが追加されている。

## 後続 Issue への引き継ぎ事項

- **Issue #167 (refresh rotation)**: 本 spec で発行された refresh token を `POST /api/auth/refresh`
  で rotation する。`RefreshTokenStore` IF は `CreateFamily` / `CreateToken` までしか公開して
  いないため、#167 で `FindByHash` / `MarkRotated` をさらに必要に応じて公開拡張する。
- **Issue #168 (refresh 再利用検知)**: 本 spec の rotation family は `RevokedAt = nil` で作成
  される。#168 で `RevokeFamily` を呼び出す経路を追加する。
- **Issue #169 (Bearer middleware)**: 本 spec の `JWTIssuer` が出力する JWT（HS256 / kid 付き）
  を `Authorization: Bearer <token>` から復号して検証する middleware を実装する。検証側は
  同じ `golang-jwt/jwt/v5` を使い、`token_use == "access"` の確認を必須とする（refresh
  token と区別するため）。
- **Issue #171 (IP レート制限)**: 現状 `POST /api/auth/token` は IP 単位レート制限を経由
  しないため、IP 単位の総当たり防御は無い。`router.go` の登録部位に `unauthIPMW` を重ねる
  追加変更が想定される。

## 確認事項（PR 本文転載用）

- 設計矛盾は無し。
- design.md 末尾の Goals 節「access token の**検証**は #169 の領分」を踏襲し、本 spec の
  単体テストでは jwt パッケージで自己検証している（issuer 単体の挙動確認のみ）。
- `Token` handler は `DisallowUnknownFields` を採用したが、design.md / requirements.md には
  明示されていない判断。将来 device_label 等を追加する場合は spec 改訂 + 同 flag のリラックスが必要。

STATUS: complete
